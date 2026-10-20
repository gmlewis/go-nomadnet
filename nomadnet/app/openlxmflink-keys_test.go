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

package app

import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
)

// TestOpenLXMFLinkAsksTheNetworkForUnknownPeerKeys pins that opening an LXMF
// link for an unknown peer asks the network for that peer's keys. Every Python
// site that creates a conversation builds a Conversation object
// (Conversations.py:1058 for the New Conversation dialog — typed address or
// scanned QR — and the Network/Browser/Channels "Converse" paths), and
// Conversation.__init__ immediately recalls the peer's identity and, when it
// cannot be recalled, asks the network for it (Conversation.py:204-208):
//
//	if not RNS.Identity.recall(bytes.fromhex(self.source_hash)):
//	    RNS.Transport.request_path(bytes.fromhex(source_hash))
//
// That request is what turns an "addressed, but identity unknown" peer into a
// peer that can actually be messaged: LXMF encrypts to the recipient's identity,
// so without it nothing can be sent — not directly, and not through a
// propagation node either.
func TestOpenLXMFLinkAsksTheNetworkForUnknownPeerKeys(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)

	ts := rns.NewTransportSystem(nil)
	if err := ts.Start(filepath.Join(dir, "rns-storage")); err != nil {
		t.Fatalf("TransportSystem.Start: %v", err)
	}
	t.Cleanup(ts.Stop)

	// Capture everything the transport puts on the wire. A path request for a
	// PLAIN destination is not encrypted, so the requested destination hash
	// travels verbatim: the peer's 16 bytes appearing in any frame is proof that
	// this node asked the network about that peer.
	peerHash := "abcdef0123456789abcdef0123456789"
	peerBytes, err := hex.DecodeString(peerHash)
	if err != nil {
		t.Fatal(err)
	}

	frames := make(chan []byte, 1024)
	// A PipeInterface delivers what it is sent to its PEER's OnReceive
	// (pipe.go processQueue), so the recorder sits on the far end of the wire
	// and the near end is what gets registered on the transport.
	sink := interfaces.NewPipeInterface("sink", func(data []byte, _ interfaces.Interface) {
		select {
		case frames <- append([]byte{}, data...):
		default:
		}
	})
	pipe := interfaces.NewPipeInterface("capture", nil)
	pipe.SetOther(sink)
	ts.RegisterInterface(pipe)
	t.Cleanup(func() { _ = pipe.Detach() })

	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport: %v", err)
	}
	if a.Transport.Recall(peerBytes) != nil {
		t.Fatal("precondition: the transport already knows this peer's identity")
	}

	isNew, err := a.OpenLXMFLink(peerHash)
	if err != nil {
		t.Fatalf("OpenLXMFLink(%q): %v", peerHash, err)
	}
	if !isNew {
		t.Errorf("OpenLXMFLink(%q) reported an existing conversation for a brand-new peer", peerHash)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case frame := <-frames:
			if bytes.Contains(frame, peerBytes) {
				return // the node asked the network for this peer's keys
			}
		case <-deadline:
			t.Fatal("OpenLXMFLink created the conversation without asking the network for the peer's identity; " +
				"Python's Conversation.__init__ requests it (Conversation.py:204-208), and without the identity " +
				"no LXMF message to that peer can be built at all")
		}
	}
}
