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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// captureSetter is the subset of tview primitives that expose an input capture
// (SetInputCapture/GetInputCapture). All dialog widgets embed *tview.Box
// (directly or via *tview.InputField), so they all satisfy it. The method
// signatures match *tview.Box's promoted methods; *tview.InputField overrides
// SetInputCapture with a different return type, so it is handled via a type
// switch in setCapture/getCapture rather than this interface.
type captureSetter interface {
	GetInputCapture() func(*tcell.EventKey) *tcell.EventKey
}

// focuser is the subset of tview's application/root primitives that can move
// keyboard focus. Both *tui.App (which embeds *tview.Application) and a raw
// *tview.Application satisfy it, so wireDialogNav can be called from the
// DialogManager (which only holds the raw *tview.Application) as well as from
// widget displays (which hold a *tui.App).
type focuser interface {
	SetFocus(p tview.Primitive) *tview.Application
}

// Focuser is the exported form of focuser, for wiring-layer callers that build
// slot-placed dialogs (e.g. the URL dialog) and need the same Pile-style
// Tab/Down/Up/Esc focus traversal across the dialog's items.
type Focuser = focuser

// WireDialogNav is the exported form of wireDialogNav: it wires urwid-Pile-style
// keyboard focus traversal across the given focusable dialog items (Tab/Down →
// next, BackTab/Up → previous, Escape → dismiss), focusing the first item. Used
// by slot-placed dialogs that are not owned by the DialogManager.
func WireDialogNav(app Focuser, dismiss func(), items []tview.Primitive) {
	wireDialogNav(app, dismiss, items)
}

// setItemCapture installs a new input capture on a dialog widget, dispatching
// by concrete type because tview's SetInputCapture returns a different
// (chained) type on *tview.InputField versus *tview.Box, so no single interface
// covers both.
func setItemCapture(p tview.Primitive, cap func(*tcell.EventKey) *tcell.EventKey) {
	switch v := p.(type) {
	case *ReadlineEdit:
		v.SetInputCapture(cap)
	case *RadioButton:
		v.SetInputCapture(cap)
	case *UrwidCheckBox:
		v.SetInputCapture(cap)
	case *UrwidButton:
		v.SetInputCapture(cap)
	case *tview.InputField:
		v.SetInputCapture(cap)
	case *tview.List:
		v.SetInputCapture(cap)
	case *tview.TextView:
		v.SetInputCapture(cap)
	case *tview.Checkbox:
		v.SetInputCapture(cap)
	}
}

// getItemCapture returns the widget's current input capture (or nil).
func getItemCapture(p tview.Primitive) func(*tcell.EventKey) *tcell.EventKey {
	if cs, ok := p.(captureSetter); ok {
		return cs.GetInputCapture()
	}
	return nil
}

// wireDialogNav wires urwid-Pile-style keyboard focus traversal across the
// given focusable dialog items: Tab/Down moves focus to the next item,
// BackTab/Up to the previous, and Escape calls dismiss. All other
// keys pass through to each widget's own handler (and, for widgets that already
// install an input capture such as ReadlineEdit's emacs kill/yank, that
// existing capture is chained so its behavior is preserved). The first item
// receives the initial focus.
//
// This mirrors urwid's Pile focus model (urwid/widget/pile.py: Pile.keypress
// moves focus on Tab/Up/Down when the focused widget returns the key unhandled)
// without relying on tview's Flex, whose InputHandler only forwards to an
// already-focused child and provides no Tab traversal. Traversal does NOT
// wrap: like urwid's Pile, pressing Up on the first item or Down on the last
// leaves focus where it is (urwid returns the key unhandled; the enclosing
// containers ignore up/down, so the visible effect is "stay put").
//
// Left/Right are deliberately NOT handled here: urwid's Pile does not consume
// them, and inside a horizontal row of widgets (e.g. a button Columns) the
// row's own Columns model moves focus between siblings.
func wireDialogNav(app focuser, dismiss func(), items []tview.Primitive) {
	if len(items) == 0 {
		return
	}
	for i := range items {
		i := i
		orig := getItemCapture(items[i])
		setItemCapture(items[i], func(ev *tcell.EventKey) *tcell.EventKey {
			switch ev.Key() {
			case tcell.KeyTab, tcell.KeyDown:
				if i+1 < len(items) {
					app.SetFocus(items[i+1])
				}
				return nil
			case tcell.KeyBacktab, tcell.KeyUp:
				if i > 0 {
					app.SetFocus(items[i-1])
				}
				return nil
			case tcell.KeyEscape:
				if dismiss != nil {
					dismiss()
				}
				return nil
			}
			if orig != nil {
				return orig(ev)
			}
			return ev
		})
	}
	app.SetFocus(items[0])
}
