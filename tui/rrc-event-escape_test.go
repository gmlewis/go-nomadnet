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
	"strings"
	"testing"
	"time"
)

// TestFormatRRCEventLinesEscapesBrackets pins the /help alignment bug: tview
// treats "[room]" as a color tag and ate those tokens (and the dash column
// after them). Python urwid does not; the body must keep literal brackets by
// doubling "[" on the way into tview.
func TestFormatRRCEventLinesEscapesBrackets(t *testing.T) {
	t.Parallel()

	opts := RRCRenderOpts{Theme: ThemeDark, Glyphs: glyphsUnicode}
	ts := time.Date(2026, 9, 10, 19, 31, 12, 0, time.Local).UnixMilli()
	helpLine := "/part [room]                         - leave a room (default: current)"
	msg := ChannelMessage{Text: helpLine, IsSystem: true, TsMs: ts}

	for _, width := range []int{0, 80, 120} {
		joined := strings.Join(formatRRCEventLines(msg, opts, width), "\n")
		// tview escapes a literal "[" as "[[", so "[room]" becomes "[[room]".
		if !strings.Contains(joined, "[[room]") {
			t.Errorf("width=%v: missing escaped [[room] in %q", width, joined)
		}
		if !strings.Contains(joined, "- leave a room") {
			t.Errorf("width=%v: missing help description in %q", width, joined)
		}
		plain := strings.ReplaceAll(joined, "[[", "[")
		if strings.Contains(plain, "[[room]") {
			t.Errorf("width=%v: double-escape leaked into output %q", width, plain)
		}
	}

	// Every Python SLASH_HELP line must survive render + tview unescape.
	for _, line := range strings.Split(SlashHelpText(), "\n") {
		if line == "" {
			continue
		}
		m := ChannelMessage{Text: line, IsSystem: true, TsMs: ts}
		plain := strings.ReplaceAll(strings.Join(formatRRCEventLines(m, opts, 0), "\n"), "[[", "[")
		if !strings.Contains(plain, line) {
			t.Errorf("help line lost: %q → %q", line, plain)
		}
	}
}
