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
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// BrowserDisplay provides URL-based page browsing.
type BrowserDisplay struct {
	app     *tview.Application
	widget  tview.Primitive
	layout  *tview.Flex
	urlBar  *ReadlineEdit
	content *tview.TextView
	history []string
	histIdx int

	// Keyboard shortcut callbacks (Python: BrowserFrame.keypress)
	OnDisconnect       func()
	OnBack             func()
	OnForward          func()
	OnReload           func()
	OnURLDialog        func()
	OnSaveNode         func()
	OnToggleFullscreen func()
	OnCopyURL          func()
	OnOpenLXMF         func(hash string)
	OnOpenRRC          func(hubHex, room string)
	OnBrowserError     func(msg string)
	OnJumpAnchor       func(name string)
	OnRetrieveURL      func(url string)
	OnPartialUpdate    func(ids []string)
}

// NewBrowserDisplay creates a new browser display.
func NewBrowserDisplay(app *tview.Application) *BrowserDisplay {
	bd := &BrowserDisplay{app: app}

	// Title
	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[::b]Browser[-]")

	// URL bar
	bd.urlBar = NewReadlineEdit("URL: ", "Enter RNS address or lxmf:// URI")
	bd.urlBar.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	bd.urlBar.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Content area
	bd.content = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText("[gray]Enter a URL and press Enter to load a page[-]")

	// Navigation bar
	navBar := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0x999999)).
		SetText("[yellow]Enter[-] Load  [yellow]Ctrl-L[-] Back  [yellow]Ctrl-R[-] Forward  [yellow]Esc[-] URL bar")

	// Layout
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 1, 0, false).
		AddItem(bd.urlBar, 1, 0, false).
		AddItem(navBar, 1, 0, false).
		AddItem(bd.content, 0, 1, true)
	layout.SetBorder(true)
	layout.SetInputCapture(bd.handleInput)

	bd.layout = layout
	bd.widget = layout
	return bd
}

// handleInput processes keyboard shortcuts for the browser display.
// Matches Python's BrowserFrame.keypress() at Browser.py:21.
func (bd *BrowserDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlW:
		if bd.OnDisconnect != nil {
			bd.OnDisconnect()
		}
		return nil
	case tcell.KeyCtrlD:
		if bd.OnBack != nil {
			bd.OnBack()
		}
		return nil
	case tcell.KeyCtrlF:
		if bd.OnForward != nil {
			bd.OnForward()
		}
		return nil
	case tcell.KeyCtrlR:
		if bd.OnReload != nil {
			bd.OnReload()
		}
		return nil
	case tcell.KeyCtrlU:
		if bd.OnURLDialog != nil {
			bd.OnURLDialog()
		}
		return nil
	case tcell.KeyCtrlS:
		if bd.OnSaveNode != nil {
			bd.OnSaveNode()
		}
		return nil
	case tcell.KeyCtrlB:
		// Ctrl-B is an alias for Ctrl-S (save node) in Python
		if bd.OnSaveNode != nil {
			bd.OnSaveNode()
		}
		return nil
	case tcell.KeyCtrlG:
		if bd.OnToggleFullscreen != nil {
			bd.OnToggleFullscreen()
		}
		return nil
	case tcell.KeyCtrlY:
		if bd.OnCopyURL != nil {
			bd.OnCopyURL()
		}
		return nil
	}

	return event
}

// Widget returns the tview primitive for this display.
func (bd *BrowserDisplay) Widget() tview.Primitive {
	return bd.widget
}

// LoadURL loads a URL and displays the content.
func (bd *BrowserDisplay) LoadURL(url string) {
	if url == "" {
		return
	}

	// If we're not at the end of history, truncate forward
	if bd.histIdx < len(bd.history)-1 {
		bd.history = bd.history[:bd.histIdx+1]
	}
	bd.history = append(bd.history, url)
	bd.histIdx = len(bd.history) - 1

	bd.displayURL(url)
}

// GoBack navigates to the previous URL in history.
func (bd *BrowserDisplay) GoBack() {
	if bd.histIdx > 0 {
		bd.histIdx--
		bd.displayURL(bd.history[bd.histIdx])
	}
}

// GoForward navigates to the next URL in history.
func (bd *BrowserDisplay) GoForward() {
	if bd.histIdx < len(bd.history)-1 {
		bd.histIdx++
		bd.displayURL(bd.history[bd.histIdx])
	}
}

// displayURL shows a URL without modifying history.
func (bd *BrowserDisplay) displayURL(url string) {
	bd.urlBar.SetText(url)
	content := fmt.Sprintf("[::b]Loading: %s[-]\n\n", url)
	content += "[gray]In a full implementation, this would:[-]\n"
	content += "1. Parse the RNS address\n"
	content += "2. Establish a link\n"
	content += "3. Request the page\n"
	content += "4. Render the Micron content\n\n"
	content += fmt.Sprintf("[gray]URL: %s[-]", url)
	bd.content.SetText(content)
}

// Reload refreshes the current page.
func (bd *BrowserDisplay) Reload() {
	if bd.histIdx >= 0 && bd.histIdx < len(bd.history) {
		bd.displayURL(bd.history[bd.histIdx])
	}
}

// CurrentURL returns the currently loaded URL.
func (bd *BrowserDisplay) CurrentURL() string {
	if bd.histIdx >= 0 && bd.histIdx < len(bd.history) {
		return bd.history[bd.histIdx]
	}
	return ""
}

// SetContent replaces the browser content area text.
func (bd *BrowserDisplay) SetContent(text string) {
	bd.content.SetText(text)
}

// PageCache stores cached Micron pages by URL.
var PageCache = make(map[string][]byte)

// CachePage stores page data in the in-memory cache.
// Matches Python's Browser.cache_page at Browser.py:1615.
func CachePage(url string, data []byte) {
	PageCache[url] = data
}

// GetCached retrieves page data from cache, or nil if not cached.
// Matches Python's Browser.get_cached at Browser.py:1564.
func GetCached(url string) []byte {
	return PageCache[url]
}

// CleanCache removes pages older than maxAge from the cache.
// Matches Python's Browser.clean_cache at Browser.py:1598.
func CleanCache(maxEntries int) {
	if len(PageCache) <= maxEntries {
		return
	}
	// Remove oldest entries (simple LRU approximation)
	for k := range PageCache {
		delete(PageCache, k)
		if len(PageCache) <= maxEntries {
			break
		}
	}
}

// HandleLink dispatches a link target based on its type.
// Matches Python's Browser.handle_link at Browser.py:216.
// Returns (destType, hash, err).
func HandleLink(target string) (destType, hash string, err error) {
	if target == "" {
		return "", "", fmt.Errorf("empty link target")
	}

	// Anchor links (#name) — handled locally
	if strings.HasPrefix(target, "#") {
		return "anchor", target[1:], nil
	}

	// RRC hub links (rrc://...)
	if strings.HasPrefix(target, "rrc://") {
		return "rrc", target[6:], nil
	}

	// LXMF delivery links (lxmf@hash)
	if strings.HasPrefix(target, "lxmf@") {
		return "lxmf", target[5:], nil
	}

	// Page address (32-hex hash)
	if len(target) == 64 {
		allHex := true
		for _, c := range target {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				allHex = false
				break
			}
		}
		if allHex {
			return "page", target, nil
		}
	}

	return "", "", fmt.Errorf("unrecognized link target: %s", target)
}

// DetectPartials scans page markup for partial include directives.
// Returns a list of partial names to fetch.
// Matches Python's Browser.detect_partials at Browser.py:659.
func DetectPartials(markup string) []string {
	var partials []string
	lines := strings.Split(markup, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">>") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, ">>"))
			if name != "" {
				partials = append(partials, name)
			}
		}
	}
	return partials
}

// ParseMicronColors extracts#!bg= and #!fg= directives from page markup.
// Returns (bg, fg) hex color strings, or empty strings if not found.
func ParseMicronColors(markup string) (bg, fg string) {
	bgpos := strings.Index(markup, "#!bg=")
	if bgpos >= 0 {
		endpos := strings.Index(markup[bgpos:], "\n")
		if endpos < 0 {
			endpos = len(markup) - bgpos
		}
		bgVal := markup[bgpos+5 : bgpos+endpos]
		if len(bgVal) == 3 || len(bgVal) == 6 {
			bg = bgVal
		}
	}

	fgpos := strings.Index(markup, "#!fg=")
	if fgpos >= 0 {
		endpos := strings.Index(markup[fgpos:], "\n")
		if endpos < 0 {
			endpos = len(markup) - fgpos
		}
		fgVal := markup[fgpos+5 : fgpos+endpos]
		if len(fgVal) == 3 || len(fgVal) == 6 {
			fg = fgVal
		}
	}

	return bg, fg
}
