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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

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
// and the var_page a partial sends keeps one page's count independent of every
// other page's.
func TestBrowserShippedHitCounterLoopback(t *testing.T) {
	t.Parallel()

	pages := demoPagesDir(t, "hit-counter.wasm")
	visit := func(page string) string {
		var requestData map[string]string
		if page != "" {
			requestData = map[string]string{"var_page": page}
		}
		return string(browser.ServeLocalPageWithCaller(pages, "/page/hit-counter.wasm", loopbackHash(), requestData))
	}

	for _, tc := range []struct {
		page string
		want uint32
	}{
		{page: "index.mu", want: 1},
		{page: "index.mu", want: 2},
		{page: "about.mu", want: 1},
		{page: "index.mu", want: 3},
		{page: "", want: 1},
	} {
		want := fmt.Sprintf("You are visitor %v to this site.\n", tc.want)
		if tc.page != "" {
			want = fmt.Sprintf("You are visitor %v to this page.\n", tc.want)
		}
		if got := visit(tc.page); got != want {
			t.Errorf("visit to page %q = %q, want %q", tc.page, got, want)
		}
	}
}

// TestBrowserIndexInlineCounterPartial pins the inline counter end to end,
// exactly as a browsing client experiences it: an index page declares a
// one-shot partial that names the page being counted, the browser extracts the
// directive and fetches it (from the local pages directory, since the partial
// belongs to this node), and the rendered count is substituted into the page at
// the directive's position. Two loads of the same page report two visits.
func TestBrowserIndexInlineCounterPartial(t *testing.T) {
	t.Parallel()

	pages := demoPagesDir(t, "hit-counter.wasm")
	markup := ">Index\n\nThis page has been viewed:\n\n`{:/page/hit-counter.wasm`0`page=index.mu}\n\nThanks for stopping by.\n"

	partials := browser.ExtractPartials(markup)
	if len(partials) != 1 {
		t.Fatalf("ExtractPartials = %v partials, want 1", len(partials))
	}
	localHash := loopbackHash()

	// load serves the index page and resolves its partial the way the browser
	// does. A nil transport is passed so the test proves the partial came from
	// the local node rather than a link.
	load := func(want uint32) string {
		t.Helper()
		rendered, err := browser.FetchPartial(context.Background(), nil, partials[0], browser.PartialFetch{
			CurrentDest:  localHash,
			LoopbackDest: localHash,
			PagesPath:    pages,
		})
		if err != nil {
			t.Fatalf("FetchPartial: %v", err)
		}
		return strings.Replace(markup, partials[0].Raw, strings.TrimRight(string(rendered), "\n"), 1)
	}

	for _, want := range []uint32{1, 2, 3} {
		page := load(want)
		line := fmt.Sprintf("You are visitor %v to this page.", want)
		if !strings.Contains(page, line) {
			t.Fatalf("render %v = %q, want it to contain %q", want, page, line)
		}
		// The counter replaces the directive in place, so the surrounding page
		// text still frames it.
		if strings.Contains(page, partials[0].Raw) {
			t.Errorf("render %v still carries the raw directive: %q", want, page)
		}
		if !strings.Contains(page, "Thanks for stopping by.") {
			t.Errorf("render %v lost the page text after the counter: %q", want, page)
		}
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
		{label: "Name:", value: "Glenn"},
		{label: "Message:", value: "signed from the loopback browser"},
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
	if !strings.HasPrefix(signed, ">Guestbook\n\nName: ") {
		t.Errorf("signed render =\n%q\nwant the form first, then the new entry", signed)
	}
	if strings.Contains(signed, "No entries yet.") {
		t.Errorf("signed render = %q, want the entry instead of the empty notice", signed)
	}

	// The entry is stamped with the request's unix seconds, written as the
	// timestamp construct so that the client rendering the page can show each
	// reader the instant in that reader's own timezone.
	entryLine := regexp.MustCompile("`T([0-9]+)`T Glenn: signed from the loopback browser\n")
	match := entryLine.FindStringSubmatch(signed)
	if match == nil {
		t.Fatalf("signed render =\n%q\nwant a timestamped entry line", signed)
	}
	secs, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		t.Fatalf("ParseInt(%q): %v", match[1], err)
	}

	// Parsing the served markup is what a browser does with it: the construct is
	// gone and the reader's own localized timestamp stands in its place.
	var text []string
	for _, n := range micron.Parse(signed) {
		if n.Type == micron.NodeText {
			text = append(text, n.Text)
		}
	}
	renderedPage := strings.Join(text, "")
	if strings.Contains(renderedPage, "`T") {
		t.Errorf("rendered page still carries the construct: %q", renderedPage)
	}
	if want := micron.FormatUnix(secs, "", time.Local); !strings.Contains(renderedPage, want) {
		t.Errorf("rendered page =\n%q\nwant the entry timestamped %q", renderedPage, want)
	}
	if !strings.Contains(renderedPage, "Glenn: signed from the loopback browser") {
		t.Errorf("rendered page = %q, want the stored entry", renderedPage)
	}
}
