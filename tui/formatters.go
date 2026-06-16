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
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// RelativeTime formats a timestamp as a relative string matching
// Python's relative_time() exactly. Includes the weeks range that
// the previous Go implementation was missing.
func RelativeTime(t time.Time) string {
	delta := time.Since(t)
	switch {
	case delta < 0:
		return "just now"
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	case delta < 48*time.Hour:
		return "yesterday"
	case delta < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(delta.Hours()/24))
	case delta < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(delta.Hours()/(7*24)))
	default:
		return t.Format("2006-01-02")
	}
}

// FormatSize formats a byte count as a human-readable string.
// Matches Python's _format_size() exactly.
func FormatSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1048576:
		return fmt.Sprintf("%.1f KB", float64(size)/1024.0)
	default:
		return fmt.Sprintf("%.1f MB", float64(size)/1048576.0)
	}
}

// FormatBytes formats a byte count as a human-readable string with units.
// Matches Python's format_bytes() exactly. Uses units: bytes, KB, MB, GB, TB.
func FormatBytes(size float64) string {
	units := []string{"bytes", "KB", "MB", "GB", "TB"}
	unitIndex := 0

	for size >= 1024.0 && unitIndex < len(units)-1 {
		size /= 1024.0
		unitIndex++
	}

	if unitIndex == 0 {
		return fmt.Sprintf("%d %s", int(size), units[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", size, units[unitIndex])
}

// FormatSyncStatus produces the sync status line shown in the
// conversation list footer. Matches Python's _sync_status_line().
func FormatSyncStatus(lastSyncTime time.Time, hasSynced bool, nodeLabel string) string {
	var when string
	if !hasSynced {
		when = "never"
	} else {
		when = RelativeTime(lastSyncTime)
	}

	line := "Last sync: " + when
	if nodeLabel != "" {
		line += "  (" + nodeLabel + ")"
	}
	return line
}

// FormatHubStatus produces a formatted status line for a hub entry.
// Includes status icon, name, room count, and unread indicator.
func FormatHubStatus(hub HubEntry) string {
	icon := StatusIcon(hub.Status)
	roomCount := len(hub.Rooms)
	unread := hub.UnreadCount()

	status := "Disconnected"
	switch hub.Status {
	case HubConnected:
		status = "Connected"
	case HubReconnecting:
		status = "Reconnecting"
	}

	line := fmt.Sprintf("%s %s (%s", icon, hub.Name, status)
	if roomCount > 0 {
		line += fmt.Sprintf(", %d rooms", roomCount)
	}
	if unread > 0 {
		line += fmt.Sprintf(", %d unread", unread)
	}
	line += ")"
	return line
}

// FormatSyncProgress produces a progress bar string for sync operations.
// Returns a string like "[========    ] 40%" or "" if not syncing.
func FormatSyncProgress(progress int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	const barWidth = 20
	filled := progress * barWidth / 100
	empty := barWidth - filled

	bar := "[" + strings.Repeat("=", filled) + strings.Repeat(" ", empty) + "]"
	return fmt.Sprintf("%s %d%%", bar, progress)
}

// FormatAnnounceSummary formats a single announce for the list view.
func FormatAnnounceSummary(ann AnnounceEntry) string {
	typeIcon := "○"
	switch ann.Type {
	case "node":
		typeIcon = "Ⓝ"
	case "pn":
		typeIcon = "↑"
	case "peer":
		typeIcon = "Ⓟ"
	}
	return fmt.Sprintf("%s %s [%s] %s",
		typeIcon,
		ann.DisplayName,
		ann.Type,
		RelativeTime(ann.Timestamp))
}

// ExpandShorthands maps short destination type prefixes to their
// full names. Matches Python's Browser.expand_shorthands().
func ExpandShorthands(destType string) string {
	switch destType {
	case "nnn":
		return "nomadnetwork.node"
	case "lxmf":
		return "lxmf.delivery"
	case "rrc":
		return "rrc.hub.session"
	default:
		return destType
	}
}

// hashLen is the number of hex characters for a truncated RNS hash
// (TRUNCATED_HASHLENGTH//8 * 2 = 20 chars).
const hashLen = 20

// ParseURL parses a browser URL into destination hash (hex string)
// and path. Matches Python's Browser.parse_url() exactly.
// currentHash is the currently connected node's hash for relative URLs.
func ParseURL(url, currentHash string) (hash, path string, err error) {
	hash, path, _, _, err = ParseURLWithQuery(url, currentHash)
	return hash, path, err
}

// ParseURLWithQuery parses a browser URL into destination hash, path,
// query fields, and wildcard flag. The URL format is:
//
//	hash:path`field1=val1|field2=val2
//
// or
//
//	hash:path`*    (wildcard — collect all fields)
//
// Matches Python's Browser.parse_url() and current_url() exactly.
func ParseURLWithQuery(url, currentHash string) (hash, path string, fields map[string]string, wildcard bool, err error) {
	// Split off query part (backtick separator)
	urlPart := url
	queryPart := ""
	if idx := strings.Index(url, "`"); idx >= 0 {
		urlPart = url[:idx]
		queryPart = url[idx+1:]
	}

	components := strings.Split(urlPart, ":")

	switch len(components) {
	case 1:
		// Bare hash — use default path
		if len(components[0]) != hashLen {
			return "", "", nil, false, fmt.Errorf("malformed URL: hash must be %d hex chars, got %d", hashLen, len(components[0]))
		}
		if _, hexErr := hex.DecodeString(components[0]); hexErr != nil {
			return "", "", nil, false, fmt.Errorf("malformed URL: invalid hex in hash")
		}
		hash, path = components[0], "/page/index.mu"

	case 2:
		hashPart, pathPart := components[0], components[1]

		if hashPart == "" {
			// Relative URL — use current hash
			if currentHash == "" {
				return "", "", nil, false, fmt.Errorf("malformed URL: relative URL with no current hash")
			}
			if pathPart == "" {
				pathPart = "/page/index.mu"
			}
			hash, path = currentHash, pathPart
		} else {
			if len(hashPart) != hashLen {
				return "", "", nil, false, fmt.Errorf("malformed URL: hash must be %d hex chars, got %d", hashLen, len(hashPart))
			}
			if _, hexErr := hex.DecodeString(hashPart); hexErr != nil {
				return "", "", nil, false, fmt.Errorf("malformed URL: invalid hex in hash")
			}
			if pathPart == "" {
				pathPart = "/page/index.mu"
			}
			hash, path = hashPart, pathPart
		}

	default:
		return "", "", nil, false, fmt.Errorf("malformed URL: too many colons")
	}

	// Parse query fields
	if queryPart != "" {
		fields, wildcard = ParseQueryFields(queryPart)
	}

	return hash, path, fields, wildcard, nil
}

// ParseQueryFields parses a pipe-separated field string like
// "name=alice|role=admin" or "*" for wildcard.
func ParseQueryFields(query string) (fields map[string]string, wildcard bool) {
	if query == "*" {
		return nil, true
	}

	fields = make(map[string]string)
	for _, part := range strings.Split(query, "|") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			fields[kv[0]] = kv[1]
		} else if len(kv) == 1 && kv[0] != "" {
			fields[kv[0]] = ""
		}
	}
	return fields, false
}

// ParseLinkTarget parses a Micron link target into its type and
// hash/address. Matches the link routing logic in Python's
// Browser.handle_link() (lines 272-294).
// Returns (destinationType, hash).
func ParseLinkTarget(target string) (destType, hash string) {
	destType, hash, _, _ = ParseLinkTargetWithFields(target)
	return destType, hash
}

// ParseLinkTargetWithFields parses a Micron link target into its
// type, hash, optional field list, and wildcard flag.
// The backtick separates the link target from field specifications.
// Returns (destinationType, hash, fields, allFields).
func ParseLinkTargetWithFields(target string) (destType, hash string, fields []string, allFields bool) {
	if target == "" {
		return "", "", nil, false
	}

	// Split off field data (backtick separator)
	linkPart := target
	fieldPart := ""
	if idx := strings.Index(target, "`"); idx >= 0 {
		linkPart = target[:idx]
		fieldPart = target[idx+1:]
	}

	// Parse fields
	if fieldPart != "" {
		if fieldPart == "*" {
			allFields = true
		} else {
			fields = strings.Split(fieldPart, "|")
		}
	}

	// Anchor link
	if strings.HasPrefix(linkPart, "#") {
		return "anchor", linkPart[1:], fields, allFields
	}

	// RRC link
	if strings.HasPrefix(linkPart, "rrc://") {
		return "rrc", linkPart[6:], fields, allFields
	}

	// Partial update
	if strings.HasPrefix(linkPart, "p:") {
		return "partial", linkPart[2:], fields, allFields
	}

	// Split on @ for type prefix
	parts := strings.SplitN(linkPart, "@", 2)
	if len(parts) == 2 {
		return ExpandShorthands(parts[0]), parts[1], fields, allFields
	}

	// Plain address — treat as nomadnetwork.node
	return "nomadnetwork.node", linkPart, fields, allFields
}

// FormatAnnounceDetail formats a full announce for the detail/info view.
// Matches Python's AnnounceInfo widget layout.
func FormatAnnounceDetail(ann AnnounceEntry) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[::b]%s[-]\n", ann.DisplayName))
	sb.WriteString(fmt.Sprintf("  Type: %s\n", ann.Type))
	sb.WriteString(fmt.Sprintf("  Trust: %s\n", ann.TrustLevel))
	sb.WriteString(fmt.Sprintf("  Hash: %s\n", ann.SourceHash))
	sb.WriteString(fmt.Sprintf("  Time: %s\n", ann.Timestamp.Format("2006-01-02 15:04:05")))
	if ann.AppData != "" {
		sb.WriteString(fmt.Sprintf("  Data: %s\n", truncateStr(ann.AppData, 64)))
	}

	return sb.String()
}

// ConversationTabStats computes counts for the conversation list tabs.
// Matches Python's update_listbox trusted/untrusted counts.
type ConversationTabStats struct {
	TrustedCount    int
	UntrustedCount  int
	TrustedUnread   int
	UntrustedUnread int
}

// ComputeConversationTabStats returns tab counts for the conversation list.
func ComputeConversationTabStats(convs []ConversationInfo) ConversationTabStats {
	var stats ConversationTabStats

	for _, c := range convs {
		isTrusted := c.TrustLevel == "trusted"
		if isTrusted {
			stats.TrustedCount++
			if c.Unread || c.Failed {
				stats.TrustedUnread++
			}
		} else {
			stats.UntrustedCount++
			if c.Unread || c.Failed {
				stats.UntrustedUnread++
			}
		}
	}

	return stats
}
