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
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

// These tests cover the app-level persistence writers end to end: they drive the App
// methods, then read the state back through a FRESH App over the same config directory.
// The on-disk formats are already pinned byte-for-byte by the *-byteparity tests (see
// parity-gaps §5.1), so what is under test here is the wiring those format tests cannot
// see — the right path, the right field, and the write actually happening. A writer that
// silently wrote to the wrong file, or a field the loader never restored, would pass
// every format test and still lose the user's setting on restart.

// newPersistApp builds an app over dir with a real (offline) transport, initialized far
// enough to persist and load peer settings and the ignored list.
func newPersistApp(t *testing.T, dir string) *App {
	t.Helper()
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport: %v", err)
	}
	t.Cleanup(a.Shutdown)
	return a
}

// TestPeerSettingsRoundTripThroughTheApp pins that the settings an App writes are
// restored by the next App over the same directory: the display name set through
// SetDisplayName (which persists) and the propagation node chosen through
// SetUserSelectedPropagationNode. Both are user-visible after a restart — the advertised
// peer name and where messages are propagated — and neither had an app-level round-trip
// test.
func TestPeerSettingsRoundTripThroughTheApp(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)

	const name = "Round Trip Peer"
	propNode := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0x01, 0x02, 0x03, 0x04,
		0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}

	first := newPersistApp(t, dir)
	first.SetDisplayName(name)
	first.SetUserSelectedPropagationNode(propNode)

	// The files must exist on disk before the second app is built, so a failure here
	// is a write that never happened rather than a load that lost it.
	if _, err := os.Stat(first.PeerSettingsPath); err != nil {
		t.Fatalf("peer settings were not written to %v: %v", first.PeerSettingsPath, err)
	}

	second := newPersistApp(t, dir)

	if got := second.GetDisplayName(); got != name {
		t.Errorf("display name after restart = %q, want %q", got, name)
	}
	if got := second.GetUserSelectedPropagationNode(); !bytes.Equal(got, propNode) {
		t.Errorf("propagation node after restart = %x, want %x", got, propNode)
	}
}

// TestIgnoredListRoundTripThroughTheApp pins the same for the ignored-destinations list:
// a destination blocked through the App stays blocked across a restart, both in the
// loaded list and through IsIgnored. The list is what refuses a peer's messages, so a
// write that landed nowhere would silently un-block them.
func TestIgnoredListRoundTripThroughTheApp(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)

	blocked := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00}
	alsoBlocked := []byte{0x0f, 0x1e, 0x2d, 0x3c, 0x4b, 0x5a, 0x69, 0x78,
		0x87, 0x96, 0xa5, 0xb4, 0xc3, 0xd2, 0xe1, 0xf0}

	first := newPersistApp(t, dir)
	first.BlockDestination(blocked, "spam")
	first.BlockDestination(alsoBlocked, "spam")
	if !first.IsIgnored(blocked) {
		t.Fatal("precondition: the first app does not report the blocked destination")
	}

	data, err := os.ReadFile(first.IgnoredPath)
	if err != nil {
		t.Fatalf("the ignored list was not written to %v: %v", first.IgnoredPath, err)
	}
	if !bytes.Contains(data, []byte("112233445566778899aabbccddeeff00")) {
		t.Errorf("ignored file does not hold the blocked hash: %q", data)
	}

	second := newPersistApp(t, dir)

	if !second.IsIgnored(blocked) {
		t.Error("the blocked destination is not ignored after a restart")
	}
	if !second.IsIgnored(alsoBlocked) {
		t.Error("the second blocked destination is not ignored after a restart")
	}
	if second.IsIgnored([]byte{0x00}) {
		t.Error("a destination that was never blocked reports as ignored")
	}
	if len(second.IgnoredList) != 2 {
		t.Errorf("restored ignored list holds %v entries, want 2", len(second.IgnoredList))
	}
}

// TestBlockedDestinationIsNotWrittenTwice pins the dedupe in BlockDestination: blocking
// the same destination again must not append a second line. Python guards the same way
// (`if hash not in self.ignored_list`), and a list that grew on every block would both
// bloat the file and re-decode the same entry on each start.
func TestBlockedDestinationIsNotWrittenTwice(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)

	hash := []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04,
		0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}

	a := newPersistApp(t, dir)
	a.BlockDestination(hash, "first")
	a.BlockDestination(hash, "second")

	if got := len(a.IgnoredList); got != 1 {
		t.Errorf("ignored list holds %v entries after blocking the same destination twice, want 1", got)
	}

	data, err := os.ReadFile(filepath.Clean(a.IgnoredPath))
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(data, []byte("\n")); got != 1 {
		t.Errorf("ignored file holds %v lines, want 1: %q", got, data)
	}
}
