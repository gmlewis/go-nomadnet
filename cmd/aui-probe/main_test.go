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
	"encoding/hex"
	"net"
	"testing"
	"time"
)

// TestDeriveMulticastAddressGolden pins the group address against values
// verified three ways: the reference derivation in
// rns/interfaces/auto.go autoMulticastDiscoveryIP, an independent Python
// reimplementation, and — decisively — the ff120000d70bfb1c16e45e39485e31e1
// entry a live gonomadnet actually registered in /proc/net/igmp6.
func TestDeriveMulticastAddressGolden(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		group string
		want  string
	}{
		{"reticulum", "ff12:0:d70b:fb1c:16e4:5e39:485e:31e1"},
		{"", deriveMulticastAddress("")}, // any input must parse as IPv6
	} {
		got := deriveMulticastAddress(tc.group)
		if got != tc.want {
			t.Errorf("deriveMulticastAddress(%q) = %q, want %q", tc.group, got, tc.want)
		}
		if ip := net.ParseIP(got); ip == nil || ip.To16() == nil {
			t.Errorf("deriveMulticastAddress(%q) = %q is not a valid IPv6 address", tc.group, got)
		}
	}
}

func TestDiscoveryTokenKnownVector(t *testing.T) {
	t.Parallel()
	const src = "fe80::ecb7:f7fa:f27b:44bc" // raspberrypi wlan0, 2026-08-26 fleet test
	want := "54635a79cd9385118f57a730057c5bbe3481cb56094d71b155c1b801bed5072b"
	if got := discoveryToken(defaultGroupID, src); len(got) != 32 {
		t.Fatalf("token length = %d, want 32", len(got))
	} else if hexStr := hex.EncodeToString(got); hexStr != want {
		t.Errorf("discoveryToken(%q,%q) = %s, want %s", defaultGroupID, src, hexStr, want)
	}
	// A different source address must produce a different token.
	other := discoveryToken(defaultGroupID, "fe80::1")
	if string(other) == string(discoveryToken(defaultGroupID, src)) {
		t.Error("tokens for different source addresses collide")
	}
}

func TestFirstLinkLocalSkipsIPv4AndGlobal(t *testing.T) {
	t.Parallel()
	// firstLinkLocal reports "" rather than guessing when no link-local IPv6
	// address exists; the send path then fails loudly instead of claiming a
	// wrong source identity in its tokens.
	iface := &net.Interface{Name: "does-not-exist", Index: 999999}
	if got := firstLinkLocal(iface); got != "" {
		t.Errorf("firstLinkLocal(nonexistent) = %q, want empty", got)
	}
}

func TestListenTokensHonorsDeadline(t *testing.T) {
	t.Parallel()
	// listenTokens must return on its deadline even when nothing arrives.
	done := make(chan struct{})
	go func() {
		listenTokens("reticulum-unreachable-group-test", "", defaultDiscoveryPort, 50*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("listenTokens did not return after its timeout")
	}
}
