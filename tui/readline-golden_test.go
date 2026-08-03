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

// Golden (text, cursorPos, kill-ring) states captured by running the EXACT
// Python ReadlineMixin logic (ReadlineEdit.py) in a standalone harness that
// backs the mixin with a fake urwid.Edit, since urwid itself won't import on
// this host. Each step is the buffer state after the named key. These lock the
// Go port to Python's observable behavior for every readline binding plus the
// shared kill-ring accumulation/chain semantics.

type rlStep struct {
	key      string // "ctrl a", "ctrl left", "x", ...
	wantText string
	wantPos  int
	wantKR   string
}

type rlScenario struct {
	name  string
	init  string
	steps []rlStep
}

func rlEvent(desc string) *tcell.EventKey {
	switch desc {
	case "ctrl a":
		return tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone)
	case "ctrl e":
		return tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone)
	case "ctrl u":
		return tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone)
	case "ctrl k":
		return tcell.NewEventKey(tcell.KeyCtrlK, 0, tcell.ModNone)
	case "ctrl w":
		return tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone)
	case "ctrl l":
		return tcell.NewEventKey(tcell.KeyCtrlL, 0, tcell.ModNone)
	case "ctrl y":
		return tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone)
	case "ctrl left":
		return tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModCtrl)
	case "ctrl right":
		return tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModCtrl)
	default:
		// A single printable rune.
		return tcell.NewEventKey(tcell.KeyRune, []rune(desc)[0], tcell.ModNone)
	}
}

func TestReadlineGolden(t *testing.T) {
	t.Parallel()

	scenarios := []rlScenario{
		{
			name: "beg_end",
			init: "hello world",
			steps: []rlStep{
				{"ctrl a", "hello world", 0, ""},
				{"ctrl e", "hello world", 11, ""},
			},
		},
		{
			name: "kill_to_beg_yank",
			init: "hello world",
			steps: []rlStep{
				{"ctrl e", "hello world", 11, ""},
				{"ctrl u", "", 0, "hello world"},
				{"ctrl y", "hello world", 11, "hello world"},
			},
		},
		{
			name: "kill_to_end_mid_yank",
			init: "hello world",
			steps: []rlStep{
				{"ctrl a", "hello world", 0, ""},
				{"ctrl right", "hello world", 5, ""},
				{"ctrl k", "hello", 5, " world"},
				{"ctrl y", "hello world", 11, " world"},
			},
		},
		{
			name: "kill_word_back_punct",
			init: "foo.bar baz",
			steps: []rlStep{
				// Python _rl_kill_word_back is whitespace-delimited, so the
				// whole "baz" word dies (not just an alphanumeric token).
				{"ctrl w", "foo.bar ", 8, "baz"},
			},
		},
		{
			name: "kill_word_back_x2",
			init: "foo bar baz",
			steps: []rlStep{
				{"ctrl w", "foo bar ", 8, "baz"},
				// Consecutive backward kill PREPENDS -> "bar baz".
				{"ctrl w", "foo ", 4, "bar baz"},
			},
		},
		{
			name: "kill_chain_empty",
			init: "abc def ghi",
			steps: []rlStep{
				// Ctrl-K at end of line kills nothing; chain stays open but
				// the ring is never populated.
				{"ctrl k", "abc def ghi", 11, ""},
				{"ctrl k", "abc def ghi", 11, ""},
			},
		},
		{
			name: "kill_then_type_breaks",
			init: "abc def",
			steps: []rlStep{
				{"ctrl k", "abc def", 7, ""},
				// Typing a regular char breaks the kill chain.
				{"x", "abc defx", 8, ""},
				{"ctrl k", "abc defx", 8, ""},
			},
		},
		{
			name: "kill_then_move_breaks",
			init: "abc def",
			steps: []rlStep{
				{"ctrl k", "abc def", 7, ""},
				// A movement key breaks the chain, so the next kill replaces.
				{"ctrl a", "abc def", 0, ""},
				{"ctrl k", "", 0, "abc def"},
			},
		},
		{
			name: "ctrl_l_then_yank",
			init: "keep this",
			steps: []rlStep{
				{"ctrl l", "", 0, "keep this"},
				{"ctrl y", "keep this", 9, "keep this"},
			},
		},
		{
			name: "backward_forward_word",
			init: "hello world",
			steps: []rlStep{
				{"ctrl a", "hello world", 0, ""},
				{"ctrl right", "hello world", 5, ""},
				{"ctrl right", "hello world", 11, ""},
				{"ctrl left", "hello world", 6, ""},
			},
		},
		{
			name: "forward_word_mid",
			init: "ab cd ef",
			steps: []rlStep{
				{"ctrl a", "ab cd ef", 0, ""},
				{"ctrl right", "ab cd ef", 2, ""},
				{"ctrl left", "ab cd ef", 0, ""},
			},
		},
		{
			name: "yank_empty_nop",
			init: "abc",
			steps: []rlStep{
				{"ctrl k", "abc", 3, ""},
				{"ctrl l", "", 0, "abc"},
				{"ctrl y", "abc", 3, "abc"},
			},
		},
		{
			name: "ctrl_w_leading_ws",
			init: "   spaced",
			steps: []rlStep{
				// Ctrl-W skips leading whitespace then kills the word.
				{"ctrl w", "   ", 3, "spaced"},
			},
		},
		{
			name: "multiline_mid_kill",
			init: "line1\nline2\nline3",
			steps: []rlStep{
				// Ctrl-A goes to start of the *current* (third) line, not the buffer.
				{"ctrl a", "line1\nline2\nline3", 12, ""},
				{"ctrl right", "line1\nline2\nline3", 17, ""},
				// Ctrl-U from end of line3 kills "line3" (current line only).
				{"ctrl u", "line1\nline2\n", 12, "line3"},
			},
		},
		{
			name: "unicode_word",
			init: "café Müller",
			steps: []rlStep{
				{"ctrl a", "café Müller", 0, ""},
				// Python isalnum() treats é as a word char -> "café" is one word.
				{"ctrl right", "café Müller", 4, ""},
				{"ctrl w", " Müller", 0, "café"},
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			// Each scenario gets its own isolated kill ring shared across
			// the single ReadLineEdit under test (matching Python, where the
			// kill ring is per-process).
			kr := &killRing{}
			re := NewReadlineEdit(kr, "", "")
			re.SetText(sc.init)

			for i, st := range sc.steps {
				re.handleKey(rlEvent(st.key))
				if got := re.GetText(); got != st.wantText {
					t.Errorf("step %v (%q): text = %q, want %q", i, st.key, got, st.wantText)
				}
				if got := re.CursorPos(); got != st.wantPos {
					t.Errorf("step %v (%q): pos = %v, want %v", i, st.key, got, st.wantPos)
				}
				if got := kr.Text(); got != st.wantKR {
					t.Errorf("step %v (%q): killring = %q, want %q", i, st.key, got, st.wantKR)
				}
			}
		})
	}
}

// TestKillRingSharedAcrossInstances verifies the kill ring is shared across
// ReadlineEdit instances that were constructed with the same *killRing: text
// killed in one can be yanked into another.
func TestKillRingSharedAcrossInstances(t *testing.T) {
	t.Parallel()
	kr := &killRing{}

	a := NewReadlineEdit(kr, "", "")
	a.SetText("secret value")
	// Kill to end of line from the end (kills nothing), so move to start first.
	a.SetCursorPos(0)
	a.handleKey(tcell.NewEventKey(tcell.KeyCtrlK, 0, tcell.ModNone)) // kills "secret value"

	if got := kr.Text(); got != "secret value" {
		t.Fatalf("killring after kill = %q, want %q", got, "secret value")
	}

	b := NewReadlineEdit(kr, "", "")
	b.SetText("")
	b.handleKey(tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone)) // yank into other field

	if got := b.GetText(); got != "secret value" {
		t.Errorf("yank into second field: text = %q, want %q", got, "secret value")
	}
}
