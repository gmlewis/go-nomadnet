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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// benchScreenW/benchScreenH is the terminal size used across the Draw/input
// benchmarks — a representative 120x30 desktop pane. Large enough that pages
// actually wrap and the ScrollBar overflow branch runs.
const (
	benchScreenW = 120
	benchScreenH = 30
)

// newBenchScreen returns an initialized headless SimulationScreen of the
// benchmark size. The caller defers Fini. Every Draw benchmark draws onto a
// screen created this way; the SimulationScreen has no real terminal I/O, so
// it isolates the Go-side render/CPU cost (NOT the real-terminal flush/cursor
// cost — that is what the live pprof harness in cmd/gonomadnet is for).
func newBenchScreen() tcell.SimulationScreen {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		panic(err)
	}
	screen.SetSize(benchScreenW, benchScreenH)
	return screen
}

// drawFrame replicates one iteration of the tview event loop's a.draw() — the
// work performed whenever a mouse/keyboard handler returns consumed=true:
// screen.Clear() + root.Draw(screen) + screen.Show(). Benchmarks call this per
// iteration so the measured number is the honest per-frame full-redraw cost,
// not just root.Draw in isolation.
func drawFrame(screen tcell.Screen, root tview.Primitive) {
	screen.Clear()
	root.Draw(screen)
	screen.Show()
}

// benchSyntheticMicron builds a deterministic micron document of roughly the
// given paragraph count, exercising headings, inline toggles, links and
// dividers. Used to scale Draw/render benchmarks so O(document) costs appear
// as a slope rather than a single point.
func benchSyntheticMicron(paragraphs int) string {
	var b strings.Builder
	for i := range paragraphs {
		b.WriteString(">> Heading ")
		b.WriteString(benchItoa(i))
		b.WriteString("\n\nThis is paragraph number ")
		b.WriteString(benchItoa(i))
		b.WriteString(". It has some !bold`*italic`_underline text and a link: [label`https://example.invalid/")
		b.WriteString(benchItoa(i))
		b.WriteString("] plus enough filler words to force at least one wrap at a typical pane width. The quick brown fox jumps over the lazy dog repeatedly here.\n\n")
		if i%5 == 0 {
			b.WriteString("------------------------------------------------------------\n\n")
		}
	}
	return b.String()
}

// benchItoa is a tiny dependency-free int->string so the generator's
// allocation profile reflects the renderer, not strconv.
func benchItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// benchGuideTopicMarkups returns the 12 embedded guide topic markups as the
// realistic corpus for guide-side benchmarks (accessible in-package).
func benchGuideTopicMarkups() []string {
	out := make([]string, len(guideTopics))
	for i, t := range guideTopics {
		out[i] = t.markup
	}
	return out
}
