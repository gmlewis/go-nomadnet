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

// These tests cover the render-gate / request-tracking layer added to
// BrowserDisplay to fix the "stale Connect overtakes the current page" race.
// Each new Connect/URL-load/link-click calls beginRequest, which cancels any
// in-flight fetch and bumps a monotonic sequence number; the fetch goroutine
// captures ctx+seq BY VALUE and, inside its QueueUpdateDraw callback, drops the
// render when seq no longer equals CurrentRequestSeq() or ctx was cancelled.
//
// The fields are event-loop-confined, so these tests call the methods directly
// (no tview event loop) — the cancellation closes the Done channel, making the
// assertions deterministic without any sleep.

// TestBeginRequestCancelsPriorAndBumpsSeq verifies that a second beginRequest
// cancels the first request's context and advances the sequence number, so a
// fetch still in flight under the first ctx observes a stale seq + a cancelled
// ctx and drops its render.
func TestBeginRequestCancelsPriorAndBumpsSeq(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)

	ctx1, seq1 := bd.BeginRequest()
	if ctx1 == nil {
		t.Fatal("ctx1 is nil")
	}
	if seq1 != 1 {
		t.Fatalf("seq1 = %d, want 1", seq1)
	}

	ctx2, seq2 := bd.BeginRequest()
	if seq2 != 2 {
		t.Fatalf("seq2 = %d, want 2", seq2)
	}
	// ctx1 must be cancelled by the second beginRequest.
	<-ctx1.Done()
	if ctx1.Err() == nil {
		t.Fatal("ctx1 not cancelled by second beginRequest")
	}
	// ctx2 must still be live.
	if ctx2.Err() != nil {
		t.Fatal("ctx2 cancelled prematurely")
	}

	curCtx, curSeq := bd.CurrentRequest()
	if curSeq != 2 {
		t.Fatalf("CurrentRequest seq = %d, want 2", curSeq)
	}
	if curCtx != ctx2 {
		t.Fatal("CurrentRequest ctx is not ctx2")
	}
	if bd.CurrentRequestSeq() != 2 {
		t.Fatalf("CurrentRequestSeq = %d, want 2", bd.CurrentRequestSeq())
	}
}

// TestCancelRequestInvalidatesPendingSeq verifies that CancelRequest (called by
// Disconnect) cancels the in-flight ctx AND bumps the seq, so a late render
// arriving after Disconnect is dropped even though no new request began.
func TestCancelRequestInvalidatesPendingSeq(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	ctx1, seq1 := bd.BeginRequest()

	bd.CancelRequest()
	<-ctx1.Done()
	if ctx1.Err() == nil {
		t.Fatal("ctx1 not cancelled by CancelRequest")
	}
	if bd.CurrentRequestSeq() == seq1 {
		t.Fatalf("seq unchanged after CancelRequest: %d", seq1)
	}
	if bd.CurrentRequestSeq() != seq1+1 {
		t.Fatalf("CurrentRequestSeq = %d, want %d", bd.CurrentRequestSeq(), seq1+1)
	}
}

// TestRenderGateDropsStaleSeq simulates the exact gate condition used at the top
// of every QueueUpdateDraw callback in the OnRetrieveURL wiring: a stale
// request's captured seq must differ from CurrentRequestSeq() once a newer
// request has begun, while the current request's seq must match.
func TestRenderGateDropsStaleSeq(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	_, seqA := bd.BeginRequest()
	_, seqB := bd.BeginRequest() // B overtakes A

	// A's late callback fires — its seq must NOT match current.
	if seqA == bd.CurrentRequestSeq() {
		t.Fatal("stale A seq equals current; gate would wrongly render A")
	}
	// B's callback fires — its seq must match current.
	if seqB != bd.CurrentRequestSeq() {
		t.Fatal("current B seq does not match CurrentRequestSeq; gate would drop B")
	}
}
