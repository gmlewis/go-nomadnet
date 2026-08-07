// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Command png-to-mu converts PNG images to 24-bit color micron markdown (.mu) files.
//
// Usage:
//
//	png-to-mu [options] input.png [output.mu]
//
// Options:
//
//	-width int      Output width in characters (default: 120)
//	-dither         Use ImageMagick for highest-quality Lanczos rescale
//	-no-comments    Omit comment header from output
//	-input string   Original input filename (for comment header when using -dither)
//
// The tool uses the ▀ character with separate foreground (`FT) and background (`BT)
// colors to achieve maximum fidelity ASCII art. Each character represents 2 vertical
// pixels: the top pixel becomes the foreground color, the bottom becomes background.
//
// When -dither is specified, ImageMagick's `convert` (or `magick`) command performs
// a high-quality Lanczos resample to exactly 2× the target character resolution,
// then this tool samples pairs of pixels for FG/BG colors.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Half-block character (upper half)
const blockUpper = '▀' // U+2580 - upper half block

type config struct {
	width      int
	dither     bool
	noComments bool
	inputFile  string
}

func main() {
	cfg := config{
		width:      120,
		dither:     false,
		noComments: false,
		inputFile:  "",
	}

	flag.IntVar(&cfg.width, "width", 120, "Output width in characters")
	flag.BoolVar(&cfg.dither, "dither", false, "Use ImageMagick for highest-quality Lanczos rescale")
	flag.BoolVar(&cfg.noComments, "no-comments", false, "Omit comment header from output")
	flag.StringVar(&cfg.inputFile, "input", "", "Original input filename (for comment header)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] input.png [output.mu]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Converts PNG images to 24-bit color micron markdown (.mu) files.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nThis tool uses half-block characters (▀) with separate foreground\n")
		fmt.Fprintf(os.Stderr, "(`FT) and background (`BT) colors for maximum fidelity ASCII art.\n")
		fmt.Fprintf(os.Stderr, "\nWhen -dither is specified, ImageMagick performs a high-quality\n")
		fmt.Fprintf(os.Stderr, "Lanczos resample to 2× the target resolution for optimal sampling.\n")
	}

	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputPath := args[0]
	outputPath := ""
	if len(args) >= 2 {
		outputPath = args[1]
	} else {
		base := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
		outputPath = base + ".mu"
	}

	// Track original filename for comment header
	originalFile := cfg.inputFile
	if originalFile == "" {
		originalFile = inputPath
	}

	var img image.Image
	var err error
	var outW, outH int

	if cfg.dither {
		// Use ImageMagick for highest-quality rescale
		// ImageMagick resizes to (width × height*2) so we sample 2 pixels per char row
		img, outW, outH, err = preprocessWithImageMagick(inputPath, cfg.width)
	} else {
		// Standard Go image loading and resizing
		img, outW, outH, err = loadImageAndResize(inputPath, cfg.width)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading image: %v\n", err)
		os.Exit(1)
	}

	micron, err := renderHalfBlockToMicron(img, outW, outH, originalFile, cfg.noComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error converting image: %v\n", err)
		os.Exit(1)
	}

	var out io.Writer
	if outputPath == "-" || outputPath == "" {
		out = os.Stdout
	} else {
		f, err := os.Create(outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
		fmt.Fprintf(os.Stderr, "Written to: %s\n", outputPath)
	}

	_, err = io.WriteString(out, micron)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}
}

// preprocessWithImageMagick uses ImageMagick to perform high-quality Lanczos
// resampling to exactly 2× the target height for optimal half-block sampling.
func preprocessWithImageMagick(inputPath string, targetWidth int) (image.Image, int, int, error) {
	imgFile, err := os.Open(inputPath)
	if err != nil {
		return nil, 0, 0, err
	}
	defer imgFile.Close()

	img, format, err := image.Decode(imgFile)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decoding image (%s): %w", format, err)
	}

	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// Calculate target dimensions
	// Height = width * (srcH/srcW) / 2 (for aspect ratio with half-block chars)
	targetH := max(1, int(float64(targetWidth)*float64(srcH)/float64(srcW)/2))

	// ImageMagick will resize to width × (targetH*2)
	// so we can sample 2 pixels per character row
	resizeH := targetH * 2

	// Create a temporary file for the rescaled image
	tmpFile, err := os.CreateTemp("", "png-to-mu-*.png")
	if err != nil {
		return nil, 0, 0, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Use ImageMagick's magick (IMv7) or convert (IMv6) with Lanczos filter
	// Try "magick" first (IMv7), fall back to "convert" if not available
	cmd := exec.Command("magick",
		inputPath,
		"-filter", "Lanczos",
		"-resize", fmt.Sprintf("%dx%d!", targetWidth, resizeH),
		"-depth", "16",
		tmpPath,
	)
	if err := cmd.Run(); err != nil {
		// Fall back to "convert" for IMv6
		cmd = exec.Command("convert",
			inputPath,
			"-filter", "Lanczos",
			"-resize", fmt.Sprintf("%dx%d!", targetWidth, resizeH),
			"-depth", "16",
			tmpPath,
		)
		if err := cmd.Run(); err != nil {
			return nil, 0, 0, fmt.Errorf("ImageMagick: %w", err)
		}
	}

	// Load the rescaled image
	rescaledFile, err := os.Open(tmpPath)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rescaledFile.Close()

	rescaledImg, _, err := image.Decode(rescaledFile)
	if err != nil {
		return nil, 0, 0, err
	}

	return rescaledImg, targetWidth, targetH, nil
}

// loadImageAndResize loads an image using Go's standard image package
// and returns it with calculated dimensions (no actual resizing in Go).
func loadImageAndResize(inputPath string, targetWidth int) (image.Image, int, int, error) {
	imgFile, err := os.Open(inputPath)
	if err != nil {
		return nil, 0, 0, err
	}
	defer imgFile.Close()

	img, format, err := image.Decode(imgFile)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decoding image (%s): %w", format, err)
	}

	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// Calculate target dimensions (same formula as ImageMagick path)
	targetH := max(1, int(float64(targetWidth)*float64(srcH)/float64(srcW)/2))

	return img, targetWidth, targetH, nil
}

// colorKey returns a string key for comparing colors
func colorKey(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("%04x%04x%04x", r, g, b)
}

// formatMicronColor converts a color to micron 24-bit foreground color code
func formatMicronColor(c color.Color) string {
	r, g, b, _ := c.RGBA()
	r8 := uint8(r >> 8)
	g8 := uint8(g >> 8)
	b8 := uint8(b >> 8)
	return fmt.Sprintf("`FT%02x%02x%02x", r8, g8, b8)
}

// formatMicronBGColor converts a color to micron 24-bit background color code
func formatMicronBGColor(c color.Color) string {
	r, g, b, _ := c.RGBA()
	r8 := uint8(r >> 8)
	g8 := uint8(g >> 8)
	b8 := uint8(b >> 8)
	return fmt.Sprintf("`BT%02x%02x%02x", r8, g8, b8)
}

// renderHalfBlockToMicron renders an image using half-block characters.
// For ImageMagick-preprocessed images, the image is already at 2× height,
// so we sample pairs of vertical pixels (2y, 2y+1) for FG and BG colors.
// For Go-native images, we sample 4 pixels and average them into 2.
func renderHalfBlockToMicron(img image.Image, outW, outH int, originalFile string, noComments bool) (string, error) {
	bounds := img.Bounds()
	imgW := bounds.Dx()
	imgH := bounds.Dy()

	var sb strings.Builder

	if !noComments {
		fmt.Fprintf(&sb, "# ASCII art generated from PNG image (half-block mode)\n# Original: %s\n# Dimensions: %dx%d characters (using ▀ with FG/BG colors)\n#\n\n", filepath.Base(originalFile), outW, outH)
	}

	// Check if image is pre-scaled (from ImageMagick, imgH should be outH*2)
	isPreScaled := (imgH == outH*2) && (imgW == outW)

	for y := 0; y < outH; y++ {
		var lastFGKey, lastBGKey string
		for x := 0; x < outW; x++ {
			var upper, lower color.Color

			if isPreScaled {
				// ImageMagick path: image is exactly outW × (outH*2)
				// Sample pixels at (x, y*2) and (x, y*2+1)
				srcX := bounds.Min.X + x
				srcY1 := bounds.Min.Y + y*2
				srcY2 := srcY1 + 1

				upper = img.At(srcX, srcY1)
				lower = img.At(srcX, srcY2)
			} else {
				// Go-native path: sample 4 pixels and average
				srcX := int(float64(x) * float64(imgW) / float64(outW))
				baseY := int(float64(y*4) * float64(imgH) / float64(outH*4))

				if srcX >= imgW {
					srcX = imgW - 1
				}

				// Sample 4 pixels
				pixels := make([]color.Color, 4)
				for i := 0; i < 4; i++ {
					yi := baseY + i
					if yi >= imgH {
						yi = imgH - 1
					}
					pixels[i] = img.At(bounds.Min.X+srcX, bounds.Min.Y+yi)
				}

				// Average top 2 for foreground
				r1, g1, b1, _ := pixels[0].RGBA()
				r2, g2, b2, _ := pixels[1].RGBA()
				upper = color.RGBA{
					R: uint8(((r1 >> 8) + (r2 >> 8)) / 2),
					G: uint8(((g1 >> 8) + (g2 >> 8)) / 2),
					B: uint8(((b1 >> 8) + (b2 >> 8)) / 2),
					A: 255,
				}

				// Average bottom 2 for background
				r3, g3, b3, _ := pixels[2].RGBA()
				r4, g4, b4, _ := pixels[3].RGBA()
				lower = color.RGBA{
					R: uint8(((r3 >> 8) + (r4 >> 8)) / 2),
					G: uint8(((g3 >> 8) + (g4 >> 8)) / 2),
					B: uint8(((b3 >> 8) + (b4 >> 8)) / 2),
					A: 255,
				}
			}

			_, _, _, ua := upper.RGBA()
			_, _, _, la := lower.RGBA()

			if ua < 0x1000 && la < 0x1000 {
				// Both transparent - use space
				if lastFGKey != "" {
					sb.WriteString("`f`b")
					lastFGKey = ""
					lastBGKey = ""
				}
				sb.WriteByte(' ')
				continue
			}

			fgKey := colorKey(upper)
			bgKey := colorKey(lower)

			// Output background color if changed (use BT tag for background)
			if bgKey != lastBGKey {
				sb.WriteString(formatMicronBGColor(lower))
				lastBGKey = bgKey
			}

			// Output foreground color if changed (use FT tag for foreground)
			if fgKey != lastFGKey {
				sb.WriteString(formatMicronColor(upper))
				lastFGKey = fgKey
			}

			sb.WriteRune(blockUpper)
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("`f`b`=\n")
	return sb.String(), nil
}
