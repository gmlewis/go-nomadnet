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
	"strconv"
	"strings"
	"unicode/utf8"
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
	// NodeLiteral toggles literal (preformatted) mode.
	NodeLiteral
	// NodeTable is a formatted table with rows and columns.
	NodeTable
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

	// Depth is the section depth (number of >) that applies to this node.
	// Content after a heading inherits the heading's depth until reset.
	Depth int

	// For NodeLink
	LinkURL    string
	LinkLabel  string
	LinkFields string

	// For NodeField
	FieldName       string
	FieldType       string // "field", "checkbox", "radio"
	FieldWidth      int
	FieldData       string // text fields: initial text; checkbox/radio: label
	FieldMask       bool
	FieldValue      string // checkbox/radio: selected value (field_value or label)
	FieldPrechecked bool   // checkbox/radio: pre-checked ("*" flag)

	// For NodeColor
	FGColor string
	BGColor string

	// For NodeAlign
	Align Alignment

	// For NodePartial
	PartialURL        string
	PartialFields     []string
	PartialID         string
	PartialRefresh    float64
	HasRefresh        bool
	PartialDescriptor string // "|"-joined raw components; the hash input (MicronParser.py:187)
	PartialRaw        string // the full directive as it appeared in markup, "`{...}"

	// For NodeTable
	TableRows     [][]string // parsed cells (header + data rows; separator row skipped)
	TableRawLines []string   // raw buffered markdown table lines (header/sep/data)
	TableAlign    Alignment  // table-level alignment (l/c/r)
	TableHasAlign bool       // true when the `t tag specified an alignment (else Python align=None)
	TableMaxWidth int        // max table width in chars

	// For NodeAnchor
	AnchorName string

	// Children for composite nodes
	Children []*Node
}

// MaxTableWidth is the default maximum width for Micron tables.
const MaxTableWidth = 100

// parseState holds cross-line parsing state for stateful features like
// literal mode and table mode.
type parseState struct {
	literal       bool
	depth         int
	tableMode     bool
	tableBuf      []string
	tableAlign    Alignment
	tableHasAlign bool
	tableMaxW     int
}

// Parse parses Micron markup text and returns a list of top-level nodes.
func Parse(markup string) []*Node {
	lines := splitLines(markup)
	var nodes []*Node
	state := &parseState{}

	for _, line := range lines {
		if lineNodes := parseLine(line, state); len(lineNodes) > 0 {
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
	state := &parseState{}

	for _, line := range lines {
		if lineNodes := parseLine(line, state); len(lineNodes) > 0 {
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
func parseLine(line string, state *parseState) []*Node {
	if len(line) == 0 {
		return nil
	}

	// Check for literal toggle
	if line == "`=" {
		state.literal = !state.literal
		return []*Node{{Type: NodeLiteral, Depth: state.depth}}
	}

	// In literal mode, output the line as-is (no inline formatting)
	if state.literal {
		// Handle escaped literal toggle: \`= → `=
		escaped := strings.ReplaceAll(line, "\\`=", "`=")
		return []*Node{{Type: NodeText, Text: escaped, Depth: state.depth}}
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
		// Heading lines containing a field (`<) lose their heading status:
		// the leading ">"s are stripped and the line is reclassified,
		// matching Python MicronParser.parse_line (MicronParser.py:233-236).
		if strings.Contains(line, "`<") {
			return parseLine(strings.TrimLeft(line, ">"), state)
		}
		return parseHeading(line, state)
	}

	// Check for horizontal dividers: -X (custom char) or longer (default)
	if line[0] == '-' {
		char := rune('\u2500')
		// Python's `len(line) == 2` / `line[1]` operate on CHARACTERS
		// (codepoints), not bytes \u2014 markup.mu uses "-\u223f" (U+223F SINE WAVE,
		// 2 UTF-8 bytes) as its section divider. Use rune count so a multibyte
		// custom char is recognized; fall through to the default \u2500 otherwise.
		if utf8.RuneCountInString(line) == 2 {
			r, _ := utf8.DecodeRuneInString(line[1:])
			char = r
			if char < 32 {
				char = '\u2500'
			}
		}
		return []*Node{{Type: NodeDivider, Text: string(char), Depth: state.depth}}
	}

	// Check for table start/end: `t
	if len(line) >= 2 && line[0] == '`' && line[1] == 't' {
		if state.tableMode {
			rawLines := state.tableBuf
			savedAlign := state.tableAlign
			savedHasAlign := state.tableHasAlign
			savedMaxW := state.tableMaxW
			state.tableMode = false
			state.tableBuf = nil
			state.tableAlign = AlignLeft
			state.tableHasAlign = false
			state.tableMaxW = MaxTableWidth
			// Python render_table: `if len(lines) < 2: return None`.
			if len(rawLines) < 2 {
				return nil
			}
			return []*Node{{
				Type:          NodeTable,
				TableRows:     parseTableRows(rawLines),
				TableRawLines: rawLines,
				TableAlign:    savedAlign,
				TableHasAlign: savedHasAlign,
				TableMaxWidth: savedMaxW,
				Depth:         state.depth,
			}}
		}

		rest := line[2:]
		align := AlignLeft
		hasAlign := false
		maxWidth := MaxTableWidth
		if len(rest) > 0 && (rest[0] == 'l' || rest[0] == 'c' || rest[0] == 'r') {
			hasAlign = true
			switch rest[0] {
			case 'c':
				align = AlignCenter
			case 'r':
				align = AlignRight
			}
			rest = rest[1:]
		}
		if len(rest) > 0 {
			if w, err := strconv.Atoi(rest); err == nil && w > 0 {
				maxWidth = w
			}
		}

		state.tableMode = true
		state.tableBuf = nil
		state.tableAlign = align
		state.tableHasAlign = hasAlign
		state.tableMaxW = maxWidth
		return nil
	}

	// Buffer lines while in table mode
	if state.tableMode {
		state.tableBuf = append(state.tableBuf, line)
		return nil
	}

	// Check for partial: `{
	if len(line) >= 2 && line[0] == '`' && line[1] == '{' {
		return parsePartial(line[2:])
	}

	// Check for section reset: < resets section depth, then re-parses
	// the rest of the line. Matches Python MicronParser.parse_line
	// (MicronParser.py:281-284): a leading "<" always resets depth.
	if line[0] == '<' {
		state.depth = 0
		return parseLine(line[1:], state)
	}

	// Parse inline formatting
	nodes := parseInline(line)
	for _, n := range nodes {
		n.Depth = state.depth
	}
	return nodes
}

// parseHeading parses heading lines starting with >. The level is the count
// of leading ">" characters and is not clamped, matching Python
// MicronParser.parse_line (MicronParser.py:288-298): heading styles only
// exist for levels 1-3, but the depth (and thus indent) grows unbounded.
func parseHeading(line string, state *parseState) []*Node {
	level := 0
	for i, c := range line {
		if c == '>' {
			level = i + 1
		} else {
			break
		}
	}

	state.depth = level

	headingText := line[level:]
	if len(headingText) == 0 {
		return nil
	}

	children := parseInline(headingText)
	return []*Node{{
		Type:     NodeHeading,
		Level:    level,
		Depth:    level,
		Children: children,
	}}
}

// parseTableRows splits buffered markdown table lines into rows of cells,
// skipping the alignment separator row (| :--: | --: |). Mirrors the row
// parsing in MarkdownToMicron.format_table_raw (util.py): rows[0] is the
// header, rows[1] is the separator (skipped here), and the rest are data.
func parseTableRows(lines []string) [][]string {
	var rows [][]string
	for _, line := range lines {
		if line == "" {
			continue
		}
		cells := parseTableRow(line)
		if isTableSeparator(cells) {
			continue // alignment separator row
		}
		rows = append(rows, cells)
	}
	return rows
}

// tableSeparatorCellRe matches a single separator cell: optional leading/trailing
// ":" with one or more "-" between (MarkdownToMicron._is_table_separator).
var tableSeparatorCellRe = regexp.MustCompile(`^:?-+:?$`)

// isTableSeparator reports whether cells form a markdown table separator row.
func isTableSeparator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if !tableSeparatorCellRe.MatchString(strings.TrimSpace(c)) {
			return false
		}
	}
	return true
}

// parsePartial parses a partial reference line.
// Matches Python MicronParser.parse_partial (MicronParser.py:149-195):
// components are split on the backtick char, the second component is a
// float refresh interval, and the third is a pipe-separated field list that
// may carry a pid= entry. Any parse failure (missing brace, non-numeric
// refresh) yields no node, mirroring the Python try/except → None.
func parsePartial(line string) []*Node {
	before, _, ok := strings.Cut(line, "}")
	if !ok {
		return nil
	}

	partialData := before
	components := strings.Split(partialData, "`")

	p := &Node{Type: NodePartial, PartialFields: []string{""}}
	// The full directive as it appeared in the source markup, "`{<partialData>}".
	// Used by the browser to substitute the rendered partial content into the
	// page on refresh (Python replaces the partial's urwid Pile slot instead).
	p.PartialRaw = "`{" + partialData + "}"
	// partial_descriptor = "|".join(partial_components) — the hash input
	// (MicronParser.py:187). Built from the raw components before field parsing.
	p.PartialDescriptor = strings.Join(components, "|")

	switch len(components) {
	case 1:
		p.PartialURL = components[0]
	case 2:
		p.PartialURL = components[0]
		refresh, err := strconv.ParseFloat(components[1], 64)
		if err != nil {
			return nil
		}
		p.PartialRefresh = refresh
		p.HasRefresh = true
	case 3:
		p.PartialURL = components[0]
		refresh, err := strconv.ParseFloat(components[1], 64)
		if err != nil {
			return nil
		}
		p.PartialRefresh = refresh
		p.HasRefresh = true
		p.PartialFields = strings.Split(components[2], "|")
		for _, f := range p.PartialFields {
			if strings.HasPrefix(f, "pid=") {
				p.PartialID = f[4:]
			}
		}
	default:
		// Python's else branch: empty url, empty fields, no refresh.
	}

	// Python: refresh < 1 is treated as no refresh.
	if p.HasRefresh && p.PartialRefresh < 1 {
		p.HasRefresh = false
		p.PartialRefresh = 0
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
			// Escape next character: the backslash and the following char
			// are consumed and the char is taken literally. A trailing
			// backslash with no following char contributes nothing,
			// matching Python's escape flag (MicronParser.py:829-835). The
			// escaped char may be multibyte (e.g. "\✓"); decode the whole rune
			// rather than taking a single byte (string(byte) mangles UTF-8).
			if i+1 < len(line) {
				if line[i+1] < utf8.RuneSelf {
					part += string(line[i+1])
					i += 2
				} else {
					r, size := utf8.DecodeRuneInString(line[i+1:])
					part += string(r)
					i += 1 + size
				}
				continue
			}
			i++ // drop trailing backslash
			continue
		}

		if c == '`' {
			// Start of formatting. In Micron, links (`[) and fields (`<)
			// are entered from formatting mode, never via bare brackets:
			// bare "[" / "<" in text mode are literal characters, matching
			// Python MicronParser.make_output (MicronParser.py:605-846).
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

		// ASCII bytes round-trip through string(byte) unchanged, but a
		// multibyte rune (e.g. "✓" = e2 9c 93) must be decoded whole: string(byte(0xe2))
		// yields "â" (U+00E2), and the continuation bytes 0x9c/0x93 become C1
		// controls that vanish on output — so each glyph collapsed to "â" in the
		// Go cast. Decode the rune and append it (and any RuneError as U+FFFD).
		if c < utf8.RuneSelf {
			part += string(c)
			i++
		} else {
			r, size := utf8.DecodeRuneInString(line[i:])
			part += string(r)
			i += size
		}
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
		// Python make_output (MicronParser.py:617-626): the guard is
		// len(line) >= i+4 where i is the index of 'F' (start+1 here), i.e.
		// start+5. The 24-bit `FTRRGGBB form is taken only when 'T' is present
		// AND there are 6 trailing digits (start+9); otherwise it falls back
		// to the 12-bit `FRRGGB form. consumed covers backtick+F+3 digits = 5
		// (Python skip=3 from 'F'); the prior value of 4 left the last digit
		// in the following text.
		if start+5 <= len(line) {
			if line[start+2] == 'T' && start+9 <= len(line) {
				color := line[start+3 : start+9]
				return []*Node{{Type: NodeColor, FGColor: color}}, 9
			}
			color := line[start+2 : start+5]
			return []*Node{{Type: NodeColor, FGColor: color}}, 5
		}
	case 'B': // background color
		if start+5 <= len(line) {
			if line[start+2] == 'T' && start+9 <= len(line) {
				color := line[start+3 : start+9]
				return []*Node{{Type: NodeColor, BGColor: color}}, 9
			}
			color := line[start+2 : start+5]
			return []*Node{{Type: NodeColor, BGColor: color}}, 5
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
	case '<': // input field: `<...`...>
		field, fc := parseField(line, start+1)
		if field != nil {
			return []*Node{field}, 1 + fc
		}
		// Invalid field: parseField reports how far it got (just the
		// bracket, or through a closing > if present); the rest of the
		// line is treated as text, matching Python's `pass` + revert to
		// text mode (MicronParser.py:676,733).
		return nil, 1 + fc
	case '[': // link: `[label`url`fields]
		link, lc := parseLink(line, start+1)
		if link != nil {
			return []*Node{link}, 1 + lc
		}
		// Invalid link: parseLink reports how far it got (just the
		// bracket, or through a closing ] if present); the rest is text.
		return nil, 1 + lc
	}

	return nil, consumed
}

// parseLink parses a link starting with [. The bracket is at `start`.
// Matches Python MicronParser.make_output's "[" branch
// (MicronParser.py:763-822): link_data is split on the backtick char into
// 1 (url only), 2 (label+url), or 3 (label+url+fields) components; any
// other count yields an empty url and no link is emitted. A link is only
// emitted when link_url is non-empty; an empty label falls back to the url.
func parseLink(line string, start int) (*Node, int) {
	endpos := -1
	for i := start + 1; i < len(line); i++ {
		if line[i] == ']' {
			endpos = i
			break
		}
	}

	if endpos == -1 {
		// No closing ]: consume just the bracket; rest of line is text.
		return nil, 1
	}

	linkData := line[start+1 : endpos]
	components := splitOnChar(linkData, '`')

	link := &Node{Type: NodeLink}
	switch len(components) {
	case 1:
		link.LinkURL = components[0]
	case 2:
		link.LinkLabel = components[0]
		link.LinkURL = components[1]
	case 3:
		link.LinkLabel = components[0]
		link.LinkURL = components[1]
		link.LinkFields = components[2]
	default:
		// Python: 4+ components → empty url/label/fields, no link emitted.
		return nil, endpos - start + 1
	}

	if link.LinkURL == "" {
		// Python only appends a link when link_url is non-empty.
		return nil, endpos - start + 1
	}
	if link.LinkLabel == "" {
		link.LinkLabel = link.LinkURL
	}

	return link, endpos - start + 1
}

// parseField parses an input field starting with <. The bracket is at
// `start`. Matches Python MicronParser.make_output's "<" branch
// (MicronParser.py:669-758): the content between "<" and the first inner
// backtick is pipe-separated into [flags, name, value, check]; the text
// after the inner backtick up to the closing ">" is field_data.
//
// For text fields, field_data is the initial text and width/masked apply;
// value and prechecked are not emitted. For checkboxes/radios, field_data
// is the label, the value is field_value (or the label when unset), the
// prechecked flag comes from a "*" 4th component, and width is not emitted.
func parseField(line string, start int) (*Node, int) {
	// Find the inner backtick separating field content from field data.
	backtickPos := -1
	for i := start + 1; i < len(line); i++ {
		if line[i] == '`' {
			backtickPos = i
			break
		}
	}

	if backtickPos == -1 {
		// No inner backtick: invalid field, consume just the bracket.
		return nil, 1
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
		// No closing >: invalid field, consume just the bracket.
		return nil, 1
	}

	fieldData := line[backtickPos+1 : fieldEnd]
	field := &Node{
		Type:       NodeField,
		FieldType:  "field",
		FieldWidth: 24,
		FieldData:  fieldData,
	}

	fieldValue := ""
	fieldPrechecked := false

	// Parse field content with pipe separators.
	if contains(fieldContent, '|') {
		parts := splitPartial(fieldContent)
		flags := parts[0]
		if len(parts) >= 2 {
			field.FieldName = parts[1]
		}

		// Type indicators (Python elif order: ^, then ?, then !).
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

		// Parse width from remaining flags.
		if w, ok := parseInt(flags); ok {
			field.FieldWidth = min(w, 256)
		}

		if len(parts) >= 3 {
			fieldValue = parts[2]
		}
		if len(parts) >= 4 && parts[3] == "*" {
			fieldPrechecked = true
		}
	} else {
		field.FieldName = fieldContent
	}

	// Checkboxes and radios emit value/label/prechecked but no width.
	if field.FieldType == "checkbox" || field.FieldType == "radio" {
		field.FieldWidth = 0
		field.FieldData = fieldData // label
		if fieldValue != "" {
			field.FieldValue = fieldValue
		} else {
			field.FieldValue = fieldData
		}
		field.FieldPrechecked = fieldPrechecked
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
