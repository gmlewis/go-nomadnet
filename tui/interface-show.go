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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// ParamCategories holds interface parameters grouped by category for
// the ShowInterface detail view. Matches Python's parameter sorting
// in ShowInterface.__init__() at Interfaces.py:2208.
type ParamCategories struct {
	Connection map[string]any
	Radio      map[string]any
	Network    map[string]any
	IFAC       map[string]any
	Other      map[string]any
}

// connectionKeys are interface config keys in the "Connection" category.
var connectionKeys = map[string]bool{
	"port": true, "listen_ip": true, "listen_port": true,
	"target_host": true, "target_port": true, "device": true,
}

// radioKeys are interface config keys in the "Radio" category.
var radioKeys = map[string]bool{
	"frequency": true, "bandwidth": true,
	"spreadingfactor": true, "codingrate": true, "txpower": true,
}

// networkKeys are interface config keys in the "Network" category.
var networkKeys = map[string]bool{
	"network_name": true, "bitrate": true, "peers": true,
	"group_id": true, "multicast_address_type": true,
	"discovery_scope": true, "announce_cap": true, "mode": true,
}

// ifacKeys are interface config keys in the "IFAC" category.
var ifacKeys = map[string]bool{
	"passphrase": true, "ifac_size": true, "ifac_netname": true, "ifac_netkey": true,
}

// skipKeys are config keys that should not be displayed in the detail view.
var skipKeys = map[string]bool{
	"type": true, "interface_enabled": true, "enabled": true,
	"selected_interface_mode": true, "name": true,
}

// CategorizeInterfaceParams groups interface config parameters into
// connection, radio, network, IFAC, and other categories. Empty and
// nil values are excluded. Keys in skipKeys are omitted.
// Matches Python's ShowInterface parameter sorting at Interfaces.py:2368.
func CategorizeInterfaceParams(config map[string]any) ParamCategories {
	cats := ParamCategories{
		Connection: make(map[string]any),
		Radio:      make(map[string]any),
		Network:    make(map[string]any),
		IFAC:       make(map[string]any),
		Other:      make(map[string]any),
	}

	for key, value := range config {
		if skipKeys[key] {
			continue
		}
		if value == nil {
			continue
		}
		if s, ok := value.(string); ok && s == "" {
			continue
		}

		switch {
		case connectionKeys[key]:
			cats.Connection[key] = value
		case radioKeys[key]:
			cats.Radio[key] = value
		case networkKeys[key]:
			cats.Network[key] = value
		case ifacKeys[key]:
			cats.IFAC[key] = value
		default:
			cats.Other[key] = value
		}
	}

	return cats
}

// SortedKeys returns the keys of a map sorted alphabetically.
func SortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FormatParamValue formats an interface parameter value for display.
// Special-cases: frequency (Hz → MHz), bandwidth (Hz → kHz),
// passphrase (masked), boolean (Yes/No).
// Matches Python's create_param_row() at Interfaces.py:2413.
func FormatParamValue(key string, value any) string {
	switch key {
	case "frequency":
		return formatRadioFrequency(value)
	case "bandwidth":
		return formatRadioBandwidth(value)
	case "passphrase":
		if s, ok := value.(string); ok {
			return strings.Repeat("*", len(s))
		}
		return "***"
	}

	switch v := value.(type) {
	case bool:
		if v {
			return "Yes"
		}
		return "No"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// FormatParamKey converts a snake_case config key to a display label.
// For example, "listen_port" becomes "Listen Port".
// Matches Python's key.replace('_', ' ').title() at Interfaces.py:2427.
// Python's .title() treats digit-letter boundaries as word breaks,
// so "i2p" becomes "I2P". We replicate this behavior.
func FormatParamKey(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = titleWord(p)
		}
	}
	return strings.Join(parts, " ")
}

// titleWord mimics Python's str.title() behavior for a single
// word. Python's title() capitalizes a character if the preceding
// character is not a letter or digit (i.e., after digits, the next
// letter is also uppercased). For example: "i2p" → "I2P".
func titleWord(word string) string {
	if len(word) == 0 {
		return word
	}
	var sb strings.Builder
	prevIsLetter := false
	for i, r := range word {
		if i == 0 || !prevIsLetter {
			sb.WriteRune(unicode.ToUpper(r))
		} else {
			sb.WriteRune(unicode.ToLower(r))
		}
		prevIsLetter = unicode.IsLetter(r)
	}
	return sb.String()
}

// paramNumber reads a parameter value as a number. The detail view's parameters
// come from the RNS config text (App.InterfaceStats keeps them as strings), so a
// numeric parameter arrives as "915000000" rather than as an int: Python reads
// the same value out of configobj, which has already parsed it into a number, so
// every numeric formatter here has to accept the string spelling too.
func paramNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// formatRadioFrequency converts a Hz value to MHz display string.
func formatRadioFrequency(value any) string {
	hz, ok := paramNumber(value)
	if !ok {
		return fmt.Sprintf("%v", value)
	}
	return fmt.Sprintf("%.3f MHz", hz/1000000.0)
}

// formatRadioBandwidth converts a Hz value to kHz display string.
func formatRadioBandwidth(value any) string {
	hz, ok := paramNumber(value)
	if !ok {
		return fmt.Sprintf("%v", value)
	}
	return fmt.Sprintf("%.1f kHz", hz/1000.0)
}

// interfaceShow is the FULL-PAGE interface detail view (Python ShowInterface,
// Interfaces.py:2198-2620) opened by Enter on an interface in the list — G1
// replaces the former tiny `Interface: <name>` modal. Layout (a Frame in a
// LineBox):
//
//	header: "Interface: <name>" centered + an "=" divider
//	body:   Type/Status info rows, a "-" divider, the TX/RX stat row, a "-"
//	        divider, the RX/TX traffic chart boxes (or the disconnected
//	        notice), a "-" divider, and the Connection/Radio/Network/IFAC/
//	        Additional parameter blocks (each sorted by key, "-" divider
//	        after; "No additional parameters" centered when none)
//	footer: an "=" divider + the [Back | Disable/Enable | Edit] button row
type interfaceShow struct {
	*tview.Flex
	app  *App
	info InterfaceInfo

	body      *showBody
	buttons   []*UrwidButton // Back, Disable/Enable, Edit — footer row order
	buttonRow *urwidColumns
	inFooter  bool
	btnFocus  int

	// chart tracks the interface's rx/tx byte counters and renders the 8-row
	// RX/TX traffic charts, mirroring Python ShowInterface.bandwidth_chart
	// (Interfaces.py:2308). It is fed on every interface snapshot while the
	// interface is connected (Python's update_bandwidth_charts poll,
	// Interfaces.py:2759-2790). chartFed records whether at least one poll
	// update has arrived, so the box shows Python's initial "Loading RX data..."
	// until then.
	chart    *InterfaceBandwidthChart
	chartFed bool
	// now, when set, supplies the clock for chart sampling so tests can drive a
	// deterministic rate series. Nil uses time.Now.
	now func() time.Time

	// OnBack switches back to the interface list (Python on_back →
	// switch_to_list, Interfaces.py:2817-2819 + 3011-3016).
	OnBack func()
	// OnToggle fires the Disable/Enable action (Python on_toggle_enabled,
	// Interfaces.py:2516).
	OnToggle func()
	// OnEdit fires the Edit action (Python on_edit → switch_to_edit_interface,
	// Interfaces.py:2821-2823).
	OnEdit func()
}

// NewInterfaceShow builds the full-page detail view for one interface.
func NewInterfaceShow(app *App, info InterfaceInfo) *interfaceShow {
	s := &interfaceShow{app: app, info: info}

	// Live bandwidth chart (Python ShowInterface.__init__ builds
	// InterfaceBandwidthChart(history_length, glyphset) and feeds it the current
	// counters once, Interfaces.py:2308-2309).
	s.chart = NewInterfaceBandwidthChart(60)
	s.chart.SetGlyphSet(s.glyphSet())
	s.chart.Update(float64(info.RX), float64(info.TX), s.clockNow())

	// Header (Interfaces.py:2222-2226): centered title + "=" divider.
	header := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(newCenteredText(tcell.ColorDefault, "Interface: "+info.Name), 1, 0, false).
		AddItem(newDividerRow("="), 1, 0, false)

	// Body (Interfaces.py:2268-2502). The traffic boxes it draws are as wide as
	// the grid leaves it, so showBody rebuilds the text when the width changes.
	s.body = newShowBody(s)

	// Footer (Interfaces.py:2228-2248): "=" divider + weighted button row —
	// Back (0.3), spacer (0.05), Disable/Enable (0.3), spacer (0.05), Edit
	// (0.3).
	toggleLabel := "Disable"
	if !info.Enabled {
		toggleLabel = "Enable"
	}
	s.buttons = []*UrwidButton{
		NewUrwidButton("Back").SetSelectedFunc(func() {
			if s.OnBack != nil {
				s.OnBack()
			}
		}),
		NewUrwidButton(toggleLabel).SetSelectedFunc(func() {
			if s.OnToggle != nil {
				s.OnToggle()
			}
		}),
		NewUrwidButton("Edit").SetSelectedFunc(func() {
			if s.OnEdit != nil {
				s.OnEdit()
			}
		}),
	}
	spacer := func() tview.Primitive { return tview.NewBox() }
	s.buttonRow = newURWIDColumns(0, s.buttons[0], spacer(), s.buttons[1], spacer(), s.buttons[2]).
		SetWeight(0, 30).SetWeight(1, 5).SetWeight(2, 30).SetWeight(3, 5).SetWeight(4, 30)
	footer := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(newDividerRow("="), 1, 0, false).
		AddItem(s.buttonRow, 1, 0, false)

	s.Flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 2, 0, false).
		AddItem(s.body, 0, 1, true).
		AddItem(footer, 2, 0, false)
	s.Flex.SetBorder(true)
	s.Flex.SetInputCapture(s.handleKey)
	return s
}

// Widget returns the full-page primitive.
func (s *interfaceShow) Widget() tview.Primitive { return s.Flex }

// glyphs returns the glyph set this view draws its status marker from.
func (s *interfaceShow) glyphs() GlyphSet {
	if s.app != nil && s.app.Glyphs != nil {
		return s.app.Glyphs
	}
	return glyphsUnicode
}

// glyphSet returns the chart glyph-set name ("unicode" or "plain"), mirroring
// Python's parent.glyphset passed to InterfaceBandwidthChart (Interfaces.py:2308).
func (s *interfaceShow) glyphSet() string {
	if s.app != nil && s.app.GlyphSet != "" {
		return s.app.GlyphSet
	}
	return "unicode"
}

// clockNow returns the chart sampler's clock, honouring an injected now (tests)
// and falling back to time.Now.
func (s *interfaceShow) clockNow() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// defaultBodyWidth is the width the body text is first built for, before the
// grid has measured the view. It only shows through in a capture that never
// lays the view out; a real layout calls SetRect and rebuilds at the true width.
const defaultBodyWidth = 80

// showBodyPadding is the body's left and right inset. Python wraps the whole
// body in Padding(left=2, right=2) (Interfaces.py:2465), so the traffic boxes and
// the dividers stop two columns short of the frame instead of merging with it.
const showBodyPadding = 2

// showBody is the detail view's scrolling body. What it holds depends on the
// width the grid gives it — the two traffic boxes are drawn to that width — so
// the text is rebuilt whenever the rect changes. Without that, the boxes keep
// whatever width they were first built at and their borders stop agreeing with
// the contents drawn inside them.
type showBody struct {
	*tview.TextView
	parent *interfaceShow
	width  int
}

// newShowBody creates the body, built once at defaultBodyWidth so that a capture
// which never lays it out still reads the whole view.
func newShowBody(parent *interfaceShow) *showBody {
	b := &showBody{TextView: tview.NewTextView(), parent: parent, width: -1}
	b.SetDynamicColors(true)
	b.SetScrollable(true)
	b.SetTextColor(tcell.ColorDefault)
	b.SetBackgroundColor(tcell.ColorDefault)
	b.SetWrap(true)
	b.SetBorderPadding(0, 0, showBodyPadding, showBodyPadding)
	b.width = defaultBodyWidth - 2*showBodyPadding
	b.TextView.SetText(parent.buildBodyText(b.width))
	return b
}

// SetRect rebuilds the body at the width the layout gives it.
func (b *showBody) SetRect(x, y, w, h int) {
	contentW := max(w-2*showBodyPadding, 1)
	if contentW != b.width {
		b.width = contentW
		b.TextView.SetText(b.parent.buildBodyText(contentW))
	}
	b.TextView.SetRect(x, y, w, h)
}

// Refresh rebuilds the body from the current interface reading at the width the
// layout last gave it, leaving the reader where they were.
func (b *showBody) Refresh() {
	if b.width <= 0 {
		b.width = defaultBodyWidth - 2*showBodyPadding
	}
	row, _ := b.TextView.GetScrollOffset()
	b.TextView.SetText(b.parent.buildBodyText(b.width))
	b.TextView.ScrollTo(row, 0)
}

// Name returns the interface this view was opened on.
func (s *interfaceShow) Name() string { return s.info.Name }

// SetInfo replaces the reading this view was built from. Python's ShowInterface
// polls its interface every second — start() arms poll_stats on a 1 s alarm
// (Interfaces.py:2921-2925), poll_stats re-arms itself (Interfaces.py:2958-2999)
// — and re-renders the Type/Status and TX/RX rows from it. Without this the one
// screen whose entire purpose is a live reading is the only stale one: a radio
// that comes up while the view is open keeps showing "Disconnected".
func (s *interfaceShow) SetInfo(info InterfaceInfo) {
	s.info = info
	if len(s.buttons) > 1 {
		label := "Disable"
		if !info.Enabled {
			label = "Enable"
		}
		if s.buttons[1].Label() != label {
			s.buttons[1].SetLabel(label)
		}
	}
	// Feed the bandwidth chart (Python update_bandwidth_charts calls
	// self.bandwidth_chart.update(rx, tx) each poll while the interface is
	// connected, Interfaces.py:2771-2778). A disconnect leaves the chart
	// untouched, exactly as Python returns early without updating it.
	if info.Connected {
		if s.chart == nil {
			s.chart = NewInterfaceBandwidthChart(60)
			s.chart.SetGlyphSet(s.glyphSet())
		}
		s.chart.Update(float64(info.RX), float64(info.TX), s.clockNow())
		s.chartFed = true
	}
	s.body.Refresh()
}

// buildBodyText assembles the scrollable body rows with color tags. Python
// styles "key"/"value"/"rx"/"tx" are absent from the palette, so urwid renders
// them in the terminal default; the Status row uses connected_status /
// disconnected_status. width is the body's laid-out width, which the traffic
// boxes are drawn to.
func (s *interfaceShow) buildBodyText(width int) string {
	var b strings.Builder
	tag := func(style string) string {
		tc := GetThemeColors(ThemeDark)
		fg := tc[style+"_fg"]
		bg := tc[style+"_bg"]
		return buildColorTag(fg, bg)
	}
	connStyle := "disconnected_status"
	if s.info.Enabled {
		connStyle = "connected_status"
	}
	connTag := tag(connStyle)
	reset := "[-:-]"

	g := s.glyphs()
	marker := g["unselected"]
	if s.info.Enabled {
		marker = g["selected"]
	}
	connWord := "Disconnected"
	if s.info.Connected {
		connWord = "Connected"
	}

	// Python's urwid.Divider("-") fills its whole row with the divider
	// character; these rows are already inside the body's padding.
	divider := strings.Repeat("-", max(width, 1))

	// Info rows (Interfaces.py:2278-2300). Type is a raw RNS-config value and
	// this body is a dynamic-colors TextView.
	b.WriteString("Type:    " + escapeTviewTags(s.info.Type) + "\n")
	b.WriteString("Status:  " + connTag + marker + reset + "  " + connTag + statusWord(s.info.Enabled) + reset + "  |  " + connTag + connWord + reset + "\n")
	b.WriteString(divider + "\n")

	// TX/RX stat row (Interfaces.py:2302-2308).
	b.WriteString("TX:      " + FormatBytes(float64(s.info.TX)) + "\n")
	b.WriteString("RX:      " + FormatBytes(float64(s.info.RX)) + "\n")
	b.WriteString(divider + "\n")

	// Charts (Interfaces.py:2312-2360): the RX/TX traffic boxes while the
	// interface is connected; otherwise the notice box titled "Bandwidth
	// Charts". Their width comes from the grid, so they are drawn for the body
	// width this text is being built for.
	for _, line := range s.chartsRows(width) {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(divider + "\n")

	// Parameter blocks (Interfaces.py:2436-2492).
	b.WriteString(s.paramBlocks(width))
	return b.String()
}

// boxAlign places a drawn box's content row inside its border.
type boxAlign int

const (
	boxLeft boxAlign = iota
	boxRight
	boxCenter
)

// boxRow is one content row of a drawn box.
type boxRow struct {
	text  string
	align boxAlign
}

// drawBox renders a titled, bordered box exactly width columns wide: a top
// border carrying the title, the content rows, and a bottom border. Python
// builds these as real urwid LineBox widgets (Interfaces.py:2316-2348), which
// take their width from the grid; drawing them into a text body means computing
// that width here and keeping every row of the box the same width, or the
// borders stop agreeing with the contents.
func drawBox(width int, title string, rows []boxRow) []string {
	// "┌─ " + title + " " + dashes + "┐"
	const overhead = 5
	if width < len(title)+overhead {
		width = len(title) + overhead
	}
	inner := width - 2
	out := make([]string, 0, len(rows)+2)
	out = append(out, "┌─ "+title+" "+strings.Repeat("─", width-len(title)-overhead)+"┐")
	for _, r := range rows {
		out = append(out, "│"+alignInBox(r.text, inner, r.align)+"│")
	}
	out = append(out, "└"+strings.Repeat("─", inner)+"┘")
	return out
}

// alignInBox pads or truncates text to exactly width cells, per align.
func alignInBox(text string, width int, align boxAlign) string {
	runes := []rune(text)
	if len(runes) > width {
		runes = runes[:width]
	}
	text = string(runes)
	pad := width - len(runes)
	switch align {
	case boxRight:
		return strings.Repeat(" ", pad) + text
	case boxCenter:
		left := pad / 2
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", pad-left)
	default:
		return text + strings.Repeat(" ", pad)
	}
}

// chartsRows renders the traffic boxes for a body of the given width, or the
// disconnected notice box. Python stacks the two boxes below 145 columns and
// puts them side by side at 145 and above (Interfaces.py:2350-2360), and shows
// the notice box instead of either whenever the interface is not connected.
func (s *interfaceShow) chartsRows(width int) []string {
	if !s.info.Connected {
		return drawBox(width, "Bandwidth Charts", []boxRow{
			{text: "Charts not available - Interface is not connected", align: boxCenter},
		})
	}
	// Each box holds the RX/TX traffic chart body (or Python's initial
	// "Loading RX data..." before the first poll) and a right-aligned peak row
	// (Interfaces.py:2312-2333).
	rxRows, txRows := s.chartBoxRows()
	if width >= 145 {
		left := width / 2
		rx := drawBox(left, "RX Traffic (60s)", rxRows)
		tx := drawBox(width-left, "TX Traffic (60s)", txRows)
		return zipBoxRows(rx, tx)
	}
	rx := drawBox(width, "RX Traffic (60s)", rxRows)
	return append(rx, drawBox(width, "TX Traffic (60s)", txRows)...)
}

// chartBoxRows returns the RX and TX box contents: the traffic chart body (one
// boxRow per rendered chart line, or the initial "Loading RX data..." line) and
// the right-aligned peak row, matching Python's rx_box/tx_box Pile
// (Interfaces.py:2316-2333). The peak label uses RNS.prettyspeed, so the
// pre-initialisation value reads "0 bps".
func (s *interfaceShow) chartBoxRows() (rxRows, txRows []boxRow) {
	if s.chart == nil || !s.chartFed {
		return loadingChartRows("Loading RX data..."), loadingChartRows("Loading TX data...")
	}
	rxChart, txChart, peakRX, peakTX := s.chart.GetCharts(8)
	return chartRows(rxChart, peakRX), chartRows(txChart, peakTX)
}

// loadingChartRows is the pre-poll box content: the loading line plus Python's
// literal "Peak: 0 B/s" placeholder (Interfaces.py:2313-2317).
func loadingChartRows(loading string) []boxRow {
	return []boxRow{
		{text: loading, align: boxLeft},
		{text: "Peak: 0 B/s", align: boxRight},
	}
}

// chartRows turns a rendered chart string plus its peak label into box rows: one
// left-aligned row per chart line, then the right-aligned "Peak: <label>" row.
func chartRows(chart, peak string) []boxRow {
	lines := strings.Split(chart, "\n")
	rows := make([]boxRow, 0, len(lines)+1)
	for _, line := range lines {
		rows = append(rows, boxRow{text: line, align: boxLeft})
	}
	rows = append(rows, boxRow{text: "Peak: " + peak, align: boxRight})
	return rows
}

// zipBoxRows places two equally tall boxes side by side, as urwid's
// Columns([(WEIGHT,1,rx_box),(WEIGHT,1,tx_box)]) does.
func zipBoxRows(a, b []string) []string {
	out := make([]string, 0, len(a))
	for i := range a {
		if i < len(b) {
			out = append(out, a[i]+b[i])
			continue
		}
		out = append(out, a[i])
	}
	return out
}

// statusWord maps the enabled flag to Python's Enabled/Disabled words.
func statusWord(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

// paramBlocks renders the Connection/Radio/Network/IFAC/Additional parameter
// groups, sorted by key (Python Interfaces.py:2436-2498), with the MHz/kHz and
// Yes/No value formatting of create_param_row (Interfaces.py:2400-2434) and a
// full-width "-" divider after each group.
func (s *interfaceShow) paramBlocks(width int) string {
	var b strings.Builder
	groups := CategorizeInterfaceParams(paramMapToAny(s.info.Params))
	emit := func(title string, m map[string]any) {
		if len(m) == 0 {
			return
		}
		b.WriteString(title + "\n")
		for _, k := range SortedKeys(m) {
			b.WriteString(escapeTviewTags(FormatParamKey(k)) + ": " + escapeTviewTags(FormatParamValue(k, m[k])) + "\n")
		}
		b.WriteString(strings.Repeat("-", max(width, 1)) + "\n")
	}
	emit("Connection Parameters", groups.Connection)
	emit("Radio Parameters", groups.Radio)
	emit("Network Parameters", groups.Network)
	emit("IFAC Parameters", groups.IFAC)
	emit("Additional Parameters", groups.Other)
	if len(groups.Connection)+len(groups.Radio)+len(groups.Network)+len(groups.IFAC)+len(groups.Other) == 0 {
		// Python centers this row (Interfaces.py:2454-2455).
		b.WriteString(alignInBox("No additional parameters", max(width, 1), boxCenter) + "\n")
	}
	return b.String()
}

// paramMapToAny widens the string params for the existing categorizer.
func paramMapToAny(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// handleKey implements the ShowInterface keypress (Interfaces.py:2644-2700):
// Tab body→footer and Back→Disable/Enable→Edit cycling in the footer;
// Shift-Tab footer→body; Down at the body bottom → footer (Back); Up in the
// footer → body (scrolled to the bottom, Interfaces.py:2682-2690); Left/Right
// cycle the buttons in the footer; Esc → Back (the accepted Go enhancement).
func (s *interfaceShow) handleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyTab:
		if !s.inFooter {
			s.enterFooter(0)
			return nil
		}
		s.btnFocus = (s.btnFocus + 1) % len(s.buttons)
		s.syncFooterFocus()
		return nil
	case tcell.KeyBacktab:
		if s.inFooter {
			if s.btnFocus == 0 {
				s.exitFooter()
			} else {
				s.btnFocus--
				s.syncFooterFocus()
			}
			return nil
		}
	case tcell.KeyDown:
		if !s.inFooter && s.bodyAtBottom() {
			s.enterFooter(0)
			return nil
		}
	case tcell.KeyUp:
		if s.inFooter {
			s.exitFooter()
			return nil
		}
	case tcell.KeyLeft:
		if s.inFooter {
			s.btnFocus = (s.btnFocus - 1 + len(s.buttons)) % len(s.buttons)
			s.syncFooterFocus()
			return nil
		}
	case tcell.KeyRight:
		if s.inFooter {
			s.btnFocus = (s.btnFocus + 1) % len(s.buttons)
			s.syncFooterFocus()
			return nil
		}
	case tcell.KeyEscape:
		if s.OnBack != nil {
			s.OnBack()
		}
		return nil
	}
	return event
}

// enterFooter moves focus to the footer button row at the given button index
// (Python Interfaces.py:2645-2656: footer focus + button row focus 0).
func (s *interfaceShow) enterFooter(btn int) {
	s.inFooter = true
	s.btnFocus = btn
	s.syncFooterFocus()
}

// exitFooter returns focus to the body, scrolled to its bottom (Python
// Interfaces.py:2682-2690 sets the listbox focus to the last entry).
func (s *interfaceShow) exitFooter() {
	s.inFooter = false
	s.body.ScrollToEnd()
	if s.app != nil {
		s.app.SetFocus(s.body)
		return
	}
	s.body.Focus(func(tview.Primitive) {})
}

// syncFooterFocus moves the application focus to the button at btnFocus so
// Enter reaches it (urwid moves focus_position; tview needs the real focus).
func (s *interfaceShow) syncFooterFocus() {
	if s.btnFocus >= 0 && s.btnFocus < len(s.buttons) {
		if s.app != nil {
			s.app.SetFocus(s.buttons[s.btnFocus])
			return
		}
		s.buttons[s.btnFocus].Focus(func(tview.Primitive) {})
	}
}

// bodyAtBottom reports whether the body TextView is scrolled to its last line
// (Python: super().keypress("down") returns "down" at the bottom of the
// ListBox body, Interfaces.py:2669-2680).
func (s *interfaceShow) bodyAtBottom() bool {
	_, _, _, h := s.body.GetInnerRect()
	row, _ := s.body.GetScrollOffset()
	total := len(tview.WordWrap(s.body.GetText(false), max(s.bodyWidth(), 1)))
	return row+h >= total
}

// bodyWidth returns the body's laid-out inner width (0 before layout).
func (s *interfaceShow) bodyWidth() int {
	_, _, w, _ := s.body.GetInnerRect()
	return w
}
