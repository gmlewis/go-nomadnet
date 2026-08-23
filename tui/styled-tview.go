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
		width = 80
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
		// Heading lines: Python wraps the heading Text in
		// urwid.AttrMap(urwid.Text(output, align=...), heading_style)
		// (MicronParser.py:300-318). The AttrMap's heading_style is the
		// fallback attribute for every cell the Text does not cover, so urwid
		// fills the ENTIRE row — the left-indent spaces, the heading text, and
		// the right padding — with the heading background. The left-aligned
		// path below only colors the text characters, so the highlight stops
		// after the last letter (the reported parity bug). Reproduce the
		// full-width highlight: emit the heading background across the whole
		// row (indent + text + trailing pad), wrapping at the full width like
		// Python's heading Text (the indent lives inside the text, not as a
		// Padding inset, so the wrap width is the full pane width).
		if line.HeadingLevel > 0 && len(line.Spans) > 0 {
			base := line.Spans[0]
			hfg := tviewColor(base.FG)
			hbg := tviewColor(base.BG)
			cu := underlineOn
			var cb strings.Builder
			// Indent spaces carry the heading background: Python inserts them
			// as a plain-text run inside the AttrMap, so the fallback style
			// paints them. Emit them as a heading-styled run.
			if line.Indent > 0 {
				cb.WriteString(headingFillTag(hfg, hbg, cu))
				cb.WriteString(strings.Repeat(" ", line.Indent))
				cb.WriteString("[-:-:-]")
			}
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
			for _, row := range tview.WordWrap(cb.String(), width) {
				row = strings.TrimRight(row, " ")
				b.WriteString(row)
				// Pad the row to the full pane width with heading-background
				// spaces (the AttrMap fallback in Python). The heading style is
				// non-bold/non-underline, so the pad uses the plain heading
				// colors regardless of any inline toggles inside the text.
				if pad := width - tview.TaggedStringWidth(row); pad > 0 {
					b.WriteString(headingFillTag(hfg, hbg, underlineOn))
					b.WriteString(strings.Repeat(" ", pad))
					b.WriteString("[-:-:-]")
				}
				b.WriteByte('\n')
			}
			underlineOn = cu
			continue
		}
		// Aligned (`c`/`r`) lines: wrap the tagged content at the pane width and
		// prefix each wrapped row with the per-row alignment pad, mirroring urwid's
		// text_layout (urwid/text_layout.py:166-178): right pad = width-sc, center
		// pad = (width-sc+1)//2, where sc is the wrapped row's content width and no
		// pad is added when sc == width. The previous emission wrote the whole line
		// as a single row with one pad computed from the full text width, which (a)
		// left-aligned tview's wrapped continuation rows instead of right-aligning
		// each, diverging from Python, and (b) used floor (avail-textWidth)/2 for
		// center instead of urwid's ceiling, off by one on odd slack. The per-row
		// pad also keeps the hardware cursor (cursorScreenXY → alignPad) on the
		// rendered glyphs of a right-justified menu.
		if line.Align == micron.AlignCenter || line.Align == micron.AlignRight {
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
			rows := tview.WordWrap(cb.String(), wrapW)
			for i, row := range rows {
				// Drop the single break space tview.WordWrap leaves at the end of
				// a wrapped (non-final) row so the row's content width sc matches
				// urwid's line_width (which excludes it). This keeps the intra-word
				// trailing spaces urwid keeps (e.g. 2 of 3 separator spaces) so the
				// right/center pad lands the glyphs where urwid does, instead of
				// shifting them left by the full trailing-space count an
				// all-spaces TrimRight would drop. The final row keeps its
				// trailing spaces (content); force-broken rows end mid-word (no
				// trailing space), so the drop only affects non-final rows that
				// broke at a space.
				row = rtrimBreakSpace(row, i == len(rows)-1)
				rowW := tview.TaggedStringWidth(row)
				if line.Indent > 0 {
					underlineOn = clearUnderline(&b, underlineOn)
				}
				b.WriteString(strings.Repeat(" ", line.Indent))
				if pad := alignPad(line.Align, wrapW, rowW); pad > 0 {
					underlineOn = clearUnderline(&b, underlineOn)
					b.WriteString(strings.Repeat(" ", pad))
				}
				b.WriteString(row)
				b.WriteByte('\n')
			}
			underlineOn = cu
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

// headingFillTag emits the tview color-tag prefix that opens a run painted
// with the heading foreground/background but no bold/italic, and clears a
// latched underline toggle so the run is not underlined. It is used to color
// the indent and trailing-pad spaces of a heading line so the heading
// highlight fills the full row width — matching Python's urwid.AttrMap
// fallback style (MicronParser.py:318), which is the plain heading style
// (non-bold, non-underline) regardless of inline toggles inside the text.
func headingFillTag(fg, bg string, underlineOn bool) string {
	flags := tviewFlags(false, false, false, underlineOn)
	if flags == "" {
		return fmt.Sprintf("[%v:%v]", fg, bg)
	}
	return fmt.Sprintf("[%v:%v:%v]", fg, bg, flags)
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
// "-" (tview's default placeholder); "gNN" is mapped to the urwid 256-color
// grayscale ramp (matching palette.go's gray256Color); "#RRGGBB" passes
// through unchanged.
func tviewColor(spec string) string {
	if spec == "" || spec == "default" {
		return "-"
	}
	if len(spec) >= 2 && spec[0] == 'g' {
		digits := spec[1:]
		val, err := strconv.Atoi(digits)
		if err != nil || val < 0 || val > 100 {
			return "-"
		}
		c := gray256Color(val)
		h := uint32(c.Hex()) & 0xffffff
		return fmt.Sprintf("#%06x", h)
	}
	return spec
}
