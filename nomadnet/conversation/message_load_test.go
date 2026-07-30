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
	"bytes"
	"path/filepath"
	"testing"

	"os"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

func writeLXMFixture(t *testing.T, dir string) (path string, hash []byte) {
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
	msg, err := lxmf.NewMessage(dest, dest, "World body", "Hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	msg.State = lxmf.StateSent
	msg.TransportEncrypted = true
	msg.TransportEncryption = "AES-128"
	msg.Method = lxmf.MethodDirect

	written, err := msg.WriteToDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	return written, msg.Hash
}

func TestMessageLoadParsesEnvelope(t *testing.T) {
	t.Parallel()
	ts := rns.NewTransportSystem(nil)
	dir := tempDir(t)
	convDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, hash := writeLXMFixture(t, convDir)

	m := NewMessageWithTransport(path, ts)
	m.Load()

	if !m.Loaded {
		t.Fatal("message should be loaded")
	}
	if got := m.GetTitle(); got != "Hello" {
		t.Errorf("title = %q, want %q", got, "Hello")
	}
	if got := m.GetContent(); got != "World body" {
		t.Errorf("content = %q, want %q", got, "World body")
	}
	if got := m.GetHash(); !bytes.Equal(got, hash) {
		t.Errorf("hash = %x, want %x", got, hash)
	}
	if got := m.GetState(); got != StateSent {
		t.Errorf("state = %v, want StateSent", got)
	}
	if !m.GetTransportEncrypted() {
		t.Error("transport should be encrypted")
	}
	if got := m.GetTransportEncryption(); got != "AES-128" {
		t.Errorf("transport encryption = %q, want AES-128", got)
	}
	// Source identity is not registered in the transport recall store, so the
	// signature cannot be verified (matches Python's SOURCE_UNKNOWN branch).
	if m.SignatureValidated() {
		t.Error("signature should not be validated for unknown source")
	}
	if got := m.GetSignatureDescription(); got != "Unknown Origin" {
		t.Errorf("signature description = %q, want Unknown Origin", got)
	}
}

func TestMessageLoadStateMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		lxmfState int
		want      MessageState
	}{
		{lxmf.StateGenerating, StateGenerating},
		{lxmf.StateOutbound, StatePending},
		{lxmf.StateSending, StatePending},
		{lxmf.StateSent, StateSent},
		{lxmf.StateDelivered, StateDelivered},
		{lxmf.StateFailed, StateFailed},
		{lxmf.StateRejected, StateFailed},
		{lxmf.StateCancelled, StateFailed},
		{lxmfStatePaper, StatePaper},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			if got := mapLXMFState(tt.lxmfState); got != tt.want {
				t.Errorf("mapLXMFState(%#x) = %v, want %v", tt.lxmfState, got, tt.want)
			}
		})
	}
}
