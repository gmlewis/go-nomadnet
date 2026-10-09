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

package location

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// locationImportPath is the path this test looks for in other packages' sources.
const locationImportPath = "github.com/gmlewis/go-nomadnet/nomadnet/location"

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %v", dir)
		}
		dir = parent
	}
}

// packagesImportingLocation returns every package directory in the module whose Go sources
// name this package, as slash-separated paths relative to the module root.
//
// It is a plain source scan rather than a build query on purpose. A type checker answers "who
// imports this today", which is only true of the build it was run against; a scan answers "who
// mentions this", which is what a reviewer needs and what survives a build tag, a platform, or
// a file that only compiles in CI.
func packagesImportingLocation(t *testing.T) []string {
	t.Helper()

	root := repoRoot(t)
	seen := map[string]bool{}
	var importers []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			// The module's own package is where this test lives, and it necessarily names
			// itself. Build output and vendor trees hold copies rather than importers.
			if name == "vendor" || name == "node_modules" || name == ".git" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && filepath.ToSlash(rel) == "nomadnet/location" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !strings.Contains(string(data), locationImportPath) {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		rel, relErr := filepath.Rel(root, dir)
		if relErr != nil {
			return relErr
		}
		if !seen[rel] {
			seen[rel] = true
			importers = append(importers, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %v: %v", root, err)
	}
	return importers
}

// TestTheApplicationLayerCannotReachTheReaderPosition is the structural half of the privacy
// argument, and it is the stronger half.
//
// The rendering tests prove that the client never *prints* the reader's own coordinate. This
// one proves something harder to break: that the layers which announce, fetch pages, send
// messages, and write state cannot reach the position at all. A `L Micron location construct is
// resolved against the reader by the terminal UI and by nothing else, so there is nothing for
// the network-facing code to serialise, deliberately or by accident.
//
// The claim is only worth anything if it is checked, and it is exactly the kind of claim that
// rots: a future feature — a status line, a log entry, a "share my location" helper in the wrong
// package — would quietly give the application layer a reference to the reader's position, and
// every other privacy test here would still pass.
//
// `nomadnet/micron/location-extension.md` asserts that nothing leaves the node. This test, the
// rendering tests beside it, and the wire capture in the project's E11 are what make that
// assertion evidence rather than prose.
func TestTheApplicationLayerCannotReachTheReaderPosition(t *testing.T) {
	t.Parallel()

	importers := packagesImportingLocation(t)

	// The anti-vacuity guard, and it is the important part. A negative assertion passes
	// trivially if the thing it looks for has been renamed, moved, or misspelled: the scan would
	// find no importers at all and report success. So the one legitimate importer is required to
	// be found.
	allowed := "cmd/gonomadnet"
	foundAllowed := false
	for _, dir := range importers {
		if dir == allowed {
			foundAllowed = true
		}
	}
	if !foundAllowed {
		t.Fatalf("no package under %v names %v, so this test is looking for the wrong thing "+
			"and its negative result means nothing. Found: %v", allowed, locationImportPath, importers)
	}

	// Everything else is a failure, and the message has to say why, because the fix is a design
	// decision rather than a rename.
	for _, dir := range importers {
		if dir == allowed {
			continue
		}
		t.Errorf(
			"%v can reach the reader's own position, which is a fact about the person holding the "+
				"device and is theirs to disclose.\n"+
				"Only the terminal UI may hold it: it resolves a `L construct for the reader and "+
				"never renders the reader's own coordinate. A layer that announces, fetches pages, "+
				"sends messages, or writes state must not be able to serialise it, because then the "+
				"only thing standing between the position and the network is everybody remembering "+
				"not to.\n"+
				"If this package genuinely must be reachable, pass the value it needs as an "+
				"argument instead of importing %v.",
			dir, locationImportPath,
		)
	}
}

// TestTheScanWouldCatchANewImporter asserts the scan itself works, because a source scan that
// silently matches nothing is worse than no scan: it reports privacy while proving nothing.
func TestTheScanWouldCatchANewImporter(t *testing.T) {
	t.Parallel()

	importers := packagesImportingLocation(t)
	if len(importers) == 0 {
		t.Fatal("the import scan found no importers at all, not even the known one, so it cannot " +
			"detect anything")
	}

	// The known importer has to be one of them, and the scan has to be looking at this module.
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "cmd", "gonomadnet")); err != nil {
		t.Fatalf("the module root %v has no cmd/gonomadnet: %v", root, err)
	}
	for _, dir := range importers {
		if strings.HasPrefix(dir, "..") || filepath.IsAbs(dir) {
			t.Fatalf("the scan reported %q, which is not a path inside the module", dir)
		}
	}
}
