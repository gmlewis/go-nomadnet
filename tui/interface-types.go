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

// Interface type constants matching Python's INTERFACE_FIELDS keys.
const (
	IfaceBackbone       = "BackboneInterface"
	IfaceAuto           = "AutoInterface"
	IfaceI2P            = "I2PInterface"
	IfaceTCPServer      = "TCPServerInterface"
	IfaceTCPClient      = "TCPClientInterface"
	IfaceUDP            = "UDPInterface"
	IfaceRNode          = "RNodeInterface"
	IfaceRNodeMulti     = "RNodeMultiInterface"
	IfaceSerial         = "SerialInterface"
	IfacePipe           = "PipeInterface"
	IfaceKISS           = "KISSInterface"
	IfaceAX25KISS       = "AX25KISSInterface"
	IfaceCustom         = "CustomInterface"
)

// allInterfaceTypes lists all supported interface type names.
var allInterfaceTypes = []string{
	IfaceBackbone, IfaceAuto, IfaceI2P, IfaceTCPServer, IfaceTCPClient,
	IfaceUDP, IfaceRNode, IfaceRNodeMulti, IfaceSerial, IfacePipe,
	IfaceKISS, IfaceAX25KISS, IfaceCustom,
}

// networkTypes are IP-based transport interfaces.
var networkTypes = map[string]bool{
	IfaceBackbone:  true,
	IfaceAuto:      true,
	IfaceI2P:       true,
	IfaceTCPServer: true,
	IfaceTCPClient: true,
	IfaceUDP:       true,
}

// rnodeTypes are RNode LoRa interfaces.
var rnodeTypes = map[string]bool{
	IfaceRNode:      true,
	IfaceRNodeMulti: true,
}

// serialTypes are serial-based interfaces.
var serialTypes = map[string]bool{
	IfaceSerial:   true,
	IfaceKISS:     true,
	IfaceAX25KISS: true,
}

// requiredFields maps each interface type to its required config fields.
var requiredFields = map[string][]string{
	IfaceTCPClient:  {"target_host", "target_port"},
	IfaceTCPServer:  {"listen_ip"},
	IfaceUDP:        {"listen_ip", "forward_ip", "forward_port"},
	IfaceRNode:      {"frequency"},
	IfaceSerial:     {"speed"},
	IfacePipe:       {"command"},
	IfaceKISS:       {"speed", "preamble", "txtail", "slottime", "persistence"},
	IfaceAX25KISS:   {"speed", "callsign", "ssid", "preamble", "txtail", "slottime", "persistence"},
}

// InterfaceCategory returns the glyph category for the given type.
func InterfaceCategory(ifType string) string {
	switch {
	case networkTypes[ifType]:
		return "network"
	case rnodeTypes[ifType]:
		return "rnode"
	case serialTypes[ifType]:
		return "serial"
	default:
		return "other"
	}
}

// InterfaceGlyph returns a display glyph for the interface type.
func InterfaceGlyph(ifType string) string {
	switch {
	case networkTypes[ifType]:
		return "network"
	case rnodeTypes[ifType]:
		return "rnode"
	case serialTypes[ifType]:
		return "serial"
	default:
		return "other"
	}
}

// InterfaceIcon returns a single-character icon for the interface type.
func InterfaceIcon(ifType string) string {
	switch ifType {
	case IfaceTCPClient:
		return "↗"
	case IfaceTCPServer:
		return "↙"
	case IfaceRNode, IfaceRNodeMulti:
		return "R"
	case IfaceSerial, IfaceKISS, IfaceAX25KISS:
		return "↔"
	case IfacePipe:
		return "#"
	case IfaceCustom:
		return "C"
	default:
		return "•"
	}
}

// AllInterfaceTypes returns the list of all supported interface types.
func AllInterfaceTypes() []string {
	out := make([]string, len(allInterfaceTypes))
	copy(out, allInterfaceTypes)
	return out
}

// RequiredFields returns the required config fields for the given type.
func RequiredFields(ifType string) []string {
	fields := requiredFields[ifType]
	out := make([]string, len(fields))
	copy(out, fields)
	return out
}
