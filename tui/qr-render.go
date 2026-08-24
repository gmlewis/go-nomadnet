// Copyright 2026 Glenn Lewis. All rights reserved.
//
// Use of this source code is governed by the Reticulum License
// that can be found in the LICENSE file.

package tui

import (
	"strings"

	"github.com/gmlewis/go-reticulum/qr"
)

// RenderQRText renders a QR code for the given text as Unicode half-block
// characters with a 1-module quiet-zone border, matching Python qrcode's
// print_ascii (border=1, box_size=1). Each pair of QR rows is compressed into
// one text line using half-block glyphs: space (both white), ▀ (top black),
// ▄ (bottom black), █ (both black). Returns empty string on error.
func RenderQRText(text string) string {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return ""
	}

	// QR size + 2 for the 1-module quiet-zone border on each side.
	size := code.Size + 2
	// Build a bitmap with the border (all white = false).
	bitmap := make([][]bool, size)
	for i := range bitmap {
		bitmap[i] = make([]bool, size)
	}
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			bitmap[y+1][x+1] = code.Black(x, y)
		}
	}

	var b strings.Builder
	for y := 0; y < size; y += 2 {
		for x := range size {
			top := bitmap[y][x]
			bottom := false
			if y+1 < size {
				bottom = bitmap[y+1][x]
			}
			switch {
			case top && bottom:
				b.WriteRune('█')
			case top && !bottom:
				b.WriteRune('▀')
			case !top && bottom:
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		if y+2 < size {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
