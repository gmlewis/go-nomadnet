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

	"github.com/gdamore/tcell/v2"
)

// TestConfigDisplayExplainerColor pins the Config page explainer text color to
// the cube-quantized body_text palette entry. Python wraps the explainer in
// `urwid.Text(("body_text", ...), align=CENTER)` (Config.py:40); body_text is
// 3-hex #ddd (dark) / #222 (light) (ui/TextUI.py:26,80), cube-quantized to
// #d7d7d7 / #000000. The Go port previously used 0xbbbbbb.
func TestConfigDisplayExplainerColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
		want  uint32
	}{
		{"dark", ThemeDark, 0xd7d7d7},
		{"light", ThemeLight, 0x000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			cd := NewConfigDisplay(app, "/tmp/cfg")
			if got := uint32(cd.explainer.color.Hex()) & 0xffffff; got != tc.want {
				t.Errorf("explainer color = #%06x, want #%06x (body_text cube-quantized)", got, tc.want)
			}
		})
	}
}

// TestDirectoryDisplayDetailColor pins the Directory detail pane base color to
// body_text. Python's Directory.py is a 20-line stub whose only text uses the
// body_text attr (Directory.py:14 `urwid.Text(("body_text", ...))`); the Go
// port's richer two-pane list+detail has no direct Python color spec, so
// body_text is the closest defensible base (the prior 0xbbbbbb is a color
// Python never emits). body_text is 3-hex #ddd / #222, cube-quantized to
// #d7d7d7 / #000000.
func TestDirectoryDisplayDetailColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
		want  uint32
	}{
		{"dark", ThemeDark, 0xd7d7d7},
		{"light", ThemeLight, 0x000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			dd := NewDirectoryDisplay(app, nil)
			dd.detail.SetText("X")
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(40, 3)
			dd.detail.SetRect(0, 0, 40, 3)
			dd.detail.Draw(screen)
			if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
				t.Fatalf("detail cell (0,0) = %q, want 'X'", string(c))
			} else {
				fg, _, _ := style.Decompose()
				if got := uint32(fg.Hex()) & 0xffffff; got != tc.want {
					t.Errorf("detail base fg = #%06x, want #%06x (body_text cube-quantized)", got, tc.want)
				}
			}
		})
	}
}
