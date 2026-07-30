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

package directory

import (
	"testing"
	"time"
)

func TestNumberOfKnownNodes(t *testing.T) {
	t.Parallel()
	d := New()
	if got := d.NumberOfKnownNodes(); got != 0 {
		t.Fatalf("empty dir: got %v, want 0", got)
	}
	d.Remember(&Entry{SourceHash: []byte{1}, HostsNode: true, DisplayName: "a"})
	d.Remember(&Entry{SourceHash: []byte{2}, HostsNode: false, DisplayName: "b"})
	d.Remember(&Entry{SourceHash: []byte{3}, HostsNode: true, DisplayName: "c"})
	if got := d.NumberOfKnownNodes(); got != 2 {
		t.Fatalf("got %v, want 2", got)
	}
}

func TestNumberOfKnownPeers(t *testing.T) {
	t.Parallel()
	d := New()
	now := float64(time.Now().Unix())
	old := now - 10000
	// peer announces: hash 1 (recent), hash 1 again (recent dup), hash 2 (old)
	d.PeerAnnounceReceived(Announce{Timestamp: now, SourceHash: []byte{1}}, false)
	d.PeerAnnounceReceived(Announce{Timestamp: now, SourceHash: []byte{1}}, false)
	d.PeerAnnounceReceived(Announce{Timestamp: old, SourceHash: []byte{2}}, false)
	// node announces also count: hash 3 recent
	d.NodeAnnounceReceived(Announce{Timestamp: now, SourceHash: []byte{3}}, false)

	// no lookback: all unique hashes -> 1, 2, 3 => 3
	if got := d.NumberOfKnownPeers(nil); got != 3 {
		t.Fatalf("no lookback: got %v, want 3", got)
	}

	// lookback of 100s: only recent entries (hash 1, hash 3) => 2
	lb := 100.0
	if got := d.NumberOfKnownPeers(&lb); got != 2 {
		t.Fatalf("lookback 100s: got %v, want 2", got)
	}
}

func TestSortRank(t *testing.T) {
	t.Parallel()
	d := New()
	rank := 5
	d.Remember(&Entry{SourceHash: []byte{1}, SortRank: &rank})
	if got := d.SortRank([]byte{1}); got == nil || *got != 5 {
		t.Fatalf("got %v, want 5", got)
	}
	if got := d.SortRank([]byte{2}); got != nil {
		t.Fatalf("unknown hash: got %v, want nil", got)
	}
	d2 := New()
	d2.Remember(&Entry{SourceHash: []byte{9}})
	if got := d2.SortRank([]byte{9}); got != nil {
		t.Fatalf("nil rank entry: got %v, want nil", got)
	}
}
