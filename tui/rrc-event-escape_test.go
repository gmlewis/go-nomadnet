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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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
		// tview.Escape("[room]") → "[room[]"
		if !strings.Contains(joined, "[room[]") {
			t.Errorf("width=%v: missing tview-escaped [room[] in %q", width, joined)
		}
		if !strings.Contains(joined, "- leave a room") {
			t.Errorf("width=%v: missing help description in %q", width, joined)
		}
		// At width 0 the line is one run; at 80 urwid may split
		// "(default: current)" across continuation lines.
		if width == 0 && !strings.Contains(joined, "(default: current)") {
			t.Errorf("width=%v: missing trailing parenthetical in %q", width, joined)
		}
	}

	// Every Python SLASH_HELP line must survive render + tview unescape.
	for line := range strings.SplitSeq(SlashHelpText(), "\n") {
		if line == "" {
			continue
		}
		m := ChannelMessage{Text: line, IsSystem: true, TsMs: ts}
		rendered := strings.Join(formatRRCEventLines(m, opts, 0), "\n")
		plain := tview.Unescape(rendered)
		if !strings.Contains(plain, line) {
			t.Errorf("help line lost: %q → %q", line, plain)
		}
	}
}

// TestSlashHelpRendersThroughTviewEngine draws /help through a real tview
// TextView (SetDynamicColors) on a simulation screen and reads the painted
// cells. String-level "[[" / tview.Escape assertions are not enough — the
// fork's tag parser can still eat "room]" after a half-open escape (the
// raspberrypi/local capture). Only a full render proves the user sees
// "/part [room]" aligned like Python nomadnet.
func TestSlashHelpRendersThroughTviewEngine(t *testing.T) {
	t.Parallel()

	opts := RRCRenderOpts{Theme: ThemeDark, Glyphs: glyphsUnicode}
	ts := time.Date(2026, 9, 10, 19, 39, 40, 0, time.Local).UnixMilli()
	const width = 200
	const height = 40

	var tagged strings.Builder
	for line := range strings.SplitSeq(SlashHelpText(), "\n") {
		if line == "" {
			continue
		}
		m := ChannelMessage{Text: line, IsSystem: true, TsMs: ts}
		for _, l := range formatRRCEventLines(m, opts, width) {
			tagged.WriteString(l)
			tagged.WriteByte('\n')
		}
	}

	tv := tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetWordWrap(false)
	tv.SetText(tagged.String())
	tv.SetRect(0, 0, width, height)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	tv.Draw(screen)
	screen.Show()

	var painted strings.Builder
	for row := range height {
		for col := range width {
			main, _, _ := screen.Get(col, row)
			painted.WriteString(main)
		}
		painted.WriteByte('\n')
	}
	visible := painted.String()

	for _, want := range []string{
		"/part [room]",
		"- leave a room (default: current)",
		"/who [room]",
		"/topic <room> [text]",
		"/mode <room> [+-flags] [arg]",
		"/ban <room> add|del|list [target]",
	} {
		if !strings.Contains(visible, want) {
			t.Errorf("tview engine did not paint %q\npainted=\n%v", want, visible)
		}
	}
	// No half-open escape leftovers in the visible text.
	if strings.Contains(visible, "[room[]") || strings.Contains(visible, "[[room") {
		t.Errorf("escape tokens leaked into painted cells:\n%v", visible)
	}
}
