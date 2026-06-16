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

func TestNewClickableIcon(t *testing.T) {
	t.Parallel()

	ci := NewClickableIcon("📋", nil)
	if ci == nil {
		t.Fatal("NewClickableIcon returned nil")
	}
}

func TestClickableIconClickFiresCallback(t *testing.T) {
	t.Parallel()

	var clicked bool
	ci := NewClickableIcon("📋", func() { clicked = true })

	ci.HandleMouseLeftClick()

	if !clicked {
		t.Error("ClickableIcon should fire callback on left click")
	}
}

func TestClickableIconClickNilCallback(t *testing.T) {
	t.Parallel()

	ci := NewClickableIcon("📋", nil)

	ci.HandleMouseLeftClick()
}

func TestClickableIconClickWithText(t *testing.T) {
	t.Parallel()

	var copied string
	ci := NewClickableIcon("📋", func() { copied = "test data" })

	ci.HandleMouseLeftClick()

	if copied != "test data" {
		t.Errorf("callback result = %q, want %q", copied, "test data")
	}
}

func TestOSC52CopyEmpty(t *testing.T) {
	t.Parallel()

	err := OSC52Copy("")
	if err != nil {
		t.Errorf("OSC52Copy empty string should not error: %v", err)
	}
}
