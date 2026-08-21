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
	"github.com/mattn/go-runewidth"
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
	// underlineOn tracks whether the tview color-tag state currently has the
	// underline attribute latched ON. tview's [-:-:-] reset (parseTag in
	// strings.go) clears the foreground, background, and the bold/italic
	// attribute MASK, but it does NOT clear the separate Underline toggle,
	// which is set by the lowercase 'u' tag and only cleared by the uppercase
	// 'U' tag. So once a span emits :u, every following run — plain indent
	// spaces, spans whose tag has no attribute field, divider chars —
	// inherits underline until an explicit :U is emitted. The micron
	// renderer already scopes underline per span/line; the converter must
	// mirror that by emitting 'U' to turn the latched toggle off whenever a
	// run should not be underlined (and before plain indent/divider content).
	underlineOn := false
	for _, line := range lines {
		if line == nil {
			b.WriteByte('\n')
			continue
		}
		if line.Divider {
			underlineOn = clearUnderline(&b, underlineOn)
			ch := line.DividerChar
			if ch == "" {
				ch = "─"
			}
			// Python: depth 0 → urwid.Divider fills the full width; depth>0 →
			// Padding(Divider, left=left_indent, right=right_indent). The divider
			// run spans width - left - right, offset by left indent spaces.
			n := max(width-line.Indent-line.DividerRight, 1)
			b.WriteString(strings.Repeat(" ", line.Indent))
			b.WriteString(strings.Repeat(ch, n))
			b.WriteByte('\n')
			continue
		}
		// Aligned (`c`/`r`) lines are emitted as a single row with the alignment
		// pad, as before. Aligned content is depth-0 in practice (Indent 0), and
		// aligned text that wraps is not exercised by the guide/pages, so the
		// single-row emission is preserved rather than re-deriving per-row
		// centering for the pre-wrap path below.
		if line.Align == micron.AlignCenter || line.Align == micron.AlignRight {
			// Plain indent spaces inherit the latched underline; clear it first.
			if line.Indent > 0 {
				underlineOn = clearUnderline(&b, underlineOn)
			}
			b.WriteString(strings.Repeat(" ", line.Indent))
			// Python centers/right-aligns within Padding(left=left_indent,
			// right=right_indent): the centering space is (avail-textWidth)//2
			// where avail = width-left_indent-right_indent, and the left_indent
			// (already written) shifts the block right. right_indent is 0 for
			// depth-0 aligned content (the only place `c`/`r` appear).
			textWidth := 0
			for _, span := range line.Spans {
				textWidth += runewidth.StringWidth(span.Text)
			}
			avail := max(width-line.Indent, textWidth)
			pad := 0
			switch line.Align {
			case micron.AlignCenter:
				pad = (avail - textWidth) / 2
			case micron.AlignRight:
				pad = avail - textWidth
			}
			if pad > 0 {
				underlineOn = clearUnderline(&b, underlineOn)
				b.WriteString(strings.Repeat(" ", pad))
			}
			for _, span := range line.Spans {
				if span.Link != nil {
					idx := len(links)
					links = append(links, *span.Link)
					fmt.Fprintf(&b, `["%v"]`, idx)
					underlineOn = writeSpanTag(&b, span, underlineOn)
					b.WriteString(`[""]`)
					continue
				}
				underlineOn = writeSpanTag(&b, span, underlineOn)
			}
			b.WriteByte('\n')
			continue
		}
		// Left-aligned text line: mirror Python's Padding(left_indent,
		// right_indent) around the line's urwid.Text. Python's left_indent =
		// right_indent = (depth-1)*SECTION_INDENT (MicronParser.py:418-422); the
		// Go StyledLine carries left_indent as line.Indent and right_indent is
		// the same value, so the content wraps at width-2*Indent and EVERY
		// wrapped row — including continuations — is offset line.Indent. The
		// page body is a single tview.TextView, so per-line Padding cannot be a
		// layout inset; instead the content is pre-wrapped here (rows joined by
		// newlines, each prefixed with the indent) so the TextView displays the
		// indented continuation rows Python's Padding produces. The previous
		// code baked only the first row's indent and let the TextView wrap at
		// the full width, so depth>=2 body text wrapped one word too wide and
		// continuation rows sat at column 0.
		//
		// Build the line's tagged content (spans + region tags) into a scratch
		// buffer, tracking the post-line underline latch (cu) so the next line
		// sees the correct state. tview.WordWrap parses color AND region tags as
		// zero-width (parseTag handles both), so wrapping the tagged content
		// breaks by visible width and preserves the ["N"]…[""] region tags. A
		// link region that straddles a wrapped row stays open across the hard
		// newline (tview carries the region state into the next line), so clicks
		// on the continuation row still resolve to the link.
		var cb strings.Builder
		cu := underlineOn
		for _, span := range line.Spans {
			if span.Link != nil {
				idx := len(links)
				links = append(links, *span.Link)
				fmt.Fprintf(&cb, `["%v"]`, idx)
				cu = writeSpanTag(&cb, span, cu)
				cb.WriteString(`[""]`)
				continue
			}
			cu = writeSpanTag(&cb, span, cu)
		}
		wrapW := max(width-2*line.Indent, 1)
		for _, row := range tview.WordWrap(cb.String(), wrapW) {
			// WordWrap leaves the break space at the end of a row; drop it so the
			// trailing cells are pure background fill (matching Python, which
			// pads rows with the background color rather than a trailing space).
			row = strings.TrimRight(row, " ")
			if line.Indent > 0 {
				underlineOn = clearUnderline(&b, underlineOn)
			}
			b.WriteString(strings.Repeat(" ", line.Indent))
			b.WriteString(row)
			b.WriteByte('\n')
		}
		// Sync the real underline latch to the post-line state (the rows were
		// written as opaque tagged strings, so the live latch was not updated by
		// their tags). The next line starts from the correct state.
		underlineOn = cu
	}
	return b.String(), links
}

// clearUnderline emits a tview tag that turns the latched underline toggle OFF
// ([-:-:U]) if it is currently on, returning the new (now off) state. This is
// a no-op when underline is already off, so runs that never use underline are
// emitted unchanged.
func clearUnderline(b *strings.Builder, underlineOn bool) bool {
	if underlineOn {
		b.WriteString("[-:-:U]")
		return false
	}
	return underlineOn
}

// writeSpanTag emits one styled run: a color tag, the (tag-escaped) text, and
// a full reset. Fully-default spans (no fg/bg/flags and no link) are emitted
// as plain escaped text with no tag. It returns the updated latched-underline
// state so the caller can emit an explicit 'U' to turn underline off for a
// later run (tview's [-:-:-] reset does not clear the underline toggle).
func writeSpanTag(b *strings.Builder, span micron.StyledSpan, underlineOn bool) bool {
	fg := tviewColor(span.FG)
	bg := tviewColor(span.BG)
	flags := tviewFlags(span.Bold, span.Underline, span.Italic, underlineOn)
	if fg == "-" && bg == "-" && flags == "" {
		b.WriteString(tview.Escape(span.Text))
		return underlineOn
	}
	if flags == "" {
		fmt.Fprintf(b, "[%v:%v]%v[-:-:-]", fg, bg, tview.Escape(span.Text))
	} else {
		fmt.Fprintf(b, "[%v:%v:%v]%v[-:-:-]", fg, bg, flags, tview.Escape(span.Text))
	}
	return span.Underline
}

// tviewFlags builds the tview color-tag attribute string from the style bools.
// When the run is not underlined but the latched toggle is on (underlineOn),
// an uppercase 'U' is prepended so tview turns the underline OFF for this run —
// the [-:-:-] reset only clears the bold/italic mask, not the underline toggle.
func tviewFlags(bold, underline, italic, underlineOn bool) string {
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
	if !underline && underlineOn {
		f = "U" + f
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
