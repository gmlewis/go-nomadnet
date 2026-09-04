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
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// killRing is an emacs-style kill buffer shared across the ReadlineEdit
// instances of one App, mirroring Python's module-global _KillRing
// (ReadlineEdit.py). Consecutive kills accumulate into the same entry
// (forward kills append, backward kills prepend); any non-kill keypress
// breaks the chain so the next kill replaces the buffer. It is owned by App
// (App.killRing) and passed into each ReadlineEdit so the buffer is shared
// without a package global.
type killRing struct {
	text        string
	lastWasKill bool
}

func (kr *killRing) resetChain() {
	kr.lastWasKill = false
}

// kill appends/prepends killed text to the shared buffer, following GNU
// readline accumulation rules. direction is "forward" (append) or "backward"
// (prepend), matching Python _KillRing.kill's `direction` argument.
func (kr *killRing) kill(killed string, forward bool) {
	if killed == "" {
		return
	}
	if kr.lastWasKill {
		if forward {
			kr.text += killed
		} else {
			kr.text = killed + kr.text
		}
	} else {
		kr.text = killed
	}
	kr.lastWasKill = true
}

// ReadlineEdit is a tview.InputField with readline-style editing keys,
// ported from Python's ReadlineMixin.keypress (ReadlineEdit.py). Bindings:
//
//	Ctrl-A        beginning of line
//	Ctrl-E        end of line
//	Ctrl-U        unix-line-discard (kill from cursor to beginning of line)
//	Ctrl-K        kill-line (kill from cursor to end of line)
//	Ctrl-W        unix-word-rubout (kill previous whitespace-delimited word)
//	Ctrl-L        kill-whole-buffer (kill the entire edit buffer)
//	Ctrl-Y        yank (insert most-recently-killed text)
//	Ctrl-Left     backward-word (alphanumeric boundary)
//	Ctrl-Right    forward-word (alphanumeric boundary)
//
// "Line" is the current logical line within the buffer (newline-delimited),
// so on multiline edits these act on the line under the cursor.
//
// The kill buffer is shared across all ReadlineEdit instances of one App via
// the App-owned *killRing passed into NewReadLineEdit.
//
// The edit model (text + cursorPos, both rune-based) is the source of truth
// and is fully driven by handleKey; the embedded tview.InputField is synced
// via SetText for display. tview's InputField exposes no public cursor
// setter, so after a non-end cursor move the displayed caret may lag the
// model cursor — the model itself (tested in readline_test.go) is correct.
type ReadlineEdit struct {
	*tview.InputField
	killRing  *killRing
	cursorPos int // rune offset, mirrors Python edit_pos
	// multiline switches on the urwid Edit(multiline=True) parity editor:
	// wrapping rows, Enter inserting a newline, wrapped-row cursor
	// navigation, and the custom drawMultiline renderer. Only the editors
	// Python declares multiline=True use it (SetMultiline,
	// tui/readline-multiline.go).
	multiline bool
	// prefCol/prefColW mirror urwid's pref_col_maxcol: the column targeted
	// by the last vertical navigation (or the 'left'/'right' literals stored
	// by Home/End), plus the width it was set at. A width change makes it
	// stale, falling back to the current cursor column (urwid get_pref_col).
	prefCol  int
	prefColW int
	// OnFocusTopRow, when set, fires when Up is pressed on the TOP wrapped
	// row of a multiline editor — Python RoomMessageEdit.keypress "up"
	// (Channels.py:429-434) moves the frame focus to the message body. nil
	// for editors without a body to jump to.
	OnFocusTopRow func()
	// onExit, when set, is invoked for Down/Up/Tab while editing so an embedded
	// field editor (the browser's in-place field overlay) can hand focus back to
	// the surrounding nav model — mirroring Python's urwid.Edit returning these
	// keys to the Pile/Scrollable. Enter/Esc are consumed (stay editing) to match
	// Python's Edit not submitting from a field row. nil for standalone editors
	// (compose, dialogs, conversations) preserves the existing behavior.
	onExit func(key tcell.Key)
}

// NewReadLineEdit creates a new ReadlineEdit with the given shared kill ring,
// label, and placeholder. The kill ring is shared across all ReadlineEdit
// instances of one App so emacs kill/yank works across fields.
func NewReadlineEdit(kr *killRing, label, placeholder string) *ReadlineEdit {
	re := &ReadlineEdit{
		InputField: tview.NewInputField(),
		killRing:   kr,
		cursorPos:  0,
		prefColW:   -1, // no preferred column yet (urwid pref_col_maxcol None)
	}
	re.SetLabel(label)
	re.SetPlaceholder(placeholder)
	re.SetInputCapture(re.handleKey)
	return re
}

// SetText sets the edit buffer and positions the model cursor at the end,
// matching Python's urwid.Edit.set_edit_text (which leaves edit_pos at end).
func (re *ReadlineEdit) SetText(text string) {
	re.cursorPos = len([]rune(text))
	re.InputField.SetText(text)
}

// CursorPos returns the current model cursor position as a rune offset.
func (re *ReadlineEdit) CursorPos() int {
	return re.cursorPos
}

// SetCursorPos sets the model cursor position (clamped to the buffer length).
func (re *ReadlineEdit) SetCursorPos(pos int) {
	if pos < 0 {
		pos = 0
	}
	if n := len([]rune(re.GetText())); pos > n {
		pos = n
	}
	re.cursorPos = pos
}

// Draw overrides tview.InputField.Draw to reposition the terminal hardware
// cursor at the model cursorPos. tview's InputField wraps an internal TextArea
// whose cursor stays at the end after SetText (there is no public cursor
// setter), so a non-end model cursor (e.g. after Ctrl-A) would otherwise show
// the caret at the wrong column. tview's Application.Draw hides the cursor each
// frame, so the focused field must re-show it on every draw. This mirrors the
// Python original, where urwid positions the hardware cursor at edit_pos
// (ReadlineEdit.py / urwid.Edit.render → canvas.cursor).
//
// The caret column is the inner-rect origin plus the label width plus the
// display width of the text before the cursor. For text that fits the field
// width (the common case for nomadnet's short single-line inputs) this matches
// the rendered glyph exactly; when the field horizontally scrolls the position
// is clamped to the inner right edge (tview's scroll offset is internal and
// not exposed) — the unclamped caret previously walked past the panel edge,
// the fleet-reported symptom on glenn-OMEN-875.
func (re *ReadlineEdit) Draw(screen tcell.Screen) {
	if re.multiline {
		// urwid Edit(multiline=True) renderer: wrapped rows + wrapped caret
		// (readline-multiline.go).
		re.drawMultiline(screen)
		return
	}
	re.InputField.Draw(screen)
	if !re.InputField.HasFocus() {
		return
	}
	x, y, w, _ := re.GetInnerRect()
	labelW := tview.TaggedStringWidth(re.GetLabel())
	runes := []rune(re.GetText())
	pos := min(max(re.cursorPos, 0), len(runes))
	col := runewidth.StringWidth(string(runes[:pos]))
	if w > 0 && x+labelW+col > x+w-1 {
		col = w - 1 - labelW
	}
	screen.ShowCursor(x+labelW+col, y)
}

// handleKey processes one key event, mirroring ReadlineMixin.keypress. It
// returns nil when it consumes the event (readline keys and regular rune
// insertion) or the event itself to let tview handle non-readline keys.
func (re *ReadlineEdit) handleKey(event *tcell.EventKey) *tcell.EventKey {
	// Multiline (urwid Edit multiline=True) keys: Enter inserts a newline
	// (urwid/widget/edit.py:436), Up/Down move between wrapped rows
	// (edit.py:449-464), Home/End bound the wrapped row (MAX_LEFT/MAX_RIGHT).
	// Everything else falls through to the shared readline handling, whose
	// buffer-level keys work identically on the multiline buffer.
	if re.multiline {
		switch event.Key() {
		case tcell.KeyEnter:
			runes := []rune(re.GetText())
			pos := min(re.cursorPos, len(runes))
			newRunes := make([]rune, 0, len(runes)+1)
			newRunes = append(newRunes, runes[:pos]...)
			newRunes = append(newRunes, '\n')
			newRunes = append(newRunes, runes[pos:]...)
			re.cursorPos = pos + 1
			re.killRing.resetChain()
			re.InputField.SetText(string(newRunes))
			return nil
		case tcell.KeyUp, tcell.KeyDown:
			return re.multilineVertical(event)
		case tcell.KeyHome, tcell.KeyEnd:
			if re.multilineRowKeys(event) {
				return nil
			}
		}
	}
	// Embedded field-overlay nav keys: hand Down/Up/Tab back to the browser nav
	// model (Python Edit returns these to the Pile/Scrollable). Enter/Esc stay
	// editing (a field row does not submit on Enter — only links do). Only
	// active when onExit is set (the browser overlay); standalone editors are
	// unaffected.
	if re.onExit != nil {
		switch event.Key() {
		case tcell.KeyDown, tcell.KeyUp, tcell.KeyTab:
			re.killRing.resetChain()
			re.onExit(event.Key())
			return nil
		case tcell.KeyEnter, tcell.KeyEsc:
			re.killRing.resetChain()
			return nil
		}
	}
	runes := []rune(re.GetText())
	pos := min(re.cursorPos, len(runes))

	killKey := false
	consumed := true

	switch event.Key() {
	case tcell.KeyCtrlA:
		pos = lineStart(runes, pos)
	case tcell.KeyCtrlE:
		pos = lineEnd(runes, pos)
	case tcell.KeyCtrlU:
		killKey = true
		bol := lineStart(runes, pos)
		re.killRing.kill(string(runes[bol:pos]), false)
		runes = append(runes[:bol], runes[pos:]...)
		pos = bol
	case tcell.KeyCtrlK:
		killKey = true
		eol := lineEnd(runes, pos)
		re.killRing.kill(string(runes[pos:eol]), true)
		runes = append(runes[:pos], runes[eol:]...)
	case tcell.KeyCtrlW:
		killKey = true
		p := pos
		for p > 0 && unicode.IsSpace(runes[p-1]) {
			p--
		}
		for p > 0 && !unicode.IsSpace(runes[p-1]) {
			p--
		}
		re.killRing.kill(string(runes[p:pos]), false)
		runes = append(runes[:p], runes[pos:]...)
		pos = p
	case tcell.KeyCtrlL:
		killKey = true
		re.killRing.kill(string(runes), true)
		runes = runes[:0]
		pos = 0
	case tcell.KeyCtrlY:
		if re.killRing.text != "" {
			kr := []rune(re.killRing.text)
			newRunes := make([]rune, 0, len(runes)+len(kr))
			newRunes = append(newRunes, runes[:pos]...)
			newRunes = append(newRunes, kr...)
			newRunes = append(newRunes, runes[pos:]...)
			runes = newRunes
			pos += len(kr)
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		// Delete backward, matching urwid.Edit's own backspace handling. The
		// model must consume this itself: letting the embedded InputField
		// delete its own (display-only) buffer would desync the model cursor.
		if pos > 0 {
			runes = append(runes[:pos-1], runes[pos:]...)
			pos--
		}
	case tcell.KeyDelete:
		if pos < len(runes) {
			runes = append(runes[:pos], runes[pos+1:]...)
		}
	case tcell.KeyLeft:
		if event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			for pos > 0 && !isWordChar(runes[pos-1]) {
				pos--
			}
			for pos > 0 && isWordChar(runes[pos-1]) {
				pos--
			}
		} else {
			consumed = false
		}
	case tcell.KeyRight:
		if event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			n := len(runes)
			for pos < n && !isWordChar(runes[pos]) {
				pos++
			}
			for pos < n && isWordChar(runes[pos]) {
				pos++
			}
		} else {
			consumed = false
		}
	case tcell.KeyRune:
		// Own regular character insertion so the model stays consistent with
		// the buffer (tview's InputField has no readable cursor position).
		ch := event.Rune()
		newRunes := make([]rune, 0, len(runes)+1)
		newRunes = append(newRunes, runes[:pos]...)
		newRunes = append(newRunes, ch)
		newRunes = append(newRunes, runes[pos:]...)
		runes = newRunes
		pos++
	default:
		consumed = false
	}

	// Match Python: any key that is NOT a kill key breaks the kill chain.
	if !killKey {
		re.killRing.resetChain()
	}

	if !consumed {
		// Let tview handle non-readline keys (Backspace, Delete, plain
		// arrows, Enter, ...). Re-sync the model cursor on plain horizontal
		// movement so subsequent readline kills use a sane position.
		switch event.Key() {
		case tcell.KeyLeft:
			if pos > 0 {
				pos--
			}
		case tcell.KeyRight:
			if pos < len(runes) {
				pos++
			}
		}
		re.cursorPos = pos
		return event
	}

	re.cursorPos = pos
	re.InputField.SetText(string(runes))
	return nil
}

// lineStart returns the rune index of the start of the current logical line
// (the rune after the previous newline, or 0), matching Python _rl_line_bounds bol.
func lineStart(runes []rune, pos int) int {
	for i := pos - 1; i >= 0; i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}
	return 0
}

// lineEnd returns the rune index of the end of the current logical line (the
// next newline, or len(runes)), matching Python _rl_line_bounds eol.
func lineEnd(runes []rune, pos int) int {
	for i := pos; i < len(runes); i++ {
		if runes[i] == '\n' {
			return i
		}
	}
	return len(runes)
}

// isWordChar reports whether ch is an alphanumeric or underscore character,
// matching Python's _rl_is_word_char (ch.isalnum() or ch == '_'). Unicode
// letters/digits are word chars, so "café" is one word.
func isWordChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_'
}

// Text returns the current kill-ring contents (for testing).
func (kr *killRing) Text() string {
	return kr.text
}

// LastWasKill reports whether the previous keypress was a kill (i.e. the
// accumulation chain is still open) — for testing.
func (kr *killRing) LastWasKill() bool {
	return kr.lastWasKill
}

// Reset clears the kill-ring state (text and chain flag), for testing.
func (kr *killRing) Reset() {
	kr.text = ""
	kr.lastWasKill = false
}
