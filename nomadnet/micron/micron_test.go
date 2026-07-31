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
	"testing"
)

func TestParsePlainText(t *testing.T) {
	t.Parallel()

	nodes := Parse("Hello world")
	if len(nodes) != 1 {
		t.Fatalf("Parse plain text len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeText {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeText)
	}
	if nodes[0].Text != "Hello world" {
		t.Errorf("node text = %q, want %q", nodes[0].Text, "Hello world")
	}
}

func TestParseEmpty(t *testing.T) {
	t.Parallel()

	nodes := Parse("")
	if len(nodes) != 0 {
		t.Errorf("Parse empty = %d nodes, want 0", len(nodes))
	}
}

func TestParseComment(t *testing.T) {
	t.Parallel()

	nodes := Parse("# This is a comment")
	if len(nodes) != 0 {
		t.Errorf("Parse comment = %d nodes, want 0", len(nodes))
	}
}

func TestParseHeading1(t *testing.T) {
	t.Parallel()

	nodes := Parse(">Heading 1")
	if len(nodes) != 1 {
		t.Fatalf("Parse heading len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeHeading {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeHeading)
	}
	if nodes[0].Level != 1 {
		t.Errorf("heading level = %d, want 1", nodes[0].Level)
	}
	if len(nodes[0].Children) == 0 {
		t.Fatal("heading has no children")
	}
	if nodes[0].Children[0].Text != "Heading 1" {
		t.Errorf("heading text = %q, want %q", nodes[0].Children[0].Text, "Heading 1")
	}
}

func TestParseHeading2(t *testing.T) {
	t.Parallel()

	nodes := Parse(">>Heading 2")
	if len(nodes) != 1 {
		t.Fatalf("Parse heading len = %d, want 1", len(nodes))
	}
	if nodes[0].Level != 2 {
		t.Errorf("heading level = %d, want 2", nodes[0].Level)
	}
}

func TestParseHeading3(t *testing.T) {
	t.Parallel()

	nodes := Parse(">>>Heading 3")
	if len(nodes) != 1 {
		t.Fatalf("Parse heading len = %d, want 1", len(nodes))
	}
	if nodes[0].Level != 3 {
		t.Errorf("heading level = %d, want 3", nodes[0].Level)
	}
}

func TestParseHeadingEmpty(t *testing.T) {
	t.Parallel()

	nodes := Parse(">")
	if len(nodes) != 0 {
		t.Errorf("Parse empty heading = %d nodes, want 0", len(nodes))
	}
}

func TestParseBold(t *testing.T) {
	t.Parallel()

	nodes := Parse("This is `!bold` text")
	if len(nodes) != 4 {
		t.Fatalf("Parse bold len = %d, want 4: %v", len(nodes), nodes)
	}
	if nodes[0].Type != NodeText || nodes[0].Text != "This is " {
		t.Errorf("node[0] = %v", nodes[0])
	}
	if nodes[1].Type != NodeBold {
		t.Errorf("node[1] type = %d, want %d", nodes[1].Type, NodeBold)
	}
	if nodes[2].Type != NodeText || nodes[2].Text != "bold" {
		t.Errorf("node[2] = %v", nodes[2])
	}
	if nodes[3].Type != NodeText || nodes[3].Text != " text" {
		t.Errorf("node[3] = %v", nodes[3])
	}
}

func TestParseUnderline(t *testing.T) {
	t.Parallel()

	nodes := Parse("`_underline_`")
	if len(nodes) < 1 {
		t.Fatalf("Parse underline len = %d, want >= 1", len(nodes))
	}
	if nodes[0].Type != NodeUnderline {
		t.Errorf("node[0] type = %d, want %d", nodes[0].Type, NodeUnderline)
	}
}

func TestParseItalic(t *testing.T) {
	t.Parallel()

	nodes := Parse("`*italic*`")
	if len(nodes) < 1 {
		t.Fatalf("Parse italic len = %d, want >= 1", len(nodes))
	}
	if nodes[0].Type != NodeItalic {
		t.Errorf("node[0] type = %d, want %d", nodes[0].Type, NodeItalic)
	}
}

func TestParseDivider(t *testing.T) {
	t.Parallel()

	nodes := Parse("--")
	if len(nodes) != 1 {
		t.Fatalf("Parse divider len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeDivider {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeDivider)
	}
}

func TestParseDividerCustomChar(t *testing.T) {
	t.Parallel()

	nodes := Parse("-=")
	if len(nodes) != 1 {
		t.Fatalf("Parse custom divider len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeDivider {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeDivider)
	}
	if nodes[0].Text != "=" {
		t.Errorf("divider char = %q, want %q", nodes[0].Text, "=")
	}
}

func TestParseDividerControlCharFallback(t *testing.T) {
	t.Parallel()

	nodes := Parse("-\x01")
	if len(nodes) != 1 {
		t.Fatalf("Parse control-char divider len = %d, want 1", len(nodes))
	}
	if nodes[0].Text != "\u2500" {
		t.Errorf("divider char = %q, want \\u2500", nodes[0].Text)
	}
}

func TestParseDividerLongLine(t *testing.T) {
	t.Parallel()

	nodes := Parse("-------")
	if len(nodes) != 1 {
		t.Fatalf("Parse long divider len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeDivider {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeDivider)
	}
	if nodes[0].Text != "\u2500" {
		t.Errorf("long divider char = %q, want \\u2500", nodes[0].Text)
	}
}

func TestParseLink(t *testing.T) {
	t.Parallel()

	nodes := Parse("`[Click here`http://example.com]")
	if len(nodes) != 1 {
		t.Fatalf("Parse link len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeLink {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeLink)
	}
	if nodes[0].LinkLabel != "Click here" {
		t.Errorf("link label = %q, want %q", nodes[0].LinkLabel, "Click here")
	}
	if nodes[0].LinkURL != "http://example.com" {
		t.Errorf("link url = %q, want %q", nodes[0].LinkURL, "http://example.com")
	}
}

func TestParseLinkWithFields(t *testing.T) {
	t.Parallel()

	nodes := Parse("`[Link`target`field1|field2]")
	if len(nodes) != 1 {
		t.Fatalf("Parse link len = %d, want 1", len(nodes))
	}
	if nodes[0].LinkFields != "field1|field2" {
		t.Errorf("link fields = %q, want %q", nodes[0].LinkFields, "field1|field2")
	}
}

func TestParseLinkURLOnly(t *testing.T) {
	t.Parallel()

	nodes := Parse("`[http://example.com]")
	if len(nodes) != 1 {
		t.Fatalf("Parse link len = %d, want 1", len(nodes))
	}
	if nodes[0].LinkLabel != "http://example.com" {
		t.Errorf("link label = %q, want %q", nodes[0].LinkLabel, "http://example.com")
	}
	if nodes[0].LinkURL != "http://example.com" {
		t.Errorf("link url = %q, want %q", nodes[0].LinkURL, "http://example.com")
	}
}

func TestParseField(t *testing.T) {
	t.Parallel()

	nodes := Parse("`<fieldname`default data>")
	if len(nodes) != 1 {
		t.Fatalf("Parse field len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeField {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeField)
	}
	if nodes[0].FieldName != "fieldname" {
		t.Errorf("field name = %q, want %q", nodes[0].FieldName, "fieldname")
	}
	if nodes[0].FieldData != "default data" {
		t.Errorf("field data = %q, want %q", nodes[0].FieldData, "default data")
	}
}

func TestParseFieldWithPipe(t *testing.T) {
	t.Parallel()

	nodes := Parse("text `<32|myfield`value>` more")
	// At least one field node should be present
	if len(nodes) < 1 {
		t.Fatalf("Parse field len = %d, want >= 1", len(nodes))
	}
	found := false
	for _, n := range nodes {
		if n.Type == NodeField {
			found = true
			if n.FieldWidth != 32 {
				t.Errorf("field width = %d, want 32", n.FieldWidth)
			}
			if n.FieldName != "myfield" {
				t.Errorf("field name = %q, want %q", n.FieldName, "myfield")
			}
		}
	}
	if !found {
		t.Error("field node not found")
	}
}

func TestParseFieldMasked(t *testing.T) {
	t.Parallel()

	nodes := Parse("`<!|secret`password>")
	if len(nodes) != 1 {
		t.Fatalf("Parse field len = %d, want 1", len(nodes))
	}
	if !nodes[0].FieldMask {
		t.Error("field mask = false, want true")
	}
}

func TestParseFieldCheckbox(t *testing.T) {
	t.Parallel()

	nodes := Parse("`<?|agree`yes`I agree>")
	if len(nodes) != 1 {
		t.Fatalf("Parse field len = %d, want 1", len(nodes))
	}
	if nodes[0].FieldType != "checkbox" {
		t.Errorf("field type = %q, want %q", nodes[0].FieldType, "checkbox")
	}
}

func TestParseFieldRadio(t *testing.T) {
	t.Parallel()

	nodes := Parse("`<^|choice`opt1`Option 1>")
	if len(nodes) != 1 {
		t.Fatalf("Parse field len = %d, want 1", len(nodes))
	}
	if nodes[0].FieldType != "radio" {
		t.Errorf("field type = %q, want %q", nodes[0].FieldType, "radio")
	}
}

func TestParseFieldIncomplete(t *testing.T) {
	t.Parallel()

	// No `< prefix: a bare "<" in text mode is a literal character, so no
	// field node is produced.
	nodes := Parse("text <fieldname`data more")
	for _, n := range nodes {
		if n.Type == NodeField {
			t.Errorf("Parse incomplete field produced a field node, want none")
		}
	}

	// < at start of line is a section reset, not a field. The rest of the
	// line is re-parsed as text (matching Python MicronParser.py:281-284).
	nodes = Parse("<fieldname>")
	for _, n := range nodes {
		if n.Type == NodeField {
			t.Errorf("Parse leading-< produced a field node, want section reset")
		}
	}
}

func TestParseColorFG(t *testing.T) {
	t.Parallel()

	nodes := Parse("`F000text`f")
	if len(nodes) > 0 {
		// Should have color and text nodes
		found := false
		for _, n := range nodes {
			if n.Type == NodeColor && n.FGColor == "000" {
				found = true
			}
		}
		if !found {
			t.Error("FG color node not found")
		}
	}
}

func TestParseColorBG(t *testing.T) {
	t.Parallel()

	nodes := Parse("`BFFftext`b")
	if len(nodes) > 0 {
		found := false
		for _, n := range nodes {
			if n.Type == NodeColor && n.BGColor == "FFf" {
				found = true
			}
		}
		if !found {
			t.Error("BG color node not found")
		}
	}
}

func TestParseReset(t *testing.T) {
	t.Parallel()

	// `` `` (two backticks) resets all formatting
	nodes := Parse("``")
	if len(nodes) != 1 {
		t.Fatalf("Parse reset len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeReset {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeReset)
	}
}

func TestParseAlignment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		align Alignment
	}{
		{"`c", AlignCenter},
		{"`l", AlignLeft},
		{"`r", AlignRight},
		{"`a", AlignLeft},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			nodes := Parse(tt.input)
			if len(nodes) != 1 {
				t.Fatalf("Parse len = %d, want 1", len(nodes))
			}
			if nodes[0].Type != NodeAlign {
				t.Errorf("type = %d, want %d", nodes[0].Type, NodeAlign)
			}
			if nodes[0].Align != tt.align {
				t.Errorf("align = %d, want %d", nodes[0].Align, tt.align)
			}
		})
	}
}

func TestParseAnchor(t *testing.T) {
	t.Parallel()

	nodes := Parse("`:my-anchor")
	if len(nodes) != 1 {
		t.Fatalf("Parse anchor len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeAnchor {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeAnchor)
	}
	if nodes[0].AnchorName != "my-anchor" {
		t.Errorf("anchor name = %q, want %q", nodes[0].AnchorName, "my-anchor")
	}
}

func TestParsePartial(t *testing.T) {
	t.Parallel()

	nodes := Parse("`{http://example.com/page}")
	if len(nodes) != 1 {
		t.Fatalf("Parse partial len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodePartial {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodePartial)
	}
	if nodes[0].PartialURL != "http://example.com/page" {
		t.Errorf("partial url = %q, want %q", nodes[0].PartialURL, "http://example.com/page")
	}
}

func TestParsePartialIncomplete(t *testing.T) {
	t.Parallel()

	nodes := Parse("`{no closing brace")
	if len(nodes) != 0 {
		t.Errorf("Parse incomplete partial = %d nodes, want 0", len(nodes))
	}
}

func TestParseEscape(t *testing.T) {
	t.Parallel()

	nodes := Parse("Hello \\`world")
	if len(nodes) != 1 {
		t.Fatalf("Parse escape len = %d, want 1", len(nodes))
	}
	if nodes[0].Text != "Hello `world" {
		t.Errorf("text = %q, want %q", nodes[0].Text, "Hello `world")
	}
}

func TestParseMultiLine(t *testing.T) {
	t.Parallel()

	markup := ">Title\nParagraph text\n--\n>>Subtitle"
	nodes := Parse(markup)

	// Should have heading, text, divider, heading
	if len(nodes) < 4 {
		t.Fatalf("Parse multi-line len = %d, want >= 4", len(nodes))
	}

	if nodes[0].Type != NodeHeading || nodes[0].Level != 1 {
		t.Errorf("node[0] = %v, want heading1", nodes[0])
	}
	if nodes[1].Type != NodeText {
		t.Errorf("node[1] = %v, want text", nodes[1])
	}
	if nodes[2].Type != NodeDivider {
		t.Errorf("node[2] = %v, want divider", nodes[2])
	}
	if nodes[3].Type != NodeHeading || nodes[3].Level != 2 {
		t.Errorf("node[3] = %v, want heading2", nodes[3])
	}
}

func TestParseMixedFormatting(t *testing.T) {
	t.Parallel()

	nodes := Parse("Hello `!bold` and `*italic*` world")
	if len(nodes) < 4 {
		t.Fatalf("Parse mixed len = %d, want >= 4", len(nodes))
	}

	// Check for bold and italic nodes
	foundBold := false
	foundItalic := false
	for _, n := range nodes {
		if n.Type == NodeBold {
			foundBold = true
		}
		if n.Type == NodeItalic {
			foundItalic = true
		}
	}
	if !foundBold {
		t.Error("bold node not found")
	}
	if !foundItalic {
		t.Error("italic node not found")
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input, want string
	}{
		{"Hello World", "hello-world"},
		{"This is a Test", "this-is-a-test"},
		{"Special!@#Chars", "special-chars"},
		{"", ""},
		{"`!bold` text", "bold-text"},
		{"Mixed CASE", "mixed-case"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripFormatting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input, want string
	}{
		{"Hello World", "Hello World"},
		{"`!bold`", "bold`"},
		{"`F000red`f", "red"},
		{"`*italic*`", "italic*`"},
		{"`_underline_`", "underline_`"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := stripFormatting(tt.input)
			if got != tt.want {
				t.Errorf("stripFormatting(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNodeTypes(t *testing.T) {
	t.Parallel()

	// Verify all node type constants exist and are distinct
	types := map[NodeType]bool{
		NodeText:      true,
		NodeBold:      true,
		NodeUnderline: true,
		NodeItalic:    true,
		NodeHeading:   true,
		NodeLink:      true,
		NodeField:     true,
		NodeDivider:   true,
		NodePartial:   true,
		NodeColor:     true,
		NodeReset:     true,
		NodeAlign:     true,
		NodeAnchor:    true,
	}

	if len(types) != 13 {
		t.Errorf("NodeTypes count = %d, want 13", len(types))
	}
}

func TestParseDocument(t *testing.T) {
	t.Parallel()

	markup := ">Title\nContent here\n>>Subtitle\nMore content"
	doc := ParseDocument(markup)

	if doc == nil {
		t.Fatal("ParseDocument returned nil")
	}
	if len(doc.Nodes) < 4 {
		t.Errorf("Document.Nodes len = %d, want >= 4", len(doc.Nodes))
	}
}

func TestParseHeadingWithInline(t *testing.T) {
	t.Parallel()

	nodes := Parse(">Title `!with bold`")
	if len(nodes) != 1 {
		t.Fatalf("Parse heading len = %d, want 1", len(nodes))
	}
	if nodes[0].Type != NodeHeading {
		t.Errorf("node type = %d, want %d", nodes[0].Type, NodeHeading)
	}
	if len(nodes[0].Children) < 2 {
		t.Fatalf("heading children len = %d, want >= 2", len(nodes[0].Children))
	}
}
