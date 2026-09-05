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

import "strings"

// CommandAlias maps aliases to their canonical command names.
var CommandAlias = map[string]string{
	"j":          "join",
	"leave":      "part",
	"q":          "quit",
	"disconnect": "quit",
}

// localCommands are commands handled entirely on the client side.
var localCommands = map[string]bool{
	"help":  true,
	"clear": true,
	"nick":  true,
	"quit":  true,
}

// serverForwardedCommands are forwarded verbatim to the RRC hub.
var serverForwardedCommands = map[string]bool{
	"who":        true,
	"names":      true,
	"topic":      true,
	"mode":       true,
	"kick":       true,
	"kline":      true,
	"ban":        true,
	"invite":     true,
	"op":         true,
	"deop":       true,
	"voice":      true,
	"devoice":    true,
	"register":   true,
	"unregister": true,
	"stats":      true,
	"reload":     true,
}

// ParseSlashCommand parses a chat input line and returns the command
// name, argument, and whether it was a slash command.
// Aliases are resolved: /j → join, /leave → part, /q → quit.
//
// Matches Python Channels.py _handle_slash_command parsing
// (Channels.py:997-1003): the command name is lowercased and the line is
// split on the first whitespace run (str.split(None, 1)), so any whitespace
// (spaces or tabs) separates the command from its argument while internal
// whitespace in the argument is preserved.
func ParseSlashCommand(input string) (cmd, arg string, isCmd bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	body := strings.TrimPrefix(input, "/")
	// Skip leading whitespace (Python's split(None, 1) collapses it).
	i := 0
	for i < len(body) && isMicronSpace(body[i]) {
		i++
	}
	// The command word runs up to the next whitespace.
	j := i
	for j < len(body) && !isMicronSpace(body[j]) {
		j++
	}
	cmd = body[i:j]
	if cmd == "" {
		return "", "", false
	}
	cmd = strings.ToLower(cmd)
	if j < len(body) {
		arg = strings.TrimSpace(body[j:])
	}

	if alias, ok := CommandAlias[cmd]; ok {
		cmd = alias
	}

	return cmd, arg, true
}

// isMicronSpace reports whether b is an ASCII whitespace byte, matching the
// separators used by Python's str.split(None) (space, tab, newline, carriage
// return, form feed, vertical tab).
func isMicronSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// IsLocalCommand returns true if the command is handled locally
// (no message sent to the server).
func IsLocalCommand(cmd string) bool {
	return localCommands[cmd]
}

// IsServerForwardedCommand returns true if the command is forwarded
// verbatim to the RRC hub.
func IsServerForwardedCommand(cmd string) bool {
	return serverForwardedCommands[cmd]
}

// SlashHelpText returns the /help output: the Python RoomWidget.SLASH_HELP
// constant verbatim (Channels.py:948-979), including the column-aligned
// descriptions and the server-side command group.
func SlashHelpText() string {
	return `/help                                - show this list
/ping                                - measure round-trip to hub
/list                                - list public rooms on this hub
/join <room>                         - join a room on this hub
/part [room]                         - leave a room (default: current)
/leave [room]                        - alias for /part
/me <text>                           - send an action (e.g. /me waves)
/nick <name>                         - set your nick on this hub only
/who [room]                          - list users (current room if omitted)
/names [room]                        - alias for /who
/clear                               - clear local messages in this room
/connect                             - connect this hub
/disconnect                          - disconnect this hub
/quit                                - alias for /disconnect

Server-side commands (auth enforced by hub):
/topic <room> [text]                 - view or set room topic
/mode <room> [+-flags] [arg]         - view or set room modes
/register <room>                     - register the current room
/unregister <room>                   - unregister the current room
/kick <room> <target>                - remove user from room
/ban <room> add|del|list [target]    - room ban list
/invite <room> add|del|list [target] - room invite list
/op <room> <target>                  - grant op
/deop <room> <target>                - revoke op
/voice <room> <target>               - grant voice
/devoice <room> <target>             - revoke voice
/kline add|del|list [target]         - global ban
/stats                               - server statistics
/reload                              - reload server config`
}
