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
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
	"github.com/gmlewis/tcell/v2"
)

// markdownBodyBattery is the input battery for the converter-stage comparison. Each entry
// exercises a markdown construct the converter has to turn into micron.
var markdownBodyBattery = []string{
	"# Title\n\nSome **bold** text and a [link](https://example.com).\n",
	"- item one\n- item two\n",
	"1. first\n2. second\n",
	"plain paragraph with _emphasis_ and `code`\n",
	"> a quotation\n",
	"## Sub heading\n\nbody\n",
	"a line\n\nanother paragraph\n",
}

// TestMarkdownToMicronBodyMatchesPython pins the message body's converter stage against
// the reference. Python renders a FIELD_RENDERER=markdown body through
// MarkdownToMicron(max_width=80) and then strips the non-formatting tags and the
// modifiers before handing it to the micron renderer (Conversations.py:2592, 2714-2718).
// The expected text is produced by running the installed reference on the same inputs,
// so it cannot go stale.
func TestMarkdownToMicronBodyMatchesPython(t *testing.T) {
	t.Parallel()
	testutils.SkipIfNoPythonNomadnet(t)

	req := map[string]any{"bodies": markdownBodyBattery}
	var want []string
	testutils.RunPythonNomadnet(t, req, markdownBodyPythonScript, &want)
	if len(want) != len(markdownBodyBattery) {
		t.Fatalf("python returned %v bodies for %v inputs", len(want), len(markdownBodyBattery))
	}

	// A positive assertion first: a known construct must convert, so a broken reference
	// interpreter cannot make the comparison vacuous.
	if got := markdownToMicronBody(markdownBodyBattery[0]); !strings.Contains(got, "`!bold`!") {
		t.Fatalf("markdownToMicronBody did not convert **bold**: %q", got)
	}

	for i, content := range markdownBodyBattery {
		if got := markdownToMicronBody(content); got != want[i] {
			t.Errorf("markdownToMicronBody(%q) =\n%q\nwant the Python reference's\n%q", content, got, want[i])
		}
	}
}

// markdownBodyPythonScript reproduces the reference's converter stage with the real
// MarkdownToMicron the widget uses, and prints one body per input.
const markdownBodyPythonScript = `
import json, sys
from nomadnet.Conversation import Conversation
import nomadnet.ui.textui.Conversations as C
from nomadnet.util import strip_modifiers, strip_non_formatting_tags

req = json.loads(sys.stdin.read() or "{}")
mdc = C.LXMessageWidget.mdc
out = [strip_modifiers(strip_non_formatting_tags(mdc.format_block(b))) for b in req["bodies"]]
print(json.dumps(out))
`

// TestMarkdownMessageBodyText pins the render half: the body is drawn in the message
// body's own default colour (Python passes fg_color="bbb" for the dark theme and "444" for
// the light one, Conversations.py:2715) rather than the page plain colour, every row is
// inset two columns (the urwid Padding), and the raw markdown punctuation is gone.
func TestMarkdownMessageBodyText(t *testing.T) {
	t.Parallel()

	const md = "# Title\n\nSome **bold** text.\n"

	dark := markdownMessageBodyText(md, ThemeDark, 100)
	if strings.Contains(dark, "**") {
		t.Errorf("the body still carries raw markdown: %q", dark)
	}
	if !strings.Contains(dark, "bold") {
		t.Errorf("the body lost its text: %q", dark)
	}
	if !strings.Contains(dark, "#bbbbbb") {
		t.Errorf("the body does not use the message default foreground #bbbbbb on the dark theme: %q", dark)
	}
	for row := range strings.SplitSeq(strings.TrimSuffix(dark, "\n"), "\n") {
		if !strings.HasPrefix(row, "  ") {
			t.Errorf("body row %q is not inset two columns", row)
		}
	}

	light := markdownMessageBodyText(md, ThemeLight, 100)
	if !strings.Contains(light, "#444444") {
		t.Errorf("the body does not use the message default foreground #444444 on the light theme: %q", light)
	}

	// A message with no markdown content still renders its text (the converter passes a
	// plain paragraph through), and an empty body renders nothing rather than a blank row.
	if got := markdownMessageBodyText("just text", ThemeDark, 100); !strings.Contains(got, "just text") {
		t.Errorf("plain body = %q, want it to carry the text", got)
	}
	if got := markdownMessageBodyText("", ThemeDark, 100); got != "" {
		t.Errorf("empty body = %q, want empty", got)
	}
}

// TestConversationMessageMarkdownIsFormatted pins the user-visible contract in the
// widget: a message whose renderer is markdown shows its body formatted, while the same
// content in a plain message is shown raw. Before this, every body was emitted as
// "  "+line, so a markdown message rendered its own "**bold**" punctuation.
func TestConversationMessageMarkdownIsFormatted(t *testing.T) {
	t.Parallel()

	content := "Some **bold** text.\n"
	markdown := renderTestEntry(t, ConversationMessage{Content: content, Markdown: true})
	if strings.Contains(markdown, "**") {
		t.Errorf("markdown message body was not converted: %q", markdown)
	}
	if !strings.Contains(markdown, "bold") {
		t.Errorf("markdown message body lost its text: %q", markdown)
	}

	plain := renderTestEntry(t, ConversationMessage{Content: content})
	if !strings.Contains(plain, "**bold**") {
		t.Errorf("a non-markdown message must keep its literal text: %q", plain)
	}
}

// renderTestEntry renders one message into its entry text, with the message list given a
// width so a markdown body is laid out as it would be on screen.
func renderTestEntry(t *testing.T, msg ConversationMessage) string {
	t.Helper()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	app.SetScreen(screen)
	screen.SetSize(100, 30)

	cw := NewConversationWidget(app, "deadbeef")
	cw.messageList.SetRect(0, 0, 96, 20)

	entry := cw.renderMessageEntry(msg)
	return entry.GetText(true)
}
