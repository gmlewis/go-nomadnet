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
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/util"
)

// FormatAnnounceStreamRow builds the single-line announce-stream entry text,
// matching Python's AnnounceStreamEntry row (Network.py:259-390):
//
//	"{ts_string} {type_symbol} {display_str}"
//
// ts_string is the time-only "15:04:05" when the announce is on the same
// calendar day as now, otherwise the date-only "2006-01-02". type_symbol is
// the node/peer/sent glyph (pn uses the "sent" glyph). display_str is the
// sanitized (SanitizeName) or modifier-stripped (StripModifiers) name per the
// sanitize flag, truncated to 34 runes + "…"; or the full hex hash when
// showDestination is true; or "<hex>" (RNS.prettyhexrep) when the name is empty.
func FormatAnnounceStreamRow(ann AnnounceEntry, now time.Time, showDestination, sanitize bool, g GlyphSet) string {
	if g == nil {
		g = glyphsUnicode
	}
	tsString := announceTimestamp(ann.Timestamp, now)
	typeSymbol := announceTypeSymbol(ann.Type, g)
	displayStr := announceDisplayStr(ann, showDestination, sanitize)
	return tsString + " " + typeSymbol + " " + displayStr
}

// announceTimestamp returns the same-day time or other-day date, mirroring
// Python AnnounceStreamEntry's ts_string (Network.py:278-282).
func announceTimestamp(ts, now time.Time) string {
	if ts.Format("2006-01-02") == now.Format("2006-01-02") {
		return ts.Format("15:04:05")
	}
	return ts.Format("2006-01-02")
}

// announceTypeSymbol maps an announce type to its glyph, mirroring
// Network.py:383-389 (node/True→node, peer/False→peer, pn→sent).
func announceTypeSymbol(announceType string, g GlyphSet) string {
	switch announceType {
	case "peer":
		return g["peer"]
	case "pn":
		return g["sent"]
	default: // "node" (and the legacy boolean-True form)
		return g["node"]
	}
}

// announceDisplayStr computes the display name portion of the row, mirroring
// Network.py:284-292 (sanitization, show_destination hex, prettyhexrep fallback,
// 34-rune truncation).
func announceDisplayStr(ann AnnounceEntry, showDestination, sanitize bool) string {
	if showDestination {
		return truncateRunes(ann.SourceHash, 34)
	}
	name := ann.DisplayName
	if sanitize {
		if s := util.SanitizeName(&name); s != nil {
			name = *s
		}
	} else {
		if s := util.StripModifiers(&name); s != nil {
			name = *s
		}
	}
	if name == "" {
		// RNS.prettyhexrep(source_hash) = "<" + full lowercase hex + ">"
		name = "<" + ann.SourceHash + ">"
	}
	return truncateRunes(name, 34)
}

// truncateRunes returns s truncated to maxRunes code points with a trailing
// "…" when it exceeds that length, mirroring Python's display_str[:34] + "…"
// (len counts code points).
func truncateRunes(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

// AnnounceTrustStyle maps a trust level to the urwid list style + focus-style
// names used by Python AnnounceStreamEntry (Network.py:347-369). The default
// (unmatched) branch mirrors Python's else: list_untrusted / list_focus_untrusted.
func AnnounceTrustStyle(trustLevel string) (style, focusStyle string) {
	switch trustLevel {
	case "trusted":
		return "list_trusted", "list_focus_trusted"
	case "untrusted":
		return "list_untrusted", "list_focus_untrusted"
	case "unknown":
		return "list_unknown", "list_focus"
	case "warning":
		return "list_warning", "list_focus"
	default:
		return "list_untrusted", "list_focus_untrusted"
	}
}
