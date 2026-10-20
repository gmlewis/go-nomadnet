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

	"github.com/gmlewis/tcell/v2"
)

// mouseRecordingScreen counts the mouse-mode transitions the application asks the
// terminal to make, so a test can tell "the mouse mode was never entered" from
// "it was entered and then left".
type mouseRecordingScreen struct {
	tcell.Screen
	enableCalls  int
	disableCalls int
}

func (r *mouseRecordingScreen) EnableMouse(flags ...tcell.MouseFlags) {
	r.enableCalls++
	r.Screen.EnableMouse(flags...)
}
func (r *mouseRecordingScreen) DisableMouse() { r.disableCalls++; r.Screen.DisableMouse() }

// newMouseRecordingApp builds an App over a recording screen supplied through the
// provider seam, so construction runs exactly the terminal setup a real client
// runs. Not parallel: SetScreenProvider is process-wide state.
func newMouseRecordingApp(t *testing.T, opts ...Option) (*App, *mouseRecordingScreen) {
	t.Helper()
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatalf("simulation screen init: %v", err)
	}
	rec := &mouseRecordingScreen{Screen: sim}
	SetScreenProvider(func() (tcell.Screen, error) { return rec, nil })
	t.Cleanup(func() {
		SetScreenProvider(nil)
		sim.Fini()
	})
	return NewApp(ThemeDark, GlyphUnicode, ColorModeTrue, opts...), rec
}

// TestMouseEnabledGatesTerminalMouseMode pins that [textui] mouse_enabled
// controls the terminal mouse mode, which Python passes to urwid as
// `handle_mouse=mouse_enabled` (ui/TextUI.py:201,229; default true). With the
// key disabled the terminal must never enter mouse mode, so clicks do nothing.
func TestMouseEnabledGatesTerminalMouseMode(t *testing.T) {
	tests := []struct {
		name          string
		opts          []Option
		wantMouseMode bool
	}{
		{"default is on, as the reference default is", nil, true},
		{"explicitly enabled", []Option{WithMouseEnabled(true)}, true},
		{"disabled", []Option{WithMouseEnabled(false)}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, rec := newMouseRecordingApp(t, tc.opts...)

			// tview only reaches the screen when the requested state differs from
			// its own initially-false state, so "enabled" is exactly one
			// EnableMouse call and "disabled" is no call at all — the mouse mode is
			// never entered, which is what makes a click do nothing.
			if tc.wantMouseMode && rec.enableCalls == 0 {
				t.Error("the terminal was never put into mouse mode for an enabled config")
			}
			if tc.wantMouseMode && rec.disableCalls != 0 {
				t.Errorf("DisableMouse called %v times for an enabled config", rec.disableCalls)
			}
			if !tc.wantMouseMode && rec.enableCalls != 0 {
				t.Error("the terminal was put into mouse mode although the config disables the mouse")
			}
		})
	}
}
