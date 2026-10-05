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
	"fmt"
	"strings"

	"github.com/gmlewis/go-reticulum/geo"
)

// This file adds the location construct to Micron:
//
//	`L<code>`L
//	`L<code>|<format>`L
//
// A page stores an Open Location Code (Plus Code) and names how it wants it
// shown. A client that knows its own position renders the great-circle
// distance and compass bearing to the code as well, all from data it already
// has, so a reader on a 1 kbps link with no map and no geocoder still learns how
// far away something is and in which direction. Python's NomadNet has no such
// construct: it consumes the backtick and the marker character and renders the
// payload as ordinary text, so a reader on the original client sees the bare
// code where gonomadnet shows distance and bearing. Because pages are parsed
// client-side, only the reader's own client can resolve a position.
//
// The code is validated before it is rendered. A malformed code, or a format
// token this file does not define, makes the construct unrecognized: the marker
// is consumed and the payload falls through as text, exactly the way Python
// renders it. An extension that cannot be understood is text, never an error.

// Viewer describes where the reader of a page is, which is what the `L
// construct resolves its distance and bearing against. Known is false when the
// client has no position, and every position-dependent form then degrades to
// the bare code rather than guessing.
type Viewer struct {
	Pos   geo.LatLng
	Known bool
}

// The format vocabulary of a `L construct. With no format the default applies.
const (
	locationFormatDefault  = "%default"
	locationFormatCode     = "%c"
	locationFormatDistance = "%d"
	locationFormatBearing  = "%b"
	locationFormatLatLng   = "%ll"
)

// locationMarker opens and closes a location construct.
const locationMarker = "`L"

// locationScheme prefixes the link target a rendered location carries. A UI
// that opens the client's location actions recognises this scheme; every other
// consumer (a link hit-test, a status line) can fall back to the code it
// carries.
const locationScheme = "location:"

// LocationURL is the link target that makes a rendered location clickable. It
// carries the Plus Code itself, never the rendered sentence, so a location
// copied out of a page is always a code.
func LocationURL(code string) string { return locationScheme + code }

// ParseLocationLink reports whether target is a location link and, if so,
// returns the Plus Code it carries.
func ParseLocationLink(target string) (string, bool) {
	code, ok := strings.CutPrefix(target, locationScheme)
	if !ok || code == "" {
		return "", false
	}
	return code, true
}

// LocationDetail describes a location link's target in the forms a UI wants to
// show or copy. Coordinate is empty when the code cannot be decoded, and the
// distance and bearing are empty unless the reader's position is known.
type LocationDetail struct {
	// Code is the Plus Code exactly as the page wrote it.
	Code string
	// Coordinate is the code's centre as signed decimal degrees, or empty.
	Coordinate string
	// Distance and Bearing describe the code from the reader's position.
	Distance string
	Bearing  string
	// HasFix reports whether the reader's position was known, so Distance and
	// Bearing are meaningful.
	HasFix bool
}

// DescribeLocation resolves a location link's code for a viewer. It never fails:
// a code that cannot be decoded yields a detail carrying only the code.
func DescribeLocation(code string, viewer Viewer) LocationDetail {
	detail := LocationDetail{Code: code}

	// A shortened code has no reference point, so there is nothing to place.
	if !geo.IsFullOLC(code) {
		return detail
	}
	area, err := geo.DecodeOLC(code)
	if err != nil {
		return detail
	}
	target := area.Center()
	detail.Coordinate = geo.FormatLatLng(target)

	if !viewer.Known {
		return detail
	}
	detail.HasFix = true
	detail.Distance = micronDistance(geo.HaversineDistance(viewer.Pos, target))
	detail.Bearing = geo.FormatBearing(geo.InitialBearing(viewer.Pos, target))
	return detail
}

// parseLocation parses a location construct whose marker starts at start. It
// returns the node holding the code and the number of bytes consumed, or nil
// when the construct is unusable. A recognized marker is always consumed even
// when its payload falls back to text, so the raw bytes are never re-read as
// markup.
func parseLocation(line string, start int) (*Node, int) {
	body := start + len(locationMarker)
	payload := line[body:]
	end := strings.Index(payload, locationMarker)
	if end < 0 {
		return nil, len(locationMarker)
	}

	code, format, _ := strings.Cut(payload[:end], "|")
	code = strings.TrimSpace(code)
	format = strings.TrimSpace(format)
	if format == "" {
		format = locationFormatDefault
	}
	if !locationFormatValid(format) || !geo.IsValidOLC(code) {
		return nil, len(locationMarker)
	}

	return &Node{Type: NodeLocation, LocationCode: code, LocationFormat: format}, end + 2*len(locationMarker)
}

// locationFormatValid reports whether format is one this construct defines. An
// unknown token makes the construct unrecognized rather than silently rendering
// the default, so a typo in a page is visible while the page is being written.
func locationFormatValid(format string) bool {
	switch format {
	case locationFormatDefault, locationFormatCode, locationFormatDistance,
		locationFormatBearing, locationFormatLatLng:
		return true
	}
	return false
}

// renderLocation renders one location construct for a reader. A shortened code
// cannot be placed without a reference point, so it renders exactly as written
// and every position-dependent format adds nothing; %c and %ll need no reader
// position, and %ll is available only for a full code.
func renderLocation(code, format string, viewer Viewer) string {
	if format == locationFormatCode {
		return code
	}

	if !geo.IsFullOLC(code) {
		// Shortened code: the digits that identify the region are missing, and
		// guessing a reference point could place the answer in the wrong
		// hemisphere. The default renders the code as written; %d and %b add
		// nothing rather than printing a placeholder.
		if format == locationFormatDistance || format == locationFormatBearing {
			return ""
		}
		return code
	}

	area, err := geo.DecodeOLC(code)
	if err != nil {
		return code
	}
	target := area.Center()

	if format == locationFormatLatLng {
		// A coordinate needs no reference point, so %ll always computes.
		return geo.FormatLatLng(target)
	}

	if !viewer.Known {
		if format == locationFormatDistance || format == locationFormatBearing {
			return ""
		}
		return code
	}

	distance := micronDistance(geo.HaversineDistance(viewer.Pos, target))
	bearing := geo.FormatBearing(geo.InitialBearing(viewer.Pos, target))
	switch format {
	case locationFormatDistance:
		return distance
	case locationFormatBearing:
		return bearing
	}
	return fmt.Sprintf("%v (%v, bearing %v)", code, distance, bearing)
}

// micronDistance formats a distance the way a Micron page shows it: whole
// meters below a kilometer, one decimal place in kilometers below a thousand
// kilometers, and whole kilometers above that. The unit set is deliberately
// narrower than the bot's multi-unit FormatDistance, because a page has one
// line and a reader wants the one number that answers "how far".
func micronDistance(meters float64) string {
	km := meters / 1000
	switch {
	case meters < 1000:
		return fmt.Sprintf("%.0f m", meters)
	case km < 1000:
		return fmt.Sprintf("%.1f km", km)
	default:
		return fmt.Sprintf("%.0f km", km)
	}
}
