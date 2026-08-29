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

	"github.com/rivo/tview"
)

// TestURWIDColumnsWidthCacheReused verifies SetRect reuses the cached width
// table when the total width and layout config are unchanged (the per-frame
// hot path draws the same columns row many times at a settled geometry).
func TestURWIDColumnsWidthCacheReused(t *testing.T) {
	c := newURWIDColumns(1, tview.NewTextView(), tview.NewTextView(), tview.NewTextView())
	c.SetFixedWidth(0, 52)

	c.SetRect(0, 0, 100, 30)
	first := c.widthsCache
	if first == nil {
		t.Fatal("SetRect did not populate the width cache")
	}
	c.SetRect(0, 0, 100, 30)
	if &first[0] != &c.widthsCache[0] {
		t.Errorf("SetRect recomputed the width table for an unchanged layout: got %v, want reuse of %v", c.widthsCache, first)
	}
}

// TestURWIDColumnsWidthCacheInvalidatedByConfigChange verifies that mutating
// the layout config (weights, fixed widths) after a SetRect forces a
// recompute — the cached table must never serve a stale layout. The Network
// page toggles its list pane's fixed width dynamically (fullscreen list),
// so this invalidation is load-bearing.
func TestURWIDColumnsWidthCacheInvalidatedByConfigChange(t *testing.T) {
	t.Run("SetFixedWidth", func(t *testing.T) {
		c := newURWIDColumns(0, tview.NewTextView(), tview.NewTextView())
		c.SetRect(0, 0, 100, 30)
		hidden := append([]int(nil), c.widthsCache...)

		c.SetFixedWidth(0, 0) // hide column 0 (fullscreen list)
		c.SetRect(0, 0, 100, 30)
		if len(c.widthsCache) != 2 || c.widthsCache[0] != 0 || c.widthsCache[1] != 100 {
			t.Errorf("cached widths not recomputed after SetFixedWidth: got %v (before: %v), want [0 100]", c.widthsCache, hidden)
		}
	})

	t.Run("SetWeight", func(t *testing.T) {
		c := newURWIDColumns(0, tview.NewTextView(), tview.NewTextView())
		c.SetRect(0, 0, 100, 30)
		even := append([]int(nil), c.widthsCache...)

		c.SetWeight(0, 3)
		c.SetRect(0, 0, 100, 30)
		if c.widthsCache[0] == even[0] && even[0] != c.widthsCache[1] {
			t.Errorf("weights %v change after SetWeight not reflected in cached widths: got %v, before %v", []int{3, 1}, c.widthsCache, even)
		}
	})
}

// TestURWIDColumnsWidthCacheMatchesFreshComputation cross-checks the cached
// table against a from-scratch computation after alternating geometry and
// config changes, so the cache can never drift from urwidColumnWidthsEx.
func TestURWIDColumnsWidthCacheMatchesFreshComputation(t *testing.T) {
	c := newURWIDColumns(1, tview.NewTextView(), tview.NewTextView(), tview.NewTextView())
	for _, step := range []struct {
		width    int
		weight   int
		idx      int
		fixedW   int
		useFixed bool
	}{
		{width: 100, weight: 1, idx: 0, fixedW: 0, useFixed: false},
		{width: 100, weight: 3, idx: 0, fixedW: 0, useFixed: false},
		{width: 80, weight: 3, idx: 0, fixedW: 0, useFixed: false},
		{width: 80, weight: 3, idx: 1, fixedW: 52, useFixed: true},
		{width: 100, weight: 3, idx: 1, fixedW: 52, useFixed: true},
	} {
		c.SetWeight(step.idx, step.weight)
		if step.useFixed {
			c.SetFixedWidth(step.idx, step.fixedW)
		} else {
			c.SetFixedWidth(step.idx, -1)
		}
		c.SetRect(0, 0, step.width, 30)
		want := urwidColumnWidthsEx(step.width, c.weights, c.fixedWidths, c.dividechars)
		for i := range want {
			if c.widthsCache[i] != want[i] {
				t.Fatalf("step %+v: cached widths %v != fresh %v", step, c.widthsCache, want)
			}
		}
	}
}
