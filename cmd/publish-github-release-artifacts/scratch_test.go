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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gmlewis/go-reticulum/testutils"
)

// sweeperPath is the /tmp sweep script, named relative to this package's
// directory because go test runs with the package dir as the working directory.
const sweeperPath = "../../scripts/clean-test-tmp.sh"

// sweepPrefixLists parses the sweep script's two prefix arrays, so the tests
// below assert against the lists the sweeper really uses rather than a copy of
// them that could drift out of step with it.
func sweepPrefixLists(t *testing.T) (swept, notSwept []string) {
	t.Helper()

	data, err := os.ReadFile(sweeperPath)
	if err != nil {
		t.Fatalf("read %v: %v", sweeperPath, err)
	}

	parse := func(name string) []string {
		var prefixes []string
		inList := false
		for line := range strings.SplitSeq(string(data), "\n") {
			switch {
			case strings.HasPrefix(line, name+"=("):
				inList = true
			case inList && strings.TrimSpace(line) == ")":
				return prefixes
			case inList:
				prefixes = append(prefixes, strings.Fields(line)...)
			}
		}
		return prefixes
	}
	return parse("prefixes"), parse("not_swept_prefixes")
}

// TestScratchDirNeverSwept pins the property whose absence let a concurrent
// test run delete a publish's artifacts: the scratch dir's name must match no
// prefix the /tmp sweeper deletes, and must be named in the sweep script's
// not_swept_prefixes so its own drift check keeps it that way.
func TestScratchDirNeverSwept(t *testing.T) {
	t.Parallel()

	swept, notSwept := sweepPrefixLists(t)
	name := strings.TrimSuffix(scratchDirPrefix, "*")

	for _, prefix := range swept {
		if strings.HasPrefix(name, prefix) {
			t.Errorf("scratch dir %v is matched by swept prefix %v: a concurrent "+
				"test run's /tmp sweep would delete the artifacts mid-publish",
				name, prefix)
		}
	}

	if !slices.ContainsFunc(notSwept, func(prefix string) bool {
		return strings.HasPrefix(name, prefix)
	}) {
		t.Errorf("scratch dir %v matches no not_swept_prefixes entry in %v, so "+
			"that script's -c check cannot keep a future swept prefix from "+
			"swallowing it", name, sweeperPath)
	}
}

// TestScratchInUseMarkerIsHonored checks the other half of the agreement with
// the sweeper: it must know the marker file name this program writes, and must
// decide on the marker's PID being alive rather than on the file existing, or a
// marker left by a killed run would make its scratch dir permanent.
func TestScratchInUseMarkerIsHonored(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(sweeperPath)
	if err != nil {
		t.Fatalf("read %v: %v", sweeperPath, err)
	}
	script := string(data)

	if !strings.Contains(script, inUseMarker) {
		t.Errorf("sweeper %v does not know the %v marker this program writes, so "+
			"a sweep would delete a running publish's scratch dir",
			sweeperPath, inUseMarker)
	}
	if !strings.Contains(script, "kill -0") {
		t.Errorf("sweeper %v does not test whether the marker's PID is alive, so "+
			"a stale marker would protect its scratch dir forever", sweeperPath)
	}
}

// TestMarkScratchInUse verifies the marker names this process, which is what
// makes the sweeper leave the directory alone.
func TestMarkScratchInUse(t *testing.T) {
	t.Parallel()

	dir := testutils.TempDir(t, "publish-release-test-")
	if err := markScratchInUse(dir); err != nil {
		t.Fatalf("markScratchInUse: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, inUseMarker))
	if err != nil {
		t.Fatalf("read %v: %v", inUseMarker, err)
	}
	if want := fmt.Sprint(os.Getpid()); strings.TrimSpace(string(got)) != want {
		t.Errorf("marker holds PID %q, want %v", strings.TrimSpace(string(got)), want)
	}
}

// TestVerifyArtifacts covers the guard that turns a vanished artifact into an
// error naming the file, instead of gh's opaque "no matches found for <path>".
func TestVerifyArtifacts(t *testing.T) {
	t.Parallel()

	dir := testutils.TempDir(t, "publish-release-test-")
	present := filepath.Join(dir, "gonomadnet-0.1.0-linux-amd64")
	if err := os.WriteFile(present, []byte("binary"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	missing := filepath.Join(dir, "gonomadnet-0.1.0-darwin-arm64")

	tests := []struct {
		name    string
		assets  []string
		wantErr string // substring the error must contain; "" means no error
	}{
		{"all present", []string{present}, ""},
		{"no assets", nil, ""},
		{"one missing", []string{present, missing}, missing},
		{"all missing", []string{missing}, missing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyArtifacts(tt.assets)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("verifyArtifacts(%v) = %v, want nil", tt.assets, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("verifyArtifacts(%v) = nil, want an error naming %v",
					tt.assets, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("verifyArtifacts error %v does not name %v", err, tt.wantErr)
			}
		})
	}
}
