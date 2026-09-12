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
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
)

// TestBrowserOneShotPartialFetchedOnLoad pins Python Browser.update_partials
// (Browser.py:836-846): a partial is fetched when it has never been updated
// (`not partial["updated"]`) OR when its refresh interval has elapsed. A
// partial with NO refresh interval is therefore still fetched exactly once, as
// soon as the page renders — the ⧖ placeholder is only what shows while that
// first fetch is in flight.
func TestBrowserOneShotPartialFetchedOnLoad(t *testing.T) {
	// The fetch runs on a goroutine; collect calls under a mutex.
	var mu sync.Mutex
	var fetched []string
	done := make(chan struct{})

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 12)
	bd.OnFetchPartial = func(p browser.Partial) ([]byte, error) {
		mu.Lock()
		fetched = append(fetched, p.URL)
		n := len(fetched)
		mu.Unlock()
		if n == 1 {
			close(done)
		}
		return []byte("Visits: 7"), nil
	}

	// A three-component partial declares fields; the refresh slot "0" is
	// Python's "< 1 means no refresh", so this partial must not re-fetch.
	bd.RenderPage(">Page\n`{/page/hit-counter.wasm`0`page=index.mu}\nEnd")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		mu.Lock()
		got := slicesClone(fetched)
		mu.Unlock()
		t.Fatalf("one-shot partial was never fetched (fetched = %v); Python fetches a never-updated partial once", got)
	}

	// It must be fetched exactly once, not on a 1-second fallback tick.
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	got := slicesClone(fetched)
	mu.Unlock()
	if len(got) != 1 {
		t.Errorf("fetched = %v, want exactly one fetch for a partial with no refresh interval", got)
	}
	if got[0] != "/page/hit-counter.wasm" {
		t.Errorf("fetched URL = %q, want /page/hit-counter.wasm", got[0])
	}
}

// TestBrowserRefreshingPartialStillRepeats pins that the fix does not disable
// auto-refresh: a partial declaring a refresh interval keeps re-fetching.
func TestBrowserRefreshingPartialStillRepeats(t *testing.T) {
	var mu sync.Mutex
	fetched := 0
	twice := make(chan struct{})

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 12)
	bd.OnFetchPartial = func(p browser.Partial) ([]byte, error) {
		mu.Lock()
		fetched++
		n := fetched
		mu.Unlock()
		if n == 2 {
			close(twice)
		}
		return []byte("Visits: 7"), nil
	}

	bd.RenderPage(">Page\n`{/page/hit-counter.wasm`1}\nEnd")
	select {
	case <-twice:
	case <-time.After(4 * time.Second):
		mu.Lock()
		n := fetched
		mu.Unlock()
		t.Fatalf("refreshing partial fetched %v times in 4s, want at least 2", n)
	}
	bd.StopPartials()
}

// TestBrowserPartialRequestDataPerPage pins the request data a page's inline
// partial sends, mirroring Python Browser.__get_partial_request_data
// (Browser.py:763-777): a "k=v" field becomes var_k=v, so a page can tell the
// partial which page it is counting without rendering an input field.
func TestBrowserPartialRequestDataPerPage(t *testing.T) {
	t.Parallel()

	partials := browser.ExtractPartials(">Page\n`{/page/hit-counter.wasm`0`page=index.mu}\nEnd")
	if len(partials) != 1 {
		t.Fatalf("ExtractPartials = %v partials, want 1", len(partials))
	}
	rd, linkFields := browser.PartialRequestData(partials[0].Fields)
	if len(linkFields) != 0 {
		t.Errorf("linkFields = %v, want none (the entry carries a literal value)", linkFields)
	}
	if got := rd["var_page"]; got != "index.mu" {
		t.Errorf("request data var_page = %q, want index.mu", got)
	}
	if partials[0].Refresh != 0 {
		t.Errorf("partial refresh = %v, want 0 (one-shot)", partials[0].Refresh)
	}
	if !strings.Contains(partials[0].Raw, "`{/page/hit-counter.wasm`0`page=index.mu}") {
		t.Errorf("partial raw = %q, want the full directive", partials[0].Raw)
	}
}

// slicesClone copies a string slice under the caller's lock.
func slicesClone(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}
