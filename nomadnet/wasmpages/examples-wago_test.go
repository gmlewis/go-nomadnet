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

// This file smoke-tests the shipped example executable page from
// assets/wasm-pages/ so the repository's example binary is guaranteed to
// work with the tools that document it.

package wasmpages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleDynamicPageWorks loads the shipped dynamic-page example and
// renders it through the sandboxed renderer.
func TestExampleDynamicPageWorks(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "assets", "wasm-pages", "dynamic-page.wasm")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("example page missing: %v", err)
	}
	markup, err := Render(path, PageRequest{Path: "/page/dynamic.wasm", RequestedAt: 1730000000})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.HasPrefix(string(markup), ">WASM PAGE\n----\n") {
		t.Fatalf("markup = %q, want the canned prefix", markup)
	}
	if !strings.Contains(string(markup), `"path":"/page/dynamic.wasm"`) {
		t.Errorf("markup = %q, want the request payload appended", markup)
	}
}
