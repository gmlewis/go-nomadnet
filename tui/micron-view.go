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
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// MicronViewDisplay renders Micron pages as tview widgets.
type MicronViewDisplay struct {
	app    *App
	widget tview.Primitive
	view   *tview.TextView
}

// NewMicronViewDisplay creates a new Micron page viewer.
func NewMicronViewDisplay(app *App) *MicronViewDisplay {
	mvd := &MicronViewDisplay{app: app}

	mvd.view = applyWheelMultiplier(tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetTextAlign(tview.AlignLeft))

	mvd.widget = mvd.view
	return mvd
}

// Widget returns the tview primitive for this display.
func (mvd *MicronViewDisplay) Widget() tview.Primitive {
	return mvd.widget
}

// RenderPage renders Micron markup to tview-compatible colored text.
func (mvd *MicronViewDisplay) RenderPage(markup string) {
	nodes := micron.Parse(markup)
	result := renderNodes(nodes)
	mvd.view.SetText(result)
}

// Clear clears the display.
func (mvd *MicronViewDisplay) Clear() {
	mvd.view.SetText("")
}

// renderNodes converts Micron AST nodes to tview colored text.
func renderNodes(nodes []*micron.Node) string {
	var sb strings.Builder
	for _, node := range nodes {
		switch node.Type {
		case micron.NodeHeading:
			level := node.Level
			switch {
			case level >= 3:
				sb.WriteString("[::d]")
			case level == 2:
				sb.WriteString("[::b]")
			default:
				sb.WriteString("[::b]")
			}
			for _, child := range node.Children {
				sb.WriteString(child.Text)
			}
			sb.WriteString("[-]\n")
		case micron.NodeText:
			sb.WriteString(node.Text)
		case micron.NodeBold:
			sb.WriteString("[::b]")
		case micron.NodeItalic:
			sb.WriteString("[::i]")
		case micron.NodeUnderline:
			sb.WriteString("[::u]")
		case micron.NodeReset:
			sb.WriteString("[-]")
		case micron.NodeDivider:
			sb.WriteString(strings.Repeat("─", 30))
			sb.WriteString("\n")
		case micron.NodeLink:
			sb.WriteString("[yellow]")
			if node.LinkLabel != "" {
				sb.WriteString(node.LinkLabel)
			} else {
				sb.WriteString(node.LinkURL)
			}
			sb.WriteString("[-]")
		case micron.NodeField:
			if node.FieldName != "" {
				fmt.Fprintf(&sb, "[cyan]%v[-]: ", node.FieldName)
			}
			if node.FieldData != "" {
				fmt.Fprintf(&sb, "[gray][%v][-]", node.FieldData)
			} else {
				sb.WriteString("[gray][...] [-]")
			}
		case micron.NodeCheckbox:
			cb := "[ ]"
			if node.FieldData == "true" || node.FieldData == "1" {
				cb = "[x]"
			}
			if node.FieldName != "" {
				fmt.Fprintf(&sb, "[cyan]%v[-] %v", node.FieldName, cb)
			} else {
				sb.WriteString(cb)
			}
		case micron.NodeRadio:
			rb := "( )"
			if node.FieldData == "true" || node.FieldData == "1" {
				rb = "(*)"
			}
			if node.FieldName != "" {
				fmt.Fprintf(&sb, "[cyan]%v[-] %v", node.FieldName, rb)
			} else {
				sb.WriteString(rb)
			}
		case micron.NodeColor:
			if node.FGColor != "" {
				sb.WriteString("[")
				sb.WriteString(mapColor(node.FGColor))
				sb.WriteString("]")
			}
		case micron.NodePartial:
			fmt.Fprintf(&sb, "[gray][partial:%v][-]", node.PartialID)
		case micron.NodeAnchor:
			fmt.Fprintf(&sb, "[gray][#%v][-]", node.AnchorName)
		default:
			if node.Text != "" {
				sb.WriteString(node.Text)
			}
		}
	}
	return sb.String()
}

// mapColor maps Micron color codes to tview color names.
func mapColor(color string) string {
	switch strings.ToLower(color) {
	case "red", "f00":
		return "red"
	case "green", "0f0":
		return "green"
	case "blue", "00f":
		return "blue"
	case "yellow", "ff0":
		return "yellow"
	case "cyan", "0ff":
		return "cyan"
	case "magenta", "f0f":
		return "magenta"
	case "white", "fff":
		return "white"
	case "black", "000":
		return "black"
	case "gray", "888":
		return "gray"
	default:
		if len(color) > 0 && color[0] == '#' {
			return color
		}
		return "#" + color
	}
}
