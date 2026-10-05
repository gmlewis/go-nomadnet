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
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-reticulum/geo"
	"github.com/gmlewis/tcell/v2"
)

// locationTestApp builds a test app with the given reader position and a
// recording clipboard.
func locationTestApp(fix bool) (*App, *fakeClipboard) {
	app := newTestApp()
	if fix {
		app.Viewer = micron.Viewer{Pos: geo.LatLng{Lat: 37.4220, Lng: -122.0841}, Known: true}
	}
	fake := &fakeClipboard{}
	app.clipboard = fake
	return app, fake
}

// TestShowLocationActionsCopiesTheCode pins the primary action: activating a
// location link opens the client's location actions, and copying yields the
// Plus Code itself — never the rendered sentence with its distance and bearing.
func TestShowLocationActionsCopiesTheCode(t *testing.T) {
	t.Parallel()

	app, fake := locationTestApp(true)
	app.ShowLocationActions("849VCWC8+R9")

	if got := app.Dialogs.Count(); got != 1 {
		t.Fatalf("dialog count = %v, want 1", got)
	}
	if got := app.GetFocus(); !isButton(got, "Copy code") {
		t.Fatalf("initial focus = %T, want the Copy code button", got)
	}

	pressKeyThroughFocus(app, tcell.KeyEnter)

	if len(fake.texts) != 1 || fake.texts[0] != "849VCWC8+R9" {
		t.Fatalf("clipboard = %v, want just the code", fake.texts)
	}
	// The dialog stays open so the reader can also copy the coordinate.
	if got := app.Dialogs.Count(); got != 1 {
		t.Fatalf("dialog count after copy = %v, want the dialog to stay open", got)
	}
	if got := app.GetFocus(); !isButton(got, "Copy code") {
		t.Fatalf("focus after copy = %T, want to stay on Copy code", got)
	}
}

// TestShowLocationActionsCopiesTheCoordinate pins the second action: Tab moves
// to Copy coordinate, which writes the code's centre in signed decimal degrees.
func TestShowLocationActionsCopiesTheCoordinate(t *testing.T) {
	t.Parallel()

	app, fake := locationTestApp(true)
	app.ShowLocationActions("849VCWC8+R9")

	if got := pressKeyThroughFocus(app, tcell.KeyTab); !isButton(got, "Copy coordinate") {
		t.Fatalf("focus after Tab = %T, want the Copy coordinate button", got)
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)

	if len(fake.texts) != 1 || fake.texts[0] != "37.422062, -122.084063" {
		t.Fatalf("clipboard = %v, want the decoded coordinate", fake.texts)
	}
}

// TestShowLocationActionsCloseDismisses pins that Close leaves no dialog and
// writes nothing.
func TestShowLocationActionsCloseDismisses(t *testing.T) {
	t.Parallel()

	app, fake := locationTestApp(true)
	app.ShowLocationActions("849VCWC8+R9")

	// Tab twice: Copy code → Copy coordinate → Close.
	pressKeyThroughFocus(app, tcell.KeyTab)
	if got := pressKeyThroughFocus(app, tcell.KeyTab); !isButton(got, "Close") {
		t.Fatalf("focus after two Tabs = %T, want the Close button", got)
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)

	if got := app.Dialogs.Count(); got != 0 {
		t.Fatalf("dialog count after Close = %v, want 0", got)
	}
	if len(fake.texts) != 0 {
		t.Fatalf("clipboard = %v, want no write", fake.texts)
	}
}

// TestShowLocationActionsEscapeDismisses pins that Escape closes the actions
// without writing anything.
func TestShowLocationActionsEscapeDismisses(t *testing.T) {
	t.Parallel()

	app, fake := locationTestApp(true)
	app.ShowLocationActions("849VCWC8+R9")

	pressKeyThroughFocus(app, tcell.KeyEscape)

	if got := app.Dialogs.Count(); got != 0 {
		t.Fatalf("dialog count after Esc = %v, want 0", got)
	}
	if len(fake.texts) != 0 {
		t.Fatalf("clipboard = %v, want no write", fake.texts)
	}
}

// TestShowLocationActionsWithoutAPosition pins that a client with no configured
// position still offers the actions it can honour: the code and the coordinate
// need no position, and the distance line says so rather than sitting blank.
func TestShowLocationActionsWithoutAPosition(t *testing.T) {
	t.Parallel()

	app, fake := locationTestApp(false)
	app.ShowLocationActions("849VCWC8+R9")

	if got := app.Dialogs.Count(); got != 1 {
		t.Fatalf("dialog count = %v, want 1", got)
	}

	// Copy code still works with no position.
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if len(fake.texts) != 1 || fake.texts[0] != "849VCWC8+R9" {
		t.Fatalf("clipboard = %v, want just the code", fake.texts)
	}
}

// TestShowLocationActionsShortenedCodeOmitsTheCoordinate pins that a shortened
// code has no reference point, so there is no coordinate to copy and the dialog
// offers only Copy code and Close.
func TestShowLocationActionsShortenedCodeOmitsTheCoordinate(t *testing.T) {
	t.Parallel()

	app, _ := locationTestApp(true)
	app.ShowLocationActions("V75V+8R")

	if got := pressKeyThroughFocus(app, tcell.KeyTab); !isButton(got, "Close") {
		t.Fatalf("focus after Tab = %T, want Close (no coordinate for a short code)", got)
	}
}

// TestLocationCard pins what the actions card shows, including the two
// degraded cases: a shortened code has no coordinate, and a client without a
// position says so instead of showing a blank distance.
func TestLocationCard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		detail micron.LocationDetail
		want   []string
		absent []string
	}{
		{
			// The certified vector: a reader in Sydney, the Googleplex code.
			name:   "with a position",
			detail: micron.DescribeLocation("849VCWC8+R9", micron.Viewer{Pos: geo.LatLng{Lat: -33.8568, Lng: 151.2153}, Known: true}),
			want: []string{
				"Code:       849VCWC8+R9",
				"Coordinate: 37.422062, -122.084063",
				"Distance:   11953 km",
				"Bearing:    056° NE",
			},
		},
		{
			name:   "without a position",
			detail: micron.DescribeLocation("849VCWC8+R9", micron.Viewer{}),
			want: []string{
				"Coordinate: 37.422062, -122.084063",
				"Distance:   (no position set)",
			},
			absent: []string{"Bearing:"},
		},
		{
			name:   "shortened code",
			detail: micron.DescribeLocation("V75V+8R", micron.Viewer{Pos: geo.LatLng{Lat: 37.4220, Lng: -122.0841}, Known: true}),
			want:   []string{"Code:       V75V+8R", "Distance:   (no position set)"},
			absent: []string{"Coordinate:"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := locationCard(tc.detail)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("locationCard() = %q, want it to contain %q", got, want)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Errorf("locationCard() = %q, want it NOT to contain %q", got, absent)
				}
			}
		})
	}
}

// TestBrowserLinkOpensLocationActions pins the dispatch: a rendered location's
// link target is handled by the client, opening the actions dialog instead of
// being treated as a page address.
func TestBrowserLinkOpensLocationActions(t *testing.T) {
	t.Parallel()

	app, fake := locationTestApp(true)
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	if got := micron.LocationURL("849VCWC8+R9"); got != "location:849VCWC8+R9" {
		t.Fatalf("LocationURL = %q, want the code behind the scheme", got)
	}

	bd.HandleLink(micron.LocationURL("849VCWC8+R9"), "")

	if got := app.Dialogs.Count(); got != 1 {
		t.Fatalf("dialog count = %v, want the location actions dialog", got)
	}
	if got := app.GetFocus(); !isButton(got, "Copy code") {
		t.Fatalf("initial focus = %T, want Copy code", got)
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if len(fake.texts) != 1 || fake.texts[0] != "849VCWC8+R9" {
		t.Fatalf("clipboard = %v, want just the code", fake.texts)
	}
}
