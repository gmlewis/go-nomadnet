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
	_ "embed"
	"encoding/json"
	"testing"
)

//go:embed testdata/rrc_lxmf_link_parity.json
var rrcLXMFLinkParityJSON string

// TestParseRRCLinkPythonParity verifies ParseRRCLink matches Python's
// handle_rrc_link parsing (hub hex, dest, normalized room) for a battery of
// link targets captured from the Python reference. Python normalizes the room
// as room.strip().lstrip("#").strip().lower() — strip whitespace first, then
// strip all leading "#", then strip again, then lowercase — and treats an
// empty room as absent. Go must match this order.
func TestParseRRCLinkPythonParity(t *testing.T) {
	t.Parallel()

	var golden struct {
		RRC []struct {
			URL   string  `json:"url"`
			Hub   string  `json:"hub"`
			Dest  string  `json:"dest"`
			Room  string  `json:"room"`
			Error *string `json:"error"`
		} `json:"rrc"`
	}
	if err := json.Unmarshal([]byte(rrcLXMFLinkParityJSON), &golden); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	for _, tc := range golden.RRC {
		t.Run(tc.URL, func(t *testing.T) {
			t.Parallel()

			hub, room, dest, err := ParseRRCLink(tc.URL)

			if tc.Error != nil {
				if err == nil {
					t.Fatalf("ParseRRCLink(%q) want error %q, got nil (hub=%q room=%q dest=%q)",
						tc.URL, *tc.Error, hub, room, dest)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRRCLink(%q) want success, got error: %v", tc.URL, err)
			}
			if hub != tc.Hub {
				t.Errorf("hub = %q, want %q", hub, tc.Hub)
			}
			if dest != tc.Dest {
				t.Errorf("dest = %q, want %q", dest, tc.Dest)
			}
			if room != tc.Room {
				t.Errorf("room = %q, want %q", room, tc.Room)
			}
		})
	}
}

// TestValidateLXMFLinkPythonParity verifies ValidateLXMFLink matches Python's
// handle_lxmf_link validation (length == 32 hex chars, decodable hex) for a
// battery of inputs captured from the Python reference.
func TestValidateLXMFLinkPythonParity(t *testing.T) {
	t.Parallel()

	var golden struct {
		LXMF []struct {
			URL   string  `json:"url"`
			Hash  string  `json:"hash"`
			Error *string `json:"error"`
		} `json:"lxmf"`
	}
	if err := json.Unmarshal([]byte(rrcLXMFLinkParityJSON), &golden); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	for _, tc := range golden.LXMF {
		t.Run(tc.URL, func(t *testing.T) {
			t.Parallel()

			err := ValidateLXMFLink(tc.URL)
			if tc.Error != nil {
				if err == nil {
					t.Fatalf("ValidateLXMFLink(%q) want error, got nil", tc.URL)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateLXMFLink(%q) want success, got error: %v", tc.URL, err)
			}
		})
	}
}
