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

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

func TestLinkClickTrackerEmpty(t *testing.T) {
	t.Parallel()
	nodes := []*micron.Node{}
	tracker := NewLinkClickTracker(nodes)
	if got := tracker.FindLinkAtPosition(0); got != "" {
		t.Errorf("FindLinkAtPosition(0) = %q, want empty", got)
	}
}

func TestLinkClickTrackerSingleLink(t *testing.T) {
	t.Parallel()
	nodes := []*micron.Node{
		{Type: micron.NodeText, Text: "Hello "},
		{Type: micron.NodeLink, LinkURL: "lxmf@abcd1234"},
		{Type: micron.NodeText, Text: " world"},
	}
	tracker := NewLinkClickTracker(nodes)

	// "Hello " = 6 visible chars, link "lxmf@abcd1234" = 13 visible chars
	// Link spans visible positions [6, 19)
	tests := []struct {
		pos  int
		want string
	}{
		{0, ""},
		{4, ""},
		{5, ""},
		{6, "lxmf@abcd1234"},
		{10, "lxmf@abcd1234"},
		{14, "lxmf@abcd1234"},
		{15, "lxmf@abcd1234"},
		{18, "lxmf@abcd1234"},
		{19, ""},
		{20, ""},
	}

	for _, tt := range tests {
		got := tracker.FindLinkAtPosition(tt.pos)
		if got != tt.want {
			t.Errorf("FindLinkAtPosition(%d) = %q, want %q", tt.pos, got, tt.want)
		}
	}
}

func TestLinkClickTrackerMultipleLinks(t *testing.T) {
	t.Parallel()
	nodes := []*micron.Node{
		{Type: micron.NodeLink, LinkURL: "page1"},
		{Type: micron.NodeText, Text: "     "},
		{Type: micron.NodeLink, LinkURL: "lxmf@hash1"},
		{Type: micron.NodeText, Text: "  "},
		{Type: micron.NodeLink, LinkURL: "#room1"},
	}
	tracker := NewLinkClickTracker(nodes)

	// "page1"=5, "     "=5, "lxmf@hash1"=10, "  "=2, "#room1"=6
	// page1: [0,5), lxmf@hash1: [10,20), #room1: [22,28)
	tests := []struct {
		pos  int
		want string
	}{
		{0, "page1"},
		{4, "page1"},
		{5, ""},
		{10, "lxmf@hash1"},
		{19, "lxmf@hash1"},
		{20, ""},
		{22, "#room1"},
		{27, "#room1"},
		{28, ""},
	}

	for _, tt := range tests {
		got := tracker.FindLinkAtPosition(tt.pos)
		if got != tt.want {
			t.Errorf("FindLinkAtPosition(%d) = %q, want %q", tt.pos, got, tt.want)
		}
	}
}

func TestLinkClickTrackerWithHeadings(t *testing.T) {
	t.Parallel()
	nodes := []*micron.Node{
		{Type: micron.NodeHeading, Level: 1, Children: []*micron.Node{
			{Type: micron.NodeText, Text: "Title "},
			{Type: micron.NodeLink, LinkURL: "http://example.com"},
			{Type: micron.NodeText, Text: " here"},
		}},
		{Type: micron.NodeText, Text: "\nBody text."},
	}
	tracker := NewLinkClickTracker(nodes)

	// "Title " = 6, link "http://example.com" = 18, " here" = 5
	// Link at visible positions [6, 24)
	got := tracker.FindLinkAtPosition(15)
	if got != "http://example.com" {
		t.Errorf("FindLinkAtPosition(15) = %q, want http://example.com", got)
	}
}

func TestLinkClickTrackerSetLinks(t *testing.T) {
	t.Parallel()
	tracker := &LinkClickTracker{}
	if len(tracker.Links()) != 0 {
		t.Error("new tracker should have no links")
	}
	tracker.SetLinks([]LinkPosition{{Start: 0, End: 5, Target: "test"}})
	if len(tracker.Links()) != 1 {
		t.Errorf("after SetLinks, got %d links, want 1", len(tracker.Links()))
	}
}
