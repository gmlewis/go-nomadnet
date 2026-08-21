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

// TestSavePythonByteParity pins the hard parity requirement for the msgpack
// migration: Go's Save MUST emit byte-for-byte identical msgpack to Python
// NomadNet's save_peer_settings (NomadNetworkApp.py:647), which serialises
// self.peer_settings (an insertion-ordered dict) via RNS.vendor.umsgpack.
//
// This is a LIVE cross-implementation test: it execs the real Python nomadnet
// reference, builds the SAME peer_settings dict (same keys, same insertion
// order, same values) the Go test constructs, msgpack.packb's it FRESH, and
// diffs the bytes against Go's Save output. It is skipped (not failed) when
// the Python nomadnet reference is not importable.
//
// The existing TestSavePythonCompatRoundTrip only round-trips Go Save -> Go
// Load and checks field equality — it never compares the bytes to Python, so
// it could not catch that the old vmihailenco codec chose different integer
// widths (signed int16/int32 vs Python's unsigned uint16/uint32) and would
// have diverged on announce_interval and last_lxmf_sync. This live byte diff
// catches that class of regression on every run.
func TestSavePythonByteParity(t *testing.T) {
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
		{
			name: "defaults",
			s:    DefaultSettings(720),
		},
	}

	// Build the Python input: one entry per case carrying the field values
	// Go uses. Bytes fields are forwarded as hex; nil stays nil (JSON null).
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

	// peerParityScript imports the real nomadnet reference's msgpack
	// (RNS.vendor.umsgpack, the exact packer save_peer_settings uses) and
	// rebuilds the peer_settings dict in Python's declaration order
	// (NomadNetworkApp.py:287-298), packb's it, and emits base64 per case.
	const peerParityScript = `
import sys, json, base64
import RNS.vendor.umsgpack as msgpack
req = json.loads(sys.stdin.read() or "{}")
out = {}
for c in req["cases"]:
    def hb(x):
        return bytes.fromhex(x) if x is not None else None
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
    out[c["name"]] = base64.b64encode(msgpack.packb(d)).decode()
print(json.dumps(out))
`
	var pyOut map[string]string
	testutils.RunPythonNomadnet(t, map[string]any{"cases": pyCases}, peerParityScript, &pyOut)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			want, err := base64.StdEncoding.DecodeString(pyOut[c.name])
			if err != nil {
				t.Fatalf("decode python bytes: %v", err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "peersettings")
			if err := Save(c.s, path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("Save(%v) bytes diverge from fresh Python:\n got %x\n want %x", c.name, got, want)
			}
		})
	}
}

// hexOrNil returns the hex encoding of a []byte value, or nil for a nil value,
// so bytes fields can round-trip through JSON to the Python parity script.
func hexOrNil(v any) any {
	if v == nil {
		return nil
	}
	b, ok := v.([]byte)
	if !ok {
		return nil
	}
	return hex.EncodeToString(b)
}
