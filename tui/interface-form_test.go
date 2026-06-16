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

func TestNewInterfaceFormData(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)

	if fd.IfaceType != IfaceTCPServer {
		t.Errorf("IfaceType = %q, want %q", fd.IfaceType, IfaceTCPServer)
	}
	if len(fd.Fields) == 0 {
		t.Error("Fields is empty for TCPServerInterface")
	}
	if _, ok := fd.Fields["name"]; !ok {
		t.Error("Fields missing required 'name' field")
	}
	if _, ok := fd.Fields["listen_ip"]; !ok {
		t.Error("Fields missing 'listen_ip' for TCPServerInterface")
	}
}

func TestInterfaceFormDataBuildsConfig(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)
	fd.Fields["name"].Value = "myserver"
	fd.Fields["listen_ip"].Value = "0.0.0.0"
	fd.Fields["listen_port"].Value = "4242"

	config := fd.BuildConfig()

	if config["type"] != IfaceTCPServer {
		t.Errorf("config type = %q, want %q", config["type"], IfaceTCPServer)
	}
	if config["interface_enabled"] != true {
		t.Error("interface_enabled should be true")
	}
	if config["listen_ip"] != "0.0.0.0" {
		t.Errorf("listen_ip = %q, want %q", config["listen_ip"], "0.0.0.0")
	}
	if config["listen_port"] != "4242" {
		t.Errorf("listen_port = %q, want %q", config["listen_port"], "4242")
	}
}

func TestInterfaceFormDataValidateRequired(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)
	fd.Fields["name"].Value = ""

	errs := fd.ValidateAll()
	if len(errs) == 0 {
		t.Error("ValidateAll should report error for empty required name")
	}

	fd.Fields["name"].Value = "myiface"
	errs = fd.ValidateAll()
	for _, e := range errs {
		if e.FieldKey == "name" {
			t.Errorf("name field should not have error after being set: %v", e)
		}
	}
}

func TestInterfaceFormDataDuplicateName(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)
	fd.Fields["name"].Value = "existing"

	existingNames := map[string]bool{"existing": true}
	errs := fd.ValidateAll(existingNames)
	found := false
	for _, e := range errs {
		if e.FieldKey == "name" && e.Message != "" {
			found = true
		}
	}
	if !found {
		t.Error("ValidateAll should report error for duplicate name")
	}
}

func TestInterfaceFormDataExcludesEmptyValues(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfacePipe)
	fd.Fields["name"].Value = "mypipe"
	fd.Fields["command"].Value = "netcat -l 5757"
	fd.Fields["respawn_delay"].Value = ""

	config := fd.BuildConfig()

	if _, ok := config["respawn_delay"]; ok {
		t.Error("BuildConfig should exclude empty values")
	}
}

func TestInterfaceFormDataAdditionalFields(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)
	_, hasI2P := fd.AdditionalFields["i2p_tunneled"]
	if !hasI2P {
		t.Error("TCPServerInterface should have i2p_tunneled additional field")
	}
}

func TestInterfaceFormDataCommonFields(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)
	_, hasNetworkName := fd.CommonFields["network_name"]
	if !hasNetworkName {
		t.Error("should have network_name common field")
	}
}

func TestInterfaceFormDataFieldLabel(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceTCPServer)
	if fd.Fields["listen_ip"].Label == "" {
		t.Error("listen_ip field should have a label")
	}
}

func TestInterfaceFormDataBuildConfigCustom(t *testing.T) {
	t.Parallel()

	fd := NewInterfaceFormData(IfaceCustom)
	fd.Fields["name"].Value = "mycustom"
	fd.Fields["type"].Value = "MyCustomClass"

	config := fd.BuildConfig()
	if config["type"] != "MyCustomClass" {
		t.Errorf("CustomInterface type = %q, want %q", config["type"], "MyCustomClass")
	}
}
