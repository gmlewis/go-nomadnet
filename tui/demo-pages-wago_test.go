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

// This file closes the loop between the shipped executable-page demos and the
// browser: it renders a demo through the loopback page path, parses the markup
// the demo produced as Micron, fills in the form that markup declares, and
// submits it back through the same path a real click would take.

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-reticulum/testutils"
)

// demoPagesDir copies the shipped demo modules into a fresh pages directory
// and returns that directory.
func demoPagesDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := testutils.TempDir(t, "nomadnet-tui-demo")
	for _, name := range names {
		src := filepath.Join("..", "assets", "wasm-pages", name)
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

// loopbackHash is the caller identity a loopback render records.
func loopbackHash() []byte {
	hash := make([]byte, 32)
	for i := range hash {
		hash[i] = byte(0x24)
	}
	return hash
}

// TestBrowserShippedHitCounterLoopback pins the shipped hit counter through
// the loopback page path the TUI uses: each render advances the on-disk count,
// so the same page served twice reports two different visits.
func TestBrowserShippedHitCounterLoopback(t *testing.T) {
	t.Parallel()

	pages := demoPagesDir(t, "hit-counter.wasm")
	first := string(browser.ServeLocalPageWithCaller(pages, "/page/hit-counter.wasm", loopbackHash(), nil))
	if first != ">Hit Counter\nVisits: 1\n----\n" {
		t.Fatalf("first render = %q, want one visit", first)
	}
	second := string(browser.ServeLocalPageWithCaller(pages, "/page/hit-counter.wasm", loopbackHash(), nil))
	if second != ">Hit Counter\nVisits: 2\n----\n" {
		t.Errorf("second render = %q, want two visits", second)
	}
}

// TestBrowserShippedGuestbookFormRoundTrip pins the shipped guestbook's own
// form: the markup the page renders declares the form the browser must fill
// in, and submitting the collected values through the loopback path stores the
// entry and renders it back.
func TestBrowserShippedGuestbookFormRoundTrip(t *testing.T) {
	t.Parallel()

	pages := demoPagesDir(t, "guestbook.wasm")
	rendered := string(browser.ServeLocalPageWithCaller(pages, "/page/guestbook.wasm", loopbackHash(), nil))
	if !strings.Contains(rendered, "No entries yet.") {
		t.Fatalf("first render = %q, want the empty notice", rendered)
	}

	// The submit link must be one the browser can follow: a relative URL that
	// reopens this page, naming the fields to collect.
	var linkFields string
	for _, n := range micron.Parse(rendered) {
		if n.Type == micron.NodeLink && n.LinkURL == ":/page/guestbook.wasm" {
			linkFields = n.LinkFields
		}
	}
	if linkFields != "name|message" {
		t.Fatalf("guestbook submit link fields = %q, want %q", linkFields, "name|message")
	}

	_, bd := newFieldTestBrowser(t, rendered)
	for _, tc := range []struct{ label, value string }{
		{label: "Your name", value: "Glenn"},
		{label: "Your message", value: "signed from the loopback browser"},
	} {
		line := findLine(bd, tc.label)
		if line < 0 {
			t.Fatalf("field line %q not found in the guestbook form", tc.label)
		}
		bd.lineFields[line][0].editor.SetText(tc.value)
	}

	requestData := bd.collectFields(linkFields)
	destHash := make([]byte, 16)
	for i := range destHash {
		destHash[i] = byte(0x11)
	}
	target := "11111111111111111111111111111111:/page/guestbook.wasm"
	_, path, merged, err := browser.ParseURL(target, nil, requestData)
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}

	signed := string(browser.ServeLocalPageWithCaller(pages, path, loopbackHash(), merged))
	want := ">Guestbook\n\nGlenn: signed from the loopback browser\n"
	if !strings.HasPrefix(signed, want) {
		t.Errorf("signed render =\n%q\nwant the new entry first:\n%q", signed, want)
	}
	if strings.Contains(signed, "No entries yet.") {
		t.Errorf("signed render = %q, want the entry instead of the empty notice", signed)
	}
}
