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

package conversation

import (
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/lxmf"
)

// TestIngestNotifiesOnChangedOffTheCallerGoroutine pins the contract the
// production UI wiring depends on: ConversationCache.OnChanged is delivered
// from a goroutine of the cache's own, never from the goroutine that called
// Ingest.
//
// It matters because the client ingests the message it has just sent from
// INSIDE the tview event loop — C-d in a conversation composer reaches Ingest
// through Conversation.Send (send.go:129) — while the wiring's OnChanged
// handler marshals to the UI thread with tview's QueueUpdateDraw, which blocks
// until the event loop runs the function it queued (gmlewis/tview
// application.go:911-916: QueueUpdate sends the update and then waits on
// <-ch). A notification delivered synchronously from the event loop therefore
// waits on the event loop itself, and the client stops dead on the spot: no
// drawing, no keys, no resize, no mouse — only a console restart clears it.
// That was the reported "Ctrl-D is completely ignored" freeze.
//
// The test stands in for that wiring. The calling goroutine holds callerLock
// for the whole Ingest, as the event loop does while it is inside a key
// handler, and the notification reports which goroutine it ran on by trying
// to take the lock.
func TestIngestNotifiesOnChangedOffTheCallerGoroutine(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	lxm, err := lxmf.NewMessage(newLXMFDest(t), newLXMFDest(t), "body", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// The handler waits for the dispatch to be over, which is what the wiring
	// layer's QueueUpdateDraw does: it queues the update and waits for the event
	// loop to run it, and the loop can only get there once the handler it is
	// already in — the one that sent the message — has returned.
	dispatched := make(chan struct{})
	cc := NewConversationCache()
	cc.OnChanged = func(string) { <-dispatched }

	ingested := make(chan error, 1)
	go func() {
		_, err := cc.Ingest(lxm, dir, true)
		ingested <- err
	}()

	select {
	case err := <-ingested:
		if err != nil {
			t.Fatalf("Ingest: %v", err)
		}
		close(dispatched) // let the notification finish
	case <-time.After(2 * time.Second):
		close(dispatched) // unwedge the caller before failing
		t.Fatal("Ingest waited for OnChanged to be dispatched: notified from the " +
			"event loop, the UI wiring's QueueUpdateDraw deadlocks the client here")
	}
}
