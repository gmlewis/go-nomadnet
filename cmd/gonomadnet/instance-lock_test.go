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
	"sort"
	"testing"

	"github.com/gmlewis/go-reticulum/testutils"
)

// tempDir is a short-path temp dir (avoids the macOS t.TempDir socket-path
// length pitfall, per repo convention) cleaned up with t.Cleanup.
func tempDir(t *testing.T) string {
	t.Helper()
	return testutils.TempDir(t, "instance-lock-test-*")
}

// TestAcquireInstanceLockSingleton verifies a second acquirer on the same lock
// path is refused and given the holder's PID, while the first holder keeps the
// lock until it releases.
func TestAcquireInstanceLockSingleton(t *testing.T) {
	lockPath := filepath.Join(tempDir(t), "gonomadnet.lock")

	release1, holderPID, err := acquireInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if release1 == nil {
		t.Fatal("first acquire should succeed (release != nil)")
	}
	if holderPID != 0 {
		t.Fatalf("first acquire holderPID = %v, want 0", holderPID)
	}
	t.Cleanup(release1)

	// A second acquirer on the same path must be refused and told our PID.
	release2, holderPID2, err := acquireInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if release2 != nil {
		t.Fatal("second acquire should be refused (release == nil)")
	}
	if holderPID2 != os.Getpid() {
		t.Fatalf("second acquire holderPID = %v, want %v", holderPID2, os.Getpid())
	}
}

// TestNomadnetPIDsFromProcs verifies the nomadnet-detection filter matches real
// nomadnet invocations while excluding gonomadnet, its launcher, editors, and
// the calling process itself.
func TestNomadnetPIDsFromProcs(t *testing.T) {
	self := 9999
	procs := []processArg{
		{100, "nomadnet"},                          // nomadnet script → match
		{101, "python3 -m nomadnet"},               // python -m → match
		{102, "/usr/bin/python3 /srv/nomadnet.py"}, // python script → match
		{103, "gonomadnet -config /tmp/x"},         // gonomadnet → exclude
		{104, "bash -ex ./gonomadnet.sh"},          // launcher → exclude
		{105, "vim /home/me/nomadnet.py"},          // editor → exclude
		{106, "grep -r nomadnet ."},                // grep → exclude
		{self, "python3 -m nomadnet"},              // self → exclude
		{107, "python3 -m reticulum"},              // python, not nomadnet → exclude
	}
	got := nomadnetPIDsFromProcs(procs, self)
	sort.Ints(got)
	want := []int{100, 101, 102}
	if len(got) != len(want) {
		t.Fatalf("matched PIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matched PIDs = %v, want %v", got, want)
		}
	}
}
