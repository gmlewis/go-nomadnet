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

import "testing"

// Expected values captured from Python Channels.py _handle_slash_command
// (Channels.py:997-1003), extracted and run in /tmp/slash_parse.py. The
// Python code lowercases the command name and splits on any whitespace run
// (str.split(None, 1)). Input is lstripped upstream before reaching the
// parser, matching ParseSlashCommand's leading TrimSpace.
func TestParseSlashCommandParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input     string
		wantCmd   string
		wantArg   string
		wantIsCmd bool
	}{
		{"/JOIN #general", "join", "#general", true},
		{"/Join #General", "join", "#General", true},
		{"/HELP", "help", "", true},
		{"/Me dances", "me", "dances", true},
		{"/join\t#general", "join", "#general", true},
		{"/nick   Alice  ", "nick", "Alice", true},
		{"/JOIN", "join", "", true},
		{"/J #room", "join", "#room", true},
		{"/Leave #room", "part", "#room", true},
		{"/", "", "", false},
		{"/   ", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			cmd, arg, isCmd := ParseSlashCommand(tc.input)
			if cmd != tc.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tc.wantCmd)
			}
			if arg != tc.wantArg {
				t.Errorf("arg = %q, want %q", arg, tc.wantArg)
			}
			if isCmd != tc.wantIsCmd {
				t.Errorf("isCmd = %v, want %v", isCmd, tc.wantIsCmd)
			}
		})
	}
}
