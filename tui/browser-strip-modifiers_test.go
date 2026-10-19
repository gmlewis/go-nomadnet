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

	"github.com/gmlewis/tcell/v2"
)

// TestRenderPageStripsModifiers pins that a fetched page is rendered through
// strip_modifiers, as Python does before parsing (Browser.py:1269,1348,1513):
// CRLF line ends, zero-width joiners and emoji modifiers never reach the
// parse, so a CRLF page does not keep a '\r' at every line end.
func TestRenderPageStripsModifiers(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	app.SetScreen(screen)
	screen.SetSize(100, 30)

	bd := NewBrowserDisplay(app)
	bd.RenderPage("first line\r\nsecond‍ line\r\n")

	var all strings.Builder
	for _, line := range bd.currentLines {
		for _, span := range line.Spans {
			all.WriteString(span.Text)
		}
		all.WriteByte('\n')
	}
	got := all.String()
	if strings.ContainsRune(got, '\r') {
		t.Errorf("rendered page still contains CR: %q", got)
	}
	if strings.ContainsRune(got, '‍') {
		t.Errorf("rendered page still contains a zero-width joiner: %q", got)
	}
	if !strings.Contains(got, "first line") || !strings.Contains(got, "second line") {
		t.Errorf("rendered page lost text while stripping modifiers: %q", got)
	}
}
