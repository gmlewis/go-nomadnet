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

//go:build wago && (linux || darwin || windows) && (amd64 || arm64)

// This file verifies that a node passes a request's form-field data to a .wasm
// executable page as request_data, mirroring Python's executable-page branch,
// which copies every str key starting with field_ or var_ from the request's
// dict into the page's environment map (Node.py:168-172).

package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/wasmpages"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

// decodeRenderedPageRequest renders a .wasm page handler response and decodes
// the request payload the fixture echoes back as JSON.
func decodeRenderedPageRequest(t *testing.T, out any) wasmpages.PageRequest {
	t.Helper()
	markup, ok := out.([]byte)
	if !ok {
		t.Fatalf("wasm page handler returned %T, want []byte", out)
	}
	suffix := strings.TrimPrefix(string(markup), ">WASM PAGE\n----\n")
	var req wasmpages.PageRequest
	if err := json.Unmarshal([]byte(suffix), &req); err != nil {
		t.Fatalf("markup suffix does not decode as the request JSON: %v (%q)", err, markup)
	}
	return req
}

// TestNodeWasmPageRequestData pins that a form submission carrying
// field_*/var_* entries reaches the .wasm page's request_data map, and that
// unrelated entries are dropped exactly as Python drops them.
func TestNodeWasmPageRequestData(t *testing.T) {
	t.Parallel()

	dir := testutils.TempDir(t, "nomadnet-node-wasm-fields")
	wasmPath := filepath.Join(dir, "dynamic.wasm")
	if err := os.WriteFile(wasmPath, nodePageWasm, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	n := NewNode("test-node", dir, dir, 10, 10, 10, false)
	handler := n.makePageHandler(wasmPath)

	packed, err := rns.Pack(map[string]any{"field_title": "hi", "var_n": "5", "ignored": 123})
	mustTestErr(t, err)
	out := handler("/page/dynamic.wasm", packed, []byte("req"), []byte("link"), nil, time.Unix(1730000000, 0))
	req := decodeRenderedPageRequest(t, out)

	want := map[string]string{"field_title": "hi", "var_n": "5"}
	if len(req.RequestData) != len(want) {
		t.Fatalf("request_data = %v, want %v", req.RequestData, want)
	}
	for key, value := range want {
		if req.RequestData[key] != value {
			t.Errorf("request_data[%v] = %v, want %v", key, req.RequestData[key], value)
		}
	}
	if req.LinkID != "6c696e6b" {
		t.Errorf("link_id = %q, want the hex-encoded link ID", req.LinkID)
	}
}

// TestNodeWasmPageRequestDataAbsent pins that a request with no data leaves
// request_data unset, so the payload shape of an ordinary page fetch is
// unchanged (omitempty).
func TestNodeWasmPageRequestDataAbsent(t *testing.T) {
	t.Parallel()

	dir := testutils.TempDir(t, "nomadnet-node-wasm-nofields")
	wasmPath := filepath.Join(dir, "dynamic.wasm")
	if err := os.WriteFile(wasmPath, nodePageWasm, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	n := NewNode("test-node", dir, dir, 10, 10, 10, false)
	handler := n.makePageHandler(wasmPath)

	for _, tc := range []struct {
		name string
		data any
	}{
		{name: "nil data", data: nil},
		{name: "empty packed map", data: []byte{0x80}},
		{name: "no matching keys", data: mustPack(t, map[string]any{"other": "x"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := handler("/page/dynamic.wasm", tc.data, []byte("req"), nil, nil, time.Unix(1730000000, 0))
			markup, ok := out.([]byte)
			if !ok {
				t.Fatalf("wasm page handler returned %T, want []byte", out)
			}
			if strings.Contains(string(markup), "request_data") {
				t.Errorf("%v: payload carries request_data: %q", tc.name, markup)
			}
			req := decodeRenderedPageRequest(t, out)
			if req.RequestData != nil {
				t.Errorf("%v: request_data = %v, want nil", tc.name, req.RequestData)
			}
		})
	}
}

// mustPack msgpack-encodes a request-data value for a handler invocation.
func mustPack(t *testing.T, v any) []byte {
	t.Helper()
	packed, err := rns.Pack(v)
	mustTestErr(t, err)
	return packed
}

// mustTestErr fails the test when err is non-nil.
func mustTestErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
