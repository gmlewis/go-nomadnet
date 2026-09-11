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

//go:build !wago || (!linux && !darwin && !windows) || (!amd64 && !arm64)

// This file verifies the stub-build .wasm page behavior: without the wago
// runtime the node keeps serving .wasm page files statically (their source
// bytes), exactly the pre-sandbox behavior.

package node

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/wasmpages"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestNodeServesWasmPageStatically verifies that in the stub build a .wasm
// page request reads the file statically: any bytes are served verbatim and
// the served-page accounting still increments.
func TestNodeServesWasmPageStub(t *testing.T) {
	t.Parallel()

	if wasmpages.Enabled() {
		t.Fatal("stub build reports wasmpages.Enabled, want false")
	}

	dir := testutils.TempDir(t, "nomadnet-node-wasm")
	wasmPath := filepath.Join(dir, "dynamic.wasm")
	if err := os.WriteFile(wasmPath, []byte("arbitrary plugin source bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	n := NewNode("test-node", dir, dir, 10, 10, 10, false)
	handler := n.makePageHandler(wasmPath)
	out := handler("/page/dynamic.wasm", nil, []byte("req"), []byte("link"), nil, time.Unix(1730000000, 0))
	if string(out.([]byte)) != "arbitrary plugin source bytes" {
		t.Errorf("stub .wasm page = %q, want the static file bytes", out)
	}
	n.mu.Lock()
	gotServed := n.ServedPageRequests
	n.mu.Unlock()
	if gotServed != 1 {
		t.Errorf("ServedPageRequests = %v, want 1", gotServed)
	}
}
