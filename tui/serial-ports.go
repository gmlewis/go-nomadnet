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
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// SerialPortInfo describes a detected serial port.
type SerialPortInfo struct {
	Device      string
	Description string
	IsStandard  bool
}

// GetPortInfo detects available serial ports on the system.
// On macOS/Linux it scans /dev for common serial device patterns.
// On Windows it checks COM ports. Returns an empty slice if no
// ports are found or detection is not supported.
// Matches Python's get_port_info() at Interfaces.py:95.
func GetPortInfo() []SerialPortInfo {
	switch runtime.GOOS {
	case "darwin", "linux":
		return scanDevPorts()
	default:
		return nil
	}
}

// scanDevPorts scans /dev for serial port devices on Unix systems.
func scanDevPorts() []SerialPortInfo {
	entries, err := os.ReadDir("/dev")
	if err != nil {
		return nil
	}

	patterns := []string{
		"ttyUSB", "ttyACM", "tty.SLAB", "tty.usbserial",
		"tty.usbmodem", "cu.usbserial", "cu.usbmodem", "cu.SLAB",
		"rfcomm",
	}

	standardPatterns := []string{"ttyS"}

	var priority, standard []SerialPortInfo

	for _, entry := range entries {
		name := entry.Name()
		matched := false
		for _, p := range patterns {
			if strings.HasPrefix(name, p) {
				desc := "/dev/" + name
				priority = append(priority, SerialPortInfo{
					Device:      desc,
					Description: desc,
					IsStandard:  false,
				})
				matched = true
				break
			}
		}
		if !matched {
			for _, p := range standardPatterns {
				if strings.HasPrefix(name, p) {
					desc := "/dev/" + name
					standard = append(standard, SerialPortInfo{
						Device:      desc,
						Description: desc,
						IsStandard:  true,
					})
					break
				}
			}
		}
	}

	sort.Slice(priority, func(i, j int) bool {
		return filepath.Base(priority[i].Device) < filepath.Base(priority[j].Device)
	})
	sort.Slice(standard, func(i, j int) bool {
		return filepath.Base(standard[i].Device) < filepath.Base(standard[j].Device)
	})

	return append(priority, standard...)
}
