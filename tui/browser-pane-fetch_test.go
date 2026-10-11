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
)

// TestBrowserPaneRendersFetchedPage pins the Network page's right pane end to
// end: a navigation from the left column must reach the pane's fetch backend,
// and the markup that backend returns must appear inside the pane itself.
//
// The pane owns no fetch of its own. Its only route to the network is the
// BrowserDisplay.OnRetrieveURL seam (browser-pane.go:161 ->
// BrowserDisplay.displayURL -> OnRetrieveURL), which the wiring layer fills
// with the real backend — cmd/gonomadnet/textui.go calls wireBrowser on this
// pane's display, and wireBrowser is what assigns OnRetrieveURL. The pre-
// existing pane tests deliberately leave that seam nil, so they could only
// ever observe the loading placeholder or the disconnected view; nothing
// asserted that a returned page is actually rendered into the pane.
//
// Both entry paths are covered, because BrowserDisplay's loading -> content
// shuttle is focus-sensitive (browser-pane.go:145-157) and the two paths differ
// in exactly that: the Announce-Info Connect leaves focus on the left list,
// while a load against an already-focused pane has to carry focus across the
// fetch itself. A regression in either would leave a fetched page invisible
// while the fetch itself still reported success.
func TestBrowserPaneRendersFetchedPage(t *testing.T) {
	t.Parallel()

	const pageMarkup = ">>Fetched page\n\nHello from the remote node."

	tests := []struct {
		name    string
		connect func(t *testing.T, nd *NetworkDisplay, ann AnnounceEntry)
	}{
		{
			// Connect from Announce Info: onNavigate -> BrowserPane.LoadURL,
			// with focus left on the node list.
			name: "connect from announce info",
			connect: func(t *testing.T, nd *NetworkDisplay, ann AnnounceEntry) {
				t.Helper()
				nd.connectToNode(ann)
			},
		},
		{
			// The URL-dialog path against a pane that already holds focus.
			name: "load into the focused pane",
			connect: func(t *testing.T, nd *NetworkDisplay, ann AnnounceEntry) {
				t.Helper()
				pane := nd.BrowserPane()
				pane.app.SetFocus(pane.Widget())
				pane.LoadURL(ann.SourceHash)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
			md := app.Main
			ann := nodeAnnounce("TestNode")
			nd := NewNetworkDisplay(app, []AnnounceEntry{ann}, nil)

			// Stand in for the real RNS backend: record the request, then render
			// the page the transport would have returned.
			bd := nd.BrowserDisplay()
			var gotURL string
			bd.OnRetrieveURL = func(url string, requestData map[string]string) {
				gotURL = url
				bd.RenderPage(pageMarkup)
			}
			nd.SetNavigateCallback(func(u string) { nd.BrowserPane().LoadURL(u) })

			md.SetDisplay("network", nd.Widget())
			md.SelectPage("network")
			md.Root().SetRect(0, 0, 200, 50)
			app.SetRoot()

			tt.connect(t, nd, ann)

			if gotURL != ann.SourceHash {
				t.Errorf("the pane fetched %q, want the node hash %q", gotURL, ann.SourceHash)
			}

			joined := strings.Join(drawRows(t, nd.BrowserPane().Widget(), 80, 24), "\n")
			if !strings.Contains(joined, "Hello from the remote node.") {
				t.Errorf("the fetched page is not rendered in the pane; pane rows:\n%v", joined)
			}
			if strings.Contains(joined, "Disconnected") {
				t.Errorf("the pane still shows the disconnected view after a successful fetch:\n%v", joined)
			}
			if !strings.Contains(joined, "<"+ann.SourceHash+">") {
				t.Errorf("the pane title does not name the fetched node; pane rows:\n%v", joined)
			}
		})
	}
}
