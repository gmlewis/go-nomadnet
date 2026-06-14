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

	"github.com/rivo/tview"
)

func TestNewChannelsDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	rooms := []ChannelInfo{
		{Name: "general", Members: 10, Joined: true},
		{Name: "random", Members: 5, Unread: true},
	}

	cd := NewChannelsDisplay(app, rooms)
	if cd == nil {
		t.Fatal("NewChannelsDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestChannelsDisplayEmpty(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewChannelsDisplay(app, nil)

	if cd == nil {
		t.Fatal("NewChannelsDisplay with empty list returned nil")
	}
}

func TestChannelsDisplayShowMessages(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewChannelsDisplay(app, nil)

	msgs := []ChannelMessage{
		{Nick: "Alice", Text: "Hello!", IsSelf: false},
		{Nick: "Bob", Text: "Hi there!", IsSelf: true},
		{Text: "System message", IsSystem: true},
	}

	// Should not panic
	cd.ShowMessages(msgs)
}

func TestChannelsDisplayShowMembers(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewChannelsDisplay(app, nil)

	members := []ChannelMember{
		{Nick: "Alice", Hash: "abc123def456", Online: true},
		{Nick: "Bob", Hash: "789ghi012jkl", Online: false},
	}

	// Should not panic
	cd.ShowMembers(members)
}

func TestNickColor(t *testing.T) {
	t.Parallel()

	color1 := nickColor("Alice")
	color2 := nickColor("Bob")

	if color1 == "" {
		t.Error("nickColor returned empty string")
	}
	if color2 == "" {
		t.Error("nickColor returned empty string")
	}

	// Same nick should always return same color
	if nickColor("Alice") != color1 {
		t.Error("nickColor not deterministic")
	}
}

func TestNickColorDifferent(t *testing.T) {
	t.Parallel()

	// Different nicks should (usually) get different colors
	color1 := nickColor("Alice12345")
	color2 := nickColor("Bob67890")
	// Not guaranteed to be different, but shouldn't panic
	_ = color1
	_ = color2
}
