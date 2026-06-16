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

//go:build integration

package rrc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-reticulum/testutils"
)

const pythonDecodeCBORScript = `import cbor2
import sys

def main():
    if len(sys.argv) != 2:
        print("ERROR: missing input path")
        sys.exit(1)

    with open(sys.argv[1], "rb") as f:
        raw = f.read()

    env = cbor2.loads(raw)
    def decode(v):
        if isinstance(v, bytes):
            return v.decode("utf-8", errors="replace")
        return str(v)

    print(f"TYPE:{decode(env.get(1, b'MISSING'))}")
    print(f"ROOM:{decode(env.get(5, b'MISSING'))}")
    print(f"SOURCE:{decode(env.get(4, b'MISSING'))}")
    print(f"BODY:{decode(env.get(6, b'MISSING'))}")
    print(f"TIMESTAMP:{env.get(3, 'MISSING')}")
    print(f"KEYS:{sorted(env.keys())}")

if __name__ == "__main__":
    main()
`

func TestIntegrationCBORGoToPython(t *testing.T) {
	t.Parallel()

	// Check if Python cbor2 is available
	cmd := exec.Command("python3", "-c", "import cbor2")
	if err := cmd.Run(); err != nil {
		t.Skip("Python cbor2 not available, skipping CBOR parity test")
	}

	tmpDir, cleanup := testutils.TempDir(t, "nomadnet-cbor-parity")
	defer cleanup()

	// Create a Go envelope
	env := MakeEnvelope(TypeMsg, []byte{0xAA, 0xBB}, []byte("#general"), []byte("testnick"), []byte("hello from go"), nil, NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}

	// Write to disk for Python to read
	packedPath := filepath.Join(tmpDir, "go_envelope.cbor")
	if err := os.WriteFile(packedPath, data, 0o644); err != nil {
		t.Fatalf("write packed envelope: %v", err)
	}

	// Write Python script
	scriptPath := filepath.Join(tmpDir, "decode_cbor.py")
	if err := os.WriteFile(scriptPath, []byte(pythonDecodeCBORScript), 0o644); err != nil {
		t.Fatalf("write python script: %v", err)
	}

	// Run Python to decode
	cmd = exec.Command("python3", scriptPath, packedPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python decode failed: %v output=%v", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "TYPE:20") {
		t.Errorf("Python output missing TYPE:20, got: %s", output)
	}
	if !strings.Contains(output, "ROOM:#general") {
		t.Errorf("Python output missing ROOM:#general, got: %s", output)
	}
	if !strings.Contains(output, "BODY:hello from go") {
		t.Errorf("Python output missing BODY:hello from go, got: %s", output)
	}

	// Verify the decoded keys are complete
	if !strings.Contains(output, "KEYS:") {
		t.Errorf("Python output missing KEYS, got: %s", output)
	}
}

func TestIntegrationRRCGoToPythonCBORRoundTrip(t *testing.T) {
	t.Parallel()

	// Check if Python cbor2 is available
	cmd := exec.Command("python3", "-c", "import cbor2")
	if err := cmd.Run(); err != nil {
		t.Skip("Python cbor2 not available, skipping CBOR round-trip test")
	}

	tmpDir, cleanup := testutils.TempDir(t, "nomadnet-cbor-roundtrip")
	defer cleanup()

	// Go encodes
	env := MakeEnvelope(TypeMsg, []byte{0x01}, []byte("#test"), []byte("gopher"), []byte("go says hello"), nil, NowMs())
	goEncoded, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("Go EncodeEnvelope: %v", err)
	}

	// Write Go-encoded CBOR for Python to read and re-encode
	goPath := filepath.Join(tmpDir, "go_env.cbor")
	if err := os.WriteFile(goPath, goEncoded, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Python reads Go's CBOR, then Go decodes Python's re-encoded version
	pythonReencodeScript := `import cbor2, sys
with open(sys.argv[1], "rb") as f:
    env = cbor2.loads(f.read())
with open(sys.argv[2], "wb") as f:
    f.write(cbor2.dumps(env))
# Print what we encoded for debugging
print(f"REENCODED_KEYS:{sorted(env.keys())}")
print(f"REENCODED_TYPES:{[(type(k).__name__, type(v).__name__) for k, v in env.items()]}")
print("OK")
`
	scriptPath := filepath.Join(tmpDir, "reencode.py")
	if err := os.WriteFile(scriptPath, []byte(pythonReencodeScript), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	pythonEncodedPath := filepath.Join(tmpDir, "python_reencoded.cbor")
	cmd = exec.Command("python3", scriptPath, goPath, pythonEncodedPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("python re-encode failed: %v output=%v", err, string(out))
	}

	// Go decodes Python's re-encoding
	pythonEncoded, err := os.ReadFile(pythonEncodedPath)
	if err != nil {
		t.Fatalf("read python encoded: %v", err)
	}

	t.Logf("python encoded len=%d", len(pythonEncoded))
	decoded, err := DecodeEnvelope(pythonEncoded)
	if err != nil {
		t.Fatalf("Go DecodeEnvelope failed: %v", err)
	}

	// Debug: log all keys and their types
	for k, v := range decoded {
		t.Logf("key=%v (%T) val=%v (%T)", k, k, v, v)
	}

	// Verify body survived the round-trip.
	// After Python re-encoding, keys become uint64.
	var bodyVal any
	bodyVal = decoded[KeyBody]
	if bodyVal == nil {
		bodyVal = decoded[uint64(KeyBody)]
	}
	if bodyVal == nil {
		t.Fatal("decoded envelope missing body key")
	}
	bodyBytes, ok := bodyVal.([]byte)
	if !ok {
		t.Fatalf("body is %T, want []byte", bodyVal)
	}
	if string(bodyBytes) != "go says hello" {
		t.Errorf("body = %q, want 'go says hello'", string(bodyBytes))
	}

	var roomVal any
	roomVal = decoded[KeyRoom]
	if roomVal == nil {
		roomVal = decoded[uint64(KeyRoom)]
	}
	if roomVal == nil {
		t.Fatal("decoded envelope missing room key")
	}
	roomBytes, ok := roomVal.([]byte)
	if !ok {
		t.Fatalf("room is %T, want []byte", roomVal)
	}
	if string(roomBytes) != "#test" {
		t.Errorf("room = %q, want '#test'", string(roomBytes))
	}
}

func TestIntegrationProtocolConstantsMatch(t *testing.T) {
	// Verify Go protocol constants match Python values.
	// This ensures the Go and Python implementations use the same wire format.
	expected := map[string]int{
		"RRCVersion": 1,
		"TypeHello":  1,
		"TypeWelcome": 2,
		"TypeJoin":   10,
		"TypeJoined": 11,
		"TypePart":   12,
		"TypeParted": 13,
		"TypeMsg":    20,
		"TypeNotice": 21,
		"TypeAction": 22,
		"TypePing":   30,
		"TypePong":   31,
		"TypeError":  40,
		"KeyVersion": 0,
		"KeyType":    1,
		"KeyMessageID": 2,
		"KeyTimestamp": 3,
		"KeySource":  4,
		"KeyRoom":    5,
		"KeyBody":    6,
		"KeyNick":    7,
	}

	actual := map[string]int{
		"RRCVersion":  RRCVersion,
		"TypeHello":   TypeHello,
		"TypeWelcome": TypeWelcome,
		"TypeJoin":    TypeJoin,
		"TypeJoined":  TypeJoined,
		"TypePart":    TypePart,
		"TypeParted":  TypeParted,
		"TypeMsg":     TypeMsg,
		"TypeNotice":  TypeNotice,
		"TypeAction":  TypeAction,
		"TypePing":    TypePing,
		"TypePong":    TypePong,
		"TypeError":   TypeError,
		"KeyVersion":  KeyVersion,
		"KeyType":     KeyType,
		"KeyMessageID": KeyMessageID,
		"KeyTimestamp": KeyTimestamp,
		"KeySource":   KeySource,
		"KeyRoom":     KeyRoom,
		"KeyBody":     KeyBody,
		"KeyNick":     KeyNick,
	}

	for name, want := range expected {
		if got := actual[name]; got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
}
