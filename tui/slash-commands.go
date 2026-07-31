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

// SlashHelpText returns the formatted help text listing all
// available slash commands. Matches Python's SLASH_HELP constant.
func SlashHelpText() string {
	return `/help — Show this help
/ping — Ping the hub
/list — List rooms
/join #room — Join a room (alias: /j)
/part [#room] — Leave a room (alias: /leave)
/me action — Send an action
/nick [name] — Show or set your nick
/who — List room members (server)
/names — List room members (server)
/topic [text] — Show or set topic (server)
/clear — Clear message history
/connect — Connect to hub
/disconnect — Disconnect from hub (alias: /quit)
/op, /deop, /voice, /devoice — Moderation (server)
/kick, /ban, /kline — Moderation (server)`
}
