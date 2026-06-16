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

func TestChannelsExpandGutterCreation(t *testing.T) {
	t.Parallel()

	g := NewChannelsExpandGutter(func() {})
	if g == nil {
		t.Fatal("NewChannelsExpandGutter returned nil")
	}
}

func TestChannelsExpandGutterToggleOnMouseLeftClick(t *testing.T) {
	t.Parallel()

	var toggled bool
	g := NewChannelsExpandGutter(func() { toggled = true })

	g.HandleMouseLeftClick()

	if !toggled {
		t.Error("ChannelsExpandGutter should call toggle on left click")
	}
}

func TestChannelsExpandGutterExpandedState(t *testing.T) {
	t.Parallel()

	g := NewChannelsExpandGutter(nil)

	if !g.Expanded() {
		t.Error("ChannelsExpandGutter should start expanded")
	}

	g.SetExpanded(false)
	if g.Expanded() {
		t.Error("SetExpanded(false) should make Expanded() return false")
	}

	g.SetExpanded(true)
	if !g.Expanded() {
		t.Error("SetExpanded(true) should make Expanded() return true")
	}
}

func TestUsersExpandGutterCreation(t *testing.T) {
	t.Parallel()

	g := NewUsersExpandGutter(func() {})
	if g == nil {
		t.Fatal("NewUsersExpandGutter returned nil")
	}
}

func TestUsersExpandGutterToggleOnMouseLeftClick(t *testing.T) {
	t.Parallel()

	var toggled bool
	g := NewUsersExpandGutter(func() { toggled = true })

	g.HandleMouseLeftClick()

	if !toggled {
		t.Error("UsersExpandGutter should call toggle on left click")
	}
}

func TestUsersExpandGutterExpandedState(t *testing.T) {
	t.Parallel()

	g := NewUsersExpandGutter(nil)

	if !g.Expanded() {
		t.Error("UsersExpandGutter should start expanded")
	}

	g.SetExpanded(false)
	if g.Expanded() {
		t.Error("SetExpanded(false) should make Expanded() return false")
	}
}

func TestChannelsExpandGutterDirection(t *testing.T) {
	t.Parallel()

	g := NewChannelsExpandGutter(nil)
	if g.Direction() != GutterRight {
		t.Error("ChannelsExpandGutter should have GutterRight direction")
	}
}

func TestUsersExpandGutterDirection(t *testing.T) {
	t.Parallel()

	g := NewUsersExpandGutter(nil)
	if g.Direction() != GutterLeft {
		t.Error("UsersExpandGutter should have GutterLeft direction")
	}
}
