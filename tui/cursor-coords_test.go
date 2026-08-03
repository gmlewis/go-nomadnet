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

import "testing"

// TestCalcCoordsGolden verifies CalcCoords against (x,y) values captured from
// Python urwid.text_layout.calc_coords (urwid 4.x, default "space" wrap). The
// golden table was produced by tooling/tui-parity/calc_coords_golden.py.
// Captures cannot see the terminal hardware cursor, so correctness is asserted
// here against the Python reference instead.
func TestCalcCoordsGolden(t *testing.T) {
	type coord struct{ x, y int }
	cases := []struct {
		text   string
		maxcol int
		coords map[int]coord
	}{
		{
			text:   "The quick brown fox jumps over the lazy dog",
			maxcol: 20,
			coords: map[int]coord{
				0: {0, 0}, 1: {1, 0}, 5: {5, 0}, 10: {10, 0}, 18: {18, 0},
				19: {19, 0}, 20: {0, 1}, 21: {1, 1}, 25: {5, 1}, 30: {10, 1},
				39: {19, 1}, 41: {1, 2}, 43: {3, 2}, 44: {3, 2},
			},
		},
		{
			text:   "hello",
			maxcol: 20,
			coords: map[int]coord{0: {0, 0}, 3: {3, 0}, 5: {5, 0}},
		},
		{
			text:   "abcdefghijklmnopqrstuvwxyz",
			maxcol: 10,
			coords: map[int]coord{
				0: {0, 0}, 5: {5, 0}, 9: {9, 0}, 10: {0, 1}, 11: {1, 1},
				20: {0, 2}, 25: {5, 2}, 26: {6, 2},
			},
		},
		{
			text:   "one two three four",
			maxcol: 8,
			coords: map[int]coord{
				0: {0, 0}, 3: {3, 0}, 4: {4, 0}, 7: {7, 0}, 8: {0, 1},
				11: {3, 1}, 12: {4, 1}, 17: {3, 2}, 18: {4, 2},
			},
		},
		{
			text:   "  leading spaces here",
			maxcol: 12,
			coords: map[int]coord{
				0: {0, 0}, 2: {2, 0}, 9: {9, 0}, 12: {2, 1}, 13: {3, 1},
				21: {11, 1}, 22: {11, 1},
			},
		},
		{
			text:   "word",
			maxcol: 4,
			coords: map[int]coord{0: {0, 0}, 2: {2, 0}, 4: {4, 0}},
		},
		{
			text:   "a b c d e f",
			maxcol: 3,
			coords: map[int]coord{
				0: {0, 0}, 1: {1, 0}, 2: {2, 0}, 3: {3, 0}, 4: {0, 1},
				5: {1, 1}, 6: {2, 1}, 10: {2, 2}, 11: {3, 2},
			},
		},
		{
			text:   "",
			maxcol: 20,
			coords: map[int]coord{0: {0, 0}},
		},
	}
	for _, c := range cases {
		for pos, want := range c.coords {
			gotX, gotY := CalcCoords(c.text, c.maxcol, pos)
			if gotX != want.x || gotY != want.y {
				t.Errorf("CalcCoords(%q, %v, %v) = (%v,%v), want (%v,%v)",
					c.text, c.maxcol, pos, gotX, gotY, want.x, want.y)
			}
		}
	}
}
