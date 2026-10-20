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
	"runtime"
	"testing"
)

// TestPortFieldFromBranches pins the three shapes Python's get_port_field()
// returns (Interfaces.py:142-181):
//
//   - no ports   → a text field with the "/dev/ttyXXX (or COM port on Windows)"
//     placeholder;
//   - one port   → a text field defaulted to that port's device;
//   - two or more → a dropdown listing the ports by description.
func TestPortFieldFromBranches(t *testing.T) {
	t.Parallel()

	one := []SerialPortInfo{{Device: "/dev/ttyUSB0", Description: "/dev/ttyUSB0", IsStandard: false}}
	two := []SerialPortInfo{
		{Device: "/dev/ttyUSB0", Description: "/dev/ttyUSB0 - USB Serial"},
		{Device: "/dev/ttyACM0", Description: "/dev/ttyACM0 - ACM Modem"},
	}

	tests := []struct {
		name        string
		ports       []SerialPortInfo
		wantType    string
		wantDefault string
		wantPlace   string
		wantOptions []string
	}{
		{
			name:        "no ports falls back to an edit field",
			ports:       nil,
			wantType:    "edit",
			wantDefault: "",
			wantPlace:   "/dev/ttyXXX (or COM port on Windows)",
		},
		{
			name:        "a single port defaults the edit field to its device",
			ports:       one,
			wantType:    "edit",
			wantDefault: "/dev/ttyUSB0",
			wantPlace:   "/dev/ttyXXX (or COM port on Windows)",
		},
		{
			name:        "several ports become a dropdown of descriptions",
			ports:       two,
			wantType:    "dropdown",
			wantDefault: two[0].Description,
			wantOptions: []string{two[0].Description, two[1].Description},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := PortFieldFrom(tc.ports)

			if got.ConfigKey != "port" || got.Label != "Port: " {
				t.Errorf("field is %q/%q, want %q/%q", got.ConfigKey, got.Label, "port", "Port: ")
			}
			if got.Type != tc.wantType {
				t.Errorf("type = %q, want %q", got.Type, tc.wantType)
			}
			if got.Default != tc.wantDefault {
				t.Errorf("default = %q, want %q", got.Default, tc.wantDefault)
			}
			if tc.wantPlace != "" && got.Placeholder != tc.wantPlace {
				t.Errorf("placeholder = %q, want %q", got.Placeholder, tc.wantPlace)
			}
			if len(tc.wantOptions) > 0 {
				if len(got.Options) != len(tc.wantOptions) {
					t.Fatalf("options = %v, want %v", got.Options, tc.wantOptions)
				}
				for i := range tc.wantOptions {
					if got.Options[i] != tc.wantOptions[i] {
						t.Errorf("options[%v] = %q, want %q", i, got.Options[i], tc.wantOptions[i])
					}
				}
			}
			if len(got.Validation) != 1 || got.Validation[0] != "required" {
				t.Errorf("validation = %v, want [required]", got.Validation)
			}
		})
	}
}

// TestPortFieldDropdownCarriesTheDeviceTransform pins the piece that makes the
// dropdown usable: Python's dropdown shows each port's description but its
// transform writes the DEVICE to the config (`lambda x: device_map[x]`,
// Interfaces.py:164). Without the transform on the Go field, picking
// "/dev/ttyUSB0 - USB Serial" would write that whole description into the
// configuration as the port.
func TestPortFieldDropdownCarriesTheDeviceTransform(t *testing.T) {
	t.Parallel()

	two := []SerialPortInfo{
		{Device: "/dev/ttyUSB0", Description: "/dev/ttyUSB0 - USB Serial"},
		{Device: "/dev/ttyACM0", Description: "/dev/ttyACM0 - ACM Modem"},
	}

	got := PortFieldFrom(two)
	if got.Type != "dropdown" {
		t.Fatalf("type = %q, want a dropdown", got.Type)
	}
	if got.Transform != transformPortDevice {
		t.Fatalf("transform = %q, want %q", got.Transform, transformPortDevice)
	}

	// Every option round-trips through the field's own map: the description shown
	// is keyed to the device written, and the device written selects the
	// description again on load.
	for _, p := range two {
		if dev, ok := got.ValueMap[p.Description]; !ok || dev != p.Device {
			t.Errorf("ValueMap[%q] = %q (present %v), want %q", p.Description, dev, ok, p.Device)
		}
		if idx := indexOfOption(got.Options, p.Description); idx < 0 {
			t.Errorf("option %q not present for the device %q", p.Description, p.Device)
		}
	}

	// The named transform stays as the declared conversion (and as the fallback a
	// field without a map would use). Against the LIVE port list it can only
	// resolve ports that are actually detected, so an unknown description is passed
	// through rather than dropped — the field's ValueMap is what makes the option
	// list authoritative.
	for _, p := range two {
		if dev, ok := portDeviceForDescriptionFrom(p.Description, two); !ok || dev != p.Device {
			t.Errorf("portDeviceForDescriptionFrom(%q) = %q (found %v), want %q", p.Description, dev, ok, p.Device)
		}
	}
	const unknown = "/dev/ttyNOTREAL - Not A Port"
	if got := applyFormTransform(transformPortDevice, unknown); got != unknown {
		t.Errorf("applyFormTransform(%q) = %v, want it passed through unchanged", unknown, got)
	}
}

// TestPortFieldNoSerialPlaceholder pins the pyserial-unavailable wording, which is
// what Python shows when it cannot detect ports at all (Interfaces.py:149):
// a placeholder naming the pyserial dependency rather than the port.
func TestPortFieldNoSerialPlaceholder(t *testing.T) {
	t.Parallel()

	got := PortFieldNoSerial()
	if got.Type != "edit" {
		t.Errorf("type = %q, want an edit field", got.Type)
	}
	const want = "/dev/ttyUSB0 or COM port (pyserial not installed)"
	if got.Placeholder != want {
		t.Errorf("placeholder = %q, want %q", got.Placeholder, want)
	}
}

// TestSerialInterfaceTypesUseThePortField pins the wiring: the five serial-capable
// interface types must carry the detector-backed port field, not the fixed
// "detection not available" one. Python gives each of them get_port_field()
// (Interfaces.py:677,766,833,892,996); with the stub, a detected-port dropdown
// can never appear.
func TestSerialInterfaceTypesUseThePortField(t *testing.T) {
	t.Parallel()

	types := []string{
		IfaceRNode, IfaceRNodeMulti, IfaceSerial, IfaceKISS, IfaceAX25KISS,
	}
	for _, ifType := range types {
		t.Run(ifType, func(t *testing.T) {
			t.Parallel()

			field, ok := portFieldOf(InterfaceFieldsFor(ifType))
			if !ok {
				t.Fatalf("%v has no port field", ifType)
			}
			if field.Placeholder == PortFieldNoSerial().Placeholder {
				t.Errorf("%v uses the detection-unavailable port field (placeholder %q)", ifType, field.Placeholder)
			}
			// Where detection is implemented the field is the detector's own
			// output; elsewhere the stub is the honest answer.
			if serialDetectionAvailable() {
				if want := PortField(); field.Type != want.Type || field.Placeholder != want.Placeholder {
					t.Errorf("%v port field = %q/%q, want the detector's %q/%q",
						ifType, field.Type, field.Placeholder, want.Type, want.Placeholder)
				}
			}
		})
	}

	// The platform guard itself: this build must report detection on darwin and
	// linux, and not elsewhere, which is what GetPortInfo assumes.
	switch runtime.GOOS {
	case "darwin", "linux":
		if !serialDetectionAvailable() {
			t.Errorf("serialDetectionAvailable() = false on %v", runtime.GOOS)
		}
	default:
		if serialDetectionAvailable() {
			t.Errorf("serialDetectionAvailable() = true on %v, where GetPortInfo cannot enumerate", runtime.GOOS)
		}
	}
}

// portFieldOf returns the config-keyed "port" field from a type's field groups.
func portFieldOf(groups []InterfaceFieldGroup) (InterfaceField, bool) {
	for _, g := range groups {
		for _, f := range append(append([]InterfaceField{}, g.Fields...), g.AdditionalOptions...) {
			if f.ConfigKey == "port" {
				return f, true
			}
		}
	}
	return InterfaceField{}, false
}
