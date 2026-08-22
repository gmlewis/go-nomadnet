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

// SlotOverlay replicates Python's urwid.Overlay(dialog, bottom, align=CENTER,
// width=RELATIVE_100|("relative", N), valign=MIDDLE, height=PACK, left=2,
// right=2) as used by nomadnet's in-slot dialogs (Network.py:916/948,
// Browser.py:1169/1184): it draws the bottom (the slot's current content) and
// then the dialog centered on top, so the slot content shows through around the
// dialog — NOT a screen-centered modal. widthPct is the dialog width as a
// percentage of the slot width (100 = urwid.RELATIVE_100); the dialog is inset
// at least left/right=2 cells. dialogHeight is the dialog's PACK (natural)
// height (content rows + 2 border rows).
//
// This is what makes gonomadnet's Network/Browser dialogs render in the correct
// slot at the correct width instead of as a screen-centered box of a fixed size.
type SlotOverlay struct {
	*tview.Box
	bottom       tview.Primitive // slot content drawn underneath (show-through)
	dialog       *DialogLineBox  // centered dialog drawn on top
	widthPct     int             // 100 = RELATIVE_100; else relative N%
	dialogHeight int             // PACK natural height of the dialog
}

// NewSlotOverlay builds an overlay placing dialog centered over bottom with the
// given width percentage (100 = full slot width minus left/right=2) and the
// dialog's natural (PACK) height. Esc on the dialog calls its onDismiss (the
// caller restores the slot). The dialog uses border-inside (urwid LineBox model)
// so its rect already includes the border and it fits the slot without overlap.
func NewSlotOverlay(bottom tview.Primitive, dialog *DialogLineBox, widthPct, dialogHeight int) *SlotOverlay {
	dialog.SetBorderInside(true)
	return &SlotOverlay{
		Box:          tview.NewBox(),
		bottom:       bottom,
		dialog:       dialog,
		widthPct:     widthPct,
		dialogHeight: dialogHeight,
	}
}

// Dialog returns the overlaid DialogLineBox.
func (o *SlotOverlay) Dialog() *DialogLineBox { return o.dialog }

// SetRect lays out the bottom at the full slot rect and the dialog centered
// within it (valign MIDDLE, align CENTER), sized to widthPct% of the slot width
// (inset at least left/right=2), at its natural PACK height.
func (o *SlotOverlay) SetRect(x, y, w, h int) {
	o.Box.SetRect(x, y, w, h)
	if o.bottom != nil {
		o.bottom.SetRect(x, y, w, h)
	}
	dw := w * o.widthPct / 100
	dw = min(dw, w-4) // urwid caps width at maxcol - left - right
	dw = max(dw, 0)
	dx := x + (w-dw)/2
	dx = max(dx, x+2) // at least left=2
	dh := o.dialogHeight
	dh = min(dh, h)
	dy := y + (h-dh)/2 // valign MIDDLE
	dy = max(dy, y)
	if o.dialog != nil {
		o.dialog.SetRect(dx, dy, dw, dh)
	}
}

// Draw draws the bottom (show-through) then the dialog on top.
func (o *SlotOverlay) Draw(screen tcell.Screen) {
	o.Box.DrawForSubclass(screen, o)
	if o.bottom != nil {
		o.bottom.Draw(screen)
	}
	if o.dialog != nil {
		o.dialog.Draw(screen)
	}
}

// Focus delegates to the dialog (the dialog holds focus while the overlay is
// shown; the bottom is non-interactive).
func (o *SlotOverlay) Focus(delegate func(p tview.Primitive)) {
	if o.dialog != nil {
		o.dialog.Focus(delegate)
		return
	}
	o.Box.Focus(delegate)
}

// Blur clears focus on the dialog so it stops showing a caret/highlight.
func (o *SlotOverlay) Blur() {
	if o.dialog != nil {
		o.dialog.Blur()
	}
	o.Box.Blur()
}

func (o *SlotOverlay) HasFocus() bool {
	if o.dialog != nil {
		return o.dialog.HasFocus()
	}
	return o.Box.HasFocus()
}

// InputHandler forwards keys to the dialog; the DialogLineBox dismisses on Esc
// (calling its onDismiss, which the caller uses to restore the slot).
func (o *SlotOverlay) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return o.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if o.dialog != nil {
			if h := o.dialog.InputHandler(); h != nil {
				h(event, setFocus)
			}
		}
	})
}

// MouseHandler routes clicks to the dialog first (so buttons/fields respond),
// then to the bottom (so the show-through slot content remains non-interactive
// but a click outside the dialog is not silently swallowed).
func (o *SlotOverlay) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if o.dialog != nil {
			if h := o.dialog.MouseHandler(); h != nil {
				if consumed, capture := h(action, event, setFocus); consumed {
					return consumed, capture
				}
			}
		}
		return false, nil
	}
}
