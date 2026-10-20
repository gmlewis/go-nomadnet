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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/msgpack"
)

// announceIntakeApp builds the minimal App state the announce handlers touch.
func announceIntakeApp(t *testing.T) *App {
	t.Helper()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Logger = rns.NewLogger()
	a.Dir = directory.New()
	return a
}

// TestPNAnnounceGate pins Python PNAnnounceHandler (Directory.py:20-30): only a
// well-formed LXMF propagation announce whose node-state element is True enters
// the PN stream (and re-runs auto-selection).
func TestPNAnnounceGate(t *testing.T) {
	t.Parallel()

	a := announceIntakeApp(t)
	dest := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	a.handlePNAnnounce(dest, nil, []byte("not msgpack"), false)
	if got := len(a.Dir.PNAnnounces()); got != 0 {
		t.Fatalf("malformed announce entered the PN stream (%v entries)", got)
	}

	withdrawn, err := msgpack.Pack([]any{0, 1700000000, false, 1000, 1000, []any{0, 0, 0}, map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	a.handlePNAnnounce(dest, nil, withdrawn, false)
	if got := len(a.Dir.PNAnnounces()); got != 0 {
		t.Fatalf("a withdrawn (node_state=false) announce entered the PN stream (%v entries)", got)
	}

	serving, err := msgpack.Pack([]any{0, 1700000000, true, 1000, 1000, []any{0, 0, 0}, map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	a.handlePNAnnounce(dest, nil, serving, false)
	if got := len(a.Dir.PNAnnounces()); got != 1 {
		t.Fatalf("a serving announce produced %v PN entries, want 1", got)
	}
}

// TestAnnounceStreamCompactionGatedOnConfig pins Python's compact_stream gate
// (Directory.py:158,181,222, default False): with compaction off, repeated
// announces from one source all enter the stream; with it on, only the newest
// survives.
func TestAnnounceStreamCompactionGatedOnConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		compact bool
		want    int
	}{
		{"compaction off keeps every announce", false, 2},
		{"compaction on keeps only the newest", true, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := announceIntakeApp(t)
			a.CompactStream = tc.compact
			dest := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
			a.handleLXMFAnnounce(dest, nil, []byte("Peer Name"), false)
			a.handleLXMFAnnounce(dest, nil, []byte("Peer Name"), false)
			if got := len(a.Dir.PeerAnnounces()); got != tc.want {
				t.Errorf("peer stream after two announces = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPropagationAnnounceDataValidationMatchesLXMF pins the helper the gate
// relies on: the LXMF reference accepts a seven-element list and rejects a
// short one and a non-boolean state.
func TestPropagationAnnounceDataValidationMatchesLXMF(t *testing.T) {
	t.Parallel()

	if _, ok := lxmf.PropagationNodeStateFromAppData([]byte("x")); ok {
		t.Error("PropagationNodeStateFromAppData accepted a non-msgpack payload")
	}
	short, _ := msgpack.Pack([]any{0, 1700000000, true})
	if _, ok := lxmf.PropagationNodeStateFromAppData(short); ok {
		t.Error("PropagationNodeStateFromAppData accepted a three-element payload")
	}
}
