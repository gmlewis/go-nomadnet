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
	"strings"
	"testing"
)

//go:embed testdata/handlelink_parity.json
var handleLinkParityJSON string

// handleLinkCase is one row of the Python handle_link resolution golden.
type handleLinkCase struct {
	URL             string   `json:"url"`
	Kind            string   `json:"kind"` // "anchor" | "rrc" | "typed"
	DestinationType *string  `json:"destination_type"`
	Target          *string  `json:"target"`
	PartialIDs      []string `json:"partial_ids"`
}

// TestParseLinkTargetPythonParity verifies that ParseLinkTargetWithFields
// matches Python's Browser.handle_link *resolution* (the destination_type +
// resolved target / partial_ids produced before any side-effecting dispatch)
// for a battery of link targets captured from the Python reference.
//
// The golden master (testdata/handlelink_parity.json) was produced by running
// Python's real handle_link resolution logic with RNS mocked. Python uses an
// unbounded link_target.split("@") and only treats the link as typed when
// exactly one "@" is present (len(components) == 2); a target with two or more
// "@" falls through to the bare-address branch (nomadnetwork.node, first
// component). Go must match this — not split on only the first "@".
func TestParseLinkTargetPythonParity(t *testing.T) {
	t.Parallel()

	var golden struct {
		Cases []handleLinkCase `json:"cases"`
	}
	if err := json.Unmarshal([]byte(handleLinkParityJSON), &golden); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	for _, tc := range golden.Cases {
		t.Run(tc.URL, func(t *testing.T) {
			t.Parallel()

			gotType, gotHash, _, _ := ParseLinkTargetWithFields(tc.URL)

			switch tc.Kind {
			case "anchor":
				if gotType != "anchor" {
					t.Errorf("type = %q, want %q", gotType, "anchor")
				}
				if tc.Target == nil {
					t.Fatal("anchor case missing target")
				}
				if gotHash != *tc.Target {
					t.Errorf("hash = %q, want %q", gotHash, *tc.Target)
				}

			case "rrc":
				// Python routes rrc:// straight to handle_rrc_link(target[6:]).
				// Go signals this with destType "rrc" and hash = target[6:].
				if gotType != "rrc" {
					t.Errorf("type = %q, want %q", gotType, "rrc")
				}
				if tc.Target == nil {
					t.Fatal("rrc case missing target")
				}
				if gotHash != *tc.Target {
					t.Errorf("hash = %q, want %q", gotHash, *tc.Target)
				}

			case "typed":
				if tc.DestinationType == nil {
					t.Fatal("typed case missing destination_type")
				}
				wantType := *tc.DestinationType
				if gotType != wantType {
					t.Errorf("type = %q, want %q", gotType, wantType)
				}
				if wantType == "partial" {
					// Go returns the partial spec as the hash (everything after
					// "p:"); Python returns partial_ids as comps[1:]. Splitting
					// Go's hash on ":" reconstructs Python's list.
					gotIDs := strings.Split(gotHash, ":")
					if !equalStrings(gotIDs, tc.PartialIDs) {
						t.Errorf("partial ids = %v, want %v", gotIDs, tc.PartialIDs)
					}
				} else {
					if tc.Target == nil {
						t.Fatal("typed case missing target")
					}
					if gotHash != *tc.Target {
						t.Errorf("hash = %q, want %q", gotHash, *tc.Target)
					}
				}

			default:
				t.Fatalf("unknown kind %q in golden", tc.Kind)
			}
		})
	}
}

// TestHandleLinkDispatchPythonParity verifies that BrowserDisplay.HandleLink
// dispatches each link target to the same handler Python's handle_link would,
// via the On* callbacks. This covers the side-effecting dispatch routing that
// complements the pure resolution tested above.
func TestHandleLinkDispatchPythonParity(t *testing.T) {
	t.Parallel()

	const h32 = "aabb1122aabb1122aabb1122aabb1122"

	tests := []struct {
		name        string
		link        string
		wantAnchor  string // set to expect OnJumpAnchor
		wantRRC     bool   // expect OnOpenRRC
		wantLXMF    string // expect OnOpenLXMF
		wantNode    string // expect OnRetrieveURL
		wantPartial []string
		wantError   bool
	}{
		{"anchor", "#intro", "intro", false, "", "", nil, false},
		{"empty anchor", "#", "", false, "", "", nil, false},
		{"rrc scheme", "rrc://" + h32 + "/general", "", true, "", "", nil, false},
		{"nnn@ node", "nnn@" + h32, "", false, "", h32, nil, false},
		{"lxmf@ delivery", "lxmf@" + h32, "", false, h32, "", nil, false},
		{"rrc@ hub", "rrc@" + h32 + "/general", "", true, "", "", nil, false},
		{"plain bare address", h32, "", false, "", h32, nil, false},
		{"non-hex bare address", "somenode", "", false, "", "somenode", nil, false},
		{"partial", "p:sidebar:header", "", false, "", "", []string{"sidebar", "header"}, false},
		// Multiple "@" must fall through to the bare-address branch in Python
		// (nomadnetwork.node, first component), not be treated as typed.
		{"multi at is bare node", "a@b@c", "", false, "", "a", nil, false},
		{"unknown typed -> error", "unknowntype@abc", "", false, "", "", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			bd := NewBrowserDisplay(app)

			var gotAnchor, gotLXMF, gotNode string
			var gotRRC bool
			var gotPartial []string
			var gotError string
			gotAnchorFired := false
			gotNodeFired := false

			bd.OnJumpAnchor = func(name string) { gotAnchor = name; gotAnchorFired = true }
			bd.OnOpenRRC = func(hub, room string) { gotRRC = true }
			bd.OnOpenLXMF = func(hash string) { gotLXMF = hash }
			bd.OnRetrieveURL = func(url string, requestData map[string]string) { gotNode = url; gotNodeFired = true }
			bd.OnPartialUpdate = func(ids []string) { gotPartial = ids }
			bd.OnBrowserError = func(msg string) { gotError = msg }

			bd.HandleLink(tt.link, "")

			if tt.wantAnchor != "" || (tt.name == "empty anchor") {
				if !gotAnchorFired {
					t.Error("expected OnJumpAnchor to fire")
				}
				if gotAnchor != tt.wantAnchor {
					t.Errorf("anchor = %q, want %q", gotAnchor, tt.wantAnchor)
				}
			}
			if tt.wantRRC && !gotRRC {
				t.Error("expected OnOpenRRC to fire")
			}
			if tt.wantLXMF != "" && gotLXMF != tt.wantLXMF {
				t.Errorf("lxmf = %q, want %q", gotLXMF, tt.wantLXMF)
			}
			if tt.wantNode != "" {
				if !gotNodeFired {
					t.Error("expected OnRetrieveURL to fire")
				}
				if gotNode != tt.wantNode {
					t.Errorf("node = %q, want %q", gotNode, tt.wantNode)
				}
			}
			if tt.wantPartial != nil && !equalStrings(gotPartial, tt.wantPartial) {
				t.Errorf("partial = %v, want %v", gotPartial, tt.wantPartial)
			}
			if tt.wantError && gotError == "" {
				t.Error("expected OnBrowserError to fire")
			}
			if !tt.wantError && gotError != "" {
				t.Errorf("unexpected error: %q", gotError)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
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
