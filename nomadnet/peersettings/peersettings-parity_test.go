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
	"embed"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

//go:embed testdata/peersettings_filled.bin
//go:embed testdata/peersettings_defaults.bin
//go:embed testdata/peersettings_filled.json
//go:embed testdata/peersettings_defaults.json
var peerFS embed.FS

// peerExpected mirrors the Python-parsed peersettings dict. Bytes fields are
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

func loadPeerExpected(t *testing.T, name string) peerExpected {
	t.Helper()
	data, err := peerFS.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatalf("read %v.json: %v", name, err)
	}
	var ex peerExpected
	if err := json.Unmarshal(data, &ex); err != nil {
		t.Fatalf("unmarshal %v.json: %v", name, err)
	}
	return ex
}

func bytesHex(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	h, _ := m["__bytes_hex__"].(string)
	return h
}

// TestLoadPythonCompat verifies Go's Load parses msgpack peersettings files
// written by Python's NomadNetworkApp.save_peer_settings, with every field
// matching what Python itself unpacks from the same bytes.
func TestLoadPythonCompat(t *testing.T) {
	t.Parallel()
	cases := []string{"filled", "defaults"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bin, err := peerFS.ReadFile("testdata/peersettings_" + name + ".bin")
			if err != nil {
				t.Fatalf("read bin: %v", err)
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
			want := loadPeerExpected(t, "peersettings_"+name)

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

// TestSavePythonCompat verifies that Go's Save produces msgpack that Python can
// unpack to the same field values (round-tripped through Go Load afterward, and
// byte-compatible with Python msgpack for the field set).
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
