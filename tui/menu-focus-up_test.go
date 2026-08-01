// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without the implied warranty of
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

// TestFocusUpAtTopToMenu asserts the Phase-1 focus-model gap is closed: when
// the body region is focused on a *tview.List sitting at item 0, an Up key
// moves focus to the menu bar (MainFrame.focus_position = "header"), matching
// Python's MainFrame where Up at the top of the body pile collapses focus to
// the header menu (Main.py MainFrame:80-86). tview.List clamps silently at the
// top (it does NOT fire SetDoneFunc on Up-at-top, only on Escape), so the
// MainDisplay dispatcher must own this transition.
//
// Guards:
//   - Up mid-list forwards (the list must still scroll up).
//   - Up on a non-list primitive (TextView) forwards.
//   - Up at top while a modal dialog is open forwards (the dispatcher must not
//     steal focus from an open dialog overlay).
func TestFocusUpAtTopToMenu(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)

	t.Run("top of list focuses menu", func(t *testing.T) {
		list := tview.NewList()
		list.AddItem("a", "", 0, nil)
		list.AddItem("b", "", 0, nil)
		list.SetCurrentItem(0)
		app.SetFocus(list)
		md.focusRegion = "body"

		got := md.handleInput(up)
		if got != nil {
			t.Errorf("handleInput(Up) = %v, want nil (consumed)", got)
		}
		if md.focusRegion != "menu" {
			t.Errorf("focusRegion = %q, want menu", md.focusRegion)
		}
		if app.GetFocus() != md.menuBar {
			t.Errorf("app focus = %v, want menuBar", app.GetFocus())
		}
	})

	t.Run("mid-list forwards", func(t *testing.T) {
		list := tview.NewList()
		list.AddItem("a", "", 0, nil)
		list.AddItem("b", "", 0, nil)
		list.SetCurrentItem(1)
		app.SetFocus(list)
		md.focusRegion = "body"

		got := md.handleInput(up)
		if got == nil {
			t.Error("handleInput(Up) consumed at mid-list; want forwarded")
		}
		if md.focusRegion != "body" {
			t.Errorf("focusRegion = %q, want body", md.focusRegion)
		}
	})

	t.Run("non-list forwards", func(t *testing.T) {
		tv := tview.NewTextView()
		app.SetFocus(tv)
		md.focusRegion = "body"

		got := md.handleInput(up)
		if got == nil {
			t.Error("handleInput(Up) on TextView consumed; want forwarded")
		}
		if md.focusRegion != "body" {
			t.Errorf("focusRegion = %q, want body", md.focusRegion)
		}
	})

	t.Run("dialog open does not steal focus", func(t *testing.T) {
		list := tview.NewList()
		list.AddItem("a", "", 0, nil)
		list.SetCurrentItem(0)
		app.SetFocus(list)
		md.focusRegion = "body"

		// Simulate an open modal overlay by pushing onto the dialog stack.
		md.app.Dialogs.stack = append(md.app.Dialogs.stack, &dialogEntry{})
		defer func() {
			md.app.Dialogs.stack = md.app.Dialogs.stack[:0]
		}()

		got := md.handleInput(up)
		if got == nil {
			t.Error("handleInput(Up) consumed while dialog open; want forwarded")
		}
		if md.focusRegion != "body" {
			t.Errorf("focusRegion = %q, want body (dialog keeps focus)", md.focusRegion)
		}
	})
}
