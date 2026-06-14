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

package asciichart

import (
	"math"
	"strings"
	"testing"
)

func TestNewUnicode(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	if c.Symbols.Cross != "┼" {
		t.Errorf("unicode cross = %q, want %q", c.Symbols.Cross, "┼")
	}
	if c.Offset != 3 {
		t.Errorf("offset = %d, want 3", c.Offset)
	}
}

func TestNewPlain(t *testing.T) {
	t.Parallel()

	c := New("plain")
	if c.Symbols.Cross != "+" {
		t.Errorf("plain cross = %q, want %q", c.Symbols.Cross, "+")
	}
}

func TestNewDefault(t *testing.T) {
	t.Parallel()

	c := New("unknown")
	if c.Symbols.Cross != "┼" {
		t.Error("unknown glyph set did not default to unicode")
	}
}

func TestPlotEmpty(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	result := c.Plot([][]float64{})
	if result != "" {
		t.Errorf("Plot empty = %q, want empty", result)
	}
}

func TestPlotSingleSeries(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	series := []float64{1, 2, 3, 4, 5}
	result := c.PlotSingle(series)

	if len(result) == 0 {
		t.Error("PlotSingle returned empty")
	}

	// Should have multiple lines
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		t.Errorf("PlotSingle returned %d lines, want >= 2", len(lines))
	}
}

func TestPlotMultipleSeries(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	series := [][]float64{
		{1, 2, 3, 4, 5},
		{5, 4, 3, 2, 1},
	}
	result := c.Plot(series)

	if len(result) == 0 {
		t.Error("Plot multiple series returned empty")
	}
}

func TestPlotWithNaN(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	series := []float64{1, math.NaN(), 3, 4, 5}
	result := c.PlotSingle(series)

	if len(result) == 0 {
		t.Error("Plot with NaN returned empty")
	}
}

func TestPlotAllNaN(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	series := []float64{math.NaN(), math.NaN(), math.NaN()}
	result := c.PlotSingle(series)

	if result != "" {
		t.Errorf("Plot all NaN = %q, want empty", result)
	}
}

func TestPlotConstant(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	series := []float64{5, 5, 5, 5, 5}
	result := c.PlotSingle(series)

	if len(result) == 0 {
		t.Error("Plot constant returned empty")
	}
}

func TestPlotWithMinMax(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	minVal := 0.0
	maxVal := 10.0
	c.Min = &minVal
	c.Max = &maxVal

	series := []float64{2, 4, 6, 8}
	result := c.PlotSingle(series)

	if len(result) == 0 {
		t.Error("Plot with min/max returned empty")
	}
}

func TestPlotMinMaxExceeded(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	series := []float64{-5, 0, 5}
	// Should not panic even with values outside default range
	result := c.PlotSingle(series)

	if len(result) == 0 {
		t.Error("Plot with exceeded min/max returned empty")
	}
}

func TestPlotPlain(t *testing.T) {
	t.Parallel()

	c := New("plain")
	series := []float64{1, 2, 3, 2, 1}
	result := c.PlotSingle(series)

	if len(result) == 0 {
		t.Error("Plot plain returned empty")
	}

	// Should not contain Unicode box drawing chars
	if strings.Contains(result, "┼") || strings.Contains(result, "│") {
		t.Error("Plain chart contains Unicode characters")
	}
}

func TestPlotWidth(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	c.Offset = 5
	series := []float64{1, 2, 3, 4, 5}
	result := c.PlotSingle(series)

	lines := strings.Split(result, "\n")
	for _, line := range lines {
		// Each line should have at least offset + series width
		if len([]rune(line)) < 5+5 {
			t.Errorf("Line too short: %q", line)
		}
	}
}

func TestPlotHeight(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	c.Height = 2
	series := []float64{1, 2, 3, 4, 5}
	result := c.PlotSingle(series)

	lines := strings.Split(result, "\n")
	// Height should limit vertical extent
	if len(lines) > 10 {
		t.Errorf("Plot height too large: %d lines", len(lines))
	}
}

func TestPlotRising(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	series := []float64{1, 2, 3, 4, 5}
	result := c.PlotSingle(series)

	lines := strings.Split(result, "\n")
	// Rising series should have corner characters
	hasCorner := false
	for _, line := range lines {
		if strings.Contains(line, "╭") || strings.Contains(line, "╰") {
			hasCorner = true
			break
		}
	}
	if !hasCorner {
		t.Error("Rising series missing corner characters")
	}
}

func TestPlotFalling(t *testing.T) {
	t.Parallel()

	c := New("unicode")
	series := []float64{5, 4, 3, 2, 1}
	result := c.PlotSingle(series)

	lines := strings.Split(result, "\n")
	hasCorner := false
	for _, line := range lines {
		if strings.Contains(line, "╮") || strings.Contains(line, "╯") {
			hasCorner = true
			break
		}
	}
	if !hasCorner {
		t.Error("Falling series missing corner characters")
	}
}

func TestFormatInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{123, "123"},
		{-1, "-1"},
		{-42, "-42"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := formatInt(tt.input)
			if got != tt.want {
				t.Errorf("formatInt(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestUnicodeSymbols(t *testing.T) {
	t.Parallel()

	s := UnicodeSymbols
	if s.Cross != "┼" || s.TeeRight != "┤" || s.VertLine != "│" {
		t.Error("UnicodeSymbols values incorrect")
	}
}

func TestPlainSymbols(t *testing.T) {
	t.Parallel()

	s := PlainSymbols
	if s.Cross != "+" || s.TeeRight != "|" || s.VertLine != "|" {
		t.Error("PlainSymbols values incorrect")
	}
}
