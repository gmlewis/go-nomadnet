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
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

// formatRadioFrequency converts a Hz value to MHz display string.
func formatRadioFrequency(value any) string {
	var hz float64
	switch v := value.(type) {
	case float64:
		hz = v
	case int64:
		hz = float64(v)
	case int:
		hz = float64(v)
	default:
		return fmt.Sprintf("%v", value)
	}
	mhz := hz / 1000000.0
	return fmt.Sprintf("%.3f MHz", mhz)
}

// formatRadioBandwidth converts a Hz value to kHz display string.
func formatRadioBandwidth(value any) string {
	var hz float64
	switch v := value.(type) {
	case float64:
		hz = v
	case int64:
		hz = float64(v)
	case int:
		hz = float64(v)
	default:
		return fmt.Sprintf("%v", value)
	}
	khz := hz / 1000.0
	return fmt.Sprintf("%.1f kHz", khz)
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

	body      *tview.TextView
	buttons   []*UrwidButton // Back, Disable/Enable, Edit — footer row order
	buttonRow *urwidColumns
	inFooter  bool
	btnFocus  int

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
	g := glyphsUnicode
	if app != nil && app.Glyphs != nil {
		g = app.Glyphs
	}

	s := &interfaceShow{app: app, info: info}

	// Header (Interfaces.py:2222-2226): centered title + "=" divider.
	header := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(newCenteredText(tcell.ColorDefault, "Interface: "+info.Name), 1, 0, false).
		AddItem(newDividerRow("="), 1, 0, false)

	// Body (Interfaces.py:2268-2502).
	s.body = tview.NewTextView()
	s.body.SetDynamicColors(true)
	s.body.SetScrollable(true)
	s.body.SetTextColor(tcell.ColorDefault)
	s.body.SetBackgroundColor(tcell.ColorDefault)
	s.body.SetWrap(true)
	s.body.SetText(s.buildBodyText(g))

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

// buildBodyText assembles the scrollable body rows with color tags. Python
// styles "key"/"value"/"rx"/"tx" are absent from the palette, so urwid renders
// them in the terminal default; the Status row uses connected_status /
// disconnected_status.
func (s *interfaceShow) buildBodyText(g GlyphSet) string {
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

	marker := g["unselected"]
	if s.info.Enabled {
		marker = g["selected"]
	}
	connWord := "Disconnected"
	if s.info.Connected {
		connWord = "Connected"
	}

	// Info rows (Interfaces.py:2278-2300).
	b.WriteString("Type:    " + s.info.Type + "\n")
	b.WriteString("Status:  " + connTag + marker + reset + "  " + connTag + statusWord(s.info.Enabled) + reset + "  |  " + connTag + connWord + reset + "\n")
	b.WriteString("---\n")

	// TX/RX stat row (Interfaces.py:2302-2308).
	b.WriteString("TX:      " + FormatBytes(float64(s.info.TX)) + "\n")
	b.WriteString("RX:      " + FormatBytes(float64(s.info.RX)) + "\n")
	b.WriteString("---\n")

	// Charts (Interfaces.py:2312-2346): the chart boxes when connected;
	// otherwise the disconnected notice box titled "Bandwidth Charts".
	if s.info.Connected {
		b.WriteString("┌─ RX Traffic (60s) ─────┐  ┌─ TX Traffic (60s) ─────┐\n")
		b.WriteString("│ Loading RX data...     │  │ Loading TX data...     │\n")
		b.WriteString("│                 Peak: 0 B/s  │                 Peak: 0 B/s │\n")
		b.WriteString("└────────────────────────┘  └────────────────────────┘\n")
	} else {
		b.WriteString("┌─ Bandwidth Charts ─────┐\n")
		b.WriteString("│ Charts not available - Interface is not connected │\n")
		b.WriteString("└────────────────────────┘\n")
	}
	b.WriteString("---\n")

	// Parameter blocks (Interfaces.py:2436-2492).
	b.WriteString(s.paramBlocks())
	return b.String()
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
// Yes/No value formatting of create_param_row (Interfaces.py:2400-2434).
func (s *interfaceShow) paramBlocks() string {
	var b strings.Builder
	groups := CategorizeInterfaceParams(paramMapToAny(s.info.Params))
	emit := func(title string, m map[string]any) {
		if len(m) == 0 {
			return
		}
		b.WriteString(title + "\n")
		for _, k := range SortedKeys(m) {
			b.WriteString(FormatParamKey(k) + ": " + FormatParamValue(k, m[k]) + "\n")
		}
		b.WriteString("---\n")
	}
	emit("Connection Parameters", groups.Connection)
	emit("Radio Parameters", groups.Radio)
	emit("Network Parameters", groups.Network)
	emit("IFAC Parameters", groups.IFAC)
	emit("Additional Parameters", groups.Other)
	if len(groups.Connection)+len(groups.Radio)+len(groups.Network)+len(groups.IFAC)+len(groups.Other) == 0 {
		b.WriteString("No additional parameters\n")
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
