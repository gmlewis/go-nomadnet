// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// StyledLinesToTviewText converts rendered Micron styled lines (the output of
// micron.RenderToStyledLines, the Phase-3 24-bit-depth renderer) into a tview
// color-tagged string suitable for a tview.TextView with dynamic colors (and
// regions) enabled. It is the display bridge between the structured micron
// renderer and tview, used by the Guide reader and the Browser page so that
// body text is NOT all-bold: each styled span becomes one self-contained
// `[fg:bg:flags]text[-:-:-]` run with a full reset after it, so bold/italic
// cannot bleed into the following plain run.
//
// width is the rendering column count used to expand horizontal-divider lines
// (Python urwid.Divider fills the pane); pass 0 to default to 60. Links are
// wrapped in numbered tview region tags ["N"]...[""] so a TextView with
// SetRegions(true) can resolve clicks; the mapping (index → LinkSpec) is
// returned so the caller can dispatch activations.
func StyledLinesToTviewText(lines []*micron.StyledLine, width int) (string, []micron.LinkSpec) {
	if width <= 0 {
		width = 60
	}
	var b strings.Builder
	var links []micron.LinkSpec
	for _, line := range lines {
		if line == nil {
			b.WriteByte('\n')
			continue
		}
		if line.Divider {
			ch := line.DividerChar
			if ch == "" {
				ch = "─"
			}
			n := width - line.Indent
			if n < 1 {
				n = 1
			}
			b.WriteString(strings.Repeat(" ", line.Indent))
			b.WriteString(strings.Repeat(ch, n))
			b.WriteByte('\n')
			continue
		}
		b.WriteString(strings.Repeat(" ", line.Indent))
		for _, span := range line.Spans {
			if span.Link != nil {
				idx := len(links)
				links = append(links, *span.Link)
				fmt.Fprintf(&b, `["%d"]`, idx)
				writeSpanTag(&b, span)
				b.WriteString(`[""]`)
				continue
			}
			writeSpanTag(&b, span)
		}
		b.WriteByte('\n')
	}
	return b.String(), links
}

// writeSpanTag emits one styled run: a color tag, the (tag-escaped) text, and
// a full reset. Fully-default spans (no fg/bg/flags and no link) are emitted
// as plain escaped text with no tag.
func writeSpanTag(b *strings.Builder, span micron.StyledSpan) {
	fg := tviewColor(span.FG)
	bg := tviewColor(span.BG)
	flags := tviewFlags(span.Bold, span.Underline, span.Italic)
	if fg == "-" && bg == "-" && flags == "" {
		b.WriteString(tview.Escape(span.Text))
		return
	}
	if flags == "" {
		fmt.Fprintf(b, "[%s:%s]%s[-:-:-]", fg, bg, tview.Escape(span.Text))
	} else {
		fmt.Fprintf(b, "[%s:%s:%s]%s[-:-:-]", fg, bg, flags, tview.Escape(span.Text))
	}
}

// tviewFlags builds the tview color-tag attribute string from the style bools.
func tviewFlags(bold, underline, italic bool) string {
	var f string
	if bold {
		f += "b"
	}
	if underline {
		f += "u"
	}
	if italic {
		f += "i"
	}
	return f
}

// tviewColor converts a micron high-color spec ("#RRGGBB", "default", or
// "gNN" grayscale) into a tview color-tag color token. "default"/"" becomes
// "-" (tview's default placeholder); "gNN" is mapped to a linear #RRGGBB
// grayscale (NN 00..99 → 0..255); "#RRGGBB" passes through unchanged.
func tviewColor(spec string) string {
	if spec == "" || spec == "default" {
		return "-"
	}
	if len(spec) >= 3 && spec[0] == 'g' {
		// Grayscale gNN, NN is two decimal digits 00..99.
		digits := spec[1:]
		val, err := strconv.Atoi(digits)
		if err != nil || val < 0 {
			return "-"
		}
		if val > 99 {
			val = 99
		}
		level := val * 255 / 99
		return fmt.Sprintf("#%02x%02x%02x", level, level, level)
	}
	return spec
}
