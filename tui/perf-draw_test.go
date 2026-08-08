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
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// =============================================================================
// Layer 4 — full-page end-to-end Draw (broad sweep across every body page)
//
// Each benchmark mounts one body page into a real MainDisplay and measures the
// per-frame cost the tview event loop pays whenever ANY consumed event fires:
// screen.Clear() + MainDisplay.Root().Draw() + screen.Show(). This is the whole
// tree (menu bar + content page + shortcut bar), so it surfaces pages whose
// full-tree Draw is expensive even outside the reported scroll/flicker
// symptoms — the "broad sweep" requested.
// =============================================================================

// benchPageDraw mounts the page built by build into a fresh MainDisplay,
// selects it, and measures full-tree drawFrame cost per iteration.
func benchPageDraw(b *testing.B, key string, build func(app *App) tview.Primitive) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	widget := build(app)
	app.Main.SetDisplay(key, widget)
	app.Main.SelectPage(key)

	screen := newBenchScreen()
	defer screen.Fini()
	root := app.Main.Root()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	// Settle focus/rects so the measured frames are steady-state, not first-paint.
	drawFrame(screen, root)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		drawFrame(screen, root)
	}
}

// benchLogPath writes a throwaway log file to /tmp and returns its path. The
// Log display tails it at construction; a real (small) file exercises the
// text-load path without depending on the user's environment. (On macOS we
// use /tmp explicitly, never os.MkdirTemp("").)
func benchLogPath(b *testing.B) string {
	dir := "/tmp/gonomadnet-bench"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatalf("mkdir: %v", err)
	}
	b.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "bench.log")
	var buf []byte
	for range 256 {
		buf = append(buf, "[2026-08-08 12:00:00] [Notice] a representative log line with enough words to wrap\n"...)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		b.Fatalf("write: %v", err)
	}
	return path
}

func BenchmarkMainDisplayDrawGuide(b *testing.B) {
	benchPageDraw(b, "guide", func(app *App) tview.Primitive {
		gd := NewGuideDisplay(app)
		// Load a long topic so the reader/scrollbar actually wrap a large document.
		gd.showMarkupForTest(benchSyntheticMicron(64))
		return gd.Widget()
	})
}

// BenchmarkStandaloneBrowserDraw measures the full BrowserDisplay widget tree
// (url header + dividers + body content + footer status) per frame, with a
// large page loaded via the pure render chain so no fetch timers start.
func BenchmarkStandaloneBrowserDraw(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	bd := NewBrowserDisplay(app)
	markup := benchSyntheticMicron(64)
	lines := micron.RenderToStyledLines(markup, micronTheme(app.Theme))
	text, _ := StyledLinesToTviewText(lines, benchScreenW-2)
	bd.content.SetText(text)
	bd.renderedWidth = benchScreenW - 2

	screen := newBenchScreen()
	defer screen.Fini()
	root := bd.Widget()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	drawFrame(screen, root)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		drawFrame(screen, root)
	}
}

func BenchmarkMainDisplayDrawNetwork(b *testing.B) {
	benchPageDraw(b, "network", func(app *App) tview.Primitive {
		// A handful of entries so the left/announce panes render real content.
		announces := []AnnounceEntry{
			{DisplayName: "Example Node Alpha", TrustLevel: "trusted", Type: "node"},
			{DisplayName: "Example Node Beta", TrustLevel: "untrusted", Type: "node"},
		}
		nodes := []NodeEntry{
			{DisplayName: "node-one", TrustLevel: "trusted", HostsNode: true},
			{DisplayName: "node-two", TrustLevel: "untrusted", HostsNode: false},
		}
		nd := NewNetworkDisplay(app, announces, nodes)
		return nd.Widget()
	})
}

func BenchmarkMainDisplayDrawChannels(b *testing.B) {
	benchPageDraw(b, "channels", func(app *App) tview.Primitive {
		cd := NewChannelsDisplay(app, nil)
		return cd.Widget()
	})
}

func BenchmarkMainDisplayDrawLog(b *testing.B) {
	path := benchLogPath(b)
	benchPageDraw(b, "log", func(app *App) tview.Primitive {
		ld := NewLogDisplay(app, path, 200)
		return ld.Widget()
	})
}

func BenchmarkMainDisplayDrawInterfaces(b *testing.B) {
	benchPageDraw(b, "interfaces", func(app *App) tview.Primitive {
		id := NewInterfacesDisplay(app, []InterfaceInfo{
			{Name: "Interface1", Type: "AutoInterface", Status: "connected"},
		})
		return id.Widget()
	})
}

func BenchmarkMainDisplayDrawConfig(b *testing.B) {
	benchPageDraw(b, "config", func(app *App) tview.Primitive {
		cd := NewConfigDisplay(app, "/tmp/gonomadnet-bench/reticulum.conf")
		return cd.Widget()
	})
}

func BenchmarkMainDisplayDrawConversations(b *testing.B) {
	benchPageDraw(b, "conversations", func(app *App) tview.Primitive {
		convs := []ConversationInfo{
			{DisplayName: "Alice", LastMessage: "last message preview", TrustLevel: "trusted", Unread: true},
			{DisplayName: "Bob", LastMessage: "another preview", TrustLevel: "untrusted"},
		}
		cd := NewConversationsDisplay(app, convs)
		return cd.Widget()
	})
}

// BenchmarkGuideTopicCorpusDraw draws the full Guide page once per embedded
// topic (all 12), exposing which real-world topics are most expensive to
// render+draw end-to-end. Uses the guideTopics corpus directly.
func BenchmarkGuideTopicCorpusDraw(b *testing.B) {
	for _, markup := range benchGuideTopicMarkups() {
		b.Run("topic", func(b *testing.B) {
			app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
			gd := NewGuideDisplay(app)
			gd.showMarkupForTest(markup)
			app.Main.SetDisplay("guide", gd.Widget())
			app.Main.SelectPage("guide")

			screen := newBenchScreen()
			defer screen.Fini()
			root := app.Main.Root()
			root.SetRect(0, 0, benchScreenW, benchScreenH)
			drawFrame(screen, root)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				drawFrame(screen, root)
			}
		})
	}
}
