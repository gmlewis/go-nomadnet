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
		input      string
		wantCmd    string
		wantArg    string
		wantIsCmd  bool
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

func cmdContainsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}
