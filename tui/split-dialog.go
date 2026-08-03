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
	"unicode/utf8"
)

// SplitDialogInfo holds the computed content for the message-too-long split
// dialog, matching the values Python's _open_split_dialog (Channels.py:889)
// derives before building its urwid widgets.
type SplitDialogInfo struct {
	BodyBytes int      // len(text.encode("utf-8"))
	Limit     int      // per-message byte limit
	Parts     []string // result of SplitMessage (nil if it cannot split)
	K         int      // number of parts
	Noun      string   // "message" (K==1) or "messages"
	Preview   string   // truncated, whitespace-flattened preview of parts[0]
	Error     string   // non-empty when the message cannot be split
}

// ComputeSplitDialog computes the message-too-long dialog content for text and
// a per-message byte limit, matching Python's _open_split_dialog
// (Channels.py:889-900). When the limit is too small to split (SplitMessage
// returns nil), only BodyBytes and Error are populated.
func ComputeSplitDialog(text string, limit int) SplitDialogInfo {
	info := SplitDialogInfo{Limit: limit, BodyBytes: len([]byte(text))}
	parts := SplitMessage(text, limit)
	if parts == nil {
		info.Error = fmt.Sprintf(
			"Message is %v bytes but per-message limit is too small to split.",
			info.BodyBytes)
		return info
	}
	info.Parts = parts
	info.K = len(parts)
	if info.K != 1 {
		info.Noun = "messages"
	} else {
		info.Noun = "message"
	}
	info.Preview = SplitDialogPreview(parts[0])
	return info
}

// SplitDialogPreview builds the part-1 preview for the split dialog, matching
// Python's preview logic (Channels.py:897-900): when the part exceeds 70 code
// points it is truncated to 70 code points with a trailing U+2026 ellipsis,
// then newlines and tabs are replaced with spaces.
func SplitDialogPreview(part0 string) string {
	preview := part0
	if utf8.RuneCountInString(preview) > 70 {
		preview = string([]rune(preview)[:70]) + "…"
	}
	preview = strings.ReplaceAll(preview, "\n", " ")
	preview = strings.ReplaceAll(preview, "\t", " ")
	return preview
}

// SplitDialogLines returns the body text lines Python's _open_split_dialog
// renders in the dialog (Channels.py:920-925), built from a computed
// SplitDialogInfo. It must not be called on an info that could not be split
// (Error != "").
func SplitDialogLines(info SplitDialogInfo) []string {
	return []string{
		fmt.Sprintf("  Message is %v bytes.", info.BodyBytes),
		fmt.Sprintf("  Hub limit  : %v bytes per message.", info.Limit),
		fmt.Sprintf("  Split into %v %s.", info.K, info.Noun),
		"  Preview of part 1:",
		"    " + info.Preview,
	}
}
