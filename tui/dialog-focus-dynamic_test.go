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

// TestNextActiveItemSkipsInactive pins the dynamic dialog-nav traversal: the
// Add Room dialog's key field only EXISTS while "Keyed room (+k)" is checked
// (Python update_key_visibility, Channels.py:1946-1955), so the traversal must
// skip the inactive item instead of focusing a detached widget whose capture
// swallows every further key.
func TestNextActiveItemSkipsInactive(t *testing.T) {
	t.Parallel()

	a := tview.NewInputField()
	b := tview.NewInputField()
	c := tview.NewInputField()
	items := []tview.Primitive{a, b, c}
	active := func(p tview.Primitive) bool { return p != b }

	if got := nextActiveItem(items, a, 1, active); got != c {
		t.Errorf("Tab from a must skip the inactive b and land on c, got %v", nextActiveItem(items, a, 1, active))
	}
	if got := nextActiveItem(items, c, -1, active); got != a {
		t.Errorf("Backtab from c must skip the inactive b and land on a, got %v", got)
	}
	if got := nextActiveItem(items, a, -1, active); got != nil {
		t.Errorf("no wrap: Backtab at the first active item should stay, got %v", got)
	}
	if got := nextActiveItem(items, c, 1, active); got != nil {
		t.Errorf("no wrap: Tab at the last active item should stay, got %v", got)
	}
	if got := nextActiveItem(items, b, 1, active); got != nil {
		t.Errorf("an absent item should have no successor, got %v", got)
	}
}

// TestWireDialogNavDynamicSkipsInactive drives the installed captures end to
// end: initial focus lands on the first ACTIVE item and Tab hops over the
// inactive one.
func TestWireDialogNavDynamicSkipsInactive(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	a := tview.NewInputField()
	b := tview.NewInputField()
	c := tview.NewInputField()
	active := func(p tview.Primitive) bool { return p != b }

	wireDialogNavDynamic(app, nil, []tview.Primitive{a, b, c}, active)
	if got := app.GetFocus(); got != a {
		t.Fatalf("initial focus should be the first active item, got %v", got)
	}

	if h := a.InputHandler(); h != nil {
		h(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	}
	if got := app.GetFocus(); got != c {
		t.Fatalf("Tab from the first item must skip the inactive item, got %v", got)
	}

	if h := c.InputHandler(); h != nil {
		h(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	}
	if got := app.GetFocus(); got != a {
		t.Fatalf("Backtab from the last item must skip the inactive item, got %v", got)
	}
}
