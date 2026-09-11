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
// assets/wasm-pages/: every render increments the on-disk visit count and the
// rendered Micron markup carries the new count in decimal.

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
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x19, 0x04, 0x60,
	0x04, 0x7f, 0x7f, 0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x00, 0x01, 0x7f, 0x60,
	0x01, 0x7f, 0x01, 0x7f, 0x60, 0x02, 0x7f, 0x7f, 0x02, 0x7f, 0x7f, 0x02,
	0x1b, 0x02, 0x03, 0x72, 0x6e, 0x73, 0x06, 0x6b, 0x76, 0x5f, 0x67, 0x65,
	0x74, 0x00, 0x00, 0x03, 0x72, 0x6e, 0x73, 0x06, 0x6b, 0x76, 0x5f, 0x73,
	0x65, 0x74, 0x00, 0x00, 0x03, 0x05, 0x04, 0x01, 0x01, 0x02, 0x03, 0x05,
	0x04, 0x01, 0x01, 0x01, 0x01, 0x07, 0x2b, 0x03, 0x06, 0x6d, 0x65, 0x6d,
	0x6f, 0x72, 0x79, 0x02, 0x00, 0x10, 0x77, 0x61, 0x67, 0x6f, 0x70, 0x6c,
	0x75, 0x67, 0x69, 0x6e, 0x5f, 0x61, 0x6c, 0x6c, 0x6f, 0x63, 0x00, 0x04,
	0x0b, 0x72, 0x65, 0x6e, 0x64, 0x65, 0x72, 0x5f, 0x70, 0x61, 0x67, 0x65,
	0x00, 0x05, 0x0a, 0xd0, 0x01, 0x04, 0x05, 0x00, 0x41, 0x80, 0x04, 0x0b,
	0x05, 0x00, 0x41, 0xbc, 0x05, 0x0b, 0x05, 0x00, 0x41, 0x80, 0x20, 0x0b,
	0xbb, 0x01, 0x01, 0x04, 0x7f, 0x41, 0xc0, 0x00, 0x41, 0x04, 0x10, 0x02,
	0x41, 0x04, 0x10, 0x00, 0x1a, 0x10, 0x02, 0x28, 0x02, 0x00, 0x41, 0x01,
	0x6a, 0x21, 0x02, 0x10, 0x02, 0x20, 0x02, 0x36, 0x02, 0x00, 0x41, 0xc0,
	0x00, 0x41, 0x04, 0x10, 0x02, 0x41, 0x04, 0x10, 0x01, 0x1a, 0x20, 0x02,
	0x21, 0x03, 0x10, 0x03, 0x21, 0x05, 0x03, 0x40, 0x20, 0x05, 0x20, 0x03,
	0x41, 0x0a, 0x70, 0x41, 0x30, 0x6a, 0x3a, 0x00, 0x00, 0x20, 0x05, 0x41,
	0x01, 0x6a, 0x21, 0x05, 0x20, 0x03, 0x41, 0x0a, 0x6e, 0x21, 0x03, 0x20,
	0x03, 0x0d, 0x00, 0x0b, 0x20, 0x05, 0x10, 0x03, 0x6b, 0x21, 0x04, 0x41,
	0x80, 0x08, 0x41, 0x80, 0x01, 0x41, 0x15, 0xfc, 0x0a, 0x00, 0x00, 0x41,
	0x00, 0x21, 0x05, 0x02, 0x40, 0x03, 0x40, 0x20, 0x05, 0x20, 0x04, 0x4f,
	0x0d, 0x01, 0x41, 0x80, 0x08, 0x41, 0x15, 0x6a, 0x20, 0x05, 0x6a, 0x10,
	0x03, 0x20, 0x04, 0x41, 0x01, 0x6b, 0x20, 0x05, 0x6b, 0x6a, 0x2d, 0x00,
	0x00, 0x3a, 0x00, 0x00, 0x20, 0x05, 0x41, 0x01, 0x6a, 0x21, 0x05, 0x0c,
	0x00, 0x0b, 0x0b, 0x41, 0x80, 0x08, 0x41, 0x15, 0x6a, 0x20, 0x04, 0x6a,
	0x41, 0x80, 0x02, 0x41, 0x06, 0xfc, 0x0a, 0x00, 0x00, 0x41, 0x80, 0x08,
	0x41, 0x15, 0x20, 0x04, 0x6a, 0x41, 0x06, 0x6a, 0x0b, 0x0b, 0x32, 0x03,
	0x00, 0x41, 0xc0, 0x00, 0x0b, 0x04, 0x68, 0x69, 0x74, 0x73, 0x00, 0x41,
	0x80, 0x01, 0x0b, 0x15, 0x3e, 0x48, 0x69, 0x74, 0x20, 0x43, 0x6f, 0x75,
	0x6e, 0x74, 0x65, 0x72, 0x0a, 0x56, 0x69, 0x73, 0x69, 0x74, 0x73, 0x3a,
	0x20, 0x00, 0x41, 0x80, 0x02, 0x0b, 0x06, 0x0a, 0x2d, 0x2d, 0x2d, 0x2d,
	0x0a,
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

// renderHitCounter copies the shipped example into a fresh pages directory and
// renders it, so the KV store lands in that directory rather than next to the
// checked-in binary.
func renderHitCounter(t *testing.T, pages string) string {
	t.Helper()
	src, err := os.ReadFile(hitCounterExamplePath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	path := filepath.Join(pages, "hit-counter.wasm")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	markup, err := Render(path, PageRequest{Path: "/page/hit-counter.wasm", RequestedAt: 1730000000})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return string(markup)
}

// TestExampleHitCounterIncrements pins the counter across renders: the first
// render reports one visit, the second two, and the stored little-endian
// counter matches.
func TestExampleHitCounterIncrements(t *testing.T) {
	t.Parallel()

	pages := tempDir(t)
	for visit := 1; visit <= 3; visit++ {
		got := renderHitCounter(t, pages)
		want := fmt.Sprintf(">Hit Counter\nVisits: %v\n----\n", visit)
		if got != want {
			t.Fatalf("visit %v rendered %q, want %q", visit, got, want)
		}
	}

	stored, err := os.ReadFile(filepath.Join(pages, "data", "hit-counter", "hits"))
	if err != nil {
		t.Fatalf("ReadFile(hits): %v", err)
	}
	if len(stored) != 4 {
		t.Fatalf("stored counter = %d bytes (%v), want 4", len(stored), stored)
	}
	if got := binary.LittleEndian.Uint32(stored); got != 3 {
		t.Errorf("stored counter = %v, want 3", got)
	}
}

// TestExampleHitCounterRendersLargeCounts pins the decimal conversion for a
// count that needs several digits, which the single-digit case cannot cover.
func TestExampleHitCounterRendersLargeCounts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		prior uint32
		want  string
	}{
		{name: "single digit", prior: 8, want: "9"},
		{name: "two digits", prior: 98, want: "99"},
		{name: "three digits", prior: 999, want: "1000"},
		{name: "large", prior: 4294967294, want: "4294967295"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := tempDir(t)
			src, err := os.ReadFile(hitCounterExamplePath(t))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			path := filepath.Join(pages, "hit-counter.wasm")
			if err := os.WriteFile(path, src, 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			storeDir := filepath.Join(pages, "data", "hit-counter")
			if err := os.MkdirAll(storeDir, 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			var prior [4]byte
			binary.LittleEndian.PutUint32(prior[:], tc.prior)
			if err := os.WriteFile(filepath.Join(storeDir, "hits"), prior[:], 0o600); err != nil {
				t.Fatalf("WriteFile(hits): %v", err)
			}

			markup, err := Render(path, PageRequest{Path: "/page/hit-counter.wasm"})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			want := ">Hit Counter\nVisits: " + tc.want + "\n----\n"
			if string(markup) != want {
				t.Errorf("rendered %q, want %q", markup, want)
			}
		})
	}
}

// TestExampleHitCounterIgnoresRequestData pins that a visitor cannot inject
// markup through the page: the request payload never reaches the rendered
// output.
func TestExampleHitCounterIgnoresRequestData(t *testing.T) {
	t.Parallel()

	pages := tempDir(t)
	src, err := os.ReadFile(hitCounterExamplePath(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	path := filepath.Join(pages, "hit-counter.wasm")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	req := PageRequest{
		Path:        "/page/hit-counter.wasm",
		RequestData: map[string]string{"var_name": "`[evil`https://example.invalid]"},
		LinkID:      "aabb",
	}
	markup, err := Render(path, req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(string(markup), "evil") || strings.Contains(string(markup), "example.invalid") {
		t.Errorf("rendered markup carries the request data: %q", markup)
	}
}
