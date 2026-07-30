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

import "testing"

func TestRemoveAnnounceWithTimestampPerStream(t *testing.T) {
	t.Parallel()
	d := New()
	d.PeerAnnounceReceived(Announce{Timestamp: 10, SourceHash: []byte{1}, AnnounceType: "peer"}, false)
	d.PNAnnounceReceived(Announce{Timestamp: 20, SourceHash: []byte{2}, AnnounceType: "pn"}, false)

	d.RemoveAnnounceWithTimestamp(10)
	if len(d.PeerAnnounces()) != 0 {
		t.Fatalf("peer announce should be removed, got %d", len(d.PeerAnnounces()))
	}
	if len(d.PNAnnounces()) != 1 {
		t.Fatalf("pn announce should remain, got %d", len(d.PNAnnounces()))
	}

	d.RemoveAnnounceWithTimestamp(20)
	if len(d.PNAnnounces()) != 0 {
		t.Fatalf("pn announce should be removed, got %d", len(d.PNAnnounces()))
	}

	// no-op when timestamp absent
	d.RemoveAnnounceWithTimestamp(999)
}
