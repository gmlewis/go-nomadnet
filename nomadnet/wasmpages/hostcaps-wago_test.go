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

// This file verifies the page plugins' host capabilities: rns.log and the
// per-plugin KV scratch store behind rns.kv_get / rns.kv_set, which let an
// executable page remember state across requests even though every render
// runs a fresh sandbox instance.

package wasmpages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// MinWasmKVPagePlugin imports rns.kv_get and rns.kv_set and exports
// wagoplugin_alloc + render_page. render_page reads the previously stored
// value into guest memory at 1024, stores the current request under the same
// key, and returns the bytes it read (or the canned "MISSING" marker when the
// key was absent), so the rendered markup proves the store persisted across
// renders:
//
//	(module (import "rns" "kv_get" (func (param i32 i32 i32 i32) (result i32)))
//	        (import "rns" "kv_set" (func (param i32 i32 i32 i32) (result i32)))
//	        (func $alloc (param i32) (result i32) i32.const 4096)
//	        (func $render (param i32 i32) (result i32 i32)
//	          (local $n i32)
//	          i32.const 64 i32.const 5 i32.const 1024 i32.const 128 call 0
//	          local.set $n
//	          i32.const 64 i32.const 5 local.get 0 local.get 1 call 1 drop
//	          local.get $n i32.const 0 i32.gt_s
//	          if (result i32 i32) i32.const 1024 local.get $n
//	          else i32.const 512 i32.const 7 end)
//	        (memory 1 1)
//	        (data (i32.const 64) "kvkey") (data (i32.const 512) "MISSING")).
var MinWasmKVPagePlugin = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x1a, 0x04, 0x60, 0x04, 0x7f, 0x7f, 0x7f,
	0x7f, 0x01, 0x7f, 0x60, 0x01, 0x7f, 0x01, 0x7f, 0x60, 0x02, 0x7f, 0x7f, 0x02, 0x7f, 0x7f, 0x60,
	0x00, 0x02, 0x7f, 0x7f, 0x02, 0x1b, 0x02, 0x03, 0x72, 0x6e, 0x73, 0x06, 0x6b, 0x76, 0x5f, 0x67,
	0x65, 0x74, 0x00, 0x00, 0x03, 0x72, 0x6e, 0x73, 0x06, 0x6b, 0x76, 0x5f, 0x73, 0x65, 0x74, 0x00,
	0x00, 0x03, 0x03, 0x02, 0x01, 0x02, 0x05, 0x04, 0x01, 0x01, 0x01, 0x01, 0x07, 0x2b, 0x03, 0x06,
	0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00, 0x10, 0x77, 0x61, 0x67, 0x6f, 0x70, 0x6c, 0x75,
	0x67, 0x69, 0x6e, 0x5f, 0x61, 0x6c, 0x6c, 0x6f, 0x63, 0x00, 0x02, 0x0b, 0x72, 0x65, 0x6e, 0x64,
	0x65, 0x72, 0x5f, 0x70, 0x61, 0x67, 0x65, 0x00, 0x03, 0x0a, 0x3a, 0x02, 0x05, 0x00, 0x41, 0x80,
	0x20, 0x0b, 0x32, 0x01, 0x01, 0x7f, 0x41, 0xc0, 0x00, 0x41, 0x05, 0x41, 0x80, 0x08, 0x41, 0x80,
	0x01, 0x10, 0x00, 0x21, 0x02, 0x41, 0xc0, 0x00, 0x41, 0x05, 0x20, 0x00, 0x20, 0x01, 0x10, 0x01,
	0x1a, 0x20, 0x02, 0x41, 0x00, 0x4a, 0x04, 0x03, 0x41, 0x80, 0x08, 0x20, 0x02, 0x05, 0x41, 0x80,
	0x04, 0x41, 0x07, 0x0b, 0x0b, 0x0b, 0x19, 0x02, 0x00, 0x41, 0xc0, 0x00, 0x0b, 0x05, 0x6b, 0x76,
	0x6b, 0x65, 0x79, 0x00, 0x41, 0x80, 0x04, 0x0b, 0x07, 0x4d, 0x49, 0x53, 0x53, 0x49, 0x4e, 0x47,
}

// MinWasmUnwiredImportPlugin imports rns.now, a capability the page host does
// not wire, so the deny-by-default policy must refuse instantiation:
//
//	(module (import "rns" "now" (func (result i64))) (func (export "render_page")
//	  (param i32 i32) (result i32 i32) i32.const 0 i32.const 0)).
var MinWasmUnwiredImportPlugin = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	// Type section: 0: () -> (i64), 1: (i32, i32) -> (i32, i32)
	0x01, 0x0d, 0x02,
	0x60, 0x00, 0x01, 0x7e,
	0x60, 0x02, 0x7f, 0x7f, 0x02, 0x7f, 0x7f,
	// Import section: "rns"."now" -> type 0
	0x02, 0x0b, 0x01,
	0x03, 'r', 'n', 's',
	0x03, 'n', 'o', 'w',
	0x00, 0x00,
	// Function section: 1 local function of type 1
	0x03, 0x02, 0x01, 0x01,
	// Export section: render_page -> func 1
	0x07, 0x0f, 0x01,
	0x0b, 'r', 'e', 'n', 'd', 'e', 'r', '_', 'p', 'a', 'g', 'e', 0x00, 0x01,
	// Code section: one body, locals 0, i32.const 0, i32.const 0, end
	0x0a, 0x08, 0x01,
	0x06, 0x00, 0x41, 0x00, 0x41, 0x00, 0x0b,
}

// TestRenderPageKVStoreRoundTrip pins that a page plugin can persist a value
// through rns.kv_set and read it back through rns.kv_get on a later render:
// the first render reports the absent key, the second render returns the bytes
// the first one stored.
func TestRenderPageKVStoreRoundTrip(t *testing.T) {
	t.Parallel()

	pages := tempDir(t)
	path := filepath.Join(pages, "kv-page.wasm")
	if err := os.WriteFile(path, MinWasmKVPagePlugin, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	first, err := Render(path, PageRequest{Path: "/page/kv-page.wasm", RequestedAt: 1})
	if err != nil {
		t.Fatalf("first Render: %v", err)
	}
	if string(first) != "MISSING" {
		t.Fatalf("first render = %q, want the absent-key marker", first)
	}

	second, err := Render(path, PageRequest{Path: "/page/kv-page.wasm", RequestedAt: 2})
	if err != nil {
		t.Fatalf("second Render: %v", err)
	}
	if strings.Contains(string(second), "MISSING") {
		t.Fatalf("second render = %q, want the value stored by the first render", second)
	}
	if !strings.Contains(string(second), `"requested_at":1`) {
		t.Errorf("second render = %q, want the first render's request payload", second)
	}
}

// TestRenderPageKVStoreScopedPerPlugin pins the store layout: a page plugin's
// keys live under <pages>/data/<plugin>/ so two pages on the same node never
// share state.
func TestRenderPageKVStoreScopedPerPlugin(t *testing.T) {
	t.Parallel()

	pages := tempDir(t)
	path := filepath.Join(pages, "kv-page.wasm")
	if err := os.WriteFile(path, MinWasmKVPagePlugin, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Render(path, PageRequest{Path: "/page/kv-page.wasm", RequestedAt: 1}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	keyFile := filepath.Join(pages, "data", "kv-page", "kvkey")
	stored, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", keyFile, err)
	}
	if !strings.Contains(string(stored), `"requested_at":1`) {
		t.Errorf("stored value = %q, want the request payload", stored)
	}
}

// TestRenderPageDeniesUnwiredImport pins the deny-by-default policy after the
// KV and log capabilities were wired: a module importing a capability the host
// does not provide fails to load rather than reaching the guest.
func TestRenderPageDeniesUnwiredImport(t *testing.T) {
	t.Parallel()

	pages := tempDir(t)
	path := filepath.Join(pages, "unwired.wasm")
	if err := os.WriteFile(path, MinWasmUnwiredImportPlugin, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Render(path, PageRequest{Path: "/page/unwired.wasm"}); err == nil {
		t.Fatal("Render of a module importing rns.now succeeded, want a load error")
	}
}

// MinWasmLogPagePlugin imports rns.log and renders by emitting a fixed
// message before returning it, so the test observes the guest-to-host log
// path:
//
//	(module (import "rns" "log" (func (param i32 i32)))
//	        (func $alloc (param i32) (result i32) i32.const 4096)
//	        (func $render (param i32 i32) (result i32 i32)
//	          i32.const 64 i32.const 16 call 0 i32.const 64 i32.const 16)
//	        (memory 1 1)
//	        (data (i32.const 64) "hello from guest")).
var MinWasmLogPagePlugin = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x12, 0x03, 0x60, 0x02, 0x7f, 0x7f, 0x00,
	0x60, 0x01, 0x7f, 0x01, 0x7f, 0x60, 0x02, 0x7f, 0x7f, 0x02, 0x7f, 0x7f, 0x02, 0x0b, 0x01, 0x03,
	0x72, 0x6e, 0x73, 0x03, 0x6c, 0x6f, 0x67, 0x00, 0x00, 0x03, 0x03, 0x02, 0x01, 0x02, 0x05, 0x04,
	0x01, 0x01, 0x01, 0x01, 0x07, 0x2b, 0x03, 0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00,
	0x10, 0x77, 0x61, 0x67, 0x6f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x5f, 0x61, 0x6c, 0x6c, 0x6f,
	0x63, 0x00, 0x01, 0x0b, 0x72, 0x65, 0x6e, 0x64, 0x65, 0x72, 0x5f, 0x70, 0x61, 0x67, 0x65, 0x00,
	0x02, 0x0a, 0x16, 0x02, 0x05, 0x00, 0x41, 0x80, 0x20, 0x0b, 0x0e, 0x00, 0x41, 0xc0, 0x00, 0x41,
	0x10, 0x10, 0x00, 0x41, 0xc0, 0x00, 0x41, 0x10, 0x0b, 0x0b, 0x17, 0x01, 0x00, 0x41, 0xc0, 0x00,
	0x0b, 0x10, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x66, 0x72, 0x6f, 0x6d, 0x20, 0x67, 0x75, 0x65,
	0x73, 0x74,
}

// TestRenderPageLogImport pins that rns.log forwards a guest message to the
// installed logger, tagged with the page that emitted it.
func TestRenderPageLogImport(t *testing.T) {
	// SetLogFunc installs a package-wide logger, so this test must not run
	// concurrently with the other render tests.
	var logMu sync.Mutex
	var messages []string
	SetLogFunc(func(format string, args ...any) {
		logMu.Lock()
		defer logMu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	})
	t.Cleanup(func() { SetLogFunc(nil) })

	pages := tempDir(t)
	path := filepath.Join(pages, "log-page.wasm")
	if err := os.WriteFile(path, MinWasmLogPagePlugin, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Render(path, PageRequest{Path: "/page/log-page.wasm"}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	logMu.Lock()
	defer logMu.Unlock()
	if len(messages) != 1 {
		t.Fatalf("logger received %v messages (%v), want 1", len(messages), messages)
	}
	if !strings.Contains(messages[0], "hello from guest") || !strings.Contains(messages[0], "log-page.wasm") {
		t.Errorf("logged message = %q, want the guest message tagged with the page", messages[0])
	}
}
