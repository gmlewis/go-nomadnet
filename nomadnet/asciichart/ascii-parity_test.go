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
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// asciiInputs is the input battery for the live cross-implementation parity
// check. The Go test owns these inputs; the expected rendered outputs are
// derived FRESH on every run by executing the real Python
// nomadnet.vendor.AsciiChart reference (see asciiPythonOnce). A nil element in
// a series row represents NaN (a gap), mirroring Python's use of float('nan').
var asciiInputs = []struct {
	Label  string         `json:"label"`
	Glyph  string         `json:"glyph"`
	Series [][]*float64   `json:"series"`
	Cfg    map[string]any `json:"cfg"`
}{
	{"rising_unicode", "unicode", [][]*float64{{new(1.0), new(2.0), new(3.0), new(4.0), new(5.0)}}, nil},
	{"falling_unicode", "unicode", [][]*float64{{new(5.0), new(4.0), new(3.0), new(2.0), new(1.0)}}, nil},
	{"plain_wave", "plain", [][]*float64{{new(1.0), new(2.0), new(3.0), new(2.0), new(1.0)}}, nil},
	{"with_nan", "unicode", [][]*float64{{new(1.0), nil, new(3.0), new(4.0), new(5.0)}}, nil},
	{"minmax", "unicode", [][]*float64{{new(2.0), new(4.0), new(6.0), new(8.0)}}, map[string]any{"min": 0, "max": 10}},
	{"height", "unicode", [][]*float64{{new(1.0), new(2.0), new(3.0), new(4.0), new(5.0)}}, map[string]any{"height": 2}},
	{"offset", "unicode", [][]*float64{{new(1.0), new(2.0), new(3.0), new(4.0), new(5.0)}}, map[string]any{"offset": 5}},
	{"multi", "unicode", [][]*float64{{new(1.0), new(2.0), new(3.0), new(4.0), new(5.0)}, {new(5.0), new(4.0), new(3.0), new(2.0), new(1.0)}}, nil},
	{"constant", "unicode", [][]*float64{{new(5.0), new(5.0), new(5.0), new(5.0), new(5.0)}}, nil},
	{"custom_format", "unicode", [][]*float64{{new(1.0), new(2.0), new(3.0), new(4.0), new(5.0)}}, map[string]any{"format": "{:6.1f}"}},
	{"negative", "unicode", [][]*float64{{new(-5.0), new(0.0), new(5.0)}}, nil},
	{"bigvals", "unicode", [][]*float64{{new(100.0), new(200.0), new(300.0), new(250.0), new(150.0)}}, nil},
	{"fractional", "unicode", [][]*float64{{new(1.5), new(2.25), new(3.125), new(2.5), new(1.875)}}, nil},
	{"single_point", "unicode", [][]*float64{{new(7.0)}}, nil},
	{"two_point", "unicode", [][]*float64{{new(1.0), new(9.0)}}, nil},
}

// asciiParityScript imports the real nomadnet.vendor.AsciiChart reference and
// runs plot on each input case supplied as JSON on stdin, emitting the fresh
// rendered strings as a JSON list on stdout. JSON null elements in a series
// row are converted to float('nan') before calling plot, matching the Go
// representation of a gap.
const asciiParityScript = `
import sys, json, math
from nomadnet.vendor.AsciiChart import AsciiChart
def fix(s):
    if isinstance(s, list):
        return [fix(x) for x in s]
    return math.nan if s is None else s
cases = json.loads(sys.stdin.read() or "[]")
out = []
for c in cases:
    ch = AsciiChart(c.get("glyph", "unicode"))
    out.append(ch.plot(fix(c["series"]), c.get("cfg")))
print(json.dumps(out, ensure_ascii=False))
`

// asciiPythonOnce caches the single live Python run that derives fresh expected
// rendered outputs for every asciichart parity case, so the test below shares
// one python3 exec instead of one per case.
var (
	asciiPythonOnce sync.Once
	asciiPythonOut  []string
)

func asciiPython(t *testing.T) []string {
	t.Helper()
	asciiPythonOnce.Do(func() {
		testutils.RunPythonNomadnet(t, asciiInputs, asciiParityScript, &asciiPythonOut)
	})
	return asciiPythonOut
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

// TestPlotPythonParity runs the Go asciichart Plot against the input battery
// and diffs each rendered chart against the FRESH output of the real Python
// nomadnet.vendor.AsciiChart reference executed live via python3. The test is
// skipped (not failed) when the Python nomadnet reference is not importable.
func TestPlotPythonParity(t *testing.T) {
	t.Parallel()
	want := asciiPython(t)
	for i, tc := range asciiInputs {
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
					fv := toFloat(v)
					c.Min = &fv
				}
				if v, ok := tc.Cfg["max"]; ok {
					fv := toFloat(v)
					c.Max = &fv
				}
				if v, ok := tc.Cfg["format"]; ok {
					if s, ok := v.(string); ok {
						c.Format = pyFormatToGo(s)
					}
				}
			}
			got := c.Plot(toFloatSeries(tc.Series))
			if got != want[i] {
				t.Errorf("Plot(%v) mismatch:\n--- got ---\n%v\n--- want ---\n%v", tc.Label, got, want[i])
			}
		})
	}
}
