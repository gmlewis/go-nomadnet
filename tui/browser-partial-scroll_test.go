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
	"testing"
)

// scrollPage is a page long enough to scroll: a heading and n short lines.
func scrollPage(heading string, n int) string {
	lines := make([]string, 0, n+1)
	lines = append(lines, heading)
	for i := range n {
		lines = append(lines, fmt.Sprintf("line %v of the body", i))
	}
	return strings.Join(lines, "\n")
}

// TestBrowserFreshLoadStartsAtTop pins the reader-facing rule for navigation: a
// page that is being loaded starts at the top, whatever the last page's position
// was. It also pins that a preceding in-place repaint does not smuggle its keep
// request into the next load — renderPage consumes the request on the render
// that made it.
func TestBrowserFreshLoadStartsAtTop(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 40, 6)

	bd.RenderPage(scrollPage(">First", 40))
	bd.content.ScrollTo(10, 0)
	if row, _ := bd.content.GetScrollOffset(); row != 10 {
		t.Fatalf("scroll row after scrolling = %v, want 10 (test must be able to scroll)", row)
	}

	// A partial update repaint would keep the reader here; it must not carry that
	// request into the navigation below.
	bd.repaint()

	bd.RenderPage(scrollPage(">Second", 40))
	if row, _ := bd.content.GetScrollOffset(); row != 0 {
		t.Errorf("scroll row on a freshly loaded page = %v, want 0 (a new page starts at the top)", row)
	}
}

// TestBrowserRepaintKeepsReaderPlace pins the other half: a partial's content
// arriving mid-read repaints the page in place, and the reader must not be
// thrown back to the top by it. Python gets this for free — partial_received
// swaps only the partial's own pile and never rebuilds the page (Browser.py:692
// -705) — while this port rebuilds the whole page, so renderPage carries the
// offset across.
func TestBrowserRepaintKeepsReaderPlace(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 40, 6)

	bd.RenderPage(scrollPage(">Counters", 40))
	bd.content.ScrollTo(12, 0)

	bd.repaint()
	if row, _ := bd.content.GetScrollOffset(); row != 12 {
		t.Errorf("scroll row after a partial repaint = %v, want 12 (the reader keeps their place)", row)
	}

	// A second repaint keeps it too: the offset is re-read from where the reader
	// now is rather than from where the page first landed.
	bd.repaint()
	if row, _ := bd.content.GetScrollOffset(); row != 12 {
		t.Errorf("scroll row after a second repaint = %v, want 12", row)
	}
}

// TestBrowserRepaintKeepsPlaceAfterAnchorJump pins the case that makes the two
// features interact: a page opened at an anchor because its link asked for one
// (see splitLinkAnchor) must keep that section when a partial of that page lands
// afterwards. Without the offset carrying across the repaint, any page that both
// anchors into a section and embeds a partial would scroll back to the top a
// moment after arriving.
func TestBrowserRepaintKeepsPlaceAfterAnchorJump(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 40, 6)
	bd.OnRetrieveURL = func(url string, requestData map[string]string) {}

	bd.RenderPage(">Docs\n`[The Section`:/page/docs/tool.mu`anchor=the-section]\n")
	bd.HandleLink(":/page/docs/tool.mu", "anchor=the-section")

	target := ">Target\nthe quick brown fox jumps over the lazy dog and keeps on running for a while\n>>The Section\nbody text"
	bd.RenderPage(target)

	landed, _ := bd.content.GetScrollOffset()
	if landed == 0 {
		t.Fatalf("the anchored load did not jump, so the repaint cannot be exercised")
	}

	bd.repaint()
	if row, _ := bd.content.GetScrollOffset(); row != landed {
		t.Errorf("scroll row after a partial repaint = %v, want %v (the section the link asked for)", row, landed)
	}
}
