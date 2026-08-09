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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// scrollWheelStep computes the target line offset for a mouse-wheel notch that
// moves mouseWheelLines rows, clamped to the valid [0, total-height] range. ok
// is false (and next is 0) when there is nothing to scroll — no content, the
// content fits the viewport, or the offset is already at the requested
// boundary; ok is true with the clamped next offset otherwise.
//
// This is the shared engine behind every scroll region's wheel multiplier
// (IndicativeListBox lists move by the multiplier directly via SetCurrentItem;
// every TextView-based region — the Guide ScrollBar, the Browser page view,
// and the bare scrollable TextViews — uses it through applyWheelMultiplier).
// Each notch scrolls N rows in ONE delivery, so a notch always redraws at most
// once. total is the TextView's own wrapped-line count (GetWrappedLineCount),
// not a separately-computed figure, so the clamp matches the offset the
// TextView actually indexes (lineOffset into lineIndex).
func scrollWheelStep(action tview.MouseAction, offset, height, total int) (next int, ok bool) {
	if total <= 0 || height <= 0 || total <= height {
		return 0, false
	}
	delta := max(1, mouseWheelLines)
	next = offset - delta
	if action == tview.MouseScrollDown {
		next = offset + delta
	}
	maxOff := total - height
	next = max(0, min(maxOff, next))
	if next == offset {
		return 0, false
	}
	return next, true
}

// applyWheelMultiplier installs a mouse capture on a scrollable *tview.TextView
// so a mouse-wheel notch scrolls mouseWheelLines rows in a single delivery via
// ScrollTo, then cancels tview's default 1-row wheel handler. It returns the
// same TextView for fluent chaining.
//
// Why a capture and not a root re-dispatch: tview's TextView wheel handler
// (textview.go MouseScrollDown) bumps lineOffset by 1 and sets trackEnd=true
// when its lazy line index is short relative to the offset; the next Draw then
// jumps lineOffset to len(lineIndex)-height (the bottom). Normally a Draw runs
// between wheel notches and rebuilds the index, so trackEnd is never set
// prematurely. A root wrapper that re-dispatches a single notch N times with NO
// Draw between deliveries breaks that — the 2nd delivery sees the shortfall
// and sets trackEnd, and the post-notch Draw leaps to the bottom. That was the
// "first scroll after a Guide topic switch jumps to the end" bug. Scrolling by
// N rows in ONE ScrollTo sidesteps the trackEnd path entirely (ScrollTo sets
// trackEnd=false), so there is no jump and exactly one redraw per notch.
//
// The capture runs before the TextView's own handler (Box.WrapMouseHandler). On
// a wheel it computes the N-row target with scrollWheelStep and returns
// MouseConsumed with a nil event when it scrolled (consumed; default 1-row
// handler skipped; tview redraws), or a nil event without MouseConsumed at a
// boundary (no-op; default skipped; no redraw). Non-wheel actions pass through
// unchanged so clicks and region highlighting still work.
//
// This keeps the *tview.TextView type (no wrapper primitive), so it is
// non-intrusive: install once at construction and all TextView behaviour
// (scrolling, regions, dynamic colors, SetText) is unchanged. Primitives that
// already override MouseHandler around a TextView (ScrollBar, browserPageView)
// delegate their wheel to the TextView's handler, so installing this capture
// on that TextView is sufficient for them too — their own boundary guard is
// then redundant (scrollWheelStep handles the boundary) and is removed.
// wheelScrollable is the subset of a scrollable *tview.TextView (or a wrapper
// that embeds one, like the Guide's guideReader) that the wheel-multiplier
// capture needs: SetMouseCapture (promoted from *tview.Box) installs the
// capture, and the getters drive scrollWheelStep + ScrollTo. Both *tview.TextView
// and *guideReader satisfy it via embedding, so NewScrollBar can install the
// capture on the Guide reader even though it is not literally *tview.TextView.
type wheelScrollable interface {
	SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse)) *tview.Box
	GetInnerRect() (int, int, int, int)
	GetScrollOffset() (int, int)
	GetWrappedLineCount() int
	ScrollTo(row, column int) *tview.TextView
}

// installWheelCapture installs the wheel-multiplier capture on any
// wheelScrollable (a *tview.TextView or a wrapper that embeds one). See
// applyWheelMultiplier for the rationale.
func installWheelCapture(ws wheelScrollable) {
	ws.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action != tview.MouseScrollUp && action != tview.MouseScrollDown {
			return action, event
		}
		_, _, _, h := ws.GetInnerRect()
		if h <= 0 {
			return action, event
		}
		row, _ := ws.GetScrollOffset()
		next, ok := scrollWheelStep(action, row, h, ws.GetWrappedLineCount())
		if !ok {
			// Boundary (or nothing to scroll): skip the default 1-row handler
			// and decline to consume so tview skips the no-op redraw.
			return tview.MouseMove, nil
		}
		ws.ScrollTo(next, 0)
		// Consumed: skip the default handler (we already scrolled by N) and let
		// tview redraw once to show the new position.
		return tview.MouseConsumed, nil
	})
}

// applyWheelMultiplier installs the wheel-multiplier capture on a scrollable
// *tview.TextView and returns it for fluent chaining. It is the
// *tview.TextView-specific entry point for bare scrollable TextViews (and the
// Browser page view); NewScrollBar uses installWheelCapture directly so it also
// covers TextView wrappers like the Guide reader.
func applyWheelMultiplier(tv *tview.TextView) *tview.TextView {
	installWheelCapture(tv)
	return tv
}
