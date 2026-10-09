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

// TestDescribeRelativeBearing pins the one sentence that turns a compass reading
// into an instruction. "271° true" is a number the operator has to work out;
// "40° to your left" is something somebody holding a tablet can act on, which is
// the whole reason the client wants a heading at all.
func TestDescribeRelativeBearing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		deg  float64
		want string
	}{
		{"dead ahead", 0, "straight ahead"},
		{"just inside ahead right", 9.9, "straight ahead"},
		{"just inside ahead left", 350.1, "straight ahead"},
		{"right", 40, "40° to your right"},
		{"right at ninety", 90, "90° to your right"},
		{"left", 320, "40° to your left"},
		{"left at ninety", 270, "90° to your left"},
		{"behind right", 175, "behind you"},
		{"behind left", 185, "behind you"},
		{"dead behind", 180, "behind you"},
		{"just short of behind", 160, "160° to your right"},
		{"just past behind", 200, "160° to your left"},
		{"wrapped past a full turn", 400, "40° to your right"},
		{"negative wraps to the left", -40, "40° to your left"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := DescribeRelativeBearing(tc.deg); got != tc.want {
				t.Fatalf("DescribeRelativeBearing(%v) = %q, want %q", tc.deg, got, tc.want)
			}
		})
	}
}

// TestDescribeLocationRelativeBearing asserts the relative bearing is only ever
// rendered when the client holds BOTH a position and a heading, because a bearing
// from a stale or absent heading is a confidently wrong instruction.
func TestDescribeLocationRelativeBearing(t *testing.T) {
	t.Parallel()

	googleplex := geo.LatLng{Lat: 37.4220, Lng: -122.0841}
	fix := Viewer{Pos: googleplex, Known: true}

	tests := []struct {
		name    string
		code    string
		viewer  Viewer
		wantRel string
	}{
		{
			name:    "no heading at all",
			code:    "849VCWC8+R9",
			viewer:  fix,
			wantRel: "",
		},
		{
			name:    "no position at all",
			code:    "849VCWC8+R9",
			viewer:  Viewer{HeadingDeg: 180, HasHeading: true},
			wantRel: "",
		},
		{
			// The code sits 25 degrees true from the Googleplex, so a reader
			// facing due north finds it a little to the right.
			name:    "a heading yields a relative bearing",
			code:    "849VCWC8+R9",
			viewer:  Viewer{Pos: googleplex, Known: true, HeadingDeg: 0, HasHeading: true},
			wantRel: "25° to your right",
		},
		{
			name:    "facing the target is straight ahead",
			code:    "849VCWC8+R9",
			viewer:  Viewer{Pos: googleplex, Known: true, HeadingDeg: 25, HasHeading: true},
			wantRel: "straight ahead",
		},
		{
			name:    "facing away from the target puts it behind",
			code:    "849VCWC8+R9",
			viewer:  Viewer{Pos: googleplex, Known: true, HeadingDeg: 205, HasHeading: true},
			wantRel: "behind you",
		},
		{
			name:    "the relative bearing is measured from the heading, never the cardinal bearing",
			code:    "849VCWC8+R9",
			viewer:  Viewer{Pos: googleplex, Known: true, HeadingDeg: 180, HasHeading: true},
			wantRel: "155° to your left",
		},
		{
			name:    "a malformed code still has no relative bearing",
			code:    "not-a-code",
			viewer:  Viewer{Pos: googleplex, Known: true, HeadingDeg: 0, HasHeading: true},
			wantRel: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := DescribeLocation(tc.code, tc.viewer)
			if got.RelativeBearing != tc.wantRel {
				t.Fatalf("RelativeBearing = %q, want %q (detail %+v)", got.RelativeBearing, tc.wantRel, got)
			}
			if tc.wantRel != "" && !got.HasFix {
				t.Fatalf("a relative bearing was rendered without a fix: %+v", got)
			}
		})
	}
}

// TestDescribeLocationRelativeBearingNeverLeaksIntoRendering asserts the heading
// stays out of the rendered page text. A page is remote content; what the reader
// is facing is not something a page gets to print.
func TestDescribeLocationRelativeBearingNeverLeaksIntoRendering(t *testing.T) {
	t.Parallel()

	viewer := Viewer{
		Pos:        geo.LatLng{Lat: 37.4220, Lng: -122.0841},
		Known:      true,
		HeadingDeg: 17,
		HasHeading: true,
	}
	lines := RenderToStyledLinesFor("`L849VCWC8+R9`L", ThemeDark, viewer)
	if len(lines) == 0 {
		t.Fatalf("the page rendered no lines at all")
	}
	for _, line := range lines {
		var sb []byte
		for _, span := range line.Spans {
			sb = append(sb, span.Text...)
		}
		if containsAny(string(sb), "to your", "behind you", "straight ahead") {
			t.Fatalf("the rendered page carries a relative bearing: %q", string(sb))
		}
	}
}

// containsAny reports whether text contains any of the needles.
func containsAny(text string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}
