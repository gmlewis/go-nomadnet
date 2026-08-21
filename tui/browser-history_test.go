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

import "testing"

// TestHandleLinkNodeHistoryPush pins the fix for Ctrl-d (GoBack) being a no-op
// after click-navigating a nomadnetwork.node link. Python's retrieve_url appends
// to history on success (Browser.py:131-145, 216-268), so after clicking a link
// from page A to page B, Back must return to A. The Go fetch resolves via an
// async app-layer callback (RenderPage with markup, not the URL), so the tui
// layer cannot push on success; HandleLink instead pushes eagerly and the
// failure paths (NotifyLinkError for a dispatch error, SetContent for a
// fetch-fatal timeout/no-path) roll the push back, while RenderPage clears the
// pending flag on success. Before the fix HandleLink called OnRetrieveURL
// without touching history, so history stayed [A] with histIdx 0 and GoBack was
// a no-op — the user was stranded on B with Back doing nothing.
func TestHandleLinkNodeHistoryPush(t *testing.T) {
	t.Parallel()

	const startPage = ">> Start Page\n\nSome starting content here.\n"
	const targetPage = ">> Target Page\n\nTarget content rendered after fetch.\n"
	const startURL = "aabb1122aabb1122aabb1122aabb1122"
	const targetURL = "cccc3333cccc3333cccc3333cccc3333"

	// simulate drives the app-layer fetch outcome the same way
	// cmd/gonomadnet/textui.go does: success → RenderPage(markup), link-dispatch
	// error → NotifyLinkError, fetch-fatal → SetContent. Calling these
	// synchronously inside OnRetrieveURL exercises the tui-layer history logic
	// without a live RNS transport.
	tests := []struct {
		name        string
		simulate    func(bd *BrowserDisplay, url string)
		wantHistLen int // history length after the link click resolves
		wantHistIdx int // histIdx after the link click resolves
		wantCurURL  string
		wantPending bool   // pendingLinkHist after resolution (false once settled)
		wantBackURL string // CurrentURL after one GoBack ("" to skip)
	}{
		{
			name:        "success keeps pushed entry and Back returns to start",
			simulate:    func(bd *BrowserDisplay, url string) { bd.RenderPage(targetPage) },
			wantHistLen: 2,
			wantHistIdx: 1,
			wantCurURL:  targetURL,
			wantPending: false,
			wantBackURL: startURL,
		},
		{
			name:        "link-dispatch error rolls back the eager push",
			simulate:    func(bd *BrowserDisplay, url string) { bd.NotifyLinkError("could not open link") },
			wantHistLen: 1,
			wantHistIdx: 0,
			wantCurURL:  startURL,
			wantPending: false,
			wantBackURL: "",
		},
		{
			name:        "fetch-fatal rolls back the eager push",
			simulate:    func(bd *BrowserDisplay, url string) { bd.SetContent("Request timed out") },
			wantHistLen: 1,
			wantHistIdx: 0,
			wantCurURL:  startURL,
			wantPending: false,
			wantBackURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			bd := NewBrowserDisplay(app)
			bd.content.SetRect(0, 0, 80, 24)

			bd.OnRetrieveURL = func(url string, requestData map[string]string) {
				if url == startURL {
					bd.RenderPage(startPage)
					return
				}
				tt.simulate(bd, url)
			}

			// Load the starting page (LoadURL pushes startURL, displayURL fetches
			// and renders it). After this history == [startURL], histIdx == 0.
			bd.LoadURL(startURL)
			if got := bd.CurrentURL(); got != startURL {
				t.Fatalf("after LoadURL(start): CurrentURL = %q, want %q", got, startURL)
			}
			if len(bd.history) != 1 || bd.histIdx != 0 {
				t.Fatalf("after LoadURL(start): history=%v histIdx=%v, want [start] 0", bd.history, bd.histIdx)
			}

			// Click the node link. HandleLink pushes targetURL eagerly, then the
			// simulated fetch outcome either keeps it (success) or rolls it back.
			bd.HandleLink(targetURL, "")

			if len(bd.history) != tt.wantHistLen {
				t.Errorf("history len = %v (%v), want %v", len(bd.history), bd.history, tt.wantHistLen)
			}
			if bd.histIdx != tt.wantHistIdx {
				t.Errorf("histIdx = %v, want %v", bd.histIdx, tt.wantHistIdx)
			}
			if got := bd.CurrentURL(); got != tt.wantCurURL {
				t.Errorf("CurrentURL = %q, want %q", got, tt.wantCurURL)
			}
			if bd.pendingLinkHist != tt.wantPending {
				t.Errorf("pendingLinkHist = %v, want %v", bd.pendingLinkHist, tt.wantPending)
			}

			if tt.wantBackURL != "" {
				bd.GoBack()
				if got := bd.CurrentURL(); got != tt.wantBackURL {
					t.Errorf("after GoBack: CurrentURL = %q, want %q (the page the link was on)", got, tt.wantBackURL)
				}
			}
		})
	}
}

// TestHandleLinkSupersededPendingPush pins the supersession path: a second link
// click while the first fetch is still in flight rolls back the first eager
// push before pushing the second, so a stale click does not leave a dangling
// history row. OnRetrieveURL here records but does not resolve (the fetch stays
// "in flight"), so the first click leaves pendingLinkHist set; the second click
// must pop the first target and push the second.
func TestHandleLinkSupersededPendingPush(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 24)

	const startPage = ">> Start Page\n\nContent.\n"
	const startURL = "aabb1122aabb1122aabb1122aabb1122"
	const firstTarget = "cccc3333cccc3333cccc3333cccc3333"
	const secondTarget = "eeee5555eeee5555eeee5555eeee5555"

	var fetched []string
	bd.OnRetrieveURL = func(url string, requestData map[string]string) {
		fetched = append(fetched, url)
		if url == startURL {
			bd.RenderPage(startPage)
		}
		// Link targets are left unresolved (in flight) to exercise supersession.
	}

	bd.LoadURL(startURL)

	// First click pushes firstTarget and stays pending.
	bd.HandleLink(firstTarget, "")
	if len(bd.history) != 2 || bd.histIdx != 1 || bd.CurrentURL() != firstTarget {
		t.Fatalf("after first click: history=%v histIdx=%v cur=%q, want [start,first] 1 first", bd.history, bd.histIdx, bd.CurrentURL())
	}
	if !bd.pendingLinkHist {
		t.Error("after first click: pendingLinkHist = false, want true (fetch in flight)")
	}

	// Second click must roll back the first pending push, then push secondTarget.
	bd.HandleLink(secondTarget, "")
	if len(bd.history) != 2 || bd.histIdx != 1 || bd.CurrentURL() != secondTarget {
		t.Errorf("after second click: history=%v histIdx=%v cur=%q, want [start,second] 1 second", bd.history, bd.histIdx, bd.CurrentURL())
	}
	// The first target must not survive in history (rolled back, not just appended).
	for i, h := range bd.history {
		if h == firstTarget {
			t.Errorf("first target %q still in history at %v after supersession: %v", firstTarget, i, bd.history)
		}
	}
}
