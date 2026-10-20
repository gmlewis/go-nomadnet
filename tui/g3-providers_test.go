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

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
)

// TestBrowserRefreshPartialsFetches pins RefreshPartials: it fetches exactly the
// page's partials whose ids were requested (Python Browser.handle_partial_updates,
// Browser.py:823-834) and leaves the others alone.
func TestBrowserRefreshPartialsFetches(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 12)
	bd.RenderPage(">Page\n`{url1`5`pid=sb}\n`{url2`5`pid=hd}\nEnd")

	fetched := make(chan string, 4)
	bd.OnFetchPartial = func(p browser.Partial) ([]byte, error) {
		fetched <- p.ID
		return []byte("content"), nil
	}

	bd.RefreshPartials([]string{"sb"})

	select {
	case id := <-fetched:
		if id != "sb" {
			t.Errorf("fetched partial id = %q, want sb", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RefreshPartials did not fetch the requested partial")
	}
	select {
	case id := <-fetched:
		t.Errorf("RefreshPartials fetched an unrequested partial %q", id)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestBrowserRetainedLinkTeardown pins SetRetainedLink/SetRetainedLinkTeardown:
// the registered teardown receives the link and the retained slot is cleared
// when it is the retained one.
func TestBrowserRetainedLinkTeardown(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	link := new(int)
	var torn []any
	bd.SetRetainedLink(link)
	bd.SetRetainedLinkTeardown(func(l any) { torn = append(torn, l) })

	bd.teardownRetainedLink(link)
	if len(torn) != 1 || torn[0] != link {
		t.Errorf("teardown calls = %v, want the retained link", torn)
	}
	bd.destMu.Lock()
	retained := bd.retainedLink
	bd.destMu.Unlock()
	if retained != nil {
		t.Error("teardown did not clear the retained link")
	}

	// A different link is still torn down, but the slot (already empty) stays so.
	other := new(int)
	bd.teardownRetainedLink(other)
	if len(torn) != 2 || torn[1] != other {
		t.Errorf("teardown calls = %v, want the second link too", torn)
	}
}

// TestNetworkLocalPeerProviders pins the Network display's Local Peer Info
// providers: SetLocalPeerName seeds the name edit, UpdateLocalPeer refreshes the
// read-only rows, and SetLastAnnounce refreshes only the announce line
// (Network.py:1271,1036-1040).
func TestNetworkLocalPeerProviders(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	nd.SetLocalPeerName("Glenn")
	if got := nd.localPeer.Name(); got != "Glenn" {
		t.Errorf("SetLocalPeerName -> %q, want Glenn", got)
	}

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	nd.UpdateLocalPeer("<aaaa>", "<bbbb>", ts)
	if got := nd.localPeer.lxmfAddr.GetText(true); !strings.Contains(got, "<aaaa>") {
		t.Errorf("LXMF row = %q, want the new address", got)
	}
	if got := nd.localPeer.identity.GetText(true); !strings.Contains(got, "<bbbb>") {
		t.Errorf("identity row = %q, want the new hash", got)
	}
	if got := nd.localPeer.announce.GetText(true); !strings.Contains(got, PrettyDate(ts)) {
		t.Errorf("announce row = %q, want %q", got, PrettyDate(ts))
	}

	// SetLastAnnounce touches only the announce line.
	earlier := ts.Add(-time.Hour)
	nd.SetLastAnnounce(earlier)
	if got := nd.localPeer.announce.GetText(true); got != "Announced : "+PrettyDate(earlier) {
		t.Errorf("announce row = %q, want %q", got, "Announced : "+PrettyDate(earlier))
	}
	if got := nd.localPeer.Name(); got != "Glenn" {
		t.Errorf("SetLastAnnounce changed the name edit to %q", got)
	}

	// A display with no Local Peer panel ignores every provider.
	bare := &NetworkDisplay{}
	bare.SetLocalPeerName("x")
	bare.UpdateLocalPeer("a", "b", ts)
	bare.SetLastAnnounce(ts)
}
