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

package tui

import (
	gomicron "github.com/gmlewis/go-reticulum/micron"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-nomadnet/nomadnet/util"
)

// markdownMessageBodyPrefix is the left (and right) inset Python wraps a message body
// in: `urwid.Padding(urwid.Pile(rendered), left=2, right=2)` (Conversations.py:2722).
const markdownMessageBodyPrefix = "  "

// markdownMaxWidth is the width Python's converter lays markdown out at:
// MarkdownToMicron(max_width=80, …) (Conversations.py:2592).
const markdownMaxWidth = 80

// markdownToMicronBody runs the first half of the message-body pipeline — the part that
// turns the markdown the sender composed into the micron document the renderer draws:
//
//	strip_modifiers(strip_non_formatting_tags(
//	    MarkdownToMicron(max_width=80, syntax_highlighter=…).format_block(content)))
//
// (Conversations.py:2592, 2714-2718). It is split out because that text is exactly
// comparable with the Python reference, which the parity test does.
func markdownToMicronBody(content string) string {
	conv := gomicron.NewConverter(
		gomicron.WithMaxWidth(markdownMaxWidth),
		gomicron.WithHighlighter(gomicron.NewHighlighter()),
	)
	// Python strip_modifiers takes and returns the text; the Go helper takes a pointer so
	// a nil is preserved (Python's None), hence the local.
	stripped := util.StripModifiers(new(util.StripNonFormattingTags(conv.FormatBlock(content))))
	if stripped == nil {
		return ""
	}
	return *stripped
}

// markdownMessageBodyText renders an LXMF message body whose FIELD_RENDERER is
// markdown, reproducing Python's LXMessageWidget pipeline (Conversations.py:2714-2722):
//
//	formatted    = mdc.format_block(content)          # max_width 80
//	message_body = strip_non_formatting_tags(formatted)
//	rendered     = markup_to_attrmaps(strip_modifiers(message_body),
//	                                  fg_color="bbb" if THEME_DARK else "444")
//	content_pile = urwid.Padding(urwid.Pile(rendered), left=2, right=2)
//
// width is the pane width available to the body; the right inset is subtracted here,
// matching the Padding, because a heading row is painted across the full pile width.
// A non-positive width falls back to the converter's own 80 columns, which the first
// layout then replaces.
//
// The returned string is tview color-tag markup, ready to append to a dynamic-colors
// TextView: StyledLinesToTviewText escapes each span's text, so a message containing
// "[" cannot inject a tag.
func markdownMessageBodyText(content string, theme int, width int) string {
	// Python's default_fg for a message body, not the theme's plain color: "bbb" on
	// the dark palette, "444" on the light one (Conversations.py:2715).
	defaultFG := "bbb"
	if theme == ThemeLight {
		defaultFG = "444"
	}

	body := markdownToMicronBody(content)
	if body == "" {
		return ""
	}

	inner := width - 2*len(markdownMessageBodyPrefix)
	if inner <= 0 {
		inner = markdownMaxWidth
	}
	lines := micron.RenderToStyledLinesWith(body, micronTheme(theme), micron.StyledOptions{DefaultFG: defaultFG})
	text, _ := StyledLinesToTviewText(lines, inner)

	// Inset every emitted row by two columns (Python's Padding). The rendered text has
	// one row per line, each newline-terminated, but a wrapped heading carries embedded
	// newlines, so prefix after each one.
	var out []byte
	out = append(out, markdownMessageBodyPrefix...)
	for i := 0; i < len(text); i++ {
		out = append(out, text[i])
		if text[i] == '\n' && i+1 < len(text) {
			out = append(out, markdownMessageBodyPrefix...)
		}
	}
	return string(out)
}
