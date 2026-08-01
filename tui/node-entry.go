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
	"github.com/gmlewis/go-nomadnet/nomadnet/util"
)

// FormatNodeEntryRow builds the single-line KnownNodes entry text, matching
// Python's NodeEntry row (Network.py:984-1015):
//
//	"{node_glyph} {display_str}"
//
// The node glyph is g["node"] ("Ⓝ " with a trailing space, so the row contains
// two spaces). display_str mirrors Directory.simplest_display_str(source_hash,
// san=False) (Directory.py:277-300): the display name has modifiers stripped
// (strip_modifiers, NOT sanitize_name since san=False); an empty name yields
// RNS.prettyhexrep ("<hex>"); a WARNING/UNTRUSTED entry appends " <hex>" after
// the name; TRUSTED/UNKNOWN shows the name alone. A node not present in the
// directory has no display name, so it falls into the empty-name → "<hex>" path
// — the same result as Python's not-in-directory branch.
//
// Note: Python's NodeEntry computes a per-trust-level "symbol" (cross/check/
// unknown/warning) but does NOT use it in the row text — the row always uses
// the node type glyph — so it is intentionally not reproduced here.
func FormatNodeEntryRow(node NodeEntry, g GlyphSet) string {
	if g == nil {
		g = glyphsUnicode
	}
	return g["node"] + " " + nodeDisplayStr(node)
}

// nodeDisplayStr mirrors Directory.simplest_display_str(source_hash, san=False)
// for a known-node entry (Directory.py:277-300).
func nodeDisplayStr(node NodeEntry) string {
	name := node.DisplayName
	if s := util.StripModifiers(&name); s != nil {
		name = *s
	}
	hexStr := "<" + node.SourceHash + ">" // RNS.prettyhexrep / hexrep(delimit=False)
	if name == "" {
		return hexStr
	}
	switch node.TrustLevel {
	case "warning", "untrusted":
		return name + " " + hexStr
	default: // trusted, unknown, and any other level
		return name
	}
}

// NodeTrustStyle maps a trust level to the urwid list style + focus-style names
// used by Python's NodeEntry (Network.py:993-1013). This is identical to
// AnnounceTrustStyle — NodeEntry and AnnounceStreamEntry share the same
// trust→style mapping — so this delegates to it.
func NodeTrustStyle(trustLevel string) (style, focusStyle string) {
	return AnnounceTrustStyle(trustLevel)
}
