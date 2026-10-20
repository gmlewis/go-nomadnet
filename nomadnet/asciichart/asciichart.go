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
	"errors"
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
	// HasHeight marks Height as explicitly set. Python's height comes from the
	// cfg dict and `cfg.get('height', interval)` distinguishes an absent height
	// from an explicit 0; Go's zero Height means "unset" unless this is set.
	HasHeight bool
	Format    string
	// FormatFunc, when set, overrides Format with a callable label formatter,
	// mirroring the Python AsciiChart cfg['format'] hook when it is a function
	// rather than a format string (e.g. RNS.prettyspeed, Interfaces.py:1341-1353).
	// Python right-justifies the returned string to 12 columns
	// (AsciiChart.py:42-43); FormatFunc labels are padded the same way.
	FormatFunc func(float64) string
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
func (c *Chart) Plot(series [][]float64) (string, error) {
	if len(series) == 0 {
		return "", nil
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
		return "", nil
	}

	if c.Min != nil {
		minVal = *c.Min
	}
	if c.Max != nil {
		maxVal = *c.Max
	}
	// Python raises ValueError here (AsciiChart.py:21-22); an inverted range
	// must be reported, not silently swapped.
	if minVal > maxVal {
		return "", errors.New("the min value cannot exceed the max value")
	}

	interval := maxVal - minVal
	offset := max(c.Offset, 0)

	// Python: height = cfg.get('height', interval); ratio = height/interval
	// if interval > 0 else 1. An unset height (c.Height == 0) therefore yields
	// ratio = 1, matching Python's default height == interval.
	ratio := 1.0
	if interval > 0 && (c.HasHeight || c.Height != 0) {
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
		if c.FormatFunc != nil {
			label = rjust(c.FormatFunc(labelVal), 12)
		}
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
	return strings.Join(lines, "\n"), nil
}

// PlotSingle is a convenience method for plotting a single series.
func (c *Chart) PlotSingle(series []float64) (string, error) {
	return c.Plot([][]float64{series})
}

// formatLabel formats a Y-axis value. Python's cfg['format'] is a str.format
// template ("{:8.2f} ", "{}", or even a string with no field); a Go fmt verb is
// also accepted. A template with no placeholder yields the literal text, as
// Python's str.format does.
func formatLabel(val float64, format string) string {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "     NaN "
	}
	if strings.ContainsAny(format, "{}") {
		return pythonFormat(format, val)
	}
	if strings.Contains(format, "%") {
		return fmt.Sprintf(format, val)
	}
	// No placeholder: Python's str.format returns the literal template.
	return format
}

// pythonFormat applies a Python str.format template to one float.
func pythonFormat(template string, val float64) string {
	var b strings.Builder
	for i := 0; i < len(template); i++ {
		c := template[i]
		switch {
		case c == '{' && i+1 < len(template) && template[i+1] == '{':
			b.WriteByte('{')
			i++
		case c == '}' && i+1 < len(template) && template[i+1] == '}':
			b.WriteByte('}')
			i++
		case c == '{':
			end := strings.IndexByte(template[i:], '}')
			if end < 0 {
				b.WriteString(template[i:])
				i = len(template)
				continue
			}
			field := template[i+1 : i+end]
			if strings.HasPrefix(field, ":") {
				b.WriteString(fmt.Sprintf("%"+field[1:], val))
			} else {
				// An empty or named field binds this call's only value; Python
				// renders it with str(), which keeps the ".0" on an integral
				// float.
				b.WriteString(pyFloatString(val))
			}
			i += end
		default:
			if c == '%' {
				b.WriteString("%%")
			} else {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// pyFloatString renders a float the way Python's format(value, "") (str) does:
// shortest round-trip, with ".0" for an integral value.
func pyFloatString(val float64) string {
	s := strconv.FormatFloat(val, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEnN") {
		s += ".0"
	}
	return s
}

func formatInt(n int) string {
	return strconv.Itoa(n)
}

// rjust right-justifies s in a field of width columns, mirroring Python's
// str.rjust (padding on the left, no truncation when s is already at least
// width runes wide).
func rjust(s string, width int) string {
	n := len([]rune(s))
	if n >= width {
		return s
	}
	return strings.Repeat(" ", width-n) + s
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
