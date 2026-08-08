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

package micron

import (
	"fmt"
	"strings"
)

// RenderState tracks the current formatting state during rendering.
// Matches Python's state dict in MicronParser.py markup_to_attrmaps().
type RenderState struct {
	Bold      bool
	Underline bool
	Italic    bool
	FGColor   string
	BGColor   string
	Align     Alignment
	Depth     int
}

// DefaultFG is the default foreground color.
const DefaultFG = "dddddd"

// DefaultBG is the default background color ("default" = terminal bg).
const DefaultBG = "default"

// NewRenderState creates a default render state.
func NewRenderState() *RenderState {
	return &RenderState{
		FGColor: DefaultFG,
		BGColor: DefaultBG,
		Align:   AlignLeft,
	}
}

// RenderToTView renders Micron AST nodes to a tview-formatted string
// using tview color/formatting tags. This is the main render function
// used by the browser display to show Micron page content.
// Matches Python's markup_to_attrmaps() at MicronParser.py:117.
func RenderToTView(nodes []*Node) string {
	state := NewRenderState()
	var sb strings.Builder

	for _, node := range nodes {
		renderNode(&sb, node, state)
	}

	return sb.String()
}

// RenderToPlainText renders Micron AST nodes to plain text with all
// formatting stripped. Matches Python's strip_modifiers() at util.py.
func RenderToPlainText(nodes []*Node) string {
	var sb strings.Builder

	for _, node := range nodes {
		switch node.Type {
		case NodeText:
			sb.WriteString(sectionIndent(node.Depth))
			sb.WriteString(node.Text)
		case NodeHeading:
			for _, child := range node.Children {
				sb.WriteString(child.Text)
			}
			sb.WriteString("\n")
		case NodeDivider:
			char := node.Text
			if char == "" {
				char = "\u2500"
			}
			sb.WriteString(strings.Repeat(char, 40))
			sb.WriteString("\n")
		case NodeLink:
			sb.WriteString(node.LinkLabel)
		case NodeField:
			sb.WriteString(node.FieldName)
		case NodeBold, NodeUnderline, NodeItalic,
			NodeColor, NodeReset, NodeAlign,
			NodeAnchor, NodePartial, NodeLiteral:
			// Skip formatting/toggle nodes in plain text
		case NodeTable:
			renderTablePlainText(&sb, node)
		}
	}

	return sb.String()
}

// renderNode renders a single node to the string builder.
func renderNode(sb *strings.Builder, node *Node, state *RenderState) {
	switch node.Type {
	case NodeText:
		sb.WriteString(sectionIndent(node.Depth))
		sb.WriteString(node.Text)

	case NodeBold:
		state.Bold = !state.Bold
		if state.Bold {
			sb.WriteString("[::b]")
		} else {
			sb.WriteString("[-::-]")
		}

	case NodeUnderline:
		state.Underline = !state.Underline
		if state.Underline {
			sb.WriteString("[::u]")
		} else {
			sb.WriteString("[-::-]")
		}

	case NodeItalic:
		state.Italic = !state.Italic
		if state.Italic {
			sb.WriteString("[::I]")
		} else {
			sb.WriteString("[-::-]")
		}

	case NodeReset:
		state.Bold = false
		state.Underline = false
		state.Italic = false
		state.FGColor = DefaultFG
		state.BGColor = DefaultBG
		sb.WriteString("[-:-:-]")

	case NodeColor:
		if node.FGColor != "" && node.FGColor != "default" {
			state.FGColor = expandColor(node.FGColor)
			fmt.Fprintf(sb, "[#%v]", state.FGColor)
		} else if node.FGColor == "default" {
			state.FGColor = DefaultFG
			fmt.Fprintf(sb, "[#%v]", state.FGColor)
		}
		if node.BGColor != "" && node.BGColor != "default" {
			state.BGColor = expandColor(node.BGColor)
			fmt.Fprintf(sb, "[:%v]", state.BGColor)
		} else if node.BGColor == "default" {
			state.BGColor = DefaultBG
			sb.WriteString("[:-:-]")
		}

	case NodeHeading:
		renderHeading(sb, node, state)

	case NodeDivider:
		sb.WriteString("\n")
		char := node.Text
		if char == "" {
			char = "\u2500"
		}
		sb.WriteString(strings.Repeat(char, 40))
		sb.WriteString("\n")

	case NodeLink:
		fmt.Fprintf(sb, `["%v"]`, node.LinkURL)
		sb.WriteString(node.LinkLabel)
		sb.WriteString(`[""]`)

	case NodeField:
		fmt.Fprintf(sb, "[%v]", node.FieldName)

	case NodeAlign:
		state.Align = node.Align

	case NodeAnchor:
		// Anchors are position markers; no visual output needed

	case NodePartial:
		sb.WriteString("\u29D6") // ⧖ hourglass placeholder

	case NodeLiteral:
		// Literal toggle; no visual output

	case NodeTable:
		renderTable(sb, node)
	}
}

// renderHeading renders a heading node with appropriate tview formatting.
func renderHeading(sb *strings.Builder, node *Node, _ *RenderState) {
	indent := strings.Repeat("  ", node.Level-1)

	sb.WriteString(indent)
	switch node.Level {
	case 1:
		sb.WriteString("[#000000:#bbbbbb::b]")
	case 2:
		sb.WriteString("[#111111:#999999::b]")
	case 3:
		sb.WriteString("[#222222:#777777::b]")
	default:
		sb.WriteString("[::b]")
	}

	for _, child := range node.Children {
		sb.WriteString(child.Text)
	}
	sb.WriteString("[-:-:-] ")
	sb.WriteString("\n")
}

// sectionIndent returns the left indent string for the given section depth.
// SECTION_INDENT = 2, indent = (depth-1)*2.
func sectionIndent(depth int) string {
	if depth <= 1 {
		return ""
	}
	return strings.Repeat(" ", (depth-1)*2)
}

// expandColor converts a Micron color code to a 6-digit hex color.
// 3-digit colors like "F00" are expanded to "FF0000".
// 6-digit colors are passed through.
func expandColor(color string) string {
	if len(color) == 6 {
		return color
	}
	if len(color) == 3 {
		return string(color[0]) + string(color[0]) +
			string(color[1]) + string(color[1]) +
			string(color[2]) + string(color[2])
	}
	return color
}

// tableColWidths computes the maximum width for each column in a table.
func tableColWidths(rows [][]string) []int {
	if len(rows) == 0 {
		return nil
	}
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	widths := make([]int, maxCols)
	for _, row := range rows {
		for i, cell := range row {
			w := len(cell)
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// formatTableRow formats a single table row with proper column alignment.
func formatTableRow(cells []string, widths []int, pad string) string {
	var sb strings.Builder
	for i, cell := range cells {
		if i > 0 {
			sb.WriteString(" ")
		}
		cellW := 0
		if i < len(widths) {
			cellW = widths[i]
		}
		sb.WriteString(cell)
		if padding := cellW - len(cell); padding > 0 {
			sb.WriteString(strings.Repeat(pad, padding))
		}
	}
	return sb.String()
}

// renderTable renders a table node as tview-formatted text.
func renderTable(sb *strings.Builder, node *Node) {
	rows := node.TableRows
	if len(rows) == 0 {
		return
	}
	widths := tableColWidths(rows)
	indent := sectionIndent(node.Depth)

	for i, row := range rows {
		sb.WriteString(indent)
		if i == 0 {
			sb.WriteString("[::b]")
		}
		sb.WriteString(formatTableRow(row, widths, " "))
		if i == 0 {
			sb.WriteString("[-::-]")
		}
		sb.WriteString("\n")
	}
}

// renderTablePlainText renders a table node as plain text.
func renderTablePlainText(sb *strings.Builder, node *Node) {
	rows := node.TableRows
	if len(rows) == 0 {
		return
	}
	widths := tableColWidths(rows)
	indent := sectionIndent(node.Depth)

	for _, row := range rows {
		sb.WriteString(indent)
		sb.WriteString(formatTableRow(row, widths, " "))
		sb.WriteString("\n")
	}
}
