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
	"embed"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

//go:embed testdata/ascii_parity.json
var asciiFS embed.FS

type asciiCase struct {
	Label  string         `json:"label"`
	Glyph  string         `json:"glyph"`
	Series [][]*float64   `json:"series"`
	Cfg    map[string]any `json:"cfg"`
	Result string         `json:"result"`
}

// pyFormatToGo converts a Python format placeholder like "{:8.2f} " to a Go
// fmt verb like "%8.2f ".
func pyFormatToGo(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "{:", "%")
	s = strings.ReplaceAll(s, "}", "")
	return s
}

func TestPlotPythonParity(t *testing.T) {
	t.Parallel()
	data, err := asciiFS.ReadFile("testdata/ascii_parity.json")
	if err != nil {
		t.Fatalf("read embed: %v", err)
	}
	var cases []asciiCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.Label, func(t *testing.T) {
			t.Parallel()
			c := New(tc.Glyph)
			if tc.Cfg != nil {
				if v, ok := tc.Cfg["offset"]; ok {
					c.Offset = toInt(v)
				}
				if v, ok := tc.Cfg["height"]; ok {
					c.Height = toInt(v)
				}
				if v, ok := tc.Cfg["min"]; ok {
					f := toFloat(v)
					c.Min = &f
				}
				if v, ok := tc.Cfg["max"]; ok {
					f := toFloat(v)
					c.Max = &f
				}
				if v, ok := tc.Cfg["format"]; ok {
					if s, ok := v.(string); ok {
						c.Format = pyFormatToGo(s)
					}
				}
			}
			got := c.Plot(toFloatSeries(tc.Series))
			if got != tc.Result {
				t.Errorf("Plot(%v) mismatch:\n--- got ---\n%v\n--- want ---\n%v", tc.Label, got, tc.Result)
			}
		})
	}
}

func toFloatSeries(s [][]*float64) [][]float64 {
	out := make([][]float64, len(s))
	for i, row := range s {
		out[i] = make([]float64, len(row))
		for j, v := range row {
			if v == nil {
				out[i][j] = math.NaN()
			} else {
				out[i][j] = *v
			}
		}
	}
	return out
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}
