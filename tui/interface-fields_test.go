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

func TestInterfaceFieldsRegistry(t *testing.T) {
	t.Parallel()

	for _, ifType := range allInterfaceTypes {
		fields := InterfaceFieldsFor(ifType)
		if fields == nil {
			t.Errorf("InterfaceFieldsFor(%q) = nil, want non-nil", ifType)
		}
	}
}

func TestInterfaceFieldsForUnknown(t *testing.T) {
	t.Parallel()

	fields := InterfaceFieldsFor("NonexistentInterface")
	if fields == nil {
		t.Error("InterfaceFieldsFor(unknown) = nil, should fall back to default")
	}
}

func TestInterfaceFieldTypes(t *testing.T) {
	t.Parallel()

	validTypes := map[string]bool{
		"edit":          true,
		"checkbox":      true,
		"dropdown":      true,
		"multilist":     true,
		"multitable":    true,
		"keyvaluepairs": true,
	}

	for _, ifType := range allInterfaceTypes {
		fields := InterfaceFieldsFor(ifType)
		for _, fg := range fields {
			for _, f := range fg.Fields {
				if !validTypes[f.Type] {
					t.Errorf("InterfaceFieldsFor(%q) has field %q with invalid type %q", ifType, f.ConfigKey, f.Type)
				}
			}
			for _, f := range fg.AdditionalOptions {
				if !validTypes[f.Type] {
					t.Errorf("InterfaceFieldsFor(%q) additional has field %q with invalid type %q", ifType, f.ConfigKey, f.Type)
				}
			}
		}
	}
}

func TestTCPServerInterfaceFields(t *testing.T) {
	t.Parallel()

	fields := InterfaceFieldsFor(IfaceTCPServer)
	if len(fields) == 0 {
		t.Fatal("TCPServerInterface has no field groups")
	}

	found := map[string]bool{}
	for _, fg := range fields {
		for _, f := range fg.Fields {
			found[f.ConfigKey] = true
		}
		for _, f := range fg.AdditionalOptions {
			found[f.ConfigKey] = true
		}
	}

	for _, key := range []string{"listen_ip", "listen_port"} {
		if !found[key] {
			t.Errorf("TCPServerInterface missing field %q", key)
		}
	}
}

func TestTCPClientInterfaceFields(t *testing.T) {
	t.Parallel()

	fields := InterfaceFieldsFor(IfaceTCPClient)
	if len(fields) == 0 {
		t.Fatal("TCPClientInterface has no field groups")
	}

	found := map[string]bool{}
	for _, fg := range fields {
		for _, f := range fg.Fields {
			found[f.ConfigKey] = true
		}
		for _, f := range fg.AdditionalOptions {
			found[f.ConfigKey] = true
		}
	}

	for _, key := range []string{"target_host", "target_port"} {
		if !found[key] {
			t.Errorf("TCPClientInterface missing field %q", key)
		}
	}
}

func TestInterfaceFieldValidationFlags(t *testing.T) {
	t.Parallel()

	fields := InterfaceFieldsFor(IfaceTCPServer)
	for _, fg := range fields {
		for _, f := range fg.Fields {
			for _, v := range f.Validation {
				if v != "required" && v != "number" {
					t.Errorf("unexpected validation flag %q on field %q", v, f.ConfigKey)
				}
			}
		}
	}
}

func TestInterfaceFieldDropdownOptions(t *testing.T) {
	t.Parallel()

	fields := InterfaceFieldsFor(IfaceAuto)
	for _, fg := range fields {
		allFields := append(fg.Fields, fg.AdditionalOptions...)
		for _, f := range allFields {
			if f.Type == "dropdown" && len(f.Options) == 0 {
				t.Errorf("dropdown field %q has no options", f.ConfigKey)
			}
		}
	}
}
