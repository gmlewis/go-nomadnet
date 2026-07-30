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

	"github.com/gmlewis/go-reticulum/rns"
)

func TestIsKnown(t *testing.T) {
	t.Parallel()
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	ts.Remember(nil, dest.Hash, id.GetPublicKey(), nil)

	d := New()
	d.Transport = ts
	if !d.IsKnown(dest.Hash) {
		t.Error("known destination should be reported as known")
	}
	if d.IsKnown([]byte{0xff, 0xff, 0xff, 0xff}) {
		t.Error("unknown destination should not be known")
	}
}

func TestPNTrustLevel(t *testing.T) {
	t.Parallel()
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := id.GetPublicKey()

	nodeHash := rns.CalculateHash(id, "nomadnetwork", "node")
	pnHash := rns.CalculateHash(id, "lxmf", "propagation")
	ts.Remember(nil, pnHash, pub, nil)

	d := New()
	d.Transport = ts
	d.Remember(&Entry{SourceHash: nodeHash, DisplayName: "Node", TrustLevel: TrustTrusted})

	level, ok := d.PNTrustLevel(pnHash)
	if !ok {
		t.Fatal("PNTrustLevel should succeed when identity is recalled")
	}
	if level != TrustTrusted {
		t.Errorf("PNTrustLevel = %v, want trusted", level)
	}

	// Unknown PN source -> ok=false
	if _, ok := d.PNTrustLevel([]byte{0, 0, 0, 0}); ok {
		t.Error("PNTrustLevel for unrecalled identity should return ok=false")
	}
}
