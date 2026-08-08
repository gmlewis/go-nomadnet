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

// tabBarWidget lays out two TabButtons side by side with a one-column divider,
// matching Python's tab_bar (Conversations.py:392-398): a urwid Columns of two
// weight-1 TabButtons with dividechars=1. urwid distributes the leftover
// columns to the FIRST column (left-to-right), so at an odd inner width the
// left button is one cell wider than the right. tview's Flex hands the leftover
// to the last proportional item instead, so a plain Flex swaps the widths;
// this primitive reproduces the urwid distribution exactly.
type tabBarWidget struct {
	*tview.Box
	left, right *UrwidButton
	divider     int // columns between the two buttons (urwid dividechars)
	focusRight  bool
}

// newTabBarWidget lays out left and right TabButtons with a one-column divider.
func newTabBarWidget(left, right *UrwidButton) *tabBarWidget {
	return &tabBarWidget{Box: tview.NewBox(), left: left, right: right, divider: 1}
}

// SetRect divides the width urwid-style: the divider takes one column, the
// remainder is split with the extra column going to the left button (ceil).
func (t *tabBarWidget) SetRect(x, y, w, h int) {
	t.Box.SetRect(x, y, w, h)
	if w < 1 || h < 1 {
		t.left.SetRect(x, y, 0, 0)
		t.right.SetRect(x, y, 0, 0)
		return
	}
	remaining := max(w-t.divider, 0)
	leftW := (remaining + 1) / 2 // ceil → extra column to the left button
	rightW := remaining - leftW
	t.left.SetRect(x, y, leftW, h)
	t.right.SetRect(x+leftW+t.divider, y, rightW, h)
}

// Draw draws both buttons; the divider column is left as the box background.
func (t *tabBarWidget) Draw(screen tcell.Screen) {
	t.Box.DrawForSubclass(screen, t)
	t.left.Draw(screen)
	t.right.Draw(screen)
}

// Focus delegates to the currently-focused button (left by default).
func (t *tabBarWidget) Focus(delegate func(p tview.Primitive)) {
	if t.focusRight {
		t.right.Focus(delegate)
		return
	}
	t.left.Focus(delegate)
}

// HasFocus reports whether either button has focus.
func (t *tabBarWidget) HasFocus() bool {
	return t.left.HasFocus() || t.right.HasFocus()
}

// InputHandler delegates to the focused button.
func (t *tabBarWidget) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return t.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		var handler func(event *tcell.EventKey, setFocus func(tview.Primitive))
		if t.focusRight {
			handler = t.right.InputHandler()
		} else {
			handler = t.left.InputHandler()
		}
		if handler != nil {
			handler(event, setFocus)
		}
	})
}

// MouseHandler routes a click to whichever button contains the position.
func (t *tabBarWidget) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return t.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if action != tview.MouseLeftClick {
			return false, nil
		}
		mx, my := event.Position()
		if t.left.InRect(mx, my) {
			setFocus(t.left)
			if h := t.left.InputHandler(); h != nil {
				h(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), setFocus)
			}
			return true, nil
		}
		if t.right.InRect(mx, my) {
			setFocus(t.right)
			if h := t.right.InputHandler(); h != nil {
				h(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), setFocus)
			}
			return true, nil
		}
		return false, nil
	})
}
