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
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestIsWordChar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input rune
		want  bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', true},
		{' ', false},
		{'-', false},
		{'.', false},
		{'@', false},
		// Unicode letters are word chars, matching Python's str.isalnum()
		// (ReadlineEdit._rl_is_word_char) — "café" is one word.
		{'é', true},
		{'ü', true},
		{'ñ', true},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			t.Parallel()
			got := isWordChar(tt.input)
			if got != tt.want {
				t.Errorf("isWordChar(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindLineStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		pos  int
		want int
	}{
		{"hello", 0, 0},
		{"hello", 3, 0},
		{"hello\nworld", 7, 6},
		{"hello\nworld", 5, 0},
		{"hello\nworld", 11, 6},
		{"a\nb\nc", 4, 4},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()
			got := lineStart([]rune(tt.text), tt.pos)
			if got != tt.want {
				t.Errorf("lineStart(%q, %d) = %d, want %d", tt.text, tt.pos, got, tt.want)
			}
		})
	}
}

func TestFindLineEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		pos  int
		want int
	}{
		{"hello", 0, 5},
		{"hello", 3, 5},
		{"hello\nworld", 0, 5},
		{"hello\nworld", 6, 11},
		{"hello\nworld", 7, 11},
		{"a\nb\nc", 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()
			got := lineEnd([]rune(tt.text), tt.pos)
			if got != tt.want {
				t.Errorf("lineEnd(%q, %d) = %d, want %d", tt.text, tt.pos, got, tt.want)
			}
		})
	}
}

func TestKillRingBasic(t *testing.T) {
	t.Parallel()

	kr := &killRing{}

	// First kill
	kr.kill("hello", true)
	if kr.text != "hello" {
		t.Errorf("kill text = %q, want %q", kr.text, "hello")
	}
	if !kr.lastWasKill {
		t.Error("lastWasKill = false, want true")
	}

	// Second kill (consecutive) should append
	kr.kill(" world", true)
	if kr.text != "hello world" {
		t.Errorf("kill text = %q, want %q", kr.text, "hello world")
	}

	// Reset chain
	kr.resetChain()
	if kr.lastWasKill {
		t.Error("lastWasKill = true after reset")
	}

	// New kill after reset should replace
	kr.kill("new", true)
	if kr.text != "new" {
		t.Errorf("kill text = %q, want %q", kr.text, "new")
	}
}

func TestKillRingBackward(t *testing.T) {
	t.Parallel()

	kr := &killRing{}

	kr.kill("hello", false)
	if kr.text != "hello" {
		t.Errorf("backward kill = %q, want %q", kr.text, "hello")
	}

	kr.kill(" world", false)
	if kr.text != " worldhello" {
		t.Errorf("backward kill append = %q, want %q", kr.text, " worldhello")
	}
}

func TestKillRingEmpty(t *testing.T) {
	t.Parallel()

	kr := &killRing{}
	kr.kill("", true)
	if kr.text != "" {
		t.Errorf("empty kill = %q, want empty", kr.text)
	}
	if kr.lastWasKill {
		t.Error("lastWasKill should be false for empty kill")
	}
}

func TestNewReadlineEdit(t *testing.T) {
	t.Parallel()

	re := NewReadlineEdit(&killRing{}, "Label: ", "placeholder")
	if re == nil {
		t.Fatal("NewReadlineEdit returned nil")
	}
	if re.InputField == nil {
		t.Error("InputField is nil")
	}
}

func TestReadlineEditCursorTracking(t *testing.T) {
	t.Parallel()

	re := NewReadlineEdit(&killRing{}, "", "")
	re.SetText("hello")
	re.cursorPos = 5

	// Verify cursor position tracking
	if re.cursorPos != 5 {
		t.Errorf("cursor pos = %d, want 5", re.cursorPos)
	}
}

func TestResetKillRing(t *testing.T) {
	t.Parallel()

	kr := &killRing{}
	kr.kill("test", true)
	kr.Reset()

	if kr.text != "" {
		t.Errorf("kill ring text = %q, want empty", kr.text)
	}
	if kr.lastWasKill {
		t.Error("lastWasKill should be false after reset")
	}
}

func TestBackwardWord(t *testing.T) {
	t.Parallel()

	re := NewReadlineEdit(&killRing{}, "", "")
	re.SetText("hello world foo")

	// Move cursor to end
	re.cursorPos = len([]rune("hello world foo"))

	// Ctrl-Left: skip non-word (none), skip word "foo" → start of "foo"
	event := tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModAlt)
	re.handleKey(event)
	pos := re.cursorPos
	if pos != 12 {
		t.Errorf("after backward word from end: pos = %d, want 12 (start of 'foo')", pos)
	}

	// Ctrl-Left again: skip space, skip word "world" → start of "world"
	event = tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModAlt)
	re.handleKey(event)
	pos = re.cursorPos
	if pos != 6 {
		t.Errorf("after backward word from 12: pos = %d, want 6 (start of 'world')", pos)
	}

	// Ctrl-Left again: skip space, skip word "hello" → start of "hello"
	event = tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModAlt)
	re.handleKey(event)
	pos = re.cursorPos
	if pos != 0 {
		t.Errorf("after backward word from 6: pos = %d, want 0 (start of 'hello')", pos)
	}
}

func TestForwardWord(t *testing.T) {
	t.Parallel()

	re := NewReadlineEdit(&killRing{}, "", "")
	re.SetText("hello world foo")
	re.cursorPos = 0

	// Ctrl-Right: skip non-word (none), skip word "hello" → end of "hello"
	event := tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModAlt)
	re.handleKey(event)
	pos := re.cursorPos
	if pos != 5 {
		t.Errorf("after forward word from 0: pos = %d, want 5 (end of 'hello')", pos)
	}

	// Ctrl-Right again: skip space, skip word "world" → end of "world"
	event = tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModAlt)
	re.handleKey(event)
	pos = re.cursorPos
	if pos != 11 {
		t.Errorf("after forward word from 5: pos = %d, want 11 (end of 'world')", pos)
	}

	// Ctrl-Right again: skip space, skip word "foo" → end of "foo"
	event = tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModAlt)
	re.handleKey(event)
	pos = re.cursorPos
	if pos != 15 {
		t.Errorf("after forward word from 11: pos = %d, want 15 (end of 'foo')", pos)
	}
}

func TestWordMovementWithSpecialChars(t *testing.T) {
	t.Parallel()

	re := NewReadlineEdit(&killRing{}, "", "")
	re.SetText("hello-world_foo bar")

	re.cursorPos = len([]rune("hello-world_foo bar"))

	// Ctrl-Left: skip non-word (none), skip word "bar" → start of "bar"
	event := tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModAlt)
	re.handleKey(event)
	pos := re.cursorPos
	if pos != 16 {
		t.Errorf("special chars backward: pos = %d, want 16 (start of 'bar')", pos)
	}

	// Ctrl-Right from start: skip non-word (none), skip word "hello" → end of "hello"
	re.cursorPos = 0
	event = tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModAlt)
	re.handleKey(event)
	pos = re.cursorPos
	if pos != 5 {
		t.Errorf("special chars forward: pos = %d, want 5 (end of 'hello')", pos)
	}
}
