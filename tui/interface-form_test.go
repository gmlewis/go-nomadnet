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
