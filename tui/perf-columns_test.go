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
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// BenchmarkURWIDColumnsSetRect isolates the per-frame layout cost of the
// Columns widget: every full-screen redraw runs SetRect on every Columns row
// on screen, which used to recompute (and re-sort) the urwid width table from
// scratch on every call — allocations and closures per columns row per frame
// even though the geometry only changes when the total width does.
func BenchmarkURWIDColumnsSetRect(b *testing.B) {
	c := newURWIDColumns(1, tview.NewTextView(), tview.NewTextView(), tview.NewTextView())
	c.SetWeight(2, 3)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.SetRect(0, 0, 100, 30)
	}
}

// drawURWIDColumnsFrame draws a fixed columns row onto a settled screen — the
// full Draw-side frame cost (layout + child draws) for one columns row.
func drawURWIDColumnsFrame(b *testing.B, screen tcell.Screen, c *urwidColumns) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.SetRect(0, 0, 100, 30)
		c.Draw(screen)
	}
}

// BenchmarkURWIDColumnsDraw measures the full per-frame Draw cost of a single
// three-column row on a settled screen (see BenchmarkURWIDColumnsSetRect).
func BenchmarkURWIDColumnsDraw(b *testing.B) {
	screen := newBenchScreen()
	defer screen.Fini()
	c := newURWIDColumns(1, tview.NewTextView(), tview.NewTextView(), tview.NewTextView())
	c.SetWeight(2, 3)
	drawURWIDColumnsFrame(b, screen, c)
}
