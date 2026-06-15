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

	// Draw border using tview.Box primitives
	style := tcell.StyleDefault.
		Foreground(tcell.NewHexColor(0xdddddd)).
		Background(tcell.ColorDefault)

	// Top border
	screen.SetContent(x-1, y-1, '┌', nil, style)
	for i := 0; i < w; i++ {
		screen.SetContent(x+i, y-1, '─', nil, style)
	}
	screen.SetContent(x+w, y-1, '┐', nil, style)

	// Bottom border
	screen.SetContent(x-1, y+h, '└', nil, style)
	for i := 0; i < w; i++ {
		screen.SetContent(x+i, y+h, '─', nil, style)
	}
	screen.SetContent(x+w, y+h, '┘', nil, style)

	// Side borders
	for i := 0; i < h; i++ {
		screen.SetContent(x-1, y+i, '│', nil, style)
		screen.SetContent(x+w, y+i, '│', nil, style)
	}

	// Title
	if d.title != "" {
		titleStyle := style.Foreground(tcell.NewHexColor(0xdddddd))
		titleRunes := []rune(d.title)
		titleX := x + (w-len(titleRunes))/2
		for i, r := range titleRunes {
			screen.SetContent(titleX+i, y-1, r, nil, titleStyle)
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
func ShowDialog(app *tview.Application, title string, content tview.Primitive, width, height int, onDismiss func()) {
	dialog := NewDialogLineBox(title, content, onDismiss)
	app.SetRoot(dialog, true)
}

// ShowConfirmDialog shows a Yes/No confirmation dialog.
func ShowConfirmDialog(app *tview.Application, message string, onYes, onNo func()) {
	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Yes").SetSelectedFunc(func() {
			if onYes != nil {
				onYes()
			}
		}), 0, 1, true).
		AddItem(tview.NewButton("No").SetSelectedFunc(func() {
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

	ShowDialog(app, "Confirm", layout, 40, 6, nil)
}

// ShowInputDialog shows a text input dialog with Save/Cancel buttons.
func ShowInputDialog(app *tview.Application, title, label, defaultValue string, onSubmit func(string), onCancel func()) {
	input := tview.NewInputField()
	input.SetLabel(label)
	input.SetText(defaultValue)
	input.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	input.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Save").SetSelectedFunc(func() {
			if onSubmit != nil {
				onSubmit(input.GetText())
			}
		}), 0, 1, true).
		AddItem(tview.NewButton("Cancel").SetSelectedFunc(func() {
			if onCancel != nil {
				onCancel()
			}
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true).
		AddItem(buttons, 1, 0, false)

	ShowDialog(app, title, layout, 50, 5, nil)
}

// ShowRadioDialog shows a radio button selection dialog.
func ShowRadioDialog(app *tview.Application, title, message string, options []string, onSelect func(int)) {
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

	ShowDialog(app, title, layout, 40, 10, nil)
}

// ConfirmationDialog is a reusable confirmation dialog.
type ConfirmationDialog struct {
	app     *tview.Application
	message string
	onYes   func()
	onNo    func()
}

// NewConfirmationDialog creates a new confirmation dialog.
func NewConfirmationDialog(app *tview.Application, message string, onYes, onNo func()) *ConfirmationDialog {
	return &ConfirmationDialog{
		app:     app,
		message: message,
		onYes:   onYes,
		onNo:    onNo,
	}
}

// Show displays the confirmation dialog.
func (cd *ConfirmationDialog) Show() {
	ShowConfirmDialog(cd.app, cd.message, cd.onYes, cd.onNo)
}
