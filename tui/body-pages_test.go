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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// focusRecorder is a minimal focusable primitive whose InputHandler records
// whether it received a key event. It lets the bodyPages dispatch test observe
// exactly which page tview routed a key to, independent of tview.List's own
// key handling. Its HasFocus/Focus/Blur are externally controllable so the test
// can model a hidden page that retains stale focus (the root-cause condition:
// Application.SetFocus only blurs the single focused leaf, not the previously
// displayed page's containers, so a hidden page's widgets keep HasFocus()==true
// after SwitchToPage).
type focusRecorder struct {
	*tview.Box
	focused bool
	gotKey  bool
}

func newFocusRecorder() *focusRecorder {
	return &focusRecorder{Box: tview.NewBox()}
}

func (r *focusRecorder) Focus(delegate func(p tview.Primitive)) { r.focused = true }
func (r *focusRecorder) Blur()                                  { r.focused = false }
func (r *focusRecorder) HasFocus() bool                         { return r.focused }
func (r *focusRecorder) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		r.gotKey = true
	}
}

// TestBodyPagesDispatchesOnlyToVisible is the red-green test for the Guide
// focus-stealing root cause. tview.Pages.InputHandler (pages.go:329-340)
// dispatches a key to the FIRST page whose Item.HasFocus() is true, iterating
// ALL pages regardless of visibility. Because Application.SetFocus only blurs
// the single focused leaf (application.go:838-855) — not the previously
// displayed page's containers — a hidden page retains stale HasFocus()==true
// after SwitchToPage, so its widget tree steals keys from the now-visible page.
// bodyPages overrides InputHandler to dispatch only to a page that is BOTH
// visible and HasFocus, mirroring Python urwid's single Frame.focus_position
// (only the visible body receives input).
func TestBodyPagesDispatchesOnlyToVisible(t *testing.T) {
	bp := newBodyPages()
	hidden := newFocusRecorder()  // "Network" page — focused then left for Guide
	visible := newFocusRecorder() // "Guide" page — currently displayed

	bp.AddPage("network", hidden, true, false)
	bp.AddPage("guide", visible, true, false)

	// Phase 3: focus the network page's widget (it becomes the focused leaf).
	bp.SwitchToPage("network")
	hidden.focused = true

	// Phase 4: switch to the guide page WITHOUT blurring the network page's
	// widget — this is exactly what MainDisplay.selectMenuLocked does
	// (SwitchToPage only re-focuses when Pages itself HasFocus, and on a menu
	// Enter focus stays in the menu, not the body). The hidden page retains
	// stale HasFocus()==true. FocusBody then focuses the guide page's widget.
	bp.SwitchToPage("guide")
	visible.focused = true
	// hidden.focused is STILL true (stale) — the bug condition.

	// Dispatch a key. With the buggy tview.Pages.InputHandler the first page
	// with HasFocus (network, slice order) receives it. bodyPages must route it
	// to the VISIBLE page (guide) instead.
	bp.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(p tview.Primitive) {})

	if hidden.gotKey {
		t.Fatalf("hidden page received the key — focus leaked across pages (the Guide bug)")
	}
	if !visible.gotKey {
		t.Fatalf("visible page did not receive the key — bodyPages misrouted input")
	}
}

// TestBodyPagesNoVisibleFocusIsNoOp asserts that when no visible page has focus,
// bodyPages does not dispatch to a hidden page that does have (stale) focus.
func TestBodyPagesNoVisibleFocusIsNoOp(t *testing.T) {
	bp := newBodyPages()
	hidden := newFocusRecorder()
	visible := newFocusRecorder()

	bp.AddPage("network", hidden, true, false)
	bp.AddPage("guide", visible, true, false)

	bp.SwitchToPage("network")
	hidden.focused = true
	bp.SwitchToPage("guide")
	// Only the hidden page has focus; the visible page does not.
	hidden.focused = true
	visible.focused = false

	bp.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(p tview.Primitive) {})

	if hidden.gotKey {
		t.Fatalf("hidden page with stale focus received a key while no visible page had focus")
	}
}
