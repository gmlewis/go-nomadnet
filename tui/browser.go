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
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
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
	// contentFG is the browser content area's default text color: the
	// cube-quantized body_text palette entry (Python wraps the display widget
	// in AttrMap(..., "body_text"), Browser.py:562; body_text is 3-hex #ddd dark
	// / #222 light, ui/TextUI.py:26,80, cube-quantized to #d7d7d7 / #000000).
	// It colors blank/padding cells and the non-micron placeholder/loading text;
	// micron plain runs carry an explicit #dddddd tag (micron.DefaultFG, like
	// Python's high_color nibble-doubling) so they do not inherit it. It is
	// restored as the TextView default when a page declares no #!fg= (Python
	// resets page_foreground_color=None each load).
	contentFG tcell.Color
	history   []string
	histIdx   int
	// pendingLinkHist marks that a nomadnetwork.node link click (HandleLink)
	// eagerly pushed its target onto history and the fetch is still in flight.
	// Python's retrieve_url appends to history only on SUCCESS (Browser.py:131-145,
	// 216-268); the Go single-callback fetch model cannot push on success from
	// the tui layer (the success callback lives in the app layer and calls
	// RenderPage with markup, not the URL), so HandleLink pushes eagerly and the
	// failure paths (NotifyLinkError for a malformed/dispatch-error link, SetContent
	// for a fetch-fatal timeout/no-path) roll it back by popping. RenderPage clears
	// the flag on success (keeping the entry), and any new fetch (displayURL for a
	// typed URL / Back / Forward, or another HandleLink) pops a still-pending entry
	// first so a superseded click does not leave a stale history row. This mirrors
	// Python's "raise before touching history" for a failed link so the user is not
	// stranded on an error page with Ctrl-d a no-op.
	pendingLinkHist bool
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
	// ":<path>" URLs (Python Browser.destination_hash). It is read from the
	// partial-refresh goroutine (via CurrentDest) while the event loop writes
	// it (SetCurrentDest / Disconnect), so destMu guards the []byte slice
	// header against a torn concurrent read that would otherwise surface as a
	// "slice bounds out of range" panic in FetchPartial/identifyOnConnect.
	currentDest []byte
	destMu      sync.Mutex
	// retainedLink is the per-destination RNS link kept open across fetches,
	// mirroring Python Browser.self_link (Browser.py:1375-1451): Python
	// establishes a link once and reuses it for every page/partial request to
	// the same destination, so a form-submit re-fetch (search page → search
	// results) rides the already-ACTIVE link instead of re-running the
	// DH handshake + identification. The Go port used a one-shot link per
	// fetch (torn down in FetchPage), which made the second fetch re-establish
	// — flaky over a remote 2-hop path (link-establishment timeout, stalled
	// response resource). Retaining the link closes that reliability gap.
	//
	// It is opaque (any) so this package does not import go-reticulum; the
	// wiring layer (cmd/gonomadnet) owns the *rns.Link lifecycle — it passes
	// the link into FetchPageReuseLink and registers teardownLink to call
	// link.Teardown() when the destination changes or the browser disconnects.
	// destMu guards the field against the fetch goroutine reading it while the
	// event loop writes (SetCurrentDest/Disconnect), matching currentDest.
	retainedLink any
	teardownLink func(any) // tears down a retained *rns.Link (link.Teardown)
	// Rendered-page metadata, mirroring GuideDisplay: links/anchors are cached
	// for handleLink dispatch and jumpToAnchor; currentLines feeds the anchor
	// line lookup.
	links        []micron.LinkSpec
	anchors      micron.AnchorMap
	currentLines []*micron.StyledLine
	// lineTexts is the per-line tview-tagged text of the rendered page, used by
	// JumpToAnchor to measure each line's wrapped height (mirrors GuideDisplay).
	lineTexts []string
	// lineFields is the per-line table of rendered micron <field> spans, built
	// alongside currentLines in renderPage. Each entry records the field's spec,
	// its interactive widget (a ReadlineEdit for text fields, a *tview.Checkbox
	// for checkbox/radio), the display-column where the field begins on its line,
	// and the field width. The browser body is a single tview.TextView that
	// cannot host child primitives, so text fields are rendered as an in-place
	// ReadlineEdit overlay mounted over the field's screen cells when its row is
	// focused (syncFieldFocus/drawFieldOverlay); checkbox/radio fields toggle in
	// place via Space/Enter. On a form-submit link, collectFields walks this
	// table to build request_data (Python recurse_down, Browser.py:232-268).
	lineFields [][]*renderedField
	// fieldOverlay is the currently mounted text-field ReadlineEdit overlay, or
	// nil when no text field is in edit mode. fieldOverlayLine is the line it is
	// mounted for (so a no-op re-focus on the same field does not rebuild it).
	fieldOverlay     *ReadlineEdit
	fieldOverlayLine int
	// radioGroups tracks per-page radio-button groups by field name (mirroring
	// Python's per-parse radio grouping, MicronParser.py:385-394) so same-name
	// radios share a RadioGroup across one rendered page. Rebuilt each render.
	radioGroups map[string]*RadioGroup

	// Partials: the original markup is kept (with directives) and each partial's
	// latest fetched content is stored in partialContents keyed by the directive
	// (browser.Partial.Raw). renderPage substitutes them in before rendering, so
	// periodic refresh updates the content without losing the directive (Python
	// replaces the partial's urwid Pile slot instead). partialCancel stops the
	// refresh goroutines on navigation.
	currentMarkup   string
	partialContents map[string]string
	partialCancel   chan struct{}

	// In-flight page-fetch tracking, fixing the "stale Connect overtakes the
	// current page" race: every new navigation (Connect / URL load / link click)
	// calls beginRequest, which cancels the previous fetch's ctx and bumps
	// reqSeq. The fetch goroutine captures ctx+seq BY VALUE and, inside its
	// QueueUpdateDraw callback, drops the render when seq no longer equals
	// CurrentRequestSeq() or ctx was cancelled. Disconnect calls CancelRequest
	// to drop late renders of a page nobody is visiting anymore.
	//
	// All fields are event-loop-confined (no mutex): every writer
	// (displayURL/HandleLink → OnRetrieveURL → beginRequest, and Disconnect →
	// CancelRequest) runs on the tview event loop, the fetch goroutine captures
	// ctx+seq by value before spawn, and the render gate reads reqSeq from inside
	// QueueUpdateDraw (also the event loop). Same confinement model as
	// currentDest/history/partialCancel above.
	reqCtx    context.Context
	reqCancel context.CancelFunc
	reqSeq    uint64
	// renderedWidth is the column count the current page text was laid out for
	// (the width passed to StyledLinesToTviewText in renderPage). Horizontal
	// dividers are pre-rendered at this width; if the content is later drawn at a
	// different width (e.g. the fetch callback fired before the first layout),
	// browserPageView.Draw schedules a re-render so dividers reflow to the real
	// width — mirroring Python's urwid.Divider box widget which fills width at
	// draw time.
	renderedWidth int
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
	OnRetrieveURL      func(url string, requestData map[string]string)
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
	if f, err := os.OpenFile("/tmp/peek-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		_, _ = f.WriteString("=== NewBrowserDisplay created ===\n")
		_ = f.Close()
	}
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
	bd.contentFG = GetThemeColors(app.Theme)["body_text"]
	bd.content.SetTextColor(bd.contentFG)
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
	debugInputLog(bd, event)
	// A mounted text-field overlay (ReadlineEdit) is drawn over the page body by
	// drawFieldOverlay but is NOT a child of bd.layout, so tview's dispatch
	// cascades to bd.content (the tree leaf) and would never reach the editor —
	// printable runes would be dropped by the TextView and the field would never
	// accept text. Route the key through the overlay's handleKey instead:
	// printable runes + readline keys edit; Down/Up/Tab trigger onExit
	// (moveFieldFocus unmounts + advances focusLine); Enter/Esc stay editing.
	// When the overlay consumes the key, trigger a redraw (the editor is off-tree
	// so its SetText does not auto-invalidate the screen) and return nil so
	// bd.content never sees the key. A non-consumed key falls through to the
	// browser shortcuts / nav model below.
	if bd.fieldOverlay != nil {
		if bd.fieldOverlay.handleKey(event) == nil {
			if bd.app != nil {
				bd.app.QueueUpdateDraw(func() {})
			}
			return nil
		}
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
	bd.pushHistory(url)
	bd.displayURL(url)
}

// pushHistory appends url to the navigation history, truncating any forward
// entries beyond the current position (mirroring Python Browser.history append
// on a successful retrieve_url, Browser.py:131-145). bd.histIdx advances to the
// new (now last) entry. A still-pending link-click history push is rolled back
// first so a superseded click does not leave a stale row below the new entry.
func (bd *BrowserDisplay) pushHistory(url string) {
	bd.rollbackPendingLink()
	if bd.histIdx < len(bd.history)-1 {
		bd.history = bd.history[:bd.histIdx+1]
	}
	bd.history = append(bd.history, url)
	bd.histIdx = len(bd.history) - 1
}

// popHistory drops the last history entry, restoring histIdx to the previous
// page. Used to roll back a link click's eager push when its fetch fails so the
// user is left on the prior page (Python's retrieve_url raises before touching
// history). It never empties history below the entry histIdx points at when
// called from a failure callback (the failed link sat above it).
func (bd *BrowserDisplay) popHistory() {
	if len(bd.history) == 0 {
		bd.histIdx = 0
		return
	}
	bd.history = bd.history[:len(bd.history)-1]
	bd.histIdx = max(len(bd.history)-1, 0)
}

// rollbackPendingLink pops a link-click history push whose fetch never reached
// a success/failure callback (it was superseded by this new navigation) and
// clears the pending flag. No-op when no link click is in flight.
func (bd *BrowserDisplay) rollbackPendingLink() {
	if bd.pendingLinkHist {
		bd.popHistory()
		bd.pendingLinkHist = false
	}
}

// GoBack navigates to the previous URL in history. It is a no-op while a
// link-click fetch is still in flight (pendingLinkHist), matching Python's
// Browser.back() guard: `if not self.history_inc and not self.history_dec`
// (Browser.py:1079). Without this, GoBack would roll back the pending push
// and start a new fetch, but the original link fetch would overwrite the
// back-navigated page when it completed.
func (bd *BrowserDisplay) GoBack() {
	if bd.pendingLinkHist {
		return
	}
	if bd.histIdx > 0 {
		bd.histIdx--
		bd.displayURL(bd.history[bd.histIdx])
	}
}

// GoForward navigates to the next URL in history. It is a no-op while a
// link-click fetch is still in flight, matching the same guard as GoBack.
func (bd *BrowserDisplay) GoForward() {
	if bd.pendingLinkHist {
		return
	}
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
// the UI stays responsive. A still-pending link-click push is rolled back first
// (a typed URL / Back / Forward supersedes it).
func (bd *BrowserDisplay) displayURL(url string) {
	bd.rollbackPendingLink()
	bd.setURLHeader(url)
	bd.showLoading(bd.currentURLDisp)
	if bd.OnRetrieveURL != nil {
		// displayURL is the typed-URL/dialog path: entered URLs carry no
		// collected field values, so request data is nil. A backtick var_*
		// field-suffix embedded in the URL is still parsed by ParseURL in the
		// app-layer wiring (textui.go).
		bd.OnRetrieveURL(url, nil)
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
	bd.refreshURLHeader()
}

// refreshURLHeader re-applies the "Ⓝ <url>" header text, truncated to the
// current pane width. The URL bar is first set in displayURL at request time,
// before the browser pane may have been laid out at its final width, so the
// initial truncation can use a stale narrow width. Re-truncating once the page
// has rendered mirrors Python make_control_widget (Browser.py:508-515), which
// builds the control widget at page-build time with the settled content_cols,
// truncating lstr = g["node"]+" "+current_url() to lmax = content_cols-1 with
// s[:lmax-1]+"…" (clipboard copy is off in the default config).
func (bd *BrowserDisplay) refreshURLHeader() {
	disp := bd.currentURLDisp
	g := bd.app.Glyphs
	lstr := glyph(g, "node") + " " + disp
	lmax := bd.contentWidth() - 1
	bd.urlHeader.SetText(truncateEllipsis(lstr, lmax))
}

// canonicalURL returns the "<hex>:<path>" form of url when it parses as a
// nomadnet RNS address (resolving relative ":<path>" URLs against the current
// destination), falling back to the raw url on a parse error. Mirrors Python
// Browser.current_url (Browser.py:146-163) for the displayable form.
func (bd *BrowserDisplay) canonicalURL(url string) string {
	dest, path, rd, err := browser.ParseURL(url, bd.CurrentDest(), nil)
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
	// The browser's key shortcuts (Ctrl-d/w/u/etc. in handleInput) run as an
	// input capture on bd.widget, which bodyPages dispatches to only while the
	// browser page's widget tree HasFocus (body-pages.go:86). Removing the
	// focused bd.content orphans focus — no child of the page keeps it — so
	// the page stops receiving keys and the keyboard goes dead for the whole
	// fetch ("Retrieving" dead-keyboard bug: nomadnet stays responsive because
	// BrowserFrame.keypress always runs for the browser region). Move focus
	// onto the loading body when the content held it so the page keeps a
	// focused child and bodyPages keeps dispatching to handleInput. When the
	// left list (Network pane) held focus, bd.content.HasFocus() is false and
	// we must NOT steal focus — the list stays driven during the fetch.
	contentHadFocus := bd.content.HasFocus()
	bd.body.RemoveItem(bd.content)
	bd.loading = newMiddleCentered(bd.contentFG, "Retrieving", "["+url+"]")
	bd.body.AddItem(bd.loading, 0, 1, true)
	if contentHadFocus {
		bd.app.SetFocus(bd.loading)
	}
}

// showContent swaps the page content back into the body slot, removing the
// centered loading body. Called from renderPage once a page has rendered.
func (bd *BrowserDisplay) showContent() {
	// Mirror showLoading: if the loading body held focus (because bd.content
	// was focused when the load started), move focus back onto the restored
	// content so the loaded page is immediately navigable — otherwise the app
	// focus points at the removed loading body and keys go nowhere.
	loadingHadFocus := bd.loading != nil && bd.loading.HasFocus()
	if bd.loading != nil {
		bd.body.RemoveItem(bd.loading)
		bd.loading = nil
	}
	bd.body.RemoveItem(bd.content)
	bd.body.AddItem(bd.content, 0, 1, true)
	if loadingHadFocus {
		bd.app.SetFocus(bd.content)
	}
}

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
	// A successful page render keeps a link click's eager history push (the
	// fetch succeeded), clearing the rollback flag. No-op for a typed-URL load
	// (LoadURL → displayURL already cleared it).
	bd.pendingLinkHist = false
	markup := bd.effectiveMarkup()
	lines := micron.RenderToStyledLines(markup, micronTheme(bd.app.Theme))
	width := bd.contentWidth()
	text, links := StyledLinesToTviewText(lines, width)
	bd.renderedWidth = width
	bd.currentLines = lines
	bd.links = links
	bd.anchors = micron.BuildAnchorMap(lines)
	bd.lineTexts = splitLineTexts(text)
	// Build the per-line interactive field widgets (text → ReadlineEdit overlay,
	// checkbox/radio → Checkbox) from the rendered field spans, so the overlay
	// can mount on focus and collectFields can gather live values on submit.
	bd.buildLineFields(lines)

	// Apply page #!bg=/#!fg= as the TextView default colors so unstyled spans
	// (which emit plain text inheriting the TextView default) pick them up,
	// matching Python's fg_color/bg_color default. Reset to defaults when a
	// page declares no colors (Python resets both to None each load).
	bg, fg := ParseMicronColors(markup)
	if fg != "" {
		bd.content.SetTextColor(parseColor("#" + fg))
	} else {
		bd.content.SetTextColor(bd.contentFG)
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
	// Re-truncate the URL bar against the now-settled pane width. setURLHeader
	// ran at request time when the pane may still have been narrow, so the
	// initial ellipsization can be too aggressive; by page-build time the layout
	// has settled and contentWidth() is correct (Python builds the control
	// widget here too).
	bd.refreshURLHeader()

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
	// Bail out before the network fetch when the loop has been cancelled by
	// navigation/Disconnect/teardown — the QueueUpdateDraw callback below also
	// checks cancel, but without this guard a cancelled loop still launches a
	// fetch (and dereferences bd.OnFetchPartial / the captured transport) after
	// the page has moved on, racing teardown.
	select {
	case <-cancel:
		return
	default:
	}
	if bd.OnFetchPartial == nil {
		return
	}
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

// StopPartials is the exported teardown entry point for the partial-refresh
// goroutines. It must be called on shutdown BEFORE the underlying RNS transport
// is closed so the per-page refresh loops (runPartialRefresh, which call
// bd.OnFetchPartial → browser.FetchPartial → fetchBytes on the app's transport)
// stop before that transport is torn down; otherwise a tick fired after
// a.Shutdown() dereferences a closed TransportSystem.
func (bd *BrowserDisplay) StopPartials() { bd.stopPartials() }

// contentWidth is the content TextView's inner column count (the wrap/divider
// width). When the rect hasn't been laid out yet (width is 0 or the tview
// default 15×10), it falls back to the last rendered width (renderedWidth) so
// re-renders triggered before the first SetRect use a sensible width rather
// than the 15-column default. If renderedWidth is also 0 (very first render),
// 80 is used as a conventional terminal width.
func (bd *BrowserDisplay) contentWidth() int {
	_, _, w, _ := bd.content.GetInnerRect()
	if w > 15 {
		return w
	}
	if bd.renderedWidth > 0 {
		return bd.renderedWidth
	}
	return 80
}

// Reload refreshes the current page.
func (bd *BrowserDisplay) Reload() {
	if bd.histIdx >= 0 && bd.histIdx < len(bd.history) {
		bd.displayURL(bd.history[bd.histIdx])
	}
}

// Disconnect tears down the browser session, mirroring Python Browser.disconnect
// (Browser.py:862-881): the retained link is torn down (via SetCurrentDest(nil),
// which sees the destination change and invokes teardownRetainedLink), the
// history is cleared, the history pointer reset, the current-destination hint
// dropped, and the content set to the disconnected state.
func (bd *BrowserDisplay) Disconnect() {
	bd.CancelRequest()
	bd.history = nil
	bd.histIdx = 0
	bd.SetCurrentDest(nil)
	bd.stopPartials()
	bd.urlHeader.SetText(glyph(bd.app.Glyphs, "node"))
	bd.currentURLDisp = ""
	bd.footerStatus.SetText("")
	bd.showContent()
	bd.content.SetText("[gray]Disconnected.[-]")
	// "Disconnected." is not a rendered micron page: reset the per-line nav
	// model so an arrow key falls into handleNavKey's empty-page guard instead
	// of indexing an empty bd.lineCursors and panicking. See resetNavState.
	bd.resetNavState()
}

// CurrentURL returns the currently loaded URL.
func (bd *BrowserDisplay) CurrentURL() string {
	if bd.histIdx >= 0 && bd.histIdx < len(bd.history) {
		return bd.history[bd.histIdx]
	}
	return ""
}

// SetContent replaces the browser content area text. It is the app-layer
// failure callback for a fetch (OnBrowserError in cmd/gonomadnet/textui.go),
// reached when a fetch times out or returns ErrNoPath/ErrLinkTimeout/
// ErrRequestTimeout. Because a fetch in flight leaves the centered "Retrieving"
// loading body (bd.loading) in the layout in place of bd.content (showLoading,
// Browser.py:593-598), SetContent must swap that loading body back out via
// showContent — otherwise the error text lands on the hidden content widget and
// the browser stays stuck on "Retrieving" forever (the regression where a
// non-responding node appeared to never time out). showContent is a no-op when
// no fetch is in flight (bd.loading == nil).
func (bd *BrowserDisplay) SetContent(text string) {
	// A fetch-fatal failure (timeout / no path) or a local /file/ download
	// lands here. If the fetch was a nomadnetwork.node link click, HandleLink
	// pushed its target eagerly; roll that back so the user is not left on the
	// error/download page with Ctrl-d a no-op (Python's retrieve_url raises or
	// downloads without touching history). A typed-URL fetch-fatal has
	// pendingLinkHist already false (displayURL cleared it), so this is a no-op
	// for it — preserving the prior eager-push behavior for typed URLs.
	bd.rollbackPendingLink()
	bd.showContent()
	bd.content.SetText(text)
	// Error text is not a rendered micron page (no renderPage/initNavState ran):
	// reset the per-line nav model so an arrow key falls into handleNavKey's
	// empty-page guard instead of indexing an empty bd.lineCursors and
	// panicking ("index out of range [0] with length 0"). See resetNavState.
	bd.resetNavState()
}

// NotifyLinkError surfaces a link-dispatch failure in the browser FOOTER
// without replacing the current page, mirroring Python's Browser.handle_link
// and url_dialog catching retrieve_url's ValueError and setting
// self.browser_footer = "Could not open link: ..." (Browser.py:300-304,
// 1142-1150). Python's retrieve_url raises BEFORE touching status,
// destination_hash, or history, so the page the user was viewing stays put and
// Back (Ctrl-d) works as normal (it has nothing to undo for the failed link).
//
// HandleLink now pushes a nomadnetwork.node link's target eagerly (so a
// SUCCESSFUL click's Back works), so a FAILED link must roll that push back;
// rollbackPendingLink pops the just-pushed entry, leaving the user on the page
// they were viewing with the footer carrying the error. showContent swaps the
// "Retrieving" loading body back out if a URL-bar load had shown it (a no-op
// for a link click, which never showed loading). The rendered page's nav state
// is left intact so the user keeps navigating it. Fetch-FATAL errors (timeout
// / no path) still use SetContent (which also rolls back a pending link),
// matching Python's make_request_failed_widget replacing the body.
func (bd *BrowserDisplay) NotifyLinkError(msg string) {
	bd.rollbackPendingLink()
	bd.showContent()
	bd.linkStatusShowing = false
	bd.footerStatus.SetText("[red]" + tview.Escape(msg) + "[-]")
}

// CurrentDest returns a copy of the 16-byte destination hash of the currently
// loaded page (nil until the first successful navigation), used by the
// retrieveURL wiring to resolve relative ":<path>" URLs (Python
// Browser.destination_hash). A copy is returned because the partial-refresh
// goroutine reads this off the event loop while SetCurrentDest/Disconnect
// write concurrently; returning the live slice header would expose a torn read
// to FetchPartial/identifyOnConnect, which len/index the hash.
func (bd *BrowserDisplay) CurrentDest() []byte {
	bd.destMu.Lock()
	defer bd.destMu.Unlock()
	if bd.currentDest == nil {
		return nil
	}
	out := make([]byte, len(bd.currentDest))
	copy(out, bd.currentDest)
	return out
}

// SetCurrentDest records the destination hash of the page now being loaded, so
// subsequent relative URLs resolve against it. When the destination changes the
// retained link (if any) is torn down: a link is only valid for the destination
// it was established to, so reusing it against a new destination would fetch the
// wrong node. A same-destination SetCurrentDest (e.g. a form submit re-fetching
// the same node) keeps the link — mirroring Python self_link reuse.
func (bd *BrowserDisplay) SetCurrentDest(dest []byte) {
	bd.destMu.Lock()
	prev := bd.currentDest
	same := prev != nil && dest != nil && bytes.Equal(prev, dest) || prev == nil && dest == nil
	bd.currentDest = dest
	link := bd.retainedLink
	bd.destMu.Unlock()
	if !same && link != nil {
		bd.teardownRetainedLink(link)
	}
}

// RetainedLink returns the opaque retained RNS link (a *rns.Link the wiring
// layer cast in), or nil. The wiring layer checks it is still ACTIVE before
// reusing it for a fetch.
func (bd *BrowserDisplay) RetainedLink() any {
	bd.destMu.Lock()
	defer bd.destMu.Unlock()
	return bd.retainedLink
}

// SetRetainedLink stores the RNS link returned by a fetch so the next fetch to
// the same destination can reuse it. Pass nil to clear it (e.g. after a fetch
// error, so a retry re-establishes).
func (bd *BrowserDisplay) SetRetainedLink(link any) {
	bd.destMu.Lock()
	bd.retainedLink = link
	bd.destMu.Unlock()
}

// SetRetainedLinkTeardown registers the callback that tears down a retained
// *rns.Link (link.Teardown). It is set once by the wiring layer, which is the
// only side that knows the concrete go-reticulum type.
func (bd *BrowserDisplay) SetRetainedLinkTeardown(fn func(any)) {
	bd.destMu.Lock()
	bd.teardownLink = fn
	bd.destMu.Unlock()
}

// teardownRetainedLink clears the retained link and invokes the registered
// teardown callback outside destMu (the callback may take the link's own lock).
func (bd *BrowserDisplay) teardownRetainedLink(link any) {
	bd.destMu.Lock()
	if bd.retainedLink == link {
		bd.retainedLink = nil
	}
	fn := bd.teardownLink
	bd.destMu.Unlock()
	if fn != nil {
		fn(link)
	}
}

// BeginRequest cancels any in-flight page fetch, increments the request
// sequence, and installs a fresh cancellation context for the new fetch. It is
// the single entry point that invalidates a prior superseded fetch: a new
// Connect/URL-load/link-click must call this BEFORE spawning its fetch
// goroutine. Must be called on the tview event loop.
func (bd *BrowserDisplay) BeginRequest() (context.Context, uint64) {
	if bd.reqCancel != nil {
		bd.reqCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	bd.reqCtx = ctx
	bd.reqCancel = cancel
	bd.reqSeq++
	return ctx, bd.reqSeq
}

// CurrentRequest returns the in-flight request's context and sequence number.
// The fetch wiring reads this on the event loop before spawning the goroutine
// so it can capture ctx+seq by value.
func (bd *BrowserDisplay) CurrentRequest() (context.Context, uint64) {
	return bd.reqCtx, bd.reqSeq
}

// CurrentRequestSeq returns the current request sequence, for the stale-result
// check at the top of every QueueUpdateDraw render callback.
func (bd *BrowserDisplay) CurrentRequestSeq() uint64 { return bd.reqSeq }

// CancelRequest cancels any in-flight page fetch without starting a new one.
// Called from Disconnect. It bumps reqSeq so a render queued by a fetch that
// started before Disconnect is dropped by the seq check even though no new
// request has begun (belt-and-suspenders alongside the cancelled-ctx check).
func (bd *BrowserDisplay) CancelRequest() {
	if bd.reqCancel != nil {
		bd.reqCancel()
	}
	bd.reqCancel = nil
	bd.reqCtx = nil
	bd.reqSeq++
}

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
	// Python marked_link_job (Browser.py:196-200) builds lstr = "Link to " +
	// target and truncates it to lmax = content_cols with s[:lmax-1]+"…".
	// Replicate that so a long link target ends in "…" instead of being clipped
	// to spaces by the footer TextView.
	lstr := "Link to " + t
	lmax := bd.contentWidth()
	bd.footerStatus.SetText(truncateEllipsis(lstr, lmax))
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
	lines := strings.SplitSeq(markup, "\n")
	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(trimmed, ">>"); ok {
			name := strings.TrimSpace(after)
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
