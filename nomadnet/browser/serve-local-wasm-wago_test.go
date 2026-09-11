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

// This file verifies that the browser's loopback page serving renders .wasm
// executable pages through the sandboxed wasm renderer, mirroring Python's
// executable-page subprocess branch (Browser.py:1306-1316).

package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/wasmpages"
	"github.com/gmlewis/go-reticulum/testutils"
)

// browserPageWasm mirrors the wasmpages package's dynamic page fixture.
var browserPageWasm = []byte{
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

// TestServeLocalWasmPage verifies that a local .wasm page request renders
// through the sandbox: the response is dynamic Micron markup carrying the
// request metadata, not the raw plugin bytes.
func TestServeLocalWasmPage(t *testing.T) {
	t.Parallel()

	pages := testutils.TempDir(t, "nomadnet-browser-wasm")
	wasmPath := filepath.Join(pages, "dynamic.wasm")
	if err := os.WriteFile(wasmPath, browserPageWasm, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := ServeLocalPage(pages, "/page/dynamic.wasm")
	if !strings.HasPrefix(string(got), ">WASM PAGE\n----\n") {
		t.Fatalf("ServeLocalPage = %q, want the canned Micron prefix", got)
	}
	var req wasmpages.PageRequest
	if err := json.Unmarshal([]byte(string(got[len(">WASM PAGE\n----\n"):])), &req); err != nil {
		t.Fatalf("markup suffix does not decode as the request JSON: %v (%q)", err, got)
	}
	if req.Path != "/page/dynamic.wasm" {
		t.Errorf("request payload path = %q, want /page/dynamic.wasm", req.Path)
	}
}

// TestServeLocalWasmPageBrokenPlugin verifies that a plugin that fails to
// load or run returns the not-found body rather than leaking the plugin's
// binary source into the page.
func TestServeLocalWasmPageBrokenPlugin(t *testing.T) {
	t.Parallel()

	pages := testutils.TempDir(t, "nomadnet-browser-wasm")
	wasmPath := filepath.Join(pages, "broken.wasm")
	if err := os.WriteFile(wasmPath, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := ServeLocalPage(pages, "/page/broken.wasm"); string(got) != string(LocalPageNotFound) {
		t.Errorf("broken plugin page = %q, want not-found", got)
	}
}
