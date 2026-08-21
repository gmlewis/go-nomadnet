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

package peersettings

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// peerExpected mirrors the Python-unpacked peersettings dict. Bytes fields are
// encoded as {"__bytes_hex__": "..."}; nil values stay nil.
type peerExpected struct {
	DisplayName        any `json:"display_name"`
	AnnounceInterval   any `json:"announce_interval"`
	LastAnnounce       any `json:"last_announce"`
	NodeLastAnnounce   any `json:"node_last_announce"`
	PropagationNode    any `json:"propagation_node"`
	LastLXMFSync       any `json:"last_lxmf_sync"`
	NodeConnects       any `json:"node_connects"`
	ServedPageRequests any `json:"served_page_requests"`
	ServedFileRequests any `json:"served_file_requests"`
}

func bytesHex(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	h, _ := m["__bytes_hex__"].(string)
	return h
}

// peerLoadParityScript imports the real nomadnet reference's msgpack
// (RNS.vendor.umsgpack, the exact codec save_peer_settings uses) and, for each
// case, rebuilds the peer_settings dict in Python's declaration order
// (NomadNetworkApp.py:287-298), msgpack.packb's it FRESH to produce the on-disk
// bytes, then msgpack.unpackb's those FRESH bytes so the expected parsed fields
// are derived from the current Python source on every run (never a committed
// golden). Bytes fields are emitted as {"__bytes_hex__": "..."} so they survive
// JSON round-tripping.
const peerLoadParityScript = `
import sys, json, base64
import RNS.vendor.umsgpack as msgpack
req = json.loads(sys.stdin.read() or "{}")
def hb(x):
    return bytes.fromhex(x) if x is not None else None
def enc(v):
    if isinstance(v, (bytes, bytearray)):
        return {"__bytes_hex__": v.hex()}
    return v
out = {}
for c in req["cases"]:
    d = {
        "display_name": c["display_name"],
        "announce_interval": c["announce_interval"],
        "last_announce": c["last_announce"],
        "node_last_announce": hb(c.get("node_last_announce")),
        "propagation_node": hb(c.get("propagation_node")),
        "last_lxmf_sync": c["last_lxmf_sync"],
        "node_connects": c["node_connects"],
        "served_page_requests": c["served_page_requests"],
        "served_file_requests": c["served_file_requests"],
    }
    packed = msgpack.packb(d)
    unpacked = msgpack.unpackb(packed, raw=False)
    out[c["name"]] = {
        "b64": base64.b64encode(packed).decode(),
        "unpacked": {k: enc(v) for k, v in unpacked.items()},
    }
print(json.dumps(out))
`

// TestLoadPythonCompat verifies Go's Load parses msgpack peersettings files
// written by Python's NomadNetworkApp.save_peer_settings, with every field
// matching what Python itself unpacks from the SAME fresh bytes.
//
// This is a LIVE cross-implementation test: it execs the real Python nomadnet
// reference, builds the SAME peer_settings dict the Go test constructs,
// msgpack.packb's it FRESH to produce the on-disk bytes, and msgpack.unpackb's
// those bytes FRESH to derive the expected field values — so a divergence is a
// real Go/Python mismatch, never a stale committed golden. It is skipped (not
// failed) when the Python nomadnet reference is not importable.
func TestLoadPythonCompat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    *Settings
	}{
		{
			name: "filled",
			s: &Settings{
				DisplayName:        "Test Peer",
				AnnounceInterval:   720,
				LastAnnounce:       1700000123.456,
				NodeLastAnnounce:   mustHex("00112233445566778899aabbccddeeff"),
				PropagationNode:    mustHex("ffeeddccbbaa99887766554433221100"),
				LastLXMFSync:       1700000200,
				NodeConnects:       42,
				ServedPageRequests: 100,
				ServedFileRequests: 7,
			},
		},
		{name: "defaults", s: DefaultSettings(720)},
	}

	// Build the Python input: one entry per case carrying the field values Go
	// uses. Bytes fields are forwarded as hex; nil stays nil (JSON null).
	pyCases := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		entry := map[string]any{
			"name":                 c.name,
			"display_name":         c.s.DisplayName,
			"announce_interval":    c.s.AnnounceInterval,
			"last_announce":        c.s.LastAnnounce,
			"last_lxmf_sync":       c.s.LastLXMFSync,
			"node_connects":        c.s.NodeConnects,
			"served_page_requests": c.s.ServedPageRequests,
			"served_file_requests": c.s.ServedFileRequests,
		}
		entry["node_last_announce"] = hexOrNil(c.s.NodeLastAnnounce)
		entry["propagation_node"] = hexOrNil(c.s.PropagationNode)
		pyCases = append(pyCases, entry)
	}

	var pyOut map[string]struct {
		B64      string       `json:"b64"`
		Unpacked peerExpected `json:"unpacked"`
	}
	testutils.RunPythonNomadnet(t, map[string]any{"cases": pyCases}, peerLoadParityScript, &pyOut)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			bin, err := base64.StdEncoding.DecodeString(pyOut[c.name].B64)
			if err != nil {
				t.Fatalf("decode python bytes: %v", err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "peersettings")
			if err := os.WriteFile(path, bin, 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			s, err := Load(path, 720)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			want := pyOut[c.name].Unpacked

			if s.DisplayName != wantStr(want.DisplayName) {
				t.Errorf("display_name = %q, want %q", s.DisplayName, wantStr(want.DisplayName))
			}
			if s.AnnounceInterval != wantInt(want.AnnounceInterval) {
				t.Errorf("announce_interval = %v, want %v", s.AnnounceInterval, wantInt(want.AnnounceInterval))
			}
			if s.LastLXMFSync != wantInt(want.LastLXMFSync) {
				t.Errorf("last_lxmf_sync = %v, want %v", s.LastLXMFSync, wantInt(want.LastLXMFSync))
			}
			if s.NodeConnects != wantInt(want.NodeConnects) {
				t.Errorf("node_connects = %v, want %v", s.NodeConnects, wantInt(want.NodeConnects))
			}
			if s.ServedPageRequests != wantInt(want.ServedPageRequests) {
				t.Errorf("served_page_requests = %v, want %v", s.ServedPageRequests, wantInt(want.ServedPageRequests))
			}
			if s.ServedFileRequests != wantInt(want.ServedFileRequests) {
				t.Errorf("served_file_requests = %v, want %v", s.ServedFileRequests, wantInt(want.ServedFileRequests))
			}
			if !anyBytesEqual(s.LastAnnounce, want.LastAnnounce) {
				t.Errorf("last_announce = %v, want %v", s.LastAnnounce, want.LastAnnounce)
			}
			if !anyBytesEqual(s.NodeLastAnnounce, want.NodeLastAnnounce) {
				t.Errorf("node_last_announce = %v, want %v", s.NodeLastAnnounce, want.NodeLastAnnounce)
			}
			if !anyBytesEqual(s.PropagationNode, want.PropagationNode) {
				t.Errorf("propagation_node = %v, want %v", s.PropagationNode, want.PropagationNode)
			}
		})
	}
}

// TestSavePythonCompatRoundTrip verifies Go's Save -> Load round-trips every
// field (a Go-internal consistency check). Byte-for-byte parity with Python
// msgpack is covered separately and live by TestSavePythonByteParity.
func TestSavePythonCompatRoundTrip(t *testing.T) {
	t.Parallel()
	s := &Settings{
		DisplayName:        "Test Peer",
		AnnounceInterval:   720,
		LastAnnounce:       1700000123.456,
		NodeLastAnnounce:   mustHex("00112233445566778899aabbccddeeff"),
		PropagationNode:    mustHex("ffeeddccbbaa99887766554433221100"),
		LastLXMFSync:       1700000200,
		NodeConnects:       42,
		ServedPageRequests: 100,
		ServedFileRequests: 7,
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "peersettings")
	if err := Save(s, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path, 720)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.DisplayName != s.DisplayName ||
		loaded.AnnounceInterval != s.AnnounceInterval ||
		loaded.LastLXMFSync != s.LastLXMFSync ||
		loaded.NodeConnects != s.NodeConnects ||
		loaded.ServedPageRequests != s.ServedPageRequests ||
		loaded.ServedFileRequests != s.ServedFileRequests {
		t.Errorf("round-trip scalar mismatch: %+v", loaded)
	}
	if !anyBytesEqual(loaded.NodeLastAnnounce, s.NodeLastAnnounce) {
		t.Errorf("round-trip node_last_announce mismatch")
	}
	if !anyBytesEqual(loaded.PropagationNode, s.PropagationNode) {
		t.Errorf("round-trip propagation_node mismatch")
	}
}

func wantStr(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func wantInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func mustHex(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return b
}

func anyBytesEqual(got any, want any) bool {
	// Both nil.
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	// want may be {"__bytes_hex__": "..."} (from JSON); decode and compare.
	if wh := bytesHex(want); wh != "" {
		gb, ok := got.([]byte)
		if !ok {
			return false
		}
		wb, _ := hex.DecodeString(wh)
		return bytes.Equal(gb, wb)
	}
	// Both []byte (Go round-trip).
	if gb, ok := got.([]byte); ok {
		if wb, ok := want.([]byte); ok {
			return bytes.Equal(gb, wb)
		}
	}
	// Float comparison (last_announce may be a float).
	gf, gok := got.(float64)
	wf, wok := want.(float64)
	if gok && wok {
		return gf == wf
	}
	return got == want
}
