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

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

const testHash = "c388d720f56483a8dc8668ee5bea3577"

func TestParseRemoteAddress(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantPath string
		wantData map[string]string
		wantErr  bool
	}{
		{"bare hash", testHash, browser.DefaultPath, nil, false},
		{"upper bare hash", "C388D720F56483A8DC8668EE5BEA3577", browser.DefaultPath, nil, false},
		{"lxm prefix", "lxm:" + testHash, browser.DefaultPath, nil, false},
		{"lxmf prefix", "lxmf:" + testHash, browser.DefaultPath, nil, false},
		{"lxmf@ prefix", "lxmf@" + testHash, browser.DefaultPath, nil, false},
		{"lxmf scheme", "lxmf://" + testHash, browser.DefaultPath, nil, false},
		{"node scheme", "node://" + testHash, browser.DefaultPath, nil, false},
		{"nomadnetwork scheme", "nomadnetwork://" + testHash, browser.DefaultPath, nil, false},
		{"colon path", testHash + ":/page/conversations.mu", "/page/conversations.mu", nil, false},
		{"slash path", "nomadnetwork://" + testHash + "/page/conversations.mu", "/page/conversations.mu", nil, false},
		{"colon empty path defaults", testHash + ":", browser.DefaultPath, nil, false},
		{"pipe fields", testHash + ":/page/search.mu|q=robots", "/page/search.mu", map[string]string{"var_q": "robots"}, false},
		{"backtick fields", testHash + ":/page/search.mu`q=robots", "/page/search.mu", map[string]string{"var_q": "robots"}, false},
		{"relative no current", ":/page/index.mu", "", nil, true},
		{"too short", "c388d720", "", nil, true},
		{"non-hex", "zz88d720f56483a8dc8668ee5bea3577", "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, path, data, friendly, err := parseRemoteAddress(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got hash=%x path=%q friendly=%q", h, path, friendly)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			wantBytes, _ := hex.DecodeString(testHash)
			if !equalBytes(h, wantBytes) {
				t.Errorf("hash = %x, want %x", h, wantBytes)
			}
			if path != tc.wantPath {
				t.Errorf("path = %q, want %q", path, tc.wantPath)
			}
			if !equalData(data, tc.wantData) {
				t.Errorf("requestData = %v, want %v", data, tc.wantData)
			}
		})
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalData(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestRenderToJSON(t *testing.T) {
	markup := []byte(">> Title\n\nA `[Go`9388d720f56483a8dc8668ee5bea3577:/page/x.mu] link.\n\n----\n")
	var buf bytes.Buffer
	if err := renderToJSON(&buf, markup, micron.ThemeDark, "test.mu"); err != nil {
		t.Fatalf("renderToJSON: %v", err)
	}
	var doc jsonDoc
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, buf.String())
	}
	if doc.Source != "test.mu" || doc.Theme != "dark" {
		t.Errorf("source/theme = %q/%q, want test.mu/dark", doc.Source, doc.Theme)
	}
	if len(doc.Lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d", len(doc.Lines))
	}
	// Line 0 is the ">> Title" heading at level 2 with a slug anchor.
	h0 := doc.Lines[0]
	if h0.HeadingLevel != 2 {
		t.Errorf("line 0 heading_level = %d, want 2", h0.HeadingLevel)
	}
	if h0.Anchor == "" {
		t.Error("line 0 anchor is empty, want a slug")
	}
	// One of the lines carries the link span with the full URL.
	var foundLink *jsonSpan
	for i := range doc.Lines {
		for j := range doc.Lines[i].Spans {
			if l := doc.Lines[i].Spans[j].Link; l != nil {
				foundLink = &doc.Lines[i].Spans[j]
			}
		}
	}
	if foundLink == nil {
		t.Fatal("no link span in output")
	}
	if foundLink.Link.URL != "9388d720f56483a8dc8668ee5bea3577:/page/x.mu" {
		t.Errorf("link url = %q, want the page URL", foundLink.Link.URL)
	}
	// One line is a divider.
	foundDivider := false
	for _, ln := range doc.Lines {
		if ln.Divider {
			foundDivider = true
		}
	}
	if !foundDivider {
		t.Error("no divider line in output")
	}
}

func TestRenderToJSONRawBytes(t *testing.T) {
	// Bytes that would be mangled by a string round-trip through invalid UTF-8
	// must survive the -raw path unchanged. The JSON path takes []byte and
	// passes string(markup) to the renderer, so it must not panic on bad UTF-8.
	bad := []byte{0xff, 0xfe, '>', '>', ' ', 'x', 0x00}
	var buf bytes.Buffer
	if err := renderToJSON(&buf, bad, micron.ThemeDark, "bad"); err != nil {
		t.Fatalf("renderToJSON on bad bytes: %v", err)
	}
	var doc jsonDoc
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestAlignName(t *testing.T) {
	cases := map[micron.Alignment]string{
		micron.AlignLeft:   "left",
		micron.AlignCenter: "center",
		micron.AlignRight:  "right",
	}
	for in, want := range cases {
		if got := alignName(in); got != want {
			t.Errorf("alignName(%d) = %q, want %q", in, got, want)
		}
	}
}
