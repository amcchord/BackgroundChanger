// Package overlay renders a resolution-aware, fastfetch-inspired status image.
package overlay

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/amcchord/BackgroundChanger/internal/config"
	"github.com/amcchord/BackgroundChanger/internal/sysinfo"
	"github.com/fogleman/gg"
	xdraw "golang.org/x/image/draw"
)

//go:embed fonts/JetBrainsMono-Regular.ttf
var fontFS embed.FS

var (
	fontOnce sync.Once
	fontPath string
	fontErr  error
)

func Dimensions(cfg config.Config, snapshot sysinfo.Snapshot) (int, int) {
	if cfg.Width != 0 && cfg.Height != 0 {
		return cfg.Width, cfg.Height
	}
	if snapshot.DisplayWidth >= 800 && snapshot.DisplayHeight >= 600 {
		return snapshot.DisplayWidth, snapshot.DisplayHeight
	}
	return 1920, 1080
}

func Render(snapshot sysinfo.Snapshot, cfg config.Config) (image.Image, error) {
	width, height := Dimensions(cfg, snapshot)
	base, err := background(cfg.BaseImage, width, height)
	if err != nil {
		return nil, err
	}
	dc := gg.NewContextForImage(base)
	scale := min(float64(width)/1920, float64(height)/1080)
	if scale < 0.52 {
		scale = 0.52
	}
	padding := 64.0 * scale
	panelWidth := (float64(width) - padding*2) * 0.39
	panelHeight := min(470*scale, float64(height)*0.48)
	panelY := padding + 190*scale
	rightPanelX := float64(width) - padding - panelWidth

	// The identity and two side panels reserve both Windows-owned clock regions:
	// top-center on Windows 11 and lower-left on Windows 10. The center and
	// bottom-left remain background-only so Windows can draw over them cleanly.
	drawMark(dc, padding, padding, scale)
	if err := setFont(dc, 18*scale); err != nil {
		return nil, err
	}
	dc.SetColor(color.RGBA{137, 207, 240, 255})
	dc.DrawString("PRE-LOGIN MACHINE STATUS", padding+70*scale, padding+19*scale)
	if err := setFont(dc, 58*scale); err != nil {
		return nil, err
	}
	dc.SetColor(color.White)
	dc.DrawString(strings.ToUpper(snapshot.Hostname), padding, padding+105*scale)
	if err := setFont(dc, 17*scale); err != nil {
		return nil, err
	}
	dc.SetColor(color.RGBA{174, 192, 210, 255})
	dc.DrawString("Identify this machine before anyone signs in", padding, padding+139*scale)

	left := []row{
		{"OS", joinNonEmpty(snapshot.OS, snapshot.Version)},
		{"BUILD", snapshot.Build},
		{"CPU", snapshot.CPU},
		{"GPU", snapshot.GPU},
		{"MEMORY", snapshot.Memory},
		{"SYSTEM DISK", snapshot.Disk},
	}
	right := []row{
		{"IP", valueOr(strings.Join(snapshot.IPs, ", "), "Waiting for network")},
		{"SERIAL", snapshot.Serial},
		{"UPTIME", snapshot.Uptime},
		{"SERVICES", fmt.Sprintf("%d of %d running", snapshot.ServicesRunning, snapshot.ServicesTotal)},
		{"RESTART", rebootLabel(snapshot.PendingReboot)},
	}

	drawPanel(dc, padding, panelY, panelWidth, panelHeight, scale, "SYSTEM", left)
	drawPanel(dc, rightPanelX, panelY, panelWidth, panelHeight, scale, "HEALTH", right)
	drawHealth(dc, rightPanelX, panelY, panelWidth, panelHeight, scale, snapshot)

	if err := setFont(dc, 14*scale); err != nil {
		return nil, err
	}
	dc.SetColor(color.RGBA{153, 172, 191, 255})
	footer := "Updated " + snapshot.RefreshedAt.Format("2006-01-02 15:04:05 MST")
	dc.DrawStringAnchored(footer, float64(width)-padding, float64(height)-18*scale, 1, 1)
	return dc.Image(), nil
}

func RenderToFile(path string, snapshot sysinfo.Snapshot, cfg config.Config) error {
	img, err := Render(snapshot, cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	encodeErr := jpeg.Encode(f, img, &jpeg.Options{Quality: 94})
	closeErr := f.Close()
	if encodeErr != nil {
		_ = os.Remove(tmp)
		return encodeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

type row struct{ label, value string }

func drawPanel(dc *gg.Context, x, y, width, height, scale float64, title string, rows []row) {
	dc.SetRGBA(0.025, 0.055, 0.10, 0.89)
	dc.DrawRoundedRectangle(x, y, width, height, 22*scale)
	dc.Fill()
	dc.SetRGBA(0.32, 0.72, 0.93, 0.23)
	dc.SetLineWidth(1.4 * scale)
	dc.DrawRoundedRectangle(x, y, width, height, 22*scale)
	dc.Stroke()
	_ = setFont(dc, 16*scale)
	dc.SetColor(color.RGBA{94, 203, 247, 255})
	dc.DrawString(title, x+28*scale, y+38*scale)

	rowY := y + 82*scale
	step := 58 * scale
	for _, item := range rows {
		_ = setFont(dc, 12*scale)
		dc.SetColor(color.RGBA{128, 153, 178, 255})
		dc.DrawString(item.label, x+28*scale, rowY)
		_ = setFont(dc, 17*scale)
		dc.SetColor(color.RGBA{239, 246, 252, 255})
		dc.DrawString(compact(item.value, int(width/(10*scale))), x+28*scale, rowY+23*scale)
		rowY += step
		if rowY > y+height-35*scale {
			break
		}
	}
}

func drawHealth(dc *gg.Context, x, y, width, height, scale float64, snapshot sysinfo.Snapshot) {
	startY := y + 82*scale + 5*58*scale
	if startY > y+height-32*scale {
		return
	}
	items := snapshot.CriticalServices
	if len(items) > 4 {
		items = items[:4]
	}
	_ = setFont(dc, 13*scale)
	for i, service := range items {
		dotX := x + 29*scale + float64(i%2)*(width/2-18*scale)
		dotY := startY + float64(i/2)*25*scale
		if service.Running {
			dc.SetColor(color.RGBA{71, 220, 151, 255})
		} else {
			dc.SetColor(color.RGBA{255, 104, 118, 255})
		}
		dc.DrawCircle(dotX, dotY-4*scale, 4*scale)
		dc.Fill()
		dc.SetColor(color.RGBA{202, 216, 229, 255})
		dc.DrawString(service.Name, dotX+11*scale, dotY)
	}
}

func drawMark(dc *gg.Context, x, y, scale float64) {
	size := 21 * scale
	gap := 5 * scale
	colors := []color.RGBA{
		{74, 190, 245, 255}, {44, 145, 223, 255},
		{50, 154, 225, 255}, {35, 111, 194, 255},
	}
	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			dc.SetColor(colors[row*2+col])
			dc.DrawRoundedRectangle(x+float64(col)*(size+gap), y+float64(row)*(size+gap), size, size, 3*scale)
			dc.Fill()
		}
	}
}

func background(path string, width, height int) (image.Image, error) {
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open base image: %w", err)
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decode base image: %w", err)
		}
		return cover(img, width, height), nil
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		fy := float64(y) / float64(height)
		for x := 0; x < width; x++ {
			fx := float64(x) / float64(width)
			glow := max(0, 1-((fx-0.82)*(fx-0.82)+(fy-0.14)*(fy-0.14))*3.2)
			r := uint8(7 + 5*fy + 5*glow)
			g := uint8(20 + 15*fy + 28*glow)
			b := uint8(38 + 28*fy + 42*glow)
			img.SetRGBA(x, y, color.RGBA{r, g, b, 255})
		}
	}
	return img, nil
}

func cover(source image.Image, width, height int) image.Image {
	sw, sh := source.Bounds().Dx(), source.Bounds().Dy()
	scale := max(float64(width)/float64(sw), float64(height)/float64(sh))
	dw, dh := int(float64(sw)*scale), int(float64(sh)*scale)
	tmp := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(tmp, tmp.Bounds(), source, source.Bounds(), xdraw.Over, nil)
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	offset := image.Pt((dw-width)/2, (dh-height)/2)
	xdraw.Draw(result, result.Bounds(), tmp, offset, xdraw.Src)
	return result
}

func setFont(dc *gg.Context, size float64) error {
	fontOnce.Do(func() {
		data, err := fontFS.ReadFile("fonts/JetBrainsMono-Regular.ttf")
		if err != nil {
			fontErr = err
			return
		}
		dir := filepath.Join(os.TempDir(), "BackgroundChanger")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fontErr = err
			return
		}
		fontPath = filepath.Join(dir, "JetBrainsMono-Regular.ttf")
		fontErr = os.WriteFile(fontPath, data, 0o644)
	})
	if fontErr != nil {
		return fontErr
	}
	return dc.LoadFontFace(fontPath, size)
}

func rebootLabel(pending bool) string {
	if pending {
		return "Required"
	}
	return "Not pending"
}

func joinNonEmpty(values ...string) string {
	var nonempty []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			nonempty = append(nonempty, strings.TrimSpace(value))
		}
	}
	return strings.Join(nonempty, " · ")
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func compact(value string, maxRunes int) string {
	runes := []rune(strings.Join(strings.Fields(value), " "))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	if maxRunes < 2 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:maxRunes-1])) + "…"
}
