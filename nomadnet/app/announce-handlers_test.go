// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package app

import (
	"sort"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

// wantAnnounceFilters is the set of announce filters the Python original
// registers: nomadnet.Conversation (aspect_filter = "lxmf.delivery") and
// nomadnet.Directory (aspect_filter = "lxmf.propagation" and
// "nomadnetwork.node"), registered from NomadNetworkApp.py:410-411. Python
// registers no RRC announce handler at all, so "rrc.hub" is deliberately absent
// here too. RNS matches an announce by recomputing the destination hash from the
// app name and aspects, so a misspelled filter never fires — it is silent, not
// loud, which is why this set is pinned.
var wantAnnounceFilters = []string{
	"lxmf.delivery",
	"lxmf.propagation",
	"nomadnetwork.node",
}

// registeredAnnounceFilters returns the sorted, de-duplicated filters the
// transport currently holds. Multiplicity is not asserted: the LXMF router
// registers its own delivery and propagation handlers when it is created
// (lxmf/announce-handler.go:17-22, called from lxmf/router.go:446), so those two
// filters legitimately appear twice — once for the router's peer table and once
// for the app's UI bookkeeping.
func registeredAnnounceFilters(ts *rns.TransportSystem) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(wantAnnounceFilters))
	for _, handler := range ts.AnnounceHandlers() {
		if handler == nil || seen[handler.AspectFilter] {
			continue
		}
		seen[handler.AspectFilter] = true
		out = append(out, handler.AspectFilter)
	}
	sort.Strings(out)
	return out
}

// assertAnnounceFilters asserts the transport's filter set is exactly the Python
// set, naming every difference so a stray or misspelled filter is obvious.
func assertAnnounceFilters(t *testing.T, ts *rns.TransportSystem) {
	t.Helper()

	got := registeredAnnounceFilters(ts)
	want := append([]string(nil), wantAnnounceFilters...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("announce filters = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("announce filters = %v, want %v", got, want)
		}
	}
}

// TestAnnounceHandlerFiltersMatchPython asserts the test harness registers
// exactly the announce filters production registers, which are exactly the ones
// Python registers. This is the regression test for a stray "rrc.chat" handler:
// no destination is ever announced under that name (the real RRC name is
// "rrc.hub"), so the handler could never run, and wiring it up instead would have
// added every announced RRC hub to the hub list, which Python never does.
func TestAnnounceHandlerFiltersMatchPython(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport error: %v", err)
	}
	defer a.Shutdown()

	assertAnnounceFilters(t, ts)

	// The RRC manager is still wired to the transport, so hubs come from the
	// config and the hub manager — just not from announces.
	if a.RRC == nil {
		t.Error("RRC manager should still be created by InitWithTransport")
	}
}

// TestPythonAnnounceFilterNamesAreRealDestinations asserts every filter the port
// registers names a destination the client actually builds, so a typo cannot
// hide in the list, and that no RRC destination is among them.
func TestPythonAnnounceFilterNamesAreRealDestinations(t *testing.T) {
	t.Parallel()

	// Destination names this client builds: lxmf.delivery and lxmf.propagation
	// (the LXMF router) and nomadnetwork.node (the node server). RRC hubs are
	// addressed directly, so "rrc.hub" (rrc.HubDestName) is a real destination
	// name but is not an announce filter the Python client subscribes to.
	realNames := map[string]bool{
		"lxmf.delivery":     true,
		"lxmf.propagation":  true,
		"nomadnetwork.node": true,
		"rrc.hub":           true,
	}
	for _, filter := range wantAnnounceFilters {
		if !realNames[filter] {
			t.Errorf("announce filter %q is not a destination name this client builds", filter)
		}
		if filter == "rrc.hub" {
			t.Error(`"rrc.hub" must not be an announce filter: Python registers no RRC announce handler`)
		}
	}
	if realNames["rrc.chat"] {
		t.Error(`"rrc.chat" is not a real RRC destination name; the real one is "rrc.hub"`)
	}
}
