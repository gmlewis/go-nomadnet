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

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// TestDefaultIndexWasmDemoLink verifies the default index page carries a
// clickable Micron link to the gonomadnet Public Hub's dynamic-page.wasm
// demo (the link is entered from formatting mode, so the full line is
// parsed).
func TestDefaultIndexWasmDemoLink(t *testing.T) {
	t.Parallel()

	const wantURL = "c7d0e7bbd883e595f53e14fa6986188c:/page/dynamic-page.wasm"
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
func TestDefaultIndexWasmDemoLinks(t *testing.T) {
	t.Parallel()

	const hub = "c7d0e7bbd883e595f53e14fa6986188c"
	for _, tc := range []struct {
		name   string
		page   string
		label  string
		source string
	}{
		{name: "dynamic page", page: "dynamic-page.wasm", label: "View the live wasm demo", source: "assets/wasm-pages/dynamic-page.wat"},
		{name: "guestbook", page: "guestbook.wasm", label: "Sign the guestbook", source: "assets/wasm-pages/guestbook.wat"},
		{name: "hit counter", page: "hit-counter.wasm", label: "View the hit counter", source: "assets/wasm-pages/hit-counter.wat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wantURL := hub + ":/page/" + tc.page
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
