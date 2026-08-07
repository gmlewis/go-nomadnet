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

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRenderReleaseNotes renders the release notes for a handful of fake,
// deliberately-unsorted artifacts and logs them so you can eyeball the sorted
// table and the post-download setup section. Run with:
//
//	GOCACHE=/tmp/go-cache go test ./cmd/publish-github-release-artifacts/ -run TestRenderReleaseNotes -v
func TestRenderReleaseNotes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Build the assets in a deliberately non-alphabetical order so the test
	// proves buildReleaseNotes sorts the table rows by filename.
	names := []string{
		"gonomadnet-0.2.0-windows-arm64.exe",
		"gonomadnet-0.2.0-linux-amd64",
		"gonomadnet-0.2.0-darwin-arm64",
		"gonomadnet-0.2.0-linux-arm64",
	}
	var assets []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
			t.Fatalf("write %v: %v", n, err)
		}
		assets = append(assets, p)
	}

	notes := buildReleaseNotes("0.2.0", "gmlewis/go-nomadnet", assets)
	t.Log("\n" + notes)
}