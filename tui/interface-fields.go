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

// InterfaceField describes a single form field for interface configuration.
// Matches Python's INTERFACE_FIELDS field definitions at Interfaces.py:406.
type InterfaceField struct {
	ConfigKey   string              `json:"config_key"`
	Type        string              `json:"type"`
	Label       string              `json:"label"`
	Default     string              `json:"default"`
	Placeholder string              `json:"placeholder"`
	Options     []string            `json:"options,omitempty"`
	Validation  []string            `json:"validation,omitempty"`
	FieldValues []string            `json:"field_values,omitempty"`
	SubFields   map[string]SubField `json:"sub_fields,omitempty"`
}

// SubField describes a nested field for multitable interface types
// (e.g. RNodeMultiInterface subinterfaces).
type SubField struct {
	Label      string   `json:"label"`
	Type       string   `json:"type"`
	Options    []string `json:"options,omitempty"`
	Validation []string `json:"validation,omitempty"`
}

// InterfaceFieldGroup groups primary fields and additional options.
type InterfaceFieldGroup struct {
	Fields            []InterfaceField `json:"fields"`
	AdditionalOptions []InterfaceField `json:"additional_options,omitempty"`
}

// InterfaceFieldsFor returns the field groups for the given interface type.
// Falls back to the "default" (empty) group if the type is unknown.
// Matches Python's INTERFACE_FIELDS at Interfaces.py:406.
func InterfaceFieldsFor(ifType string) []InterfaceFieldGroup {
	if groups, ok := interfaceFieldRegistry[ifType]; ok {
		return groups
	}
	return interfaceFieldRegistry["default"]
}

// PortField returns the serial port field definition. When serial port
// detection is available and multiple ports exist, it returns a dropdown;
// otherwise it returns a text edit field.
// Matches Python's get_port_field() at Interfaces.py:142.
func PortField() InterfaceField {
	ports := GetPortInfo()
	if len(ports) > 1 {
		options := make([]string, len(ports))
		for i, p := range ports {
			options[i] = p.Description
		}
		defVal := ""
		if len(options) > 0 {
			defVal = options[0]
		}
		return InterfaceField{
			ConfigKey:  "port",
			Type:       "dropdown",
			Label:      "Port: ",
			Options:    options,
			Default:    defVal,
			Validation: []string{"required"},
		}
	}

	defVal := ""
	placeholder := "/dev/ttyXXX (or COM port on Windows)"
	if len(ports) == 1 {
		defVal = ports[0].Device
	}

	return InterfaceField{
		ConfigKey:   "port",
		Type:        "edit",
		Label:       "Port: ",
		Default:     defVal,
		Placeholder: placeholder,
		Validation:  []string{"required"},
	}
}

// PortFieldNoSerial returns a port field when pyserial-equivalent
// detection is not available. Matches Python's get_port_field() when
// PYSERIAL_AVAILABLE is False.
func PortFieldNoSerial() InterfaceField {
	return InterfaceField{
		ConfigKey:   "port",
		Type:        "edit",
		Label:       "Port: ",
		Default:     "",
		Placeholder: "/dev/ttyUSB0 or COM port (serial detection not available)",
		Validation:  []string{"required"},
	}
}

// interfaceFieldRegistry maps interface type names to their field groups.
// Matches Python's INTERFACE_FIELDS at Interfaces.py:406.
var interfaceFieldRegistry = map[string][]InterfaceFieldGroup{
	IfaceBackbone: {
		{
			Fields: []InterfaceField{
				{ConfigKey: "listen_on", Type: "edit", Label: "Listen On: ", Placeholder: "e.g., 0.0.0.0"},
				{ConfigKey: "port", Type: "edit", Label: "Port: ", Placeholder: "e.g., 4242", Validation: []string{"number"}},
				{ConfigKey: "device", Type: "edit", Label: "Device: ", Placeholder: "e.g., eth0"},
				{ConfigKey: "remote", Type: "edit", Label: "Remote: ", Placeholder: "e.g., a remote TCPServerInterface location"},
				{ConfigKey: "target_host", Type: "edit", Label: "Target Host: ", Placeholder: "e.g., 201:5d78:af73:5caf:a4de:a79f:3278:71e5"},
				{ConfigKey: "port", Type: "edit", Label: "Target Port: ", Placeholder: "e.g., 4242", Validation: []string{"number"}},
				{ConfigKey: "prefer_ipv6", Type: "checkbox", Label: ""},
			},
		},
	},
	IfaceAuto: {
		{},
		{
			AdditionalOptions: []InterfaceField{
				{ConfigKey: "devices", Type: "multilist", Label: "Devices: "},
				{ConfigKey: "ignored_devices", Type: "multilist", Label: "Ignored Devices: "},
				{ConfigKey: "group_id", Type: "edit", Label: "Group ID: ", Placeholder: "e.g., my_custom_network"},
				{ConfigKey: "discovery_scope", Type: "dropdown", Label: "Discovery Scope: ", Options: []string{"None", "link", "admin", "site", "organisation", "global"}, Default: "None"},
			},
		},
	},
	IfaceI2P: {
		{
			Fields: []InterfaceField{
				{ConfigKey: "peers", Type: "multilist", Label: "Peers: ", Validation: []string{"required"}},
			},
		},
	},
	IfaceTCPServer: {
		{
			Fields: []InterfaceField{
				{ConfigKey: "listen_ip", Type: "edit", Label: "Listen IP: ", Placeholder: "e.g., 0.0.0.0", Validation: []string{"required"}},
				{ConfigKey: "listen_port", Type: "edit", Label: "Listen Port: ", Placeholder: "e.g., 4242", Validation: []string{"number"}},
			},
			AdditionalOptions: []InterfaceField{
				{ConfigKey: "prefer_ipv6", Type: "checkbox", Label: "Prefer IPv6?"},
				{ConfigKey: "i2p_tunneled", Type: "checkbox", Label: "I2P Tunneled?"},
				{ConfigKey: "device", Type: "edit", Label: "Device: ", Placeholder: "A specific network device to listen on - e.g. eth0"},
				{ConfigKey: "port", Type: "edit", Label: "Port: ", Placeholder: "e.g., 4242", Validation: []string{"number"}},
			},
		},
	},
	IfaceTCPClient: {
		{
			Fields: []InterfaceField{
				{ConfigKey: "target_host", Type: "edit", Label: "Target Host: ", Placeholder: "e.g., 127.0.0.1", Validation: []string{"required"}},
				{ConfigKey: "target_port", Type: "edit", Label: "Target Port: ", Placeholder: "e.g., 8080", Validation: []string{"required", "number"}},
			},
			AdditionalOptions: []InterfaceField{
				{ConfigKey: "i2p_tunneled", Type: "checkbox", Label: "I2P Tunneled?"},
				{ConfigKey: "kiss_framing", Type: "checkbox", Label: "KISS Framing?"},
			},
		},
	},
	IfaceUDP: {
		{
			Fields: []InterfaceField{
				{ConfigKey: "listen_ip", Type: "edit", Label: "Listen IP: ", Placeholder: "e.g., 0.0.0.0", Validation: []string{"required"}},
				{ConfigKey: "listen_port", Type: "edit", Label: "Listen Port: ", Placeholder: "e.g., 4242", Validation: []string{"number"}},
				{ConfigKey: "forward_ip", Type: "edit", Label: "Forward IP: ", Placeholder: "e.g., 255.255.255.255", Validation: []string{"required"}},
				{ConfigKey: "forward_port", Type: "edit", Label: "Forward Port: ", Placeholder: "e.g., 4242", Validation: []string{"required", "number"}},
			},
			AdditionalOptions: []InterfaceField{
				{ConfigKey: "device", Type: "edit", Label: "Device: ", Placeholder: "A specific network device to listen on - e.g. eth0"},
				{ConfigKey: "port", Type: "edit", Label: "Port: ", Placeholder: "e.g., 4242", Validation: []string{"number"}},
			},
		},
	},
	IfaceRNode: {
		{
			Fields: []InterfaceField{
				PortFieldNoSerial(),
				{ConfigKey: "frequency", Type: "edit", Label: "Frequency (MHz): ", Placeholder: "868.5", Validation: []string{"required", "float"}},
				{ConfigKey: "txpower", Type: "edit", Label: "Transmit Power (dBm): ", Placeholder: "17", Validation: []string{"required", "number"}},
				{ConfigKey: "bandwidth", Type: "dropdown", Label: "Bandwidth (Hz): ", Options: []string{"7800", "10400", "15600", "20800", "31250", "41700", "62500", "125000", "250000", "500000", "1625000"}, Default: "7800", Validation: []string{"required"}},
				{ConfigKey: "spreadingfactor", Type: "dropdown", Label: "Spreading Factor: ", Options: []string{"7", "8", "9", "10", "11", "12"}, Default: "7", Validation: []string{"required"}},
				{ConfigKey: "codingrate", Type: "dropdown", Label: "Coding Rate: ", Options: []string{"4:5", "4:6", "4:7", "4:8"}, Default: "4:5", Validation: []string{"required"}},
			},
			AdditionalOptions: []InterfaceField{
				{ConfigKey: "id_callsign", Type: "edit", Label: "Callsign: ", Placeholder: "e.g. MYCALL-0"},
				{ConfigKey: "id_interval", Type: "edit", Label: "ID Interval (Seconds): ", Placeholder: "e.g. 600", Validation: []string{"number"}},
				{ConfigKey: "airtime_limit_long", Type: "edit", Label: "Airtime Limit Long (Seconds):  ", Placeholder: "e.g. 1.5", Validation: []string{"number"}},
				{ConfigKey: "airtime_limit_short", Type: "edit", Label: "Airtime Limit Short (Seconds):  ", Placeholder: "e.g. 33", Validation: []string{"number"}},
			},
		},
	},
	IfaceRNodeMulti: {
		{
			Fields: []InterfaceField{
				PortFieldNoSerial(),
				{
					ConfigKey:  "subinterfaces",
					Type:       "multitable",
					Validation: []string{"required"},
					SubFields: map[string]SubField{
						"frequency":       {Label: "Freq (Hz)", Type: "edit", Validation: []string{"required", "float"}},
						"bandwidth":       {Label: "BW (Hz)", Type: "edit", Options: []string{"7800", "10400", "15600", "20800", "31250", "41700", "62500", "125000", "250000", "500000", "1625000"}},
						"txpower":         {Label: "TX (dBm)", Type: "edit", Validation: []string{"required", "number"}},
						"vport":           {Label: "V.Port", Type: "edit", Validation: []string{"required", "number"}},
						"spreadingfactor": {Label: "SF", Type: "edit"},
						"codingrate":      {Label: "CR", Type: "edit"},
					},
				},
			},
			AdditionalOptions: []InterfaceField{
				{ConfigKey: "id_callsign", Type: "edit", Label: "Callsign: ", Placeholder: "e.g. MYCALL-0"},
				{ConfigKey: "id_interval", Type: "edit", Label: "ID Interval (Seconds): ", Placeholder: "e.g. 600", Validation: []string{"number"}},
			},
		},
	},
	IfaceSerial: {
		{
			Fields: []InterfaceField{
				PortFieldNoSerial(),
				{ConfigKey: "speed", Type: "edit", Label: "Speed (bps): ", Placeholder: "e.g. 115200", Validation: []string{"required", "number"}},
				{ConfigKey: "databits", Type: "edit", Label: "Databits: ", Placeholder: "e.g. 8", Validation: []string{"required", "number"}},
				{ConfigKey: "parity", Type: "edit", Label: "Parity: ", Validation: []string{"number"}},
				{ConfigKey: "stopbits", Type: "edit", Label: "Stopbits: ", Placeholder: "e.g. 1", Validation: []string{"number"}},
			},
		},
	},
	IfacePipe: {
		{
			Fields: []InterfaceField{
				{ConfigKey: "command", Type: "edit", Label: "Command: ", Placeholder: "e.g. netcat -l 5757", Validation: []string{"required"}},
				{ConfigKey: "respawn_delay", Type: "edit", Label: "Respawn Delay (seconds):  ", Placeholder: "e.g. 5", Validation: []string{"number"}},
			},
		},
	},
	IfaceKISS: {
		{
			Fields: []InterfaceField{
				PortFieldNoSerial(),
				{ConfigKey: "speed", Type: "edit", Label: "Speed (bps): ", Placeholder: "e.g. 115200", Validation: []string{"required", "number"}},
				{ConfigKey: "databits", Type: "edit", Label: "Databits: ", Placeholder: "e.g. 8", Validation: []string{"required", "number"}},
				{ConfigKey: "parity", Type: "edit", Label: "Parity: ", Validation: []string{"number"}},
				{ConfigKey: "stopbits", Type: "edit", Label: "Stopbits: ", Placeholder: "e.g. 1", Validation: []string{"number"}},
				{ConfigKey: "preamble", Type: "edit", Label: "Preamble (miliseconds): ", Placeholder: "e.g. 150", Validation: []string{"required", "number"}},
				{ConfigKey: "txtail", Type: "edit", Label: "TX Tail (miliseconds): ", Placeholder: "e.g. 10", Validation: []string{"required", "number"}},
				{ConfigKey: "slottime", Type: "edit", Label: "slottime (miliseconds): ", Placeholder: "e.g. 20", Validation: []string{"required", "number"}},
				{ConfigKey: "persistence", Type: "edit", Label: "Persistence (miliseconds): ", Placeholder: "e.g. 200", Validation: []string{"required", "number"}},
			},
			AdditionalOptions: []InterfaceField{
				{ConfigKey: "id_callsign", Type: "edit", Label: "ID Callsign: ", Placeholder: "e.g. MYCALL-0"},
				{ConfigKey: "id_interval", Type: "edit", Label: "ID Interval (Seconds): ", Placeholder: "e.g. 600", Validation: []string{"number"}},
				{ConfigKey: "flow_control", Type: "checkbox", Label: "Flow Control "},
			},
		},
	},
	IfaceAX25KISS: {
		{
			Fields: []InterfaceField{
				PortFieldNoSerial(),
				{ConfigKey: "callsign", Type: "edit", Label: "Callsign: ", Placeholder: "e.g. NO1CLL", Validation: []string{"required"}},
				{ConfigKey: "ssid", Type: "edit", Label: "SSID: ", Placeholder: "e.g. 0", Validation: []string{"required"}},
				{ConfigKey: "speed", Type: "edit", Label: "Speed (bps): ", Placeholder: "e.g. 115200", Validation: []string{"required", "number"}},
				{ConfigKey: "databits", Type: "edit", Label: "Databits: ", Placeholder: "e.g. 8", Validation: []string{"required", "number"}},
				{ConfigKey: "parity", Type: "edit", Label: "Parity: ", Validation: []string{"number"}},
				{ConfigKey: "stopbits", Type: "edit", Label: "Stopbits: ", Placeholder: "e.g. 1", Validation: []string{"number"}},
				{ConfigKey: "preamble", Type: "edit", Label: "Preamble (miliseconds): ", Placeholder: "e.g. 150", Validation: []string{"required", "number"}},
				{ConfigKey: "txtail", Type: "edit", Label: "TX Tail (miliseconds): ", Placeholder: "e.g. 10", Validation: []string{"required", "number"}},
				{ConfigKey: "slottime", Type: "edit", Label: "Slottime (miliseconds): ", Placeholder: "e.g. 20", Validation: []string{"required", "number"}},
				{ConfigKey: "persistence", Type: "edit", Label: "Persistence (miliseconds): ", Placeholder: "e.g. 200", Validation: []string{"required", "number"}},
			},
			AdditionalOptions: []InterfaceField{
				{ConfigKey: "flow_control", Type: "checkbox", Label: "Flow Control "},
			},
		},
	},
	IfaceCustom: {
		{
			Fields: []InterfaceField{
				{ConfigKey: "type", Type: "edit", Label: "Interface Type: ", Placeholder: "Name of custom interface class", Validation: []string{"required"}},
				{ConfigKey: "custom_parameters", Type: "keyvaluepairs", Label: "Parameters: "},
			},
		},
	},
	"default": {{}},
}
