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

	"github.com/gmlewis/tcell/v2"
)

// TestBrowserHistoryCarriesRequestData pins that Back/Forward/Reload re-issue a
// fetch with the request data the original navigation carried. Python keeps
// [destination_hash, path, request_data] per history entry (Browser.py:1069-1071)
// and back()/forward() pass it through (Browser.py:1085,1095); the port stored
// URLs only, so returning to a form-submitted page re-fetched the
// unparameterised page.
func TestBrowserHistoryCarriesRequestData(t *testing.T) {
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
	type call struct {
		url string
		rd  map[string]string
	}
	var calls []call
	bd.OnRetrieveURL = func(url string, rd map[string]string) {
		calls = append(calls, call{url, rd})
	}

	rd := map[string]string{"var_query": "hello"}
	bd.pushHistory("hash:/page/index.mu", nil)
	bd.pushHistory("hash:/page/results.mu", rd)
	// The entry must snapshot the map: a later form edit must not rewrite what
	// Back re-sends.
	rd["var_query"] = "changed"

	bd.GoBack()
	if len(calls) != 1 || calls[0].url != "hash:/page/index.mu" || calls[0].rd != nil {
		t.Fatalf("Back call = %+v, want the first URL with nil request data", calls)
	}

	bd.GoForward()
	if len(calls) != 2 || calls[1].url != "hash:/page/results.mu" {
		t.Fatalf("Forward call = %+v, want the results URL", calls)
	}
	if len(calls[1].rd) == 0 || calls[1].rd["var_query"] != "hello" {
		t.Errorf("Forward request data = %v, want the recorded var_query=hello", calls[1].rd)
	}

	bd.Reload()
	if len(calls) != 3 {
		t.Fatalf("Reload did not re-issue the fetch: calls=%+v", calls)
	}
	if calls[2].rd["var_query"] != "hello" {
		t.Errorf("Reload request data = %v, want the recorded var_query=hello", calls[2].rd)
	}

	// A new navigation truncates the forward entries and their request data
	// together, so a later Forward cannot resurrect stale parameters.
	bd.GoBack()
	bd.pushHistory("hash:/page/other.mu", nil)
	if len(bd.history) != len(bd.histRequestData) {
		t.Fatalf("history/request-data lengths diverged: %v vs %v", len(bd.history), len(bd.histRequestData))
	}
	if got := len(bd.history); got != 2 {
		t.Errorf("history length after a new navigation = %v, want 2", got)
	}
}
