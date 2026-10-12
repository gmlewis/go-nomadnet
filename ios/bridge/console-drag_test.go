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

package bridge

import (
	"testing"
	"time"

	"github.com/gmlewis/tcell/v2"
)

// TestADragArrivesAtTheClientAsAWheelEvent is the appliance's touch-drag contract,
// asked where it can be answered without a device.
//
// A drag has nowhere to go but the client's own mouse handling: a terminal has no
// gesture, so the app turns the drag into the wheel reports a real terminal would
// have sent, at the cell the finger is on. What the client does with them is its
// own business — it scrolls whatever it has at that cell — so the appliance's
// whole half of the bargain is these bytes: the SGR wheel report, at the finger's
// cell, understood as a wheel turn in the direction the finger went.
//
// The bytes are written here exactly as the app's TerminalMouse.sgr builds them,
// because a disagreement between the two halves is a console that cannot be
// dragged and reports nothing.
func TestADragArrivesAtTheClientAsAWheelEvent(t *testing.T) {
	t.Parallel()
	screen, app := openConsoleSession(t, 80, 24)
	screen.EnableMouse()
	screen.Sync()

	// What the client makes of a report is read from its own event loop, which parks
	// until something arrives: the events are collected on their own goroutine so that a
	// drag which never arrives is a failure rather than a test that hangs.
	events := make(chan tcell.Event, 8)
	go func() {
		for {
			event := screen.PollEvent()
			if event == nil {
				return
			}
			events <- event
		}
	}()

	// Initializing the terminal reports its size, which is the client's business and not the
	// drag's: the reports below are read from an empty queue.
	deadline := time.After(300 * time.Millisecond)
drain:
	for {
		select {
		case <-events:
		case <-deadline:
			break drain
		}
	}

	for _, tc := range []struct {
		name   string
		report string
		want   tcell.ButtonMask
	}{
		{"a finger moving up asks for the wheel's down", "\x1b[<65;10;5M", tcell.WheelDown},
		{"a finger moving down asks for the wheel's up", "\x1b[<64;10;5M", tcell.WheelUp},
	} {
		if err := WriteFrame(app.conn, Frame{Type: FrameData, Data: []byte(tc.report)}); err != nil {
			t.Fatalf("%v: writing the report: %v", tc.name, err)
		}
		select {
		case event := <-events:
			mouse, ok := event.(*tcell.EventMouse)
			if !ok {
				t.Fatalf("%v: the client saw %T, want a mouse event", tc.name, event)
			}
			if got := mouse.Buttons(); got != tc.want {
				t.Errorf("%v: the client saw buttons %v, want %v", tc.name, got, tc.want)
			}
			if x, y := mouse.Position(); x != 9 || y != 4 {
				t.Errorf("%v: the client saw the wheel at %v,%v, want the cell the report named, 9,4", tc.name, x, y)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%v: the client never saw the wheel report", tc.name)
		}
	}
}
