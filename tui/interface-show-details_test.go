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

package tui

import (
	"strings"
	"testing"
)

// TestInterfaceShowDetailsFitsConsoleGrid pins the ShowInterface layout inside
// the 100x40 grid an appliance console provides: the frame fits, every drawn box
// lines up with its own borders, and the interface's own settings are listed.
//
// The two states below are the two the view is read in: the appliance's own RNode
// interface — enabled, and disconnected by design whenever no radio is attached —
// and a connected TCP interface.
func TestInterfaceShowDetailsFitsConsoleGrid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		info  InterfaceInfo
		width int
		want  []string
	}{
		{
			// The interface and defaults the appliance itself writes
			// (NodeConfig.renderRadio / RNodeSpec).
			name: "rnode enabled and disconnected",
			info: InterfaceInfo{
				Name:      "RNode LoRa",
				Type:      "RNodeInterface",
				Enabled:   true,
				Connected: false,
				Params: map[string]string{
					"port":            "/dev/ttyACM0",
					"frequency":       "915000000",
					"bandwidth":       "125000",
					"txpower":         "17",
					"spreadingfactor": "9",
					"codingrate":      "5",
				},
			},
			want: []string{
				"Radio Parameters",
				"Port: /dev/ttyACM0",
				// Python's create_param_row formats these from the Hz value
				// (Interfaces.py:2404-2409).
				"Frequency: 915.000 MHz",
				"Bandwidth: 125.0 kHz",
			},
		},
		{
			name: "tcp interface connected",
			info: InterfaceInfo{
				Name:      "g00n Cloud Dallas",
				Type:      "TCPClientInterface",
				Enabled:   true,
				Connected: true,
				TX:        18432,
				RX:        460800,
				Params: map[string]string{
					"target_host": "dfw.us.g00n.cloud",
					"target_port": "6969",
					"mode":        "full",
				},
			},
			want: []string{
				"Connection Parameters",
				"Target Host: dfw.us.g00n.cloud",
				"Target Port: 6969",
				"Mode: full",
			},
		},
		{
			// Python draws the two traffic boxes side by side only at 145
			// columns and above (Interfaces.py:2350-2360).
			name:  "tcp interface connected on a wide grid",
			width: 160,
			info: InterfaceInfo{
				Name:      "g00n Cloud Dallas",
				Type:      "TCPClientInterface",
				Enabled:   true,
				Connected: true,
				Params: map[string]string{
					"target_host": "dfw.us.g00n.cloud",
					"target_port": "6969",
				},
			},
			want: []string{
				"Connection Parameters",
				"Target Host: dfw.us.g00n.cloud",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			width := tc.width
			if width == 0 {
				width = 100
			}
			app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
			s := NewInterfaceShow(app, tc.info)
			rows := drawRows(t, s.Widget(), width, 40)
			joined := strings.Join(rows, "\n")

			t.Logf("rendered at %vx40:\n%v", width, joined)

			for _, want := range tc.want {
				if !strings.Contains(joined, want) {
					t.Errorf("the interface's own settings are not rendered: %q missing", want)
				}
			}
			if strings.Contains(joined, "No additional parameters") {
				t.Errorf("the view says there are no parameters while the interface has settings")
			}

			assertDrawnBoxesLineUp(t, rows, width)
		})
	}
}

// assertDrawnBoxesLineUp checks every box drawn inside the frame: each box's
// contents must end at the same column as that box's own borders, and the last
// box must reach the body's right edge. Without it a box can be drawn narrower
// than the text inside it, so its borders and its contents disagree.
func assertDrawnBoxesLineUp(t *testing.T, rows []string, gridWidth int) {
	t.Helper()

	const frameLeft = 0
	frameRight := gridWidth - 1

	runs := 0
	for _, run := range drawnBoxRuns(rows) {
		runs++
		top := []rune(run[0])
		starts := runePositions(top, '┌')
		ends := runePositions(top, '┐')
		if len(starts) != len(ends) || len(starts) == 0 {
			t.Fatalf("box top border %q does not pair its corners", run[0])
		}
		for i, start := range starts {
			if start <= frameLeft {
				t.Fatalf("box top border %q starts on the frame's left border", run[0])
			}
			end := ends[i]
			if end <= start {
				t.Fatalf("box top border %q does not close inside the frame", run[0])
			}
			for _, row := range run {
				rs := []rune(row)
				if end >= len(rs) {
					t.Errorf("box row %q is shorter than its own top border", row)
					continue
				}
				switch rs[end] {
				case '┐', '│', '┘':
				default:
					t.Errorf("box row %q has %q at column %d, where every other row of the box has its border",
						row, rs[end], end)
				}
			}
		}

		// Python's traffic boxes fill the body's width (Interfaces.py:2332-2340):
		// stacked below 145 columns, side by side above it. Either way the
		// right-hand box ends where the body's padding ends.
		lastEnd := ends[len(ends)-1]
		if frameRight-lastEnd < 2 {
			t.Errorf("box top border %q runs into the frame's right border", run[0])
		}
		if frameRight-lastEnd > 3 {
			t.Errorf("box top border %q stops at column %d of a %d-column grid, short of the body's right edge",
				run[0], lastEnd, frameRight+1)
		}
	}
	if runs == 0 {
		t.Errorf("no box was drawn; the traffic/notice box is missing from the view")
	}
}

// runePositions returns every column holding want.
func runePositions(runes []rune, want rune) []int {
	var out []int
	for i, r := range runes {
		if r == want {
			out = append(out, i)
		}
	}
	return out
}

// drawnBoxRuns returns the runs of drawn rows that form box art inside the
// frame — one run per box, from its "┌" row through its "└" row. The frame's own
// border is skipped: it is the row that starts in column 0.
func drawnBoxRuns(rows []string) [][]string {
	var runs [][]string
	var cur []string
	for _, row := range rows {
		if i := indexRune([]rune(row), '┌'); i > 0 {
			cur = []string{row}
			continue
		}
		if cur != nil {
			cur = append(cur, row)
			if strings.ContainsRune(row, '└') {
				runs = append(runs, cur)
				cur = nil
			}
		}
	}
	return runs
}

func indexRune(runes []rune, want rune) int {
	for i, r := range runes {
		if r == want {
			return i
		}
	}
	return -1
}
