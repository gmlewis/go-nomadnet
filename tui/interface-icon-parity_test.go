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

// TestGetInterfaceIconPythonParity verifies GetInterfaceIcon against Python's
// _get_interface_icon (Interfaces.py:28). Expected values were captured from
// /tmp/icon_ref.py, which inlines the Python function (the module import pulls
// in urwid). Python maps each interface type to a glyph tuple
// (NetworkInterfaceType / RNodeInterfaceType / SerialInterfaceType /
// OtherInterfaceType) and selects the plain / unicode / nerd-font glyph by
// the glyphset index, falling back to the OtherInterfaceType glyph for
// unknown types. The cases below cover every type the Python table names
// plus the two fallback types (CustomInterface and UnknownType).
func TestGetInterfaceIconPythonParity(t *testing.T) {
	t.Parallel()

	networkPlain := "(IP)"
	networkUnicode := "\U0001f5a7"
	networkNerd := "\U000f0200"

	rnodePlain := "(R)"
	rnodeUnicode := "ᚱ"
	rnodeNerd := "\U000f043a"

	serialPlain := "(<->)"
	serialUnicode := "↔"
	serialNerd := "\U000f065c"

	otherPlain := "(#)"
	otherUnicode := "\U0001f67e"
	otherNerd := ""

	tests := []struct {
		name     string
		glyphset string
		ifType   string
		want     string
	}{
		// NetworkInterfaceType
		{"backbone plain", GlyphPlain, IfaceBackbone, networkPlain},
		{"backbone unicode", GlyphUnicode, IfaceBackbone, networkUnicode},
		{"backbone nerd", GlyphNerd, IfaceBackbone, networkNerd},
		{"auto plain", GlyphPlain, IfaceAuto, networkPlain},
		{"auto unicode", GlyphUnicode, IfaceAuto, networkUnicode},
		{"auto nerd", GlyphNerd, IfaceAuto, networkNerd},
		{"tcpclient plain", GlyphPlain, IfaceTCPClient, networkPlain},
		{"tcpclient unicode", GlyphUnicode, IfaceTCPClient, networkUnicode},
		{"tcpclient nerd", GlyphNerd, IfaceTCPClient, networkNerd},
		{"tcpserver plain", GlyphPlain, IfaceTCPServer, networkPlain},
		{"tcpserver unicode", GlyphUnicode, IfaceTCPServer, networkUnicode},
		{"tcpserver nerd", GlyphNerd, IfaceTCPServer, networkNerd},
		{"udp plain", GlyphPlain, IfaceUDP, networkPlain},
		{"udp unicode", GlyphUnicode, IfaceUDP, networkUnicode},
		{"udp nerd", GlyphNerd, IfaceUDP, networkNerd},
		{"i2p plain", GlyphPlain, IfaceI2P, networkPlain},
		{"i2p unicode", GlyphUnicode, IfaceI2P, networkUnicode},
		{"i2p nerd", GlyphNerd, IfaceI2P, networkNerd},

		// RNodeInterfaceType
		{"rnode plain", GlyphPlain, IfaceRNode, rnodePlain},
		{"rnode unicode", GlyphUnicode, IfaceRNode, rnodeUnicode},
		{"rnode nerd", GlyphNerd, IfaceRNode, rnodeNerd},
		{"rnodemulti plain", GlyphPlain, IfaceRNodeMulti, rnodePlain},
		{"rnodemulti unicode", GlyphUnicode, IfaceRNodeMulti, rnodeUnicode},
		{"rnodemulti nerd", GlyphNerd, IfaceRNodeMulti, rnodeNerd},

		// SerialInterfaceType
		{"serial plain", GlyphPlain, IfaceSerial, serialPlain},
		{"serial unicode", GlyphUnicode, IfaceSerial, serialUnicode},
		{"serial nerd", GlyphNerd, IfaceSerial, serialNerd},
		{"kiss plain", GlyphPlain, IfaceKISS, serialPlain},
		{"kiss unicode", GlyphUnicode, IfaceKISS, serialUnicode},
		{"kiss nerd", GlyphNerd, IfaceKISS, serialNerd},
		{"ax25kiss plain", GlyphPlain, IfaceAX25KISS, serialPlain},
		{"ax25kiss unicode", GlyphUnicode, IfaceAX25KISS, serialUnicode},
		{"ax25kiss nerd", GlyphNerd, IfaceAX25KISS, serialNerd},

		// OtherInterfaceType (PipeInterface + fallbacks)
		{"pipe plain", GlyphPlain, IfacePipe, otherPlain},
		{"pipe unicode", GlyphUnicode, IfacePipe, otherUnicode},
		{"pipe nerd", GlyphNerd, IfacePipe, otherNerd},
		{"custom plain", GlyphPlain, IfaceCustom, otherPlain},
		{"custom unicode", GlyphUnicode, IfaceCustom, otherUnicode},
		{"custom nerd", GlyphNerd, IfaceCustom, otherNerd},
		{"unknown plain", GlyphPlain, "UnknownType", otherPlain},
		{"unknown unicode", GlyphUnicode, "UnknownType", otherUnicode},
		{"unknown nerd", GlyphNerd, "UnknownType", otherNerd},

		// Empty/blank glyphset defaults to unicode (Python default index 1).
		{"blank glyphset defaults unicode", "", IfaceBackbone, networkUnicode},
		{"unrecognized glyphset defaults unicode", "weird", IfaceRNode, rnodeUnicode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GetInterfaceIcon(tt.glyphset, tt.ifType)
			if got != tt.want {
				t.Errorf("GetInterfaceIcon(%q, %q) = %q, want %q",
					tt.glyphset, tt.ifType, got, tt.want)
			}
		})
	}
}
