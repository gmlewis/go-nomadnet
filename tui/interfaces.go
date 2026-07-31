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
	app    *App
	widget tview.Primitive
	layout *tview.Flex

	// Keyboard shortcut callbacks (Python: InterfaceFiller.keypress)
	OnAddInterface    func()
	OnEditInterface   func()
	OnRemoveInterface func()
	OnConfigEditor    func()
}

// NewInterfacesDisplay creates a new interfaces display.
func NewInterfacesDisplay(app *App, interfaces []InterfaceInfo) *InterfacesDisplay {
	id := &InterfacesDisplay{app: app}

	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
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
	layout.SetBorder(true)
	layout.SetInputCapture(id.handleInput)

	id.layout = layout
	id.widget = layout
	return id
}

// handleInput processes keyboard shortcuts for the interfaces display.
// Matches Python's InterfaceFiller.keypress() at Interfaces.py:1391.
func (id *InterfacesDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlA:
		if id.OnAddInterface != nil {
			id.OnAddInterface()
		}
		return nil
	case tcell.KeyCtrlE:
		if id.OnEditInterface != nil {
			id.OnEditInterface()
		}
		return nil
	case tcell.KeyCtrlX:
		if id.OnRemoveInterface != nil {
			id.OnRemoveInterface()
		}
		return nil
	case tcell.KeyCtrlW:
		if id.OnConfigEditor != nil {
			id.OnConfigEditor()
		}
		return nil
	}

	return event
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

		sb.WriteString(fmt.Sprintf("[::b]%s[-] %s(%s)[-]\n", iface.Name, statusColor, iface.Status))
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

// FormatInterfaceEntry produces a single-line summary for the interface
// list widget. Includes icon, name, and status indicator.
// Matches Python's InterfaceListEntry widget.
func FormatInterfaceEntry(iface InterfaceInfo) string {
	icon := InterfaceIcon(iface.Type)
	statusColor := "[green]"
	if iface.Status != "connected" {
		statusColor = "[red]"
	}
	return fmt.Sprintf("%s %s %s(%s)[-]", icon, iface.Name, statusColor, iface.Status)
}

// FormatInterfaceDetail produces a multi-line detail view for the
// interface info panel. Shows name, type, status, target, bandwidth,
// and recent traffic data when available.
// Matches Python's InterfaceDetail view layout.
func FormatInterfaceDetail(iface InterfaceInfo) string {
	var sb strings.Builder

	sb.WriteString("[::b]Interface Details[-]\n\n")
	sb.WriteString(fmt.Sprintf("  Name: %s\n", iface.Name))
	sb.WriteString(fmt.Sprintf("  Type: %s\n", iface.Type))

	statusIcon := "○"
	if iface.Status == "connected" {
		statusIcon = "●"
	}
	sb.WriteString(fmt.Sprintf("  Status: %s %s\n", statusIcon, iface.Status))

	if iface.Target != "" {
		sb.WriteString(fmt.Sprintf("  Target: %s\n", iface.Target))
	}

	sb.WriteString(fmt.Sprintf("  Bandwidth: %s\n", formatBandwidth(iface.Bandwidth)))

	if len(iface.Traffic) > 0 {
		sb.WriteString("\n  [gray]Recent Traffic[-]\n")
		sb.WriteString(fmt.Sprintf("  Samples: %d\n", len(iface.Traffic)))
	}

	return sb.String()
}

// ShowEnableDisableConfirm shows a confirmation dialog for enabling or
// disabling an interface. Matches Python's Interfaces.py:2570-2590.
func (id *InterfacesDisplay) ShowEnableDisableConfirm(name string, enabled bool, onConfirm func()) {
	action := "Enable"
	if enabled {
		action = "Disable"
	}
	msg := fmt.Sprintf("%s interface %s?", action, name)

	id.app.Dialogs.ShowConfirmDialog(msg, func() {
		if onConfirm != nil {
			onConfirm()
		}
	}, nil)
}

// ShowRestartRequired shows a notice that a restart is required after
// interface changes. Matches Python's Interfaces.py:2589-2610.
func (id *InterfacesDisplay) ShowRestartRequired() {
	msg := "RNS must be restarted for interface changes to take effect.\nRestart Nomad Network to apply changes."

	id.app.Dialogs.ShowDialog("Restart Required",
		tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText(msg),
		50, 5, nil)
}

// ShowInterfaceError shows an error message for interface operations.
// Matches Python's Interfaces.py:2619-2650.
func (id *InterfacesDisplay) ShowInterfaceError(errMsg string) {
	msg := fmt.Sprintf("[red]Error:[-] %s", errMsg)

	id.app.Dialogs.ShowDialog("Interface Error",
		tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText(msg),
		50, 5, nil)
}

// ShowRNSDisconnected shows an overlay when the RNS transport is lost.
// Matches Python's Interfaces.py:2794-2830.
func (id *InterfacesDisplay) ShowRNSDisconnected() {
	msg := "[red]RNS Instance Disconnected[-]\n\nThe RNS transport connection has been lost.\nCheck your network configuration and restart if necessary."

	id.app.Dialogs.ShowDialog("Disconnected",
		tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText(msg),
		50, 8, nil)
}
