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

package main

import (
	"bytes"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rrc"
	"github.com/gmlewis/go-reticulum/rrc/cbor"
)

// newPrivateRenderHub builds a client hub with a real identity and one joined
// room, so the room view can be rendered for assertions.
func newPrivateRenderHub(t *testing.T) (*rrc.RRCHub, []byte) {
	t.Helper()
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatalf("rns.NewIdentity: %v", err)
	}
	mgr := rrc.NewManager(tempDir(t), nil)
	t.Cleanup(mgr.Shutdown)
	mgr.SetIdentity(id)
	hub := mgr.AddHub(bytes.Repeat([]byte{0x0a}, 16), "rrc.hub", "TestHub")
	hub.AddRoom("general")
	return hub, id.Hash
}

// TestRRCoomMessagesIncludesPrivateNotices asserts the room view a client
// renders carries the private NOTICEs the hub delivered to this client alone:
// Python's RRC.py records them in self.notices and never renders them, which is
// why a private message was invisible before. A room row stays unmarked.
func TestRRCoomMessagesIncludesPrivateNotices(t *testing.T) {
	t.Parallel()

	hub, own := newPrivateRenderHub(t)

	peer := bytes.Repeat([]byte{0x60}, 16)

	feedEnvelope(t, hub, rrc.MakeClientEnvelope(rrc.TypeMsg, peer, []byte("general"), []byte("Alice"), "room news", make([]byte, 8), rrc.NowMs()))
	private := rrc.MakeClientEnvelope(rrc.TypeNotice, peer, nil, []byte("Alice"), "psst, private", make([]byte, 8), rrc.NowMs())
	private[rrc.KeyDst] = own
	feedEnvelope(t, hub, private)

	msgs := rrcRoomMessages(hub, "general")

	var roomRows, privateRows int
	for _, m := range msgs {
		if m.IsPrivate {
			privateRows++
			if m.Nick != "Alice" || m.Text != "psst, private" {
				t.Errorf("private row = %+v, want Alice's private notice", m)
			}
			continue
		}
		if m.Room == "general" {
			roomRows++
		}
	}
	if privateRows != 1 {
		t.Errorf("room view has %v private rows, want 1 (rows: %+v)", privateRows, msgs)
	}
	if roomRows != 1 {
		t.Errorf("room view has %v room rows, want 1", roomRows)
	}
	for _, m := range msgs {
		if !m.IsPrivate && m.Text == "psst, private" {
			t.Errorf("the private notice also rendered as room traffic: %+v", m)
		}
	}
}

// TestRRCoomMessagesMarksOwnPrivateNoticesAsSelf asserts the echo half: a
// private notice this client sent comes back from the hub credited to this
// client, and renders as "private to".
func TestRRCoomMessagesMarksOwnPrivateNoticesAsSelf(t *testing.T) {
	t.Parallel()

	hub, own := newPrivateRenderHub(t)
	private := rrc.MakeClientEnvelope(rrc.TypeNotice, own, nil, []byte("Me"), "hi there", make([]byte, 8), rrc.NowMs())
	private[rrc.KeyDst] = bytes.Repeat([]byte{0x60}, 16)
	feedEnvelope(t, hub, private)

	msgs := rrcRoomMessages(hub, "general")
	if len(msgs) != 1 || !msgs[0].IsPrivate || !msgs[0].IsSelf {
		t.Fatalf("room view = %+v, want one private self row", msgs)
	}
}

// feedEnvelope drives one inbound envelope through the client hub's decode
// path.
func feedEnvelope(t *testing.T, hub *rrc.RRCHub, env map[any]any) {
	t.Helper()
	hub.HandleData(cbor.Encode(env))
}

// TestRRCoomMessagesRenderTheTypedEchoAsSelf asserts the client's own copy of a
// /msg line is rendered like any other message the local user sent: the room
// view is rebuilt from the hub buffer, so the echo must live there and resolve
// to the local user's own identity.
func TestRRCoomMessagesRenderTheTypedEchoAsSelf(t *testing.T) {
	t.Parallel()

	hub, _ := newPrivateRenderHub(t)
	hub.SetNickOverride("glenn")
	const line = "/msg gorrcbot help"
	hub.AddLocalSelfMessage("general", hub.GetEffectiveNick(), line)

	msgs := rrcRoomMessages(hub, "general")
	if len(msgs) != 1 {
		t.Fatalf("room view = %+v, want the typed echo", msgs)
	}
	if msgs[0].Text != line {
		t.Errorf("row text = %q, want the unaltered typed line", msgs[0].Text)
	}
	if !msgs[0].IsSelf {
		t.Errorf("row = %+v, want it credited to the local user", msgs[0])
	}
	if msgs[0].Nick != "glenn" || msgs[0].IsPrivate {
		t.Errorf("row nick/private = %q/%v, want the local nick and no private marker", msgs[0].Nick, msgs[0].IsPrivate)
	}
}
