// Prescale the full-resolution Alice Security logo into a small PNG suitable
// for //go:embed in the installer. Run once whenever the source logo changes:
//
//	go run ./scripts/prescale-logo
//
// Reads logo_alice_security.png from the repo root, crops to just the shield
// icon (top ~58% of the image — drops the "ALICE" wordmark and "SECURITY"
// ribbon which are illegible at terminal resolutions and are rendered as
// real text by the splash instead), and writes a 192x192 square to
// internal/assets/logo_alice_security.png.
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/disintegration/imaging"
)

const (
	srcPath = "logo_alice_security.png"
	dstPath = "internal/assets/logo_alice_security.png"
	target  = 192

	// shieldHeightPct is the fraction of the source image height occupied by
	// the shield/"A" glyph. Everything below is the ALICE wordmark + SECURITY
	// ribbon, which the splash re-renders as terminal text.
	shieldHeightPct = 0.58
)

func main() {
	src, err := imaging.Open(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", srcPath, err)
		os.Exit(1)
	}

	b := src.Bounds()
	cropH := int(float64(b.Dy()) * shieldHeightPct)
	side := min(b.Dx(), cropH)
	x0 := b.Min.X + (b.Dx()-side)/2
	shield := imaging.Crop(src, image.Rect(x0, b.Min.Y, x0+side, b.Min.Y+side))

	dst := imaging.Fit(shield, target, target, imaging.Lanczos)

	out, err := os.Create(dstPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create %s: %v\n", dstPath, err)
		os.Exit(1)
	}
	defer out.Close()

	enc := &png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(out, dst); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s (%dx%d)\n", dstPath, dst.Bounds().Dx(), dst.Bounds().Dy())
}
