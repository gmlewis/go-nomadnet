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
)

// TestTruncateEllipsis pins the Go port of Python Browser.make_control_widget
// and marked_link_job URL truncation (Browser.py:514-515, 199-200):
//
//	if len(lstr) > lmax: lstr = lstr[:lmax-1] + "…"
//
// Python len() counts Unicode code points, so the helper operates on runes,
// not bytes. Golden values were captured from the source-of-truth Python
// formula (see parity skill workflow B). The result is never longer than max
// code points; "…" is one code point / one display cell.
func TestTruncateEllipsis(t *testing.T) {
	t.Parallel()

	node := "\U000f0002" // nerdfont node glyph (1 code point), as in the default config
	url := "0123456789abcdef0123456789abcdef:/page/index.mu"

	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		// URL bar: lstr = node + " " + url (49 code points), lmax = content_cols - 1.
		{"url_bar_cc46", node + " " + url, 45, "\U000f0002 0123456789abcdef0123456789abcdef:/page/ind…"},
		{"url_bar_cc47", node + " " + url, 46, "\U000f0002 0123456789abcdef0123456789abcdef:/page/inde…"},
		{"url_bar_cc80_no_trunc", node + " " + url, 79, "\U000f0002 0123456789abcdef0123456789abcdef:/page/index.mu"},
		// Footer link-peek: lstr = "Link to " + target, lmax = content_cols.
		{"footer_cc20", "Link to " + url, 20, "Link to 0123456789a…"},
		{"footer_cc30", "Link to " + url, 30, "Link to 0123456789abcdef01234…"},
		{"footer_cc80_no_trunc", "Link to " + url, 80, "Link to 0123456789abcdef0123456789abcdef:/page/index.mu"},
		// Exactly max: no truncation (Python keeps it verbatim when len == lmax).
		{"exact_fit", "abcdef", 6, "abcdef"},
		{"one_past", "abcdefg", 6, "abcde…"},
		// Edge: max too small to be meaningful — return the string unchanged.
		{"max_zero", "abc", 0, "abc"},
		{"max_one", "abc", 1, "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := truncateEllipsis(c.s, c.max); got != c.want {
				t.Errorf("truncateEllipsis(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
		})
	}
}

// TestSetURLHeaderEllipsizes pins that the browser URL bar truncates a long URL
// with the "…" ellipsis at the same column Python make_control_widget does
// (Browser.py:505-515), instead of clipping to spaces. The displayed URL
// "<hex>:/page/index.mu" round-trips through canonicalURL unchanged.
func TestSetURLHeaderEllipsizes(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = glyphsNerd
	bd := NewBrowserDisplay(app)

	url := "0123456789abcdef0123456789abcdef:/page/index.mu"
	// contentWidth() reads bd.content.GetInnerRect(); set a 46-col content area.
	bd.content.SetRect(0, 0, 46, 6)
	bd.setURLHeader(url)

	// Python at content_cols=46: node + " " + url truncated to lmax=45 →
	// "<node> 0123456789abcdef0123456789abcdef:/page/ind…"
	want := "\U000f0002 0123456789abcdef0123456789abcdef:/page/ind…"
	got := bd.urlHeader.GetText(true)
	if got != want {
		t.Errorf("URL header at width 46 = %q, want %q", got, want)
	}
	if !containsEllipsis(got) {
		t.Errorf("URL header has no ellipsis: %q", got)
	}

	// At a width larger than the URL, no truncation occurs.
	bd.content.SetRect(0, 0, 100, 6)
	bd.setURLHeader(url)
	if got, want := bd.urlHeader.GetText(true), "\U000f0002 "+url; got != want {
		t.Errorf("URL header at width 100 = %q, want %q (no truncation)", got, want)
	}
}

// TestMarkedLinkEllipsizes pins that the footer "Link to <target>" peek
// truncates with "…" at the same column Python marked_link_job does
// (Browser.py:196-200), instead of clipping to spaces.
func TestMarkedLinkEllipsizes(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)

	target := "0123456789abcdef0123456789abcdef:/page/index.mu"
	bd.content.SetRect(0, 0, 20, 6)
	bd.MarkedLink(target, "")

	// Python at content_cols=20: "Link to " + target truncated to lmax=20 →
	// "Link to 0123456789a…"
	want := "Link to 0123456789a…"
	if got := bd.footerStatus.GetText(true); got != want {
		t.Errorf("footer link-peek at width 20 = %q, want %q", got, want)
	}

	// Clearing the target restores the transfer-status widget (no "Link to").
	bd.MarkedLink("", "")
	if got := bd.footerStatus.GetText(true); got == want {
		t.Errorf("footer still shows link-peek after clear: %q", got)
	}
}

// containsEllipsis reports whether s contains the single-cell ellipsis rune.
func containsEllipsis(s string) bool {
	for _, r := range s {
		if r == '…' {
			return true
		}
	}
	return false
}
