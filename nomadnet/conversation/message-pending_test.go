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
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// writeLXMFixtureWithState writes an LXMF message with the given wire state to
// dir and returns its on-disk path + hash.
func writeLXMFixtureWithState(t *testing.T, dir string, state int) (path string, hash []byte) {
	t.Helper()
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := lxmf.NewMessage(dest, dest, "body", "title", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	msg.SetState(state)
	msg.SetMethod(lxmf.MethodDirect)
	written, err := msg.WriteToDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	return written, msg.Hash
}

// TestMessageLoadPendingNotQueuedMarksFailed mirrors Python's
// ConversationMessage.load (Conversation.py:451-460): a message on disk whose
// LXMF state is between GENERATING and SENT (OUTBOUND, SENDING) that is NOT in
// the router's pending-outbound / pending-deferred-stamps queue is marked
// FAILED on load. Without this, interrupted outbound messages render with the
// wrong header glyph (the default "→" branch instead of "✕ →" FAILED), which
// is B16's "→ for sent" symptom on gonomadnet.
func TestMessageLoadPendingNotQueuedMarksFailed(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	dir := tempDir(t)
	convDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, _ := writeLXMFixtureWithState(t, convDir, lxmf.StateOutbound)

	m := NewMessageWithTransport(path, ts)
	// PendingChecker reports the hash is NOT in the pending queue.
	m.PendingChecker = func(hash []byte) bool { return false }
	m.Load()

	if got := m.CachedRawState; got != lxmf.StateFailed {
		t.Errorf("raw state = %#x, want %#x (FAILED) for unqueued pending message", got, lxmf.StateFailed)
	}
	if got := m.GetState(); got != StateFailed {
		t.Errorf("mapped state = %v, want StateFailed", got)
	}
}

// TestMessageLoadPendingQueuedKeepsState verifies that a pending-state message
// that IS still in the router queue keeps its original state (not marked
// FAILED).
func TestMessageLoadPendingQueuedKeepsState(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	dir := tempDir(t)
	convDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, hash := writeLXMFixtureWithState(t, convDir, lxmf.StateOutbound)

	m := NewMessageWithTransport(path, ts)
	m.PendingChecker = func(h []byte) bool {
		if len(h) != len(hash) {
			return false
		}
		for i := range h {
			if h[i] != hash[i] {
				return false
			}
		}
		return true
	}
	m.Load()

	if got := m.CachedRawState; got != lxmf.StateOutbound {
		t.Errorf("raw state = %#x, want %#x (OUTBOUND) for queued pending message", got, lxmf.StateOutbound)
	}
	if got := m.GetState(); got != StatePending {
		t.Errorf("mapped state = %v, want StatePending", got)
	}
}

// TestMessageLoadSentStateUnaffectedByPendingCheck verifies that a SENT
// message (state >= SENT) is never marked FAILED by the pending check, since
// Python's guard is `state > GENERATING and state < SENT`.
func TestMessageLoadSentStateUnaffectedByPendingCheck(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	dir := tempDir(t)
	convDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, _ := writeLXMFixtureWithState(t, convDir, lxmf.StateSent)

	m := NewMessageWithTransport(path, ts)
	m.PendingChecker = func(hash []byte) bool { return false }
	m.Load()

	if got := m.CachedRawState; got != lxmf.StateSent {
		t.Errorf("raw state = %#x, want %#x (SENT) — pending check must not touch SENT messages", got, lxmf.StateSent)
	}
}

// TestMessageLoadNoPendingCheckerKeepsState verifies that when no
// PendingChecker is wired (e.g. headless/tests), a pending-state message keeps
// its on-disk state rather than being eagerly marked FAILED — matching
// Python's behavior only when the shared instance / router is unavailable
// (the lookup is skipped).
func TestMessageLoadNoPendingCheckerKeepsState(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	dir := tempDir(t)
	convDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, _ := writeLXMFixtureWithState(t, convDir, lxmf.StateOutbound)

	m := NewMessageWithTransport(path, ts)
	m.Load()

	if got := m.CachedRawState; got != lxmf.StateOutbound {
		t.Errorf("raw state = %#x, want %#x (OUTBOUND) when no PendingChecker wired", got, lxmf.StateOutbound)
	}
}
