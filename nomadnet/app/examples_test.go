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

package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFirstRunCopiesExamples pins the first-run examples copy (Python
// NomadNetworkApp.py:215-223): a fresh config directory receives the packaged
// examples tree, and a later run leaves the user's edited copy untouched.
func TestFirstRunCopiesExamples(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)

	if err := a.copyExamplesIfAbsent(); err != nil {
		t.Fatalf("copyExamplesIfAbsent: %v", err)
	}

	muPath := filepath.Join(a.ExamplesPath, "messageboard", "messageboard.mu")
	data, err := os.ReadFile(muPath)
	if err != nil {
		t.Fatalf("the examples tree was not copied: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("copied messageboard.mu is empty")
	}
	readmePath := filepath.Join(a.ExamplesPath, "messageboard", "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("copied tree is missing README.md: %v", err)
	}

	// A user edit must survive the next run.
	const edited = "; my own board\n"
	if err := os.WriteFile(muPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.copyExamplesIfAbsent(); err != nil {
		t.Fatalf("second copyExamplesIfAbsent: %v", err)
	}
	after, err := os.ReadFile(muPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != edited {
		t.Errorf("copied example was overwritten on the second run: %q", after)
	}
}
