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

// DialogLineBox wraps a tview.Primitive with a border that dismisses on Escape.
// Matches Python's DialogLineBox which extends urwid.LineBox with esc handling.
type DialogLineBox struct {
	*tview.Box
	content   tview.Primitive
	title     string
	onDismiss func()
}

// NewDialogLineBox creates a new dialog with border and escape handling.
func NewDialogLineBox(title string, content tview.Primitive, onDismiss func()) *DialogLineBox {
	d := &DialogLineBox{
		Box:       tview.NewBox(),
		content:   content,
		title:     title,
		onDismiss: onDismiss,
	}
	return d
}

// Draw draws the dialog with a border and title.
func (d *DialogLineBox) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)

	x, y, w, h := d.Box.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}

	// The border is drawn one cell outside the inner rect (x-1..x+w,
	// y-1..y+h). When the dialog is positioned at a screen edge or is larger
	// than the terminal (which happens during a resize to a small window),
	// those coordinates go negative or past the edge. tcell clips such writes
	// silently, but relying on that is fragile and produces stray glyphs, so
	// every border/title cell is clamped to the screen bounds here.
	sw, sh := screen.Size()
	set := func(px, py int, r rune, st tcell.Style) {
		if px < 0 || py < 0 || px >= sw || py >= sh {
			return
		}
		screen.SetContent(px, py, r, nil, st)
	}

	// Draw border using tview.Box primitives
	style := tcell.StyleDefault.
		Foreground(tcell.NewHexColor(0xdddddd)).
		Background(tcell.ColorDefault)

	// Top border
	set(x-1, y-1, '┌', style)
	for i := range w {
		set(x+i, y-1, '─', style)
	}
	set(x+w, y-1, '┐', style)

	// Bottom border
	set(x-1, y+h, '└', style)
	for i := range w {
		set(x+i, y+h, '─', style)
	}
	set(x+w, y+h, '┘', style)

	// Side borders
	for i := range h {
		set(x-1, y+i, '│', style)
		set(x+w, y+i, '│', style)
	}

	// Title — urwid LineBox.format_title wraps the text in a leading and
	// trailing space (" title ") and centers it (title_align=CENTER default,
	// line_box.py:189) over the top border, with the ─ tline filling both
	// sides (left=floor, right=ceil, the urwid Columns leftover-to-last). The
	// top-border ─ loop above already drew the full line; writing the spaced
	// title segment centered here overwrites just the title portion, leaving
	// the ─ fill on either side.
	if d.title != "" {
		titleStyle := style.Foreground(tcell.NewHexColor(0xdddddd))
		seg := []rune(" " + d.title + " ")
		titleX := x + (w-len(seg))/2 // floor division → left ─ = floor, right = ceil
		for i, r := range seg {
			set(titleX+i, y-1, r, titleStyle)
		}
	}

	// Draw content
	if d.content != nil {
		d.content.SetRect(x, y, w, h)
		d.content.Draw(screen)
	}
}

// Focus delegates to the dialog's content so the content's first focusable
// widget receives focus when the application focuses the dialog
// (tview.Application.SetFocus calls Focus with a SetFocus delegate). Without
// this override the embedded *tview.Box.Focus would only flag the dialog
// itself and no inner field would ever gain focus, leaving every dialog
// keyboard-uneditable (only mouse clicks could focus a field).
func (d *DialogLineBox) Focus(delegate func(p tview.Primitive)) {
	if d.content != nil {
		d.content.Focus(delegate)
		return
	}
	d.Box.Focus(delegate)
}

// HasFocus reports whether the dialog or any of its content has focus. tview
// dispatches key events to the root only when root.HasFocus() is true, and
// Flex/Pages.HasFocus recurses through their items; without this override a
// focused inner field (whose intermediate containers do not set their own
// hasFocus flag) would leave the whole focus chain reporting false, so no key
// events would reach the field.
func (d *DialogLineBox) HasFocus() bool {
	if d.content != nil && d.content.HasFocus() {
		return true
	}
	return d.Box.HasFocus()
}

// InputHandler handles key events, dismissing on Escape.
func (d *DialogLineBox) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return d.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if event.Key() == tcell.KeyEscape {
			if d.onDismiss != nil {
				d.onDismiss()
			}
			return
		}
		if d.content != nil {
			if handler := d.content.(interface {
				InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive))
			}); handler != nil {
				handler.InputHandler()(event, setFocus)
			}
		}
	})
}

// MouseHandler forwards mouse events to the dialog's content so clicks on a
// dialog's buttons/fields reach them. Without this override, DialogLineBox
// inherits *tview.Box.MouseHandler, which does NOT forward to content (Box has
// no notion of children) — so every dialog button (OK / Yes / No / Save /
// Cancel / etc.) was keyboard-only: Enter worked (InputHandler forwards to
// content) but a mouse click was silently dropped, never reaching the button's
// selected callback. This is the same delegate-to-content pattern used by
// InputHandler and Focus above. The content's rect is set to the dialog's
// inner rect in Draw, so the content's own InRect check (Flex/Button)
// correctly gates which child the click hits.
func (d *DialogLineBox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return d.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !d.InRect(event.Position()) {
			return false, nil
		}
		if d.content != nil {
			if handler := d.content.MouseHandler(); handler != nil {
				return handler(action, event, setFocus)
			}
		}
		return false, nil
	})
}

// ShowDialog creates and shows a centered dialog overlay on the application.
// The dialog is pushed onto the modal dialog stack (see DialogManager), so the
// underlying screen is preserved and focus is restored on dismiss. Esc on the
// dialog closes it (not the app); DismissTop closes it programmatically.
func (dm *DialogManager) ShowDialog(title string, content tview.Primitive, width, height int, onDismiss func()) {
	dm.showOverlay(dm.app, title, content, width, height, onDismiss)
}

// CreateButtonRow creates a horizontal row of buttons with Left/Right arrow key
// focus navigation and solid green active styling when focused.
func CreateButtonRow(buttons ...*tview.Button) *tview.Flex {
	flex := tview.NewFlex().SetDirection(tview.FlexColumn)
	for i, btn := range buttons {
		idx := i
		btn.SetBackgroundColor(tcell.ColorBlack)
		btn.SetLabelColor(tcell.ColorWhite)
		btn.SetBackgroundColorActivated(tcell.ColorGreen)
		btn.SetLabelColorActivated(tcell.ColorBlack)

		btn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			switch event.Key() {
			case tcell.KeyLeft, tcell.KeyBacktab:
				if idx > 0 {
					buttons[idx-1].Focus(func(p tview.Primitive) {})
					return nil
				}
			case tcell.KeyRight, tcell.KeyTab:
				if idx < len(buttons)-1 {
					buttons[idx+1].Focus(func(p tview.Primitive) {})
					return nil
				}
			}
			return event
		})

		flex.AddItem(btn, 0, 1, i == 0)
	}

	return flex
}

// ShowConfirmDialog shows a Yes/No confirmation dialog.
func (dm *DialogManager) ShowConfirmDialog(message string, onYes, onNo func()) {
	yesBtn := tview.NewButton("Yes").SetSelectedFunc(func() {
		dm.DismissTop()
		if onYes != nil {
			onYes()
		}
	})
	noBtn := tview.NewButton("No").SetSelectedFunc(func() {
		dm.DismissTop()
		if onNo != nil {
			onNo()
		}
	})
	buttons := CreateButtonRow(yesBtn, noBtn)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetText(message), 3, 0, false).
		AddItem(buttons, 1, 0, true)

	dm.ShowDialog("Confirm", layout, 40, 6, nil)
}

// ShowInputDialog shows a text input dialog with Save/Cancel buttons. It is
// the common-case wrapper around ShowInputDialogBtns for dialogs whose confirm
// button is "Save" (New Hub, Join Room, Add Interface, …). URL-entry dialogs
// should call ShowInputDialogBtns directly with "Go"/"Cancel" to match Python's
// Browser.url_dialog (Browser.py:1154-1162).
func (dm *DialogManager) ShowInputDialog(title, label, defaultValue string, onSubmit func(string), onCancel func()) {
	dm.ShowInputDialogBtns(title, label, defaultValue, "Save", "Cancel", onSubmit, onCancel)
}

// ShowInputDialogBtns shows a text input dialog with caller-supplied button
// labels. It mirrors Python's url_dialog/Pile key model: Enter on the input
// field confirms (Python's UrlEdit.keypress "enter" → confirmed, Browser.py
// :1843-1849), Tab/Down moves focus input → confirm → cancel (urwid Pile
// focus traversal), and Escape cancels. The confirm button is the first button
// so an Enter typed while the field is focused submits immediately, matching
// the Python UX where the user never has to tab to a button.
func (dm *DialogManager) ShowInputDialogBtns(title, label, defaultValue, confirmLabel, cancelLabel string, onSubmit func(string), onCancel func()) {
	input := tview.NewInputField()
	input.SetLabel(label)
	input.SetText(defaultValue)
	input.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	input.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	triggerConfirm := func() {
		value := input.GetText()
		dm.DismissTop()
		if onSubmit != nil {
			onSubmit(value)
		}
	}
	triggerCancel := func() {
		dm.DismissTop()
		if onCancel != nil {
			onCancel()
		}
	}

	confirmBtn := tview.NewButton(confirmLabel).SetSelectedFunc(triggerConfirm)
	cancelBtn := tview.NewButton(cancelLabel).SetSelectedFunc(triggerCancel)
	buttons := CreateButtonRow(confirmBtn, cancelBtn)

	// Enter on the input submits (Python UrlEdit.keypress "enter" → confirmed).
	// Tab/Escape are intercepted by wireDialogNav below (for traversal/dismiss)
	// before the InputField's finish() runs, so done only fires on Enter here.
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			triggerConfirm()
		}
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true).
		AddItem(buttons, 1, 0, false)

	dm.ShowDialog(title, layout, 50, 5, nil)
	// Tab/Down/Up/Esc traversal across input → confirm → cancel (urwid Pile
	// focus model). wireDialogNav re-focuses the first item (the input).
	wireDialogNav(dm.app, triggerCancel, []tview.Primitive{input, confirmBtn, cancelBtn})
}

// ShowRadioDialog shows a radio button selection dialog.
func (dm *DialogManager) ShowRadioDialog(title, message string, options []string, onSelect func(int)) {
	list := tview.NewList()
	list.SetHighlightFullLine(true)
	// list_focus is #111/#aaa in both dark and light themes; DialogManager has
	// no theme reference, so ThemeDark yields the correct focus colors.
	ApplyListFocusStyle(list, ThemeDark)

	for i, opt := range options {
		idx := i
		list.AddItem(opt, "", 0, func() {
			if onSelect != nil {
				onSelect(idx)
			}
		})
	}

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetText(message), 1, 0, false).
		AddItem(list, 0, 1, true)

	dm.ShowDialog(title, layout, 40, 10, nil)
}

// ShowStatusDialog shows a centered status/notice message with an OK button
// that dismisses it (click OK or press Enter/Space when focused). This matches
// Python's status dialogs, which are all a DialogLineBox wrapping a Pile of a
// centered message Text + an "OK" button — e.g. LocalPeer.save_query's "Saved"
// dialog (Network.py:1282-1287) and announce_query's "Announce Sent"
// (Network.py:1305-1309), and Interfaces.show_message / show_restart_required
// / show_error_message (Interfaces.py:1969-1977, 2589-2603, 2619-2628).
//
// Esc also dismisses (DialogLineBox default, plus MainDisplay routes Esc to
// DismissTop when any dialog is open). The OK button matters because a bare
// centered-text modal gives NO visible dismiss affordance: the user can press
// Esc, but there is nothing on screen to signal that, so the dialog appears
// stuck — the Local Peer "Saved" dialog was reported undismissable for exactly
// this reason. The OK button restores the Python UX and gives a guaranteed
// click/Enter dismiss path that does not depend on tview's focus routing.
func (dm *DialogManager) ShowStatusDialog(title, message string, width, height int) {
	text := tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetTextAlign(tview.AlignCenter).
		SetText(message)
	okBtn := tview.NewButton("OK").SetSelectedFunc(func() { dm.DismissTop() })
	buttons := CreateButtonRow(okBtn)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(text, 0, 1, false).
		AddItem(buttons, 1, 0, true)
	dm.ShowDialog(title, layout, width, height, nil)
}

// ConfirmationDialog is a reusable confirmation dialog.
type ConfirmationDialog struct {
	dialogs *DialogManager
	message string
	onYes   func()
	onNo    func()
}

// NewConfirmationDialog creates a new confirmation dialog.
func NewConfirmationDialog(dm *DialogManager, message string, onYes, onNo func()) *ConfirmationDialog {
	return &ConfirmationDialog{
		dialogs: dm,
		message: message,
		onYes:   onYes,
		onNo:    onNo,
	}
}

// Show displays the confirmation dialog.
func (cd *ConfirmationDialog) Show() {
	cd.dialogs.ShowConfirmDialog(cd.message, cd.onYes, cd.onNo)
}
