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

// This file verifies the stub page renderer: without the wago build tag the
// renderer is permanently unavailable, so the node and browser fall back to
// serving .wasm page files statically (the pre-sandbox behavior).

package wasmpages

import (
	"errors"
	"testing"
)

// TestStubDisabled verifies that the stub build reports the renderer as
// unavailable and every render call errors.
func TestEnabledStub(t *testing.T) {
	t.Parallel()

	if Enabled() {
		t.Fatal("stub build reports Enabled, want false")
	}
}

// TestRenderStub verifies that stub renders fail with the sentinel error.
func TestRenderStub(t *testing.T) {
	t.Parallel()

	markup, err := Render(writeFixture(t, MinWasmPagePlugin), PageRequest{Path: "/page/page.wasm"})
	if err == nil {
		t.Fatalf("stub Render returned (%q, nil), want an error", markup)
	}
	if len(markup) != 0 {
		t.Errorf("stub Render markup = %q, want empty", markup)
	}
	if !errors.Is(err, ErrWagoNotLinked) {
		t.Errorf("stub Render error = %v, want ErrWagoNotLinked", err)
	}
}

// TestRenderStubMissingFile verifies that a missing plugin file reports a
// clean error in either build.
func TestRenderStubMissingFile(t *testing.T) {
	t.Parallel()

	_, err := Render(tempDir(t)+"/missing.wasm", PageRequest{Path: "/page/missing.wasm"})
	if err == nil {
		t.Fatal("Render of a missing file succeeded, want an error")
	}
}
