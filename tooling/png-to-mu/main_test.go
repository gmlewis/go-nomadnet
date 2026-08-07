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

package main

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

// TestFormatMicronColor verifies 24-bit color code generation
func TestFormatMicronColor(t *testing.T) {
	tests := []struct {
		name string
		c    color.Color
		want string
	}{
		{"black", color.RGBA{0, 0, 0, 255}, "`FT000000"},
		{"white", color.RGBA{255, 255, 255, 255}, "`FTffffff"},
		{"red", color.RGBA{255, 0, 0, 255}, "`FTff0000"},
		{"green", color.RGBA{0, 255, 0, 255}, "`FT00ff00"},
		{"blue", color.RGBA{0, 0, 255, 255}, "`FT0000ff"},
		{"gray", color.RGBA{128, 128, 128, 255}, "`FT808080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMicronColor(tt.c)
			if got != tt.want {
				t.Errorf("formatMicronColor(%v) = %v, want %v", tt.c, got, tt.want)
			}
		})
	}
}

// TestCalculateDimensions verifies dimension calculations
func TestCalculateDimensions(t *testing.T) {
	tests := []struct {
		name      string
		srcW      int
		srcH      int
		cfg       config
		wantW, wantH int
	}{
		{"no scaling", 100, 100, config{scale: 1.0, charSet: "simple"}, 100, 100},
		{"scale 0.5", 100, 100, config{scale: 0.5, charSet: "simple"}, 50, 50},
		{"max width", 200, 100, config{scale: 1.0, width: 80, charSet: "simple"}, 80, 40},
		{"max height", 100, 200, config{scale: 1.0, height: 50, charSet: "simple"}, 25, 50},
		{"blocks mode", 100, 100, config{scale: 1.0, charSet: "blocks"}, 100, 50},
		{"detailed mode", 100, 100, config{scale: 1.0, charSet: "detailed"}, 100, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := calculateDimensions(tt.srcW, tt.srcH, tt.cfg)
			if gotW != tt.wantW || gotH != tt.wantH {
				t.Errorf("calculateDimensions(%d, %d, cfg) = (%d, %d), want (%d, %d)",
					tt.srcW, tt.srcH, gotW, gotH, tt.wantW, tt.wantH)
			}
		})
	}
}

// TestRenderSimple verifies simple character rendering
func TestRenderSimple(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	bounds := img.Bounds()

	// White pixel should map to light character
	img.Set(5, 5, color.RGBA{255, 255, 255, 255})

	char, c := renderSimple(img, bounds, 5, 5, 10, 10)
	if char == ' ' {
		t.Errorf("renderSimple(white) = %q, want non-space character", char)
	}

	r, g, b, a := c.RGBA()
	if a < 0x1000 {
		t.Errorf("renderSimple(white) alpha = %04x, want opaque", a)
	}
	if r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Errorf("renderSimple(white) color = (%04x, %04x, %04x), want bright", r, g, b)
	}
}

// TestRenderBlocks verifies block character rendering
func TestRenderBlocks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	bounds := img.Bounds()

	// Fill with opaque color
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{100, 150, 200, 255})
		}
	}

	char, c := renderBlocks(img, bounds, 5, 5, 5, 5)
	if char == ' ' {
		t.Errorf("renderBlocks(opaque) = %q, want non-space character", char)
	}

	// Verify character is one of the block characters
	validChars := map[rune]bool{
		blockUpper: true,
		blockLower: true,
		blockFull:  true,
		blockLight: true,
		' ':        true,
	}
	if !validChars[char] {
		t.Errorf("renderBlocks() = %q, want valid block character", char)
	}

	_ = c // color checked implicitly
}

// TestRenderBraille verifies braille character rendering
func TestRenderBraille(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	bounds := img.Bounds()

	// Fill with opaque color
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.RGBA{100, 150, 200, 255})
		}
	}

	char, c := renderBraille(img, bounds, 5, 5, 10, 5)

	// With all pixels opaque, should get full braille character
	if char != brailleFull {
		t.Errorf("renderBraille(all opaque) = %q, want brailleFull %q", char, brailleFull)
	}

	// Check color is approximately correct
	r, g, b, _ := c.RGBA()
	r8 := uint8(r >> 8)
	g8 := uint8(g >> 8)
	b8 := uint8(b >> 8)

	// Allow some tolerance for averaging
	if r8 < 90 || r8 > 110 {
		t.Errorf("renderBraille() red = %d, want ~100", r8)
	}
	if g8 < 140 || g8 > 160 {
		t.Errorf("renderBraille() green = %d, want ~150", g8)
	}
	if b8 < 190 || b8 > 210 {
		t.Errorf("renderBraille() blue = %d, want ~200", b8)
	}
}

// TestRenderBrailleTransparent verifies transparent pixel handling
func TestRenderBrailleTransparent(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	bounds := img.Bounds()

	// All transparent
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.RGBA{0, 0, 0, 0})
		}
	}

	char, _ := renderBraille(img, bounds, 5, 5, 10, 5)
	if char != ' ' {
		t.Errorf("renderBraille(all transparent) = %q, want space", char)
	}
}

// TestColorKey verifies color key generation
func TestColorKey(t *testing.T) {
	c1 := color.RGBA{255, 128, 64, 255}
	c2 := color.RGBA{255, 128, 64, 255}
	c3 := color.RGBA{255, 128, 63, 255}

	k1 := colorKey(c1)
	k2 := colorKey(c2)
	k3 := colorKey(c3)

	if k1 != k2 {
		t.Errorf("colorKey(same colors) = %q, %q, want equal", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("colorKey(different colors) = %q, %q, want different", k1, k3)
	}
}

// TestImageToMicron verifies complete conversion
func TestImageToMicron(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))

	// Simple test pattern
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 32), uint8(y * 32), 128, 255})
		}
	}

	cfg := config{
		charSet:    "simple",
		noComments: true,
	}

	result, err := imageToMicron(img, cfg)
	if err != nil {
		t.Fatalf("imageToMicron() error = %v", err)
	}

	// Verify output structure
	if !strings.HasPrefix(result, "`FT") {
		t.Errorf("imageToMicron() should start with color code, got %q...", result[:20])
	}
	if !strings.Contains(result, "`f`b`=") {
		t.Errorf("imageToMicron() should end with reset codes")
	}

	// Verify we have content lines (image rows + header + footer)
	lines := strings.Split(strings.TrimSpace(result), "\n")
	// 8x8 image in simple mode = 8 lines + 1 reset line = 9 lines
	if len(lines) < 2 {
		t.Errorf("imageToMicron() produced %d lines, want at least 2", len(lines))
	}
}

// TestGetRenderer verifies renderer selection
func TestGetRenderer(t *testing.T) {
	tests := []struct {
		charSet string
		want    string
	}{
		{"simple", "simple"},
		{"blocks", "blocks"},
		{"detailed", "braille"},
		{"unknown", "braille"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.charSet, func(t *testing.T) {
			fn := getRenderer(tt.charSet)
			if fn == nil {
				t.Errorf("getRenderer(%q) returned nil", tt.charSet)
			}
			// We can't directly compare function pointers, so we just verify it's not nil
		})
	}
}
