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

// This file verifies the stub-build .wasm page behavior in the browser:
// without the wago runtime the loopback keeps serving .wasm files statically
// (their source bytes), exactly the pre-sandbox behavior.

package browser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/wasmpages"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestServeLocalWasmPageStub verifies that in the stub build a local .wasm
// page reads the file statically: any bytes are served verbatim.
func TestServeLocalWasmPageStub(t *testing.T) {
	t.Parallel()

	if wasmpages.Enabled() {
		t.Fatal("stub build reports wasmpages.Enabled, want false")
	}

	pages := testutils.TempDir(t, "nomadnet-browser-wasm")
	wasmPath := filepath.Join(pages, "dynamic.wasm")
	if err := os.WriteFile(wasmPath, []byte("arbitrary plugin source bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := ServeLocalPage(pages, "/page/dynamic.wasm"); string(got) != "arbitrary plugin source bytes" {
		t.Errorf("stub .wasm page = %q, want the static file bytes", got)
	}
}
