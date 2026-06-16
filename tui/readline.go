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

// killRing is a module-global kill buffer shared across all ReadlineEdit
// widgets, mirroring GNU readline behavior.
type killRing struct {
	text        string
	lastWasKill bool
}

var globalKillRing = &killRing{}

func (kr *killRing) resetChain() {
	kr.lastWasKill = false
}

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

// ReadlineEdit is a tview.InputField wrapper with readline-style
// editing key bindings:
//
//	Ctrl-A        beginning of line
//	Ctrl-E        end of line
//	Ctrl-U        kill to beginning of line
//	Ctrl-K        kill to end of line
//	Ctrl-W        kill previous word
//	Ctrl-L        kill whole buffer
//	Ctrl-Y        yank (paste killed text)
//	Ctrl-Left     backward word (alphanumeric boundary)
//	Ctrl-Right    forward word (alphanumeric boundary)
type ReadlineEdit struct {
	*tview.InputField
	cursorPos int
}

// NewReadlineEdit creates a new ReadlineEdit with the given label and placeholder.
func NewReadlineEdit(label, placeholder string) *ReadlineEdit {
	re := &ReadlineEdit{
		InputField: tview.NewInputField(),
		cursorPos:  0,
	}
	re.SetLabel(label)
	re.SetPlaceholder(placeholder)
	re.SetInputCapture(re.handleKey)
	return re
}

// handleKey processes readline-style key events.
func (re *ReadlineEdit) handleKey(event *tcell.EventKey) *tcell.EventKey {
	text := re.GetText()
	runes := []rune(text)

	switch event.Key() {
	case tcell.KeyCtrlA:
		// Beginning of line
		bol := findLineStart(text, re.cursorPos)
		re.cursorPos = bol
		return nil

	case tcell.KeyCtrlE:
		// End of line
		eol := findLineEnd(text, re.cursorPos)
		re.cursorPos = eol
		return nil

	case tcell.KeyCtrlU:
		// Kill to beginning of line
		bol := findLineStart(text, re.cursorPos)
		killed := string(runes[bol:re.cursorPos])
		globalKillRing.kill(killed, false)
		newRunes := append(runes[:bol], runes[re.cursorPos:]...)
		re.SetText(string(newRunes))
		re.cursorPos = bol
		return nil

	case tcell.KeyCtrlK:
		// Kill to end of line
		eol := findLineEnd(text, re.cursorPos)
		killed := string(runes[re.cursorPos:eol])
		globalKillRing.kill(killed, true)
		newRunes := append(runes[:re.cursorPos], runes[eol:]...)
		re.SetText(string(newRunes))
		return nil

	case tcell.KeyCtrlW:
		// Kill previous word
		p := re.cursorPos
		// Skip trailing whitespace
		for p > 0 && runes[p-1] == ' ' {
			p--
		}
		// Skip word characters
		for p > 0 && isWordChar(runes[p-1]) {
			p--
		}
		killed := string(runes[p:re.cursorPos])
		globalKillRing.kill(killed, false)
		newRunes := append(runes[:p], runes[re.cursorPos:]...)
		re.SetText(string(newRunes))
		re.cursorPos = p
		return nil

	case tcell.KeyCtrlL:
		// Kill whole buffer
		globalKillRing.kill(text, true)
		re.SetText("")
		re.cursorPos = 0
		return nil

	case tcell.KeyCtrlY:
		// Yank
		if globalKillRing.text == "" {
			return event
		}
		killRunes := []rune(globalKillRing.text)
		newRunes := make([]rune, 0, len(runes)+len(killRunes))
		newRunes = append(newRunes, runes[:re.cursorPos]...)
		newRunes = append(newRunes, killRunes...)
		newRunes = append(newRunes, runes[re.cursorPos:]...)
		re.SetText(string(newRunes))
		re.cursorPos += len(killRunes)
		globalKillRing.resetChain()
		return nil

	case tcell.KeyLeft:
		// Ctrl-Left: backward word (some terminals report Ctrl+Left as Left with ModAlt)
		if event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			pos := re.cursorPos
			for pos > 0 && !isWordChar(runes[pos-1]) {
				pos--
			}
			for pos > 0 && isWordChar(runes[pos-1]) {
				pos--
			}
			re.cursorPos = pos
			return nil
		}
		// Plain Left arrow
		if re.cursorPos > 0 {
			re.cursorPos--
		}
		return event

	case tcell.KeyRight:
		// Ctrl-Right: forward word
		if event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			n := len(runes)
			pos := re.cursorPos
			for pos < n && !isWordChar(runes[pos]) {
				pos++
			}
			for pos < n && isWordChar(runes[pos]) {
				pos++
			}
			re.cursorPos = pos
			return nil
		}
		// Plain Right arrow
		if re.cursorPos < len(runes) {
			re.cursorPos++
		}
		return event
	}

	// Track cursor position on regular key input
	if event.Key() == tcell.KeyRune {
		re.cursorPos++
		if re.cursorPos > len(runes) {
			re.cursorPos = len(runes)
		}
	}

	return event
}

// findLineStart returns the position of the start of the current line.
func findLineStart(text string, pos int) int {
	runes := []rune(text)
	for i := pos - 1; i >= 0; i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}
	return 0
}

// findLineEnd returns the position of the end of the current line.
func findLineEnd(text string, pos int) int {
	runes := []rune(text)
	for i := pos; i < len(runes); i++ {
		if runes[i] == '\n' {
			return i
		}
	}
	return len(runes)
}

// isWordChar returns true if the character is alphanumeric or underscore.
func isWordChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}

// ResetKillRing resets the global kill ring state.
func ResetKillRing() {
	globalKillRing.resetChain()
	globalKillRing.text = ""
}
