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
	"strconv"
	"strings"
)

// Theme selects the Micron color palette, mirroring Python's STYLES_DARK vs
// STYLES_LIGHT selection (MicronParser.py:18-30, 88-92).
type Theme int

const (
	// ThemeDark is the default dark palette (DEFAULT_FG_DARK="ddd").
	ThemeDark Theme = iota
	// ThemeLight is the light palette (DEFAULT_FG_LIGHT="222").
	ThemeLight
)

// styleEntry is a base style from the Python STYLES_DARK/STYLES_LIGHT tables
// (MicronParser.py:18-30): raw micron fg/bg codes plus the formatting flags.
type styleEntry struct {
	FG, BG                  string
	Bold, Underline, Italic bool
}

var stylesDark = map[string]styleEntry{
	"plain":    {"ddd", "default", false, false, false},
	"heading1": {"222", "bbb", false, false, false},
	"heading2": {"111", "999", false, false, false},
	"heading3": {"000", "777", false, false, false},
}

var stylesLight = map[string]styleEntry{
	"plain":    {"222", "default", false, false, false},
	"heading1": {"000", "777", false, false, false},
	"heading2": {"111", "aaa", false, false, false},
	"heading3": {"222", "ccc", false, false, false},
}

// themeStyle returns the named style entry for theme, falling back to plain.
func themeStyle(theme Theme, name string) styleEntry {
	table := stylesDark
	if theme == ThemeLight {
		table = stylesLight
	}
	if e, ok := table[name]; ok {
		return e
	}
	return table["plain"]
}

// LinkSpec describes a clickable link span, mirroring Python's LinkableText /
// LinkSpec (MicronParser.py:855+). It carries the label text, target URL, and
// optional pipe-separated form fields.
type LinkSpec struct {
	Label  string
	URL    string
	Fields string
}

// FieldSpec describes an input-field span, mirroring Python's ReadlineEdit /
// CheckBox / RadioButton field dicts (MicronParser.py:740-757).
type FieldSpec struct {
	Name       string
	Type       string // "field", "checkbox", "radio"
	Data       string // text fields: initial text; checkbox/radio: label
	Value      string // checkbox/radio: selected value (field_value or label)
	Width      int
	Masked     bool
	Prechecked bool
}

// StyledSpan is one styled text run within a rendered Micron line. FG/BG are
// high-color specs ("#RRGGBB", "default", or "gNN") — the 24-bit-depth values
// produced by Python make_style's high_color (MicronParser.py:518-567).
type StyledSpan struct {
	Text      string
	FG        string
	BG        string
	Bold      bool
	Underline bool
	Italic    bool
	Link      *LinkSpec
	Field     *FieldSpec
}

// StyledLine is one rendered line: a sequence of styled spans plus layout
// metadata. It is the Go equivalent of one urwid.Text row in Python's
// markup_to_attrmaps output (MicronParser.py:137-139).
type StyledLine struct {
	Spans        []StyledSpan
	Align        Alignment
	Indent       int    // left indent in chars (section depth-1)*2
	HeadingLevel int    // 0 = not a heading; else the heading level
	Divider      bool   // true for a horizontal-divider line
	DividerChar  string // divider character (default U+2500)
	DividerRight int    // right pad for a divider at depth>0 (Python right_indent)
	Anchor       string // anchor name bound to this line (heading slug or `:name)
}

// renderState tracks the inline formatting state across lines, mirroring
// Python's state dict (fg_color/bg_color/formatting/align) in
// markup_to_attrmaps (MicronParser.py:39-68). It stores RAW micron color codes;
// spans convert via highColor at emit time, matching make_style.
type renderState struct {
	bold, underline, italic bool
	fg, bg                  string // raw micron color codes
	align                   Alignment
	defaultFG               string // plain fg (for `f reset)
	defaultBG               string // plain bg (for `b reset)
}

func newRenderState(theme Theme) *renderState {
	plain := themeStyle(theme, "plain")
	return &renderState{
		fg:        plain.FG,
		bg:        plain.BG,
		align:     AlignLeft,
		defaultFG: plain.FG,
		defaultBG: plain.BG,
	}
}

func (rs *renderState) snapshot() renderState { return *rs }
func (rs *renderState) restore(s renderState) { *rs = s }

// RenderToStyledLines parses Micron markup and renders it to a list of styled
// lines, each a sequence of styled spans with full fg/bg — the Go equivalent of
// Python MicronParser.markup_to_attrmaps (MicronParser.py:94-147) at the 24-bit
// color depth. Body text uses the plain style (NOT bold); headings use the
// headingN fg/bg from the theme table. Inline formatting toggles and section
// depth persist across lines, matching Python's shared state.
func RenderToStyledLines(markup string, theme Theme) []*StyledLine {
	lines := splitLines(markup)
	ps := &parseState{}
	rs := newRenderState(theme)
	plain := themeStyle(theme, "plain")
	blank := StyledSpan{Text: "", FG: highColor(plain.FG), BG: highColor(plain.BG)}

	var out []*StyledLine
	for _, line := range lines {
		if line == "" {
			out = append(out, &StyledLine{Spans: []StyledSpan{blank}})
			continue
		}
		nodes := parseLine(line, ps)
		for _, sl := range renderLineNodes(nodes, rs, theme, ps.depth) {
			out = append(out, sl)
		}
	}
	return out
}

// renderLineNodes renders one line's parsed nodes into zero or more StyledLines
// (tables expand to one line per row). Returns nil for lines that produce no
// visible output (literal toggle, comment — parseLine returns nil for those, so
// this is usually a no-op, but a lone NodeLiteral is also suppressed).
func renderLineNodes(nodes []*Node, rs *renderState, theme Theme, depth int) []*StyledLine {
	if len(nodes) == 0 {
		return nil
	}
	// A lone literal-toggle marker produces no visible line.
	if len(nodes) == 1 && nodes[0].Type == NodeLiteral {
		return nil
	}

	var out []*StyledLine
	sl := &StyledLine{Indent: leftIndent(depth)}

	hasLiteral := false
	for _, node := range nodes {
		switch node.Type {
		case NodeHeading:
			renderHeadingLine(sl, node, rs, theme)
			out = append(out, sl)
			sl = &StyledLine{Indent: leftIndent(depth)}
		case NodeDivider:
			sl.Divider = true
			sl.DividerChar = node.Text
			// Python MicronParser.py:334-336: depth 0 → urwid.Divider fills the
			// full width; depth>0 → Padding(Divider, left=left_indent,
			// right=right_indent) pads both sides by (depth-1)*SECTION_INDENT.
			ind := leftIndent(node.Depth)
			sl.Indent = ind
			sl.DividerRight = ind // right_indent == left_indent
			out = append(out, sl)
			sl = &StyledLine{Indent: leftIndent(depth)}
		case NodeTable:
			for _, row := range renderTableLines(node, rs, theme, depth) {
				out = append(out, row)
			}
		case NodePartial:
			sl.Spans = append(sl.Spans, StyledSpan{
				Text: partialPlaceholder, // ⧖ hourglass (Python parse_partial)
				FG:   highColor(rs.fg), BG: highColor(rs.bg),
				Bold: rs.bold, Underline: rs.underline, Italic: rs.italic,
			})
		case NodeLiteral:
			hasLiteral = true
		default:
			renderInlineNode(sl, node, rs)
		}
	}

	// Flush a trailing line that accumulated inline content.
	if len(sl.Spans) > 0 {
		sl.Align = rs.align
		out = append(out, sl)
	}

	// A line that was only a literal toggle (no content) is suppressed.
	if hasLiteral && len(out) == 0 {
		return nil
	}
	return out
}

// renderInlineNode walks a single inline node, mutating the render state and
// appending styled spans to the line. Mirrors Python make_output's per-character
// state machine (MicronParser.py:593-855) operating on the already-parsed AST.
func renderInlineNode(sl *StyledLine, node *Node, rs *renderState) {
	switch node.Type {
	case NodeText:
		sl.Spans = append(sl.Spans, StyledSpan{
			Text:      node.Text,
			FG:        highColor(rs.fg),
			BG:        highColor(rs.bg),
			Bold:      rs.bold,
			Underline: rs.underline,
			Italic:    rs.italic,
		})

	case NodeBold:
		rs.bold = !rs.bold

	case NodeUnderline:
		rs.underline = !rs.underline

	case NodeItalic:
		rs.italic = !rs.italic

	case NodeColor:
		if node.FGColor != "" {
			if node.FGColor == "default" {
				rs.fg = rs.defaultFG
			} else {
				rs.fg = node.FGColor
			}
		}
		if node.BGColor != "" {
			if node.BGColor == "default" {
				rs.bg = rs.defaultBG
			} else {
				rs.bg = node.BGColor
			}
		}

	case NodeReset:
		rs.bold = false
		rs.underline = false
		rs.italic = false
		rs.fg = rs.defaultFG
		rs.bg = rs.defaultBG
		rs.align = AlignLeft

	case NodeAlign:
		rs.align = node.Align

	case NodeAnchor:
		if sl.Anchor == "" {
			sl.Anchor = node.AnchorName
		}

	case NodeLink:
		sl.Spans = append(sl.Spans, StyledSpan{
			Text:      node.LinkLabel,
			FG:        highColor(rs.fg),
			BG:        highColor(rs.bg),
			Bold:      rs.bold,
			Underline: rs.underline,
			Italic:    rs.italic,
			Link:      &LinkSpec{Label: node.LinkLabel, URL: node.LinkURL, Fields: node.LinkFields},
		})

	case NodeField:
		fs := &FieldSpec{
			Name:       node.FieldName,
			Type:       node.FieldType,
			Data:       node.FieldData,
			Value:      node.FieldValue,
			Width:      node.FieldWidth,
			Masked:     node.FieldMask,
			Prechecked: node.FieldPrechecked,
		}
		sl.Spans = append(sl.Spans, StyledSpan{
			Text:      node.FieldData,
			FG:        highColor(rs.fg),
			BG:        highColor(rs.bg),
			Bold:      rs.bold,
			Underline: rs.underline,
			Italic:    rs.italic,
			Field:     fs,
		})
	}
}

// renderHeadingLine renders a heading node into the given line, applying the
// headingN style (fg/bg, NOT bold) and restoring the prior state afterward —
// mirroring Python parse_line's latched_style save/restore
// (MicronParser.py:300-306). The heading slug is bound as the line anchor.
func renderHeadingLine(sl *StyledLine, node *Node, rs *renderState, theme Theme) {
	level := node.Level
	styleName := "heading" + strconv.Itoa(level)
	if level > 3 {
		styleName = "heading3" // Python: only heading1-3 exist; deeper → heading3
	}
	style := themeStyle(theme, styleName)

	latched := rs.snapshot()
	rs.fg = style.FG
	rs.bg = style.BG
	rs.bold = style.Bold
	rs.underline = style.Underline
	rs.italic = style.Italic

	for _, child := range node.Children {
		renderInlineNode(sl, child, rs)
	}
	rs.restore(latched)

	sl.HeadingLevel = level
	sl.Indent = leftIndent(level)
	sl.Anchor = Slugify(headingText(node))
}

// headingText returns the concatenated text of a heading's inline children.
func headingText(node *Node) string {
	var sb strings.Builder
	for _, c := range node.Children {
		if c.Type == NodeText {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}

// renderTableLines renders a table node by delegating to FormatTableRaw for the
// box-drawing layout, then re-parsing each formatted micron line through
// parseLine/renderLineNodes so the borders, `c/`a align tags, and inline cell
// formatting (colors/bold) become properly styled spans — mirroring Python
// render_table (MicronParser.py:197-218). Python applies no special header
// bolding; the formatted lines carry their own formatting tags.
func renderTableLines(node *Node, rs *renderState, theme Theme, depth int) []*StyledLine {
	if len(node.TableRawLines) == 0 {
		return nil
	}
	alignStr := ""
	if node.TableHasAlign {
		switch node.TableAlign {
		case AlignCenter:
			alignStr = "c"
		case AlignRight:
			alignStr = "r"
		default:
			alignStr = "l"
		}
	}
	formatted := FormatTableRaw(node.TableRawLines, alignStr, node.TableMaxWidth)

	// Re-parse each formatted micron line with table mode off (mirroring
	// Python's state['table_mode']=False around the recursive parse_line) and
	// render into the SHARED render state so align/color changes propagate.
	tps := &parseState{depth: depth}
	var out []*StyledLine
	for _, line := range formatted {
		nodes := parseLine(line, tps)
		out = append(out, renderLineNodes(nodes, rs, theme, depth)...)
	}
	return out
}

// leftIndent returns the left indent for a section depth, matching Python
// left_indent (MicronParser.py:418-419): (depth-1)*SECTION_INDENT.
func leftIndent(depth int) int {
	if depth <= 1 {
		return 0
	}
	return (depth - 1) * 2
}
