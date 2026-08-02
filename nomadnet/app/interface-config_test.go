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

	newProps := map[string]any{
		"type":              "TCPClientInterface",
		"target_host":       "127.0.0.1",
		"target_port":       "4242",
		"interface_enabled": true,
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

	updatedProps := map[string]any{
		"type":              "TCPClientInterface",
		"target_host":       "192.168.1.100",
		"target_port":       "4242",
		"interface_enabled": false,
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
