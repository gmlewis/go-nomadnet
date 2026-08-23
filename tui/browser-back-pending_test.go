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
	"testing"
)

// TestGoBackNoopWhileLinkFetchPending verifies that GoBack is a no-op
// while a link click's fetch is still in flight, matching Python's
// Browser.back() guard: `if not self.history_inc and not self.history_dec`
// (Browser.py:1079). Before the fix, GoBack would decrement histIdx and
// call displayURL, which rolled back the pending link push and started a
// new fetch — but the original link fetch was still in flight and would
// overwrite the back-navigated page when it completed.
func TestGoBackNoopWhileLinkFetchPending(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 24)

	const startPage = ">> Start Page\n\nContent.\n"
	const startURL = "aabb1122aabb1122aabb1122aabb1122"
	const targetURL = "cccc3333cccc3333cccc3333cccc3333"

	bd.OnRetrieveURL = func(url string, requestData map[string]string) {
		if url == startURL {
			bd.RenderPage(startPage)
		}
		// Link target fetch is left unresolved (in flight).
	}

	bd.LoadURL(startURL)
	if bd.CurrentURL() != startURL {
		t.Fatalf("after LoadURL: CurrentURL=%q, want %q", bd.CurrentURL(), startURL)
	}

	// Click the link — pushes targetURL eagerly, fetch stays in flight.
	bd.HandleLink(targetURL, "")
	if !bd.pendingLinkHist {
		t.Fatal("after HandleLink: pendingLinkHist=false, want true (fetch in flight)")
	}
	if bd.histIdx != 1 {
		t.Fatalf("after HandleLink: histIdx=%d, want 1", bd.histIdx)
	}

	// Press Back while the link fetch is still pending — must be a no-op.
	bd.GoBack()

	// History and histIdx must be unchanged (the pending push is NOT
	// rolled back by GoBack; the user must wait for the fetch to complete).
	if bd.histIdx != 1 {
		t.Errorf("after GoBack while pending: histIdx=%d, want 1 (no-op)", bd.histIdx)
	}
	if len(bd.history) != 2 {
		t.Errorf("after GoBack while pending: history len=%d, want 2 (no-op)", len(bd.history))
	}
	if bd.CurrentURL() != targetURL {
		t.Errorf("after GoBack while pending: CurrentURL=%q, want %q (no-op)", bd.CurrentURL(), targetURL)
	}
	if !bd.pendingLinkHist {
		t.Error("after GoBack while pending: pendingLinkHist=false, want true (still in flight)")
	}
}
