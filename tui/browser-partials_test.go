// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"slices"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
)

// TestBrowserPartialsTrackedAndSelectable pins Python Browser.handle_partial_updates
// (Browser.py:823-834): a "p:<id>:<id>" link forces a refresh of the page's
// partials whose id is among the link's ids. RenderPage must track EVERY
// declared partial (including ones with no auto-refresh interval, which Python
// still iterates in page_partials), and partialsToRefresh must select exactly
// the matching ids — a partial with no pid (empty id) matches only when "" is
// among the requested ids (Python: `partial["id"] in partial_ids`).
func TestBrowserPartialsTrackedAndSelectable(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 12)

	// Three partials: sb (refresh 5), hd (refresh 5), and one with no pid.
	// OnFetchPartial is nil, so no refresh goroutines start, but RenderPage must
	// still track all three for force-refresh selection.
	bd.RenderPage(">Page\n`{url1`5`pid=sb}\n`{url2`5`pid=hd}\n`{url3`5}\nEnd")

	if got := len(bd.partials); got != 3 {
		t.Fatalf("bd.partials = %v partials, want 3 (all declared partials tracked)", got)
	}

	ids := func(ps []browser.Partial) []string {
		out := make([]string, 0, len(ps))
		for _, p := range ps {
			out = append(out, p.ID)
		}
		return out
	}

	if got, want := len(bd.partialsToRefresh([]string{"sb"})), 1; got != want {
		t.Errorf("partialsToRefresh([sb]) = %v, want %v", got, want)
	}
	if got, want := len(bd.partialsToRefresh([]string{"sb", "hd"})), 2; got != want {
		t.Errorf("partialsToRefresh([sb,hd]) = %v, want %v", got, want)
	}
	if got, want := len(bd.partialsToRefresh([]string{"nope"})), 0; got != want {
		t.Errorf("partialsToRefresh([nope]) = %v, want %v", got, want)
	}
	// The pid-less partial (empty id) is not selected by named ids.
	got := ids(bd.partialsToRefresh([]string{"sb", "hd"}))
	if sliceContains(got, "") {
		t.Errorf("pid-less partial selected by named ids: %v", got)
	}
	// But it IS selected when "" is explicitly requested.
	if got, want := len(bd.partialsToRefresh([]string{""})), 1; got != want {
		t.Errorf("partialsToRefresh([\"\"]) = %v, want %v (pid-less partial matches empty id)", got, want)
	}
}

func sliceContains(s []string, v string) bool {
	return slices.Contains(s, v)
}
