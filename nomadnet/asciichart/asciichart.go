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
	"math"
	"strconv"
	"strings"
)

// Symbols for chart drawing.
type Symbols struct {
	Cross     string // ┼ or +
	TeeRight  string // ┤ or |
	LeftLine  string // ─ or -
	RightLine string // ─ or -
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

// Plot renders the series as an ASCII chart.
// Series can be a single slice or multiple slices for multi-line charts.
func (c *Chart) Plot(series [][]float64) string {
	if len(series) == 0 {
		return ""
	}

	// Find min/max across all series
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

	// Apply overrides
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
	offset := c.Offset
	height := c.Height
	if height == 0 {
		height = int(interval)
	}
	if height < 1 {
		height = 1
	}

	ratio := float64(height) / interval
	if interval == 0 {
		ratio = 1
	}

	min2 := int(math.Floor(minVal * ratio))
	max2 := int(math.Ceil(maxVal * ratio))

	scaled := func(y float64) int {
		clamped := math.Max(math.Min(y, maxVal), minVal)
		return int(math.Round(clamped*ratio)) - min2
	}

	rows := max2 - min2

	// Find max width
	width := 0
	for _, s := range series {
		if len(s) > width {
			width = len(s)
		}
	}
	width += offset

	// Build result grid
	result := make([][]rune, rows+1)
	for i := range result {
		result[i] = make([]rune, width)
		for j := range result[i] {
			result[i][j] = ' '
		}
	}

	// Draw Y-axis labels and cross marks
	for y := min2; y <= max2+1; y++ {
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
		labelLen := len([]rune(label))

		row := y - min2
		if row >= 0 && row <= rows {
			startPos := offset - labelLen
			if startPos < 0 {
				startPos = 0
			}
			for i, ch := range label {
				if startPos+i < width {
					result[row][startPos+i] = ch
				}
			}
			if offset-1 >= 0 && offset-1 < width {
				if y == 0 {
					result[row][offset-1] = []rune(c.Symbols.Cross)[0]
				} else {
					result[row][offset-1] = []rune(c.Symbols.TeeRight)[0]
				}
			}
		}
	}

	// Draw first point
	if len(series) > 0 && len(series[0]) > 0 && !math.IsNaN(series[0][0]) {
		scaledVal := scaled(series[0][0])
		row := rows - scaledVal
		if row >= 0 && row <= rows && offset-1 >= 0 && offset-1 < width {
			result[row][offset-1] = []rune(c.Symbols.Cross)[0]
		}
	}

	// Draw series
	for _, s := range series {
		for x := 0; x < len(s)-1; x++ {
			d0 := s[x]
			d1 := s[x+1]

			if math.IsNaN(d0) && math.IsNaN(d1) {
				continue
			}

			col := x + offset

			if math.IsNaN(d0) && !math.IsNaN(d1) {
				scaledVal := scaled(d1)
				row := rows - scaledVal
				if row >= 0 && row <= rows && col < width {
					result[row][col] = []rune(c.Symbols.LeftLine)[0]
				}
				continue
			}

			if !math.IsNaN(d0) && math.IsNaN(d1) {
				scaledVal := scaled(d0)
				row := rows - scaledVal
				if row >= 0 && row <= rows && col < width {
					result[row][col] = []rune(c.Symbols.RightLine)[0]
				}
				continue
			}

			y0 := scaled(d0)
			y1 := scaled(d1)

			if y0 == y1 {
				row := rows - y0
				if row >= 0 && row <= rows && col < width {
					result[row][col] = []rune(c.Symbols.HorizLine)[0]
				}
				continue
			}

			// Draw corners
			if col < width {
				if y0 > y1 {
					result[rows-y1][col] = []rune(c.Symbols.CornerBL)[0]
					result[rows-y0][col] = []rune(c.Symbols.CornerBR)[0]
				} else {
					result[rows-y1][col] = []rune(c.Symbols.CornerTL)[0]
					result[rows-y0][col] = []rune(c.Symbols.CornerTR)[0]
				}
			}

			// Draw vertical line between corners
			start := min(y0, y1) + 1
			end := max(y0, y1)
			for y := start; y < end; y++ {
				row := rows - y
				if row >= 0 && row <= rows && col < width {
					result[row][col] = []rune(c.Symbols.VertLine)[0]
				}
			}
		}
	}

	// Convert to strings, trimming trailing spaces
	var lines []string
	for _, row := range result {
		line := string(row)
		line = strings.TrimRight(line, " ")
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// PlotSingle is a convenience method for plotting a single series.
func (c *Chart) PlotSingle(series []float64) string {
	return c.Plot([][]float64{series})
}

func formatLabel(val float64, format string) string {
	if !strings.Contains(format, "%") {
		return strings.Repeat(" ", 12)
	}
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "     NaN "
	}
	if val == 0 {
		return "    0.00 "
	}
	neg := val < 0
	absVal := val
	if neg {
		absVal = -val
	}
	intPart := int(absVal)
	fracPart := absVal - float64(intPart)
	frac2 := int(fracPart*100 + 0.5)

	suffix := " "
	prefix := " "
	if neg {
		prefix = "-"
	}

	intStr := formatInt(intPart)
	padLen := 7 - len(strings.TrimSpace(intStr))
	if padLen < 0 {
		padLen = 0
	}

	var s strings.Builder
	s.WriteString(prefix)
	s.WriteString(strings.Repeat(" ", padLen))
	s.WriteString(intStr)
	s.WriteString(".")
	if frac2 < 10 {
		s.WriteString("0")
	}
	s.WriteString(formatInt(frac2))
	s.WriteString(suffix)
	return s.String()
}

func formatInt(n int) string {
	return strconv.Itoa(n)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
