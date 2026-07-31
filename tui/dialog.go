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
	for i := 0; i < w; i++ {
		set(x+i, y-1, '─', style)
	}
	set(x+w, y-1, '┐', style)

	// Bottom border
	set(x-1, y+h, '└', style)
	for i := 0; i < w; i++ {
		set(x+i, y+h, '─', style)
	}
	set(x+w, y+h, '┘', style)

	// Side borders
	for i := 0; i < h; i++ {
		set(x-1, y+i, '│', style)
		set(x+w, y+i, '│', style)
	}

	// Title
	if d.title != "" {
		titleStyle := style.Foreground(tcell.NewHexColor(0xdddddd))
		titleRunes := []rune(d.title)
		titleX := x + (w-len(titleRunes))/2
		for i, r := range titleRunes {
			set(titleX+i, y-1, r, titleStyle)
		}
	}

	// Draw content
	if d.content != nil {
		d.content.SetRect(x, y, w, h)
		d.content.Draw(screen)
	}
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

// ShowDialog creates and shows a centered dialog overlay on the application.
// The dialog is pushed onto the modal dialog stack (see DialogManager), so the
// underlying screen is preserved and focus is restored on dismiss. Esc on the
// dialog closes it (not the app); DismissTop closes it programmatically.
func (dm *DialogManager) ShowDialog(title string, content tview.Primitive, width, height int, onDismiss func()) {
	dm.showOverlay(dm.app, title, content, width, height, onDismiss)
}

// ShowConfirmDialog shows a Yes/No confirmation dialog.
func (dm *DialogManager) ShowConfirmDialog(message string, onYes, onNo func()) {
	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Yes").SetSelectedFunc(func() {
			dm.DismissTop()
			if onYes != nil {
				onYes()
			}
		}), 0, 1, true).
		AddItem(tview.NewButton("No").SetSelectedFunc(func() {
			dm.DismissTop()
			if onNo != nil {
				onNo()
			}
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetText(message), 3, 0, false).
		AddItem(buttons, 1, 0, true)

	dm.ShowDialog("Confirm", layout, 40, 6, nil)
}

// ShowInputDialog shows a text input dialog with Save/Cancel buttons.
func (dm *DialogManager) ShowInputDialog(title, label, defaultValue string, onSubmit func(string), onCancel func()) {
	input := tview.NewInputField()
	input.SetLabel(label)
	input.SetText(defaultValue)
	input.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	input.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Save").SetSelectedFunc(func() {
			value := input.GetText()
			dm.DismissTop()
			if onSubmit != nil {
				onSubmit(value)
			}
		}), 0, 1, true).
		AddItem(tview.NewButton("Cancel").SetSelectedFunc(func() {
			dm.DismissTop()
			if onCancel != nil {
				onCancel()
			}
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true).
		AddItem(buttons, 1, 0, false)

	dm.ShowDialog(title, layout, 50, 5, nil)
}

// ShowRadioDialog shows a radio button selection dialog.
func (dm *DialogManager) ShowRadioDialog(title, message string, options []string, onSelect func(int)) {
	list := tview.NewList()
	list.SetHighlightFullLine(true)
	list.SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

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
