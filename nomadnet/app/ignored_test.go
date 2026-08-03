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
	"testing"
)

func TestIgnoredListLoadAndIsIgnored(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	// seed an ignored file with two hashes + one bad line
	content := "0102030405060708090a0b0c0d0e0f10\nnotahex\naabbccddeeff00112233445566778899\n"
	if err := os.WriteFile(a.IgnoredPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a.loadIgnoredList()
	if len(a.IgnoredList) != 2 {
		t.Fatalf("IgnoredList len = %v, want 2", len(a.IgnoredList))
	}
	if !a.IsIgnored([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}) {
		t.Fatal("first hash not ignored")
	}
	if a.IsIgnored([]byte{0xff}) {
		t.Fatal("random hash should not be ignored")
	}
}

func TestBlockAndUnblockDestinationPersistence(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	h := []byte{0xde, 0xad, 0xbe, 0xef}
	if a.IsIgnored(h) {
		t.Fatal("should not be ignored initially")
	}
	if !a.BlockDestination(h, "test") {
		t.Fatal("BlockDestination should return true")
	}
	if !a.IsIgnored(h) {
		t.Fatal("should be ignored after block")
	}
	// persisted
	data, err := os.ReadFile(a.IgnoredPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("ignored file should not be empty")
	}
	// reload from disk fresh
	a2 := NewApp(a.ConfigDir, "", false, false)
	a2.setupPaths()
	a2.loadIgnoredList()
	if !a2.IsIgnored(h) {
		t.Fatal("reloaded app should still ignore h")
	}
	if !a2.UnblockDestination(h) {
		t.Fatal("UnblockDestination should return true")
	}
	if a2.IsIgnored(h) {
		t.Fatal("should not be ignored after unblock")
	}
}

func TestBlockDestinationNil(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if a.BlockDestination(nil, "") {
		t.Fatal("nil should not block")
	}
	if a.UnblockDestination(nil) {
		t.Fatal("nil should not unblock")
	}
}
