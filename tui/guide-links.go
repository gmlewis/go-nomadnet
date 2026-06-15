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
	"regexp"
	"strings"
)

// LinkAction represents the type of action to take when a guide link is clicked.
type LinkAction int

const (
	// ActionNone indicates no action should be taken.
	ActionNone LinkAction = iota
	// ActionAnchorJump indicates an in-page anchor jump.
	ActionAnchorJump
	// ActionOpenPage indicates opening a NomadNet page in the browser.
	ActionOpenPage
	// ActionSendMessage indicates opening a conversation to send a message.
	ActionSendMessage
	// ActionOpenRRC indicates opening an RRC hub/room.
	ActionOpenRRC
	// ActionPartial indicates a partial page update.
	ActionPartial
)

// ResolveGuideLink parses a guide link target and returns the action
// type and relevant data. This matches the link routing logic in
// Python's Guide.py GuideLinkDelegate.handle_link() and
// Browser.py handle_link().
func ResolveGuideLink(target string) (action LinkAction, data string) {
	if target == "" {
		return ActionNone, ""
	}

	// Anchor link
	if strings.HasPrefix(target, "#") {
		return ActionAnchorJump, target[1:]
	}

	// RRC link
	if strings.HasPrefix(target, "rrc://") {
		return ActionOpenRRC, target[6:]
	}

	// Partial update
	if strings.HasPrefix(target, "p:") {
		return ActionPartial, target[2:]
	}

	// Split on @ for type prefix
	parts := strings.SplitN(target, "@", 2)
	if len(parts) == 2 {
		destType := expandDestType(parts[0])
		hash := parts[1]

		switch destType {
		case "lxmf.delivery":
			return ActionSendMessage, hash
		case "nomadnetwork.node":
			return ActionOpenPage, hash
		default:
			return ActionOpenPage, target
		}
	}

	// Plain address — treat as page node
	return ActionOpenPage, target
}

// expandDestType maps shorthand destination type names to their
// full forms, matching Python's Browser.expand_shorthands().
func expandDestType(short string) string {
	switch short {
	case "nnn":
		return "nomadnetwork.node"
	case "lxmf":
		return "lxmf.delivery"
	case "rrc":
		return "rrc.hub.session"
	default:
		return short
	}
}

// anchorRe matches Micron anchor declarations like [#anchor-name].
var anchorRe = regexp.MustCompile(`\[#([^\]]+)\]`)

// ExtractAnchors extracts all anchor names from Micron content.
// Matches Python's MicronParser anchor extraction logic.
func ExtractAnchors(content string) []string {
	matches := anchorRe.FindAllStringSubmatch(content, -1)
	anchors := make([]string, 0, len(matches))
	for _, m := range matches {
		anchors = append(anchors, m[1])
	}
	return anchors
}

// isExternalURL checks if a target string looks like an HTTP(S) URL.
func isExternalURL(target string) bool {
	return strings.HasPrefix(target, "http://") ||
		strings.HasPrefix(target, "https://")
}

// ResolveExternalURL extracts and validates an external URL from
// a link target. Returns the URL and true if valid, or empty
// string and false if the target is not an external URL.
func ResolveExternalURL(target string) (url string, ok bool) {
	if isExternalURL(target) {
		return target, true
	}
	return "", false
}
