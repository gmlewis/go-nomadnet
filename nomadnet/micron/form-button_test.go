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

package micron

import "testing"

// TestFormSubmitLinkRendersAsButton pins the reviewed, deliberate divergence
// that gives a Micron form-submit link button chrome.
//
// Python renders the label as an ordinary LinkableText (MicronParser.py:780-819:
// the label is the only thing that varies, wrapped in a LinkSpec), so the one
// control that submits a form looks exactly like the prose around it. The Go
// port brackets it — "< Sign the guestbook >" — the shape urwid's flat Button
// gives the rest of the UI, so a form page shows where to submit. Only links
// that carry link_fields (the third backtick component, i.e. form submissions)
// get the chrome; every navigation link keeps the exact Python rendering.
func TestFormSubmitLinkRendersAsButton(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		markup   string
		wantText string
		wantLink *LinkSpec
	}{
		{
			name:     "submit link gets button chrome",
			markup:   "`[Sign the guestbook`:/page/guestbook.wasm`name|message]",
			wantText: "< Sign the guestbook >",
			wantLink: &LinkSpec{Label: "Sign the guestbook", URL: ":/page/guestbook.wasm", Fields: "name|message"},
		},
		{
			name:     "plain navigation link is unchanged",
			markup:   "`[Home`:/page/index.mu]",
			wantText: "Home",
			wantLink: &LinkSpec{Label: "Home", URL: ":/page/index.mu"},
		},
		{
			name:     "label-less link falls back to the url, unbracketed",
			markup:   "`[`:/page/index.mu]",
			wantText: ":/page/index.mu",
			wantLink: &LinkSpec{Label: ":/page/index.mu", URL: ":/page/index.mu"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lines := RenderToStyledLines(tt.markup, ThemeDark)
			var span *StyledSpan
			for _, line := range lines {
				for i := range line.Spans {
					if line.Spans[i].Link != nil {
						span = &line.Spans[i]
					}
				}
			}
			if span == nil {
				t.Fatalf("no link span rendered from %q", tt.markup)
			}
			if span.Text != tt.wantText {
				t.Errorf("link span text = %q, want %q", span.Text, tt.wantText)
			}
			// The LinkSpec keeps the raw label and target: hit-testing, the
			// "Link to" footer peek, and form-field collection all resolve
			// through it, so the chrome lives only in the rendered text.
			if span.Link.Label != tt.wantLink.Label {
				t.Errorf("LinkSpec.Label = %q, want %q", span.Link.Label, tt.wantLink.Label)
			}
			if span.Link.URL != tt.wantLink.URL {
				t.Errorf("LinkSpec.URL = %q, want %q", span.Link.URL, tt.wantLink.URL)
			}
			if span.Link.Fields != tt.wantLink.Fields {
				t.Errorf("LinkSpec.Fields = %q, want %q", span.Link.Fields, tt.wantLink.Fields)
			}
		})
	}
}

// TestFormSubmitLinkButtonKeepsLineGeometry pins that the button chrome lands
// in the span the line model measures: the submit line's plain text carries the
// brackets, so the link's part cursor, the region tags the click handler
// resolves, and the wrapped-row widths all agree with what is drawn.
func TestFormSubmitLinkButtonKeepsLineGeometry(t *testing.T) {
	t.Parallel()

	markup := "Sign below:\n`[Sign the guestbook`:/page/guestbook.wasm`name|message]\n"
	lines := RenderToStyledLines(markup, ThemeDark)
	if len(lines) < 2 {
		t.Fatalf("rendered %v lines, want at least 2", len(lines))
	}
	line := lines[1]
	if len(line.Spans) != 1 {
		t.Fatalf("submit line has %v spans, want 1", len(line.Spans))
	}
	if got, want := line.Spans[0].Text, "< Sign the guestbook >"; got != want {
		t.Fatalf("submit line text = %q, want %q", got, want)
	}
	// The bracketed text is what the browser wraps, clicks, and puts the
	// hardware cursor into, so its rune count must match the span exactly.
	if got := len([]rune(line.Spans[0].Text)); got != len(FormSubmitLeft)+len("Sign the guestbook")+len(FormSubmitRight) {
		t.Errorf("rendered button width = %v runes, want the label plus its chrome", got)
	}
}
