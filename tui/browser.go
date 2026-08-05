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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// BrowserDisplay provides URL-based page browsing.
type BrowserDisplay struct {
	app    *App
	widget tview.Primitive
	layout *tview.Flex
	body   *tview.Flex // the header/footer-framed body slot (content or loading)
	// urlHeader is the non-editable "Ⓝ <url>" header row, mirroring Python
	// Browser.make_control_widget (Browser.py:505-524): g["node"]+" "+
	// current_url() as a urwid.Text (editing is via the C-u URL dialog, not an
	// inline field).
	urlHeader      *tview.TextView
	headerDivider  tview.Primitive
	footerDivider  tview.Primitive
	footerStatus   *tview.TextView
	currentURLDisp string
	// Transfer stats for the footer status text (Python response_size/
	// response_transfer_size/response_time/loaded_from_cache, Browser.py:1756).
	responseSize      int64
	responseTransfer  int64
	responseTime      float64
	hasTransfer       bool
	loadedFromCache   bool
	linkStatusShowing bool
	content           *browserPageView
	history           []string
	histIdx           int
	// loading is the MIDDLE-centered "Retrieving\n[<url>]" body shown while a
	// page fetch is in flight (Python Browser.update_display REQUEST_SENT branch,
	// Browser.py:593-598: Filler(Text("Retrieving\n["+url+"]", CENTER), MIDDLE)).
	// It is swapped into the layout in place of bd.content during displayURL and
	// swapped back out when renderPage paints the fetched page. nil ⇒ the page
	// content is currently shown.
	loading tview.Primitive

	// Per-line focus + part-cursor nav model, mirroring Python's Pile of
	// LinkableText (MicronParser.py:858-1048): focusLine is the focused
	// selectable line; lineCursors[idx] is the rune offset of the part cursor on
	// line idx. cursorLastKeypress/cursorHasKeypress drive the 2s hardware-
	// cursor visibility window (LinkableText.key_timeout=2); cursorHideTimer
	// queues the expiry redraw. See browser-nav.go.
	focusLine          int
	lineCursors        []int
	cursorLastKeypress time.Time
	cursorHasKeypress  bool
	cursorHideTimer    *time.Timer
	// OnReleaseFocus fires when Left is pressed at the start of the focused
	// line's first part — Python's delegate.micron_released_focus → focus_lists
	// (MicronParser.py:972-974). For the Network right pane it shifts focus to
	// the left node list; for the standalone browser page it returns focus to
	// the menu bar.
	OnReleaseFocus func()

	// currentDest is the 16-byte destination hash of the page currently loaded
	// (nil until the first successful navigation), used to resolve relative
	// ":<path>" URLs (Python Browser.destination_hash).
	currentDest []byte
	// Rendered-page metadata, mirroring GuideDisplay: links/anchors are cached
	// for handleLink dispatch and jumpToAnchor; currentLines feeds the anchor
	// line lookup.
	links        []micron.LinkSpec
	anchors      micron.AnchorMap
	currentLines []*micron.StyledLine
	// lineTexts is the per-line tview-tagged text of the rendered page, used by
	// JumpToAnchor to measure each line's wrapped height (mirrors GuideDisplay).
	lineTexts []string

	// Partials: the original markup is kept (with directives) and each partial's
	// latest fetched content is stored in partialContents keyed by the directive
	// (browser.Partial.Raw). renderPage substitutes them in before rendering, so
	// periodic refresh updates the content without losing the directive (Python
	// replaces the partial's urwid Pile slot instead). partialCancel stops the
	// refresh goroutines on navigation.
	currentMarkup   string
	partialContents map[string]string
	partialCancel   chan struct{}
	// renderedWidth is the column count the current page text was laid out for
	// (the width passed to StyledLinesToTviewText in renderPage). Horizontal
	// dividers are pre-rendered at this width; if the content is later drawn at a
	// different width (e.g. the fetch callback fired before the first layout),
	// browserPageView.Draw schedules a re-render so dividers reflow to the real
	// width — mirroring Python's urwid.Divider box widget which fills width at
	// draw time.
	renderedWidth   int
	// partials is the list of every partial declared in the current page markup
	// (including ones with no auto-refresh interval), tracked so a "p:<id>"
	// force-refresh link (Python handle_partial_updates) can select and re-fetch
	// them by id.
	partials []browser.Partial
	// OnFetchPartial fetches one partial's markup over RNS (wired in
	// cmd/gonomadnet to browser.FetchPartial with the app's TransportSystem). nil
	// ⇒ partials render as the ⧖ placeholder and never refresh.
	OnFetchPartial func(p browser.Partial) ([]byte, error)

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
//
// The layout mirrors Python Browser.build_display (Browser.py:473-487):
// LineBox(BrowserFrame(body, header, footer), title=<hash>) — a Frame whose
// header = make_control_widget() (Pile([Text("Ⓝ <url>"), Divider(┄)])), body =
// the page content, footer = make_status_widget() (Pile([Divider(┄),
// Text(status)])). The surrounding LineBox (border + "<hash>" title) is owned
// by the BrowserPane (Network right pane) or the standalone browser page — the
// BrowserDisplay itself renders NO border and NO title, so there is no nested
// "Browser" box. There is no top nav bar; controls live in the footer.
func NewBrowserDisplay(app *App) *BrowserDisplay {
	bd := &BrowserDisplay{app: app}
	g := app.Glyphs
	divGlyph := glyph(g, "divider1")
	if divGlyph == "" {
		divGlyph = "┄"
	}
	nodeGlyph := glyph(g, "node")
	controlsColor := browserControlsColor(app)

	// URL header: non-editable "Ⓝ <url>" (Python make_control_widget,
	// Browser.py:505-524). Editing is via the C-u URL dialog, not inline.
	bd.urlHeader = tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(controlsColor)
	bd.urlHeader.SetText(nodeGlyph)

	// Chrome dividers (urwid.Divider(self.g["divider1"]) — full width).
	bd.headerDivider = newDividerRow(divGlyph)
	bd.footerDivider = newDividerRow(divGlyph)

	// Footer status: transfer stats / "Link to <target>" (Python
	// make_status_widget + marked_link_job, Browser.py:497-501, 196-204).
	bd.footerStatus = tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(controlsColor)

	// Content area — a browserPageView wraps a TextView so the page body can
	// reposition the hardware cursor on each draw (see browser-nav.go). All
	// TextView behavior (scrolling, color tags, region tags, #!bg/#!fg) is
	// preserved; the nav model drives scrolling + link dispatch instead of the
	// TextView's own InputHandler.
	bd.content = newBrowserPageView(bd)
	bd.content.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	bd.content.SetText("[gray]Enter a URL and press Enter to load a page[-]")

	// The body slot holds the page content (or the centered "Retrieving" body
	// while a fetch is in flight). Swapping its single child keeps the header
	// and footer rows in place (Python keeps the control widget + status widget
	// during loading).
	bd.body = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(bd.content, 0, 1, true)

	// Layout: header (url + divider) · body · footer (divider + status). No
	// border, no title — the enclosing LineBox is the caller's responsibility.
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(bd.urlHeader, 1, 0, false).
		AddItem(bd.headerDivider, 1, 0, false).
		AddItem(bd.body, 0, 1, true).
		AddItem(bd.footerDivider, 1, 0, false).
		AddItem(bd.footerStatus, 1, 0, false)
	layout.SetInputCapture(bd.handleInput)

	bd.layout = layout
	bd.widget = layout
	return bd
}

// handleInput processes keyboard shortcuts for the browser display.
// Matches Python's BrowserFrame.keypress() (Browser.py:21): the C-w/d/f/r/u/
// s/b/y/g shortcuts route to the delegate callbacks, and when the page body
// (content) is focused every navigation key runs through the Python page-key
// model in handleNavKey (browser-nav.go) — arrows move per-line focus and the
// per-line part cursor, Home/End/PgUp/PgDn scroll with automove, Enter/Space
// follow the link at the cursor, Left-at-start releases focus. When the URL
// bar (or any non-content child) holds focus, navigation keys pass through
// unchanged so the input field keeps editing.
func (bd *BrowserDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if event == nil {
		return event
	}
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

	// Page body focused: run the full Python page-key model (browser-nav.go).
	if bd.content.HasFocus() && bd.handleNavKey(event) {
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

// displayURL shows a URL without modifying history. It sets the URL bar, shows
// a loading placeholder, and dispatches to OnRetrieveURL — the app-layer
// callback (wired in cmd/gonomadnet/textui.go) that runs the real fetch backend
// (nomadnet/browser ParseURL + FetchPage over the app's TransportSystem) and
// calls back into RenderPage on success or OnBrowserError on failure. This
// mirrors Python Browser.retrieve_url → __load, run on a background thread so
// the UI stays responsive.
func (bd *BrowserDisplay) displayURL(url string) {
	bd.setURLHeader(url)
	bd.showLoading(bd.currentURLDisp)
	if bd.OnRetrieveURL != nil {
		bd.OnRetrieveURL(url)
	}
}

// setURLHeader sets the non-editable "Ⓝ <url>" header text, mirroring Python
// make_control_widget (Browser.py:505-524): g["node"]+" "+current_url(). The
// displayed URL is the canonical "<hex>:<path>" form when the URL parses as a
// nomadnet RNS address (Python current_url, Browser.py:146-163); a non-RNS or
// synthetic URL is shown verbatim so unit tests and error states still surface
// what was requested. The raw value is also stored in currentURLDisp for
// accessors.
func (bd *BrowserDisplay) setURLHeader(url string) {
	disp := bd.canonicalURL(url)
	bd.currentURLDisp = disp
	g := bd.app.Glyphs
	bd.urlHeader.SetText(glyph(g, "node") + " " + disp)
}

// canonicalURL returns the "<hex>:<path>" form of url when it parses as a
// nomadnet RNS address (resolving relative ":<path>" URLs against the current
// destination), falling back to the raw url on a parse error. Mirrors Python
// Browser.current_url (Browser.py:146-163) for the displayable form.
func (bd *BrowserDisplay) canonicalURL(url string) string {
	dest, path, rd, err := browser.ParseURL(url, bd.currentDest, nil)
	if err != nil {
		return url
	}
	return browser.CurrentURL(dest, path, rd)
}

// URLDisplayText returns the URL currently shown in the header (without the
// node glyph), for test accessors and the copy-URL action.
func (bd *BrowserDisplay) URLDisplayText() string { return bd.currentURLDisp }

// showLoading swaps the MIDDLE-centered "Retrieving\n[<url>]" body into the
// layout in place of the page content, mirroring Python Browser.update_display
// while a request is in flight (status <= REQUEST_SENT, no attr_maps yet:
// Filler(Text("Retrieving\n["+current_url()+"]", CENTER), MIDDLE),
// Browser.py:593-598). The URL bar above stays visible (Python keeps the
// control-widget header during loading); the centered message replaces only
// the body. Restored to the page content by renderPage (showContent).
func (bd *BrowserDisplay) showLoading(url string) {
	if bd.loading != nil {
		bd.body.RemoveItem(bd.loading)
	}
	bd.body.RemoveItem(bd.content)
	bd.loading = newMiddleCentered(tcell.NewHexColor(defaultContentFG), "Retrieving", "["+url+"]")
	bd.body.AddItem(bd.loading, 0, 1, true)
}

// showContent swaps the page content back into the body slot, removing the
// centered loading body. Called from renderPage once a page has rendered.
func (bd *BrowserDisplay) showContent() {
	if bd.loading != nil {
		bd.body.RemoveItem(bd.loading)
		bd.loading = nil
	}
	bd.body.RemoveItem(bd.content)
	bd.body.AddItem(bd.content, 0, 1, true)
}

// defaultContentFG is the browser content area's default text color, matching
// the SetTextColor used in NewBrowserDisplay. It is restored when a page
// declares no #!fg= (Python load_page resets page_foreground_color=None each
// load).
const defaultContentFG = 0xbbbbbb

// RenderPage renders Micron markup into the browser content area, mirroring
// Python Browser.load_page (Browser.py:1236-1282): micron.RenderToStyledLines
// → StyledLinesToTviewText → content.SetText, with the page's #!bg=/#!fg=
// directives applied as the default colors for unstyled spans (Python passes
// them as fg_color/bg_color to markup_to_attrmaps). Links and anchors are
// cached for handleLink dispatch and jumpToAnchor. The fetch that produced
// markup is the caller's responsibility (OnRetrieveURL wiring).
//
// After the initial render it starts a refresh loop for any partial directives
// in the markup (Python detect_partials + start_partial_updater); the partial
// fetch runs via OnFetchPartial. On the next navigation the previous loop is
// stopped.
func (bd *BrowserDisplay) RenderPage(markup string) {
	bd.currentMarkup = markup
	bd.partialContents = nil
	bd.stopPartials()
	bd.startPartials(markup)
	bd.renderPage()
}

// renderPage renders the current markup (with any fetched partial content
// substituted in) into the content area. It does NOT touch the partial loop,
// so it is safe to call from a partial-fetch callback to refresh the page
// without restarting the refresh timers.
func (bd *BrowserDisplay) renderPage() {
	markup := bd.effectiveMarkup()
	lines := micron.RenderToStyledLines(markup, micronTheme(bd.app.Theme))
	width := bd.contentWidth()
	text, links := StyledLinesToTviewText(lines, width)
	bd.renderedWidth = width
	bd.currentLines = lines
	bd.links = links
	bd.anchors = micron.BuildAnchorMap(lines)
	bd.lineTexts = splitLineTexts(text)

	// Apply page #!bg=/#!fg= as the TextView default colors so unstyled spans
	// (which emit plain text inheriting the TextView default) pick them up,
	// matching Python's fg_color/bg_color default. Reset to defaults when a
	// page declares no colors (Python resets both to None each load).
	bg, fg := ParseMicronColors(markup)
	if fg != "" {
		bd.content.SetTextColor(parseColor("#" + fg))
	} else {
		bd.content.SetTextColor(tcell.NewHexColor(defaultContentFG))
	}
	if bg != "" {
		bd.content.SetBackgroundColor(parseColor("#" + bg))
	} else {
		bd.content.SetBackgroundColor(tcell.ColorDefault)
	}

	bd.content.SetText(text)

	// Swap the page content back in if the centered loading body was showing
	// (displayURL swaps it out while a fetch is in flight).
	bd.showContent()

	// Reset the per-line focus + part-cursor nav model for the freshly rendered
	// page (browser-nav.go): focus defaults to the first selectable line, each
	// line's cursor starts at part 0, and the hardware-cursor visibility window
	// is cleared. Mirrors Python update_page_display building a fresh Pile of
	// LinkableText on every load (Browser.py:469-486).
	bd.initNavState()

	// A completed render ⇒ DONE: refresh the footer so the transfer-status line
	// (or "Link to" peek) reflects the current page (Python update_display DONE
	// branch sets browser_footer = make_status_widget(), Browser.py:529-531).
	bd.refreshFooterStatus()
}

// JumpToAnchor scrolls the browser content so the named anchor's line sits at
// the top, mirroring Python Browser._jump_to_anchor (Browser.py:324-357). With
// a non-empty name it looks up the anchor (declared `:name or a heading slug)
// and scrolls to rowsAbove(targetIdx) wrapped display rows. An unknown anchor
// is a no-op (Python sets a footer message "Unknown anchor: #name"; the Go port
// has no browser-footer slot, so the faithful mapping is a no-op, matching
// GuideDisplay). An empty name (a bare "#" link) jumps to the first heading
// below the current scroll position (Python's else-branch, Browser.py:337-348),
// or is a no-op if no heading lies below the cursor.
func (bd *BrowserDisplay) JumpToAnchor(name string) {
	if bd.anchors == nil || len(bd.currentLines) == 0 {
		return
	}
	targetIdx := -1
	if name != "" {
		idx, ok := bd.anchors.JumpTarget(name)
		if !ok {
			return
		}
		targetIdx = idx
	} else {
		current, _ := bd.content.GetScrollOffset()
		for i, sl := range bd.currentLines {
			if sl != nil && sl.HeadingLevel > 0 && bd.rowsAbove(i) > current {
				targetIdx = i
				break
			}
		}
		if targetIdx < 0 {
			return
		}
	}
	bd.content.ScrollTo(bd.rowsAbove(targetIdx), 0)
}

// rowsAbove returns the number of wrapped display rows preceding line idx,
// mirroring Python Browser._rows_above (Browser.py:373-381). Each preceding
// line's row count is len(tview.WordWrap(lineText, innerWidth)) — the same
// word-wrap tview's TextView uses to draw — so the computed offset tracks the
// real rendered layout.
func (bd *BrowserDisplay) rowsAbove(idx int) int {
	if idx <= 0 || len(bd.lineTexts) == 0 {
		return 0
	}
	_, _, innerW, _ := bd.content.GetInnerRect()
	if innerW <= 0 {
		innerW = bd.contentWidth()
	}
	total := 0
	for i := 0; i < idx && i < len(bd.lineTexts); i++ {
		rows := 1
		if innerW > 0 {
			if w := len(tview.WordWrap(bd.lineTexts[i], innerW)); w > 0 {
				rows = w
			}
		}
		total += rows
	}
	return total
}

// effectiveMarkup returns the current page markup with each fetched partial's
// content substituted for its directive (browser.Partial.Raw). Unfetched
// partials keep their directive (the micron renderer shows the ⧖ placeholder),
// matching Python's pre-fetch state.
func (bd *BrowserDisplay) effectiveMarkup() string {
	if len(bd.partialContents) == 0 {
		return bd.currentMarkup
	}
	out := bd.currentMarkup
	for raw, content := range bd.partialContents {
		out = strings.Replace(out, raw, content, 1)
	}
	return out
}

// startPartials starts a refresh goroutine for each partial in markup that
// declares a refresh interval (Python start_partial_updater). Each goroutine
// fetches its partial immediately and then on every Refresh-second tick via
// OnFetchPartial, marshaling the substituted re-render to the UI loop. No-op
// when OnFetchPartial is nil (partials stay as ⧖ placeholders).
func (bd *BrowserDisplay) startPartials(markup string) {
	bd.partials = browser.ExtractPartials(markup)
	if bd.OnFetchPartial == nil || len(bd.partials) == 0 {
		return
	}
	cancel := make(chan struct{})
	bd.partialCancel = cancel
	for _, p := range bd.partials {
		if p.Refresh <= 0 {
			continue
		}
		p := p
		interval := time.Duration(p.Refresh * float64(time.Second))
		if interval <= 0 {
			interval = time.Second
		}
		go bd.runPartialRefresh(p, interval, cancel)
	}
}

// partialsToRefresh returns the page partials whose id is among ids, mirroring
// Python handle_partial_updates (Browser.py:823-834): it iterates the page's
// partials and selects those whose id is in partial_ids. A partial with no
// pid (empty id) matches only when "" is among the requested ids.
func (bd *BrowserDisplay) partialsToRefresh(ids []string) []browser.Partial {
	idset := make(map[string]bool, len(ids))
	for _, id := range ids {
		idset[id] = true
	}
	var out []browser.Partial
	for _, p := range bd.partials {
		if idset[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

// RefreshPartials force-fetches the page partials whose id is among ids,
// mirroring Python handle_partial_updates (Browser.py:823-834) (a "p:<id>:<id>"
// link). The fetch runs on a goroutine (Python spawns a thread) so the UI does
// not block on the network; each result is substituted into the page on the UI
// loop via fetchAndSubstitute. No-op when OnFetchPartial is nil or no partials
// match.
func (bd *BrowserDisplay) RefreshPartials(ids []string) {
	if bd.OnFetchPartial == nil {
		return
	}
	matches := bd.partialsToRefresh(ids)
	if len(matches) == 0 {
		return
	}
	cancel := bd.partialCancel
	go func() {
		for _, p := range matches {
			p := p
			bd.fetchAndSubstitute(p, cancel)
		}
	}()
}

// runPartialRefresh fetches the partial once immediately and then on each tick,
// until cancel is closed. Each fetch's result is applied on the UI loop.
func (bd *BrowserDisplay) runPartialRefresh(p browser.Partial, interval time.Duration, cancel chan struct{}) {
	bd.fetchAndSubstitute(p, cancel)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-cancel:
			return
		case <-ticker.C:
			bd.fetchAndSubstitute(p, cancel)
		}
	}
}

// fetchAndSubstitute fetches one partial and, on the UI loop, records its
// content (or an error message) in partialContents and re-renders. Stale
// results from a cancelled loop are dropped.
func (bd *BrowserDisplay) fetchAndSubstitute(p browser.Partial, cancel chan struct{}) {
	data, err := bd.OnFetchPartial(p)
	bd.app.QueueUpdateDraw(func() {
		select {
		case <-cancel:
			return
		default:
		}
		if bd.partialContents == nil {
			bd.partialContents = map[string]string{}
		}
		if err != nil {
			bd.partialContents[p.Raw] = fmt.Sprintf("[red]Could not load partial %v: %v[-]", p.URL, err)
		} else {
			bd.partialContents[p.Raw] = strings.TrimRight(string(data), "\n")
		}
		bd.renderPage()
	})
}

// stopPartials closes any active partial-refresh loop, stopping its goroutines.
func (bd *BrowserDisplay) stopPartials() {
	if bd.partialCancel != nil {
		select {
		case <-bd.partialCancel:
			// already closed
		default:
			close(bd.partialCancel)
		}
		bd.partialCancel = nil
	}
}

// contentWidth is the content TextView's inner column count (the wrap/divider
// width), falling back to 80 before the first layout — mirroring
// GuideDisplay.readerWidth.
func (bd *BrowserDisplay) contentWidth() int {
	_, _, w, _ := bd.content.GetInnerRect()
	if w <= 0 {
		return 80
	}
	return w
}

// Reload refreshes the current page.
func (bd *BrowserDisplay) Reload() {
	if bd.histIdx >= 0 && bd.histIdx < len(bd.history) {
		bd.displayURL(bd.history[bd.histIdx])
	}
}

// Disconnect tears down the browser session, mirroring Python Browser.disconnect
// (Browser.py:862-881): the link is torn down (the Go port has no persistent
// link — each fetch is one-shot — so there is nothing to teardown here), the
// history is cleared, the history pointer reset, the current-destination hint
// dropped, and the content set to the disconnected state. request_data is
// cleared implicitly since the Go port does not retain it between fetches.
func (bd *BrowserDisplay) Disconnect() {
	bd.history = nil
	bd.histIdx = 0
	bd.currentDest = nil
	bd.stopPartials()
	bd.urlHeader.SetText(glyph(bd.app.Glyphs, "node"))
	bd.currentURLDisp = ""
	bd.footerStatus.SetText("")
	bd.content.SetText("[gray]Disconnected.[-]")
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

// CurrentDest returns the 16-byte destination hash of the currently loaded page
// (nil until the first successful navigation), used by the retrieveURL wiring
// to resolve relative ":<path>" URLs (Python Browser.destination_hash).
func (bd *BrowserDisplay) CurrentDest() []byte { return bd.currentDest }

// SetCurrentDest records the destination hash of the page now being loaded, so
// subsequent relative URLs resolve against it.
func (bd *BrowserDisplay) SetCurrentDest(dest []byte) { bd.currentDest = dest }

// MarkedLink shows "Link to <target>" in the browser footer (the in-pane
// BrowserFrame footer, NOT the global shortcut bar), mirroring Python
// Browser.marked_link_job (Browser.py:181-204): when the cursor rests on a link
// the footer swaps to "Link to <target>`<fields>"; clearing it (target=="")
// restores the transfer-status widget. This is the link-peek that enables
// in-page link-following.
func (bd *BrowserDisplay) MarkedLink(target, fields string) {
	if target == "" {
		bd.linkStatusShowing = false
		bd.refreshFooterStatus()
		return
	}
	var f []string
	if fields != "" {
		f = strings.Split(fields, "|")
	}
	t := browser.MarkedLinkTarget(target, f)
	bd.linkStatusShowing = true
	bd.footerStatus.SetText("Link to " + t)
}

// SetTransferStats records the response size, transfer size, and elapsed time
// for the most recent fetch, then refreshes the footer status — mirroring
// Python Browser.status_text (Browser.py:1756-1803) which renders
// "Done  ▤ <size>   ↓<size> in <t>s   ◷ <speed>b/s". cached marks a cache hit
// (" (cached)"). Call with hasTransfer=false before the fetch completes to show
// the in-flight status label instead.
func (bd *BrowserDisplay) SetTransferStats(responseSize, transferSize int64, responseTime float64, cached bool) {
	bd.responseSize = responseSize
	bd.responseTransfer = transferSize
	bd.responseTime = responseTime
	bd.hasTransfer = transferSize > 0
	bd.loadedFromCache = cached
	bd.refreshFooterStatus()
}

// SetBrowserStatus sets the lifecycle status driving the footer status text
// (Python Browser.status, Browser.py:80-101) and refreshes the footer. Use
// before/while a fetch is in flight to show "Sending request...", "Request
// sent, awaiting response...", etc.
func (bd *BrowserDisplay) SetBrowserStatus(status int) {
	bd.refreshFooterStatusFor(browserStatus(status))
}

// refreshFooterStatus rebuilds the footer status text for the current stats and
// the DONE state (the common state once a page has rendered), unless a link
// peek is currently showing (Python link_status_showing suppresses the status
// widget while "Link to ..." is displayed).
func (bd *BrowserDisplay) refreshFooterStatus() {
	if bd.linkStatusShowing {
		return
	}
	bd.refreshFooterStatusFor(browserDone)
}

// refreshFooterStatusFor renders the footer text for a given lifecycle status.
func (bd *BrowserDisplay) refreshFooterStatusFor(status browserStatus) {
	g := bd.app.Glyphs
	text := browserStatusText(g, status, bd.responseSize, bd.responseTransfer,
		bd.responseTime, bd.hasTransfer, bd.loadedFromCache, "")
	bd.footerStatus.SetText(text)
}

// MicronReleasedFocus handles focus release from the micron text area, mirroring
// Python delegate.micron_released_focus → focus_lists (MicronParser.py:972-974):
// it clears the link peek and hands focus back to the owning view via
// OnReleaseFocus (the Network left list, or the menu bar for the standalone
// browser page).
func (bd *BrowserDisplay) MicronReleasedFocus() {
	bd.MarkedLink("", "")
	if bd.OnReleaseFocus != nil {
		bd.OnReleaseFocus()
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

	return "", "", fmt.Errorf("unrecognized link target: %v", target)
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

// ParseMicronColors extracts the #!bg= and #!fg= page-color directives from
// page markup, matching Python Browser.load_page (Browser.py:1247-1267 +
// 1326-1346). It finds the FIRST occurrence of each directive, takes the slice
// from after "= to the NEXT newline, and accepts it only when that slice is
// exactly 3 or 6 characters. A directive with no trailing newline (end of
// markup) yields no color — Python's str.find returns -1, making the length
// check negative, so the Go port must NOT fall back to end-of-markup. Returns
// (bg, fg) hex color strings (without leading #), or empty strings if not
// found. The first directive in the markup wins (Python finds the first).
func ParseMicronColors(markup string) (bg, fg string) {
	bg = parseColorDirective(markup, "#!bg=")
	fg = parseColorDirective(markup, "#!fg=")
	return bg, fg
}

// parseColorDirective is the shared #!xx= extraction: find the first directive,
// slice up to the next newline, accept only if exactly 3 or 6 chars. Returns
// "" when absent, unterminated (no newline), or wrong length.
func parseColorDirective(markup, directive string) string {
	pos := strings.Index(markup, directive)
	if pos < 0 {
		return ""
	}
	relEnd := strings.Index(markup[pos+len(directive):], "\n")
	if relEnd < 0 {
		return "" // no terminating newline → Python yields no color
	}
	val := markup[pos+len(directive) : pos+len(directive)+relEnd]
	if len(val) == 3 || len(val) == 6 {
		return val
	}
	return ""
}
