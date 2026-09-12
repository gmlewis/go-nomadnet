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
	"github.com/rivo/tview"
)

// fieldPageProbe drives the capture chain of the Network page, which hosts a
// browser pane (and therefore form fields) inside a container that also
// declares page-wide shortcuts (network.go:313-317). It reproduces the live
// dispatch order, where tview applies every container's InputCapture from the
// app down to the focused widget (tview flex.go InputHandler delegates to the
// focused child; application.go runs the app capture first).
type fieldPageProbe struct {
	app *App
	md  *MainDisplay
	nd  *NetworkDisplay
	bd  *BrowserDisplay

	urlDialog  int
	editNode   int
	disconnect int
	copyURL    int
}

// newFieldPageProbe builds a Network page whose browser pane has finished
// rendering page, with the page body focused and the first form field in edit
// mode — the state a visitor is in while typing into a form.
func newFieldPageProbe(t *testing.T, page string) *fieldPageProbe {
	t.Helper()
	p := &fieldPageProbe{}
	p.app = newTestApp()
	p.md = NewMainDisplay(p.app, ThemeDark, GlyphUnicode)
	p.app.Main = p.md
	p.nd = NewNetworkDisplay(p.app, nil, nil)
	p.bd = p.nd.BrowserDisplay()
	if p.bd == nil {
		t.Fatal("Network page has no browser pane")
	}
	p.bd.OnURLDialog = func() { p.urlDialog++ }
	p.nd.OnURLDialog = func() { p.urlDialog++ }
	p.nd.OnEditNode = func() { p.editNode++ }
	p.nd.OnDisconnect = func() { p.disconnect++ }
	p.bd.OnCopyURL = func() { p.copyURL++ }

	p.bd.content.SetRect(0, 0, 80, 6)
	p.app.SetFocus(p.bd.content)
	p.bd.currentMarkup = page
	p.bd.renderPage()
	return p
}

// dispatch runs one key through the capture chain in tview's order — the app
// capture (MainDisplay), then the Network page's container capture, then the
// browser pane — stopping at the first handler that consumes it.
func (p *fieldPageProbe) dispatch(k tcell.Key, r rune) *tcell.EventKey {
	event := key(k, r)
	if event = p.md.handleInput(event); event == nil {
		return nil
	}
	if event = p.nd.handleInput(event); event == nil {
		return nil
	}
	return p.bd.handleInput(event)
}

// typeText sends each rune through the same chain so the text lands in the
// mounted field.
func (p *fieldPageProbe) typeText(s string) {
	for _, r := range s {
		if p.dispatch(tcell.KeyRune, r) != nil {
			panic("rune not consumed by the mounted field")
		}
	}
}

// TestPageShortcutsDoNotStealReadlineKeysFromField pins urwid's key ordering
// for a form field inside a page that declares page-wide shortcuts.
//
// In Python the shortcuts live in the WIDGETS that own them: the Network page's
// left column is NetworkLeftPile (Network.py:1600-1610) and the right column is
// BrowserFrame (Browser.py:21-40), and urwid delivers a key to the focused
// widget first, bubbling up only when that widget passes the key on. So a form
// field — a ReadlineEdit with the readline kill keys (ReadlineEdit.py:30-70) —
// receives ctrl u, ctrl k, ctrl w, ctrl l, ctrl e and ctrl y first, and the page
// shortcut only runs when no field has the key.
//
// The Go port installs each page's shortcuts as a tview InputCapture on a
// container (network.go:317 is mainCols, which holds BOTH the left column and
// the browser pane), and tview captures run BEFORE the focused widget. The page
// therefore stole the editing keys from the visitor's field: ctrl u opened the
// URL dialog instead of clearing to the beginning of the line, ctrl e opened
// Edit Node instead of jumping to the end of the line, ctrl w disconnected
// instead of deleting a word, and ctrl l toggled the list instead of clearing
// the field — the live report that the text could not be cleared.
func TestPageShortcutsDoNotStealReadlineKeysFromField(t *testing.T) {
	t.Parallel()

	t.Run("ctrl-U kills to the beginning of the line", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, guestbookTestPage)
		if p.bd.fieldOverlay == nil {
			t.Fatal("no field in edit mode")
		}
		p.typeText("Glenn")

		if ev := p.dispatch(tcell.KeyCtrlU, 0); ev != nil {
			t.Error("Ctrl-U not consumed while a field is in edit mode")
		}
		if got, want := p.bd.fieldOverlay.GetText(), ""; got != want {
			t.Errorf("field text after Ctrl-U = %q, want %q (the page shortcut stole the key)", got, want)
		}
		if p.urlDialog != 0 {
			t.Errorf("Ctrl-U opened the URL dialog %v time(s) while a form field was in edit mode", p.urlDialog)
		}
	})

	t.Run("ctrl-E jumps to the end of the line", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, guestbookTestPage)
		p.typeText("Glenn")
		if ev := p.dispatch(tcell.KeyCtrlA, 0); ev != nil {
			t.Error("Ctrl-A not consumed while a field is in edit mode")
		}
		if got := p.bd.fieldOverlay.cursorPos; got != 0 {
			t.Fatalf("cursor after Ctrl-A = %v, want 0", got)
		}

		if ev := p.dispatch(tcell.KeyCtrlE, 0); ev != nil {
			t.Error("Ctrl-E not consumed while a field is in edit mode")
		}
		if got, want := p.bd.fieldOverlay.cursorPos, 5; got != want {
			t.Errorf("cursor after Ctrl-E = %v, want %v (end of line)", got, want)
		}
		if p.editNode != 0 {
			t.Errorf("Ctrl-E opened Edit Node %v time(s) while a form field was in edit mode", p.editNode)
		}
	})

	t.Run("ctrl-W kills the previous word", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, guestbookTestPage)
		p.typeText("Glenn Miller")

		if ev := p.dispatch(tcell.KeyCtrlW, 0); ev != nil {
			t.Error("Ctrl-W not consumed while a field is in edit mode")
		}
		if got, want := p.bd.fieldOverlay.GetText(), "Glenn "; got != want {
			t.Errorf("field text after Ctrl-W = %q, want %q", got, want)
		}
		if p.disconnect != 0 {
			t.Errorf("Ctrl-W disconnected %v time(s) while a form field was in edit mode", p.disconnect)
		}
	})

	t.Run("ctrl-L kills the whole buffer", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, guestbookTestPage)
		p.typeText("Glenn")
		showing := p.nd.showingNodes

		if ev := p.dispatch(tcell.KeyCtrlL, 0); ev != nil {
			t.Error("Ctrl-L not consumed while a field is in edit mode")
		}
		if got, want := p.bd.fieldOverlay.GetText(), ""; got != want {
			t.Errorf("field text after Ctrl-L = %q, want %q", got, want)
		}
		if p.nd.showingNodes != showing {
			t.Error("Ctrl-L toggled the node list while a form field was in edit mode")
		}
	})

	// The browser pane's own shortcut bar binds Ctrl-Y to "Copy URL"
	// (cmd/gonomadnet/textui.go:2187), which collides with the readline yank.
	// The focused field wins, as it does in Python, where the Edit's ReadlineMixin
	// takes ctrl y before BrowserFrame.keypress sees it.
	t.Run("ctrl-Y yanks instead of copying the URL", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, guestbookTestPage)
		p.typeText("Glenn")
		if ev := p.dispatch(tcell.KeyCtrlU, 0); ev != nil {
			t.Fatal("Ctrl-U not consumed while a field is in edit mode")
		}

		if ev := p.dispatch(tcell.KeyCtrlY, 0); ev != nil {
			t.Error("Ctrl-Y not consumed while a field is in edit mode")
		}
		if got, want := p.bd.fieldOverlay.GetText(), "Glenn"; got != want {
			t.Errorf("field text after Ctrl-Y = %q, want %q (the killed text)", got, want)
		}
		if p.copyURL != 0 {
			t.Errorf("Ctrl-Y copied the URL %v time(s) while a form field was in edit mode", p.copyURL)
		}
	})

	// The field is offered the same event twice — once by the app-level
	// dispatch, once by the browser pane's own capture — and handleKey re-syncs
	// its model cursor for a plain Left/Right it passes on, so a double dispatch
	// would move the cursor two columns.
	t.Run("plain Left moves the cursor exactly one column", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, guestbookTestPage)
		p.typeText("Glenn")
		if got := p.bd.fieldOverlay.cursorPos; got != 5 {
			t.Fatalf("cursor after typing = %v, want 5", got)
		}

		p.dispatch(tcell.KeyLeft, 0)
		if got, want := p.bd.fieldOverlay.cursorPos, 4; got != want {
			t.Errorf("cursor after one Left = %v, want %v (the same event was dispatched twice)", got, want)
		}
	})

	// An in-tree field is reached through tview's own focus rather than the
	// App's registration: the Network page's capture spans its left column too,
	// so the announce-stream search box would otherwise lose these keys.
	t.Run("focused in-tree ReadlineEdit keeps the readline keys", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, ">No fields here\n\njust text\n")
		re := NewReadlineEdit(p.app.killRing, "Search: ", "")
		re.SetText("Glenn")
		p.app.SetFocus(tview.NewFlex().AddItem(re, 0, 1, true))

		if ev := p.dispatch(tcell.KeyCtrlU, 0); ev != nil {
			t.Error("Ctrl-U not consumed by the focused in-tree field")
		}
		if got, want := re.GetText(), ""; got != want {
			t.Errorf("in-tree field text after Ctrl-U = %q, want %q", got, want)
		}
		if p.urlDialog != 0 {
			t.Errorf("Ctrl-U opened the URL dialog %v time(s) while an in-tree field had the focus", p.urlDialog)
		}
	})

	// Edit-mode registration lapses when the owner pane loses the focus, so an
	// overlay still mounted after a mouse click into another pane cannot keep
	// taking the page's keys.
	t.Run("a mounted overlay that lost the focus does not claim keys", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, guestbookTestPage)
		if p.bd.fieldOverlay == nil {
			t.Fatal("no field in edit mode")
		}
		p.app.SetFocus(tview.NewBox())
		if fe := p.app.FieldEditor(); fe != nil {
			t.Error("FieldEditor still returns the overlay after its owner lost the focus")
		}

		if ev := p.dispatch(tcell.KeyCtrlU, 0); ev != nil {
			t.Error("Ctrl-U not consumed by the page shortcut")
		}
		if p.urlDialog != 1 {
			t.Errorf("URL dialog opened %v time(s) after the field lost the focus, want 1", p.urlDialog)
		}
	})

	// The page shortcuts must still work when no field is in edit mode: that is
	// the state Python reaches when the focused widget passes the key on.
	t.Run("page shortcuts still fire with no field in edit mode", func(t *testing.T) {
		t.Parallel()
		p := newFieldPageProbe(t, ">No fields here\n\njust text\n")
		if p.bd.fieldOverlay != nil {
			t.Fatal("a field is in edit mode on a page with no fields")
		}

		if ev := p.dispatch(tcell.KeyCtrlU, 0); ev != nil {
			t.Error("Ctrl-U not consumed by the page shortcut")
		}
		if p.urlDialog != 1 {
			t.Errorf("URL dialog opened %v time(s) with no field in edit mode, want 1", p.urlDialog)
		}

		showing := p.nd.showingNodes
		if ev := p.dispatch(tcell.KeyCtrlL, 0); ev != nil {
			t.Error("Ctrl-L not consumed by the page shortcut")
		}
		if p.nd.showingNodes == showing {
			t.Error("Ctrl-L did not toggle the node list with no field in edit mode")
		}
	})
}
