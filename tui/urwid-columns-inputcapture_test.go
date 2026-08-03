// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestURWIDColumnsInputCaptureHonored pins the fix for the Ctrl-L regression:
// urwidColumns.InputHandler must route through WrapInputHandler so that an
// input capture installed with SetInputCapture is actually consulted. Before
// the fix the handler returned a bare closure that never invoked the capture,
// so page-level shortcuts on the Network page (Ctrl-L toggles Saved
// Nodes/Announce Stream) silently did nothing. The capture must run, and when
// it returns the event unchanged the inner column handling must still run.
func TestURWIDColumnsInputCaptureHonored(t *testing.T) {
	t.Parallel()

	app := newTestApp()

	// Two focusable children so Tab can actually move column focus.
	a := tview.NewList()
	a.AddItem("a", "", 0, nil)
	b := tview.NewList()
	b.AddItem("b", "", 0, nil)
	cols := newURWIDColumns(0, a, b)
	app.SetFocus(cols)

	captureRan := false
	cols.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		captureRan = true
		if ev.Key() == tcell.KeyCtrlL {
			return nil // consume: toggle handled by the page
		}
		return ev
	})

	handler := cols.InputHandler()
	if handler == nil {
		t.Fatal("InputHandler() returned nil")
	}

	// Consumed key: the capture running at all is the regression pin. Before
	// the fix SetInputCapture was silently ignored.
	handler(tcell.NewEventKey(tcell.KeyCtrlL, 0, tcell.ModNone),
		func(p tview.Primitive) { app.SetFocus(p) })
	if !captureRan {
		t.Error("input capture was not invoked; SetInputCapture is ignored by urwidColumns")
	}

	// Forwarded key: capture returns the event unchanged, so the column handler
	// runs and (for KeyTab) moves focus between columns.
	captureRan = false
	before := cols.FocusIndex()
	handler(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone),
		func(p tview.Primitive) { app.SetFocus(p) })
	if !captureRan {
		t.Error("input capture was not invoked for forwarded key")
	}
	if cols.FocusIndex() == before {
		t.Errorf("Tab did not move column focus: still at %v", before)
	}
}

// TestURWIDColumnsInputCaptureConsumedSkipsChild pins the contract that when
// the input capture consumes an event (returns nil), the focused child's
// InputHandler is never called — so the page shortcut wins over the child's
// own key handling.
func TestURWIDColumnsInputCaptureConsumedSkipsChild(t *testing.T) {
	t.Parallel()

	app := newTestApp()

	childHandlerRan := false
	child := newFlagInputPrimitive(func() { childHandlerRan = true })
	cols := newURWIDColumns(0, child)
	app.SetFocus(cols)

	cols.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		return nil // consume everything
	})

	handler := cols.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyCtrlL, 0, tcell.ModNone),
		func(p tview.Primitive) { app.SetFocus(p) })

	if childHandlerRan {
		t.Error("focused child InputHandler ran after input capture consumed the event; the capture must short-circuit the inner handler")
	}
}

// flagInputPrimitive is a minimal tview.Primitive wrapping a Box, used to
// observe whether its InputHandler is invoked by a parent urwidColumns.
type flagInputPrimitive struct {
	*tview.Box
	onInput func()
}

func newFlagInputPrimitive(onInput func()) *flagInputPrimitive {
	return &flagInputPrimitive{Box: tview.NewBox(), onInput: onInput}
}

func (f *flagInputPrimitive) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return func(ev *tcell.EventKey, setFocus func(tview.Primitive)) {
		if f.onInput != nil {
			f.onInput()
		}
	}
}

func (f *flagInputPrimitive) HasFocus() bool { return f.Box.HasFocus() }
