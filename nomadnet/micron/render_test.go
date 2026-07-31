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
	"strings"
	"testing"
)

func TestRenderToTViewPlainText(t *testing.T) {
	t.Parallel()

	nodes := Parse("Hello world")
	got := RenderToTView(nodes)
	if got != "Hello world" {
		t.Errorf("RenderToTView plain text = %q, want %q", got, "Hello world")
	}
}

func TestRenderToTViewBold(t *testing.T) {
	t.Parallel()

	nodes := Parse("Normal `!bold`` normal")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "[::b]") {
		t.Errorf("RenderToTView bold missing [::b] tag: %q", got)
	}
	if !strings.Contains(got, "[-:-:-]") {
		t.Errorf("RenderToTView bold missing reset tag: %q", got)
	}
}

func TestRenderToTViewUnderline(t *testing.T) {
	t.Parallel()

	nodes := Parse("`_underlined`")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "[::u]") {
		t.Errorf("RenderToTView underline missing [::u] tag: %q", got)
	}
}

func TestRenderToTViewItalic(t *testing.T) {
	t.Parallel()

	nodes := Parse("`*italic`")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "[::I]") {
		t.Errorf("RenderToTView italic missing [::I] tag: %q", got)
	}
}

func TestRenderToTViewHeading1(t *testing.T) {
	t.Parallel()

	nodes := Parse(">Heading One")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "Heading One") {
		t.Errorf("RenderToTView heading1 missing text: %q", got)
	}
	if !strings.Contains(got, "#000000:#bbbbbb") {
		t.Errorf("RenderToTView heading1 missing dark style: %q", got)
	}
}

func TestRenderToTViewHeading2(t *testing.T) {
	t.Parallel()

	nodes := Parse(">>Heading Two")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "#111111:#999999") {
		t.Errorf("RenderToTView heading2 missing style: %q", got)
	}
}

func TestRenderToTViewHeading3(t *testing.T) {
	t.Parallel()

	nodes := Parse(">>>Heading Three")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "#222222:#777777") {
		t.Errorf("RenderToTView heading3 missing style: %q", got)
	}
}

func TestRenderToTViewDivider(t *testing.T) {
	t.Parallel()

	nodes := Parse("--")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "-") {
		t.Errorf("RenderToTView divider missing dash char: %q", got)
	}
}

func TestRenderToTViewDividerCustomChar(t *testing.T) {
	t.Parallel()

	nodes := Parse("-=")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "=") {
		t.Errorf("RenderToTView custom divider missing =: %q", got)
	}
	if strings.Contains(got, "\u2500") {
		t.Errorf("RenderToTView custom divider should not have default bar: %q", got)
	}
}

func TestRenderToTViewLink(t *testing.T) {
	t.Parallel()

	nodes := Parse("`[Click`http://example.com]")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "Click") {
		t.Errorf("RenderToTView link missing label: %q", got)
	}
	if !strings.Contains(got, "http://example.com") {
		t.Errorf("RenderToTView link missing URL: %q", got)
	}
}

func TestRenderToTViewColorFG(t *testing.T) {
	t.Parallel()

	nodes := Parse("`Ff00text`f")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "[#ff0000]") {
		t.Errorf("RenderToTView FG color missing tag: %q", got)
	}
}

func TestRenderToTViewReset(t *testing.T) {
	t.Parallel()

	nodes := Parse("`!bold``")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "[-:-:-]") {
		t.Errorf("RenderToTView reset missing tag: %q", got)
	}
}

func TestRenderToTViewMultiLine(t *testing.T) {
	t.Parallel()

	markup := ">Title\nParagraph\n--\n>>Subtitle"
	nodes := Parse(markup)
	got := RenderToTView(nodes)

	if !strings.Contains(got, "Title") {
		t.Error("multi-line render missing Title")
	}
	if !strings.Contains(got, "Paragraph") {
		t.Error("multi-line render missing Paragraph")
	}
	if !strings.Contains(got, "-") {
		t.Error("multi-line render missing divider")
	}
	if !strings.Contains(got, "Subtitle") {
		t.Error("multi-line render missing Subtitle")
	}
}

func TestRenderToPlainText(t *testing.T) {
	t.Parallel()

	markup := ">Title\n`!Bold` text\n`[Link`url]\n--"
	nodes := Parse(markup)
	got := RenderToPlainText(nodes)

	if strings.Contains(got, "[::b]") {
		t.Errorf("plain text contains formatting tags: %q", got)
	}
	if !strings.Contains(got, "Title") {
		t.Error("plain text missing Title")
	}
	if !strings.Contains(got, "Bold") {
		t.Error("plain text missing Bold text")
	}
	if !strings.Contains(got, "Link") {
		t.Error("plain text missing Link label")
	}
}

func TestExpandColor3Digit(t *testing.T) {
	t.Parallel()

	if got := expandColor("F00"); got != "FF0000" {
		t.Errorf("expandColor(F00) = %q, want %q", got, "FF0000")
	}
	if got := expandColor("0F0"); got != "00FF00" {
		t.Errorf("expandColor(0F0) = %q, want %q", got, "00FF00")
	}
}

func TestExpandColor6Digit(t *testing.T) {
	t.Parallel()

	if got := expandColor("AABBCC"); got != "AABBCC" {
		t.Errorf("expandColor(AABBCC) = %q, want %q", got, "AABBCC")
	}
}

func TestRenderToTViewField(t *testing.T) {
	t.Parallel()

	nodes := Parse("`<fieldname`default>")
	got := RenderToTView(nodes)
	if !strings.Contains(got, "fieldname") {
		t.Errorf("RenderToTView field missing name: %q", got)
	}
}

func TestParseLiteralToggle(t *testing.T) {
	t.Parallel()

	nodes := Parse("`=\n`!not bold`\n`=")
	for _, n := range nodes {
		if n.Type == NodeBold {
			t.Errorf("literal mode should suppress formatting, found NodeBold")
		}
	}
}

func TestRenderToTViewLiteralMode(t *testing.T) {
	t.Parallel()

	markup := "`=\n`!this is not bold`\n`="
	nodes := Parse(markup)
	got := RenderToTView(nodes)
	if strings.Contains(got, "[::b]") {
		t.Errorf("literal mode should not produce bold tags: %q", got)
	}
	if !strings.Contains(got, "`!this is not bold`") {
		t.Errorf("literal mode should preserve raw text: %q", got)
	}
}

func TestParseLiteralEscape(t *testing.T) {
	t.Parallel()

	nodes := Parse("`=\n\\`=\n`=")
	hasLiteral := false
	for _, n := range nodes {
		if n.Type == NodeText && n.Text == "`=" {
			hasLiteral = true
		}
	}
	if !hasLiteral {
		t.Error("escaped \\`= in literal mode should produce text `=")
	}
}

func TestParseTable(t *testing.T) {
	t.Parallel()

	markup := "`t\n!Name!Age!City\n!Alice!30!NYC\n`t"
	nodes := Parse(markup)
	hasTable := false
	for _, n := range nodes {
		if n.Type == NodeTable {
			hasTable = true
			if len(n.TableRows) != 2 {
				t.Errorf("table rows = %d, want 2", len(n.TableRows))
			}
			if len(n.TableRows) > 0 && len(n.TableRows[0]) != 3 {
				t.Errorf("table cols = %d, want 3", len(n.TableRows[0]))
			}
		}
	}
	if !hasTable {
		t.Error("Parse should produce NodeTable")
	}
}

func TestParseTableTooFewRows(t *testing.T) {
	t.Parallel()

	markup := "`t\n!Name!Age\n`t"
	nodes := Parse(markup)
	for _, n := range nodes {
		if n.Type == NodeTable {
			t.Error("single-row table should not produce NodeTable")
		}
	}
}

func TestRenderToTViewTable(t *testing.T) {
	t.Parallel()

	markup := "`t\n!Name!Age\n!Alice!30\n`t"
	nodes := Parse(markup)
	got := RenderToTView(nodes)
	if !strings.Contains(got, "Name") {
		t.Errorf("table render missing Name: %q", got)
	}
	if !strings.Contains(got, "Alice") {
		t.Errorf("table render missing Alice: %q", got)
	}
	if !strings.Contains(got, "[::b]") {
		t.Errorf("table header should be bold: %q", got)
	}
}

func TestRenderToPlainTextTable(t *testing.T) {
	t.Parallel()

	markup := "`t\n!Name!Age\n!Bob!25\n`t"
	nodes := Parse(markup)
	got := RenderToPlainText(nodes)
	if !strings.Contains(got, "Name") {
		t.Errorf("table plain text missing Name: %q", got)
	}
	if strings.Contains(got, "[::b]") {
		t.Errorf("plain text should not have tview tags: %q", got)
	}
}

func TestParseTableAlign(t *testing.T) {
	t.Parallel()

	markup := "`tc\n!A!B\n!1!2\n`t"
	nodes := Parse(markup)
	for _, n := range nodes {
		if n.Type == NodeTable {
			if n.TableAlign != AlignCenter {
				t.Errorf("table align = %v, want AlignCenter", n.TableAlign)
			}
		}
	}
}

func TestParseTableMaxWidth(t *testing.T) {
	t.Parallel()

	markup := "`t80\n!A!B\n!1!2\n`t"
	nodes := Parse(markup)
	for _, n := range nodes {
		if n.Type == NodeTable {
			if n.TableMaxWidth != 80 {
				t.Errorf("table max width = %d, want 80", n.TableMaxWidth)
			}
		}
	}
}

func TestRenderToTViewSectionIndent(t *testing.T) {
	t.Parallel()

	markup := ">>Heading\nContent under depth 2"
	nodes := Parse(markup)
	got := RenderToTView(nodes)
	if !strings.HasPrefix(got, "  ") || strings.HasPrefix(got, "    ") {
		t.Errorf("depth-2 content should have 2-space indent: %q", got)
	}
}

func TestRenderToTViewSectionIndentReset(t *testing.T) {
	t.Parallel()

	markup := ">>Heading\nContent\n<Back to root\nNormal"
	nodes := Parse(markup)
	got := RenderToTView(nodes)
	if !strings.Contains(got, "Normal") {
		t.Error("missing Normal text after reset")
	}
}
