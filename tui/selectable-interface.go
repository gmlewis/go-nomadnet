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

import "fmt"

// SelectableInterfaceItem represents a selectable interface entry in
// the interfaces list. Matches Python's SelectableInterfaceItem at
// Interfaces.py:1125.
type SelectableInterfaceItem struct {
	Name        string
	Icon        string
	IfaceType   string
	IsConnected bool
	IsEnabled   bool
	TX          int64
	RX          int64
	IfaceOpts   any
	OnSelect    func(name string)
}

// StatusText returns the enabled/disabled status string.
func (s *SelectableInterfaceItem) StatusText() string {
	if s.IsEnabled {
		return "Enabled"
	}
	return "Disabled"
}

// ConnectedText returns the connected/disconnected status string.
func (s *SelectableInterfaceItem) ConnectedText() string {
	if s.IsConnected {
		return "Connected"
	}
	return "Disconnected"
}

// TitleText returns the display title with icon and name.
func (s *SelectableInterfaceItem) TitleText() string {
	return fmt.Sprintf("%v  %v", s.Icon, s.Name)
}

// TXText returns a formatted string of bytes sent.
func (s *SelectableInterfaceItem) TXText() string {
	return FormatBytes(float64(s.TX))
}

// RXText returns a formatted string of bytes received.
func (s *SelectableInterfaceItem) RXText() string {
	return FormatBytes(float64(s.RX))
}

// UpdateStats updates the TX/RX byte counters.
func (s *SelectableInterfaceItem) UpdateStats(tx, rx int64) {
	s.TX = tx
	s.RX = rx
}
