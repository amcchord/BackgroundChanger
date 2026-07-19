package overlay

import (
	"image/color"
	"math"

	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
)

type panelRect struct {
	X, Y, Width, Height float64
}

type renderLayout struct {
	Scale          float64
	Padding        float64
	HeaderMaxWidth float64
	RowStep        float64
	Stacked        bool
	Left           panelRect
	Right          panelRect
}

func calculateLayout(width, height, leftRows, rightRows, criticalRows int) renderLayout {
	w, h := float64(width), float64(height)
	aspect := w / h
	scale := math.Min(w/1920, h/1080)
	scale = clampFloat(scale, 0.36, 2.5)
	padding := clampFloat(64*scale, 22, w*0.065)
	panelY := padding + 190*scale
	footerSpace := math.Max(34, 42*scale)
	available := math.Max(1, h-panelY-footerSpace)
	rowStep := 54 * scale
	gap := 18 * scale

	layout := renderLayout{
		Scale: scale, Padding: padding, RowStep: rowStep,
		HeaderMaxWidth: math.Max(1, math.Min(w*0.44-padding, w-padding*2)),
	}
	if aspect < 1.15 {
		layout.Stacked = true
		panelWidth := w - 2*padding
		overhead := 2*(82+24)*scale + float64(criticalRows)*25*scale + gap
		rowCount := leftRows + rightRows
		if rowCount > 0 && overhead+float64(rowCount)*rowStep > available {
			rowStep = math.Max(24*scale, (available-overhead)/float64(rowCount))
		}
		leftHeight := panelHeight(scale, rowStep, leftRows, 0)
		rightHeight := panelHeight(scale, rowStep, rightRows, criticalRows)
		if leftHeight+gap+rightHeight > available {
			shrink := available / (leftHeight + gap + rightHeight)
			leftHeight *= shrink
			rightHeight *= shrink
			gap *= shrink
		}
		layout.RowStep = rowStep
		layout.HeaderMaxWidth = math.Max(1, math.Min(w*0.46-padding, w-padding*2))
		layout.Left = panelRect{X: padding, Y: panelY, Width: panelWidth, Height: leftHeight}
		layout.Right = panelRect{X: padding, Y: panelY + leftHeight + gap, Width: panelWidth, Height: rightHeight}
		return layout
	}

	usableWidth := w - 2*padding
	panelFraction := 0.39
	if aspect >= 2.05 {
		panelFraction = 0.31
		layout.HeaderMaxWidth = math.Max(1, math.Min(w*0.34-padding, w-padding*2))
	}
	panelWidth := usableWidth * panelFraction
	leftNeeded := panelHeight(scale, rowStep, leftRows, 0)
	rightNeeded := panelHeight(scale, rowStep, rightRows, criticalRows)
	needed := math.Max(leftNeeded, rightNeeded)
	if needed > available {
		maxRows := max(leftRows, rightRows)
		overhead := (82+24)*scale + float64(criticalRows)*25*scale
		if maxRows > 0 {
			rowStep = math.Max(24*scale, (available-overhead)/float64(maxRows))
		}
		needed = math.Max(panelHeight(scale, rowStep, leftRows, 0), panelHeight(scale, rowStep, rightRows, criticalRows))
	}
	panelHeight := math.Min(available, needed)
	layout.RowStep = rowStep
	layout.Left = panelRect{X: padding, Y: panelY, Width: panelWidth, Height: panelHeight}
	layout.Right = panelRect{X: w - padding - panelWidth, Y: panelY, Width: panelWidth, Height: panelHeight}
	return layout
}

func panelHeight(scale, rowStep float64, rows, criticalRows int) float64 {
	content := (82+24)*scale + float64(rows)*rowStep + float64(criticalRows)*25*scale
	return math.Max(228*scale, content)
}

func fallbackDimensions(displayWidth, displayHeight int) (int, int) {
	if displayWidth <= 0 || displayHeight <= 0 {
		return 1920, 1080
	}
	aspect := float64(displayWidth) / float64(displayHeight)
	switch {
	case aspect < 0.85:
		return 1080, 1920
	case aspect < 1.45:
		return 1600, 1200
	case aspect < 1.70:
		return 1920, 1200
	case aspect < 2.10:
		return 1920, 1080
	default:
		return 2560, 1080
	}
}

func clampFloat(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}

type renderTheme struct {
	GradientStart color.RGBA
	GradientEnd   color.RGBA
	Glow          color.RGBA
	Accent        color.RGBA
	Headline      color.RGBA
	Subtitle      color.RGBA
	Footer        color.RGBA
	Panel         color.RGBA
	PanelBorder   color.RGBA
	Label         color.RGBA
	Value         color.RGBA
	Service       color.RGBA
}

func themeFor(cfg config.Config) renderTheme {
	darkGradients := map[string][3]color.RGBA{
		config.BackgroundBlue:   {{7, 20, 38, 255}, {16, 47, 85, 255}, {31, 99, 151, 255}},
		config.BackgroundTeal:   {{7, 29, 33, 255}, {14, 74, 79, 255}, {24, 122, 122, 255}},
		config.BackgroundGreen:  {{11, 29, 22, 255}, {23, 75, 53, 255}, {43, 125, 82, 255}},
		config.BackgroundPurple: {{23, 17, 38, 255}, {60, 40, 87, 255}, {104, 70, 139, 255}},
		config.BackgroundSlate:  {{21, 25, 31, 255}, {57, 68, 82, 255}, {93, 112, 132, 255}},
		config.BackgroundCopper: {{34, 20, 14, 255}, {90, 53, 38, 255}, {145, 86, 55, 255}},
	}
	lightGradients := map[string][3]color.RGBA{
		config.BackgroundBlue:   {{220, 234, 247, 255}, {143, 185, 221, 255}, {75, 132, 178, 255}},
		config.BackgroundTeal:   {{216, 238, 233, 255}, {130, 191, 182, 255}, {55, 139, 133, 255}},
		config.BackgroundGreen:  {{220, 235, 221, 255}, {145, 189, 151, 255}, {70, 137, 82, 255}},
		config.BackgroundPurple: {{231, 224, 239, 255}, {173, 151, 194, 255}, {115, 85, 144, 255}},
		config.BackgroundSlate:  {{227, 231, 235, 255}, {165, 175, 185, 255}, {91, 108, 124, 255}},
		config.BackgroundCopper: {{240, 226, 215, 255}, {197, 160, 131, 255}, {143, 91, 59, 255}},
	}
	name := cfg.BackgroundColor
	if _, ok := darkGradients[name]; !ok {
		name = config.BackgroundBlue
	}
	if cfg.BackgroundMode == config.BackgroundLight {
		gradient := lightGradients[name]
		return renderTheme{
			GradientStart: gradient[0], GradientEnd: gradient[1], Glow: gradient[2], Accent: gradient[2],
			Headline: color.RGBA{15, 37, 55, 255}, Subtitle: color.RGBA{37, 62, 80, 255}, Footer: color.RGBA{42, 65, 82, 255},
			Panel: color.RGBA{246, 249, 251, 255}, PanelBorder: color.RGBA{gradient[2].R, gradient[2].G, gradient[2].B, 255},
			Label: color.RGBA{73, 95, 111, 255}, Value: color.RGBA{19, 42, 58, 255}, Service: color.RGBA{47, 67, 82, 255},
		}
	}
	gradient := darkGradients[name]
	return renderTheme{
		GradientStart: gradient[0], GradientEnd: gradient[1], Glow: gradient[2], Accent: color.RGBA{94, 203, 247, 255},
		Headline: color.RGBA{255, 255, 255, 255}, Subtitle: color.RGBA{174, 192, 210, 255}, Footer: color.RGBA{153, 172, 191, 255},
		Panel: color.RGBA{6, 14, 26, 255}, PanelBorder: color.RGBA{44, 105, 139, 255},
		Label: color.RGBA{128, 153, 178, 255}, Value: color.RGBA{239, 246, 252, 255}, Service: color.RGBA{202, 216, 229, 255},
	}
}
