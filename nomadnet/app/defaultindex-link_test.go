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

package app

import (
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// hubHash is the gonomadnet Public Hub's destination hash, which the default
// index page uses to address the hub's executable-page demos.
const hubHash = "c7d0e7bbd883e595f53e14fa6986188c"

// TestDefaultIndexWasmDemoLink verifies the default index page carries a
// clickable Micron link to the gonomadnet Public Hub's dynamic-page.wasm
// demo (the link is entered from formatting mode, so the full line is
// parsed).
func TestDefaultIndexWasmDemoLink(t *testing.T) {
	t.Parallel()

	const wantURL = hubHash + ":/page/dynamic-page.wasm"
	nodes := micron.Parse(defaultIndexContent)
	for _, n := range nodes {
		if n.Type == micron.NodeLink && n.LinkURL == wantURL {
			return
		}
	}
	t.Fatal("defaultIndexContent does not contain the wasm-demo link node")
}

// TestDefaultIndexWasmDemoLinks verifies that every executable-page demo the
// hub serves is reachable from the default index page by its absolute hub URL.
// The demos a visitor navigates to are links.
func TestDefaultIndexWasmDemoLinks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		page   string
		label  string
		source string
	}{
		{name: "dynamic page", page: "dynamic-page.wasm", label: "View the live wasm demo", source: "assets/wasm-pages/dynamic-page.wat"},
		{name: "guestbook", page: "guestbook.wasm", label: "Sign the guestbook", source: "assets/wasm-pages/guestbook.wat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wantURL := hubHash + ":/page/" + tc.page
			if !strings.Contains(defaultIndexContent, tc.label) {
				t.Errorf("defaultIndexContent does not carry the %q link label", tc.label)
			}
			if !strings.Contains(defaultIndexContent, tc.source) {
				t.Errorf("defaultIndexContent does not name the page source %v", tc.source)
			}
			for _, n := range micron.Parse(defaultIndexContent) {
				if n.Type == micron.NodeLink && n.LinkURL == wantURL {
					return
				}
			}
			t.Errorf("defaultIndexContent does not contain a link node with URL %v", wantURL)
		})
	}
}

// TestDefaultIndexInlineHitCounter verifies the hit counters are rendered inside
// the index page itself rather than behind a link to a separate demo page, and
// that BOTH modes ship: one partial names the page being counted (the page's own
// counter) and one names nothing at all, demonstrating the module's default
// "hits" key as a site-wide count.
//
// The partials' parsed metadata is asserted rather than their raw text, so a
// directive that parses to something other than the intended fetch cannot pass.
func TestDefaultIndexInlineHitCounter(t *testing.T) {
	t.Parallel()

	counterURL := hubHash + ":/page/hit-counter.wasm"
	partials := browser.ExtractPartials(defaultIndexContent)

	byRaw := map[string]browser.Partial{}
	for _, p := range partials {
		if p.URL == counterURL {
			byRaw[p.Raw] = p
		}
	}
	if len(byRaw) != 2 {
		t.Fatalf("defaultIndexContent declares %v hit-counter partials, want 2 (a per-page one and a keyless site-wide one); declared %v partials in total",
			len(byRaw), len(partials))
	}

	for _, tc := range []struct {
		name string
		raw  string
		page string
	}{
		{
			name: "per-page counter names index.mu",
			raw:  "`{" + counterURL + "`0`page=index.mu}",
			page: "index.mu",
		},
		{
			name: "site-wide counter names no page",
			raw:  "`{" + counterURL + "`0}",
			page: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, ok := byRaw[tc.raw]
			if !ok {
				var raws []string
				for raw := range byRaw {
					raws = append(raws, raw)
				}
				t.Fatalf("no partial with directive %q; found %v", tc.raw, raws)
			}
			if p.Refresh != 0 {
				t.Errorf("partial refresh = %v, want 0 (a hit counter counts arrivals, not seconds)", p.Refresh)
			}

			// Both directives carry only literal values, so no named form
			// field is collected. A partial with no fields segment at all
			// still yields Python's single empty field entry
			// (parse_partial's "".split("|")), which no form widget can match
			// and which PartialRequestData drops from the request data.
			requestData, linkFields := browser.PartialRequestData(p.Fields)
			for _, f := range linkFields {
				if f != "" {
					t.Errorf("partial link fields = %v, want no named form field", linkFields)
				}
			}
			if got := requestData["var_page"]; got != tc.page {
				t.Errorf("partial var_page = %q, want %q", got, tc.page)
			}
		})
	}

	// The counters are not reachable as links any more: the demo page they used
	// to link to is rendered in place instead.
	for _, n := range micron.Parse(defaultIndexContent) {
		if n.Type == micron.NodeLink && n.LinkURL == counterURL {
			t.Error("defaultIndexContent still links to the hit counter as a separate page")
		}
	}

	if !strings.Contains(defaultIndexContent, "assets/wasm-pages/hit-counter.wat") {
		t.Error("defaultIndexContent does not name the page source assets/wasm-pages/hit-counter.wat")
	}
}
