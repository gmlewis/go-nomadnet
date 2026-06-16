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

func TestSelectableInterfaceItemStatusText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		want    string
	}{
		{"enabled", true, "Enabled"},
		{"disabled", false, "Disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &SelectableInterfaceItem{IsEnabled: tt.enabled}
			if got := s.StatusText(); got != tt.want {
				t.Errorf("StatusText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectableInterfaceItemConnectedText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		connected bool
		want      string
	}{
		{"connected", true, "Connected"},
		{"disconnected", false, "Disconnected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &SelectableInterfaceItem{IsConnected: tt.connected}
			if got := s.ConnectedText(); got != tt.want {
				t.Errorf("ConnectedText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectableInterfaceItemTitleText(t *testing.T) {
	t.Parallel()

	s := &SelectableInterfaceItem{Icon: "↗", Name: "TCPClient"}
	want := "↗  TCPClient"
	if got := s.TitleText(); got != want {
		t.Errorf("TitleText() = %q, want %q", got, want)
	}
}

func TestSelectableInterfaceItemUpdateStats(t *testing.T) {
	t.Parallel()

	s := &SelectableInterfaceItem{TX: 100, RX: 200}
	s.UpdateStats(500, 1000)
	if s.TX != 500 {
		t.Errorf("TX = %v, want 500", s.TX)
	}
	if s.RX != 1000 {
		t.Errorf("RX = %v, want 1000", s.RX)
	}
}

func TestSelectableInterfaceItemByteFormatting(t *testing.T) {
	t.Parallel()

	s := &SelectableInterfaceItem{TX: 1536, RX: 1048576}
	txText := s.TXText()
	rxText := s.RXText()
	if txText != "1.5 KB" {
		t.Errorf("TXText() = %q, want %q", txText, "1.5 KB")
	}
	if rxText != "1.0 MB" {
		t.Errorf("RXText() = %q, want %q", rxText, "1.0 MB")
	}
}
