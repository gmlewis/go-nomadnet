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
	"fmt"
	"strings"
)

// TrustDisplayIcon returns the single-character trust icon for a trust level.
// Matches Python's trust display icons used in list items.
func TrustDisplayIcon(trustLevel string) string {
	switch trustLevel {
	case TrustTrusted:
		return "●"
	case TrustUntrusted:
		return "×"
	case TrustWarning:
		return "⚠"
	default:
		return "○"
	}
}

// FormatTrustLabel returns the human-readable trust label for display.
func FormatTrustLabel(trustLevel string) string {
	switch trustLevel {
	case TrustTrusted:
		return "Trusted"
	case TrustUntrusted:
		return "Untrusted"
	case TrustWarning:
		return "Warning"
	default:
		return "Unknown"
	}
}

// FormatNodeSummary produces a one-line summary for a known node entry.
// If the node has a display name, returns "icon name"; otherwise "icon <hash>".
func FormatNodeSummary(entry *NodeEntryFull) string {
	icon := TrustDisplayIcon(entry.TrustLevel)
	if entry.DisplayName != "" {
		return fmt.Sprintf("%s %s", icon, entry.DisplayName)
	}
	if len(entry.SourceHash) >= 12 {
		return fmt.Sprintf("%s <%s…>", icon, entry.SourceHash[:12])
	}
	return fmt.Sprintf("%s <%s>", icon, entry.SourceHash)
}

// FormatNodeDetail produces a multi-line detail view for a node entry.
// Supports edit mode where the name field is shown as editable.
func FormatNodeDetail(entry *NodeEntryFull, editable bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[::b]Node Details[-]\n\n"))
	sb.WriteString(fmt.Sprintf("  Name: %s\n", entry.DisplayName))
	if editable {
		sb.WriteString("  [gray](editable)[-]\n")
	}
	sb.WriteString(fmt.Sprintf("  Trust: %s %s\n", TrustDisplayIcon(entry.TrustLevel), FormatTrustLabel(entry.TrustLevel)))
	sb.WriteString(fmt.Sprintf("  Hash: %s\n", entry.SourceHash))
	sb.WriteString(fmt.Sprintf("  Delivery: %s\n", entry.PreferredDelivery))
	if entry.HostsNode {
		sb.WriteString("  Node: yes\n")
	}
	if entry.SortRank >= 0 {
		sb.WriteString(fmt.Sprintf("  Sort Rank: %d\n", entry.SortRank))
	}
	if entry.Notes != "" {
		sb.WriteString(fmt.Sprintf("  Notes: %s\n", entry.Notes))
	}
	return sb.String()
}

// SearchConversations filters conversations by name or source hash
// (case-insensitive substring match). Empty query returns all.
func SearchConversations(convs []ConversationInfo, query string) []ConversationInfo {
	if query == "" {
		return convs
	}
	q := strings.ToLower(query)
	var result []ConversationInfo
	for _, c := range convs {
		if strings.Contains(strings.ToLower(c.DisplayName), q) ||
			strings.Contains(strings.ToLower(c.SourceHash), q) {
			result = append(result, c)
		}
	}
	return result
}

// FormatConversationSummary produces a one-line summary for the detail view.
func FormatConversationSummary(conv ConversationInfo) string {
	return fmt.Sprintf(
		"[::b]%s[-]\n\nTrust: %s\nMessages: %d\nLast: %s",
		conv.DisplayName,
		FormatTrustLabel(conv.TrustLevel),
		conv.MessageCount,
		RelativeTime(conv.LastTime),
	)
}
