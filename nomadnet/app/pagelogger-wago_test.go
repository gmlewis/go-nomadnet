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

// This file verifies that the app routes a .wasm executable page's rns.log
// messages into its own logger, so an operator running the node sees what its
// pages log.

package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/wasmpages"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

// logPageWasm imports rns.log and renders by emitting a fixed message before
// returning it:
//
//	(module (import "rns" "log" (func (param i32 i32)))
//	        (func $alloc (param i32) (result i32) i32.const 4096)
//	        (func $render (param i32 i32) (result i32 i32)
//	          i32.const 64 i32.const 16 call 0 i32.const 64 i32.const 16)
//	        (memory 1 1)
//	        (data (i32.const 64) "hello from guest")).
var logPageWasm = []byte{
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

// TestInstallPageLoggerRoutesGuestLog pins that a page's rns.log message ends
// up in the app's logger, tagged with the page that emitted it.
func TestInstallPageLoggerRoutesGuestLog(t *testing.T) {
	// SetLogFunc installs a package-wide logger, so this test must not run
	// concurrently with other render tests.
	var mu sync.Mutex
	var lines []string
	a := &App{Logger: rns.NewLogger()}
	a.Logger.SetLogLevel(rns.LogInfo)
	a.Logger.SetLogDest(rns.LogCallback)
	a.Logger.SetLogCallback(func(line string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, line)
	})
	t.Cleanup(func() {
		wasmpages.SetLogFunc(nil)
		a.Logger.Close()
	})
	a.installPageLogger()

	dir := testutils.TempDir(t, "nomadnet-app-page-log")
	path := filepath.Join(dir, "log-page.wasm")
	if err := os.WriteFile(path, logPageWasm, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wasmpages.Render(path, wasmpages.PageRequest{Path: "/page/log-page.wasm"}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 1 {
		t.Fatalf("app logger received %v lines (%v), want 1", len(lines), lines)
	}
	if !strings.Contains(lines[0], "hello from guest") || !strings.Contains(lines[0], "log-page.wasm") {
		t.Errorf("logged line = %q, want the guest message tagged with the page", lines[0])
	}
}
