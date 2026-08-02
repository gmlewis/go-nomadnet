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

func TestUrwidColumnsFocusNavigation(t *testing.T) {
	t.Parallel()

	b1 := NewUrwidButton("Back")
	b2 := NewUrwidButton("Connect")
	b3 := NewUrwidButton("Save")

	cols := newURWIDColumns(0,
		b1,
		tview.NewBox(),
		b2,
		tview.NewBox(),
		b3,
	)

	app := tview.NewApplication()
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	cols.Focus(setFocus)

	if !b1.HasFocus() {
		t.Fatalf("expected b1 (Back) to have focus initially")
	}

	handler := cols.InputHandler()

	// Press RightArrow to move focus to b2 (Connect)
	handler(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), setFocus)
	if !b2.HasFocus() {
		t.Errorf("expected b2 (Connect) to have focus after RightArrow")
	}

	// Press RightArrow to move focus to b3 (Save)
	handler(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), setFocus)
	if !b3.HasFocus() {
		t.Errorf("expected b3 (Save) to have focus after second RightArrow")
	}

	// Press LeftArrow to move focus back to b2 (Connect)
	handler(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), setFocus)
	if !b2.HasFocus() {
		t.Errorf("expected b2 (Connect) to have focus after LeftArrow")
	}

	// Press LeftArrow to move focus back to b1 (Back)
	handler(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), setFocus)
	if !b1.HasFocus() {
		t.Errorf("expected b1 (Back) to have focus after second LeftArrow")
	}
}

func TestUrwidColumnsSetFocusIndex(t *testing.T) {
	t.Parallel()

	b1 := NewUrwidButton("Back")
	b2 := NewUrwidButton("Connect")

	cols := newURWIDColumns(0, b1, tview.NewBox(), b2)

	app := tview.NewApplication()
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	cols.SetFocusIndex(2)
	cols.Focus(setFocus)
	if !b2.HasFocus() {
		t.Errorf("expected b2 to have focus after SetFocusIndex(2) and Focus")
	}

	if cols.FocusIndex() != 2 {
		t.Errorf("got FocusIndex %v, want 2", cols.FocusIndex())
	}
}

func TestUrwidColumnsWithInputField(t *testing.T) {
	t.Parallel()

	kr := &killRing{}
	input := NewReadlineEdit(kr, "", "")
	btn := NewUrwidButton("Toggle")

	cols := newURWIDColumns(1, input, btn)

	app := tview.NewApplication()
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	cols.Focus(setFocus)

	if !input.HasFocus() {
		t.Fatalf("expected input to have focus initially")
	}

	handler := cols.InputHandler()

	// RightArrow on input field should be passed to input field, not change column focus
	handler(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), setFocus)
	if !input.HasFocus() {
		t.Errorf("expected input field to retain focus on RightArrow")
	}

	// Tab on input field should switch column focus to btn
	handler(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), setFocus)
	if !btn.HasFocus() {
		t.Errorf("expected btn to have focus after Tab")
	}

	// Backtab on btn should switch column focus back to input
	handler(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone), setFocus)
	if !input.HasFocus() {
		t.Errorf("expected input to have focus after Backtab")
	}
}
