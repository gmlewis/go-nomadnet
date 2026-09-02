// Throwaway: decode the Go hello bytes with fxamacker + compare with Python.
package rrc

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestDumpHelloBytes(t *testing.T) {
	src := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00}
	env := MakeEnvelope(TypeHello, src, nil, nil, map[any]any{
		BHelloName: []byte("nomadnet"),
		BHelloVer:  []byte("0.1"),
		BHelloCaps: map[any]any{
			CapResourceEnvelope: true,
			CapAction:           true,
		},
	}, nil, 1700000000000)
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty envelope")
	}
	t.Logf("first byte: %#02x (a5 = CBOR map(5))", data[0])
	// Round-trip decode with fxamacker to prove it is real CBOR.
	var back map[any]any
	if err := cbor.Unmarshal(data, &back); err != nil {
		t.Fatalf("fxamacker cannot decode our own envelope: %v", err)
	}
	t.Logf("decoded %d keys, types:", len(back))
	for k, v := range back {
		t.Logf("  key %#v -> %T %v", k, v, v)
	}
	_ = fmt.Sprint
	_ = hex.EncodeToString
}
