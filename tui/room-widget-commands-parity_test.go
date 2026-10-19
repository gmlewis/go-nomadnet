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
	"errors"
	"strings"
	"testing"
)

// commandNotice is one local notice a command produced.
type commandNotice struct {
	kind string
	text string
}

// commandHarness builds a connected RoomWidget whose callbacks record their
// inputs and whose send callback can be made to fail.
func commandHarness(t *testing.T) (*RoomWidget, *[]commandNotice, *string) {
	t.Helper()
	rw := NewRoomWidget(newTestApp(), "hub1", "general")
	rw.hubStatusFn = func() int { return hubStatusConnected }
	notices := &[]commandNotice{}
	sent := new(string)
	rw.OnLocalMessage = func(kind, text string) error {
		*notices = append(*notices, commandNotice{kind, text})
		return nil
	}
	rw.OnSendMessage = func(text string) error {
		*sent = text
		return nil
	}
	return rw, notices, sent
}

// TestSlashCommandWhitespaceParsing pins Python's text[1:].split(None, 1)
// parsing (Channels.py:997-1001): a tab (or any whitespace run) separates the
// command from its argument, so "/join\t#room" is a join rather than an
// unknown command.
func TestSlashCommandWhitespaceParsing(t *testing.T) {
	t.Parallel()

	rw, notices, _ := commandHarness(t)
	var joined string
	rw.OnJoinRoomNamed = func(room string) error { joined = room; return nil }

	rw.editor.SetText("/join\t#room")
	rw.sendMessage()
	if joined != "room" {
		t.Errorf("/join with a tab joined %q, want %q (Python splits on any whitespace)", joined, "room")
	}
	if len(*notices) != 0 {
		t.Errorf("/join with a tab produced notices %v, want none", *notices)
	}

	// A bare slash (or only whitespace after it) is Python's "Empty command".
	rw.editor.SetText("/")
	rw.sendMessage()
	if len(*notices) != 1 || (*notices)[0].text != "Empty command" {
		t.Errorf("bare slash notices = %v, want [Empty command]", *notices)
	}
}

// TestNickCommandStrings pins Python's /nick messages (Channels.py:1073-1084):
// an unset nick renders the " unset" fallback (with its leading space), an
// override appends "nick: ", and setting echoes the argument as typed.
func TestNickCommandStrings(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, effective string, override bool, want string) {
		t.Helper()
		rw, notices, _ := commandHarness(t)
		rw.OnNickInfo = func() (string, bool) { return effective, override }
		rw.editor.SetText("/nick")
		rw.sendMessage()
		if len(*notices) != 1 || (*notices)[0].text != want {
			t.Errorf("/nick notices = %v, want [%q]", *notices, want)
		}
	}

	check(t, "", false, "Nick on this hub:  unset (global)")
	check(t, "bob", true, "Nick on this hub: bob (nick: )")
	check(t, "alice", false, "Nick on this hub: alice (global)")

	rw, notices, _ := commandHarness(t)
	var setTo string
	rw.OnSetNick = func(name string) error { setTo = name; return nil }
	rw.OnNickInfo = func() (string, bool) { return "truncated", true }
	rw.editor.SetText("/nick  ALICE ")
	rw.sendMessage()
	if setTo != "ALICE" {
		t.Errorf("/nick set %q, want the stripped argument %q", setTo, "ALICE")
	}
	want := "Nick on this hub set to ALICE (use /nick with no argument to view)"
	if len(*notices) != 1 || (*notices)[0].text != want {
		t.Errorf("/nick set notices = %v, want [%q] (Python echoes the argument as typed, not the effective nick)", *notices, want)
	}
}

// TestCommandSendFailuresReportLocally pins Python's failure notices:
// "/list failed: <e>" (Channels.py:1023-1026) and "/<cmd> failed: <e>" for the
// server-forwarded commands (Channels.py:1110-1115).
func TestCommandSendFailuresReportLocally(t *testing.T) {
	t.Parallel()

	rw, notices, _ := commandHarness(t)
	rw.OnSendMessage = func(string) error { return errors.New("boom") }

	rw.editor.SetText("/list")
	rw.sendMessage()
	if len(*notices) != 1 || (*notices)[0].text != "/list failed: boom" {
		t.Errorf("/list notices = %v, want [/list failed: boom]", *notices)
	}

	rw.editor.SetText("/who")
	rw.sendMessage()
	if len(*notices) != 2 || (*notices)[1].text != "/who failed: boom" {
		t.Errorf("/who notices = %v, want a /who failed notice", *notices)
	}
}

// TestSendKeepsWhitespace pins Python send_message (Channels.py:864-887): the
// editor text is transmitted as typed; only an all-whitespace draft is
// rejected.
func TestSendKeepsWhitespace(t *testing.T) {
	t.Parallel()

	rw, _, sent := commandHarness(t)
	rw.editor.SetText("  hello  ")
	rw.sendMessage()
	if *sent != "  hello  " {
		t.Errorf("sent %q, want the text with its whitespace preserved", *sent)
	}

	*sent = ""
	rw.editor.SetText("   ")
	rw.sendMessage()
	if *sent != "" {
		t.Errorf("an all-whitespace draft was sent as %q", *sent)
	}
}

// TestServerCommandNameInFailure verifies the command name carried in a
// server-command failure notice keeps its leading slash.
func TestServerCommandNameInFailure(t *testing.T) {
	t.Parallel()

	rw, notices, _ := commandHarness(t)
	rw.OnSendMessage = func(string) error { return errors.New("nope") }
	rw.editor.SetText("/topic new topic")
	rw.sendMessage()
	if len(*notices) != 1 || !strings.HasPrefix((*notices)[0].text, "/topic failed: ") {
		t.Errorf("notices = %v, want a /topic failed notice", *notices)
	}
}
