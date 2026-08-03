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

// Package tui implements the NomadNet terminal user interface.
//
// Guide display: this file ports Python's Guide.py. The per-topic micron
// source is embedded verbatim from the original nomadnet Guide.py (under
// tui/guidetopics/*.mu) and rendered through the Phase-3 styled-line
// renderer (micron.RenderToStyledLines) so body text is NOT all-bold.

package tui

import (
	_ "embed"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// Embedded guide topic micron source, extracted verbatim from the original
// nomadnet/ui/textui/Guide.py TOPIC_* strings. Each file begins with a
// suppressed `#`-comment copyright header (micron comments render no output),
// so the first rendered line is the topic heading, matching the original.
//
//go:embed guidetopics/introduction.mu
var guideIntroduction string

//go:embed guidetopics/concepts.mu
var guideConcepts string

//go:embed guidetopics/channels.mu
var guideChannels string

//go:embed guidetopics/interfaces.mu
var guideInterfaces string

//go:embed guidetopics/hosting.mu
var guideHosting string

//go:embed guidetopics/config.mu
var guideConfig string

//go:embed guidetopics/shortcuts.mu
var guideShortcuts string

//go:embed guidetopics/markup.mu
var guideMarkup string

//go:embed guidetopics/firstrun.mu
var guideFirstRun string

//go:embed guidetopics/networks.mu
var guideNetworks string

//go:embed guidetopics/displaytest.mu
var guideDisplayTest string

//go:embed guidetopics/licenses.mu
var guideLicenses string

// guideTopics is the ordered topic list matching Python's TopicList (Guide.py
// :167-180): 12 entries, "First Run" at position 9 (index 8). The label is the
// text shown in the topic list; the markup is the micron source rendered into
// the reader pane on selection.
var guideTopics = []struct {
	label  string
	markup string
}{
	{"Introduction", guideIntroduction},
	{"Concepts & Terminology", guideConcepts},
	{"Channels & RRC", guideChannels},
	{"Interfaces", guideInterfaces},
	{"Hosting a Node", guideHosting},
	{"Configuration Options", guideConfig},
	{"Keyboard Shortcuts", guideShortcuts},
	{"Markup", guideMarkup},
	{"First Run", guideFirstRun},
	{"Network Configuration", guideNetworks},
	{"Display Test", guideDisplayTest},
	{"Credits & Licenses", guideLicenses},
}

// firstRunTopicIndex is the index of the "First Run" topic in guideTopics,
// whose content ("First Time Information") the original auto-displays on a
// first run (Guide.py:200-224).
const firstRunTopicIndex = 8

// GuideDisplay shows help content with a topic list and a scrollable micron
// reader, matching Python's GuideDisplay (Guide.py:197-284): a two-column
// GuideColumns layout (Topics list weight 0.33, reader weight 0.67), each pane
// in its own LineBox (Topics titled, reader untitled), no outer border.
type GuideDisplay struct {
	app        *App
	widget     tview.Primitive
	topics     *tview.List
	topicsList *IndicativeListBox
	reader     *guideReader
	readerBox  *tview.Flex
	scroll     *ScrollBar

	currentIdx   int // currently displayed topic, -1 = placeholder
	links        []micron.LinkSpec
	currentLines []*micron.StyledLine // rendered lines of the current topic
	anchors      micron.AnchorMap     // anchor name → line index (current topic)
	lineTexts    []string             // per-line tview-tagged text (current topic)

	// OnHandleLink is invoked when a link in the reader is activated (click or
	// keyboard). The app wires this to the GuideLinkDelegate behavior
	// (Guide.py:91-118): "#anchor" jumps in-page, external links switch to
	// Network + browser.handle_link. nil ⇒ links are inert.
	OnHandleLink func(target, fields string)
}

// NewGuideDisplay creates a new guide display with the two-column layout. On a
// normal launch the reader shows the "No topic selected" placeholder and the
// topic list has focus (Guide.py focus_column=0). Call ShowFirstRun to display
// the first-run content with the reader focused.
func NewGuideDisplay(app *App) *GuideDisplay {
	gd := &GuideDisplay{app: app, currentIdx: -1}

	// Topic list (left pane). Python's TopicList is an IndicativeListBox of
	// single-row GuideEntry urwid.Text widgets, so disable tview's secondary-
	// text row (which would otherwise render a blank line under every item).
	gd.topics = tview.NewList()
	gd.topics.SetHighlightFullLine(true)
	gd.topics.ShowSecondaryText(false)
	ApplyListFocusStyle(gd.topics, app.Theme)
	// Unfocused topic-list items use the theme's topic_list_normal foreground,
	// matching Python GuideEntry's AttrMap(widget, "topic_list_normal",
	// "list_focus") (Guide.py:133-135). ApplyListFocusStyle only sets the
	// SELECTED colors; without this the unfocused rows fall back to tview's
	// terminal-default fg instead of the themed #ddd (dark) / #222 (light)
	// (Python TextUI.py:52 dark, :105 light; the first palette block is
	// THEME_DARK at TextUI.py:19).
	gd.topics.SetMainTextColor(GetThemeColors(app.Theme)["topic_list_normal"])
	for i := range guideTopics {
		i := i
		gd.topics.AddItem(guideTopics[i].label, "", 0, func() { gd.showTopic(i) })
	}
	gd.topics.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		gd.showTopic(index)
	})

	// Reader (right pane). A thin wrapper around tview.TextView overrides
	// SetRect so horizontal dividers re-expand to the pane width on resize.
	gd.reader = newGuideReader(gd)
	gd.reader.SetDynamicColors(true)
	gd.reader.SetRegions(true)
	gd.reader.SetScrollable(true)
	gd.reader.SetWrap(true)
	gd.reader.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	gd.reader.SetHighlightedFunc(func(added, removed, remaining []string) {
		if len(added) > 0 {
			gd.activateLink(added[0])
		}
	})

	// Each pane in its own LineBox (SetBorder), matching the original: Topics
	// titled, reader untitled, no outer border around the columns. The topic
	// list is wrapped in an IndicativeListBox so it shows the ───/▲/▼ end
	// indicators like the original IndicativeListBox. The reader is wrapped in a
	// ScrollBar so it shows the ┃ thumb on the right edge when content overflows
	// (urwid ScrollBar, Guide.py:232).
	gd.topicsList = NewIndicativeListBox(gd.topics)
	topicsBox := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(gd.topicsList, 0, 1, true)
	topicsBox.SetBorder(true)
	SetTitledBorder(topicsBox, "Topics")

	gd.scroll = NewScrollBar(gd.reader)
	gd.scroll.SetThumbColor(GetThemeColors(app.Theme)["scrollbar"])
	gd.readerBox = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(gd.scroll, 0, 1, true)
	gd.readerBox.SetBorder(true)

	// Weights 1 : 2 ≈ 0.33 : 0.67 (Guide.py list_width = 0.33). The topics pane
	// is the initial focus (focus=true) on a normal launch.
	cols := newURWIDColumns(0, topicsBox, gd.readerBox)
	cols.SetWeight(0, 1)
	cols.SetWeight(1, 2)

	gd.topicsList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event != nil && event.Key() == tcell.KeyUp && gd.topics.GetCurrentItem() == 0 {
			if gd.app != nil && gd.app.Main != nil {
				gd.app.Main.FocusMenu()
				return nil
			}
		}
		return event
	})

	cols.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event == nil {
			return nil
		}
		switch event.Key() {
		case tcell.KeyRight, tcell.KeyTab:
			if gd.topicsList.HasFocus() {
				gd.FocusReader()
				return nil
			}
		case tcell.KeyLeft:
			if gd.scroll.HasFocus() {
				gd.FocusTopics()
				return nil
			}
		}
		return event
	})

	gd.widget = cols

	gd.showPlaceholder()
	return gd
}

// Widget returns the tview primitive for this display.
func (gd *GuideDisplay) Widget() tview.Primitive {
	return gd.widget
}

// ShowFirstRun displays the "First Time Information" content (the First Run
// topic) and moves focus to the reader, matching the original first-run
// behavior (Guide.py:221-224 entry.display_topic + focus_reader).
func (gd *GuideDisplay) ShowFirstRun() {
	gd.showTopic(firstRunTopicIndex)
	if gd.app != nil {
		gd.app.SetFocus(gd.scroll)
	}
}

// FocusTopics moves focus to the topic list (Guide.py focus_topics).
func (gd *GuideDisplay) FocusTopics() {
	if gd.app != nil {
		gd.app.SetFocus(gd.topicsList)
	}
}

// FocusReader moves focus to the reader (Guide.py focus_reader). Focus targets
// the ScrollBar wrapper (the layout child); it forwards focus to the reader.
func (gd *GuideDisplay) FocusReader() {
	if gd.app != nil {
		gd.app.SetFocus(gd.scroll)
	}
}

// showPlaceholder shows the "No topic selected" placeholder
// (Guide.py:204 urwid.Text("\n  No topic selected")).
func (gd *GuideDisplay) showPlaceholder() {
	gd.currentIdx = -1
	gd.links = nil
	// The reader pane border is untitled (Python Guide.py:207 LineBox has no
	// title=). The "No topic selected" text is pane CONTENT, not a border title.
	SetTitledBorder(gd.readerBox, "")
	gd.reader.SetText("\n  No topic selected")
}

// showTopic renders the given topic's micron source into the reader and syncs
// the topic-list selection (Guide.py GuideEntry.display_topic).
func (gd *GuideDisplay) showTopic(idx int) {
	if idx < 0 || idx >= len(guideTopics) {
		return
	}
	gd.currentIdx = idx
	// The reader pane border is untitled (Python Guide.py:207 LineBox has no
	// title=). The topic title is a content `>` heading rendered INSIDE the
	// pane by renderMarkup, not a border title.
	SetTitledBorder(gd.readerBox, "")
	gd.renderMarkup(guideTopics[idx].markup)
}

// Shortcuts returns the footer shortcut text for the Guide page, matching
// Python's GuideDisplayShortcuts (Guide.py:8-13): an empty urwid.Text("") wrapped
// in the "shortcutbar" attr — the Guide footer row is just the shortcutbar
// background fill with NO text. (pinned by TestGuideShortcutBarEmpty.)
func (gd *GuideDisplay) Shortcuts() string {
	return ""
}

// unit-testing jumpToAnchor/handleLink without going through the embedded topic
// list). Production uses showTopic.
func (gd *GuideDisplay) showMarkupForTest(markup string) {
	gd.currentIdx = 0 // mark "a topic is shown" so jumpToAnchor/rerender act
	gd.renderMarkup(markup)
}

// renderMarkup is the shared render path: micron-render the markup, convert to
// tview tags at the reader's current width, push it to the reader, and cache
// the line data (anchor map + per-line text) that jumpToAnchor needs.
func (gd *GuideDisplay) renderMarkup(markup string) {
	lines := micron.RenderToStyledLines(markup, micronTheme(gd.app.Theme))
	text, links := StyledLinesToTviewText(lines, gd.readerWidth())
	gd.currentLines = lines
	gd.links = links
	gd.anchors = micron.BuildAnchorMap(lines)
	gd.lineTexts = splitLineTexts(text)
	gd.reader.SetText(text)
}

// splitLineTexts splits a StyledLinesToTviewText result into one entry per
// rendered line (dropping the trailing newline), so jumpToAnchor can measure
// each line's wrapped height independently.
func splitLineTexts(text string) []string {
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// rerender re-renders the currently-displayed topic at the given width. Called
// by guideReader.SetRect on resize so horizontal dividers re-expand. No-op for
// the placeholder.
func (gd *GuideDisplay) rerender(width int) {
	if gd.currentIdx < 0 {
		return
	}
	lines := micron.RenderToStyledLines(guideTopics[gd.currentIdx].markup, micronTheme(gd.app.Theme))
	text, links := StyledLinesToTviewText(lines, width)
	gd.currentLines = lines
	gd.links = links
	gd.anchors = micron.BuildAnchorMap(lines)
	gd.lineTexts = splitLineTexts(text)
	gd.reader.SetText(text)
}

// readerWidth is the reader's current inner column count (the wrap/divider
// width). The reader is a borderless TextView laid out inside the readerBox
// LineBox, so its GetInnerRect already excludes the readerBox border — no
// further subtraction. It falls back to 80 before the first layout.
func (gd *GuideDisplay) readerWidth() int {
	_, _, w, _ := gd.reader.GetInnerRect()
	if w <= 0 {
		return 80
	}
	return w
}

// jumpToAnchor scrolls the reader so the named anchor's line sits at the top,
// mirroring Python's Guide.jump_to_anchor (Guide.py:236-261): the scroll target
// is _rows_above(attrmaps, target_idx, cols) — the count of wrapped display
// rows preceding the anchor line. Unknown anchors are a no-op (Python:
// target_idx is None → return).
func (gd *GuideDisplay) jumpToAnchor(name string) {
	if gd.currentIdx < 0 || gd.anchors == nil {
		return
	}
	targetIdx, ok := gd.anchors.JumpTarget(name)
	if !ok {
		return
	}
	gd.reader.ScrollTo(gd.rowsAbove(targetIdx), 0)
}

// rowsAbove returns the number of wrapped display rows preceding line idx,
// mirroring Python's _rows_above (Guide.py:63-72). Each preceding line's row
// count is len(tview.WordWrap(lineText, innerWidth)) — the same word-wrap
// tview's TextView uses to draw — so the computed offset tracks the real
// rendered layout.
func (gd *GuideDisplay) rowsAbove(idx int) int {
	if idx <= 0 || len(gd.lineTexts) == 0 {
		return 0
	}
	_, _, innerW, _ := gd.reader.GetInnerRect()
	if innerW <= 0 {
		innerW = gd.readerWidth()
	}
	total := 0
	for i := 0; i < idx && i < len(gd.lineTexts); i++ {
		rows := 1
		if innerW > 0 {
			if w := len(tview.WordWrap(gd.lineTexts[i], innerW)); w > 0 {
				rows = w
			}
		}
		total += rows
	}
	return total
}

// handleLink implements GuideLinkDelegate.handle_link (Guide.py:103-118): a
// "#name" URL jumps in-page to that anchor; any other URL is an external link
// handed to OnHandleLink (which the app wires to show_network +
// browser.handle_link). An empty target is a no-op.
func (gd *GuideDisplay) handleLink(target, fields string) {
	if target == "" {
		return
	}
	if strings.HasPrefix(target, "#") {
		gd.jumpToAnchor(target[1:])
		return
	}
	if gd.OnHandleLink != nil {
		gd.OnHandleLink(target, fields)
	}
}

// activateLink resolves a tview region id to a registered link and dispatches
// handleLink (Guide.py GuideLinkDelegate.handle_link).
func (gd *GuideDisplay) activateLink(regionID string) {
	idx := 0
	for _, c := range regionID {
		if c >= '0' && c <= '9' {
			idx = idx*10 + int(c-'0')
		} else {
			return
		}
	}
	if idx < 0 || idx >= len(gd.links) {
		return
	}
	gd.handleLink(gd.links[idx].URL, gd.links[idx].Fields)
}

// micronTheme maps a tui theme constant to the micron Theme used by
// RenderToStyledLines.
func micronTheme(theme int) micron.Theme {
	if theme == ThemeLight {
		return micron.ThemeLight
	}
	return micron.ThemeDark
}

// guideReader wraps tview.TextView to re-expand horizontal dividers on resize.
type guideReader struct {
	*tview.TextView
	gd    *GuideDisplay
	lastW int
}

// newGuideReader creates a reader wrapper bound to its owning GuideDisplay.
func newGuideReader(gd *GuideDisplay) *guideReader {
	return &guideReader{TextView: tview.NewTextView(), gd: gd}
}

// SetRect overrides tview.TextView.SetRect to re-render the current topic when
// the width changes, so micron horizontal dividers always fill the pane.
func (gr *guideReader) SetRect(x, y, w, h int) {
	gr.TextView.SetRect(x, y, w, h)
	// w is the reader's own width (already inside the readerBox border), so it
	// is the wrap/divider width directly — no further border subtraction.
	if w > 0 && w != gr.lastW {
		gr.lastW = w
		gr.gd.rerender(w)
	}
}

// guideTopicPlainText returns the rendered plain text of a topic (for tests),
// with tview color/region tags removed. It strips tags by re-collecting only
// the StyledSpan Text of each rendered line (which already excludes tag chars
// via micron parsing), so it needs no tview tag stripper.
func guideTopicPlainText(markup string, theme int) string {
	lines := micron.RenderToStyledLines(markup, micronTheme(theme))
	var b strings.Builder
	for _, line := range lines {
		if line == nil {
			b.WriteByte('\n')
			continue
		}
		if line.Divider {
			b.WriteByte('\n')
			continue
		}
		for _, span := range line.Spans {
			b.WriteString(span.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
