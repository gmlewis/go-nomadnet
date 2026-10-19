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

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/tcell/v2"
)

// TestBrowserFetchStageReachesTheFooter pins the wiring from a fetch lifecycle
// stage to the footer the user sees. Python renders one fixed label per status
// (SOT: ui/textui/Browser.py:1756-1803, nomadnet 1.2.8); the stage mapping must
// land on exactly those words, and a stage Python transitions through silently
// (path-known) must leave the footer alone.
func TestBrowserFetchStageReachesTheFooter(t *testing.T) {
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

	tests := []struct {
		name  string
		stage browser.FetchStage
		want  string
	}{
		{"no path", browser.FetchStageNoPath, "No path to destination known"},
		{"path requested", browser.FetchStagePathRequested, "Path requested, waiting for path..."},
		{"path known", browser.FetchStagePathKnown, "Done"},
		{"establishing link", browser.FetchStageEstablishingLink, "Establishing link..."},
		{"link established", browser.FetchStageLinkEstablished, "Link established"},
		{"requesting", browser.FetchStageRequesting, "Sending request..."},
		{"request sent", browser.FetchStageRequestSent, "Request sent, awaiting response..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Start from DONE so the path-known no-op case has an observable
			// "unchanged" value distinct from each stage's label.
			bd.SetBrowserStatus(int(browserDone))
			bd.SetBrowserFetchStage(tc.stage)
			if got := bd.FooterText(); got != tc.want {
				t.Errorf("footer after stage %v = %q, want %q", tc.stage, got, tc.want)
			}
		})
	}
}

// TestLinkPeekGatedOnDone pins Python's marked_link status gate
// (Browser.py:177-179): moving the cursor onto a link while a fetch is in
// flight must not replace the in-flight footer status with "Link to ...".
func TestLinkPeekGatedOnDone(t *testing.T) {
	t.Parallel()

	bd := newErrorPageTestDisplay(t)
	bd.SetBrowserFetchStage(browser.FetchStageEstablishingLink)
	bd.MarkedLink("abcdef01:/page/index.mu", "")
	if got := bd.FooterText(); got != "Establishing link..." {
		t.Errorf("footer during a fetch after a link peek = %q, want the in-flight status", got)
	}

	bd.SetBrowserStatus(int(browserDone))
	bd.MarkedLink("abcdef01:/page/index.mu", "")
	if got := bd.FooterText(); !strings.HasPrefix(got, "Link to ") {
		t.Errorf("footer after a completed load and a link peek = %q, want a Link-to peek", got)
	}
}
