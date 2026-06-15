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

import "testing"

func TestResolveGuideLinkAnchor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   string
		wantType LinkAction
		wantData string
	}{
		{"anchor with name", "#getting-started", ActionAnchorJump, "getting-started"},
		{"anchor empty", "#", ActionAnchorJump, ""},
		{"anchor with spaces", "#trust levels", ActionAnchorJump, "trust levels"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			action, data := ResolveGuideLink(tt.target)
			if action != tt.wantType {
				t.Errorf("action = %v, want %v", action, tt.wantType)
			}
			if data != tt.wantData {
				t.Errorf("data = %q, want %q", data, tt.wantData)
			}
		})
	}
}

func TestResolveGuideLinkRRC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   string
		wantType LinkAction
		wantData string
	}{
		{"rrc link", "rrc://hash/room", ActionOpenRRC, "hash/room"},
		{"rrc empty", "rrc://", ActionOpenRRC, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			action, data := ResolveGuideLink(tt.target)
			if action != tt.wantType {
				t.Errorf("action = %v, want %v", action, tt.wantType)
			}
			if data != tt.wantData {
				t.Errorf("data = %q, want %q", data, tt.wantData)
			}
		})
	}
}

func TestResolveGuideLinkNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   string
		wantType LinkAction
		wantData string
	}{
		{"plain hash", "a1b2c3d4e5", ActionOpenPage, "a1b2c3d4e5"},
		{"hash with path", "a1b2c3d4e5:/page/index.mu", ActionOpenPage, "a1b2c3d4e5:/page/index.mu"},
		{"lxmf link", "lxmf@a1b2c3d4e5", ActionSendMessage, "a1b2c3d4e5"},
		{"nnn shorthand", "nnn@a1b2c3d4e5", ActionOpenPage, "a1b2c3d4e5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			action, data := ResolveGuideLink(tt.target)
			if action != tt.wantType {
				t.Errorf("action = %v, want %v", action, tt.wantType)
			}
			if data != tt.wantData {
				t.Errorf("data = %q, want %q", data, tt.wantData)
			}
		})
	}
}

func TestResolveGuideLinkEmpty(t *testing.T) {
	t.Parallel()

	action, data := ResolveGuideLink("")
	if action != ActionNone {
		t.Errorf("action = %v, want ActionNone", action)
	}
	if data != "" {
		t.Errorf("data = %q, want empty", data)
	}
}

func TestResolveGuideLinkPartial(t *testing.T) {
	t.Parallel()

	action, data := ResolveGuideLink("p:section_name")
	if action != ActionPartial {
		t.Errorf("action = %v, want ActionPartial", action)
	}
	if data != "section_name" {
		t.Errorf("data = %q, want %q", data, "section_name")
	}
}

func TestGuideAnchors(t *testing.T) {
	t.Parallel()

	// Test that we can extract anchors from Micron content
	content := `>> Introduction
Some content here
>>[#getting-started] Getting Started
More content
>>[#trust-levels] Trust Levels
Trust info`

	anchors := ExtractAnchors(content)

	// Should find explicit anchors
	found := false
	for _, a := range anchors {
		if a == "getting-started" {
			found = true
		}
	}
	if !found {
		t.Errorf("ExtractAnchors() missing 'getting-started', got %v", anchors)
	}
}

func TestResolveExternalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  string
		wantURL string
		wantOK  bool
	}{
		{"https URL", "https://example.com", "https://example.com", true},
		{"http URL", "http://example.com", "http://example.com", true},
		{"not a URL", "just text", "", false},
		{"anchor", "#section", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			url, ok := ResolveExternalURL(tt.target)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && url != tt.wantURL {
				t.Errorf("url = %q, want %q", url, tt.wantURL)
			}
		})
	}
}
