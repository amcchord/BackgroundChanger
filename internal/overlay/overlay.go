// Package overlay provides functionality for rendering text overlays on images.
package overlay

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sync"

	"github.com/fogleman/gg"
)

//go:embed fonts/JetBrainsMono-Regular.ttf
var fontData embed.FS

var (
	cachedFontPath string
	fontPathOnce   sync.Once
	fontPathErr    error
)

// getFontPath extracts the embedded font to a temp file and returns its path.
// The font is only extracted once and cached.
func getFontPath() (string, error) {
	fontPathOnce.Do(func() {
		// Read the embedded font
		fontBytes, err := fontData.ReadFile("fonts/JetBrainsMono-Regular.ttf")
		if err != nil {
			fontPathErr = fmt.Errorf("failed to read embedded font: %v", err)
			return
		}

		// Create temp directory for the font
		tempDir := filepath.Join(os.TempDir(), "bgstatusservice")
		err = os.MkdirAll(tempDir, 0755)
		if err != nil {
			fontPathErr = fmt.Errorf("failed to create temp dir: %v", err)
			return
		}

		// Write font to temp file
		cachedFontPath = filepath.Join(tempDir, "JetBrainsMono-Regular.ttf")
		err = os.WriteFile(cachedFontPath, fontBytes, 0644)
		if err != nil {
			fontPathErr = fmt.Errorf("failed to write font file: %v", err)
			return
		}
	})

	if fontPathErr != nil {
		return "", fontPathErr
	}

	return cachedFontPath, nil
}

const (
	// FontSize is the size of the text in points.
	FontSize = 18
	// Padding is the padding around the text box.
	Padding = 20
	// LineSpacing is the spacing between lines.
	LineSpacing = 6
	// CornerRadius is the radius of the rounded corners on the background box.
	CornerRadius = 10
	// MarginRight is the margin from the right edge of the screen.
	MarginRight = 40
	// MarginTop is the margin from the top edge of the screen.
	MarginTop = 80
)

// TextColor represents the color scheme for text rendering.
type TextColor struct {
	Text       color.Color
	Background color.Color
	Border     color.Color
}

// LightOnDark returns a color scheme for dark backgrounds (white text).
func LightOnDark() TextColor {
	return TextColor{
		Text:       color.RGBA{255, 255, 255, 255},
		Background: color.RGBA{0, 0, 0, 160},
		Border:     color.RGBA{255, 255, 255, 80},
	}
}

// DarkOnLight returns a color scheme for light backgrounds (black text).
func DarkOnLight() TextColor {
	return TextColor{
		Text:       color.RGBA{0, 0, 0, 255},
		Background: color.RGBA{255, 255, 255, 180},
		Border:     color.RGBA{0, 0, 0, 80},
	}
}

// AnalyzeRegionBrightness analyzes the average brightness of a region in an image.
// Returns true if the region is light (brightness > 128), false if dark.
func AnalyzeRegionBrightness(img image.Image, x, y, width, height int) bool {
	bounds := img.Bounds()

	// Clamp region to image bounds
	if x < bounds.Min.X {
		x = bounds.Min.X
	}
	if y < bounds.Min.Y {
		y = bounds.Min.Y
	}
	if x+width > bounds.Max.X {
		width = bounds.Max.X - x
	}
	if y+height > bounds.Max.Y {
		height = bounds.Max.Y - y
	}

	if width <= 0 || height <= 0 {
		return false // Default to dark if region is invalid
	}

	var totalLuminance float64
	var pixelCount int

	// Sample every 4th pixel for performance
	step := 4
	for py := y; py < y+height; py += step {
		for px := x; px < x+width; px += step {
			r, g, b, _ := img.At(px, py).RGBA()
			// Convert from 16-bit to 8-bit
			r8 := float64(r >> 8)
			g8 := float64(g >> 8)
			b8 := float64(b >> 8)

			// Calculate luminance using Rec. 601 formula
			luminance := 0.299*r8 + 0.587*g8 + 0.114*b8
			totalLuminance += luminance
			pixelCount++
		}
	}

	if pixelCount == 0 {
		return false
	}

	avgLuminance := totalLuminance / float64(pixelCount)
	return avgLuminance > 128
}

// ChooseTextColor analyzes the upper-right region of an image and returns appropriate colors.
func ChooseTextColor(img image.Image, boxWidth, boxHeight int) TextColor {
	bounds := img.Bounds()
	imgWidth := bounds.Max.X - bounds.Min.X

	// Analyze the region where the text will be placed (upper right)
	regionX := imgWidth - boxWidth - MarginRight
	regionY := MarginTop
	regionWidth := boxWidth + (Padding * 2)
	regionHeight := boxHeight + (Padding * 2)

	if regionX < 0 {
		regionX = 0
	}

	isLight := AnalyzeRegionBrightness(img, regionX, regionY, regionWidth, regionHeight)

	if isLight {
		return DarkOnLight()
	}
	return LightOnDark()
}

// RenderOverlay renders text lines onto an image in the upper right corner.
func RenderOverlay(img image.Image, lines []string) (image.Image, error) {
	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

	// Create a new drawing context
	dc := gg.NewContext(width, height)

	// Draw the original image
	dc.DrawImage(img, 0, 0)

	// Load the font
	fontFile, err := getFontPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get font path: %v", err)
	}

	err = dc.LoadFontFace(fontFile, FontSize)
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %v", err)
	}

	// Calculate text dimensions
	var maxLineWidth float64
	lineHeight := float64(FontSize) + LineSpacing

	for _, line := range lines {
		w, _ := dc.MeasureString(line)
		if w > maxLineWidth {
			maxLineWidth = w
		}
	}

	textHeight := lineHeight * float64(len(lines))
	boxWidth := maxLineWidth + (Padding * 2)
	boxHeight := textHeight + (Padding * 2) - LineSpacing

	// Choose text color based on background brightness
	colors := ChooseTextColor(img, int(boxWidth), int(boxHeight))

	// Calculate position (upper right corner)
	boxX := float64(width) - boxWidth - MarginRight
	boxY := float64(MarginTop)

	// Draw semi-transparent background with rounded corners
	r, g, b, a := colors.Background.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)
	dc.DrawRoundedRectangle(boxX, boxY, boxWidth, boxHeight, CornerRadius)
	dc.Fill()

	// Draw border
	r, g, b, a = colors.Border.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)
	dc.SetLineWidth(1)
	dc.DrawRoundedRectangle(boxX, boxY, boxWidth, boxHeight, CornerRadius)
	dc.Stroke()

	// Draw text
	r, g, b, a = colors.Text.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)

	textX := boxX + Padding
	textY := boxY + Padding + float64(FontSize)

	for _, line := range lines {
		dc.DrawString(line, textX, textY)
		textY += lineHeight
	}

	return dc.Image(), nil
}

// RenderOverlayWithColors renders text with explicitly specified colors.
func RenderOverlayWithColors(img image.Image, lines []string, colors TextColor) (image.Image, error) {
	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

	// Create a new drawing context
	dc := gg.NewContext(width, height)

	// Draw the original image
	dc.DrawImage(img, 0, 0)

	// Load the font
	fontFile, err := getFontPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get font path: %v", err)
	}

	err = dc.LoadFontFace(fontFile, FontSize)
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %v", err)
	}

	// Calculate text dimensions
	var maxLineWidth float64
	lineHeight := float64(FontSize) + LineSpacing

	for _, line := range lines {
		w, _ := dc.MeasureString(line)
		if w > maxLineWidth {
			maxLineWidth = w
		}
	}

	textHeight := lineHeight * float64(len(lines))
	boxWidth := maxLineWidth + (Padding * 2)
	boxHeight := textHeight + (Padding * 2) - LineSpacing

	// Calculate position (upper right corner)
	boxX := float64(width) - boxWidth - MarginRight
	boxY := float64(MarginTop)

	// Draw semi-transparent background with rounded corners
	r, g, b, a := colors.Background.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)
	dc.DrawRoundedRectangle(boxX, boxY, boxWidth, boxHeight, CornerRadius)
	dc.Fill()

	// Draw border
	r, g, b, a = colors.Border.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)
	dc.SetLineWidth(1)
	dc.DrawRoundedRectangle(boxX, boxY, boxWidth, boxHeight, CornerRadius)
	dc.Stroke()

	// Draw text
	r, g, b, a = colors.Text.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)

	textX := boxX + Padding
	textY := boxY + Padding + float64(FontSize)

	for _, line := range lines {
		dc.DrawString(line, textX, textY)
		textY += lineHeight
	}

	return dc.Image(), nil
}
