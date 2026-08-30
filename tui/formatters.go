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
	"strconv"
	"strings"
	"time"
)

// RelativeTime formats a timestamp as a relative string matching
// Python's relative_time() exactly. Includes the weeks range that
// the previous Go implementation was missing.
func RelativeTime(t time.Time) string {
	return relativeTimeAt(t, time.Now())
}

// relativeTimeAt is the time-injected core of RelativeTime, exposed for tests
// so the age math is deterministic. It mirrors the reference nomadnet 1.2.8
// relative_time() at ui/textui/Conversations.py:28-50, using `now` in place of
// time.time(). Past 24h the buckets switch to a CALENDAR-day difference (the
// date of `t` subtracted from the date of `now`, both converted to the local
// date): 23:45 yesterday reads "yesterday" at 00:15 today even though the age
// is only 30 minutes, and a 34-hour-old message from the 28th reads "2d ago".
func relativeTimeAt(t, now time.Time) string {
	delta := now.Sub(t)
	switch {
	case delta < 0:
		return "just now"
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%vm ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%vh ago", int(delta.Hours()))
	}
	days := calendarDaysBetween(t, now)
	switch {
	case days <= 1:
		return "yesterday"
	case days < 7:
		return fmt.Sprintf("%vd ago", days)
	case days < 30:
		return fmt.Sprintf("%vw ago", days/7)
	default:
		return t.In(time.Local).Format("2006-01-02")
	}
}

// calendarDaysBetween returns the number of whole calendar days between the
// LOCAL date of t and the LOCAL date of now (nowDate − tDate), mirroring
// Python's (datetime.fromtimestamp(now).date() − datetime.fromtimestamp(ts)
// .date()).days from relative_time(). Civil-date arithmetic is used instead of
// dividing the age by 24h so DST transitions (23- and 25-hour days) cannot
// shift the answer.
func calendarDaysBetween(t, now time.Time) int {
	ty, tm, td := t.In(time.Local).Date()
	ny, nm, nd := now.In(time.Local).Date()
	return absoluteDays(ny, nm, nd) - absoluteDays(ty, tm, td)
}

// absoluteDays converts a civil date to a day count (days since 1970-01-01).
func absoluteDays(y int, m time.Month, d int) int {
	yy, mm := y, int(m)
	// Shift the year so March is month 0 (leap days fall at the end of the
	// year cycle, per the standard Howard Hinnant civil-date algorithm).
	if mm <= 2 {
		yy--
		mm += 12
	}
	era := yy / 400
	yoe := yy - era*400           // [0, 399]
	doy := (153*mm-457)/5 + d - 1 // [0, 365]
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// PrettyDate formats a timestamp as a relative phrase matching Python's
// pretty_date() at Network.py:1933 exactly, including its plural forms
// ("1 weeks ago", "1 months ago", "1 years ago") and the empty result for
// future timestamps. It mirrors a Python timedelta: dayDiff is the whole-day
// component of the age and secondDiff is the within-day remainder.
func PrettyDate(t time.Time) string {
	return prettyDateAt(t, time.Now())
}

// prettyDateAt is the time-injected core of PrettyDate, exposed for tests so
// the age math can be checked against captured Python reference values.
func prettyDateAt(t, now time.Time) string {
	diff := now.Sub(t)
	if diff < 0 {
		return ""
	}
	totalSec := int64(diff.Seconds())
	dayDiff := int(totalSec / 86400)
	secondDiff := int(totalSec % 86400)

	if dayDiff == 0 {
		switch {
		case secondDiff < 10:
			return "just now"
		case secondDiff < 60:
			return strconv.Itoa(secondDiff) + " seconds ago"
		case secondDiff < 120:
			return "a minute ago"
		case secondDiff < 3600:
			return strconv.Itoa(secondDiff/60) + " minutes ago"
		case secondDiff < 7200:
			return "an hour ago"
		case secondDiff < 86400:
			return strconv.Itoa(secondDiff/3600) + " hours ago"
		}
	}
	switch {
	case dayDiff == 1:
		return "Yesterday"
	case dayDiff < 7:
		return strconv.Itoa(dayDiff) + " days ago"
	case dayDiff < 31:
		return strconv.Itoa(dayDiff/7) + " weeks ago"
	case dayDiff < 365:
		return strconv.Itoa(dayDiff/30) + " months ago"
	default:
		return strconv.Itoa(dayDiff/365) + " years ago"
	}
}

// FormatSize formats a byte count as a human-readable string.
// Matches Python's _format_size() exactly.
func FormatSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%v B", size)
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
		return fmt.Sprintf("%v %v", int(size), units[unitIndex])
	}
	return fmt.Sprintf("%.1f %v", size, units[unitIndex])
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
func FormatHubStatus(hub *HubEntry) string {
	icon := StatusIcon(hub.Status)
	hub.mu.RLock()
	roomCount := len(hub.Rooms)
	hub.mu.RUnlock()
	unread := hub.UnreadCount()

	status := "Disconnected"
	switch hub.Status {
	case HubConnected:
		status = "Connected"
	case HubReconnecting:
		status = "Reconnecting"
	}

	line := fmt.Sprintf("%v %v (%v", icon, hub.Name, status)
	if roomCount > 0 {
		line += fmt.Sprintf(", %v rooms", roomCount)
	}
	if unread > 0 {
		line += fmt.Sprintf(", %v unread", unread)
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
	return fmt.Sprintf("%v %v%%", bar, progress)
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
	return fmt.Sprintf("%v %v [%v] %v",
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

// hashLen is the number of hex characters for a truncated RNS hash:
// RNS.Reticulum.TRUNCATED_HASHLENGTH is 128 bits, so a truncated hash is
// 128/8 = 16 bytes = 32 hex characters. This matches browser-links.go's
// truncatedHashHexLen and Python's parse_url, which expects
// (RNS.Reticulum.TRUNCATED_HASHLENGTH//8)*2 == 32 hex chars.
const hashLen = 32

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
	if before, after, ok := strings.Cut(url, "`"); ok {
		urlPart = before
		queryPart = after
	}

	components := strings.Split(urlPart, ":")

	switch len(components) {
	case 1:
		// Bare hash — use default path
		if len(components[0]) != hashLen {
			return "", "", nil, false, fmt.Errorf("malformed URL: hash must be %v hex chars, got %v", hashLen, len(components[0]))
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
				return "", "", nil, false, fmt.Errorf("malformed URL: hash must be %v hex chars, got %v", hashLen, len(hashPart))
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
	for part := range strings.SplitSeq(query, "|") {
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
	// Split off field data (backtick separator). Python's handle_link
	// receives the link target and its field list separately; Go combines
	// them with a backtick, so strip it here before resolving the target.
	linkPart := target
	fieldPart := ""
	if before, after, ok := strings.Cut(target, "`"); ok {
		linkPart = before
		fieldPart = after
	}

	// Parse fields
	if fieldPart != "" {
		if fieldPart == "*" {
			allFields = true
		} else {
			fields = strings.Split(fieldPart, "|")
		}
	}

	// In-document anchor link (#name or empty #) — handled locally.
	if strings.HasPrefix(linkPart, "#") {
		return "anchor", linkPart[1:], fields, allFields
	}

	// rrc://<hex>[:<dest>]/<room> URL form.
	if strings.HasPrefix(linkPart, "rrc://") {
		return "rrc", linkPart[6:], fields, allFields
	}

	// Split on @ for a type prefix. This mirrors Python's handle_link, which
	// uses an unbounded split("@") and only treats the link as typed when
	// exactly one "@" is present (len(components) == 2). This check comes
	// before the "p:" partial branch, so a target like "p:id@x" is treated as
	// a typed link rather than a partial.
	parts := strings.Split(linkPart, "@")
	if len(parts) == 2 {
		return ExpandShorthands(parts[0]), parts[1], fields, allFields
	}

	// Partial update.
	if strings.HasPrefix(linkPart, "p:") {
		return "partial", linkPart[2:], fields, allFields
	}

	// Plain address — treat as nomadnetwork.node. Python reassigns the link
	// target to components[0] (the first "@"-delimited segment), so a target
	// with two or more "@" uses only its first segment here.
	return "nomadnetwork.node", parts[0], fields, allFields
}

// FormatAnnounceDetail formats a full announce for the detail/info view.
// Matches Python's AnnounceInfo widget layout.
func FormatAnnounceDetail(ann AnnounceEntry) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "[::b]%v[-]\n", ann.DisplayName)
	fmt.Fprintf(&sb, "  Type: %v\n", ann.Type)
	fmt.Fprintf(&sb, "  Trust: %v\n", ann.TrustLevel)
	fmt.Fprintf(&sb, "  Hash: %v\n", ann.SourceHash)
	fmt.Fprintf(&sb, "  Time: %v\n", ann.Timestamp.Format("2006-01-02 15:04:05"))
	if ann.AppData != "" {
		fmt.Fprintf(&sb, "  Data: %v\n", truncateStr(ann.AppData, 64))
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
