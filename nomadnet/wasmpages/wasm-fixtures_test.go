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

// This file holds the embedded wasm test fixtures for the wasmpages
// package. The modules are minimal hand-assembled bytecode so the tests
// never need an external wasm compiler; the wago compiler validates the
// section sizes the first time each fixture is compiled.

package wasmpages

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// tempDir returns a fresh temp directory cleaned up at test end. On macOS
// the path lives under /tmp because the default os.MkdirTemp base is too
// long for Unix domain sockets.
func tempDir(t *testing.T) string {
	t.Helper()
	base := ""
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "nomadnet-wasmpages-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// writeFixture writes fixture bytes into a fresh temp file and returns the
// path.
func writeFixture(t *testing.T, wasm []byte) string {
	t.Helper()
	path := filepath.Join(tempDir(t), "page.wasm")
	if err := os.WriteFile(path, wasm, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// MinWasmPagePlugin exports wagoplugin_alloc and render_page over one memory
// page: alloc returns the fixed offset 4096 and render_page builds the
// response as a canned 16-byte Micron prefix followed by the request bytes:
//
//	(memory.copy 1024 <- segment 2048..2064) + (memory.copy 1040 <- request)
//	return (1024, 16 + request length)
//
// so the rendered markup provably depends on the request payload.
var MinWasmPagePlugin = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	// Type section: 0: (i32) -> (i32), 1: (i32, i32) -> (i32, i32)
	0x01, 0x0d, 0x02,
	0x60, 0x01, 0x7f, 0x01, 0x7f,
	0x60, 0x02, 0x7f, 0x7f, 0x02, 0x7f, 0x7f,
	// Function section: 0: alloc, 1: render_page
	0x03, 0x03, 0x02, 0x00, 0x01,
	// Memory section: 1 page, max 1 page
	0x05, 0x04, 0x01, 0x01, 0x01, 0x01,
	// Export section (size 0x2b = 1 + 9 + 19 + 14)
	0x07, 0x2b, 0x03,
	0x06, 'm', 'e', 'm', 'o', 'r', 'y', 0x02, 0x00,
	0x10, 'w', 'a', 'g', 'o', 'p', 'l', 'u', 'g', 'i', 'n', '_', 'a', 'l', 'l', 'o', 'c', 0x00, 0x00,
	0x0b, 'r', 'e', 'n', 'd', 'e', 'r', '_', 'p', 'a', 'g', 'e', 0x00, 0x01,
	// Code section (size 0x29 = 1 + 6 + 34)
	0x0a, 0x29, 0x02,
	// func 0 (alloc): returns the fixed offset 4096 (body: 00 41 80 20 0b)
	0x05, 0x00, 0x41, 0x80, 0x20, 0x0b,
	// func 1 (render_page): copy canned markup to 1024, append request, return (1024, 16+len)
	0x21, 0x00, 0x41, 0x80, 0x08, 0x41, 0x80, 0x10, 0x41, 0x10, 0xfc, 0x0a, 0x00, 0x00, 0x41, 0x90, 0x08, 0x20, 0x00, 0x20, 0x01, 0xfc, 0x0a, 0x00, 0x00, 0x41, 0x80, 0x08, 0x41, 0x10, 0x20, 0x01, 0x6a, 0x0b,
	// Data section: canned markup at offset 2048
	0x0b, 0x17, 0x01, 0x00, 0x41, 0x80, 0x10, 0x0b, 0x10,
	'>', 'W', 'A', 'S', 'M', ' ', 'P', 'A', 'G', 'E', '\n', '-', '-', '-', '-', '\n',
}

// MinWasmSpin is the infinite-loop module used for timeout and cancellation
// tests:
//
//	(module (func (export "spin") loop br 0 end)).
var MinWasmSpin = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
	0x03, 0x02, 0x01, 0x00,
	0x07, 0x08, 0x01, 0x04, 's', 'p', 'i', 'n', 0x00, 0x00,
	// Code section: body size 7 (locals 00, loop 03 40, br 0c 00, end 0b, end 0b), section size 9.
	0x0a, 0x09, 0x01, 0x07, 0x00, 0x03, 0x40, 0x0c, 0x00, 0x0b, 0x0b,
}

// prefixMarkup is the canned 16-byte Micron prefix MinWasmPagePlugin prepends
// to every rendered response; the page-fixture tests assert on it.
const prefixMarkup = ">WASM PAGE\n----\n"
