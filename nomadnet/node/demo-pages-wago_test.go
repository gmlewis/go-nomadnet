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

// This file drives the shipped executable-page demos through the node's real
// page handler, so their state, their on-disk layout, and the form round trip
// are verified where a visitor actually reaches them rather than only against
// the sandbox directly.

package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

// demoPagesDir copies the shipped demo modules into a fresh pages directory
// and returns that directory.
func demoPagesDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := testutils.TempDir(t, "nomadnet-node-demo")
	for _, name := range names {
		src := filepath.Join("..", "..", "assets", "wasm-pages", name)
		wasm, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("ReadFile(%v): %v", src, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), wasm, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return dir
}

// TestNodeHitCounterPageCountsAcrossRequests pins the shipped hit counter
// through the node's page handler: each request renders a fresh sandbox
// instance, and the visit count survives because it lives in the page's
// on-disk store.
func TestNodeHitCounterPageCountsAcrossRequests(t *testing.T) {
	t.Parallel()

	dir := demoPagesDir(t, "hit-counter.wasm")
	n := NewNode("test-node", dir, dir, 10, 10, 10, false)
	handler := n.makePageHandler(filepath.Join(dir, "hit-counter.wasm"))

	for visit := 1; visit <= 3; visit++ {
		out := handler("/page/hit-counter.wasm", nil, []byte("req"), []byte("link"), nil, time.Unix(1730000000, 0))
		markup, ok := out.([]byte)
		if !ok {
			t.Fatalf("visit %v: handler returned %T, want []byte", visit, out)
		}
		want := ">Hit Counter\nVisits: " + string(rune('0'+visit)) + "\n----\n"
		if string(markup) != want {
			t.Fatalf("visit %v rendered %q, want %q", visit, markup, want)
		}
	}
}

// TestNodeGuestbookPageAcceptsFormSubmission pins the shipped guestbook through
// the node's page handler: a request carrying the collected form fields is
// appended, and the entry is visible on the next plain visit.
func TestNodeGuestbookPageAcceptsFormSubmission(t *testing.T) {
	t.Parallel()

	dir := demoPagesDir(t, "guestbook.wasm")
	n := NewNode("test-node", dir, dir, 10, 10, 10, false)
	handler := n.makePageHandler(filepath.Join(dir, "guestbook.wasm"))

	fields, err := rns.Pack(map[string]any{"field_name": "Glenn", "field_message": "hello"})
	mustTestErr(t, err)
	out := handler("/page/guestbook.wasm", fields, []byte("req"), []byte("link"), nil, time.Unix(1730000000, 0))
	markup, ok := out.([]byte)
	if !ok {
		t.Fatalf("handler returned %T, want []byte", out)
	}
	if !strings.Contains(string(markup), "Glenn: hello\n") {
		t.Fatalf("submission rendered %q, want the new entry", markup)
	}

	out = handler("/page/guestbook.wasm", nil, []byte("req"), []byte("link"), nil, time.Unix(1730000001, 0))
	markup, ok = out.([]byte)
	if !ok {
		t.Fatalf("handler returned %T, want []byte", out)
	}
	if !strings.Contains(string(markup), "Glenn: hello\n") {
		t.Errorf("later visit rendered %q, want the stored entry", markup)
	}
	if strings.Contains(string(markup), "No entries yet.") {
		t.Errorf("later visit rendered %q, want the entry instead of the empty notice", markup)
	}
}

// TestNodeDemoPagesMatchIndexLinks pins that the demos the default index links
// to are the ones this package can serve: the links name hit-counter.wasm and
// guestbook.wasm, and both render through the page handler.
func TestNodeDemoPagesMatchIndexLinks(t *testing.T) {
	t.Parallel()

	dir := demoPagesDir(t, "hit-counter.wasm", "guestbook.wasm")
	n := NewNode("test-node", dir, dir, 10, 10, 10, false)

	index, err := os.ReadFile(filepath.Join("..", "app", "default-index.mu"))
	if err != nil {
		t.Fatalf("ReadFile(default-index.mu): %v", err)
	}
	for _, name := range []string{"hit-counter.wasm", "guestbook.wasm"} {
		if !strings.Contains(string(index), "/page/"+name) {
			t.Errorf("default-index.mu does not link to /page/%v", name)
		}
		out := n.makePageHandler(filepath.Join(dir, name))(
			"/page/"+name, nil, []byte("req"), []byte("link"), nil, time.Unix(1730000000, 0))
		markup, ok := out.([]byte)
		if !ok {
			t.Fatalf("%v: handler returned %T, want []byte", name, out)
		}
		if len(markup) == 0 {
			t.Errorf("%v rendered empty markup", name)
		}
	}
}
