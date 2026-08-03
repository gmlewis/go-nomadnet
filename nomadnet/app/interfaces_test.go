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
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
)

// TestInterfaceStatsNoConfig verifies InterfaceStats returns nil when no RNS
// config is available yet — the state wireDisplays runs in, since initRNS
// (which creates the standalone config dir) runs asynchronously after Init.
func TestInterfaceStatsNoConfig(t *testing.T) {
	a := &App{}
	if got := a.InterfaceStats(); got != nil {
		t.Errorf("InterfaceStats() = %v, want nil when no config is available", got)
	}
}

// TestInterfaceStatsConfigOrderAndDisabled verifies InterfaceStats enumerates
// interfaces from the RNS config in file order, INCLUDING disabled-in-config
// interfaces (which RNS does not start and so are absent from the transport),
// and merges live Connected/TX/RX from the transport by name. This is Python's
// behavior (Interfaces.py:2840-2897: config is the list source, transport
// supplies stats).
func TestInterfaceStatsConfigOrderAndDisabled(t *testing.T) {
	// Config with three interfaces in order: disabled, enabled+running, enabled
	// but not running (no transport entry).
	dir := t.TempDir()
	config := `[reticulum]
share_instance = No

[interfaces]
  [[Michmesh Testnet]]
    type = TCPClientInterface
    interface_enabled = false
    target_host = RNS.MichMesh.net

  [[Beleth RNS Hub]]
    type = TCPClientInterface
    interface_enabled = true
    target_host = rns.beleth.net

  [[g00n Cloud Dallas]]
    type = TCPClientInterface
    enabled = Yes
    target_host = dfw.us.g00n.cloud
`
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	// Transport with only "Beleth RNS Hub" running.
	ts := rns.NewTransportSystem(nil)
	if err := ts.Start(filepath.Join(t.TempDir(), "rns-storage")); err != nil {
		t.Fatalf("TransportSystem.Start: %v", err)
	}
	defer ts.Stop()
	pipe := interfaces.NewPipeInterface("Beleth RNS Hub", func([]byte, interfaces.Interface) {})
	ts.RegisterInterface(pipe)

	a := &App{RNSConfigDir: dir, Transport: ts}
	stats := a.InterfaceStats()
	if len(stats) != 3 {
		t.Fatalf("InterfaceStats() returned %v entries, want 3: %+v", len(stats), stats)
	}

	// Index 0: disabled Michmesh — from config, no transport stats.
	if stats[0].Name != "Michmesh Testnet" {
		t.Errorf("stats[0].Name = %q, want %q", stats[0].Name, "Michmesh Testnet")
	}
	if stats[0].Enabled {
		t.Error("stats[0].Enabled = true, want false (interface_enabled = false)")
	}
	if stats[0].Connected {
		t.Error("stats[0].Connected = true, want false (not running)")
	}
	if stats[0].Type != "TCPClientInterface" {
		t.Errorf("stats[0].Type = %q, want %q", stats[0].Type, "TCPClientInterface")
	}

	// Index 1: Beleth — enabled (config) + connected (transport).
	if stats[1].Name != "Beleth RNS Hub" {
		t.Errorf("stats[1].Name = %q, want %q", stats[1].Name, "Beleth RNS Hub")
	}
	if !stats[1].Enabled {
		t.Error("stats[1].Enabled = false, want true")
	}
	if !stats[1].Connected {
		t.Error("stats[1].Connected = false, want true (running)")
	}
	if stats[1].Bitrate != 1000000 {
		t.Errorf("stats[1].Bitrate = %v, want 1000000", stats[1].Bitrate)
	}

	// Index 2: g00n — enabled (config, "enabled = Yes"), not running.
	if stats[2].Name != "g00n Cloud Dallas" {
		t.Errorf("stats[2].Name = %q, want %q", stats[2].Name, "g00n Cloud Dallas")
	}
	if !stats[2].Enabled {
		t.Error("stats[2].Enabled = false, want true (enabled = Yes)")
	}
	if stats[2].Connected {
		t.Error("stats[2].Connected = true, want false (not running)")
	}
}

// TestParseInterfaceConfigEnabledDefault verifies an interface with neither
// "enabled" nor "interface_enabled" set defaults to enabled (Python's rule:
// absence is not false).
func TestParseInterfaceConfigEnabledDefault(t *testing.T) {
	dir := t.TempDir()
	config := `[interfaces]
  [[Auto]]
    type = AutoInterface
`
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := parseInterfaceConfig(filepath.Join(dir, "config"))
	if len(entries) != 1 {
		t.Fatalf("got %v entries, want 1", len(entries))
	}
	if !entries[0].enabled {
		t.Error("default enabled = false, want true")
	}
	if entries[0].typeStr != "AutoInterface" {
		t.Errorf("type = %q, want AutoInterface", entries[0].typeStr)
	}
}

// TestParseInterfaceConfigNameProperty verifies the "name" property overrides
// the section key as the display name (Python interface_data.get("name", ...)).
func TestParseInterfaceConfigNameProperty(t *testing.T) {
	dir := t.TempDir()
	config := `[interfaces]
  [[if0]]
    type = TCPClientInterface
    name = My Custom Name
`
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := parseInterfaceConfig(filepath.Join(dir, "config"))
	if len(entries) != 1 {
		t.Fatalf("got %v entries, want 1", len(entries))
	}
	if entries[0].iface != "My Custom Name" {
		t.Errorf("display name = %q, want %q", entries[0].iface, "My Custom Name")
	}
	if entries[0].name != "if0" {
		t.Errorf("section key = %q, want %q", entries[0].name, "if0")
	}
}
