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
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// ingestMsg builds a packed LXMF message from the local destination, ready for
// ConversationCache.Ingest.
func ingestMsg(t *testing.T, ts *rns.TransportSystem, dest *rns.Destination, content string) *lxmf.Message {
	t.Helper()
	msg, err := lxmf.NewMessage(dest, dest, content, "title", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	return msg
}

// newIngestFixture returns a transport system and a destination for building
// messages to ingest.
func newIngestFixture(t *testing.T) (*rns.TransportSystem, *rns.Destination) {
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
	return ts, dest
}

// TestIngestAccumulatesUnreadCount pins Python's unread accounting
// (Conversation.py:87-98): each inbound message increments a per-conversation
// counter and the *number* is written to the `unread` flag file, so three
// arrivals leave the file holding "3" and the conversation list renders "(3)".
func TestIngestAccumulatesUnreadCount(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts, dest := newIngestFixture(t)
	cache := NewConversationCache()

	var sourceHex string
	for i := range 3 {
		msg := ingestMsg(t, ts, dest, "message "+strconv.Itoa(i))
		if _, err := cache.Ingest(msg, dir, false); err != nil {
			t.Fatalf("Ingest %v: %v", i, err)
		}
		sourceHex = hex.EncodeToString(msg.SourceHash)
	}

	unreadPath := filepath.Join(dir, sourceHex, "unread")
	data, err := os.ReadFile(unreadPath)
	if err != nil {
		t.Fatalf("reading unread flag: %v", err)
	}
	if got, want := string(data), "3"; got != want {
		t.Errorf("unread flag = %q, want %q", got, want)
	}
	if got, want := cache.UnreadCount(sourceHex), 3; got != want {
		t.Errorf("cache UnreadCount = %v, want %v", got, want)
	}

	// The count must survive the reader that renders the badge.
	list := ConversationList(dir, nil, nil, nil)
	for _, ci := range list {
		if ci.SourceHash == sourceHex && ci.UnreadCount != 3 {
			t.Errorf("ConversationList unread count = %v, want 3", ci.UnreadCount)
		}
	}
	if !HasUnreadConversations(dir) {
		t.Error("HasUnreadConversations = false, want true")
	}
}

// TestIngestWritesFailedFlagForFailedOutbound pins Python's failed accounting
// (Conversation.py:100-111): an OUTBOUND message whose LXMF state is FAILED or
// REJECTED increments a per-conversation failure counter and writes it to the
// `failed` flag file, driving the failed badge, the warning glyph and the
// failed count.
func TestIngestWritesFailedFlagForFailedOutbound(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state int
		want  bool
	}{
		{"failed", lxmf.StateFailed, true},
		{"rejected", lxmf.StateRejected, true},
		{"sent", lxmf.StateSent, false},
		{"delivered", lxmf.StateDelivered, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := tempDir(t)
			ts, dest := newIngestFixture(t)
			cache := NewConversationCache()

			msg := ingestMsg(t, ts, dest, "outbound body")
			msg.SetState(tc.state)
			if _, err := cache.Ingest(msg, dir, true); err != nil {
				t.Fatalf("Ingest: %v", err)
			}
			sourceHex := hex.EncodeToString(msg.DestinationHash)

			failedPath := filepath.Join(dir, sourceHex, "failed")
			_, err := os.Stat(failedPath)
			if tc.want && err != nil {
				t.Fatalf("failed flag missing for a %v outbound message: %v", tc.name, err)
			}
			if !tc.want {
				if err == nil {
					t.Error("failed flag written for a non-failing outbound message")
				}
				if cache.IsFailed(sourceHex) {
					t.Error("cache reports failed for a non-failing outbound message")
				}
				return
			}

			data, err := os.ReadFile(failedPath)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(data), "1"; got != want {
				t.Errorf("failed flag = %q, want %q", got, want)
			}
			if got, want := cache.FailedCount(sourceHex), 1; got != want {
				t.Errorf("cache FailedCount = %v, want %v", got, want)
			}

			// A second failure on the same conversation accumulates.
			msg2 := ingestMsg(t, ts, dest, "outbound body two")
			msg2.SetState(tc.state)
			if _, err := cache.Ingest(msg2, dir, true); err != nil {
				t.Fatalf("Ingest 2: %v", err)
			}
			data, err = os.ReadFile(failedPath)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(data), "2"; got != want {
				t.Errorf("failed flag after second failure = %q, want %q", got, want)
			}

			list := ConversationList(dir, nil, nil, nil)
			for _, ci := range list {
				if ci.SourceHash == sourceHex && ci.FailedCount != 2 {
					t.Errorf("ConversationList failed count = %v, want 2", ci.FailedCount)
				}
			}
		})
	}
}

// TestIngestInboundFailureIsNotCountedAsFailed pins the branch structure of
// Python's ingest (Conversation.py:87-111): `if not originator: ... elif
// is_failed:` — the unread branch and the failed branch are mutually exclusive,
// so an inbound message is never counted as a failure even when its state says
// FAILED. Only `originator` messages reach the failed branch.
func TestIngestInboundFailureIsNotCountedAsFailed(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts, dest := newIngestFixture(t)
	cache := NewConversationCache()

	msg := ingestMsg(t, ts, dest, "inbound body")
	msg.SetState(lxmf.StateFailed)
	if _, err := cache.Ingest(msg, dir, false); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	sourceHex := hex.EncodeToString(msg.SourceHash)

	if fileExists(filepath.Join(dir, sourceHex, "failed")) {
		t.Error("inbound message wrote a failed flag; Python's elif keeps the branches exclusive")
	}
	if !fileExists(filepath.Join(dir, sourceHex, "unread")) {
		t.Error("inbound message did not write an unread flag")
	}
}

// TestIngestUnreadCountContinuesFromDisk pins the disk-seeded half of Python's
// unread accounting. Python keeps the counter in a class-level dict that
// conversation_list seeds from the flag file's contents (Conversation.py:139),
// so a restart does not reset the tally: a conversation already holding "2"
// goes to "3" on the next arrival, not back to "1".
//
// The seed is read on the first increment, so Ingest keeps the steady-state
// count even when it runs before any conversation listing.
func TestIngestUnreadCountContinuesFromDisk(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts, dest := newIngestFixture(t)
	cache := NewConversationCache()

	// Prime the conversation on disk as a running client would leave it.
	first := ingestMsg(t, ts, dest, "before restart")
	if _, err := cache.Ingest(first, dir, false); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	sourceHex := hex.EncodeToString(first.SourceHash)
	unreadPath := filepath.Join(dir, sourceHex, "unread")
	if err := os.WriteFile(unreadPath, []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A fresh cache stands in for the restart.
	restarted := NewConversationCache()
	second := ingestMsg(t, ts, dest, "after restart")
	if _, err := restarted.Ingest(second, dir, false); err != nil {
		t.Fatalf("Ingest after restart: %v", err)
	}

	data, err := os.ReadFile(unreadPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "3"; got != want {
		t.Errorf("unread flag after restart = %q, want %q", got, want)
	}
}
