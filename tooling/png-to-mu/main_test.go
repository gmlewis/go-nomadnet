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

// TestFormatMicronBGColor verifies 24-bit background color code generation
func TestFormatMicronBGColor(t *testing.T) {
	tests := []struct {
		name string
		c    color.Color
		want string
	}{
		{"black", color.RGBA{0, 0, 0, 255}, "`BT000000"},
		{"white", color.RGBA{255, 255, 255, 255}, "`BTffffff"},
		{"red", color.RGBA{255, 0, 0, 255}, "`BTff0000"},
		{"green", color.RGBA{0, 255, 0, 255}, "`BT00ff00"},
		{"blue", color.RGBA{0, 0, 255, 255}, "`BT0000ff"},
		{"gray", color.RGBA{128, 128, 128, 255}, "`BT808080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMicronBGColor(tt.c)
			if got != tt.want {
				t.Errorf("formatMicronBGColor(%v) = %v, want %v", tt.c, got, tt.want)
			}
		})
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

// TestRenderHalfBlockToMicronPreScaled verifies half-block rendering with pre-scaled images
func TestRenderHalfBlockToMicronPreScaled(t *testing.T) {
	// Create an 8x4 image (simulating ImageMagick output: 8 wide × 4 tall = 8×2 chars)
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))

	// Simple gradient pattern
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 32), uint8(y * 64), 128, 255})
		}
	}

	result, err := renderHalfBlockToMicron(img, 8, 2, "test.png", true)
	if err != nil {
		t.Fatalf("renderHalfBlockToMicron() error = %v", err)
	}

	// Verify output structure
	if !strings.Contains(result, "`FT") {
		t.Errorf("renderHalfBlockToMicron() should contain foreground color codes")
	}
	if !strings.Contains(result, "`BT") {
		t.Errorf("renderHalfBlockToMicron() should contain background color codes")
	}
	if !strings.Contains(result, "`f`b`=") {
		t.Errorf("renderHalfBlockToMicron() should end with reset codes")
	}

	// Verify we have content lines (2 rows + reset)
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) < 3 {
		t.Errorf("renderHalfBlockToMicron() produced %d lines, want at least 3", len(lines))
	}

	// Verify half-block character is used
	if !strings.ContainsRune(result, blockUpper) {
		t.Errorf("renderHalfBlockToMicron() should use half-block character ▀")
	}
}

// TestRenderHalfBlockToMicronGoNative verifies half-block rendering with Go-native images
func TestRenderHalfBlockToMicronGoNative(t *testing.T) {
	// Create a 16x8 image (Go-native, will be sampled to 8×2 chars)
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))

	// Fill with solid color
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{100, 150, 200, 255})
		}
	}

	result, err := renderHalfBlockToMicron(img, 8, 2, "test.png", true)
	if err != nil {
		t.Fatalf("renderHalfBlockToMicron() error = %v", err)
	}

	// Verify output structure
	if !strings.Contains(result, "`FT") {
		t.Errorf("renderHalfBlockToMicron() should contain foreground color codes")
	}
	if !strings.Contains(result, "`BT") {
		t.Errorf("renderHalfBlockToMicron() should contain background color codes")
	}

	// Verify we have content lines
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) < 3 {
		t.Errorf("renderHalfBlockToMicron() produced %d lines, want at least 3", len(lines))
	}
}

// TestRenderHalfBlockToMicronComments verifies comment generation
func TestRenderHalfBlockToMicronComments(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{128, 128, 128, 255})
		}
	}

	// With comments
	result, err := renderHalfBlockToMicron(img, 8, 2, "test.png", false)
	if err != nil {
		t.Fatalf("renderHalfBlockToMicron() error = %v", err)
	}

	if !strings.HasPrefix(result, "# ASCII art") {
		t.Errorf("renderHalfBlockToMicron() with comments should start with '#'")
	}
	if !strings.Contains(result, "Dimensions:") {
		t.Errorf("renderHalfBlockToMicron() should contain dimensions in comments")
	}
	if !strings.Contains(result, "test.png") {
		t.Errorf("renderHalfBlockToMicron() should contain original filename")
	}

	// Without comments
	resultNoComments, err := renderHalfBlockToMicron(img, 8, 2, "test.png", true)
	if err != nil {
		t.Fatalf("renderHalfBlockToMicron() error = %v", err)
	}

	if strings.HasPrefix(resultNoComments, "#") {
		t.Errorf("renderHalfBlockToMicron() no-comments should not start with '#'")
	}
}

// TestRenderHalfBlockToMicronTransparent verifies transparent pixel handling
func TestRenderHalfBlockToMicronTransparent(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))

	// All transparent
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{0, 0, 0, 0})
		}
	}

	result, err := renderHalfBlockToMicron(img, 8, 2, "test.png", true)
	if err != nil {
		t.Fatalf("renderHalfBlockToMicron() error = %v", err)
	}

	// Should contain spaces for transparent areas
	if !strings.Contains(result, " ") {
		t.Errorf("renderHalfBlockToMicron(transparent) should contain spaces")
	}
}

// TestBlockUpperConstant verifies the half-block character
func TestBlockUpperConstant(t *testing.T) {
	if blockUpper != '▀' {
		t.Errorf("blockUpper = %q, want '▀' (U+2580)", blockUpper)
	}
}
