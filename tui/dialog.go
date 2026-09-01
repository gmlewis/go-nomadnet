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
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// urwidCenterText renders text centered in its rect using urwid's CENTER
// alignment convention: the leftover cell for an odd (width-len) goes on the
// LEFT (left = ceil((width-len)/2)), not the right as tview's AlignCenter does.
// nomadnet's urwid Text(align=CENTER) dialogs (e.g. "Saved", "Block peer?")
// use this convention, so matching it is required for byte-exact dialog parity.
// The text is rendered in the default style (urwid dialog Text is attr=None),
// one source line per row, clipped to the rect.
type urwidCenterText struct {
	*tview.Box
	text string
}

// NewUrwidCenterText builds a default-style centered text widget.
func NewUrwidCenterText(text string) *urwidCenterText {
	return &urwidCenterText{Box: tview.NewBox(), text: text}
}

// urwidLeftText renders text left-aligned in its rect, matching urwid's default
// LEFT alignment (urwid.Text with no align= kwarg). Used by nomadnet dialog
// Text widgets like the Save Node confirmation message (Browser.py:1205). The
// text is rendered in the default style, one source line per row, clipped to
// the rect.
type urwidLeftText struct {
	*tview.Box
	text string
}

// NewUrwidLeftText builds a default-style left-aligned text widget.
func NewUrwidLeftText(text string) *urwidLeftText {
	return &urwidLeftText{Box: tview.NewBox(), text: text}
}

// SetText updates the displayed text.
func (c *urwidLeftText) SetText(text string) { c.text = text }

// Draw renders the text left-aligned, WRAPPING each source line at the widget
// width with urwid's "space" algorithm (urwid.Text wraps to maxcol; see the
// urwidCenterText.Draw note).
func (c *urwidLeftText) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	x, y, w, h := c.GetRect()
	if w <= 0 || h <= 0 {
		return
	}
	style := tcell.StyleDefault
	lines := urwidSpaceWrap(c.text, w)
	for i, line := range lines {
		if i >= h {
			break
		}
		px := x
		for _, r := range line {
			if px >= x+w {
				break
			}
			screen.SetContent(px, y+i, r, nil, style)
			px += cellWidth(r)
		}
	}
}

// SetText updates the displayed text.
func (c *urwidCenterText) SetText(text string) { c.text = text }

// Draw renders the text centered per row, WRAPPING each source line at the
// widget width with urwid's "space" algorithm (urwid.Text wraps its content to
// maxcol and the Pile PACK height follows the wrapped row count — a no-wrap
// render chopped long dialog messages mid-word, e.g. the ingest error's
// "Check your inp"). Extra columns go on the LEFT for odd slack (urwid
// CENTER).
func (c *urwidCenterText) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	x, y, w, h := c.GetRect()
	if w <= 0 || h <= 0 {
		return
	}
	style := tcell.StyleDefault
	lines := urwidSpaceWrap(c.text, w)
	for i, line := range lines {
		if i >= h {
			break
		}
		lw := stringWidth(line)
		lw = min(lw, w)
		left := (w - lw + 1) / 2 // ceil → extra on the left (urwid CENTER)
		left = max(left, 0)
		px := x + left
		for _, r := range line {
			if px >= x+w {
				break
			}
			screen.SetContent(px, y+i, r, nil, style)
			px += cellWidth(r)
		}
	}
}

// DialogLineBox wraps a tview.Primitive with a border that dismisses on Escape.
// Matches Python's DialogLineBox which extends urwid.LineBox with esc handling.
type DialogLineBox struct {
	*tview.Box
	content   tview.Primitive
	title     string
	onDismiss func()
	// borderInside selects the border placement. False (default, used by the
	// DialogManager's screen-centered centerDialog): the border is drawn one
	// cell OUTSIDE the box rect so the surrounding transparent margins absorb
	// it (the box rect is the content area; visible = content+2). True (used by
	// slot-placed dialogs via SlotOverlay/ShowLocalPeerStatus): the border is
	// drawn INSIDE the box rect (rect = border + content, urwid LineBox model)
	// so the dialog occupies exactly its allocated rows/cols with no overflow
	// into the neighboring slot widget.
	borderInside bool
}

// SetBorderInside switches the border placement to inside-the-rect (urwid
// LineBox model), used by slot-placed dialogs whose rect already includes the
// border. See DialogLineBox.borderInside.
func (d *DialogLineBox) SetBorderInside(v bool) *DialogLineBox { d.borderInside = v; return d }

// GetTitle returns the dialog's title string (the LineBox top label).
func (d *DialogLineBox) GetTitle() string { return d.title }

// Content returns the dialog's content primitive (for tree walks in tests).
func (d *DialogLineBox) Content() tview.Primitive { return d.content }

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

// Draw draws the dialog with a border and title. The border placement is
// selected by borderInside: inside-the-rect (urwid LineBox model, rect = border
// + content) for slot-placed dialogs, or one-cell-outside-the-rect for the
// DialogManager's screen-centered centerDialog (whose transparent margins
// absorb the border; box rect = content, visible = content+2). The border and
// title use the default style (urwid LineBox attrs are None), and the title is
// centered with the leftover on the LEFT (urwid's convention).
func (d *DialogLineBox) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)

	var bx, by, bw, bh int // border rect (where ┌─┐│└─┘ + title go)
	var cx, cy, cw, ch int // content rect
	if d.borderInside {
		bx, by, bw, bh = d.Box.GetRect()
		cx, cy, cw, ch = bx+1, by+1, bw-2, bh-2
	} else {
		ix, iy, iw, ih := d.Box.GetInnerRect()
		bx, by, bw, bh = ix-1, iy-1, iw+2, ih+2
		cx, cy, cw, ch = ix, iy, iw, ih
	}
	if bw < 2 || bh < 2 {
		return
	}

	sw, sh := screen.Size()
	set := func(px, py int, r rune, st tcell.Style) {
		if px < 0 || py < 0 || px >= sw || py >= sh {
			return
		}
		screen.SetContent(px, py, r, nil, st)
	}

	// urwid LineBox border is the default style (attr=None).
	style := tcell.StyleDefault

	// Top border (row by): ┌─...─┐
	set(bx, by, '┌', style)
	for i := 1; i < bw-1; i++ {
		set(bx+i, by, '─', style)
	}
	set(bx+bw-1, by, '┐', style)

	// Bottom border (row by+bh-1): └─...─┘
	set(bx, by+bh-1, '└', style)
	for i := 1; i < bw-1; i++ {
		set(bx+i, by+bh-1, '─', style)
	}
	set(bx+bw-1, by+bh-1, '┘', style)

	// Side borders (rows by+1..by+bh-2): │ ... │
	for i := 1; i < bh-1; i++ {
		set(bx, by+i, '│', style)
		set(bx+bw-1, by+i, '│', style)
	}

	// Title — urwid LineBox.format_title wraps the text in a leading and trailing
	// space (" title ") and centers it over the top border with the leftover on
	// the LEFT (left = ceil). The ─ loop above drew the full line; the title
	// segment overwrites just the title portion. Default style (attr=None).
	if d.title != "" {
		seg := []rune(" " + d.title + " ")
		inner := bw - 2 // usable border cells between the corners
		if len(seg) <= inner {
			titleX := bx + 1 + (inner-len(seg)+1)/2 // ceil → leftover on left
			for i, r := range seg {
				set(titleX+i, by, r, style)
			}
		}
	}

	// Draw content in the content rect.
	if d.content != nil && cw > 0 && ch > 0 {
		d.content.SetRect(cx, cy, cw, ch)
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

// CreateUrwidButtonRow builds a flat urwid-style button row matching Python's
// dialog button Columns (Conversations.py:801-805, Network.py:905-910,
// Browser.py:1157-1161): a sequence of urwid.Button columns separated by blank
// spacer columns. Each button renders as a flat "< label >" in the DEFAULT
// style (urwid.Button applies no color), not tview's bordered/colored button —
// so the row looks identical to nomadnet's. Left/Right/Tab move focus between
// buttons (skipping the spacers) via the urwidColumns focus model, mirroring
// urwid Columns; Enter/Space activates the focused button.
//
// The button:spacer weight ratio is 0.45:0.10 (Python's urwid.WEIGHT 0.45 /
// 0.10), expressed as the integer ratio 9:2.
func CreateUrwidButtonRow(buttons ...*UrwidButton) *urwidColumns {
	const btnW, spacerW = 9, 2
	var children []tview.Primitive
	for i, btn := range buttons {
		if i > 0 {
			children = append(children, tview.NewBox()) // blank spacer column
		}
		children = append(children, btn)
	}
	row := newURWIDColumns(0, children...)
	for i := range children {
		w := btnW
		if _, isBox := children[i].(*tview.Box); isBox {
			w = spacerW
		}
		row.SetWeight(i, w)
	}
	return row
}

// CreateUrwidButtonRowRight builds a flat urwid-style row with a single button
// right-aligned via a leading blank spacer column (Python's
// `(WEIGHT 0.6 Text("")) + (WEIGHT 0.4 Button)` pattern, e.g. Conversations.py
// ingest-result / paper-message-saved/failed dialogs). The spacer:button weight
// ratio is 0.6:0.4 = 3:2.
func CreateUrwidButtonRowRight(btn *UrwidButton) *urwidColumns {
	row := newURWIDColumns(0, tview.NewBox(), btn)
	row.SetWeight(0, 3) // spacer 0.6
	row.SetWeight(1, 2) // button 0.4
	return row
}

// ShowConfirmDialog shows a Yes/No confirmation dialog matching Python's
// confirm dialogs (e.g. Conversations.py:797-810): a DialogLineBox titled
// "Confirm" wrapping a Pile of a centered message Text + a button Columns
// (Yes / Cancel), overlaid full-width (RELATIVE_100, left=2/right=2) at
// natural height. The message and buttons render in the default style, and the
// buttons are flat "< label >" urwid buttons, not tview's colored buttons.
func (dm *DialogManager) ShowConfirmDialog(message string, onYes, onNo func()) {
	yesBtn := NewUrwidButton("Yes").SetSelectedFunc(func() {
		dm.DismissTop()
		if onYes != nil {
			onYes()
		}
	})
	noBtn := NewUrwidButton("No").SetSelectedFunc(func() {
		dm.DismissTop()
		if onNo != nil {
			onNo()
		}
	})
	buttons := CreateUrwidButtonRow(yesBtn, noBtn)

	// The message row must hold the message's WRAPPED row count (urwid.Text
	// under urwid PACK wraps to the dialog width; the caller's strings include
	// arbitrary-length error text like "Could not open LXMF link: …"). Size at
	// the 46-column inner width of the narrowest real terminal (the parity
	// tooling floor) — wider terminals wrap the same or fewer rows, and the
	// extra room just pads the dialog. Never fewer than 3 rows, matching the
	// historical fixed sizing for the standard two-line questions.
	msgRows := max(3, len(urwidSpaceWrap(message, 46)))

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(message), msgRows, 0, false).
		AddItem(buttons, 1, 0, true)

	dm.ShowDialog("Confirm", layout, 0, msgRows+3, nil)
}

// ShowBlockedNodeConfirmDialog shows the Go-only "blocked node" connect
// warning modal: a DialogLineBox titled "Blocked node" wrapping a centered
// message + a button Columns (Cancel / Connect). This is a DELIBERATE
// gonomadnet enhancement with no Python nomadnet counterpart — Python offers
// no way to block a node at all, so do NOT remove this as a parity
// divergence. The user asked for a guard before connecting to a blocked
// destination whose SAFE default is Cancel: the Cancel button is the FIRST
// button (so it takes the initial focus) and Esc also dismisses through the
// cancel path, so a bare Enter never connects. Tab/Left/Right move focus
// between Cancel and Connect, whose Enter proceeds.
func (dm *DialogManager) ShowBlockedNodeConfirmDialog(message string, onConnect, onCancel func()) {
	cancelFn := func() {
		dm.DismissTop()
		if onCancel != nil {
			onCancel()
		}
	}
	connectFn := func() {
		dm.DismissTop()
		if onConnect != nil {
			onConnect()
		}
	}
	cancelBtn := NewUrwidButton("Cancel").SetSelectedFunc(cancelFn)
	connectBtn := NewUrwidButton("Connect").SetSelectedFunc(connectFn)
	// Cancel is intentionally FIRST in the row so it takes the initial focus:
	// pressing Enter without moving the focus cancels the connect.
	buttons := CreateUrwidButtonRow(cancelBtn, connectBtn)

	msgRows := strings.Count(message, "\n") + 1
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(message), msgRows, 0, false).
		AddItem(buttons, 1, 0, true)

	dm.ShowDialog("Blocked node", layout, 0, msgRows+1+2, nil)
	// Tab/Left/Right move focus between Cancel and Connect; Escape dismisses
	// through cancel; wireDialogNav re-focuses the first item (Cancel), which
	// is what makes Enter default to the safe option.
	wireDialogNav(dm.app, cancelFn, []tview.Primitive{cancelBtn, connectBtn})
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
	// Python's UrlEdit is a bare ReadlineEdit (default style, no background),
	// so the input field uses the terminal-default background + default text,
	// not a forced dark/gray fill.
	input.SetFieldBackgroundColor(tcell.ColorDefault)
	input.SetFieldTextColor(tcell.ColorDefault)

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

	confirmBtn := NewUrwidButton(confirmLabel).SetSelectedFunc(triggerConfirm)
	cancelBtn := NewUrwidButton(cancelLabel).SetSelectedFunc(triggerCancel)
	buttons := CreateUrwidButtonRow(confirmBtn, cancelBtn)

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

	// input 1 + button row 1 + 2 border = 4 PACK height (Python url_dialog).
	dm.ShowDialog(title, layout, 50, 4, nil)
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
		AddItem(NewUrwidCenterText(message), 1, 0, false).
		AddItem(list, 0, 1, true)

	dm.ShowDialog(title, layout, 0, 10, nil)
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
	text := NewUrwidCenterText(message)
	okBtn := NewUrwidButton("OK").SetSelectedFunc(func() { dm.DismissTop() })
	buttons := CreateUrwidButtonRow(okBtn)
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
