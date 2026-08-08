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

// Package asciichart provides ASCII chart rendering for terminal
// displays. It is a Go port of the Python AsciiChart.py from
// NomadNet, derived from asciichartpy.
package asciichart

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Symbols for chart drawing, ordered to match the Python symbols list:
// [cross, tee-right, left-line, right-line, horiz, corner-bl, corner-tl,
// corner-tr, corner-br, vert].
type Symbols struct {
	Cross     string // ┼ or +
	TeeRight  string // ┤ or |
	LeftLine  string // ╶ or -
	RightLine string // ╴ or -
	HorizLine string // ─ or -
	CornerBL  string // ╰ or '
	CornerTL  string // ╭ or ,
	CornerTR  string // ╮ or .
	CornerBR  string // ╯ or `
	VertLine  string // │ or |
}

// UnicodeSymbols provides Unicode box-drawing characters.
var UnicodeSymbols = Symbols{
	Cross:     "┼",
	TeeRight:  "┤",
	LeftLine:  "╶",
	RightLine: "╴",
	HorizLine: "─",
	CornerBL:  "╰",
	CornerTL:  "╭",
	CornerTR:  "╮",
	CornerBR:  "╯",
	VertLine:  "│",
}

// PlainSymbols provides plain ASCII characters.
var PlainSymbols = Symbols{
	Cross:     "+",
	TeeRight:  "|",
	LeftLine:  "-",
	RightLine: "-",
	HorizLine: "-",
	CornerBL:  "'",
	CornerTL:  ",",
	CornerTR:  ".",
	CornerBR:  "`",
	VertLine:  "|",
}

// Chart represents an ASCII chart renderer.
type Chart struct {
	Symbols Symbols
	Offset  int
	Height  int
	Min     *float64
	Max     *float64
	Format  string
}

// New creates a new Chart with the given glyph set name.
func New(glyphSet string) *Chart {
	symbols := UnicodeSymbols
	if glyphSet == "plain" {
		symbols = PlainSymbols
	}
	return &Chart{
		Symbols: symbols,
		Offset:  3,
		Format:  "%8.2f ",
	}
}

// list returns the drawing symbols in the Python symbols-list order.
func (s Symbols) list() []string {
	return []string{s.Cross, s.TeeRight, s.LeftLine, s.RightLine, s.HorizLine,
		s.CornerBL, s.CornerTL, s.CornerTR, s.CornerBR, s.VertLine}
}

// Plot renders the series as an ASCII chart. It mirrors the Python
// AsciiChart.plot, including that library's quirk that the Y-axis label is
// stored as a single (possibly multi-character) cell in each row, so the
// rendered line width depends on the formatted label length.
func (c *Chart) Plot(series [][]float64) string {
	if len(series) == 0 {
		return ""
	}

	// Find min/max across all series.
	minVal := math.MaxFloat64
	maxVal := -math.MaxFloat64
	hasValues := false
	for _, s := range series {
		for _, v := range s {
			if !math.IsNaN(v) {
				if v < minVal {
					minVal = v
				}
				if v > maxVal {
					maxVal = v
				}
				hasValues = true
			}
		}
	}
	if !hasValues {
		return ""
	}

	if c.Min != nil {
		minVal = *c.Min
	}
	if c.Max != nil {
		maxVal = *c.Max
	}
	if minVal > maxVal {
		minVal, maxVal = maxVal, minVal
	}

	interval := maxVal - minVal
	offset := max(c.Offset, 0)

	// Python: height = cfg.get('height', interval); ratio = height/interval
	// if interval > 0 else 1. An unset height (c.Height == 0) therefore yields
	// ratio = 1, matching Python's default height == interval.
	ratio := 1.0
	if interval > 0 && c.Height != 0 {
		ratio = float64(c.Height) / interval
	}

	min2 := int(math.Floor(minVal * ratio))
	max2 := int(math.Ceil(maxVal * ratio))

	clamp := func(n float64) float64 {
		if n < minVal {
			return minVal
		}
		if n > maxVal {
			return maxVal
		}
		return n
	}
	scaled := func(y float64) int {
		return int(pyRound(clamp(y)*ratio)) - min2
	}

	rows := max2 - min2

	width := 0
	for _, s := range series {
		if len(s) > width {
			width = len(s)
		}
	}
	width += offset

	symbols := c.Symbols.list()

	// result is a grid of string cells (not single runes), mirroring Python's
	// list-of-strings rows. The label is one multi-character cell.
	result := make([][]string, rows+1)
	for i := range result {
		result[i] = make([]string, width)
		for j := range result[i] {
			result[i][j] = " "
		}
	}

	setCell := func(row, col int, sym string) {
		if row >= 0 && row <= rows && col >= 0 && col < width {
			result[row][col] = sym
		}
	}

	// Y-axis labels and axis markers.
	for y := min2; y <= max2; y++ {
		var labelVal float64
		if rows > 0 {
			labelVal = maxVal - (float64(y-min2) * interval / float64(rows))
		} else {
			if y == min2 {
				labelVal = maxVal
			} else {
				continue
			}
		}
		label := formatLabel(labelVal, c.Format)
		row := y - min2
		start := max(offset-len([]rune(label)), 0)
		setCell(row, start, label)
		if y == 0 {
			setCell(row, offset-1, symbols[0])
		} else {
			setCell(row, offset-1, symbols[1])
		}
	}

	// First point marker.
	if len(series) > 0 && len(series[0]) > 0 && !math.IsNaN(series[0][0]) {
		setCell(rows-scaled(series[0][0]), offset-1, symbols[0])
	}

	// Series.
	for _, s := range series {
		for x := 0; x < len(s)-1; x++ {
			d0 := s[x]
			d1 := s[x+1]
			if math.IsNaN(d0) && math.IsNaN(d1) {
				continue
			}
			col := x + offset
			if math.IsNaN(d0) && !math.IsNaN(d1) {
				setCell(rows-scaled(d1), col, symbols[2])
				continue
			}
			if !math.IsNaN(d0) && math.IsNaN(d1) {
				setCell(rows-scaled(d0), col, symbols[3])
				continue
			}
			y0 := scaled(d0)
			y1 := scaled(d1)
			if y0 == y1 {
				setCell(rows-y0, col, symbols[4])
				continue
			}
			if y0 > y1 {
				setCell(rows-y1, col, symbols[5])
				setCell(rows-y0, col, symbols[7])
			} else {
				setCell(rows-y1, col, symbols[6])
				setCell(rows-y0, col, symbols[8])
			}
			lo := y0
			hi := y1
			if lo > hi {
				lo, hi = hi, lo
			}
			for y := lo + 1; y < hi; y++ {
				setCell(rows-y, col, symbols[9])
			}
		}
	}

	var lines []string
	for _, row := range result {
		line := strings.TrimRight(strings.Join(row, ""), " ")
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// PlotSingle is a convenience method for plotting a single series.
func (c *Chart) PlotSingle(series []float64) string {
	return c.Plot([][]float64{series})
}

// formatLabel formats a Y-axis value. The format string is a Go fmt verb
// (e.g. "%8.2f ") mirroring the Python placeholder (e.g. "{:8.2f }"). A format
// without a "%" verb yields a blank 12-column label, matching Python's
// non-callable placeholder edge handling.
func formatLabel(val float64, format string) string {
	if !strings.Contains(format, "%") {
		return strings.Repeat(" ", 12)
	}
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "     NaN "
	}
	return fmt.Sprintf(format, val)
}

func formatInt(n int) string {
	return strconv.Itoa(n)
}

// pyRound mirrors Python's built-in round for a single float argument: round
// half to even (banker's rounding). Go's math.Round rounds half away from zero,
// which diverges from Python on exact .5 ties (e.g. round(2.5)==2, round(0.5)==0).
func pyRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	f := math.Floor(x)
	r := x - f
	if r < 0.5 {
		return f
	}
	if r > 0.5 {
		return f + 1
	}
	// Exact tie: round to even.
	if math.Mod(f, 2) == 0 {
		return f
	}
	return f + 1
}
