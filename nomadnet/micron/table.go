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
	"regexp"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Table box-drawing glyphs and constraints, mirroring MarkdownToMicron
// (RNS/Utilities/rngit/util.py:116-128).
const (
	tableH            = "─"
	tableV            = "│"
	tableTL           = "┌"
	tableTR           = "┐"
	tableBL           = "└"
	tableBR           = "┘"
	tableML           = "├"
	tableMR           = "┤"
	tableTM           = "┬"
	tableBM           = "┴"
	tableMM           = "┼"
	tableMinColWidth  = 3
	defaultMaxTableWd = 100 // MAX_TABLE_WIDTH (MicronParser.py:37)
)

// tagStripPatterns remove micron color/format tags for visible-width
// measurement, in the same order as MarkdownToMicron._visible_width
// (RNS/Utilities/rngit/util.py). A 12-bit `Fxxx/`Bxxx, a 24-bit `FTxxxxxx/
// `BTxxxxxx, the format toggles `!`*`_`=, the combined `f`b reset, and the
// single `f/`b resets.
var tagStripPatterns = []*regexp.Regexp{
	regexp.MustCompile("`[FB][0-9a-fA-F]{3}"),
	regexp.MustCompile("`[FB]T[0-9a-fA-F]{6}"),
	regexp.MustCompile("`[!*_=]"),
	regexp.MustCompile("`f`b"),
	regexp.MustCompile("`f"),
	regexp.MustCompile("`b"),
}

// visibleWidth returns the display width of a cell with micron tags stripped,
// mirroring MarkdownToMicron._visible_width (util.py). Display width uses
// runewidth.StringWidth, the Go equivalent of Python's wcwidth.wcswidth.
func visibleWidth(text string) int {
	for _, re := range tagStripPatterns {
		text = re.ReplaceAllString(text, "")
	}
	return runewidth.StringWidth(text)
}

// parseTableRow splits a markdown table line on "|" delimiters, handling "\"
// escapes and stripping leading/trailing "|" and per-cell whitespace. Mirrors
// MarkdownToMicron._parse_table_row (util.py).
func parseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")

	var cells []string
	var current strings.Builder
	escaped := false
	for _, char := range line {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		switch char {
		case '\\':
			escaped = true
		case '|':
			cells = append(cells, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}
	cells = append(cells, strings.TrimSpace(current.String()))
	return cells
}

// parseTableAlignments parses a markdown separator row (e.g. "| :--: | --: |")
// into per-column alignment strings ("left"/"center"/"right"). Mirrors
// MarkdownToMicron._parse_table_alignments (util.py).
func parseTableAlignments(line string) []string {
	cells := parseTableRow(line)
	aligns := make([]string, 0, len(cells))
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		switch {
		case strings.HasPrefix(cell, ":") && strings.HasSuffix(cell, ":"):
			aligns = append(aligns, "center")
		case strings.HasSuffix(cell, ":"):
			aligns = append(aligns, "right")
		default:
			aligns = append(aligns, "left")
		}
	}
	return aligns
}

// padCell truncates a cell to the column width and pads it according to align,
// emitting the RAW (tag-bearing) text — so a cell with micron tags overflows
// its column physically by the tag width. This is the canonical Python
// behavior (util.py _pad_cell/_truncate_cell).
func padCell(text, align string, width int) string {
	text = truncateCell(text, width)
	textWidth := visibleWidth(text)
	padding := max(width-textWidth, 0)
	switch align {
	case "right":
		return strings.Repeat(" ", padding) + text
	case "center":
		left := padding / 2
		right := padding - left
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
	default: // "left"
		return text + strings.Repeat(" ", padding)
	}
}

// truncateCell truncates a cell to fit width (visible), appending "…" and
// closing any micron tags still active at the truncation point. Mirrors
// MarkdownToMicron._truncate_cell (util.py).
func truncateCell(text string, width int) string {
	if visibleWidth(text) <= width {
		return text
	}
	runes := []rune(text)
	truncationPoint := len(runes)
	for truncationPoint > 0 && visibleWidth(string(runes[:truncationPoint])) >= width {
		truncationPoint--
	}
	truncated := runes[:truncationPoint]

	// Track tags still active at the truncation point so they can be closed.
	activeTags := map[rune]bool{}
	fgActive := false
	bgActive := false
	i := 0
	for i < len(truncated) {
		if truncated[i] == '`' {
			if i+1 < len(truncated) {
				tagChar := truncated[i+1]
				switch tagChar {
				case '!', '*', '_', '=':
					activeTags[tagChar] = !activeTags[tagChar]
					i += 2
					continue
				case 'f':
					fgActive = false
					i += 2
					continue
				case 'b':
					bgActive = false
					i += 2
					continue
				case 'F':
					fgActive = true
					if i+2 < len(truncated) && truncated[i+2] == 'T' {
						i += 8
					} else {
						i += 5
					}
					continue
				case 'B':
					bgActive = true
					if i+2 < len(truncated) && truncated[i+2] == 'T' {
						i += 8
					} else {
						i += 5
					}
					continue
				}
			}
		}
		i++
	}

	var closers strings.Builder
	if fgActive {
		closers.WriteString("`f")
	}
	if bgActive {
		closers.WriteString("`b")
	}
	// Deterministic closer order (Python uses a set; order is unspecified there
	// but these cells are not in the golden set).
	for _, fmt := range []rune{'!', '*', '_', '='} {
		if activeTags[fmt] {
			closers.WriteRune('`')
			closers.WriteRune(fmt)
		}
	}
	closers.WriteString("…")
	return string(truncated) + closers.String()
}

// FormatTableRaw renders markdown table rows as box-drawing micron lines, a port
// of MarkdownToMicron.format_table_raw (RNS/Utilities/rngit/util.py:530). align
// is the table-level micron alignment ("" / "l" / "c" / "r") from the `t tag;
// maxWidth caps the total border width (default 100). Fewer than 2 rows are
// returned unchanged. Cell text is emitted raw (tags included), so tagged cells
// overflow their column — matching Python's canonical visible-width layout.
func FormatTableRaw(rows []string, align string, maxWidth int) []string {
	if len(rows) < 2 {
		return rows
	}
	if maxWidth <= 0 {
		maxWidth = defaultMaxTableWd
	}

	headerCells := parseTableRow(rows[0])
	alignments := parseTableAlignments(rows[1])
	for len(alignments) < len(headerCells) {
		alignments = append(alignments, "left")
	}
	alignments = alignments[:len(headerCells)]

	var dataRows [][]string
	for _, line := range rows[2:] {
		cells := parseTableRow(line)
		for len(cells) < len(headerCells) {
			cells = append(cells, "")
		}
		cells = cells[:len(headerCells)]
		dataRows = append(dataRows, cells)
	}

	numCols := len(headerCells)
	colWidths := make([]int, numCols)
	for _, row := range append([][]string{headerCells}, dataRows...) {
		for i, cell := range row {
			if w := visibleWidth(cell); w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}
	for i, w := range colWidths {
		if w < tableMinColWidth {
			colWidths[i] = tableMinColWidth
		}
	}

	// Shrink widest columns down to the min when over maxWidth.
	totalWidth := sum(colWidths) + numCols*3 + 1
	if totalWidth > maxWidth {
		excess := totalWidth - maxWidth
		indexed := make([]struct {
			i, w int
		}, numCols)
		for i, w := range colWidths {
			indexed[i] = struct{ i, w int }{i, w}
		}
		sort.SliceStable(indexed, func(a, b int) bool {
			return indexed[a].w > indexed[b].w
		})
		for _, e := range indexed {
			if excess <= 0 {
				break
			}
			reduction := min(e.w-tableMinColWidth, excess)
			colWidths[e.i] -= reduction
			excess -= reduction
		}
	}

	var result []string
	if align != "" {
		result = append(result, "`"+align)
	}

	// Top border.
	result = append(result, boxBorder(tableTL, tableTM, tableTR, colWidths))

	// Header row (always left-padded).
	var headerLine strings.Builder
	headerLine.WriteString(tableV)
	for i, cell := range headerCells {
		headerLine.WriteString(" ")
		headerLine.WriteString(padCell(cell, "left", colWidths[i]))
		headerLine.WriteString(" ")
		headerLine.WriteString(tableV)
	}
	result = append(result, headerLine.String())

	// Separator row.
	result = append(result, boxBorder(tableML, tableMM, tableMR, colWidths))

	// Data rows (per-column alignment).
	for _, row := range dataRows {
		var rowLine strings.Builder
		rowLine.WriteString(tableV)
		for i, cell := range row {
			rowLine.WriteString(" ")
			rowLine.WriteString(padCell(cell, alignments[i], colWidths[i]))
			rowLine.WriteString(" ")
			rowLine.WriteString(tableV)
		}
		result = append(result, rowLine.String())
	}

	// Bottom border.
	result = append(result, boxBorder(tableBL, tableBM, tableBR, colWidths))

	if align != "" {
		result = append(result, "`a")
	}
	return result
}

// boxBorder builds a horizontal border line with the given left/mid/right
// corners and tableH fillers sized to colWidths (w+2 per column).
func boxBorder(left, mid, right string, colWidths []int) string {
	var b strings.Builder
	b.WriteString(left)
	for i, w := range colWidths {
		b.WriteString(strings.Repeat(tableH, w+2))
		if i < len(colWidths)-1 {
			b.WriteString(mid)
		} else {
			b.WriteString(right)
		}
	}
	return b.String()
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}
