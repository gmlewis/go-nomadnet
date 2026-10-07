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
	"testing"

	"github.com/gmlewis/tview"
)

func TestValidateLXMFLinkValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		errMsg string
	}{
		{"valid 32-byte hex", "aabb1122aabb1122aabb1122aabb1122", ""},
		{"too short", "aabb1122", "invalid length"},
		{"too long", "aabb1122aabb1122aabb1122aabb1122aa", "invalid length"},
		{"invalid hex", "zzbb1122aabb1122aabb1122aabb1122", "could not decode"},
		{"empty", "", "invalid length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateLXMFLink(tt.input)
			if tt.errMsg == "" {
				if err != nil {
					t.Errorf("ValidateLXMFLink(%q) = %v, want nil", tt.input, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateLXMFLink(%q) = nil, want error containing %q", tt.input, tt.errMsg)
				}
			}
		})
	}
}

func TestBrowserHandleLXMFLink(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	var openedHash string
	bd.OnOpenLXMF = func(hash string) { openedHash = hash }

	validHash := "aabb1122aabb1122aabb1122aabb1122"
	bd.HandleLXMFLink(validHash)

	if openedHash != validHash {
		t.Errorf("openedHash = %q, want %q", openedHash, validHash)
	}
}

func TestBrowserHandleLXMFLinkInvalid(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	var lastError string
	bd.OnBrowserError = func(msg string) { lastError = msg }

	bd.HandleLXMFLink("invalid")
	if lastError == "" {
		t.Error("HandleLXMFLink with invalid input should report error")
	}
}

func TestValidateRRCLinkValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantHub  string
		wantRoom string
		wantDest string
	}{
		{"hub and room", "aabb1122aabb1122aabb1122aabb1122/general", "aabb1122aabb1122aabb1122aabb1122", "general", ""},
		{"hub with dest", "aabb1122aabb1122aabb1122aabb1122:myhub/general", "aabb1122aabb1122aabb1122aabb1122", "general", "myhub"},
		{"hub only", "aabb1122aabb1122aabb1122aabb1122", "aabb1122aabb1122aabb1122aabb1122", "", ""},
		{"hub with leading slash", "/aabb1122aabb1122aabb1122aabb1122/random", "aabb1122aabb1122aabb1122aabb1122", "random", ""},
		{"room with hash prefix", "aabb1122aabb1122aabb1122aabb1122/#random", "aabb1122aabb1122aabb1122aabb1122", "random", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub, room, dest, err := ParseRRCLink(tt.input)
			if err != nil {
				t.Fatalf("ParseRRCLink(%q) error: %v", tt.input, err)
			}
			if hub != tt.wantHub {
				t.Errorf("hub = %q, want %q", hub, tt.wantHub)
			}
			if room != tt.wantRoom {
				t.Errorf("room = %q, want %q", room, tt.wantRoom)
			}
			if dest != tt.wantDest {
				t.Errorf("dest = %q, want %q", dest, tt.wantDest)
			}
		})
	}
}

func TestValidateRRCLinkInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"invalid hex", "zzzz1122aabb1122aabb1122aabb1122/general"},
		{"too short hex", "aabb/general"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, _, err := ParseRRCLink(tt.input)
			if err == nil {
				t.Errorf("ParseRRCLink(%q) = nil error, want error", tt.input)
			}
		})
	}
}

func TestBrowserHandleRRCLink(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	var hubHash, room string
	bd.OnOpenRRC = func(hash, r string) { hubHash = hash; room = r }

	bd.HandleRRCLink("aabb1122aabb1122aabb1122aabb1122/general")

	if hubHash != "aabb1122aabb1122aabb1122aabb1122" {
		t.Errorf("hubHash = %q, want %q", hubHash, "aabb1122aabb1122aabb1122aabb1122")
	}
	if room != "general" {
		t.Errorf("room = %q, want %q", room, "general")
	}
}

func TestBrowserHandleRRCLinkInvalid(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	var lastError string
	bd.OnBrowserError = func(msg string) { lastError = msg }

	bd.HandleRRCLink("invalid")
	if lastError == "" {
		t.Error("HandleRRCLink with invalid input should report error")
	}
}

func TestBrowserHandleLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		link          string
		linkFields    string
		expectAnchor  bool
		expectRRC     bool
		expectLXMF    bool
		expectNode    bool
		expectPartial bool
		expectError   bool
		anchorName    string
		rrcHub        string
		rrcRoom       string
		lxmfHash      string
		nodeURL       string
		partialIDs    []string
	}{
		{
			name:         "anchor link",
			link:         "#intro",
			expectAnchor: true,
			anchorName:   "intro",
		},
		{
			name:         "empty anchor",
			link:         "#",
			expectAnchor: true,
			anchorName:   "",
		},
		{
			name:      "rrc:// URL",
			link:      "rrc://aabb1122aabb1122aabb1122aabb1122/general",
			expectRRC: true,
			rrcHub:    "aabb1122aabb1122aabb1122aabb1122",
			rrcRoom:   "general",
		},
		{
			name:       "lxmf@ shorthand",
			link:       "lxmf@aabb1122aabb1122aabb1122aabb1122",
			expectLXMF: true,
			lxmfHash:   "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:       "nomadnetwork.node@ explicit",
			link:       "nomadnetwork.node@aabb1122aabb1122aabb1122aabb1122",
			expectNode: true,
			nodeURL:    "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:       "nnn@ shorthand for node",
			link:       "nnn@aabb1122aabb1122aabb1122aabb1122",
			expectNode: true,
			nodeURL:    "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:      "rrc@ shorthand",
			link:      "rrc@aabb1122aabb1122aabb1122aabb1122/general",
			expectRRC: true,
			rrcHub:    "aabb1122aabb1122aabb1122aabb1122",
			rrcRoom:   "general",
		},
		{
			name:          "partial link",
			link:          "p:sidebar:header",
			expectPartial: true,
			partialIDs:    []string{"sidebar", "header"},
		},
		{
			name:       "plain hash defaults to node",
			link:       "aabb1122aabb1122aabb1122aabb1122",
			expectNode: true,
			nodeURL:    "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:       "submit link with field names routes to node",
			link:       "aabb1122aabb1122aabb1122aabb1122",
			linkFields: "query",
			expectNode: true,
			nodeURL:    "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:        "unknown destination type",
			link:        "unknown_type@abc",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			bd := NewBrowserDisplay(app)

			var gotAnchor string
			var gotRRCHub, gotRRCRoom string
			var gotLXMFHash string
			var gotNodeURL string
			var gotPartialIDs []string
			var gotError string

			bd.OnJumpAnchor = func(name string) { gotAnchor = name }
			bd.OnOpenRRC = func(hub, room string) { gotRRCHub = hub; gotRRCRoom = room }
			bd.OnOpenLXMF = func(hash string) { gotLXMFHash = hash }
			bd.OnRetrieveURL = func(url string, requestData map[string]string) { gotNodeURL = url }
			bd.OnPartialUpdate = func(ids []string) { gotPartialIDs = ids }
			bd.OnBrowserError = func(msg string) { gotError = msg }

			bd.HandleLink(tt.link, tt.linkFields)

			if tt.expectAnchor && gotAnchor != tt.anchorName {
				t.Errorf("anchor = %q, want %q", gotAnchor, tt.anchorName)
			}
			if tt.expectRRC && (gotRRCHub != tt.rrcHub || gotRRCRoom != tt.rrcRoom) {
				t.Errorf("rrc = (%q, %q), want (%q, %q)", gotRRCHub, gotRRCRoom, tt.rrcHub, tt.rrcRoom)
			}
			if tt.expectLXMF && gotLXMFHash != tt.lxmfHash {
				t.Errorf("lxmf = %q, want %q", gotLXMFHash, tt.lxmfHash)
			}
			if tt.expectNode && gotNodeURL != tt.nodeURL {
				t.Errorf("node = %q, want %q", gotNodeURL, tt.nodeURL)
			}
			if tt.expectPartial {
				if len(gotPartialIDs) != len(tt.partialIDs) {
					t.Errorf("partial IDs = %v, want %v", gotPartialIDs, tt.partialIDs)
				}
				for i, id := range gotPartialIDs {
					if id != tt.partialIDs[i] {
						t.Errorf("partial[%v] = %q, want %q", i, id, tt.partialIDs[i])
					}
				}
			}
			if tt.expectError && gotError == "" {
				t.Error("expected error, got none")
			}
			if !tt.expectError && gotError != "" {
				t.Errorf("unexpected error: %q", gotError)
			}
		})
	}
}

// TestSplitLinkAnchor pins how a link's "anchor=<name>" field is peeled out of
// the pipe-separated field list the docs generator emits. Micron links carry no
// URL fragment, so sync-go-reticulum-docs rewrites a Markdown
// "page.md#section" link into a `:/page/page.mu target plus this field; the
// name has to reach the post-load jump instead of being collected as a form
// field and sent to the page as request data.
func TestSplitLinkAnchor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		linkFields string
		wantAnchor string
		wantRest   string
	}{
		{"no fields", "", "", ""},
		{"plain form fields", "name|message", "", "name|message"},
		{"anchor only", "anchor=the-section", "the-section", ""},
		{"anchor among form fields", "name|anchor=the-section|message", "the-section", "name|message"},
		{"var field kept", "page=index.mu|anchor=sec", "sec", "page=index.mu"},
		{"empty anchor ignored", "anchor=", "", ""},
		{"last non-empty wins", "anchor=one|anchor=two", "two", ""},
		{"similar key is not an anchor", "myanchor=x", "", "myanchor=x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			anchor, rest := splitLinkAnchor(tt.linkFields)
			if anchor != tt.wantAnchor {
				t.Errorf("splitLinkAnchor(%q) anchor = %q, want %q", tt.linkFields, anchor, tt.wantAnchor)
			}
			if rest != tt.wantRest {
				t.Errorf("splitLinkAnchor(%q) rest = %q, want %q", tt.linkFields, rest, tt.wantRest)
			}
		})
	}
}

// TestBrowserLinkAnchorFieldJumpsAfterLoad pins the whole point of the
// "anchor=" field: the generated documentation's cross-page section links name
// a section of the target page, and clicking one must land on that section
// rather than at the top of the page. The field must NOT be collected as a form
// field (no var_anchor request data — Python's handling, which sends it to the
// page and loses the jump), and the jump must happen exactly once, on the render
// that lands the page, never again on a later partial refresh.
func TestBrowserLinkAnchorFieldJumpsAfterLoad(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 40, 12)

	var gotURL string
	var gotRequestData map[string]string
	bd.OnRetrieveURL = func(url string, requestData map[string]string) {
		gotURL, gotRequestData = url, requestData
	}

	// The docs page carries a link whose field names the section to open, next
	// to an ordinary var field that must still reach the target page.
	bd.RenderPage(">Docs\n`[The Section`:/page/docs/tool.mu`page=other.mu|anchor=the-section]\n")
	bd.HandleLink(":/page/docs/tool.mu", "page=other.mu|anchor=the-section")

	if gotURL != ":/page/docs/tool.mu" {
		t.Errorf("fetched URL = %q, want %q", gotURL, ":/page/docs/tool.mu")
	}
	if len(gotRequestData) != 1 || gotRequestData["var_page"] != "other.mu" {
		t.Errorf("request data = %v, want only var_page=other.mu (the anchor field is navigation, not page data)", gotRequestData)
	}

	// The fetch lands: a page whose second heading slugifies to the requested
	// anchor, behind a line long enough to wrap at width 40.
	target := ">Target\nthe quick brown fox jumps over the lazy dog and keeps on running for a while\n>>The Section\nbody text"
	bd.RenderPage(target)

	targetIdx, ok := bd.anchors.JumpTarget("the-section")
	if !ok {
		t.Fatal("anchor \"the-section\" not found in the loaded page's anchor map")
	}

	// Expected scroll row = wrapped rows of every line before the anchor, the
	// same sum JumpToAnchor computes (browser-anchor_test.go).
	const innerW = 40
	expected := 0
	for i, lt := range bd.lineTexts {
		if i >= targetIdx {
			break
		}
		expected += max(len(tview.WordWrap(lt, innerW)), 1)
	}
	if expected <= 1 {
		t.Fatalf("expected wrapped rows preceding the anchor = %v, want >1 (test must exercise wrapping)", expected)
	}
	row, _ := bd.content.GetScrollOffset()
	if row != expected {
		t.Errorf("scroll row after the anchored load = %v, want %v (the page opened at the section)", row, expected)
	}

	// The request is consumed by the render that landed the page: loading that
	// same page again opens at the top like any other fresh load, rather than
	// jumping a second time. (renderPage rebuilds the content from scratch, so
	// the scroll offset starts each render at the top.) Had the anchor survived
	// the first render, this second load would have jumped to the section again.
	bd.RenderPage(target)
	if again, _ := bd.content.GetScrollOffset(); again != 0 {
		t.Errorf("scroll row after loading the page again = %v, want 0 (the anchor request must not outlive the load it was made for)", again)
	}
}

// TestBrowserLinkAnchorDroppedWhenFetchFails pins that a requested anchor
// cannot outlive the fetch that asked for it: when the navigation fails and the
// browser shows the error body instead of a page, the anchor is dropped, so the
// next page the browser happens to render (a retry of a different page, a
// back-navigation) does not mysteriously open at a section.
func TestBrowserLinkAnchorDroppedWhenFetchFails(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 40, 12)
	bd.OnRetrieveURL = func(url string, requestData map[string]string) {}

	bd.RenderPage(">Docs\n`[The Section`:/page/docs/tool.mu`anchor=the-section]\n")
	bd.HandleLink(":/page/docs/tool.mu", "anchor=the-section")

	// The fetch-fatal path (timeout / no path) paints the error body.
	bd.SetContent("The request timed out")

	target := ">Target\nthe quick brown fox jumps over the lazy dog and keeps on running for a while\n>>The Section\nbody text"
	bd.RenderPage(target)

	if row, _ := bd.content.GetScrollOffset(); row != 0 {
		t.Errorf("scroll row = %v, want 0: a failed fetch's anchor must not survive into the next page", row)
	}
}
