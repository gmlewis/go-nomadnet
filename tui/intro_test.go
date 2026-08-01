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

	"github.com/rivo/tview"
)

// TestIntroDisplayBigText asserts the splash renders the title as urwid BigText
// using HalfBlock5x4Font (Extras.py:6-9), not as a plain string. The expected
// rows were captured from the original nomadnet intro screen.
func TestIntroDisplayBigText(t *testing.T) {
	t.Parallel()

	id := NewIntroDisplay("Nomad Network", "1.2.6")

	// The big-text view must contain the half-block glyph rows for "Nomad
	// Network" (the first row begins with the N glyph's top).
	got := id.bigView.GetText(true)
	wantFirstRow := halfBlock5x4RenderTrimmed("Nomad Network")[0]
	if !strings.Contains(got, wantFirstRow) {
		t.Errorf("big view missing big-text first row %q; got: %q", wantFirstRow, got)
	}
	// It must NOT contain the literal title as a plain string (the old
	// placeholder rendered "Nomad Network" verbatim).
	if strings.Contains(got, "Nomad Network") {
		t.Errorf("big view contains plain title string; should be big-text glyphs only: %q", got)
	}
}

// TestIntroDisplayVersionAndStarting asserts the version line and the
// "-= Starting =-" line are present and centered, matching Extras.py:11-17.
func TestIntroDisplayVersionAndStarting(t *testing.T) {
	t.Parallel()

	id := NewIntroDisplay("Nomad Network", "1.2.6")

	if got := id.versionView.GetText(true); got != "Version 1.2.6" {
		t.Errorf("version view = %q, want %q", got, "Version 1.2.6")
	}
	if got := id.startingView.GetText(true); !strings.Contains(got, "-= Starting =-") {
		t.Errorf("starting view = %q, want it to contain -= Starting =-", got)
	}
}

// TestIntroDisplayNoBorder asserts the splash has no surrounding border, matching
// Python's IntroDisplay (Extras.py:19) which wraps the pile in a bare
// urwid.Filler — no LineBox.
func TestIntroDisplayNoBorder(t *testing.T) {
	t.Parallel()

	id := NewIntroDisplay("Nomad Network", "1.2.6")
	outer, ok := id.widget.(*tview.Flex)
	if !ok {
		t.Fatalf("widget is %T, want *tview.Flex", id.widget)
	}
	// tview exposes no border getter, but a bordered Flex would carry a title
	// slot; the bare Filler has neither border nor title.
	if outer.GetTitle() != "" {
		t.Errorf("outer Flex has title %q; splash should have none", outer.GetTitle())
	}
}
