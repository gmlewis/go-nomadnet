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
	"reflect"
	"strings"
	"testing"
)

func TestAddInterfaceConfig(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	cfgPath := filepath.Join(dir, "config")

	initialConfig := `[interfaces]
  [[Default Interface]]
    type = AutoInterface
    enabled = Yes
`
	if err := os.WriteFile(cfgPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	app := &App{RNSConfigDir: dir}

	newProps := []ConfigProp{
		{Key: "type", Value: "TCPClientInterface"},
		{Key: "interface_enabled", Value: true},
		{Key: "target_host", Value: "127.0.0.1"},
		{Key: "target_port", Value: "4242"},
	}

	if err := app.AddInterfaceConfig("My TCP", newProps); err != nil {
		t.Fatalf("AddInterfaceConfig failed: %v", err)
	}

	stats := app.InterfaceStats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 interface stats, got %v", len(stats))
	}

	if stats[1].Name != "My TCP" || stats[1].Type != "TCPClientInterface" || !stats[1].Enabled {
		t.Errorf("unexpected stat for My TCP: %+v", stats[1])
	}
}

func TestEditInterfaceConfig(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	cfgPath := filepath.Join(dir, "config")

	initialConfig := `[interfaces]
  [[My TCP]]
    type = TCPClientInterface
    target_host = 127.0.0.1
    target_port = 4242
    interface_enabled = True
`
	if err := os.WriteFile(cfgPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	app := &App{RNSConfigDir: dir}

	updatedProps := []ConfigProp{
		{Key: "type", Value: "TCPClientInterface"},
		{Key: "interface_enabled", Value: false},
		{Key: "target_host", Value: "192.168.1.100"},
		{Key: "target_port", Value: "4242"},
	}

	if err := app.EditInterfaceConfig("My TCP", "My TCP", updatedProps); err != nil {
		t.Fatalf("EditInterfaceConfig failed: %v", err)
	}

	stats := app.InterfaceStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 interface stat, got %v", len(stats))
	}

	if stats[0].Name != "My TCP" || stats[0].Enabled != false {
		t.Errorf("unexpected stat after edit: %+v", stats[0])
	}
}

func TestRemoveInterfaceConfig(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	cfgPath := filepath.Join(dir, "config")

	initialConfig := `[interfaces]
  [[Default Interface]]
    type = AutoInterface
    enabled = Yes
  [[My TCP]]
    type = TCPClientInterface
    target_host = 127.0.0.1
    target_port = 4242
`
	if err := os.WriteFile(cfgPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	app := &App{RNSConfigDir: dir}

	if err := app.RemoveInterfaceConfig("My TCP"); err != nil {
		t.Fatalf("RemoveInterfaceConfig failed: %v", err)
	}

	stats := app.InterfaceStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 interface stat after remove, got %v", len(stats))
	}
	if stats[0].Name != "Default Interface" {
		t.Errorf("unexpected remaining interface: %v", stats[0].Name)
	}
}

func TestGetInterfaceConfigMap(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	cfgPath := filepath.Join(dir, "config")

	initialConfig := `[interfaces]
  [[My TCP]]
    type = TCPClientInterface
    target_host = 127.0.0.1
    target_port = 4242
    interface_enabled = True
`
	if err := os.WriteFile(cfgPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	app := &App{RNSConfigDir: dir}

	cfgMap, err := app.GetInterfaceConfigMap("My TCP")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap failed: %v", err)
	}

	want := map[string]any{
		"type":              "TCPClientInterface",
		"target_host":       "127.0.0.1",
		"target_port":       "4242",
		"interface_enabled": "True",
	}

	if !reflect.DeepEqual(cfgMap, want) {
		t.Errorf("GetInterfaceConfigMap = %v, want %v", cfgMap, want)
	}
}

// TestInterfaceConfigNestedSubsections pins both directions of an [[[sub]]]
// subsection: reading one back as a nested map (what RNodeMultiInterface's
// sub-interface settings are stored as, ConfigObj's dict value) and writing one
// out again as a [[[sub]]] section rather than as a Go map printed into a
// single line.
func TestInterfaceConfigNestedSubsections(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	cfgPath := filepath.Join(dir, "config")

	initialConfig := `[interfaces]
  [[multi]]
    type = RNodeMultiInterface
    interface_enabled = True
    port = /dev/ttyACM0
    [[[sub1]]]
      frequency = 868500000
      bandwidth = 62500
    [[[sub2]]]
      frequency = 915000000
      bandwidth = 125000
`
	if err := os.WriteFile(cfgPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	a := &App{RNSConfigDir: dir}

	got, err := a.GetInterfaceConfigMap("multi")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap failed: %v", err)
	}
	if got["port"] != "/dev/ttyACM0" {
		t.Errorf("port = %v, want /dev/ttyACM0", got["port"])
	}
	sub1, ok := got["sub1"].(map[string]any)
	if !ok {
		t.Fatalf("sub1 = %#v, want a nested map", got["sub1"])
	}
	if sub1["frequency"] != "868500000" {
		t.Errorf("sub1 frequency = %v, want 868500000", sub1["frequency"])
	}
	if sub2, ok := got["sub2"].(map[string]any); !ok || sub2["bandwidth"] != "125000" {
		t.Errorf("sub2 = %#v, want a nested map with bandwidth 125000", got["sub2"])
	}

	// Writing the same shape back must produce a [[[sub]]] section that reads
	// the same way.
	props := []ConfigProp{
		{Key: "type", Value: "RNodeMultiInterface"},
		{Key: "interface_enabled", Value: true},
		{Key: "port", Value: "/dev/ttyACM0"},
		{Key: "sub1", Value: map[string]any{"frequency": "868500000", "bandwidth": "62500"}},
	}
	if err := a.EditInterfaceConfig("multi", "multi2", props); err != nil {
		t.Fatalf("EditInterfaceConfig failed: %v", err)
	}

	rewritten, err := a.GetInterfaceConfigMap("multi2")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(multi2) failed: %v", err)
	}
	sub, ok := rewritten["sub1"].(map[string]any)
	if !ok {
		t.Fatalf("rewritten sub1 = %#v, want a nested map", rewritten["sub1"])
	}
	if sub["frequency"] != "868500000" || sub["bandwidth"] != "62500" {
		t.Errorf("rewritten sub1 = %v, want frequency 868500000 and bandwidth 62500", sub)
	}

	// The written file must spell the subsection the way a config file does, not
	// as a Go map printed on one line.
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(raw), "[[[sub1]]]") {
		t.Errorf("the written config has no [[[sub1]]] subsection:\n%s", raw)
	}
	if strings.Contains(string(raw), "map[") {
		t.Errorf("the written config printed a Go map:\n%s", raw)
	}
}
