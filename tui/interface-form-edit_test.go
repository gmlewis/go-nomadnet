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

package tui

import (
	"testing"
)

func TestEditInterfaceFormDataPopulate(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)
	existingConfig := map[string]any{
		"type":              IfaceTCPServer,
		"interface_enabled": true,
		"listen_ip":         "0.0.0.0",
		"listen_port":       4242,
	}

	fd.PopulateFromConfig("myserver", existingConfig)

	if fd.Fields["name"].Value != "myserver" {
		t.Errorf("name = %q, want %q", fd.Fields["name"].Value, "myserver")
	}
	if fd.Fields["listen_ip"].Value != "0.0.0.0" {
		t.Errorf("listen_ip = %q, want %q", fd.Fields["listen_ip"].Value, "0.0.0.0")
	}
	if fd.Fields["listen_port"].Value != "4242" {
		t.Errorf("listen_port = %q, want %q", fd.Fields["listen_port"].Value, "4242")
	}
}

func TestEditInterfaceFormDataPopulateCheckbox(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)
	existingConfig := map[string]any{
		"type":         IfaceTCPServer,
		"i2p_tunneled": true,
		"prefer_ipv6":  false,
	}

	fd.PopulateFromConfig("srv", existingConfig)

	if fd.AdditionalFields["i2p_tunneled"].Value != "true" {
		t.Errorf("i2p_tunneled = %q, want %q", fd.AdditionalFields["i2p_tunneled"].Value, "true")
	}
}

func TestEditInterfaceFormDataPopulateCustomType(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceCustom)
	existingConfig := map[string]any{
		"type":  "MyCustomClass",
		"speed": "115200",
	}

	fd.PopulateFromConfig("cust", existingConfig)

	if fd.Fields["type"].Value != "MyCustomClass" {
		t.Errorf("type = %q, want %q", fd.Fields["type"].Value, "MyCustomClass")
	}
}

func TestEditInterfaceFormDataRename(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfacePipe)
	fd.PopulateFromConfig("oldname", map[string]any{
		"type":    IfacePipe,
		"command": "netcat -l 5757",
	})

	fd.Fields["name"].Value = "newname"

	config := fd.BuildConfig()
	if config["type"] != IfacePipe {
		t.Errorf("type = %q, want %q", config["type"], IfacePipe)
	}
	if config["command"] != "netcat -l 5757" {
		t.Errorf("command = %q, want %q", config["command"], "netcat -l 5757")
	}
}

func TestEditInterfaceFormDataValidateRenameDuplicate(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfacePipe)
	fd.PopulateFromConfig("oldname", map[string]any{
		"type":    IfacePipe,
		"command": "netcat -l 5757",
	})
	fd.Fields["name"].Value = "existing"

	existingNames := map[string]bool{"existing": true}
	errs := fd.ValidateAll(existingNames)
	found := false
	for _, e := range errs {
		if e.FieldKey == "name" {
			found = true
		}
	}
	if !found {
		t.Error("should detect duplicate name on rename")
	}
}

func TestEditInterfaceFormDataPopulateFrequency(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceRNode)
	existingConfig := map[string]any{
		"type":      IfaceRNode,
		"frequency": 868500000,
	}

	fd.PopulateFromConfig("rnode1", existingConfig)

	if fd.Fields["frequency"].Value != "868.5" {
		t.Errorf("frequency = %q, want %q", fd.Fields["frequency"].Value, "868.5")
	}
}

func TestEditInterfaceFormDataPopulateDropdown(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceRNode)
	existingConfig := map[string]any{
		"type":            IfaceRNode,
		"bandwidth":       "62500",
		"spreadingfactor": "10",
	}

	fd.PopulateFromConfig("rnode2", existingConfig)

	if fd.Fields["bandwidth"].Value != "62500" {
		t.Errorf("bandwidth = %q, want %q", fd.Fields["bandwidth"].Value, "62500")
	}
	if fd.Fields["spreadingfactor"].Value != "10" {
		t.Errorf("spreadingfactor = %q, want %q", fd.Fields["spreadingfactor"].Value, "10")
	}
}
