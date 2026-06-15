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
	"j":         "join",
	"leave":     "part",
	"q":         "quit",
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
func ParseSlashCommand(input string) (cmd, arg string, isCmd bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	input = strings.TrimPrefix(input, "/")
	parts := strings.SplitN(input, " ", 2)
	cmd = strings.TrimSpace(parts[0])
	if cmd == "" {
		return "", "", false
	}
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	if alias, ok := CommandAlias[cmd]; ok {
		cmd = alias
	}

	return cmd, arg, true
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
