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
	"os"
	"testing"
)

func TestLoadFromDiskPythonWritten(t *testing.T) {
	path := "testdata/py-directory.msgpack"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("committed fixture missing")
	}
	d := New()
	if err := d.LoadFromDisk(path); err != nil {
		t.Fatal(err)
	}
	e := d.Find([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
	if e == nil {
		t.Fatal("Bob entry not found")
	}
	if e.DisplayName != "Bob" {
		t.Errorf("DisplayName = %q, want Bob", e.DisplayName)
	}
	if e.TrustLevel != TrustUntrusted {
		t.Errorf("TrustLevel = %v, want untrusted", e.TrustLevel)
	}
	if !e.HostsNode {
		t.Error("HostsNode should be true")
	}
	if e.PreferredDelivery != DeliveryPropagated {
		t.Errorf("PreferredDelivery = %v, want propagated", e.PreferredDelivery)
	}
	if !e.IdentifyOnConnect {
		t.Error("IdentifyOnConnect should be true")
	}
	if e.SortRank == nil || *e.SortRank != 3 {
		t.Errorf("SortRank = %v, want 3", e.SortRank)
	}
	if e.Notes != "bob notes" {
		t.Errorf("Notes = %q, want 'bob notes'", e.Notes)
	}
	e2 := d.Find([]byte{0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11})
	if e2 == nil {
		t.Fatal("second entry not found")
	}
	if e2.DisplayName != "Undefined" {
		t.Errorf("DisplayName = %q, want Undefined", e2.DisplayName)
	}
	if e2.PreferredDelivery != DeliveryDirect {
		t.Errorf("nil PreferredDelivery should default to direct, got %v", e2.PreferredDelivery)
	}
	if len(d.PeerAnnounces()) != 1 || len(d.NodeAnnounces()) != 1 || len(d.PNAnnounces()) != 1 {
		t.Fatalf("announce counts wrong: peer=%d node=%d pn=%d", len(d.PeerAnnounces()), len(d.NodeAnnounces()), len(d.PNAnnounces()))
	}
}
