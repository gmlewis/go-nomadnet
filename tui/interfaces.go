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
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/asciichart"
	"github.com/rivo/tview"
)

// InterfaceInfo holds status information for a network interface.
type InterfaceInfo struct {
	Name      string
	Type      string
	Status    string // "connected" or "disconnected"
	Target    string
	Bandwidth float64
	Traffic   []float64 // recent traffic samples for chart
}

// InterfacesDisplay shows RNS interface status and bandwidth charts.
type InterfacesDisplay struct {
	app    *tview.Application
	widget tview.Primitive
}

// NewInterfacesDisplay creates a new interfaces display.
func NewInterfacesDisplay(app *tview.Application, interfaces []InterfaceInfo) *InterfacesDisplay {
	id := &InterfacesDisplay{app: app}

	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[::b]Network Interfaces[-]")

	content := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText(formatInterfaces(interfaces))

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(content, 0, 1, true)

	id.widget = layout
	return id
}

// Widget returns the tview primitive for this display.
func (id *InterfacesDisplay) Widget() tview.Primitive {
	return id.widget
}

// formatInterfaces formats the interface list as text.
func formatInterfaces(interfaces []InterfaceInfo) string {
	if len(interfaces) == 0 {
		return "\n[yellow]No interfaces configured[-]\n\nAdd interfaces to your Reticulum config."
	}

	var sb strings.Builder
	chart := asciichart.New("unicode")

	for _, iface := range interfaces {
		statusColor := "[green]"
		if iface.Status != "connected" {
			statusColor = "[red]"
		}

		sb.WriteString(fmt.Sprintf("[::b]%s[-] %s(%s)\n", iface.Name, statusColor, iface.Status))
		sb.WriteString(fmt.Sprintf("  Type: %s  Target: %s\n", iface.Type, iface.Target))
		sb.WriteString(fmt.Sprintf("  Bandwidth: %s\n", formatBandwidth(iface.Bandwidth)))

		if len(iface.Traffic) > 0 {
			sb.WriteString("\n")
			chartView := chart.PlotSingle(iface.Traffic)
			for _, line := range strings.Split(chartView, "\n") {
				sb.WriteString("  ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatBandwidth formats a bandwidth value in bytes/sec.
func formatBandwidth(bps float64) string {
	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	size := bps
	unitIdx := 0

	for size >= 1024.0 && unitIdx < len(units)-1 {
		size /= 1024.0
		unitIdx++
	}

	if unitIdx == 0 {
		return fmt.Sprintf("%.0f %s", size, units[unitIdx])
	}
	return fmt.Sprintf("%.1f %s", size, units[unitIdx])
}
