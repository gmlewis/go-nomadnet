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

func TestValidateLXMFLinkValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		errMsg string
	}{
		{"valid 32-byte hex", "aabb1122aabb1122aabb1122aabb1122", ""},
		{"too short", "aabb1122", "invalid length"},
		{"too long", "aabb1122aabb1122aabb1122aabb1122aa", "invalid length"},
		{"invalid hex", "zzbb1122aabb1122aabb1122aabb1122", "could not decode"},
		{"empty", "", "invalid length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateLXMFLink(tt.input)
			if tt.errMsg == "" {
				if err != nil {
					t.Errorf("ValidateLXMFLink(%q) = %v, want nil", tt.input, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateLXMFLink(%q) = nil, want error containing %q", tt.input, tt.errMsg)
				}
			}
		})
	}
}

func TestBrowserHandleLXMFLink(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	var openedHash string
	bd.OnOpenLXMF = func(hash string) { openedHash = hash }

	validHash := "aabb1122aabb1122aabb1122aabb1122"
	bd.HandleLXMFLink(validHash)

	if openedHash != validHash {
		t.Errorf("openedHash = %q, want %q", openedHash, validHash)
	}
}

func TestBrowserHandleLXMFLinkInvalid(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	var lastError string
	bd.OnBrowserError = func(msg string) { lastError = msg }

	bd.HandleLXMFLink("invalid")
	if lastError == "" {
		t.Error("HandleLXMFLink with invalid input should report error")
	}
}

func TestValidateRRCLinkValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantHub  string
		wantRoom string
		wantDest string
	}{
		{"hub and room", "aabb1122aabb1122aabb1122aabb1122/general", "aabb1122aabb1122aabb1122aabb1122", "general", ""},
		{"hub with dest", "aabb1122aabb1122aabb1122aabb1122:myhub/general", "aabb1122aabb1122aabb1122aabb1122", "general", "myhub"},
		{"hub only", "aabb1122aabb1122aabb1122aabb1122", "aabb1122aabb1122aabb1122aabb1122", "", ""},
		{"hub with leading slash", "/aabb1122aabb1122aabb1122aabb1122/random", "aabb1122aabb1122aabb1122aabb1122", "random", ""},
		{"room with hash prefix", "aabb1122aabb1122aabb1122aabb1122/#random", "aabb1122aabb1122aabb1122aabb1122", "random", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub, room, dest, err := ParseRRCLink(tt.input)
			if err != nil {
				t.Fatalf("ParseRRCLink(%q) error: %v", tt.input, err)
			}
			if hub != tt.wantHub {
				t.Errorf("hub = %q, want %q", hub, tt.wantHub)
			}
			if room != tt.wantRoom {
				t.Errorf("room = %q, want %q", room, tt.wantRoom)
			}
			if dest != tt.wantDest {
				t.Errorf("dest = %q, want %q", dest, tt.wantDest)
			}
		})
	}
}

func TestValidateRRCLinkInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"invalid hex", "zzzz1122aabb1122aabb1122aabb1122/general"},
		{"too short hex", "aabb/general"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, _, err := ParseRRCLink(tt.input)
			if err == nil {
				t.Errorf("ParseRRCLink(%q) = nil error, want error", tt.input)
			}
		})
	}
}

func TestBrowserHandleRRCLink(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	var hubHash, room string
	bd.OnOpenRRC = func(hash, r string) { hubHash = hash; room = r }

	bd.HandleRRCLink("aabb1122aabb1122aabb1122aabb1122/general")

	if hubHash != "aabb1122aabb1122aabb1122aabb1122" {
		t.Errorf("hubHash = %q, want %q", hubHash, "aabb1122aabb1122aabb1122aabb1122")
	}
	if room != "general" {
		t.Errorf("room = %q, want %q", room, "general")
	}
}

func TestBrowserHandleRRCLinkInvalid(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	var lastError string
	bd.OnBrowserError = func(msg string) { lastError = msg }

	bd.HandleRRCLink("invalid")
	if lastError == "" {
		t.Error("HandleRRCLink with invalid input should report error")
	}
}

func TestBrowserHandleLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		link          string
		expectAnchor  bool
		expectRRC     bool
		expectLXMF    bool
		expectNode    bool
		expectPartial bool
		expectError   bool
		anchorName    string
		rrcHub        string
		rrcRoom       string
		lxmfHash      string
		nodeURL       string
		partialIDs    []string
	}{
		{
			name:         "anchor link",
			link:         "#intro",
			expectAnchor: true,
			anchorName:   "intro",
		},
		{
			name:         "empty anchor",
			link:         "#",
			expectAnchor: true,
			anchorName:   "",
		},
		{
			name:      "rrc:// URL",
			link:      "rrc://aabb1122aabb1122aabb1122aabb1122/general",
			expectRRC: true,
			rrcHub:    "aabb1122aabb1122aabb1122aabb1122",
			rrcRoom:   "general",
		},
		{
			name:       "lxmf@ shorthand",
			link:       "lxmf@aabb1122aabb1122aabb1122aabb1122",
			expectLXMF: true,
			lxmfHash:   "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:       "nomadnetwork.node@ explicit",
			link:       "nomadnetwork.node@aabb1122aabb1122aabb1122aabb1122",
			expectNode: true,
			nodeURL:    "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:       "nnn@ shorthand for node",
			link:       "nnn@aabb1122aabb1122aabb1122aabb1122",
			expectNode: true,
			nodeURL:    "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:      "rrc@ shorthand",
			link:      "rrc@aabb1122aabb1122aabb1122aabb1122/general",
			expectRRC: true,
			rrcHub:    "aabb1122aabb1122aabb1122aabb1122",
			rrcRoom:   "general",
		},
		{
			name:          "partial link",
			link:          "p:sidebar:header",
			expectPartial: true,
			partialIDs:    []string{"sidebar", "header"},
		},
		{
			name:       "plain hash defaults to node",
			link:       "aabb1122aabb1122aabb1122aabb1122",
			expectNode: true,
			nodeURL:    "aabb1122aabb1122aabb1122aabb1122",
		},
		{
			name:        "unknown destination type",
			link:        "unknown_type@abc",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			bd := NewBrowserDisplay(app)

			var gotAnchor string
			var gotRRCHub, gotRRCRoom string
			var gotLXMFHash string
			var gotNodeURL string
			var gotPartialIDs []string
			var gotError string

			bd.OnJumpAnchor = func(name string) { gotAnchor = name }
			bd.OnOpenRRC = func(hub, room string) { gotRRCHub = hub; gotRRCRoom = room }
			bd.OnOpenLXMF = func(hash string) { gotLXMFHash = hash }
			bd.OnRetrieveURL = func(url string) { gotNodeURL = url }
			bd.OnPartialUpdate = func(ids []string) { gotPartialIDs = ids }
			bd.OnBrowserError = func(msg string) { gotError = msg }

			bd.HandleLink(tt.link)

			if tt.expectAnchor && gotAnchor != tt.anchorName {
				t.Errorf("anchor = %q, want %q", gotAnchor, tt.anchorName)
			}
			if tt.expectRRC && (gotRRCHub != tt.rrcHub || gotRRCRoom != tt.rrcRoom) {
				t.Errorf("rrc = (%q, %q), want (%q, %q)", gotRRCHub, gotRRCRoom, tt.rrcHub, tt.rrcRoom)
			}
			if tt.expectLXMF && gotLXMFHash != tt.lxmfHash {
				t.Errorf("lxmf = %q, want %q", gotLXMFHash, tt.lxmfHash)
			}
			if tt.expectNode && gotNodeURL != tt.nodeURL {
				t.Errorf("node = %q, want %q", gotNodeURL, tt.nodeURL)
			}
			if tt.expectPartial {
				if len(gotPartialIDs) != len(tt.partialIDs) {
					t.Errorf("partial IDs = %v, want %v", gotPartialIDs, tt.partialIDs)
				}
				for i, id := range gotPartialIDs {
					if id != tt.partialIDs[i] {
						t.Errorf("partial[%v] = %q, want %q", i, id, tt.partialIDs[i])
					}
				}
			}
			if tt.expectError && gotError == "" {
				t.Error("expected error, got none")
			}
			if !tt.expectError && gotError != "" {
				t.Errorf("unexpected error: %q", gotError)
			}
		})
	}
}
