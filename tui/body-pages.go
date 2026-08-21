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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// bodyPages is a tview.Pages wrapper used for the MainDisplay content area. It
// exists to fix a focus-stealing bug in tview.Pages.InputHandler
// (pages.go:329-340): that handler dispatches a key event to the FIRST page
// whose Item.HasFocus() is true, iterating ALL pages regardless of visibility.
//
// Application.SetFocus only calls Blur() on the single previously-focused leaf
// (application.go:838-855) — it does NOT blur the containers of a
// previously-displayed page. So after SwitchToPage a hidden page's widget tree
// retains stale HasFocus()==true, and tview.Pages.InputHandler routes the next
// key to that hidden page instead of the now-visible one. In the Go port this
// was the root cause of the Guide page never receiving keys after switching
// from Network to Guide: Down/Enter were dispatched to the
// hidden Network page's pileFiller/list, which blurred the guide topic list and
// prevented any topic from rendering.
//
// bodyPages overrides InputHandler to dispatch only to a page that is BOTH
// visible and HasFocus, mirroring Python urwid's single Frame.focus_position
// model (only the visible body receives input). It tracks each page's
// visibility in its own refs slice (AddPage/SwitchToPage keep it in sync with
// the embedded *tview.Pages, which still owns drawing and layout).
type bodyPages struct {
	*tview.Pages
	refs []*bodyPageRef
}

// bodyPageRef is bodyPages' own per-page record, parallel to tview.Pages'
// private pages slice so bodyPages can read visibility without accessing
// tview internals.
type bodyPageRef struct {
	name    string
	item    tview.Primitive
	visible bool
}

// newBodyPages returns an empty bodyPages backed by a fresh tview.Pages.
func newBodyPages() *bodyPages {
	return &bodyPages{Pages: tview.NewPages()}
}

// AddPage appends a page and records it in the parallel refs slice. It shadows
// tview.Pages.AddPage so callers transparently keep bodyPages' tracking in sync.
func (b *bodyPages) AddPage(name string, item tview.Primitive, resize, visible bool) *bodyPages {
	b.Pages.AddPage(name, item, resize, visible)
	b.refs = append(b.refs, &bodyPageRef{name: name, item: item, visible: visible})
	return b
}

// SwitchToPage makes only the named page visible, updating both the embedded
// tview.Pages (drawing/layout) and bodyPages' refs (input dispatch).
func (b *bodyPages) SwitchToPage(name string) *bodyPages {
	b.Pages.SwitchToPage(name)
	for _, r := range b.refs {
		r.visible = r.name == name
	}
	return b
}

// InputHandler dispatches a key event to the first page that is BOTH visible
// and HasFocus — never to a hidden page that merely retains stale focus. This
// is the fix for the Guide focus-stealing root cause.
func (b *bodyPages) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		for _, r := range b.refs {
			if r.visible && r.item.HasFocus() {
				if handler := r.item.InputHandler(); handler != nil {
					handler(event, setFocus)
					return
				}
			}
		}
	})
}
