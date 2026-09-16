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

// TestLinkClickUpdatesURLBar covers the reported bug: following a link rendered
// the new page but left the URL bar naming the page the link was on. The
// typed-URL / Back / Forward path runs through displayURL, which calls
// setURLHeader; the link-click path (loadLinkDirect) called OnRetrieveURL
// directly and never touched the header, and refreshURLHeader only re-truncates
// the stored bd.currentURLDisp rather than deriving it from the page.
func TestLinkClickUpdatesURLBar(t *testing.T) {
	t.Parallel()

	const startURL = "aaaa1111aaaa1111aaaa1111aaaa1111"
	const targetURL = "bbbb2222bbbb2222bbbb2222bbbb2222"

	bd := NewBrowserDisplay(newTestApp())
	bd.content.SetRect(0, 0, 80, 24)
	bd.OnRetrieveURL = func(url string, _ map[string]string) {
		if url == startURL {
			bd.RenderPage(">> Start\n\nbody text\n")
		}
		// The clicked target's fetch is left in flight: the URL bar must already
		// name it, so a slow or failed fetch cannot leave the old URL showing.
	}

	bd.LoadURL(startURL)
	if got := bd.URLDisplayText(); !strings.Contains(got, startURL) {
		t.Fatalf("after LoadURL: URL bar = %q, want it to name %q", got, startURL)
	}

	bd.HandleLink(targetURL, "")
	if got := bd.URLDisplayText(); !strings.Contains(got, targetURL) {
		t.Errorf("after a link click: URL bar = %q, want the clicked target %q", got, targetURL)
	}
}
