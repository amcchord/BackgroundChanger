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

	"github.com/amcchord/WallpaperIdentity/v4/internal/branding"
	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
	"github.com/amcchord/WallpaperIdentity/v4/internal/sysinfo"
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

const overlayBrandLabel = "W:ID  •  WALLPAPER IDENTITY"

func Dimensions(cfg config.Config, snapshot sysinfo.Snapshot) (int, int) {
	if cfg.Width != 0 && cfg.Height != 0 {
		return cfg.Width, cfg.Height
	}
	if snapshot.DisplayWidth >= 600 && snapshot.DisplayWidth <= 7680 && snapshot.DisplayHeight >= 600 && snapshot.DisplayHeight <= 7680 {
		return snapshot.DisplayWidth, snapshot.DisplayHeight
	}
	return fallbackDimensions(snapshot.DisplayWidth, snapshot.DisplayHeight)
}

func Render(snapshot sysinfo.Snapshot, cfg config.Config) (image.Image, error) {
	width, height := Dimensions(cfg, snapshot)
	theme := themeFor(cfg)
	base, err := background(cfg.BaseImage, width, height, theme)
	if err != nil {
		return nil, err
	}
	dc := gg.NewContextForImage(base)
	left, right := panelRows(snapshot, cfg)
	criticalRows := 0
	if cfg.Show.CriticalServices {
		criticalRows = (min(len(snapshot.CriticalServices), 4) + 1) / 2
	}
	layout := calculateLayout(width, height, len(left), len(right), criticalRows)
	scale, padding := layout.Scale, layout.Padding

	// The identity and two side panels reserve both Windows-owned clock regions:
	// top-center on Windows 11 and lower-left on Windows 10. The center and
	// bottom-left remain background-only on normal and wide displays. Tall
	// displays stack both panels below the identity header.
	logo, err := branding.Logo()
	if err != nil {
		return nil, err
	}
	drawLogo(dc, logo, padding, padding-4*scale, 62*scale)
	if err := setFont(dc, max(10, 18*scale)); err != nil {
		return nil, err
	}
	dc.SetColor(theme.Accent)
	dc.DrawString(overlayBrandLabel, padding+78*scale, padding+19*scale)
	dc.SetColor(theme.Headline)
	drawFittedText(dc, strings.ToUpper(snapshot.Hostname), padding, padding+105*scale, layout.HeaderMaxWidth, max(24, 58*scale), max(16, 27*scale))
	if err := setFont(dc, max(10, 17*scale)); err != nil {
		return nil, err
	}
	dc.SetColor(theme.Subtitle)
	drawFittedText(dc, "Identify this machine before anyone signs in", padding, padding+139*scale, layout.HeaderMaxWidth, max(10, 17*scale), 9)

	if len(left) > 0 {
		drawPanel(dc, layout.Left, scale, layout.RowStep, theme, "SYSTEM", left)
	}
	if len(right) > 0 || criticalRows > 0 {
		drawPanel(dc, layout.Right, scale, layout.RowStep, theme, "HEALTH", right)
		if cfg.Show.CriticalServices {
			drawHealth(dc, layout.Right, scale, layout.RowStep, theme, len(right), snapshot)
		}
	}

	if err := setFont(dc, max(9, 14*scale)); err != nil {
		return nil, err
	}
	dc.SetColor(theme.Footer)
	footer := generatedAtLabel(snapshot)
	footer, footerSize := fittedText(dc, footer, float64(width)-2*padding, max(9, 14*scale), 8)
	_ = setFont(dc, footerSize)
	dc.DrawStringAnchored(footer, float64(width)-padding, float64(height)-18*scale, 1, 1)
	return dc.Image(), nil
}

// BackgroundPreview renders only the selected backdrop. The installer uses it
// for color swatches without duplicating the production gradient implementation.
func BackgroundPreview(cfg config.Config, width, height int) (image.Image, error) {
	return background(cfg.BaseImage, width, height, themeFor(cfg))
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

func panelRows(snapshot sysinfo.Snapshot, cfg config.Config) (left, right []row) {
	show := cfg.Show
	if show.OS {
		left = append(left, row{"OS", joinNonEmpty(snapshot.OS, snapshot.Version)})
	}
	if show.Build {
		left = append(left, row{"BUILD", snapshot.Build})
	}
	if show.CPU {
		left = append(left, row{"CPU", snapshot.CPU})
	}
	if show.GPU {
		left = append(left, row{"GPU", snapshot.GPU})
	}
	if show.Memory {
		left = append(left, row{"MEMORY", snapshot.Memory})
	}
	if show.Disk {
		left = append(left, row{"SYSTEM DISK", snapshot.Disk})
	}
	if show.IP {
		right = append(right, row{"IP", valueOr(strings.Join(snapshot.IPs, ", "), "Waiting for network")})
	}
	if show.Serial {
		right = append(right, row{"SERIAL", snapshot.Serial})
	}
	if show.Uptime {
		right = append(right, row{"UPTIME", snapshot.Uptime})
	}
	if show.Services {
		right = append(right, row{"SERVICES", fmt.Sprintf("%d of %d running", snapshot.ServicesRunning, snapshot.ServicesTotal)})
	}
	if show.Restart {
		right = append(right, row{"RESTART", rebootLabel(snapshot.PendingReboot)})
	}
	if show.FailedAutoServices {
		value := "None"
		if len(snapshot.FailedAutoServices) > 0 {
			value = fmt.Sprintf("%d: %s", len(snapshot.FailedAutoServices), strings.Join(snapshot.FailedAutoServices, ", "))
		}
		right = append(right, row{"AUTO FAILURES", value})
	}
	return left, right
}

func generatedAtLabel(snapshot sysinfo.Snapshot) string {
	return "Generated at " + snapshot.RefreshedAt.Format("2006-01-02 15:04:05 MST")
}

func drawPanel(dc *gg.Context, panel panelRect, scale, rowStep float64, theme renderTheme, title string, rows []row) {
	dc.SetColor(theme.Panel)
	dc.DrawRoundedRectangle(panel.X, panel.Y, panel.Width, panel.Height, 22*scale)
	dc.Fill()
	dc.SetColor(theme.PanelBorder)
	dc.SetLineWidth(1.4 * scale)
	dc.DrawRoundedRectangle(panel.X, panel.Y, panel.Width, panel.Height, 22*scale)
	dc.Stroke()
	_ = setFont(dc, max(9, 16*scale))
	dc.SetColor(theme.Accent)
	dc.DrawString(title, panel.X+28*scale, panel.Y+38*scale)

	rowY := panel.Y + 82*scale
	for _, item := range rows {
		_ = setFont(dc, max(8, 12*scale))
		dc.SetColor(theme.Label)
		dc.DrawString(item.label, panel.X+28*scale, rowY)
		dc.SetColor(theme.Value)
		drawFittedText(dc, item.value, panel.X+28*scale, rowY+23*scale, panel.Width-56*scale, max(10, 17*scale), 8)
		rowY += rowStep
	}
}

func drawHealth(dc *gg.Context, panel panelRect, scale, rowStep float64, theme renderTheme, precedingRows int, snapshot sysinfo.Snapshot) {
	startY := panel.Y + 82*scale + float64(precedingRows)*rowStep
	if startY > panel.Y+panel.Height-20*scale {
		return
	}
	items := snapshot.CriticalServices
	if len(items) > 4 {
		items = items[:4]
	}
	_ = setFont(dc, max(8, 13*scale))
	for i, service := range items {
		columnWidth := panel.Width / 2
		dotX := panel.X + 29*scale + float64(i%2)*(columnWidth-18*scale)
		dotY := startY + float64(i/2)*25*scale
		if service.Running {
			dc.SetColor(color.RGBA{71, 220, 151, 255})
		} else {
			dc.SetColor(color.RGBA{255, 104, 118, 255})
		}
		dc.DrawCircle(dotX, dotY-4*scale, 4*scale)
		dc.Fill()
		dc.SetColor(theme.Service)
		drawFittedText(dc, service.Name, dotX+11*scale, dotY, columnWidth-52*scale, max(8, 13*scale), 7)
	}
}

func drawLogo(dc *gg.Context, logo image.Image, x, y, size float64) {
	dc.Push()
	dc.Translate(x, y)
	dc.Scale(size/float64(logo.Bounds().Dx()), size/float64(logo.Bounds().Dy()))
	dc.DrawImage(logo, 0, 0)
	dc.Pop()
}

func background(path string, width, height int, theme renderTheme) (image.Image, error) {
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
			vertical := clampFloat(fy*0.88+fx*0.12, 0, 1)
			glow := max(0, 1-((fx-0.82)*(fx-0.82)+(fy-0.14)*(fy-0.14))*3.2)
			base := mixColor(theme.GradientStart, theme.GradientEnd, vertical)
			img.SetRGBA(x, y, mixColor(base, theme.Glow, glow*0.22))
		}
	}
	return img, nil
}

func mixColor(first, second color.RGBA, amount float64) color.RGBA {
	amount = clampFloat(amount, 0, 1)
	mix := func(a, b uint8) uint8 { return uint8(float64(a)*(1-amount) + float64(b)*amount + 0.5) }
	return color.RGBA{mix(first.R, second.R), mix(first.G, second.G), mix(first.B, second.B), 255}
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
		dir := filepath.Join(os.TempDir(), "WallpaperIdentity")
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

func drawFittedText(dc *gg.Context, value string, x, y, maxWidth, preferredSize, minimumSize float64) {
	text, size := fittedText(dc, value, maxWidth, preferredSize, minimumSize)
	_ = setFont(dc, size)
	dc.DrawString(text, x, y)
}

func fittedText(dc *gg.Context, value string, maxWidth, preferredSize, minimumSize float64) (string, float64) {
	value = strings.Join(strings.Fields(value), " ")
	if preferredSize < minimumSize {
		preferredSize = minimumSize
	}
	for size := preferredSize; size >= minimumSize; size -= 0.5 {
		_ = setFont(dc, size)
		width, _ := dc.MeasureString(value)
		if width <= maxWidth {
			return value, size
		}
	}
	_ = setFont(dc, minimumSize)
	return truncateMeasured(value, maxWidth, func(candidate string) float64 {
		width, _ := dc.MeasureString(candidate)
		return width
	}), minimumSize
}

func truncateMeasured(value string, maxWidth float64, measure func(string) float64) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || measure(value) <= maxWidth {
		return value
	}
	ellipsis := "…"
	if maxWidth <= 0 || measure(ellipsis) > maxWidth {
		return ""
	}
	runes := []rune(value)
	low, high := 0, len(runes)
	for low < high {
		middle := (low + high + 1) / 2
		candidate := strings.TrimSpace(string(runes[:middle])) + ellipsis
		if measure(candidate) <= maxWidth {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return strings.TrimSpace(string(runes[:low])) + ellipsis
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
