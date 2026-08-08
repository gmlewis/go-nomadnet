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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// =============================================================================
// Layer 2 — display-bridge pure functions (no Screen, no App)
// =============================================================================

// BenchmarkStyledLinesToTviewText measures the styled-lines → tview color-tag
// string bridge (tui/styled-tview.go:42) called on every navigation and every
// width-change reflow by both Browser and Guide. Lines are built once outside
// the b.N loop so only the conversion is measured. Width is varied because the
// bridge expands horizontal dividers to the given width.
func BenchmarkStyledLinesToTviewText(b *testing.B) {
	markup := benchSyntheticMicron(64)
	lines := micron.RenderToStyledLines(markup, micronTheme(ThemeDark))
	for _, width := range []int{40, 80, 120} {
		b.Run(widthLabel(width), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(markup)))
			for b.Loop() {
				_, _ = StyledLinesToTviewText(lines, width)
			}
		})
	}
}

func widthLabel(w int) string {
	switch w {
	case 40:
		return "W40"
	case 80:
		return "W80"
	default:
		return "W120"
	}
}

// BenchmarkBodyMarkup measures the message-body parser (tui/rendering.go:88)
// used to render chat/channel message bodies. Links + mentions are the hot
// part; the input mixes plain text, a URL and an @mention.
func BenchmarkBodyMarkup(b *testing.B) {
	body := "Hello @alice, check https://example.invalid/path?x=1 and the lxmf://addr message body, plus some plain trailing words."
	own := "bob"
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		_, _ = BodyMarkup(body, ThemeDark, own)
	}
}

// BenchmarkFormatChannelMessage measures the per-message channel formatter
// (tui/rendering.go:26), called once per rendered message row.
func BenchmarkFormatChannelMessage(b *testing.B) {
	msg := ChannelMessage{
		Nick: "alice",
		Text: "a normal channel message with enough words to be representative",
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = FormatChannelMessage(msg, ThemeDark)
	}
}

// BenchmarkFormatConversationItem measures the conversation-list row formatter
// (tui/rendering.go:45), called once per conversation list entry.
func BenchmarkFormatConversationItem(b *testing.B) {
	conv := ConversationInfo{
		DisplayName: "Alice Q. Example",
		LastMessage: "the last received message preview text",
		TrustLevel:  "trusted",
		Unread:      true,
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = FormatConversationItem(conv, ThemeDark)
	}
}

// =============================================================================
// Layer 3 — widget-level Draw (the per-frame cost the event loop pays)
// =============================================================================

// BenchmarkScrollBarDraw is the CONFIRMED guide-scroll hot path. ScrollBar.Draw
// (tui/scroll-bar.go:152) runs wrappedRowCount(s.Text.GetText(true), cw) on
// EVERY frame: GetText(true) strips all color/region tags from the ENTIRE
// document (O(document)), then wrappedRowCount runs tview.WordWrap per line
// (O(document lines × width)). Nothing is cached, so one full re-wrap happens
// per wheel notch. The sub-benchmarks by document size should scale ~linearly —
// that slope is the smoking gun.
func BenchmarkScrollBarDraw(b *testing.B) {
	for _, n := range []int{8, 64, 256} {
		b.Run(sizeLabel(n), func(b *testing.B) {
			app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
			gd := NewGuideDisplay(app)
			gd.showMarkupForTest(benchSyntheticMicron(n))

			screen := newBenchScreen()
			defer screen.Fini()
			gd.scroll.SetRect(0, 0, benchScreenW, benchScreenH)
			// Warm up: the first Draw builds the TextView line index (a one-time
			// parseAhead); the per-wheel-notch cost we want is steady-state, with
			// the index cached — so draw once before timing.
			gd.scroll.Draw(screen)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				gd.scroll.Draw(screen)
			}
		})
	}
}

// BenchmarkGuideReaderDraw measures the guide reader's per-frame Draw alone:
// guideReader.Draw (tui/guide.go:589) runs TextView.Draw (the cached-index
// visible-line render) plus ShowCursor when the cursor is visible. Isolates
// the TextView.Draw cost from the ScrollBar's per-frame re-wrap above.
func BenchmarkGuideReaderDraw(b *testing.B) {
	for _, n := range []int{8, 64, 256} {
		b.Run(sizeLabel(n), func(b *testing.B) {
			app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
			gd := NewGuideDisplay(app)
			gd.showMarkupForTest(benchSyntheticMicron(n))

			screen := newBenchScreen()
			defer screen.Fini()
			gd.reader.SetRect(0, 0, benchScreenW-1, benchScreenH)
			gd.reader.Draw(screen) // warm up the line index

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				gd.reader.Draw(screen)
			}
		})
	}
}

// BenchmarkBrowserPageViewDraw measures the browser content widget's per-frame
// Draw: browserPageView.Draw (tui/browser-nav.go:67) runs TextView.Draw plus
// drawCursor (a no-op during mouse-wheel scroll — gated on a keypress) plus
// reflowIfWidthChanged (a no-op once width converges). The page is loaded via
// the pure render chain so no fetch timers are started.
func BenchmarkBrowserPageViewDraw(b *testing.B) {
	for _, n := range []int{8, 64, 256} {
		b.Run(sizeLabel(n), func(b *testing.B) {
			app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
			bd := NewBrowserDisplay(app)
			markup := benchSyntheticMicron(n)
			lines := micron.RenderToStyledLines(markup, micronTheme(app.Theme))
			text, _ := StyledLinesToTviewText(lines, benchScreenW-2)
			bd.content.SetText(text)
			bd.renderedWidth = benchScreenW - 2

			screen := newBenchScreen()
			defer screen.Fini()
			bd.content.SetRect(0, 0, benchScreenW-2, benchScreenH-4)
			bd.content.Draw(screen) // warm up the line index

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				bd.content.Draw(screen)
			}
		})
	}
}

// BenchmarkTextViewDrawWrappedLarge isolates tview's own TextView.Draw on a
// large, tag-dense, wrapped text — the shared engine under both browser and
// guide content. The sub-benchmarks test the suspected browser-sluggishness
// mechanism: tview resets the whole line index whenever width changes while
// wrap is on (textview.go:1061), and parseAhead then rebuilds from line 0 up
// to the scroll offset. "DeepOffset" measures a deep scroll at a stable width
// (index cached → baseline); "WidthOscillate" toggles the width by 1 every
// frame to force a reset each draw — if it is dramatically slower, that
// confirms the resetIndex→parseAhead O(scroll-position) cost.
func BenchmarkTextViewDrawWrappedLarge(b *testing.B) {
	markup := benchSyntheticMicron(256)
	lines := micron.RenderToStyledLines(markup, micronTheme(ThemeDark))
	text, _ := StyledLinesToTviewText(lines, benchScreenW)

	b.Run("TopOffset", func(b *testing.B) {
		tv := tview.NewTextView()
		tv.SetDynamicColors(true).SetRegions(true).SetWrap(true)
		tv.SetText(text)
		screen := newBenchScreen()
		defer screen.Fini()
		tv.SetRect(0, 0, benchScreenW, benchScreenH)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			tv.Draw(screen)
		}
	})

	b.Run("DeepOffset", func(b *testing.B) {
		tv := tview.NewTextView()
		tv.SetDynamicColors(true).SetRegions(true).SetWrap(true)
		tv.SetText(text)
		screen := newBenchScreen()
		defer screen.Fini()
		tv.SetRect(0, 0, benchScreenW, benchScreenH)
		// Parse the index once at full height so ScrollTo is meaningful, then
		// jump deep so the index reaches a large offset.
		tv.Draw(screen)
		tv.ScrollTo(500, 0)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			tv.Draw(screen)
		}
	})

	b.Run("WidthOscillate", func(b *testing.B) {
		tv := tview.NewTextView()
		tv.SetDynamicColors(true).SetRegions(true).SetWrap(true)
		tv.SetText(text)
		screen := newBenchScreen()
		defer screen.Fini()
		tv.SetRect(0, 0, benchScreenW, benchScreenH)
		tv.Draw(screen)
		tv.ScrollTo(500, 0)
		alt := false
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			w := benchScreenW
			if alt {
				w = benchScreenW - 1
			}
			alt = !alt
			tv.SetRect(0, 0, w, benchScreenH)
			tv.Draw(screen)
		}
	})
}

// sizeLabel returns a short label for a document-size sub-benchmark.
func sizeLabel(n int) string {
	switch n {
	case 8:
		return "S"
	case 64:
		return "M"
	default:
		return "L"
	}
}
