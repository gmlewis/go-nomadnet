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

//go:build wago && (linux || darwin || windows) && (amd64 || arm64)

// This file smoke-tests the shipped hit-counter example executable page from
// assets/wasm-pages/: every render increments an on-disk visit count and the
// rendered Micron markup carries the new count in decimal. The module counts
// each page separately, keyed by the var_page the embedding Micron partial
// sends, so one page can never inflate another page's counter.

package wasmpages

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hitCounterWasm is the assembled form of assets/wasm-pages/hit-counter.wat,
// pinned so a source edit that was never re-assembled is caught by a test
// rather than silently shipping a stale binary.
var hitCounterWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x2a, 0x06, 0x60,
	0x04, 0x7f, 0x7f, 0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x00, 0x01, 0x7f, 0x60,
	0x03, 0x7f, 0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x06, 0x7f, 0x7f, 0x7f, 0x7f,
	0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x01, 0x7f, 0x01, 0x7f, 0x60, 0x02, 0x7f,
	0x7f, 0x02, 0x7f, 0x7f, 0x02, 0x1b, 0x02, 0x03, 0x72, 0x6e, 0x73, 0x06,
	0x6b, 0x76, 0x5f, 0x67, 0x65, 0x74, 0x00, 0x00, 0x03, 0x72, 0x6e, 0x73,
	0x06, 0x6b, 0x76, 0x5f, 0x73, 0x65, 0x74, 0x00, 0x00, 0x03, 0x11, 0x10,
	0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
	0x02, 0x03, 0x04, 0x05, 0x05, 0x04, 0x01, 0x01, 0x01, 0x01, 0x07, 0x2b,
	0x03, 0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00, 0x10, 0x77,
	0x61, 0x67, 0x6f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x5f, 0x61, 0x6c,
	0x6c, 0x6f, 0x63, 0x00, 0x10, 0x0b, 0x72, 0x65, 0x6e, 0x64, 0x65, 0x72,
	0x5f, 0x70, 0x61, 0x67, 0x65, 0x00, 0x11, 0x0a, 0xf8, 0x04, 0x10, 0x05,
	0x00, 0x41, 0x80, 0x04, 0x0b, 0x05, 0x00, 0x41, 0xbc, 0x05, 0x0b, 0x05,
	0x00, 0x41, 0x80, 0x02, 0x0b, 0x05, 0x00, 0x41, 0xc0, 0x02, 0x0b, 0x05,
	0x00, 0x41, 0xc0, 0x00, 0x0b, 0x05, 0x00, 0x41, 0xd0, 0x00, 0x0b, 0x05,
	0x00, 0x41, 0xd0, 0x01, 0x0b, 0x05, 0x00, 0x41, 0x80, 0x08, 0x0b, 0x05,
	0x00, 0x41, 0x80, 0x01, 0x0b, 0x05, 0x00, 0x41, 0xc0, 0x01, 0x0b, 0x04,
	0x00, 0x41, 0x0f, 0x0b, 0x04, 0x00, 0x41, 0x10, 0x0b, 0x3e, 0x01, 0x02,
	0x7f, 0x41, 0x01, 0x21, 0x04, 0x41, 0x00, 0x21, 0x03, 0x02, 0x40, 0x03,
	0x40, 0x20, 0x03, 0x20, 0x02, 0x4f, 0x0d, 0x01, 0x20, 0x00, 0x20, 0x03,
	0x6a, 0x2d, 0x00, 0x00, 0x20, 0x01, 0x20, 0x03, 0x6a, 0x2d, 0x00, 0x00,
	0x47, 0x04, 0x40, 0x41, 0x00, 0x21, 0x04, 0x0c, 0x02, 0x0b, 0x20, 0x03,
	0x41, 0x01, 0x6a, 0x21, 0x03, 0x0c, 0x00, 0x0b, 0x0b, 0x20, 0x04, 0x0b,
	0xdc, 0x01, 0x01, 0x06, 0x7f, 0x41, 0x7f, 0x21, 0x0b, 0x20, 0x01, 0x20,
	0x03, 0x6b, 0x21, 0x07, 0x41, 0x00, 0x21, 0x06, 0x02, 0x40, 0x03, 0x40,
	0x20, 0x06, 0x20, 0x07, 0x4a, 0x0d, 0x01, 0x20, 0x00, 0x20, 0x06, 0x6a,
	0x20, 0x02, 0x20, 0x03, 0x10, 0x0e, 0x04, 0x40, 0x20, 0x00, 0x20, 0x06,
	0x6a, 0x20, 0x03, 0x6a, 0x21, 0x08, 0x41, 0x00, 0x21, 0x09, 0x02, 0x40,
	0x03, 0x40, 0x20, 0x08, 0x2d, 0x00, 0x00, 0x21, 0x0a, 0x20, 0x0a, 0x41,
	0x22, 0x46, 0x04, 0x40, 0x20, 0x09, 0x21, 0x0b, 0x0c, 0x02, 0x0b, 0x20,
	0x0a, 0x41, 0xdc, 0x00, 0x46, 0x04, 0x40, 0x20, 0x08, 0x41, 0x01, 0x6a,
	0x21, 0x08, 0x20, 0x08, 0x2d, 0x00, 0x00, 0x21, 0x0a, 0x20, 0x0a, 0x41,
	0x22, 0x46, 0x20, 0x0a, 0x41, 0xdc, 0x00, 0x46, 0x72, 0x20, 0x0a, 0x41,
	0x2f, 0x46, 0x72, 0x45, 0x04, 0x40, 0x0c, 0x06, 0x0b, 0x0b, 0x20, 0x0a,
	0x41, 0xe0, 0x00, 0x46, 0x20, 0x0a, 0x41, 0xdb, 0x00, 0x46, 0x72, 0x20,
	0x0a, 0x41, 0x20, 0x49, 0x72, 0x20, 0x0a, 0x41, 0x2f, 0x46, 0x72, 0x20,
	0x0a, 0x41, 0xdc, 0x00, 0x46, 0x72, 0x04, 0x40, 0x0c, 0x05, 0x0b, 0x20,
	0x09, 0x20, 0x05, 0x4f, 0x04, 0x40, 0x0c, 0x05, 0x0b, 0x20, 0x04, 0x20,
	0x09, 0x6a, 0x20, 0x0a, 0x3a, 0x00, 0x00, 0x20, 0x09, 0x41, 0x01, 0x6a,
	0x21, 0x09, 0x20, 0x08, 0x41, 0x01, 0x6a, 0x21, 0x08, 0x0c, 0x00, 0x0b,
	0x0b, 0x0c, 0x02, 0x0b, 0x20, 0x06, 0x41, 0x01, 0x6a, 0x21, 0x06, 0x0c,
	0x00, 0x0b, 0x0b, 0x20, 0x0b, 0x0b, 0x05, 0x00, 0x41, 0x80, 0x20, 0x0b,
	0x8c, 0x02, 0x01, 0x07, 0x7f, 0x10, 0x04, 0x10, 0x06, 0x41, 0x04, 0xfc,
	0x0a, 0x00, 0x00, 0x41, 0x04, 0x21, 0x03, 0x10, 0x0b, 0x21, 0x08, 0x20,
	0x00, 0x20, 0x01, 0x10, 0x07, 0x41, 0x0c, 0x10, 0x05, 0x41, 0xc0, 0x00,
	0x10, 0x0f, 0x21, 0x02, 0x02, 0x40, 0x20, 0x02, 0x41, 0x00, 0x4c, 0x0d,
	0x00, 0x10, 0x04, 0x10, 0x05, 0x20, 0x02, 0xfc, 0x0a, 0x00, 0x00, 0x20,
	0x02, 0x21, 0x03, 0x10, 0x0a, 0x21, 0x08, 0x0b, 0x10, 0x04, 0x20, 0x03,
	0x10, 0x02, 0x41, 0x04, 0x10, 0x00, 0x1a, 0x10, 0x02, 0x28, 0x02, 0x00,
	0x41, 0x01, 0x6a, 0x21, 0x04, 0x10, 0x02, 0x20, 0x04, 0x36, 0x02, 0x00,
	0x10, 0x04, 0x20, 0x03, 0x10, 0x02, 0x41, 0x04, 0x10, 0x01, 0x1a, 0x20,
	0x00, 0x20, 0x01, 0x10, 0x08, 0x41, 0x0d, 0x10, 0x05, 0x41, 0x01, 0x10,
	0x0f, 0x41, 0x00, 0x4a, 0x04, 0x40, 0x10, 0x09, 0x41, 0x00, 0x0f, 0x0b,
	0x20, 0x04, 0x21, 0x05, 0x10, 0x03, 0x21, 0x07, 0x03, 0x40, 0x20, 0x07,
	0x20, 0x05, 0x41, 0x0a, 0x70, 0x41, 0x30, 0x6a, 0x3a, 0x00, 0x00, 0x20,
	0x07, 0x41, 0x01, 0x6a, 0x21, 0x07, 0x20, 0x05, 0x41, 0x0a, 0x6e, 0x21,
	0x05, 0x20, 0x05, 0x0d, 0x00, 0x0b, 0x20, 0x07, 0x10, 0x03, 0x6b, 0x21,
	0x06, 0x10, 0x09, 0x41, 0xe0, 0x00, 0x10, 0x0d, 0xfc, 0x0a, 0x00, 0x00,
	0x41, 0x00, 0x21, 0x07, 0x02, 0x40, 0x03, 0x40, 0x20, 0x07, 0x20, 0x06,
	0x4f, 0x0d, 0x01, 0x10, 0x09, 0x10, 0x0d, 0x6a, 0x20, 0x07, 0x6a, 0x10,
	0x03, 0x20, 0x06, 0x41, 0x01, 0x6b, 0x20, 0x07, 0x6b, 0x6a, 0x2d, 0x00,
	0x00, 0x3a, 0x00, 0x00, 0x20, 0x07, 0x41, 0x01, 0x6a, 0x21, 0x07, 0x0c,
	0x00, 0x0b, 0x0b, 0x10, 0x09, 0x10, 0x0d, 0x6a, 0x20, 0x06, 0x6a, 0x20,
	0x08, 0x10, 0x0c, 0xfc, 0x0a, 0x00, 0x00, 0x10, 0x09, 0x10, 0x0d, 0x20,
	0x06, 0x6a, 0x10, 0x0c, 0x6a, 0x0b, 0x0b, 0x70, 0x06, 0x00, 0x41, 0xc0,
	0x00, 0x0b, 0x04, 0x68, 0x69, 0x74, 0x73, 0x00, 0x41, 0xd0, 0x00, 0x0b,
	0x0c, 0x22, 0x76, 0x61, 0x72, 0x5f, 0x70, 0x61, 0x67, 0x65, 0x22, 0x3a,
	0x22, 0x00, 0x41, 0xd0, 0x01, 0x0b, 0x0d, 0x22, 0x76, 0x61, 0x72, 0x5f,
	0x71, 0x75, 0x69, 0x65, 0x74, 0x22, 0x3a, 0x22, 0x00, 0x41, 0xe0, 0x00,
	0x0b, 0x10, 0x59, 0x6f, 0x75, 0x20, 0x61, 0x72, 0x65, 0x20, 0x76, 0x69,
	0x73, 0x69, 0x74, 0x6f, 0x72, 0x20, 0x00, 0x41, 0x80, 0x01, 0x0b, 0x0f,
	0x20, 0x74, 0x6f, 0x20, 0x74, 0x68, 0x69, 0x73, 0x20, 0x70, 0x61, 0x67,
	0x65, 0x2e, 0x0a, 0x00, 0x41, 0xc0, 0x01, 0x0b, 0x0f, 0x20, 0x74, 0x6f,
	0x20, 0x74, 0x68, 0x69, 0x73, 0x20, 0x73, 0x69, 0x74, 0x65, 0x2e, 0x0a,
}

// TestExampleHitCounterMatchesFixture pins the checked-in .wasm against the
// assembled .wat so a stale binary cannot ship.
func TestExampleHitCounterMatchesFixture(t *testing.T) {
	t.Parallel()

	shipped, err := os.ReadFile(hitCounterExamplePath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(shipped, hitCounterWasm) {
		t.Fatalf("assets/wasm-pages/hit-counter.wasm (%v bytes) differs from the pinned fixture (%v bytes); re-run wat2wasm hit-counter.wat -o hit-counter.wasm",
			len(shipped), len(hitCounterWasm))
	}
}

// hitCounterExamplePath is the shipped hit-counter module.
func hitCounterExamplePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "assets", "wasm-pages", "hit-counter.wasm")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("example page missing: %v", err)
	}
	return path
}

// counterPage copies the shipped example into a fresh pages directory, so its
// KV store lands there rather than next to the checked-in binary, and returns
// the module path and the pages directory.
func counterPage(t *testing.T) (path, pages string) {
	t.Helper()
	pages = tempDir(t)
	src, err := os.ReadFile(hitCounterExamplePath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	path = filepath.Join(pages, "hit-counter.wasm")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path, pages
}

// renderCounter renders the counter page with the given request data.
func renderCounter(t *testing.T, path string, requestData map[string]string) string {
	t.Helper()
	markup, err := Render(path, PageRequest{
		Path:        "/page/hit-counter.wasm",
		RequestData: requestData,
		RequestedAt: 1730000000,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return string(markup)
}

// countedPages renders the counter the way an embedding page does: the partial
// names the page being counted in its var_page request data.
func countedPage(page string) map[string]string {
	return map[string]string{"var_page": page}
}

// counterWant is the rendered line when the partial named the page being
// counted, which is the per-page counter.
func counterWant(count uint32) string {
	return fmt.Sprintf("You are visitor %v to this page.\n", count)
}

// siteWant is the rendered line when the partial named no page, so the count
// fell back to the module's own default key and counts the whole site.
func siteWant(count uint32) string {
	return fmt.Sprintf("You are visitor %v to this site.\n", count)
}

// readCounter returns the stored little-endian count for a key.
func readCounter(t *testing.T, pages, key string) uint32 {
	t.Helper()
	stored, err := os.ReadFile(filepath.Join(pages, "data", "hit-counter", key))
	if err != nil {
		t.Fatalf("ReadFile(%v): %v", key, err)
	}
	if len(stored) != 4 {
		t.Fatalf("stored counter %v = %d bytes (%v), want 4", key, len(stored), stored)
	}
	return binary.LittleEndian.Uint32(stored)
}

// TestExampleHitCounterIncrements pins the counter across renders when no page
// is named: the shared "hits" key reports one visit, then two, then three.
func TestExampleHitCounterIncrements(t *testing.T) {
	t.Parallel()

	path, pages := counterPage(t)
	for visit := uint32(1); visit <= 3; visit++ {
		if got, want := renderCounter(t, path, nil), siteWant(visit); got != want {
			t.Fatalf("visit %v rendered %q, want %q", visit, got, want)
		}
	}
	if got := readCounter(t, pages, "hits"); got != 3 {
		t.Errorf("stored counter = %v, want 3", got)
	}
}

// TestExampleHitCounterCountsPerPage pins the whole point of the per-page
// counter: each page named by a partial's var_page keeps its own independent
// count in its own store key, and a visit to one page never advances another.
func TestExampleHitCounterCountsPerPage(t *testing.T) {
	t.Parallel()

	path, pages := counterPage(t)
	for _, tc := range []struct {
		page string
		want uint32
	}{
		{page: "index.mu", want: 1},
		{page: "index.mu", want: 2},
		{page: "guestbook.mu", want: 1},
		{page: "index.mu", want: 3},
		{page: "guestbook.mu", want: 2},
		{page: "about.mu", want: 1},
		{page: "index.mu", want: 4},
	} {
		got := renderCounter(t, path, countedPage(tc.page))
		if want := counterWant(tc.want); got != want {
			t.Fatalf("page %v rendered %q, want %q", tc.page, got, want)
		}
	}

	for key, want := range map[string]uint32{"index.mu": 4, "guestbook.mu": 2, "about.mu": 1} {
		if got := readCounter(t, pages, key); got != want {
			t.Errorf("stored counter %v = %v, want %v", key, got, want)
		}
	}

	// A render with no var_page is a different counter again (the module's own
	// default "hits" key), so a site-wide count and a page's count never mix.
	if got, want := renderCounter(t, path, nil), siteWant(1); got != want {
		t.Errorf("unnamed render = %q, want %q", got, want)
	}
	if got := readCounter(t, pages, "hits"); got != 1 {
		t.Errorf("stored shared counter = %v, want 1", got)
	}
}

// TestExampleHitCounterRejectsUnsafePageName pins that a hostile var_page
// cannot escape the page's own store: request_data is attacker-controlled, so
// a name carrying a path separator or a Micron metacharacter is refused and the
// count falls back to the shared key.
func TestExampleHitCounterRejectsUnsafePageName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		page string
	}{
		{name: "path separator", page: "../../etc/passwd"},
		{name: "backslash", page: `..\windows\system32`},
		{name: "micron backtick", page: "evil`[link`https://example.invalid]"},
		{name: "empty", page: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, pages := counterPage(t)
			got := renderCounter(t, path, countedPage(tc.page))
			if want := siteWant(1); got != want {
				t.Errorf("render with page %q = %q, want %q (rejected names use the default key)", tc.page, got, want)
			}

			storeDir := filepath.Join(pages, "data", "hit-counter")
			entries, err := os.ReadDir(storeDir)
			if err != nil {
				t.Fatalf("ReadDir: %v", err)
			}
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			if len(names) != 1 || names[0] != "hits" {
				t.Errorf("store holds %v, want only [hits]", names)
			}
		})
	}
}

// TestExampleHitCounterServesBothModes pins that the two ways a page can embed
// this module stay distinct: the per-page counter (var_page names a page) and
// the keyless whole-site counter (no var_page at all, so the module falls back
// to its own default key). A page that declares both must see two independent
// numbers that never advance each other.
func TestExampleHitCounterServesBothModes(t *testing.T) {
	t.Parallel()

	path, pages := counterPage(t)

	// Interleave the two counters and assert each keeps its own series.
	for _, tc := range []struct {
		page string
		want string
	}{
		{page: "index.mu", want: counterWant(1)},
		{page: "", want: siteWant(1)},
		{page: "index.mu", want: counterWant(2)},
		{page: "", want: siteWant(2)},
		{page: "", want: siteWant(3)},
		{page: "index.mu", want: counterWant(3)},
	} {
		var requestData map[string]string
		if tc.page != "" {
			requestData = countedPage(tc.page)
		}
		if got := renderCounter(t, path, requestData); got != tc.want {
			t.Fatalf("page %q rendered %q, want %q", tc.page, got, tc.want)
		}
	}

	if got := readCounter(t, pages, "index.mu"); got != 3 {
		t.Errorf("per-page counter = %v, want 3", got)
	}
	if got := readCounter(t, pages, "hits"); got != 3 {
		t.Errorf("site-wide counter = %v, want 3", got)
	}
}

// quietPartial is the request data a page sends when it triggers the site-wide
// counter without showing it: the partial carries "quiet=1" and names no page.
func quietPartial() map[string]string {
	return map[string]string{"var_quiet": "1"}
}

// TestExampleHitCounterQuietCountsWithoutRendering pins the mode a page uses to
// bump the site-wide counter from a page that must not display it: the visit
// still counts, and the module renders nothing at all, so the embedding
// partial substitutes to no text where it was placed.
func TestExampleHitCounterQuietCountsWithoutRendering(t *testing.T) {
	t.Parallel()

	path, pages := counterPage(t)

	for visit := uint32(1); visit <= 3; visit++ {
		if got := renderCounter(t, path, quietPartial()); got != "" {
			t.Errorf("quiet render %v = %q, want no markup at all", visit, got)
		}
		if got := readCounter(t, pages, "hits"); got != visit {
			t.Errorf("site-wide count after quiet render %v = %v, want %v", visit, got, visit)
		}
	}

	// The quiet renders counted the same site-wide key an ordinary partial
	// shows, so the next visible render reports all four visits.
	if got, want := renderCounter(t, path, nil), siteWant(4); got != want {
		t.Errorf("visible render after quiet visits = %q, want %q", got, want)
	}
}

// TestExampleHitCounterQuietCountsTheNamedPage pins that quiet mode keeps every
// other behaviour of the module: a partial that names a page still counts that
// page, it just does not render the count.
func TestExampleHitCounterQuietCountsTheNamedPage(t *testing.T) {
	t.Parallel()

	path, pages := counterPage(t)

	quiet := countedPage("index.mu")
	quiet["var_quiet"] = "1"
	if got := renderCounter(t, path, quiet); got != "" {
		t.Errorf("quiet per-page render = %q, want no markup at all", got)
	}
	if got := readCounter(t, pages, "index.mu"); got != 1 {
		t.Errorf("index.mu count after one quiet visit = %v, want 1", got)
	}
	if _, err := os.Stat(filepath.Join(pages, "data", "hit-counter", "hits")); !os.IsNotExist(err) {
		t.Errorf("quiet per-page render touched the site-wide key: %v", err)
	}

	if got, want := renderCounter(t, path, countedPage("index.mu")), counterWant(2); got != want {
		t.Errorf("visible per-page render = %q, want %q", got, want)
	}
}

// TestExampleHitCounterIgnoresEmptyQuiet pins that the flag follows the same
// rule as the page name: only a value turns the rendering off, so a partial
// carrying an empty quiet field still shows its count.
func TestExampleHitCounterIgnoresEmptyQuiet(t *testing.T) {
	t.Parallel()

	path, _ := counterPage(t)

	if got, want := renderCounter(t, path, map[string]string{"var_quiet": ""}), siteWant(1); got != want {
		t.Errorf("render with an empty quiet field = %q, want %q", got, want)
	}
}

// TestExampleHitCounterRendersLargeCounts pins the decimal conversion for a
// count that needs several digits, which the single-digit case cannot cover.
func TestExampleHitCounterRendersLargeCounts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		prior uint32
		want  uint32
	}{
		{name: "single digit", prior: 8, want: 9},
		{name: "two digits", prior: 98, want: 99},
		{name: "three digits", prior: 999, want: 1000},
		{name: "large", prior: 4294967294, want: 4294967295},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, pages := counterPage(t)
			storeDir := filepath.Join(pages, "data", "hit-counter")
			if err := os.MkdirAll(storeDir, 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			var prior [4]byte
			binary.LittleEndian.PutUint32(prior[:], tc.prior)
			if err := os.WriteFile(filepath.Join(storeDir, "index.mu"), prior[:], 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			got := renderCounter(t, path, countedPage("index.mu"))
			if want := counterWant(tc.want); got != want {
				t.Errorf("rendered %q, want %q", got, want)
			}
		})
	}
}

// TestExampleHitCounterIgnoresRequestData pins that a visitor cannot inject
// markup through the page: the request payload reaches only the store key, and
// the rendered count is generated digits, never input.
func TestExampleHitCounterIgnoresRequestData(t *testing.T) {
	t.Parallel()

	path, _ := counterPage(t)
	req := map[string]string{
		"var_page": "`[evil`https://example.invalid]",
		"var_name": ">Injected Heading",
	}
	markup := renderCounter(t, path, req)
	for _, forbidden := range []string{"evil", "example.invalid", "Injected Heading"} {
		if strings.Contains(markup, forbidden) {
			t.Errorf("rendered markup carries the request data %q: %q", forbidden, markup)
		}
	}
	if want := siteWant(1); markup != want {
		t.Errorf("rendered %q, want %q", markup, want)
	}
}
