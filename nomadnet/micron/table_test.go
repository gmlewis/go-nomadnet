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

package micron

import "testing"

// TestFormatTableRaw asserts the Go port of MarkdownToMicron.format_table_raw
// (RNS/Utilities/rngit/util.py:530) produces byte-identical output to Python,
// including the canonical visible-width-vs-raw-emit behavior where cells
// containing micron color/format tags overflow their column physically. Golden
// values were captured from Python format_table_raw directly (no urwid needed).
func TestFormatTableRaw(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		rows     []string
		align    string // "" mirrors Python align=None
		maxWidth int
		want     []string
	}{
		{
			name:     "guide_example",
			align:    "c",
			maxWidth: 100,
			rows: []string{
				"| Name | Price | Qty |",
				"| ---- | :---: | --: |",
				"| `F3a3Apple`f | Free | `!5`! |",
				"| Orange | Ask, nicely | 3 |",
			},
			want: []string{
				"`c",
				"┌────────┬─────────────┬─────┐",
				"│ Name   │ Price       │ Qty │",
				"├────────┼─────────────┼─────┤",
				"│ `F3a3Apple`f  │    Free     │   `!5`! │",
				"│ Orange │ Ask, nicely │   3 │",
				"└────────┴─────────────┴─────┘",
				"`a",
			},
		},
		{
			name:     "simple_two_col",
			align:    "c",
			maxWidth: 100,
			rows: []string{
				"| A | B |",
				"| - | - |",
				"| 1 | 2 |",
			},
			want: []string{
				"`c",
				"┌─────┬─────┐",
				"│ A   │ B   │",
				"├─────┼─────┤",
				"│ 1   │ 2   │",
				"└─────┴─────┘",
				"`a",
			},
		},
		{
			name:     "shrink_to_maxwidth",
			align:    "c",
			maxWidth: 25,
			rows: []string{
				"| Name | Price | Qty |",
				"| ---- | :---: | --: |",
				"| Apple | Free | 5 |",
				"| Orange | Ask, nicely | 3 |",
			},
			want: []string{
				"`c",
				"┌────────┬────────┬─────┐",
				"│ Name   │ Price  │ Qty │",
				"├────────┼────────┼─────┤",
				"│ Apple  │  Free  │   5 │",
				"│ Orange │ Ask, … │   3 │",
				"└────────┴────────┴─────┘",
				"`a",
			},
		},
		{
			name:     "no_data_rows",
			align:    "c",
			maxWidth: 100,
			rows: []string{
				"| A | B |",
				"| - | - |",
			},
			want: []string{
				"`c",
				"┌─────┬─────┐",
				"│ A   │ B   │",
				"├─────┼─────┤",
				"└─────┴─────┘",
				"`a",
			},
		},
		{
			name:     "single_column",
			align:    "l",
			maxWidth: 100,
			rows: []string{
				"| H |",
				"| - |",
				"| x |",
			},
			want: []string{
				"`l",
				"┌─────┐",
				"│ H   │",
				"├─────┤",
				"│ x   │",
				"└─────┘",
				"`a",
			},
		},
		{
			name:     "no_table_align",
			align:    "",
			maxWidth: 100,
			rows: []string{
				"| A | B |",
				"| - | - |",
				"| 1 | 2 |",
			},
			want: []string{
				"┌─────┬─────┐",
				"│ A   │ B   │",
				"├─────┼─────┤",
				"│ 1   │ 2   │",
				"└─────┴─────┘",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FormatTableRaw(tc.rows, tc.align, tc.maxWidth)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %v, want %v\ngot=%#v", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				if got[i] != w {
					t.Errorf("line[%v]\n got = %q\nwant = %q", i, got[i], w)
				}
			}
		})
	}
}

// TestFormatTableRawTooFewRows asserts fewer than 2 rows are returned unchanged
// (Python: `if len(rows) < 2: return rows`).
func TestFormatTableRawTooFewRows(t *testing.T) {
	t.Parallel()
	rows := []string{"| A | B |"}
	got := FormatTableRaw(rows, "c", 100)
	if len(got) != 1 || got[0] != rows[0] {
		t.Errorf("got = %#v, want %#v unchanged", got, rows)
	}
}
