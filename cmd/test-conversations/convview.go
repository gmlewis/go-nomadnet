// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"regexp"
	"strings"

	"github.com/gmlewis/go-nomadnet/utils"
)

// convview.go holds conversation-specific View queries as free functions taking
// *utils.View (the methods must live in package utils, but these
// conversations-only helpers live here so utils stays generic). They reuse
// utils.View's existing ActivePage / FullText / HasBorderTitle where applicable.

// lxmfHashRe matches a 32-hex RNS/LXMF destination hash.
var lxmfHashRe = regexp.MustCompile(`[0-9a-f]{32}`)

// dialogOpen reports whether a bordered dialog with the given title is on
// screen (tview SetTitledBorder renders " Title " inside a row of '─' runes).
func dialogOpen(v *utils.View, title string) bool {
	if v == nil || v.Screen == nil {
		return false
	}
	return utils.HasBorderTitle(v.Screen.FullText(), title)
}

// extractLXMFHash returns the first 32-hex hash found anywhere on screen. Used
// to read an instance's own LXMF address off the C-p "My LXMF" dialog. Returns
// "" if no 32-hex run is present.
func extractLXMFHash(v *utils.View) string {
	if v == nil || v.Screen == nil {
		return ""
	}
	return lxmfHashRe.FindString(v.Screen.FullText())
}

// messageBodyContains reports whether the conversation message body (or any
// visible screen text) contains text. A coarse check suitable for asserting a
// sent/received message's literal content is on screen.
func messageBodyContains(v *utils.View, text string) bool {
	if v == nil || v.Screen == nil {
		return false
	}
	return strings.Contains(v.Screen.FullText(), text)
}

// onConversationsPage reports whether the active page is the Conversations page.
func onConversationsPage(v *utils.View) bool {
	return v != nil && v.ActivePage() == "conversations"
}

// onNetworkPage reports whether the active page is the Network page.
func onNetworkPage(v *utils.View) bool {
	return v != nil && v.ActivePage() == "network"
}

// shortcutRegion classifies the conversations shortcut bar into the focus
// region that produced it: "list", "editor", "body", or "" if unrecognized. The
// three bars differ only in their labels, so a substring match is robust.
//
// The bar is the LEFT-pane footer: when no conversation is open it is the
// full-width bottom row; when a conversation is open the right pane's composer
// and the bottom borders occupy the last rows, so the bar sits one row above
// the border in the left pane's footer slot. We therefore scan the bottom rows
// (not just the last non-blank one) for the distinctive per-region markers,
// which are mutually exclusive:
//   - list:  "[C-e] Peer Info" / "[C-n] New"        (and never [C-d] Send)
//   - editor: "[C-d] Send"                          (and never [C-e]/[C-w])
//   - body:   "[C-w] Close" / "[C-x] Clear History" (and never [C-e]/[C-d])
func shortcutRegion(v *utils.View) string {
	if v == nil || v.Screen == nil {
		return ""
	}
	for y := v.Screen.H - 1; y >= 0 && y >= v.Screen.H-8; y-- {
		t := v.Screen.RowText(y)
		switch {
		case strings.Contains(t, "[C-d] Send"):
			return "editor"
		case strings.Contains(t, "[C-e] Peer Info") || strings.Contains(t, "[C-n] New"):
			return "list"
		case strings.Contains(t, "[C-w] Close") || strings.Contains(t, "[C-x] Clear History"):
			return "body"
		}
	}
	return ""
}

// tabUnreadCount returns the largest "✉ N" unread count shown in the
// conversations tab bar (the row holding "Trusted"/"Untrusted"), or 0 if none.
// The tab bar renders e.g. "Untrusted (1) ✉ 1" when a tab has unread messages.
func tabUnreadCount(v *utils.View) int {
	if v == nil || v.Screen == nil {
		return 0
	}
	re := regexp.MustCompile(`✉ (\d+)`)
	maxN := 0
	for y := 0; y < v.Screen.H; y++ {
		t := v.Screen.RowText(y)
		if !strings.Contains(t, "Trusted") && !strings.Contains(t, "Untrusted") {
			continue
		}
		for _, m := range re.FindAllStringSubmatch(t, -1) {
			if len(m) > 1 {
				if n := atoiSafe(m[1]); n > maxN {
					maxN = n
				}
			}
		}
	}
	return maxN
}

// unreadRowCount counts list rows carrying an unread badge "✉ (N)". A count of
// 0 means no unread conversations are visible in the current tab.
func unreadRowCount(v *utils.View) int {
	return countBadgeRows(v, "✉ (")
}

// failedRowCount counts list rows carrying a failed badge "⚠ (N)".
func failedRowCount(v *utils.View) int {
	return countBadgeRows(v, "⚠ (")
}

// countBadgeRows counts screen rows containing the given badge prefix.
func countBadgeRows(v *utils.View, prefix string) int {
	if v == nil || v.Screen == nil {
		return 0
	}
	n := 0
	for y := 0; y < v.Screen.H; y++ {
		if strings.Contains(v.Screen.RowText(y), prefix) {
			n++
		}
	}
	return n
}

// menuUnreadShown reports whether the menu bar shows an unread indicator on the
// Conversations button (decoration_menu " +" or unread_menu " ✉"). This is the
// blink/ticker signal that an unread conversation exists while the user is
// elsewhere.
func menuUnreadShown(v *utils.View) bool {
	if v == nil || v.Screen == nil || v.Screen.H == 0 {
		return false
	}
	row := v.Screen.RowText(0)
	return strings.Contains(row, " ✉") || strings.Contains(row, " +")
}

// atoiSafe parses a base-10 int, returning 0 on any error.
func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
