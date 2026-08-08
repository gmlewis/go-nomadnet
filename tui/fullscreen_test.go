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

func TestFullscreenToggle(t *testing.T) {
	t.Parallel()

	fs := FullscreenState{ListWidth: 52}
	if fs.IsFullscreen() {
		t.Error("should not be fullscreen initially")
	}

	fs.Toggle()
	if !fs.IsFullscreen() {
		t.Error("should be fullscreen after toggle")
	}
	if fs.ListWidth != 0 {
		t.Errorf("ListWidth = %v, want 0", fs.ListWidth)
	}

	fs.Toggle()
	if fs.IsFullscreen() {
		t.Error("should not be fullscreen after second toggle")
	}
	if fs.ListWidth != 52 {
		t.Errorf("ListWidth = %v, want 52", fs.ListWidth)
	}
}

func TestFullscreenToggleCustomWidth(t *testing.T) {
	t.Parallel()

	fs := FullscreenState{ListWidth: 40}
	fs.Toggle()
	if fs.ListWidth != 0 {
		t.Errorf("ListWidth = %v, want 0", fs.ListWidth)
	}
	fs.Toggle()
	if fs.ListWidth != 40 {
		t.Errorf("ListWidth = %v, want 40", fs.ListWidth)
	}
}

func TestFullscreenToggleMultipleTimes(t *testing.T) {
	t.Parallel()

	fs := FullscreenState{ListWidth: 52}
	for range 10 {
		fs.Toggle()
	}
	// After 10 toggles (even number), should be back to normal
	if fs.IsFullscreen() {
		t.Error("should not be fullscreen after even number of toggles")
	}
	if fs.ListWidth != 52 {
		t.Errorf("ListWidth = %v, want 52", fs.ListWidth)
	}
}

func TestFullscreenSetWidth(t *testing.T) {
	t.Parallel()

	fs := FullscreenState{ListWidth: 52}
	fs.SetWidth(60)
	if fs.ListWidth != 60 {
		t.Errorf("ListWidth = %v, want 60", fs.ListWidth)
	}
}
