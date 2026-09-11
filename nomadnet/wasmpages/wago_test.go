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

// This file verifies the wago .wasm page renderer: loading embedded wasm
// fixtures, rendering dynamic Micron markup through the render_page ABI, and
// interrupting a runaway native loop through the invocation deadline.

package wasmpages

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestEnabledWago verifies that the wago build reports the renderer as
// available.
func TestEnabledWago(t *testing.T) {
	t.Parallel()

	if !Enabled() {
		t.Fatal("wago build reports Enabled=false, want true")
	}
}

// TestRenderPagePlugin renders the dynamic page fixture: the response is the
// canned Micron prefix followed by the serialized request, proving the
// plugin received the request metadata and returned markup.
func TestRenderPagePlugin(t *testing.T) {
	t.Parallel()

	req := PageRequest{
		Path:        "/page/dynamic.wasm",
		LinkID:      "aabb",
		RequestedAt: 1730000000,
	}
	markup, err := Render(writeFixture(t, MinWasmPagePlugin), req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.HasPrefix(string(markup), prefixMarkup) {
		t.Fatalf("markup = %q, want the %q prefix", markup, prefixMarkup)
	}
	var got PageRequest
	if err := json.Unmarshal([]byte(string(markup[len(prefixMarkup):])), &got); err != nil {
		t.Fatalf("markup suffix does not decode as the request JSON: %v (%q)", err, markup)
	}
	if got.Path != req.Path || got.LinkID != req.LinkID || got.RequestedAt != req.RequestedAt {
		t.Errorf("markup request payload = %+v, want %+v", got, req)
	}
}

// TestRenderSpinTimeout loads the infinite-loop module and verifies that the
// invocation deadline interrupts the native loop: the call returns
// context.DeadlineExceeded and does not block. The 50ms budget leaves the
// interrupt mechanism ample headroom; the generous wall-clock bound only
// guards against a hung process, matching the engine's own cancellation
// tests (about 20ms observed).
func TestRenderSpinTimeout(t *testing.T) {
	t.Parallel()

	start := time.Now()
	_, err := invokeExport(writeFixture(t, MinWasmSpin), "spin", 50*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("invoke(spin) error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed >= time.Second {
		t.Errorf("invoke(spin) took %v, the deadline did not interrupt the loop promptly", elapsed)
	}
	// A second invocation still works after the interrupted one.
	if _, err := invokeExport(writeFixture(t, MinWasmSpin), "spin", 50*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("second invoke(spin) error = %v, want context.DeadlineExceeded", err)
	}
}

// TestRenderMissingFile verifies that a missing plugin file reports a clean
// error.
func TestRenderMissingFile(t *testing.T) {
	t.Parallel()

	if _, err := Render(tempDir(t)+"/missing.wasm", PageRequest{Path: "/page/missing.wasm"}); err == nil {
		t.Fatal("Render of a missing file succeeded, want an error")
	}
}
