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
	"testing"
)

func TestParseSlashCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input     string
		wantCmd   string
		wantArg   string
		wantIsCmd bool
	}{
		{"/help", "help", "", true},
		{"/ping", "ping", "", true},
		{"/list", "list", "", true},
		{"/join #general", "join", "#general", true},
		{"/j #test", "join", "#test", true},
		{"/part", "part", "", true},
		{"/leave #general", "part", "#general", true},
		{"/me dances", "me", "dances", true},
		{"/nick alice", "nick", "alice", true},
		{"/nick", "nick", "", true},
		{"/who", "who", "", true},
		{"/names", "names", "", true},
		{"/clear", "clear", "", true},
		{"/connect", "connect", "", true},
		{"/disconnect", "quit", "", true},
		{"/quit", "quit", "", true},
		{"hello world", "", "", false},
		{"", "", "", false},
		{"not a command", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			cmd, arg, isCmd := ParseSlashCommand(tt.input)
			if cmd != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tt.wantCmd)
			}
			if arg != tt.wantArg {
				t.Errorf("arg = %q, want %q", arg, tt.wantArg)
			}
			if isCmd != tt.wantIsCmd {
				t.Errorf("isCmd = %v, want %v", isCmd, tt.wantIsCmd)
			}
		})
	}
}

func TestIsLocalCommand(t *testing.T) {
	t.Parallel()

	localCmds := []string{"help", "clear", "nick", "quit"}
	for _, cmd := range localCmds {
		if !IsLocalCommand(cmd) {
			t.Errorf("IsLocalCommand(%q) = false, want true", cmd)
		}
	}

	remoteCmds := []string{"ping", "list", "join", "part", "me", "who", "names", "connect", "disconnect"}
	for _, cmd := range remoteCmds {
		if IsLocalCommand(cmd) {
			t.Errorf("IsLocalCommand(%q) = true, want false", cmd)
		}
	}
}

func TestIsServerForwardedCommand(t *testing.T) {
	t.Parallel()

	serverCmds := []string{"who", "names", "topic", "mode", "kick", "kline", "ban", "invite", "op", "deop", "voice", "devoice"}
	for _, cmd := range serverCmds {
		if !IsServerForwardedCommand(cmd) {
			t.Errorf("IsServerForwardedCommand(%q) = false, want true", cmd)
		}
	}

	localCmds := []string{"help", "clear", "nick", "join", "part"}
	for _, cmd := range localCmds {
		if IsServerForwardedCommand(cmd) {
			t.Errorf("IsServerForwardedCommand(%q) = true, want false", cmd)
		}
	}
}

func TestCommandAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		wantCmd string
	}{
		{"/j #room", "join"},
		{"/leave #room", "part"},
		{"/q", "quit"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			cmd, _, _ := ParseSlashCommand(tt.input)
			if cmd != tt.wantCmd {
				t.Errorf("ParseSlashCommand(%q) cmd = %q, want %q", tt.input, cmd, tt.wantCmd)
			}
		})
	}
}

// TestParseSlashCommandExtraSpaces pins the parse's whitespace handling
// (Python str.split(None, 1)).
func TestParseSlashCommandExtraSpaces(t *testing.T) {
	t.Parallel()

	cmd, arg, isCmd := ParseSlashCommand("/join   #general  ")
	if !isCmd {
		t.Fatal("expected isCmd=true")
	}
	if cmd != "join" {
		t.Errorf("cmd = %q, want %q", cmd, "join")
	}
	if arg != "#general" {
		t.Errorf("arg = %q, want %q", arg, "#general")
	}
}

// TestBareSlashEmptyCommand pins Python's empty-command error (Channels.py:
// 998-1001): a bare "/" in the composer gets the local "Empty command"
// notice, not the unknown-command fallback.
func TestBareSlashEmptyCommand(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")
	rw.hubStatusFn = func() int { return hubStatusConnected }
	var sent int
	rw.OnSendMessage = func(string) { sent++ }

	rw.editor.SetText("/")
	rw.sendMessage()

	if sent != 0 {
		t.Errorf("bare / sent %v messages, want 0", sent)
	}
	msgs := rw.ChatMessages()
	if len(msgs) != 1 || msgs[0].Text != "Empty command" {
		t.Fatalf("notices = %+v, want one local \"Empty command\" notice", msgs)
	}
	if rw.editor.GetText() != "" {
		t.Errorf("editor text = %q, want empty", rw.editor.GetText())
	}
}

func TestSlashHelpText(t *testing.T) {
	t.Parallel()

	help := SlashHelpText()
	if help == "" {
		t.Error("SlashHelpText() returned empty")
	}
	// Help text should mention the main commands
	for _, cmd := range []string{"/help", "/ping", "/join", "/nick", "/me"} {
		if !cmdContainsStr(help, cmd) {
			t.Errorf("SlashHelpText() missing %q", cmd)
		}
	}
}

// wantSlashHelp carries the Python RoomWidget.SLASH_HELP constant verbatim
// (Channels.py:948-979, byte-identical in the SOT checkout and the installed
// user-site 1.2.8 copy) — the /help output is a literal constant, so the port
// renders the exact lines including the column-aligned descriptions.
var wantSlashHelp = []string{
	"/help                                - show this list",
	"/ping                                - measure round-trip to hub",
	"/list                                - list public rooms on this hub",
	"/join <room>                         - join a room on this hub",
	"/part [room]                         - leave a room (default: current)",
	"/leave [room]                        - alias for /part",
	"/me <text>                           - send an action (e.g. /me waves)",
	"/nick <name>                         - set your nick on this hub only",
	"/who [room]                          - list users (current room if omitted)",
	"/names [room]                        - alias for /who",
	"/clear                               - clear local messages in this room",
	"/connect                             - connect this hub",
	"/disconnect                          - disconnect this hub",
	"/quit                                - alias for /disconnect",
	"",
	"Server-side commands (auth enforced by hub):",
	"/topic <room> [text]                 - view or set room topic",
	"/mode <room> [+-flags] [arg]         - view or set room modes",
	"/register <room>                     - register the current room",
	"/unregister <room>                   - unregister the current room",
	"/kick <room> <target>                - remove user from room",
	"/ban <room> add|del|list [target]    - room ban list",
	"/invite <room> add|del|list [target] - room invite list",
	"/op <room> <target>                  - grant op",
	"/deop <room> <target>                - revoke op",
	"/voice <room> <target>               - grant voice",
	"/devoice <room> <target>             - revoke voice",
	"/kline add|del|list [target]         - global ban",
	"/stats                               - server statistics",
	"/reload                              - reload server config",
}

// TestSlashHelpTextPythonParity pins the /help output line-for-line against
// the Python SLASH_HELP constant (captured live from the SOT — see the
// wantSlashHelp comment).
func TestSlashHelpTextPythonParity(t *testing.T) {
	t.Parallel()

	got := strings.Split(SlashHelpText(), "\n")
	if len(got) != len(wantSlashHelp) {
		t.Fatalf("SlashHelpText() = %v lines, want %v", len(got), len(wantSlashHelp))
	}
	for i, want := range wantSlashHelp {
		if got[i] != want {
			t.Errorf("SlashHelpText() line %v = %q, want %q", i+1, got[i], want)
		}
	}
}

func cmdContainsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}
