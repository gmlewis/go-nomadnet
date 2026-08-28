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
	widthPct     int             // >0 = relative N% of slot width; 0 = use fixedWidth
	fixedWidth   int             // used when widthPct == 0 (urwid Overlay width=N)
	dialogHeight int             // PACK natural height of the dialog
	heightPct    int             // >0 = relative N% of slot height; 0 = use dialogHeight
	minWidth     int             // >0 = urwid Overlay min_width floor on the relative width
	insetLeft    int             // urwid Overlay left inset (default 2)
	insetRight   int             // urwid Overlay right inset (default 2)
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
		insetLeft:    2,
		insetRight:   2,
	}
}

// NewSlotOverlayFixed builds an overlay placing dialog centered over bottom with
// a fixed width (urwid Overlay width=N, align=CENTER, left=2/right=2 — capped to
// the slot width minus left/right) and the dialog's natural (PACK) height. Used
// by Conversations dialogs whose Python Overlay uses a fixed width (34/45/60).
func NewSlotOverlayFixed(bottom tview.Primitive, dialog *DialogLineBox, fixedWidth, dialogHeight int) *SlotOverlay {
	dialog.SetBorderInside(true)
	return &SlotOverlay{
		Box:          tview.NewBox(),
		bottom:       bottom,
		dialog:       dialog,
		fixedWidth:   fixedWidth,
		dialogHeight: dialogHeight,
		insetLeft:    2,
		insetRight:   2,
	}
}

// Dialog returns the overlaid DialogLineBox.
func (o *SlotOverlay) Dialog() *DialogLineBox { return o.dialog }

// SetRect lays out the bottom at the full slot rect and the dialog centered
// within it (valign MIDDLE, align CENTER), sized to widthPct% of the slot width
// (or fixedWidth when widthPct == 0), inset at least left/right=2, at its
// natural PACK height.
//
// The width and horizontal positioning replicate urwid's
// calculate_left_right_padding (widget.py): for RELATIVE widths the percentage
// is applied to (maxcol - left - right) with round-half-up rounding, then the
// remaining padding is distributed for CENTER alignment via int_scale. This
// matches Python's urwid.Overlay exactly, including the off-by-one rounding
// that a naive w*pct/100 truncation gets wrong.
func (o *SlotOverlay) SetRect(x, y, w, h int) {
	o.Box.SetRect(x, y, w, h)
	if o.bottom != nil {
		o.bottom.SetRect(x, y, w, h)
	}

	// urwid Overlay left/right insets (default 2; the QR overlay uses 0).
	left, right := o.insetLeft, o.insetRight

	dw := 0
	if o.widthPct > 0 {
		// urwid: maxwidth = max(maxcol - left - right, 0);
		//        width = int(maxwidth * amount / 100 + 0.5)
		maxwidth := max(w-left-right, 0)
		dw = int(float64(maxwidth)*float64(o.widthPct)/100 + 0.5)
	} else {
		dw = o.fixedWidth
	}
	dw = max(dw, 0)
	if o.minWidth > 0 {
		// urwid Overlay min_width: a relative width is floored at min_width
		// (Conversations.py:680 show_qr_dialog overlay min_width=44).
		dw = max(dw, min(o.minWidth, max(w-left-right, 0)))
	}

	// urwid CENTER alignment: distribute the remaining padding.
	// padding = maxcol - width - left - right
	// right += int_scale(100 - align, 101, padding + 1)  [align=50 for CENTER]
	// left = maxcol - width - right
	padding := w - dw - left - right
	rightPad := right + intScale(50, 101, padding+1)
	leftPad := max(w-dw-rightPad, 0)
	dx := x + leftPad

	dh := o.dialogHeight
	if o.heightPct > 0 {
		dh = h * o.heightPct / 100
	}
	dh = min(dh, h)
	dy := y + (h-dh)/2 // valign MIDDLE
	dy = max(dy, y)
	if o.dialog != nil {
		o.dialog.SetRect(dx, dy, dw, dh)
	}
}

// intScale replicates urwid's int_scale(n, rn, v) = (n*v + rn/2) / rn,
// used by calculate_left_right_padding to distribute padding for alignment.
func intScale(n, rn, v int) int {
	return (n*v + rn/2) / rn
}

// SetHeightPct makes the dialog height a percentage of the slot height (urwid
// Overlay height=("relative", N)), used by the Attach File browser (80%).
func (o *SlotOverlay) SetHeightPct(pct int) *SlotOverlay { o.heightPct = pct; return o }

// SetMinWidth floors the relative dialog width (urwid Overlay min_width),
// used by the My LXMF QR overlay (44, Conversations.py:680).
func (o *SlotOverlay) SetMinWidth(n int) *SlotOverlay { o.minWidth = n; return o }

// SetInsets overrides the default left/right=2 insets (urwid Overlay left/right
// args). The My LXMF QR overlay uses Python's default 0
// (Conversations.py:678-682).
func (o *SlotOverlay) SetInsets(left, right int) *SlotOverlay {
	o.insetLeft, o.insetRight = left, right
	return o
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
