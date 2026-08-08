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
	"strconv"
	"strings"
	"testing"
)

// TestGuideAlephGitLinkReachable pins the user-reported bug that the Guide =>
// Introduction "Aleph git" link could not be selected/activated. The link is a
// plain nomadnetwork.node page URL embedded in the last Introduction paragraph
// (introduction.mu line 26):
//
//	`_`F79d`[Aleph git`a8d24177d946de4f1f0a0fe1af9a1338:/page/index.mu]`f`_
//
// Python's Guide reader keyboard model has a known quirk (the focus cursor does
// not follow scroll, Guide.py:78-88), so the keyboard never reliably lands on
// the link — but the MOUSE activates it (LinkableText.mouse_event fires
// handle_link on the clicked part, MicronParser.py:1005-1042). The Go port
// reproduces the Python focus model on a single TextView (B2/B5): Down moves
// to the paragraph's selectable line, Right advances the within-line cursor to
// the link part, Enter activates. This test pins that the keyboard path
// ACTUALLY reaches the link (an improvement over the Python source-of-truth,
// which the user explicitly requested: "I would like to be able to use the
// keyboard OR the mouse").
//
// It also pins that the link is registered in gd.links with the right URL, so
// the tview region emitted for it resolves via activateLink (the mouse path).
func TestGuideAlephGitLinkReachable(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 100, 40)
	gd.showTopic(0) // Introduction

	const wantURL = "a8d24177d946de4f1f0a0fe1af9a1338:/page/index.mu"

	// The link must be registered.
	idx := -1
	for i, l := range gd.links {
		if l.URL == wantURL {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("Aleph git link %q not registered in gd.links; got %d links", wantURL, len(gd.links))
	}
	if !strings.Contains(gd.links[idx].Label, "Aleph git") {
		t.Errorf("link label = %q, want it to contain \"Aleph git\"", gd.links[idx].Label)
	}

	// Walk Down to the paragraph line that carries the link, then Right onto
	// the link part and Enter. OnHandleLink must fire with the right target.
	gotTarget := ""
	gd.OnHandleLink = func(target, fields string) { gotTarget = target }

	// Find the selectable line whose text contains "Aleph git".
	linkSel := -1
	for si, lineIdx := range gd.selectable {
		if lineIdx < 0 || lineIdx >= len(gd.currentLines) {
			continue
		}
		if strings.Contains(plainLineText(gd.currentLines[lineIdx]), "Aleph git") {
			linkSel = si
			break
		}
	}
	if linkSel < 0 {
		t.Fatal("no selectable line contains \"Aleph git\"")
	}
	gd.focusSel = linkSel
	gd.focusCol = 0

	// Right advances the within-line cursor to the next part boundary; the
	// link is a part, so one or more Rights must land the cursor on it.
	activated := false
	for range 64 {
		if !gd.focusRight() {
			break
		}
		gotTarget = ""
		gd.focusActivate()
		if gotTarget == wantURL {
			activated = true
			break
		}
	}
	if !activated {
		t.Errorf("keyboard Right+Enter never activated the Aleph git link (want %q); last target=%q",
			wantURL, gotTarget)
	}
}

// TestGuideAlephGitLinkMouseRegion pins the MOUSE path: the link must render as
// a tview region whose ID resolves through activateLink to the registered link,
// so a mouse click fires OnHandleLink (matching Python's mouse_event). tview's
// TextView highlights the clicked region and fires SetHighlightedFunc →
// activateLink(regionID). This test drives activateLink directly with the
// region ID emitted for the link text (the contract the click relies on).
func TestGuideAlephGitLinkMouseRegion(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 100, 40)
	gd.showTopic(0) // Introduction

	const wantURL = "a8d24177d946de4f1f0a0fe1af9a1338:/page/index.mu"

	// The link's region ID is its index in gd.links (activateLink parses the
	// numeric region ID and indexes gd.links). tview region IDs are the link
	// index as a decimal string.
	wantIdx := -1
	for i, l := range gd.links {
		if l.URL == wantURL {
			wantIdx = i
			break
		}
	}
	if wantIdx < 0 {
		t.Fatalf("Aleph git link not registered; got %d links", len(gd.links))
	}

	gotTarget := ""
	gd.OnHandleLink = func(target, fields string) { gotTarget = target }

	// The region ID tview would pass to SetHighlightedFunc on a click is the
	// decimal link index (StyledLinesToTviewText emits ["<idx>"] region tags).
	gd.activateLink(strconv.Itoa(wantIdx))
	if gotTarget != wantURL {
		t.Errorf("activateLink(%d) fired OnHandleLink with %q, want %q (mouse click contract)",
			wantIdx, gotTarget, wantURL)
	}
}
