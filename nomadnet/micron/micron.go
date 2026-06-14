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

// Package micron implements a parser for the Micron markup language.
//
// Micron is a lightweight markup language used by NomadNet for page
// rendering. It supports headings, formatting (bold, underline, italic),
// colors, links, input fields, tables, and partials.
//
// The parser produces an AST (Abstract Syntax Tree) of Node values
// that can be rendered to various outputs (text, tview widgets, etc.).
package micron

import (
	"regexp"
)

// NodeType identifies the type of an AST node.
type NodeType int

const (
	// NodeText is a plain text segment.
	NodeText NodeType = iota
	// NodeBold is bold text.
	NodeBold
	// NodeUnderline is underlined text.
	NodeUnderline
	// NodeItalic is italic text.
	NodeItalic
	// NodeHeading is a section heading (level 1-3).
	NodeHeading
	// NodeLink is a clickable link.
	NodeLink
	// NodeField is an input field.
	NodeField
	// NodeCheckbox is a checkbox field.
	NodeCheckbox
	// NodeRadio is a radio button field.
	NodeRadio
	// NodeDivider is a horizontal divider.
	NodeDivider
	// NodePartial is a page partial reference.
	NodePartial
	// NodeColor applies a foreground color.
	NodeColor
	// NodeReset resets all formatting to defaults.
	NodeReset
	// NodeAlign sets text alignment.
	NodeAlign
	// NodeAnchor declares a named anchor position.
	NodeAnchor
)

// Alignment represents text alignment.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
)

// Node represents a single node in the Micron AST.
type Node struct {
	Type NodeType
	Text string // for NodeText, NodeHeading

	// For NodeHeading
	Level int // 1, 2, or 3

	// For NodeLink
	LinkURL    string
	LinkLabel  string
	LinkFields string

	// For NodeField
	FieldName  string
	FieldType  string // "field", "checkbox", "radio"
	FieldWidth int
	FieldData  string
	FieldMask  bool

	// For NodeColor
	FGColor string
	BGColor string

	// For NodeAlign
	Align Alignment

	// For NodePartial
	PartialURL    string
	PartialFields string
	PartialID     string

	// For NodeAnchor
	AnchorName string

	// Children for composite nodes
	Children []*Node
}

// Parse parses Micron markup text and returns a list of top-level nodes.
func Parse(markup string) []*Node {
	lines := splitLines(markup)
	var nodes []*Node

	for _, line := range lines {
		if lineNodes := parseLine(line); len(lineNodes) > 0 {
			nodes = append(nodes, lineNodes...)
		}
	}

	return nodes
}

// ParseDocument parses a full Micron document and returns structured
// content with headings, paragraphs, and formatting.
func ParseDocument(markup string) *Document {
	lines := splitLines(markup)
	doc := &Document{}

	for _, line := range lines {
		if lineNodes := parseLine(line); len(lineNodes) > 0 {
			doc.Nodes = append(doc.Nodes, lineNodes...)
		}
	}

	return doc
}

// Document represents a parsed Micron document.
type Document struct {
	Nodes   []*Node
	Anchors map[string]int // anchor name → node index
}

// parseLine parses a single line of Micron markup.
func parseLine(line string) []*Node {
	if len(line) == 0 {
		return nil
	}

	// Check for comment
	if line[0] == '#' {
		return nil
	}

	// Check for literal toggle
	if line == "`=" {
		return nil
	}

	// Check for headings: >>>>, >>>, >>
	if line[0] == '>' {
		return parseHeading(line)
	}

	// Check for divider: --
	if line == "--" || (len(line) > 2 && line[0] == '-' && line[1] == '-' && line[2] != '-') {
		char := '\u2500'
		if len(line) == 2 {
			char = '\u2500'
		}
		return []*Node{{Type: NodeDivider, Text: string(char)}}
	}

	// Check for table start/end: `t
	if len(line) >= 2 && line[0] == '`' && line[1] == 't' {
		return nil // Tables need special handling; return nil for now
	}

	// Check for partial: `{
	if len(line) >= 2 && line[0] == '`' && line[1] == '{' {
		return parsePartial(line[2:])
	}

	// Check for section reset: < (but not field pattern)
	if line[0] == '<' {
		// Check if this is a field: <...`...>
		if !isFieldPattern(line) {
			return parseLine(line[1:])
		}
	}

	// Parse inline formatting
	return parseInline(line)
}

// parseHeading parses heading lines starting with >.
func parseHeading(line string) []*Node {
	level := 0
	for i, c := range line {
		if c == '>' {
			level = i + 1
		} else {
			break
		}
	}

	if level > 4 {
		level = 4
	}

	headingText := line[level:]
	if len(headingText) == 0 {
		return nil
	}

	children := parseInline(headingText)
	return []*Node{{
		Type:     NodeHeading,
		Level:    level,
		Children: children,
	}}
}

// parsePartial parses a partial reference line.
func parsePartial(line string) []*Node {
	endpos := -1
	for i, c := range line {
		if c == '}' {
			endpos = i
			break
		}
	}
	if endpos == -1 {
		return nil
	}

	partialData := line[:endpos]
	components := splitPartial(partialData)

	p := &Node{Type: NodePartial}
	if len(components) >= 1 {
		p.PartialURL = components[0]
	}
	if len(components) >= 2 {
		p.PartialFields = components[1]
	}
	if len(components) >= 3 {
		// Check for pid= in fields
		fields := splitPartial(components[2])
		for _, f := range fields {
			if len(f) > 4 && f[:4] == "pid=" {
				p.PartialID = f[4:]
			}
		}
	}

	if p.PartialURL != "" {
		return []*Node{p}
	}
	return nil
}

// parseInline parses inline formatting in a line of text.
func parseInline(line string) []*Node {
	var nodes []*Node
	part := ""
	i := 0

	for i < len(line) {
		c := line[i]

		if c == '\\' {
			// Escape next character
			if i+1 < len(line) {
				part += string(line[i+1])
				i += 2
				continue
			}
		}

		if c == '`' {
			// Start of formatting
			if len(part) > 0 {
				nodes = append(nodes, &Node{Type: NodeText, Text: part})
				part = ""
			}

			// Parse formatting command
			formatNodes, consumed := parseFormatting(line, i)
			nodes = append(nodes, formatNodes...)
			i += consumed
			continue
		}

		if c == '[' {
			// Start of link
			if len(part) > 0 {
				nodes = append(nodes, &Node{Type: NodeText, Text: part})
				part = ""
			}

			linkNode, consumed := parseLink(line, i)
			if linkNode != nil {
				nodes = append(nodes, linkNode)
			}
			i += consumed
			continue
		}

		if c == '<' {
			// Start of field
			if len(part) > 0 {
				nodes = append(nodes, &Node{Type: NodeText, Text: part})
				part = ""
			}

			fieldNode, consumed := parseField(line, i)
			if fieldNode != nil {
				nodes = append(nodes, fieldNode)
			}
			i += consumed
			continue
		}

		part += string(c)
		i++
	}

	if len(part) > 0 {
		nodes = append(nodes, &Node{Type: NodeText, Text: part})
	}

	return nodes
}

// parseFormatting parses a formatting command starting with `.
// Returns the nodes produced and the number of characters consumed.
func parseFormatting(line string, start int) ([]*Node, int) {
	if start+1 >= len(line) {
		return nil, 1 // consume the trailing backtick
	}

	c := line[start+1]
	consumed := 1

	switch c {
	case '!': // bold toggle
		return []*Node{{Type: NodeBold}}, 2
	case '_': // underline toggle
		return []*Node{{Type: NodeUnderline}}, 2
	case '*': // italic toggle
		return []*Node{{Type: NodeItalic}}, 2
	case 'f': // reset foreground
		return []*Node{{Type: NodeColor, FGColor: "default"}}, 2
	case 'b': // reset background
		return []*Node{{Type: NodeColor, BGColor: "default"}}, 2
	case '`': // reset all
		return []*Node{{Type: NodeReset}}, 2
	case 'c': // center align
		return []*Node{{Type: NodeAlign, Align: AlignCenter}}, 2
	case 'l': // left align
		return []*Node{{Type: NodeAlign, Align: AlignLeft}}, 2
	case 'r': // right align
		return []*Node{{Type: NodeAlign, Align: AlignRight}}, 2
	case 'a': // reset align
		return []*Node{{Type: NodeAlign, Align: AlignLeft}}, 2
	case 'F': // foreground color
		if start+4 <= len(line) {
			if start+2 < len(line) && line[start+2] == 'T' {
				// 24-bit color: `FTRRGGBB
				if start+8 <= len(line) {
					color := line[start+3 : start+9]
					return []*Node{{Type: NodeColor, FGColor: color}}, 9
				}
			}
			// 12-bit color: `FRRGGB
			if start+4 <= len(line) {
				color := line[start+2 : start+5]
				return []*Node{{Type: NodeColor, FGColor: color}}, 4
			}
		}
	case 'B': // background color
		if start+4 <= len(line) {
			if start+2 < len(line) && line[start+2] == 'T' {
				// 24-bit color: `BTRRGGBB
				if start+8 <= len(line) {
					color := line[start+3 : start+9]
					return []*Node{{Type: NodeColor, BGColor: color}}, 9
				}
			}
			// 12-bit color: `BRRGGB
			if start+4 <= len(line) {
				color := line[start+2 : start+5]
				return []*Node{{Type: NodeColor, BGColor: color}}, 4
			}
		}
	case ':': // anchor declaration
		nameEnd := start + 2
		for nameEnd < len(line) {
			ch := line[nameEnd]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
				nameEnd++
			} else {
				break
			}
		}
		name := line[start+2 : nameEnd]
		if name != "" {
			return []*Node{{Type: NodeAnchor, AnchorName: name}}, nameEnd - start
		}
	}

	return nil, consumed
}

// parseLink parses a link starting with [.
func parseLink(line string, start int) (*Node, int) {
	endpos := -1
	for i := start + 1; i < len(line); i++ {
		if line[i] == ']' {
			endpos = i
			break
		}
	}

	if endpos == -1 {
		return nil, len(line) - start
	}

	linkData := line[start+1 : endpos]
	components := splitOnChar(linkData, '`')

	link := &Node{Type: NodeLink}
	if len(components) >= 1 {
		link.LinkLabel = components[0]
	}
	if len(components) >= 2 {
		link.LinkURL = components[1]
	}
	if len(components) >= 3 {
		link.LinkFields = components[2]
	}

	if link.LinkURL == "" {
		link.LinkURL = link.LinkLabel
	}
	if link.LinkLabel == "" {
		link.LinkLabel = link.LinkURL
	}

	return link, endpos - start + 1
}

// parseField parses an input field starting with <.
func parseField(line string, start int) (*Node, int) {
	// Find the backtick
	backtickPos := -1
	for i := start + 1; i < len(line); i++ {
		if line[i] == '`' {
			backtickPos = i
			break
		}
	}

	if backtickPos == -1 {
		return nil, len(line) - start
	}

	fieldContent := line[start+1 : backtickPos]

	// Find closing >
	fieldEnd := -1
	for i := backtickPos + 1; i < len(line); i++ {
		if line[i] == '>' {
			fieldEnd = i
			break
		}
	}

	if fieldEnd == -1 {
		return nil, len(line) - start
	}

	fieldData := line[backtickPos+1 : fieldEnd]
	field := &Node{
		Type:       NodeField,
		FieldType:  "field",
		FieldWidth: 24,
		FieldData:  fieldData,
	}

	// Parse field content with pipe separators
	if contains(fieldContent, '|') {
		parts := splitPartial(fieldContent)
		flags := ""
		if len(parts) >= 1 {
			flags = parts[0]
		}
		if len(parts) >= 2 {
			field.FieldName = parts[1]
		}
		if len(parts) >= 3 {
			field.FieldData = parts[2]
		}

		// Check for type indicators
		if contains(flags, '^') {
			field.FieldType = "radio"
			flags = removeChar(flags, '^')
		} else if contains(flags, '?') {
			field.FieldType = "checkbox"
			flags = removeChar(flags, '?')
		} else if contains(flags, '!') {
			field.FieldMask = true
			flags = removeChar(flags, '!')
		}

		// Parse width from flags
		if w, ok := parseInt(flags); ok {
			field.FieldWidth = min(w, 256)
		}
	} else {
		field.FieldName = fieldContent
	}

	return field, fieldEnd - start + 1
}

// Helper functions

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

func splitPartial(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func splitOnChar(s string, sep byte) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func contains(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

func removeChar(s string, c byte) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			result = append(result, s[i])
		}
	}
	return string(result)
}

func parseInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			return 0, false
		}
	}
	return n, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isFieldPattern checks if a line starts with a field pattern: <...`...>
func isFieldPattern(line string) bool {
	if len(line) < 3 || line[0] != '<' {
		return false
	}
	// Look for backtick and closing >
	hasBacktick := false
	for i := 1; i < len(line); i++ {
		if line[i] == '`' {
			hasBacktick = true
		}
		if line[i] == '>' && hasBacktick {
			return true
		}
	}
	return false
}

// Slugify converts Micron text to a URL-friendly slug.
func Slugify(text string) string {
	if text == "" {
		return ""
	}
	// Strip Micron formatting
	stripped := stripFormatting(text)
	// Convert to slug
	var result []byte
	for _, c := range stripped {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			result = append(result, byte(c))
		} else if len(result) > 0 && result[len(result)-1] != '-' {
			result = append(result, '-')
		}
	}
	// Trim trailing dash
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return string(result)
}

// stripFormatting removes Micron formatting tags from text.
var micronStripRe = regexp.MustCompile(
	"`[FB]T[0-9a-fA-F]{6}" +
		"|`[FB][0-9a-fA-F]{3}" +
		"|`:[A-Za-z0-9_\\-]*" +
		"|`[!*_=fbacrl`<>{}]",
)

func stripFormatting(text string) string {
	stripped := micronStripRe.ReplaceAllString(text, "")
	return stripped
}
