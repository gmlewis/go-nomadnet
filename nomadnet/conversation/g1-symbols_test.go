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
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// TestConversationSetChangedCallback pins SetChangedCallback: the callback
// fires when ScanStorage replaces the message list, and a nil registration is
// harmless (Python Conversation.changed_callback).
func TestConversationSetChangedCallback(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	dir := tempDir(t)
	convDir := filepath.Join(dir, "conv")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLXMFixture(t, convDir)

	conv := NewConversation("conv", convDir)
	conv.SetTransport(ts)
	calls := 0
	conv.SetChangedCallback(func() { calls++ })

	if err := conv.ScanStorage(); err != nil {
		t.Fatalf("ScanStorage: %v", err)
	}
	if calls == 0 {
		t.Error("changed callback did not fire on ScanStorage")
	}

	// A conversation without a callback scans just as well.
	plain := NewConversation("conv", convDir)
	plain.SetTransport(ts)
	if err := plain.ScanStorage(); err != nil {
		t.Fatalf("ScanStorage without callback: %v", err)
	}
}

// fieldBytes looks a field up by an integer code, accepting the numeric kinds a
// msgpack-decoded map key can have (int for a directly-built message, int64
// after a load).
func fieldBytes(fields map[any]any, key int) any {
	if v, ok := fields[key]; ok {
		return v
	}
	if v, ok := fields[int64(key)]; ok {
		return v
	}
	if v, ok := fields[uint64(key)]; ok {
		return v
	}
	return nil
}

// TestMessageGetFields pins Message.GetFields: it returns the LXMF envelope's
// structured fields after Load, without requiring the caller to Load first.
func TestMessageGetFields(t *testing.T) {
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
	fields := map[any]any{
		lxmf.FieldThread:   []byte("thread-1"),
		lxmf.FieldCommands: []byte("ping"),
	}
	msg, err := lxmf.NewMessage(dest, dest, "body", "title", fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	dir := tempDir(t)
	if _, err := msg.WriteToDirectory(dir); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) == 0 {
		t.Fatalf("no message file written: %v", err)
	}
	path := filepath.Join(dir, files[0].Name())

	m := NewMessageWithTransport(path, ts)
	got := m.GetFields()
	if got == nil {
		t.Fatal("GetFields = nil, want the envelope fields")
	}
	if thread, _ := fieldBytes(got, lxmf.FieldThread).([]byte); !bytes.Equal(thread, []byte("thread-1")) {
		t.Errorf("FieldThread = %q, want thread-1", thread)
	}
	if commands, _ := fieldBytes(got, lxmf.FieldCommands).([]byte); !bytes.Equal(commands, []byte("ping")) {
		t.Errorf("FieldCommands = %q, want ping", commands)
	}
	// A second call returns the cached map without reloading.
	if again := m.GetFields(); len(again) != len(got) {
		t.Errorf("cached GetFields length = %v, want %v", len(again), len(got))
	}
}
