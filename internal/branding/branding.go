// Package branding owns the embedded Wallpaper Identity visual identity.
package branding

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/png"
	"sync"
)

//go:embed wid-logo.png
var logoPNG []byte

var (
	logoOnce sync.Once
	logo     image.Image
	logoErr  error
)

// Logo returns the immutable W:ID application mark.
func Logo() (image.Image, error) {
	logoOnce.Do(func() {
		logo, _, logoErr = image.Decode(bytes.NewReader(logoPNG))
		if logoErr != nil {
			logoErr = fmt.Errorf("decode embedded W:ID logo: %w", logoErr)
		}
	})
	return logo, logoErr
}
