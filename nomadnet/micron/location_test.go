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

import (
	"strings"
	"testing"

	"github.com/gmlewis/go-reticulum/geo"
)

// The certified reference vectors below are the ones published with the
// extension. The distances and bearings were produced with the formulas in the
// specification and a spherical Earth of radius 6,371,000 m.
var (
	// googleplex is the reader position in the first two vectors.
	googleplex = geo.LatLng{Lat: 37.4220, Lng: -122.0841}
	// london is the reader position in the second vector.
	london = geo.LatLng{Lat: 51.5000, Lng: -0.1200}
	// sydney is the reader position in the third vector.
	sydney = geo.LatLng{Lat: -33.8568, Lng: 151.2153}
)

// TestRenderLocationCertifiedVectors pins the reference vectors from the
// extension specification: a reader at a known position sees the code, the
// great-circle distance, and the compass bearing.
func TestRenderLocationCertifiedVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		viewer Viewer
		markup string
		want   string
	}{
		{
			"googleplex reading the eiffel tower",
			Viewer{Pos: googleplex, Known: true},
			"`L8FW4V75V+8R`L",
			"8FW4V75V+8R (8967 km, bearing 033° NNE)",
		},
		{
			"london reading the eiffel tower",
			Viewer{Pos: london, Known: true},
			"`L8FW4V75V+8R`L",
			"8FW4V75V+8R (340.3 km, bearing 149° SSE)",
		},
		{
			"sydney reading the googleplex",
			Viewer{Pos: sydney, Known: true},
			"`L849VCWC8+R9`L",
			"849VCWC8+R9 (11953 km, bearing 056° NE)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := styledText(tc.markup, tc.viewer); got != tc.want {
				t.Errorf("RenderToStyledLinesFor(%q) = %q, want %q", tc.markup, got, tc.want)
			}
		})
	}
}

// TestRenderLocationFormats pins the five format tokens, with and without a
// reader position.
func TestRenderLocationFormats(t *testing.T) {
	t.Parallel()

	fix := Viewer{Pos: googleplex, Known: true}

	tests := []struct {
		name   string
		viewer Viewer
		markup string
		want   string
	}{
		{"no position: default degrades to the bare code", Viewer{}, "`L8FW4V75V+8R`L", "8FW4V75V+8R"},
		{"no position: explicit default degrades the same way", Viewer{}, "`L8FW4V75V+8R|%default`L", "8FW4V75V+8R"},
		{"no position: distance renders nothing", Viewer{}, "`L8FW4V75V+8R|%d`L", ""},
		{"no position: bearing renders nothing", Viewer{}, "`L8FW4V75V+8R|%b`L", ""},
		{"no position: the coordinate still computes", Viewer{}, "`L8FW4V75V+8R|%ll`L", "48.858312, 2.294563"},
		{"no position: the code is always available", Viewer{}, "`L8FW4V75V+8R|%c`L", "8FW4V75V+8R"},
		{"with a position: distance alone", fix, "`L8FW4V75V+8R|%d`L", "8967 km"},
		{"with a position: bearing alone", fix, "`L8FW4V75V+8R|%b`L", "033° NNE"},
		{"with a position: code alone ignores the position", fix, "`L8FW4V75V+8R|%c`L", "8FW4V75V+8R"},
		{"with a position: the coordinate never needs one", fix, "`L8FW4V75V+8R|%ll`L", "48.858312, 2.294563"},
		{"surrounding text is preserved", fix, "Repeater: `L8FW4V75V+8R|%c`L at 2m", "Repeater: 8FW4V75V+8R at 2m"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := styledText(tc.markup, tc.viewer); got != tc.want {
				t.Errorf("RenderToStyledLinesFor(%q) = %q, want %q", tc.markup, got, tc.want)
			}
		})
	}
}

// TestRenderLocationShortenedCode pins the rule for a shortened code: it
// cannot be placed without a reference point, so it renders as written and the
// position-dependent formats add nothing.
func TestRenderLocationShortenedCode(t *testing.T) {
	t.Parallel()

	fix := Viewer{Pos: googleplex, Known: true}

	tests := []struct {
		name   string
		markup string
		want   string
	}{
		{"default renders the code as written", "`LV75V+8R`L", "V75V+8R"},
		{"distance adds nothing", "`LV75V+8R|%d`L", ""},
		{"bearing adds nothing", "`LV75V+8R|%b`L", ""},
		{"the coordinate form is unavailable", "`LV75V+8R|%ll`L", "V75V+8R"},
		{"the code form is unchanged", "`LV75V+8R|%c`L", "V75V+8R"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := styledText(tc.markup, fix); got != tc.want {
				t.Errorf("RenderToStyledLinesFor(%q) = %q, want %q", tc.markup, got, tc.want)
			}
		})
	}
}

// TestRenderLocationMalformedStaysText pins the fallback: a construct the
// parser cannot use is text, never an error. Only the markers are consumed, so
// the payload reaches the reader exactly the way a client without the construct
// renders it.
func TestRenderLocationMalformedStaysText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		markup string
		want   string
	}{
		{"no closing marker", "`L8FW4V75V+8R", "8FW4V75V+8R"},
		{"empty", "`L`L", ""},
		{"not a code", "`Lnot-a-code`L", "not-a-code"},
		{"unknown format token", "`L8FW4V75V+8R|%q`L", "8FW4V75V+8R|%q"},
		{"lowercase code is accepted and preserved", "`L8fw4v75v+8r|%c`L", "8fw4v75v+8r"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := styledText(tc.markup, Viewer{}); got != tc.want {
				t.Errorf("RenderToStyledLinesFor(%q) = %q, want %q", tc.markup, got, tc.want)
			}
		})
	}
}

// TestParseLocationNode pins the parsed shape: a usable construct becomes a
// NodeLocation that carries the code and format unrendered, so the renderer can
// resolve it against whatever position the client has at draw time.
func TestParseLocationNode(t *testing.T) {
	t.Parallel()

	nodes := Parse("`L8FW4V75V+8R|%d`L")
	if len(nodes) != 1 {
		t.Fatalf("Parse returned %v nodes, want 1", len(nodes))
	}
	if nodes[0].Type != NodeLocation {
		t.Fatalf("node type = %v, want NodeLocation", nodes[0].Type)
	}
	if want := "8FW4V75V+8R"; nodes[0].LocationCode != want {
		t.Errorf("LocationCode = %q, want %q", nodes[0].LocationCode, want)
	}
	if want := "%d"; nodes[0].LocationFormat != want {
		t.Errorf("LocationFormat = %q, want %q", nodes[0].LocationFormat, want)
	}
}

// TestRenderLocationWithoutViewerPinsDegradation pins that the position-free
// renderers show the code rather than dropping the construct.
func TestRenderLocationWithoutViewerPinsDegradation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		render func(string) string
	}{
		{"tview", func(markup string) string { return RenderToTView(Parse(markup)) }},
		{"plain text", func(markup string) string { return RenderToPlainText(Parse(markup)) }},
		{"styled, no viewer", func(markup string) string { return styledText(markup, Viewer{}) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.render("`L8FW4V75V+8R`L")
			if !strings.Contains(got, "8FW4V75V+8R") {
				t.Errorf("rendered %q, want it to contain the code", got)
			}
			if strings.Contains(got, "`L") {
				t.Errorf("rendered %q, want the markers consumed", got)
			}
		})
	}
}

// TestMicronDistance pins the page-level distance formatting: whole meters
// below a kilometer, one decimal place in kilometers below a thousand, and
// whole kilometers above that.
func TestMicronDistance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		meters float64
		want   string
	}{
		{"zero", 0, "0 m"},
		{"meters below a kilometer", 850, "850 m"},
		{"one meter below the kilometer boundary", 999, "999 m"},
		{"exactly a kilometer", 1000, "1.0 km"},
		{"one decimal place", 3200, "3.2 km"},
		{"just below a thousand kilometers", 999400, "999.4 km"},
		{"a thousand kilometers", 1000000, "1000 km"},
		{"thousands of kilometers", 8967033, "8967 km"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := micronDistance(tc.meters); got != tc.want {
				t.Errorf("micronDistance(%v) = %q, want %q", tc.meters, got, tc.want)
			}
		})
	}
}

// TestRenderLocationCarriesALink pins that a rendered location is clickable: the
// span carries a link whose target is the code behind the location scheme, never
// the rendered sentence, so copying a location always yields a code.
func TestRenderLocationCarriesALink(t *testing.T) {
	t.Parallel()

	viewer := Viewer{Pos: googleplex, Known: true}
	lines := RenderToStyledLinesFor("`L849VCWC8+R9`L", ThemeDark, viewer)
	if len(lines) == 0 || len(lines[0].Spans) == 0 {
		t.Fatal("no spans rendered")
	}

	span := lines[0].Spans[0]
	if span.Link == nil {
		t.Fatal("location span carries no link")
	}
	if want := "location:849VCWC8+R9"; span.Link.URL != want {
		t.Errorf("link URL = %q, want %q", span.Link.URL, want)
	}
	if span.Link.URL == span.Text {
		t.Error("link target must be the code, not the rendered sentence")
	}
}

// TestParseLocationLink pins the scheme: it round-trips a code and refuses
// everything that is not a location link.
func TestParseLocationLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   string
		wantCode string
		wantOK   bool
	}{
		{"location link", LocationURL("849VCWC8+R9"), "849VCWC8+R9", true},
		{"shortened code", LocationURL("V75V+8R"), "V75V+8R", true},
		{"empty code", "location:", "", false},
		{"another scheme", "lxmf@aabbccddeeff00112233445566778899", "", false},
		{"bare page address", "849VCWC8+R9", "", false},
		{"empty target", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, ok := ParseLocationLink(tc.target)
			if ok != tc.wantOK || code != tc.wantCode {
				t.Errorf("ParseLocationLink(%q) = %q,%v, want %q,%v",
					tc.target, code, ok, tc.wantCode, tc.wantOK)
			}
		})
	}
}

// styledText renders markup the way a client with the given position would and
// joins the visible span text, so a construct's expansion can be compared as a
// string.
func styledText(markup string, viewer Viewer) string {
	var sb strings.Builder
	for _, line := range RenderToStyledLinesFor(markup, ThemeDark, viewer) {
		for _, span := range line.Spans {
			sb.WriteString(span.Text)
		}
	}
	return sb.String()
}
