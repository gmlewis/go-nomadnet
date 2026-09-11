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

// This file verifies that the node serves .wasm executable pages through the
// sandboxed wasm renderer, mirroring Python's executable-page subprocess
// branch (Node.py:161-175).

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

// nodePageWasm mirrors the wasmpages package's dynamic page fixture: an
// alloc export plus a render_page that prepends a canned Micron prefix and
// appends the serialized request bytes.
var nodePageWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x0d, 0x02,
	0x60, 0x01, 0x7f, 0x01, 0x7f,
	0x60, 0x02, 0x7f, 0x7f, 0x02, 0x7f, 0x7f,
	0x03, 0x03, 0x02, 0x00, 0x01,
	0x05, 0x04, 0x01, 0x01, 0x01, 0x01,
	0x07, 0x2b, 0x03,
	0x06, 'm', 'e', 'm', 'o', 'r', 'y', 0x02, 0x00,
	0x10, 'w', 'a', 'g', 'o', 'p', 'l', 'u', 'g', 'i', 'n', '_', 'a', 'l', 'l', 'o', 'c', 0x00, 0x00,
	0x0b, 'r', 'e', 'n', 'd', 'e', 'r', '_', 'p', 'a', 'g', 'e', 0x00, 0x01,
	0x0a, 0x29, 0x02,
	0x05, 0x00, 0x41, 0x80, 0x20, 0x0b,
	0x21, 0x00, 0x41, 0x80, 0x08, 0x41, 0x80, 0x10, 0x41, 0x10, 0xfc, 0x0a, 0x00, 0x00, 0x41, 0x90, 0x08, 0x20, 0x00, 0x20, 0x01, 0xfc, 0x0a, 0x00, 0x00, 0x41, 0x80, 0x08, 0x41, 0x10, 0x20, 0x01, 0x6a, 0x0b,
	0x0b, 0x17, 0x01, 0x00, 0x41, 0x80, 0x10, 0x0b, 0x10,
	'>', 'W', 'A', 'S', 'M', ' ', 'P', 'A', 'G', 'E', '\n', '-', '-', '-', '-', '\n',
}

// TestNodeServesWasmPage verifies that a .wasm page request renders through
// the sandbox: the response is dynamic Micron markup (canned prefix plus the
// request metadata), the served-page counter increments, and OnPageServed
// fires. A static .mu page keeps its plain file content.
func TestNodeServesWasmPage(t *testing.T) {
	t.Parallel()

	dir := testutils.TempDir(t, "nomadnet-node-wasm")
	wasmPath := filepath.Join(dir, "dynamic.wasm")
	if err := os.WriteFile(wasmPath, nodePageWasm, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	muPath := filepath.Join(dir, "index.mu")
	if err := os.WriteFile(muPath, []byte("static content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	served := 0
	n := NewNode("test-node", dir, dir, 10, 10, 10, false)
	n.OnPageServed = func() { served++ }

	handler := n.makePageHandler(wasmPath)
	out := handler("/page/dynamic.wasm", nil, []byte("req"), []byte("link"), nil, time.Unix(1730000000, 0))
	markup, ok := out.([]byte)
	if !ok {
		t.Fatalf("wasm page handler returned %T, want []byte", out)
	}
	if !strings.HasPrefix(string(markup), ">WASM PAGE\n----\n") {
		t.Fatalf("wasm markup = %q, want the canned prefix", markup)
	}
	var req wasmpages.PageRequest
	if err := json.Unmarshal([]byte(string(markup[len(">WASM PAGE\n----\n"):])), &req); err != nil {
		t.Fatalf("markup suffix does not decode as the request JSON: %v (%q)", err, markup)
	}
	if req.Path != "/page/dynamic.wasm" {
		t.Errorf("request payload path = %q, want /page/dynamic.wasm", req.Path)
	}
	n.mu.Lock()
	gotServed := n.ServedPageRequests
	n.mu.Unlock()
	if gotServed != 1 {
		t.Errorf("ServedPageRequests = %v, want 1", gotServed)
	}
	if served != 1 {
		t.Errorf("OnPageServed ran %v time(s), want 1", served)
	}

	// A static page keeps its file content and the same accounting.
	staticOut := n.makePageHandler(muPath)("/page/index.mu", nil, []byte("req"), nil, nil, time.Unix(1730000000, 0))
	if string(staticOut.([]byte)) != "static content" {
		t.Errorf("static page = %q, want %q", staticOut.([]byte), "static content")
	}
	n.mu.Lock()
	gotServed = n.ServedPageRequests
	n.mu.Unlock()
	if gotServed != 2 {
		t.Errorf("ServedPageRequests = %v, want 2 after both pages", gotServed)
	}
}

// TestNodeWasmPageDenied verifies that the .allowed gate runs before the
// sandbox: a denied peer gets the not-allowed page, not plugin output.
func TestNodeWasmPageDenied(t *testing.T) {
	t.Parallel()

	dir := testutils.TempDir(t, "nomadnet-node-wasm")
	wasmPath := filepath.Join(dir, "locked.wasm")
	if err := os.WriteFile(wasmPath, nodePageWasm, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(wasmPath+".allowed", []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile(.allowed): %v", err)
	}

	n := NewNode("test-node", dir, dir, 10, 10, 10, false)
	handler := n.makePageHandler(wasmPath)
	out := handler("/page/locked.wasm", nil, []byte("req"), []byte("link"), &rns.Identity{}, time.Unix(1730000000, 0))
	if string(out.([]byte)) != string(ServeNotAllowed()) {
		t.Errorf("denied .wasm request = %q, want the not-allowed page", out)
	}
}
