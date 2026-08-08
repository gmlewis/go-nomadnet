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
	"strings"
	"testing"
)

// benchRealisticMarkup is a hand-built micron page exercising the features that
// dominate real NomadNet pages: headings, inline format toggles, links, an
// input field, a divider, and a small table. It is the realistic-shape input
// (distinct from the scaled synthetic page, which only grows line count).
const benchRealisticMarkup = `>> Getting Started

Welcome to NomadNet. This page exercises the !bold and *italic inline
formatting toggles as well as _underline and a couple of links: read the
[Aleph documentation` + "`" + `https://reticulum.network/manual] and the
[Discord` + "`" + `nomadnet:disc] for help.

> Section Two

Type your name here: [Your Name` + "`" + `field_name] and pick a flavor:

------------------------------------------------------------

| Option | Description               |
| ------ | ------------------------- |
| A      | The first choice, longer   |
| B      | The second choice          |

The quick brown fox jumps over the lazy dog. The quick brown fox jumps
over the lazy dog again, and again, until the line wraps at the render
width and we measure the wrap cost. `

// benchSyntheticPage builds a deterministic micron document of roughly the
// given paragraph count by concatenating heading + body + divider + link
// blocks. It is used to scale render benchmarks so O(document) costs show up as
// slope, not as a single point.
func benchSyntheticPage(paragraphs int) string {
	var b strings.Builder
	for i := range paragraphs {
		b.WriteString(">> Heading ")
		b.WriteString(itoa(i))
		b.WriteString("\n\nThis is paragraph number ")
		b.WriteString(itoa(i))
		b.WriteString(". It has some !bold`*italic`_underline text and a link: ")
		b.WriteString("[label")
		b.WriteString("`")
		b.WriteString("https://example.invalid/")
		b.WriteString(itoa(i))
		b.WriteString("] plus enough filler words to force at least one wrap at a typical pane width. The quick brown fox jumps over the lazy dog repeatedly.\n\n")
		if i%5 == 0 {
			b.WriteString("------------------------------------------------------------\n\n")
		}
	}
	return b.String()
}

// itoa is a tiny dependency-free int->string to keep the generator allocation
// profile focused on the renderer, not on strconv.
func itoa(n int) string {
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

// benchInputs is the scaled corpus: a realistic-shape page plus synthetic
// pages at three sizes so render benchmarks expose O(document) growth.
var benchInputs = []struct {
	name  string
	value string
}{
	{"Realistic", benchRealisticMarkup},
	{"Small", benchSyntheticPage(8)},
	{"Medium", benchSyntheticPage(64)},
	{"Large", benchSyntheticPage(512)},
}

// BenchmarkParse measures the micron parser in isolation (no rendering),
// mirroring the per-navigation/per-reflow parse cost. There is no parse cache
// today, so every render re-parses the whole document.
func BenchmarkParse(b *testing.B) {
	for _, tc := range benchInputs {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.value)))
			for b.Loop() {
				_ = Parse(tc.value)
			}
		})
	}
}

// BenchmarkRenderToStyledLines measures the whole 24-bit render path (parse +
// render to styled lines) that Browser/Guide call on every navigation and on
// every width-change reflow. This is the dominant non-Draw render cost.
func BenchmarkRenderToStyledLines(b *testing.B) {
	for _, tc := range benchInputs {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.value)))
			for b.Loop() {
				_ = RenderToStyledLines(tc.value, ThemeDark)
			}
		})
	}
}

// BenchmarkRenderToTView measures the decomposed render path
// (Parse then RenderToTView), letting Parse and the tview-text emit be
// compared against the combined RenderToStyledLines number.
func BenchmarkRenderToTView(b *testing.B) {
	for _, tc := range benchInputs {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.value)))
			for b.Loop() {
				nodes := Parse(tc.value)
				_ = RenderToTView(nodes)
			}
		})
	}
}

// BenchmarkBuildAnchorMap measures anchor-map construction over pre-rendered
// lines (built once outside the b.N loop), isolating it from the render cost.
func BenchmarkBuildAnchorMap(b *testing.B) {
	for _, tc := range benchInputs {
		lines := RenderToStyledLines(tc.value, ThemeDark)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.value)))
			for b.Loop() {
				_ = BuildAnchorMap(lines)
			}
		})
	}
}
