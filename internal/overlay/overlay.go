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

	"github.com/backgroundchanger/internal/sysinfo"
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

// Baseline dimensions (designed for 1920x1080)
const (
	BaseWidth  = 1920
	BaseHeight = 1080

	// BaseFontSize is the base size of the text in points for 1920x1080.
	BaseFontSize = 18
	// BasePadding is the base padding around the text box.
	BasePadding = 20
	// BaseLineSpacing is the base spacing between lines.
	BaseLineSpacing = 6
	// BaseCornerRadius is the base radius of the rounded corners.
	BaseCornerRadius = 10
	// BaseMarginRight is the base margin from the right edge.
	BaseMarginRight = 40
	// BaseMarginLeft is the base margin from the left edge.
	BaseMarginLeft = 40
	// BaseMarginTop is the base margin from the top edge.
	BaseMarginTop = 80

	// MinScaleFactor is the minimum scale factor (for 1024x768 readability).
	MinScaleFactor = 0.6
	// MaxScaleFactor caps scaling on high-res displays to keep text compact.
	// At 1.0, text on 4K displays stays the same size as on 1080p.
	MaxScaleFactor = 1.0
	// MinFontSize is the minimum font size for readability.
	MinFontSize = 12
)

// Legacy constants for backward compatibility
const (
	FontSize     = BaseFontSize
	Padding      = BasePadding
	LineSpacing  = BaseLineSpacing
	CornerRadius = BaseCornerRadius
	MarginRight  = BaseMarginRight
	MarginTop    = BaseMarginTop
)

// ScaledDimensions holds all the scaled values for rendering.
type ScaledDimensions struct {
	FontSize     float64
	Padding      float64
	LineSpacing  float64
	CornerRadius float64
	MarginRight  float64
	MarginLeft   float64
	MarginTop    float64
	ScaleFactor  float64
}

// CalculateScaledDimensions calculates scaled dimensions based on image size.
// The scaling is based on 1920x1080 as the reference resolution.
func CalculateScaledDimensions(width, height int) ScaledDimensions {
	return calculateScaledDimensionsForResolution(width, height)
}

// CalculateScaledDimensionsForDisplay calculates scaled dimensions based on the actual
// display resolution, which may differ from the image resolution.
// This ensures text is readable regardless of the image size.
func CalculateScaledDimensionsForDisplay() ScaledDimensions {
	// Query the actual display resolution
	displayRes := sysinfo.GetDisplayResolution()
	return calculateScaledDimensionsForResolution(displayRes.Width, displayRes.Height)
}

// calculateScaledDimensionsForResolution is the internal implementation that calculates
// scaled dimensions for a given resolution.
func calculateScaledDimensionsForResolution(width, height int) ScaledDimensions {
	// Calculate scale factor based on the smaller dimension ratio
	scaleX := float64(width) / float64(BaseWidth)
	scaleY := float64(height) / float64(BaseHeight)

	// Use the smaller scale to ensure everything fits
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Apply minimum scale factor for readability
	if scale < MinScaleFactor {
		scale = MinScaleFactor
	}

	// Cap maximum scale factor to keep text compact on high-res displays
	if scale > MaxScaleFactor {
		scale = MaxScaleFactor
	}

	// Calculate scaled font size with minimum
	fontSize := float64(BaseFontSize) * scale
	if fontSize < MinFontSize {
		fontSize = MinFontSize
	}

	return ScaledDimensions{
		FontSize:     fontSize,
		Padding:      float64(BasePadding) * scale,
		LineSpacing:  float64(BaseLineSpacing) * scale,
		CornerRadius: float64(BaseCornerRadius) * scale,
		MarginRight:  float64(BaseMarginRight) * scale,
		MarginLeft:   float64(BaseMarginLeft) * scale,
		MarginTop:    float64(BaseMarginTop) * scale,
		ScaleFactor:  scale,
	}
}

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

// RenderDualPanelOverlay renders two panels on an image - services on the left, system info on the right.
// This function uses resolution-aware scaling to ensure readability at different resolutions.
// It queries the actual display resolution to determine proper text scaling.
func RenderDualPanelOverlay(img image.Image, leftLines []string, rightLines []string) (image.Image, error) {
	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

	// Get the actual display resolution for proper scaling
	displayRes := sysinfo.GetDisplayResolution()

	// Calculate scaled dimensions based on display resolution (for text readability)
	// but we also need to account for the image dimensions for positioning
	dims := CalculateScaledDimensionsForDisplay()

	// If the image dimensions differ significantly from the display resolution,
	// we need to adjust margins proportionally to the image size
	imageScaleX := float64(width) / float64(displayRes.Width)
	imageScaleY := float64(height) / float64(displayRes.Height)

	// Adjust margins based on the image-to-display ratio
	// This ensures proper positioning regardless of image size
	dims.MarginLeft = dims.MarginLeft * imageScaleX
	dims.MarginRight = dims.MarginRight * imageScaleX
	dims.MarginTop = dims.MarginTop * imageScaleY

	// Create a new drawing context
	dc := gg.NewContext(width, height)

	// Draw the original image
	dc.DrawImage(img, 0, 0)

	// Load the font
	fontFile, err := getFontPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get font path: %v", err)
	}

	err = dc.LoadFontFace(fontFile, dims.FontSize)
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %v", err)
	}

	lineHeight := dims.FontSize + dims.LineSpacing

	// Calculate dimensions for left panel (services)
	var leftMaxWidth float64
	for _, line := range leftLines {
		w, _ := dc.MeasureString(line)
		if w > leftMaxWidth {
			leftMaxWidth = w
		}
	}
	leftTextHeight := lineHeight * float64(len(leftLines))
	leftBoxWidth := leftMaxWidth + (dims.Padding * 2)
	leftBoxHeight := leftTextHeight + (dims.Padding * 2) - dims.LineSpacing

	// Calculate dimensions for right panel (system info)
	var rightMaxWidth float64
	for _, line := range rightLines {
		w, _ := dc.MeasureString(line)
		if w > rightMaxWidth {
			rightMaxWidth = w
		}
	}
	rightTextHeight := lineHeight * float64(len(rightLines))
	rightBoxWidth := rightMaxWidth + (dims.Padding * 2)
	rightBoxHeight := rightTextHeight + (dims.Padding * 2) - dims.LineSpacing

	// Choose colors based on left region brightness
	leftBoxX := dims.MarginLeft
	leftBoxY := dims.MarginTop
	leftIsLight := AnalyzeRegionBrightness(img, int(leftBoxX), int(leftBoxY), int(leftBoxWidth), int(leftBoxHeight))
	var leftColors TextColor
	if leftIsLight {
		leftColors = DarkOnLight()
	} else {
		leftColors = LightOnDark()
	}

	// Choose colors based on right region brightness
	rightBoxX := float64(width) - rightBoxWidth - dims.MarginRight
	rightBoxY := dims.MarginTop
	rightIsLight := AnalyzeRegionBrightness(img, int(rightBoxX), int(rightBoxY), int(rightBoxWidth), int(rightBoxHeight))
	var rightColors TextColor
	if rightIsLight {
		rightColors = DarkOnLight()
	} else {
		rightColors = LightOnDark()
	}

	// Draw left panel (services)
	if len(leftLines) > 0 {
		drawPanel(dc, leftBoxX, leftBoxY, leftBoxWidth, leftBoxHeight, dims, leftColors, leftLines)
	}

	// Draw right panel (system info)
	if len(rightLines) > 0 {
		drawPanel(dc, rightBoxX, rightBoxY, rightBoxWidth, rightBoxHeight, dims, rightColors, rightLines)
	}

	return dc.Image(), nil
}

// drawPanel draws a single panel with background, border, and text.
func drawPanel(dc *gg.Context, boxX, boxY, boxWidth, boxHeight float64, dims ScaledDimensions, colors TextColor, lines []string) {
	// Draw semi-transparent background with rounded corners
	r, g, b, a := colors.Background.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)
	dc.DrawRoundedRectangle(boxX, boxY, boxWidth, boxHeight, dims.CornerRadius)
	dc.Fill()

	// Draw border
	r, g, b, a = colors.Border.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)
	dc.SetLineWidth(1)
	dc.DrawRoundedRectangle(boxX, boxY, boxWidth, boxHeight, dims.CornerRadius)
	dc.Stroke()

	// Draw text
	r, g, b, a = colors.Text.RGBA()
	dc.SetRGBA(float64(r)/65535, float64(g)/65535, float64(b)/65535, float64(a)/65535)

	lineHeight := dims.FontSize + dims.LineSpacing
	textX := boxX + dims.Padding
	textY := boxY + dims.Padding + dims.FontSize

	for _, line := range lines {
		dc.DrawString(line, textX, textY)
		textY += lineHeight
	}
}
