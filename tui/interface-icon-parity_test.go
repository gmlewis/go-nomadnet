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

// TestGetInterfaceIconPythonParity is a LIVE cross-implementation check: it
// execs Python's nomadnet.ui.textui.Interfaces._get_interface_icon
// (Interfaces.py:28) and derives the expected glyph for every (glyphset,
// interface type) pair freshly on every run. Go owns the input battery; Python
// owns the reference behavior. The test SKIPs, not fails, when the Python
// reference is not importable.
//
// Python maps each interface type to a glyph tuple
// (NetworkInterfaceType / RNodeInterfaceType / SerialInterfaceType /
// OtherInterfaceType) and selects the plain / unicode / nerd-font glyph by
// the glyphset index, falling back to the OtherInterfaceType glyph for
// unknown types. The cases below cover every type the Python table names
// plus the two fallback types (CustomInterface and UnknownType).
func TestGetInterfaceIconPythonParity(t *testing.T) {
	t.Parallel()

	type iconInput struct {
		Glyphset string `json:"glyphset"`
		Iface    string `json:"iface_type"`
	}
	tests := []struct {
		name     string
		glyphset string
		ifType   string
	}{
		// NetworkInterfaceType
		{"backbone plain", GlyphPlain, IfaceBackbone},
		{"backbone unicode", GlyphUnicode, IfaceBackbone},
		{"backbone nerd", GlyphNerd, IfaceBackbone},
		{"auto plain", GlyphPlain, IfaceAuto},
		{"auto unicode", GlyphUnicode, IfaceAuto},
		{"auto nerd", GlyphNerd, IfaceAuto},
		{"tcpclient plain", GlyphPlain, IfaceTCPClient},
		{"tcpclient unicode", GlyphUnicode, IfaceTCPClient},
		{"tcpclient nerd", GlyphNerd, IfaceTCPClient},
		{"tcpserver plain", GlyphPlain, IfaceTCPServer},
		{"tcpserver unicode", GlyphUnicode, IfaceTCPServer},
		{"tcpserver nerd", GlyphNerd, IfaceTCPServer},
		{"udp plain", GlyphPlain, IfaceUDP},
		{"udp unicode", GlyphUnicode, IfaceUDP},
		{"udp nerd", GlyphNerd, IfaceUDP},
		{"i2p plain", GlyphPlain, IfaceI2P},
		{"i2p unicode", GlyphUnicode, IfaceI2P},
		{"i2p nerd", GlyphNerd, IfaceI2P},

		// RNodeInterfaceType
		{"rnode plain", GlyphPlain, IfaceRNode},
		{"rnode unicode", GlyphUnicode, IfaceRNode},
		{"rnode nerd", GlyphNerd, IfaceRNode},
		{"rnodemulti plain", GlyphPlain, IfaceRNodeMulti},
		{"rnodemulti unicode", GlyphUnicode, IfaceRNodeMulti},
		{"rnodemulti nerd", GlyphNerd, IfaceRNodeMulti},

		// SerialInterfaceType
		{"serial plain", GlyphPlain, IfaceSerial},
		{"serial unicode", GlyphUnicode, IfaceSerial},
		{"serial nerd", GlyphNerd, IfaceSerial},
		{"kiss plain", GlyphPlain, IfaceKISS},
		{"kiss unicode", GlyphUnicode, IfaceKISS},
		{"kiss nerd", GlyphNerd, IfaceKISS},
		{"ax25kiss plain", GlyphPlain, IfaceAX25KISS},
		{"ax25kiss unicode", GlyphUnicode, IfaceAX25KISS},
		{"ax25kiss nerd", GlyphNerd, IfaceAX25KISS},

		// OtherInterfaceType (PipeInterface + fallbacks)
		{"pipe plain", GlyphPlain, IfacePipe},
		{"pipe unicode", GlyphUnicode, IfacePipe},
		{"pipe nerd", GlyphNerd, IfacePipe},
		{"custom plain", GlyphPlain, IfaceCustom},
		{"custom unicode", GlyphUnicode, IfaceCustom},
		{"custom nerd", GlyphNerd, IfaceCustom},
		{"unknown plain", GlyphPlain, "UnknownType"},
		{"unknown unicode", GlyphUnicode, "UnknownType"},
		{"unknown nerd", GlyphNerd, "UnknownType"},

		// Empty/blank glyphset defaults to unicode (Python default index 1).
		{"blank glyphset defaults unicode", "", IfaceBackbone},
		{"unrecognized glyphset defaults unicode", "weird", IfaceRNode},
	}

	inputs := make([]iconInput, len(tests))
	for i, tt := range tests {
		inputs[i] = iconInput{Glyphset: tt.glyphset, Iface: tt.ifType}
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Interfaces as I
inputs = json.load(sys.stdin)
out = [I._get_interface_icon(inp["glyphset"], inp["iface_type"]) for inp in inputs]
json.dump(out, sys.stdout)
`

	var want []string
	runPythonNomadnet(t, inputs, script, &want)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GetInterfaceIcon(tt.glyphset, tt.ifType)
			if got != want[i] {
				t.Errorf("GetInterfaceIcon(%q, %q) = %q, want %q (Python)",
					tt.glyphset, tt.ifType, got, want[i])
			}
		})
	}
}
