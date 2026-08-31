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
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// This file pins the sent/received header classification end to end,
// reproducing the fleet data the direction bug was reported against
// (glenn-macm2pro ⇄ glenn-mac-mini-m2): real LXMF envelopes packed with real
// identities, persisted on disk, scanned by Conversation.ScanStorage/Load,
// and mapped through headerInputs → LXMessageHeader exactly as production
// does. Python renders each class as:
//
//	outbound + DELIVERED (raw 0x08)       → msg_header_delivered, "✓ →" (blue)
//	outbound + SENDING (raw 0x02) with a
//	  drained router queue (pending-check → FAILED) → msg_header_failed, "✕ →"
//	inbound from a known peer, valid sig  → msg_header_ok,         "✓ ←" (green)
//
// The reported bug — an own-sent message rendering with the green inbound
// header and "←" — occurs exactly when the classifier loses the source hash
// or the raw state, so every step below pins the fields feeding the branch.

// TestSentMessageHeaderParity runs the three message classes from the
// reported conversation through the full pipeline and pins the header each
// must render.
func TestSentMessageHeaderParity(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	ownID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	ownDest, err := rns.NewDestination(ts, ownID, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	peerDest, err := rns.NewDestination(ts, peerID, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}

	dir := sentParityTempDir(t)
	writeMsg := func(dest, src *rns.Destination, content string, state, method int) {
		t.Helper()
		m, err := lxmf.NewMessage(dest, src, content, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Pack(); err != nil {
			t.Fatal(err)
		}
		m.SetState(state)
		m.SetMethod(method)
		if _, err := m.WriteToDirectory(dir); err != nil {
			t.Fatal(err)
		}
	}

	// The three message classes exactly as they existed in the reported
	// conversation directory on glenn-mac-mini-m2.
	writeMsg(peerDest, ownDest, "msg2: hello from Mac Mini (nomadnet)", lxmf.StateDelivered, lxmf.MethodDirect)
	writeMsg(peerDest, ownDest, "msg10: from Mac Mini", lxmf.StateSending, lxmf.MethodDirect)
	writeMsg(ownDest, peerDest, "Message from glenn-macm2pro to glenn-mac-mini-m2", lxmf.StateDelivered, lxmf.MethodDirect)

	conv := conversation.NewConversation(hex.EncodeToString(peerDest.Hash), dir)
	conv.SetTransport(ts)
	// Nothing is pending in the router queue at rest, so the Python
	// pending-check (Conversation.py:451-460) marks the SENDING message
	// FAILED. Mirror that here.
	conv.SetPendingChecker(func(hash []byte) bool { return false })
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	msgs := conv.DisplayMessages()
	if len(msgs) != 3 {
		t.Fatalf("message count = %v, want 3", len(msgs))
	}

	cases := map[string]struct {
		wantSource *rns.Destination
		wantRaw    int
		wantStyle  string
		wantPrefix string
	}{
		"msg2: hello from Mac Mini (nomadnet)": {
			wantSource: ownDest, wantRaw: lxmf.StateDelivered,
			wantStyle: "msg_header_delivered", wantPrefix: "✓ →",
		},
		"msg10: from Mac Mini": {
			wantSource: ownDest, wantRaw: lxmf.StateFailed,
			wantStyle: "msg_header_failed", wantPrefix: "✕ →",
		},
		"Message from glenn-macm2pro to glenn-mac-mini-m2": {
			wantSource: peerDest, wantRaw: lxmf.StateDelivered,
			wantStyle: "msg_header_ok", wantPrefix: "✓ ←",
		},
	}

	for _, d := range msgs {
		tc, ok := cases[d.Content]
		if !ok {
			t.Fatalf("unexpected message content %q in DisplayMessages", d.Content)
		}
		if !bytes.Equal(d.SourceHash, tc.wantSource.Hash) {
			t.Errorf("content %q: source hash = %x, want %x",
				d.Content, d.SourceHash, tc.wantSource.Hash)
		}
		if d.State != tc.wantRaw {
			t.Errorf("content %q: raw state = %#x, want %#x", d.Content, d.State, tc.wantRaw)
		}
		title, style := renderMessageHeader(d, ownDest.Hash)
		if style != tc.wantStyle {
			t.Errorf("content %q: header style = %q, want %q", d.Content, style, tc.wantStyle)
		}
		if !strings.HasPrefix(title, tc.wantPrefix+" ") {
			t.Errorf("content %q: header title = %q, want prefix %q", d.Content, title, tc.wantPrefix)
		}
	}
}

// TestUnparsableSentMessageHeader pins the reported fleet bug exactly: an
// own-sent message whose on-disk envelope cannot be parsed reaches the header
// with no wire fields (State 0, nil source, method 0, unencrypted). Python's
// ConversationMessage keeps msg_source_hash = None after the failed load, so
// LXMessageWidget's first branch renders "msg_header_failed" with the
// warning glyph and no arrow (Conversations.py:2606-2608). The previous Go
// legacy fallback fabricated an inbound source with a "validated" signature,
// rendering the green msg_header_ok header with "✓ ←" for own-sent messages —
// the reported divergence from the Python source of truth.
func TestUnparsableSentMessageHeader(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	ownID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	ownDest, err := rns.NewDestination(ts, ownID, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	peerDest, err := rns.NewDestination(ts, peerID, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}

	dir := sentParityTempDir(t)

	// An own-sent message whose container (state/transport metadata from the
	// send path) exists but cannot be parsed by the renderer.
	unparsable, err := lxmf.NewMessage(peerDest, ownDest, "sent but on-disk envelope later corrupt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := unparsable.Pack(); err != nil {
		t.Fatal(err)
	}
	unparsable.SetState(lxmf.StateDelivered)
	unparsable.SetMethod(lxmf.MethodDirect)
	written, err := unparsable.WriteToDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt the container bytes after writing (same shape message a
	// foreign tool or crash leaves behind).
	if err := os.WriteFile(written, []byte("this is not a msgpack lxmf container"), 0o600); err != nil {
		t.Fatal(err)
	}

	conv := conversation.NewConversation(hex.EncodeToString(peerDest.Hash), dir)
	conv.SetTransport(ts)
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	got := conv.DisplayMessages()
	if len(got) != 1 {
		t.Fatalf("message count = %v, want 1 (the unparsable envelope)", len(got))
	}
	d := got[0]
	if d.SourceHash != nil {
		t.Errorf("unparsable message must keep a nil source hash, got %x", d.SourceHash)
	}
	title, style := renderMessageHeader(d, ownDest.Hash)
	if style != "msg_header_failed" {
		t.Errorf("unparsable own-sent message: header style = %q, want msg_header_failed (Python's no-source branch); title %q", style, title)
	}
	if strings.Contains(title, "←") {
		t.Errorf("unparsable message title %q must not carry the inbound ← arrow (the reported green-← bug)", title)
	}
}

// renderMessageHeader maps a MessageDisplayData through the production
// composition: the wiring-layer passthrough (toConversationMessages) onto a
// ConversationMessage, then ConversationWidget.headerInputs (including the
// no-wire-fields legacy fallback and the live OnOwnHash resolver), then
// LXMessageHeader.
func renderMessageHeader(d conversation.MessageDisplayData, ownHash []byte) (string, string) {
	msg := ConversationMessage{
		Content:              d.Content,
		Timestamp:            unixSecondsToTime(d.Timestamp),
		State:                d.State,
		Method:               d.Method,
		SourceHash:           d.SourceHash,
		Hash:                 d.Hash,
		TransportEncrypted:   d.TransportEncrypted,
		SignatureValidated:   d.SignatureValidated,
		SignatureDescription: sigDescription(d.SignatureValidated),
	}
	cw := &ConversationWidget{OwnHash: ownHash}
	in := cw.headerInputs(msg)
	return LXMessageHeader(in)
}

func sigDescription(validated bool) string {
	if validated {
		return "Signature Verified"
	}
	return "Unknown signature validation failure"
}

// TestSentMessageDeliveredIndexRestoreParity pins the .index restore path:
// an own-sent DELIVERED message restored from a Go/Python-written .index
// (source_hash as a 16-byte BIN, state as the raw LXMF constant) must keep
// the source hash and delivered state — if the restore ever drops them, the
// header falls into the inbound branch and the sentinel renders green "←".
func TestSentMessageDeliveredIndexRestoreParity(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	ownID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	ownDest, err := rns.NewDestination(ts, ownID, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	peerDest, err := rns.NewDestination(ts, peerID, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}

	dir := sentParityTempDir(t)
	m, err := lxmf.NewMessage(peerDest, ownDest, "sent delivered via index restore", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Pack(); err != nil {
		t.Fatal(err)
	}
	m.SetState(lxmf.StateDelivered)
	m.SetMethod(lxmf.MethodDirect)
	if _, err := m.WriteToDirectory(dir); err != nil {
		t.Fatal(err)
	}

	conv := conversation.NewConversation(hex.EncodeToString(peerDest.Hash), dir)
	conv.SetTransport(ts)
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	if err := conversation.WriteIndex(dir, conv.Messages); err != nil {
		t.Fatalf("write index: %v", err)
	}

	// A fresh scan must restore from the index (mtime unchanged) and keep
	// the sent classification: source = own, state = DELIVERED.
	restored := conversation.NewConversation(hex.EncodeToString(peerDest.Hash), dir)
	restored.SetTransport(ts)
	if err := restored.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	got := restored.DisplayMessages()
	if len(got) != 1 {
		t.Fatalf("restored message count = %v, want 1", len(got))
	}
	d := got[0]
	if !bytes.Equal(d.SourceHash, ownDest.Hash) {
		t.Fatalf("restored source hash = %x, want own %x", d.SourceHash, ownDest.Hash)
	}
	if d.State != lxmf.StateDelivered {
		t.Fatalf("restored raw state = %#x, want %#x", d.State, lxmf.StateDelivered)
	}
	title, style := renderMessageHeader(d, ownDest.Hash)
	if style != "msg_header_delivered" {
		t.Errorf("restored header style = %q, want msg_header_delivered (title %q)", style, title)
	}
	if !strings.HasPrefix(title, "✓ →") {
		t.Errorf("restored title %q does not carry the outbound ✓ → prefix", title)
	}
}

// sentParityTempDir returns a short-lived /tmp directory; a short path keeps
// Unix domain sockets working on macOS in tests (see AGENTS.md).
func sentParityTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nomadnet-sent-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// unixSecondsToTime converts a Unix-seconds float message timestamp into a
// time.Time, exactly like the production wiring (toConversationMessages).
func unixSecondsToTime(f float64) time.Time {
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// TestLateOwnHashSelfCorrection pins the fleet symptom from the 0.54.0 run on
// glenn-mac-mini-m2: the conversation widget was created while the LXMF
// router was still registering, so its cached OwnHash snapshot was nil and
// EVERY message — own-sent ones included — rendered with the inbound green
// "✓ ←" msg_header_ok header. Python reads app.lxmf_destination.hash fresh at
// every LXMessageWidget construction; the widget must do the same via the
// OnOwnHash resolver, so the render self-corrects as soon as the app's LXMF
// destination is registered.
func TestLateOwnHashSelfCorrection(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerDest, err := rns.NewDestination(ts, peerID, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}

	// The production wiring: OnOwnHash reads app.LXMFDest, which is nil until
	// the router registers the delivery identity.
	var lxmfd *rns.Destination
	widget := &ConversationWidget{
		OnOwnHash: func() []byte {
			if lxmfd == nil {
				return nil
			}
			return lxmfd.Hash
		},
	}

	d := conversation.MessageDisplayData{
		Content:            "Message from glenn-mac-mini-m2 to glenn-macm2pro",
		Timestamp:          1788174856.0,
		State:              lxmf.StateDelivered,
		Method:             lxmf.MethodDirect,
		SourceHash:         dest.Hash, // own-sent message
		TransportEncrypted: true,
		SignatureValidated: true,
	}
	msg := ConversationMessage{
		Content:              d.Content,
		Timestamp:            unixSecondsToTime(d.Timestamp),
		State:                d.State,
		Method:               d.Method,
		SourceHash:           d.SourceHash,
		TransportEncrypted:   d.TransportEncrypted,
		SignatureValidated:   d.SignatureValidated,
		SignatureDescription: sigDescription(d.SignatureValidated),
	}

	// Pre-registration render: LXMFDest unset — the widget cannot classify
	// outbound from inbound yet.
	_, styleBefore := LXMessageHeader(widget.headerInputs(msg))
	if styleBefore != "msg_header_ok" {
		t.Errorf("pre-registration style = %q, want msg_header_ok (the transient inbound render)", styleBefore)
	}

	// The router registers the delivery identity.
	lxmfd = dest

	// The next render must self-correct via the live resolver: the same
	// own-sent message now classifies outbound-delivered, matching nomadnet's
	// blue "✓ →" header.
	in := widget.headerInputs(msg)
	if !bytes.Equal(in.OwnHash, dest.Hash) {
		t.Errorf("post-registration OwnHash = %x, want %x", in.OwnHash, dest.Hash)
	}
	title, style := LXMessageHeader(in)
	if style != "msg_header_delivered" {
		t.Errorf("post-registration style = %q, want msg_header_delivered (title %q)", style, title)
	}
	if !strings.HasPrefix(title, "✓ →") {
		t.Errorf("title %q does not carry the outbound ✓ → prefix", title)
	}
	_ = peerDest
}
