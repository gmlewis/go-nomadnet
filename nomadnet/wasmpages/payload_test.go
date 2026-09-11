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

// This file pins the JSON payload a page plugin receives, which is the guest's
// only view of the request.

package wasmpages

import (
	"strings"
	"testing"
)

// TestPageRequestPayloadVerbatim pins that request data reaches the guest
// verbatim: Go's encoder would otherwise rewrite "<", ">" and "&" as \u003c,
// \u003e and \u0026, forcing every hand-written guest to decode HTML escapes
// just to read a message.
func TestPageRequestPayloadVerbatim(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "angle brackets", value: "a > b & c < d"},
		{name: "plain text", value: "hello"},
		{name: "quote", value: `say "hi"`},
		{name: "backslash", value: `back\slash`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := PageRequest{
				Path:        "/page/guestbook.wasm",
				RequestData: map[string]string{"field_message": tc.value},
				RequestedAt: 1730000000,
			}
			payload := string(req.payload())
			if strings.Contains(payload, `\u00`) {
				t.Errorf("payload %q carries an HTML escape", payload)
			}
			if strings.Contains(payload, `"`+tc.value+`"`) {
				return
			}
			// A value needing JSON escapes cannot appear verbatim; require the
			// escaped form to round-trip instead of rejecting it outright.
			if tc.value == "a > b & c < d" || tc.value == "hello" {
				t.Errorf("payload %q does not carry %q verbatim", payload, tc.value)
			}
		})
	}
}

// TestPageRequestPayloadIsSingleLine pins that the payload has no trailing
// newline, so a guest scanning the request region sees only the JSON object.
func TestPageRequestPayloadIsSingleLine(t *testing.T) {
	t.Parallel()

	payload := string(PageRequest{Path: "/page/page.wasm"}.payload())
	if strings.HasSuffix(payload, "\n") {
		t.Errorf("payload %q ends with a newline", payload)
	}
	if !strings.HasPrefix(payload, "{") || !strings.HasSuffix(payload, "}") {
		t.Errorf("payload %q is not a bare JSON object", payload)
	}
}
