// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// screen.go parses `tmux capture-pane -e -p` output (which embeds real ANSI
// SGR escapes) into a 2D cell grid, and exposes query helpers that let the test
// driver actively verify the TUI's state instead of blindly sleeping.
//
// Per a full audit of the Go NomadNet render path (dark theme, true color), the
// ONLY focus signal that is distinguishable by SGR color is the tview.List
// selected row — fg #111111 bg #aaaaaa (full-line, palette.go "list_focus"). The
// menu buttons, tab buttons, and action buttons are all rendered in a neutral
// style whether focused or not; their focus is indicated solely by the terminal
// hardware cursor position, which `tmux capture-pane -e` does NOT embed as SGR.
// So the screen model is paired with a cursor position (queried separately via
// `tmux display-message #{cursor_x},#{cursor_y}`) in the View type, and the
// focus-detecting helpers use the cursor position rather than color.

// Sentinel for "default" fg/bg (the terminal default color, emitted as SGR 39/49
// or the absence of any color directive). Distinct from black (0x000000).
const colorDefault int32 = -1

// Cell is one screen cell with its decoded SGR style.
type Cell struct {
	Ch            rune
	FG, BG        int32 // colorDefault (-1) = terminal default; else 0xRRGGBB
	Bold, Reverse bool
	Underline     bool
}

// Screen is the decoded 2D grid of the visible pane.
type Screen struct {
	W, H int
	Rows [][]Cell
}

// rowText returns row y as a string (cells with no rune become spaces). Used
// for plain-text queries (border titles, "Retrieving", "URL: ", error strings)
// where color is irrelevant.
func (s *Screen) rowText(y int) string {
	if y < 0 || y >= len(s.Rows) {
		return ""
	}
	var b strings.Builder
	for _, c := range s.Rows[y] {
		if c.Ch != 0 {
			b.WriteRune(c.Ch)
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// fullText returns the whole screen as a newline-joined string.
func (s *Screen) fullText() string {
	var b strings.Builder
	for y := 0; y < s.H; y++ {
		b.WriteString(strings.TrimRight(s.rowText(y), " "))
		b.WriteByte('\n')
	}
	return b.String()
}

// parseScreen decodes `tmux capture-pane -e -p` output into a cell grid. It
// tracks SGR state (fg/bg/bold/reverse/underline), newline/carriage-return, and
// skips other CSI/OSC sequences. Cell columns are grown as characters are
// written; short rows are padded to the screen width at the end.
func parseScreen(raw []byte) *Screen {
	type rowState struct{ cells []Cell }

	var rows [][]Cell
	cur := Cell{FG: colorDefault, BG: colorDefault}
	cx, cy := 0, 0

	ensureRow := func(y int) []Cell {
		for len(rows) <= y {
			rows = append(rows, nil)
		}
		return rows[y]
	}
	writeCell := func(x, y int, c Cell) {
		r := ensureRow(y)
		for len(r) <= x {
			r = append(r, Cell{FG: colorDefault, BG: colorDefault})
		}
		r[x] = c
		rows[y] = r
	}
	// eraseToEndOfLine handles SGR K (EL): blank from cx to end of current row.
	eraseToEndOfLine := func(x, y int) {
		r := ensureRow(y)
		for i := x; i < len(r); i++ {
			r[i] = Cell{FG: colorDefault, BG: colorDefault}
		}
		rows[y] = r
	}

	i := 0
	n := len(raw)
	for i < n {
		b := raw[i]
		switch {
		case b == 0x1b: // ESC
			if i+1 >= n {
				i++
				continue
			}
			nb := raw[i+1]
			switch nb {
			case '[': // CSI
				j := i + 2
				// collect params (digits and ';'), up to a final byte 0x40..0x7e
				for j < n && (raw[j] == ';' || (raw[j] >= '0' && raw[j] <= '9') || raw[j] == '?') {
					j++
				}
				if j >= n {
					i = n
					continue
				}
				final := raw[j]
				params := string(raw[i+2 : j])
				j++ // consume final byte
				switch final {
				case 'm':
					applySGR(params, &cur)
				case 'K': // erase line (EL), param 0 = to end of line
					eraseToEndOfLine(cx, cy)
				case 'J': // erase display (ED); treat 0/2 as clear from cursor
					// Approximate: 2J clears whole screen is rarely emitted by
					// tview; 0J erases from cursor to end of screen.
					if params == "" || params == "0" {
						eraseToEndOfLine(cx, cy)
						for yy := cy + 1; yy < len(rows); yy++ {
							rows[yy] = rows[yy][:0]
						}
					}
				case 'H', 'f': // CUP/HVP cursor position
					x, y := 1, 1
					if params != "" {
						parts := strings.Split(params, ";")
						if len(parts) >= 1 {
							if v, err := strconv.Atoi(parts[0]); err == nil {
								y = v
							}
						}
						if len(parts) >= 2 {
							if v, err := strconv.Atoi(parts[1]); err == nil {
								x = v
							}
						}
					}
					cx, cy = x-1, y-1
					if cx < 0 {
						cx = 0
					}
					if cy < 0 {
						cy = 0
					}
				case 'A':
					cy -= csiInt(params, 1)
					if cy < 0 {
						cy = 0
					}
				case 'B':
					cy += csiInt(params, 1)
				case 'C':
					cx += csiInt(params, 1)
				case 'D':
					cx -= csiInt(params, 1)
					if cx < 0 {
						cx = 0
					}
				default:
					// other CSI (cursor show/hide ?25h/l, etc.) — ignore
				}
				i = j
			case ']': // OSC ... BEL or ST (ESC \)
				j := i + 2
				for j < n && raw[j] != 0x07 {
					if raw[j] == 0x1b && j+1 < n && raw[j+1] == '\\' {
						j++ // consume ST
						break
					}
					j++
				}
				if j < n && raw[j] == 0x07 {
					j++
				}
				i = j
			default:
				// other ESC (e.g. ESC =, ESC >, ESC M) — skip ESC + next
				i += 2
			}
		case b == '\r':
			cx = 0
			i++
		case b == '\n', b == 0x0b, b == 0x0c:
			cy++
			cx = 0
			i++
		case b == 0x08: // backspace
			if cx > 0 {
				cx--
			}
			i++
		default:
			if b < 0x80 {
				writeCell(cx, cy, Cell{Ch: rune(b), FG: cur.FG, BG: cur.BG, Bold: cur.Bold, Reverse: cur.Reverse, Underline: cur.Underline})
				cx++
				i++
			} else {
				r, size := utf8.DecodeRune(raw[i:])
				if r == utf8.RuneError && size <= 1 {
					// lone invalid byte; emit replacement, advance one
					writeCell(cx, cy, Cell{Ch: ' ', FG: cur.FG, BG: cur.BG})
					cx++
					i++
					continue
				}
				// Combining/zero-width runes would skew columns; tview places
				// each printable rune in its own cell, so treat every decoded
				// rune as one cell. (capture-pane emits one cell per column.)
				writeCell(cx, cy, Cell{Ch: r, FG: cur.FG, BG: cur.BG, Bold: cur.Bold, Reverse: cur.Reverse, Underline: cur.Underline})
				cx++
				i += size
			}
		}
	}

	// Normalize: compute width, pad every row to that width.
	w := 0
	for _, r := range rows {
		if len(r) > w {
			w = len(r)
		}
	}
	if w == 0 {
		w = 1
	}
	for y, r := range rows {
		for len(r) < w {
			r = append(r, Cell{FG: colorDefault, BG: colorDefault})
		}
		rows[y] = r
	}
	return &Screen{W: w, H: len(rows), Rows: rows}
}

func csiInt(params string, def int) int {
	if params == "" {
		return def
	}
	if v, err := strconv.Atoi(strings.Split(params, ";")[0]); err == nil {
		if v == 0 {
			return def
		}
		return v
	}
	return def
}

// applySGR updates the current cell style from an SGR parameter string.
func applySGR(params string, cur *Cell) {
	if params == "" {
		params = "0"
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		v, err := strconv.Atoi(p)
		if err != nil {
			// ignore non-numeric (e.g. private markers)
			continue
		}
		switch {
		case v == 0:
			*cur = Cell{FG: colorDefault, BG: colorDefault}
		case v == 1:
			cur.Bold = true
		case v == 2 || v == 22:
			cur.Bold = false // 22 = normal intensity
		case v == 4:
			cur.Underline = true
		case v == 24:
			cur.Underline = false
		case v == 7:
			cur.Reverse = true
		case v == 27:
			cur.Reverse = false
		case v == 39:
			cur.FG = colorDefault
		case v == 49:
			cur.BG = colorDefault
		case v >= 30 && v <= 37:
			cur.FG = basic8(v - 30)
		case v >= 40 && v <= 47:
			cur.BG = basic8(v - 40)
		case v >= 90 && v <= 97:
			cur.FG = basic8(v - 90 + 8)
		case v >= 100 && v <= 107:
			cur.BG = basic8(v - 100 + 8)
		case v == 38 || v == 48:
			// extended color: 38;5;n  |  38;2;r;g;b  (and 48 counterparts)
			if i+1 >= len(parts) {
				continue
			}
			mode, _ := strconv.Atoi(parts[i+1])
			switch mode {
			case 5: // 256-color
				if i+2 < len(parts) {
					idx, _ := strconv.Atoi(parts[i+2])
					c := xterm256(idx)
					if v == 38 {
						cur.FG = c
					} else {
						cur.BG = c
					}
					i += 2
				}
			case 2: // truecolor r;g;b
				if i+4 < len(parts) {
					r, _ := strconv.Atoi(parts[i+2])
					g, _ := strconv.Atoi(parts[i+3])
					bl, _ := strconv.Atoi(parts[i+4])
					c := int32(r)<<16 | int32(g)<<8 | int32(bl)
					if v == 38 {
						cur.FG = c
					} else {
						cur.BG = c
					}
					i += 4
				}
			}
		}
	}
}

// basic8 maps the 8 ANSI base colors (and bright offsets via +8) to the xterm
// palette RGB values.
func basic8(idx int) int32 {
	switch idx {
	case 0:
		return 0x000000 // black
	case 1:
		return 0x800000 // red
	case 2:
		return 0x008000 // green
	case 3:
		return 0x808000 // yellow
	case 4:
		return 0x000080 // blue
	case 5:
		return 0x800080 // magenta
	case 6:
		return 0x008080 // cyan
	case 7:
		return 0xc0c0c0 // white
	case 8:
		return 0x808080 // bright black (grey)
	case 9:
		return 0xff0000 // bright red
	case 10:
		return 0x00ff00 // bright green
	case 11:
		return 0xffff00 // bright yellow
	case 12:
		return 0x0000ff // bright blue
	case 13:
		return 0xff00ff // bright magenta
	case 14:
		return 0x00ffff // bright cyan
	case 15:
		return 0xffffff // bright white
	}
	return colorDefault
}

// xterm256 decodes a 256-color palette index to RGB.
func xterm256(idx int) int32 {
	switch {
	case idx < 0:
		return colorDefault
	case idx < 8:
		return basic8(idx)
	case idx < 16:
		return basic8(idx)
	case idx < 232:
		idx -= 16
		r := idx / 36
		g := (idx / 6) % 6
		b := idx % 6
		cube := func(v int) int32 {
			switch v {
			case 0:
				return 0
			case 1:
				return 95
			case 2:
				return 135
			case 3:
				return 175
			case 4:
				return 215
			case 5:
				return 255
			}
			return 0
		}
		return cube(r)<<16 | cube(g)<<8 | cube(b)
	default:
		// grayscale ramp 232..255: 8..238 in steps of 10
		v := int32(8 + (idx-232)*10)
		return v<<16 | v<<8 | v
	}
}

// --- focus / state constants from the render audit ---

const (
	listFocusBG = int32(0xaaaaaa) // palette "list_focus" bg — the ONLY SGR focus signal
)

// menuLabels is the menu-bar button text in order (tui/theme.go MenuItems),
// rendered as "[ <Label> ]" with one space between buttons.
var menuLabels = []string{
	"Conversations", "Network", "Channels", "Log",
	"Interfaces", "Config", "Guide", "Quit",
}

// ListRow is a detected list entry: its screen row Y and whether it is the
// selected (cursor) row, detected by the full-line #aaaaaa background.
type ListRow struct {
	Y        int
	Text     string
	Selected bool
}

// View bundles the decoded screen with the terminal cursor position, so a
// single Wait condition can assert against both color and cursor.
type View struct {
	Screen   *Screen
	CursorX  int
	CursorY  int
	CursorOK bool
}

// MenuButton is a parsed menu-bar button with its column span on row 0.
type MenuButton struct {
	Index int
	Label string
	Start int // column of the '[' (inclusive)
	End   int // column after the ']' (exclusive)
}

// menuButtons parses the "[ Label ]" buttons from row 0.
func (v *View) menuButtons() []MenuButton {
	if v.Screen == nil || len(v.Screen.Rows) == 0 {
		return nil
	}
	row := v.Screen.rowText(0)
	var out []MenuButton
	for _, label := range menuLabels {
		target := "[ " + label + " ]"
		idx := strings.Index(row, target)
		if idx < 0 {
			continue
		}
		// Only accept the FIRST occurrence per label (buttons are unique).
		out = append(out, MenuButton{
			Index: indexOfLabel(label),
			Label: label,
			Start: idx,
			End:   idx + len(target),
		})
	}
	return out
}

func indexOfLabel(label string) int {
	for i, l := range menuLabels {
		if l == label {
			return i
		}
	}
	return -1
}

// MenuFocusedButton returns the index of the menu button the hardware cursor is
// on, only when the cursor is on the menu bar row (y==0). This is the reliable
// menu-focus check: the menu buttons are color-neutral, so the cursor position
// is the sole indicator. ok=false if the cursor is not on row 0 or not within a
// button.
func (v *View) MenuFocusedButton() (int, bool) {
	if !v.CursorOK || v.CursorY != 0 {
		return -1, false
	}
	for _, b := range v.menuButtons() {
		if v.CursorX >= b.Start && v.CursorX < b.End {
			return b.Index, true
		}
	}
	return -1, false
}

// ActivePage infers the current body page from border titles / distinctive
// body text. It scans rows y>=1 only so the menu-bar buttons on row 0 do not
// cause false positives. Returns one of the page keys or "" if unknown.
func (v *View) ActivePage() string {
	if v.Screen == nil {
		return ""
	}
	// Gather body text (rows below the menu bar).
	var body strings.Builder
	for y := 1; y < v.Screen.H; y++ {
		body.WriteString(v.Screen.rowText(y))
		body.WriteByte('\n')
	}
	s := body.String()
	// "Topics" is the Guide's left-pane border title — the distinctive signal
	// the blind script never checked for (so it walked 12 "topics" on the
	// Network page). Match it as a border title: flanked by border dashes.
	if hasBorderTitle(s, "Topics") {
		return "guide"
	}
	if hasBorderTitle(s, "Announce Stream") || hasBorderTitle(s, "Saved Nodes") ||
		hasBorderTitle(s, "Announce Info") || hasBorderTitle(s, "Remote Node") {
		return "network"
	}
	if hasBorderTitle(s, "Conversations") {
		return "conversations"
	}
	if hasBorderTitle(s, "Channels") {
		return "channels"
	}
	if hasBorderTitle(s, "Local Peer Info") || hasBorderTitle(s, "Local Node Info") {
		return "network" // network page side panels
	}
	// Interfaces page header is a centered "Interfaces" text (no border), and
	// Config shows an "Open Editor" button + "edit the config file" explainer.
	if strings.Contains(s, "Open Editor") || strings.Contains(s, "edit the config file") {
		return "config"
	}
	if hasCenteredText(s, "Interfaces") {
		return "interfaces"
	}
	return ""
}

// hasBorderTitle reports whether title appears as a border title, i.e. on a
// row that is mostly box-drawing dashes (tview's top border with SetTitledBorder
// renders " Title " inside a row of '─' runes, U+2500). A row counts as a
// border row if it contains the '─' substring at least 3 times AND contains the
// title text. This is robust to terminal width without parsing border geometry.
func hasBorderTitle(s, title string) bool {
	const dash = "─" // U+2500 BOX DRAWINGS LIGHT HORIZONTAL
	for _, line := range strings.Split(s, "\n") {
		if !strings.Contains(line, title) {
			continue
		}
		if strings.Count(line, dash) >= 3 {
			return true
		}
	}
	return false
}

// hasCenteredText reports whether text appears on its own (surrounded by
// whitespace) on a body line — used for the Interfaces centered header which is
// not a border title.
func hasCenteredText(s, text string) bool {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == text {
			return true
		}
	}
	return false
}

// ListSelectedRows returns rows that carry the list_focus background (#aaaaaa,
// full-line). Each is a selected/cursor row of some tview.List on screen. The
// caller scopes these to a region (e.g. the Announce Stream left pane or the
// Guide topics box) by row Y range.
func (v *View) ListSelectedRows() []ListRow {
	if v.Screen == nil {
		return nil
	}
	var out []ListRow
	for y := 0; y < v.Screen.H; y++ {
		row := v.Screen.Rows[y]
		if len(row) == 0 {
			continue
		}
		bgCount := 0
		for _, c := range row {
			if c.BG == listFocusBG {
				bgCount++
			}
		}
		// A full-line highlighted list row fills most of its width with the
		// #aaaaaa bg. Require a meaningful span to avoid noise.
		if bgCount >= 3 {
			out = append(out, ListRow{Y: y, Text: strings.TrimSpace(v.Screen.rowText(y)), Selected: true})
		}
	}
	return out
}

// FirstSelectedRow returns the topmost #aaaaaa-bg row in y range [yMin,yMax]
// (inclusive), or nil. Used to find the Announce Stream / Guide topic cursor.
func (v *View) FirstSelectedRow(yMin, yMax int) *ListRow {
	for _, r := range v.ListSelectedRows() {
		if r.Y >= yMin && r.Y <= yMax {
			rr := r
			return &rr
		}
	}
	return nil
}

// networkLeftWidth is the fixed width of the Network page's left list pane
// (tui/network.go:231 SetFixedWidth(0, 52)).
const networkLeftWidth = 52

// announceNodeGlyph is the circled-N rune that prefixes every announce-stream
// NODE entry (tui/glyphs.go "node" = "Ⓝ "; the row reads "{ts} Ⓝ  {name}").
// Peer and propagation-node entries use different glyphs, so matching this
// rune specifically selects connectable nodes.
const announceNodeGlyph = 'Ⓝ'

// leftPaneText returns the trimmed text of the left (list) pane of row y — the
// cells with x < networkLeftWidth. The Network page's left pane is a fixed
// 52 cols, so this isolates the announce-stream row regardless of where the
// cursor sits (Python's ilb reports cursor_x at the right edge, ~135).
func (v *View) leftPaneText(y int) string {
	if v.Screen == nil || y < 0 || y >= len(v.Screen.Rows) {
		return ""
	}
	row := v.Screen.Rows[y]
	var b strings.Builder
	for x := 0; x < networkLeftWidth && x < len(row); x++ {
		ch := row[x].Ch
		if ch == 0 {
			ch = ' '
		}
		b.WriteRune(ch)
	}
	return strings.TrimSpace(b.String())
}

// cursorOnAnnounceNodeRow reports whether the hardware cursor is on an
// announce-stream NODE row: the left pane of the row at CursorY contains the
// Ⓝ glyph. This is the PRIMARY selection signal for the Python original,
// which renders the selected node row with NO background fill (just fg
// #afafaf) and signals selection solely via the cursor position. (The Go port
// additionally fills the selected row with #aaaaaa — see SelectedAnnounceRow's
// fallback.) cursor_x is unreliable for Python's ilb (it reports the right
// edge, ~135), so only cursor_y is used.
func (v *View) cursorOnAnnounceNodeRow() bool {
	if !v.CursorOK || v.Screen == nil {
		return false
	}
	if v.CursorY < 0 || v.CursorY >= v.Screen.H {
		return false
	}
	return strings.ContainsRune(v.leftPaneText(v.CursorY), announceNodeGlyph)
}

// HasAnnounceNode reports whether any announce-stream NODE row (a left-pane
// row containing Ⓝ) is present on screen. Used by Phase 2 to wait for
// announcing nodes to populate WITHOUT relying on the cursor (which flickers
// because the stream re-sorts as new announces arrive).
func (v *View) HasAnnounceNode() bool {
	if v.Screen == nil {
		return false
	}
	for y := 0; y < v.Screen.H; y++ {
		if strings.ContainsRune(v.leftPaneText(y), announceNodeGlyph) {
			return true
		}
	}
	return false
}

// IsAnnounceTabOrFilter reports whether rowText looks like the Announce
// Stream tab bar ("[ Nodes (N) ] [ Peers ] ...") or the filter/search bar —
// the pile widgets ABOVE the node list. enterAnnounceList sends Down past
// these to reach the list. Detection is by plain text (no color needed) and
// works for both the Go port and the Python original.
func IsAnnounceTabOrFilter(rowText string) bool {
	return strings.Contains(rowText, "Nodes ]") ||
		strings.Contains(rowText, "Peers ]") ||
		strings.Contains(rowText, "Propagation") ||
		strings.Contains(rowText, "Search") ||
		strings.Contains(rowText, "Filter")
}

// AnnounceListRows returns the content rows of the Network left pane (the
// Announce Stream / Saved Nodes list, x < networkLeftWidth), marking the
// #aaaaaa-bg row as Selected. Empty/blank rows are excluded. The tab bar and
// filter bar rows above the list are not #aaaaaa, so only real list cursor rows
// are marked Selected.
func (v *View) AnnounceListRows() []ListRow {
	if v.Screen == nil {
		return nil
	}
	var out []ListRow
	for y := 1; y < v.Screen.H; y++ {
		row := v.Screen.Rows[y]
		if len(row) == 0 {
			continue
		}
		var b strings.Builder
		selected := false
		for x := 0; x < networkLeftWidth && x < len(row); x++ {
			c := row[x]
			if c.BG == listFocusBG {
				selected = true
			}
			ch := c.Ch
			if ch == 0 {
				ch = ' '
			}
			b.WriteRune(ch)
		}
		t := strings.TrimSpace(b.String())
		if t == "" {
			continue
		}
		// Skip the bordered-title row and the tab/filter bar rows: these are
		// above the list and never carry the list_focus background. Keep them
		// out of the "node row" count by only treating #aaaaaa rows as nodes.
		out = append(out, ListRow{Y: y, Text: t, Selected: selected})
	}
	return out
}

// SelectedAnnounceRow returns the currently-selected Announce Stream node.
//
// The Python original signals selection SOLELY via the hardware cursor (the
// selected node row has no background fill, just fg #afafaf), so the PRIMARY
// signal is the row the cursor is on (when it is an Ⓝ node row). The Go port
// additionally fills the selected row with the #aaaaaa list_focus background
// whether the list has focus or not, so we fall back to that row when the
// cursor is not on a node row (e.g. focus is in the browser but the list still
// shows the #aaaaaa selection). Returns nil if neither signal is present.
//
// NOTE: the Python announce stream re-sorts as new announces arrive, which
// makes the cursor reading flicker (the ilb cursor is sometimes hidden at
// (0,0) even while the list has focus). Callers that must identify a node
// robustly should key on the Announce Info address hash (content-based), not
// on this row — see Phase 3.
func (v *View) SelectedAnnounceRow() *ListRow {
	if v.cursorOnAnnounceNodeRow() {
		return &ListRow{Y: v.CursorY, Text: v.leftPaneText(v.CursorY), Selected: true}
	}
	for _, r := range v.AnnounceListRows() {
		if r.Selected {
			rr := r
			return &rr
		}
	}
	return nil
}

// announceNodeName extracts the stable display name from an announce-stream row
// so a node can be recognized across re-announces. The row text (captured inside
// the bordered list box) is "│{ts} {glyph} {displayStr}   │" — the timestamp
// changes on every re-announce, but displayStr (the node name, or the hex hash in
// show-destination mode) is stable. We strip the wrapping '│' borders, then take
// the text after the second space (the "{ts} {glyph} " prefix has no internal
// spaces), which is displayStr. Returns "" for a non-conforming row.
func announceNodeName(rowText string) string {
	s := strings.TrimSpace(rowText)
	// Strip the box-drawing border chars that wrap every list content row.
	s = strings.Trim(s, "│┃║")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Format (announce-entry.go): "{ts} {glyph} {displayStr}".
	parts := strings.SplitN(s, " ", 3)
	if len(parts) < 3 {
		return ""
	}
	return strings.TrimSpace(parts[2])
}

// AnnounceInfoAddr returns the node address hash shown in an open Announce Info
// (the "Addr  : <hash>" row), with ok=false if no such row is present.
//
// The row is rendered inside a bordered box, so its text actually begins with a
// '│' border char: "│Addr  : <hash>   │". TrimSpace does not remove '│', so we
// must use Contains (not HasPrefix) and extract the first "<...>" that follows the
// "Addr" marker. The Announce Info box sits above the Local Peer Info box, whose
// "LXMF Addr : <hash>" row also contains "Addr" — but the Announce Info row comes
// first, so the first match is the node hash we want.
func (v *View) AnnounceInfoAddr() (string, bool) {
	if v.Screen == nil {
		return "", false
	}
	for y := 0; y < v.Screen.H; y++ {
		t := v.Screen.rowText(y)
		ai := strings.Index(t, "Addr")
		if ai < 0 {
			continue
		}
		// Extract the first "<...>" that appears after the "Addr" marker.
		rest := t[ai:]
		lo := strings.IndexByte(rest, '<')
		hi := strings.IndexByte(rest, '>')
		if lo >= 0 && hi > lo {
			return rest[lo+1 : hi], true
		}
	}
	return "", false
}

// FocusedActionButton returns the label of the focused "< Label >" button on
// the AnnounceInfo / KnownNodeInfo button row, detected via the cursor position
// (buttons are color-neutral). ok=false if the cursor is not on a button row.
//
// Buttons are padded to a fixed inner width, so the on-screen text is actually
// "< Back    >", "< Connect >", "< Msg Op  >", "< Save    >", etc. — NOT the
// unpadded "< Back >" a naive search would look for. We therefore scan the row
// generically for every "< ... >" span, trim+collapse the inner label, and
// return the one whose column range contains CursorX.
func (v *View) FocusedActionButton() (string, bool) {
	if v.Screen == nil || !v.CursorOK {
		return "", false
	}
	if v.CursorY < 0 || v.CursorY >= v.Screen.H {
		return "", false
	}
	row := v.Screen.rowText(v.CursorY)
	for pos := 0; ; {
		start := strings.Index(row[pos:], "<")
		if start < 0 {
			return "", false
		}
		start += pos
		end := strings.IndexByte(row[start+1:], '>')
		if end < 0 {
			return "", false
		}
		end += start + 1 // index just past '<'
		// end is the index of '>' itself; the span is [start, end+1).
		closeIdx := end + 1
		if v.CursorX >= start && v.CursorX < closeIdx {
			label := strings.TrimSpace(row[start+1 : end])
			// Collapse internal runs of whitespace (defensive).
			label = strings.Join(strings.Fields(label), " ")
			if label != "" {
				return label, true
			}
			return "", false
		}
		pos = closeIdx
	}
}

// browserState enumerates the browser pane states the test distinguishes.
const (
	bsDisconnected = "disconnected"
	bsRetrieving   = "retrieving"
	bsRendered     = "rendered"
	// bsError is "error:<message>"
)

// BrowserState returns the browser pane's state and the URL bar value. The
// state is one of bsDisconnected, bsRetrieving, "error:<msg>", or bsRendered,
// detected from plain-text strings (no color needed). url is the browser's
// current URL — the node hash (path stripped) — extracted from the Go port's
// "URL: <hash>" bar OR the Python original's border title "<hash>" / URL line
// "Ⓝ <hash>:/page/...".
func (v *View) BrowserState() (state, url string) {
	if v.Screen == nil {
		return "", ""
	}
	full := v.Screen.fullText()
	url = v.browserURL()

	switch {
	case strings.Contains(full, "Disconnected"):
		return bsDisconnected, url
	case strings.Contains(full, "Retrieving"),
		strings.Contains(full, "Request sent, awaiting response"),
		strings.Contains(full, "Awaiting response"),
		strings.Contains(full, "Establishing link"):
		return bsRetrieving, url
	case strings.Contains(full, "No path to destination known"),
		strings.Contains(full, "Link establishment timed out"),
		strings.Contains(full, "Request failed"),
		strings.Contains(full, "Request timed out"),
		strings.Contains(full, "Invalid URL:"),
		strings.Contains(full, "Could not load partial"):
		// Capture the error line for logging.
		return "error:" + firstErrorLine(full), url
	default:
		if url != "" {
			return bsRendered, url
		}
		return "", ""
	}
}

// browserURL extracts the browser pane's current URL — the node hash with any
// path (":/page/...") stripped, so callers compare against the Announce Info
// target hash. The Go port renders a "URL: <hash>" bar; the Python original
// renders the URL as the browser border title "<hash>" and a
// "Ⓝ <hash>:/page/..." line. Both live in the right pane (x >=
// networkLeftWidth), so we scan only the right pane to avoid matching the
// Announce Info / Local Peer Info "<hash>" rows in the left pane.
func (v *View) browserURL() string {
	if v.Screen == nil {
		return ""
	}
	// Go: a row containing "URL:" -> first token after it. "URL:" is
	// browser-specific (the left-pane Announce Info / Local Peer Info use
	// "Addr :"/"LXMF Addr :", never "URL:"), so a full-row scan is safe and
	// keeps working for test fixtures that place the bar at column 0.
	for y := 0; y < v.Screen.H; y++ {
		t := v.Screen.rowText(y)
		if idx := strings.Index(t, "URL:"); idx >= 0 {
			if fs := strings.Fields(t[idx+len("URL:"):]); len(fs) > 0 {
				return stripURLPath(fs[0])
			}
		}
	}
	// Python: the browser border title "<hash>" — a RIGHT-PANE border row
	// (contains '─', U+2500) with a "<...>" span. Scoped to the right pane
	// because the left-pane Announce Info / Local Peer Info also render
	// "<hash>" rows. The first such row is the outer browser frame; its inner
	// text is the node hash.
	for y := 0; y < v.Screen.H; y++ {
		t := v.rightPaneText(y)
		if t == "" || !strings.Contains(t, "─") {
			continue
		}
		lo := strings.Index(t, "<")
		if lo < 0 {
			continue
		}
		hi := strings.IndexByte(t[lo+1:], '>')
		if hi < 0 {
			continue
		}
		inner := strings.TrimSpace(t[lo+1 : lo+1+hi])
		if inner == "" {
			continue
		}
		return stripURLPath(strings.Fields(inner)[0])
	}
	// Python fallback: the "Ⓝ <hash>:/page/..." URL line in the RIGHT pane
	// (the left-pane announce rows also contain Ⓝ, so this must be scoped).
	for y := 0; y < v.Screen.H; y++ {
		t := v.rightPaneText(y)
		if t == "" {
			continue
		}
		if strings.ContainsRune(t, announceNodeGlyph) {
			if fs := strings.Fields(t); len(fs) > 0 {
				return stripURLPath(fs[len(fs)-1])
			}
		}
	}
	return ""
}

// rightPaneText returns the trimmed text of the right (browser) pane of row y
// — the cells with x >= networkLeftWidth. The browser pane is everything right
// of the fixed 52-col left list, so this isolates browser content/labels from
// the announce list / side panels on the left.
func (v *View) rightPaneText(y int) string {
	if v.Screen == nil || y < 0 || y >= len(v.Screen.Rows) {
		return ""
	}
	row := v.Screen.Rows[y]
	var b strings.Builder
	for x := networkLeftWidth; x < len(row); x++ {
		ch := row[x].Ch
		if ch == 0 {
			ch = ' '
		}
		b.WriteRune(ch)
	}
	return strings.TrimRight(b.String(), " ")
}

// browserFooterLink returns the link target the browser footer currently shows
// ("Link to <target>"), or "" when the cursor is not resting on a link.
//
// The Python browser sets its footer via LinkableText.peek_link (called from
// render() while the focused LinkableText is within key_timeout of the last
// keypress) -> marked_link -> set_alarm_in(0.1s, marked_link_job), which renders
// "Link to <target>" in the BrowserFrame footer — the LAST content row INSIDE
// the browser LineBox, i.e. the right-pane row immediately ABOVE the LineBox
// bottom border ('└'). So during a Down-walk (a keypress every ~200ms <
// key_timeout) the footer reliably reports the link under the cursor's position
// 0, letting examineMainPage follow ONLY link-bearing lines.
//
// The scan is restricted to that single footer row (located by finding the
// right-pane bottom border and checking the row above it). Scanning the WHOLE
// right pane produced false positives from page BODY text that happens to contain
// "Link to ", which was NOT harmless: examineMainPage pressed Enter (a no-op on a
// non-link line) and then C-d (Back), navigating AWAY from the main page and
// leaving the browser mid-retrieval — the cause of the Phase-3 "Announce Info
// opened" cascade failures (the next Down+Enter landed in the browser).
func (v *View) browserFooterLink() string {
	if v.Screen == nil {
		return ""
	}
	// Locate the browser LineBox bottom border: the right-pane row whose text
	// starts with '└'. Scan from the bottom so the FIRST match is the browser
	// border (not an inner box). The footer is the row immediately above it.
	for y := v.Screen.H - 1; y > 0; y-- {
		t := v.rightPaneText(y)
		if strings.HasPrefix(t, "└") {
			ft := v.rightPaneText(y - 1)
			if i := strings.Index(ft, "Link to "); i >= 0 {
				return strings.TrimSpace(ft[i+len("Link to "):])
			}
			return ""
		}
	}
	// Fallback: no bottom border found (e.g. an unusual layout) — scan the bottom
	// few right-pane rows so a real footer is still detected.
	for y := v.Screen.H - 4; y < v.Screen.H; y++ {
		if y < 0 {
			continue
		}
		t := v.rightPaneText(y)
		if i := strings.Index(t, "Link to "); i >= 0 {
			return strings.TrimSpace(t[i+len("Link to "):])
		}
	}
	return ""
}

// stripURLPath reduces a browser URL token to its bare node hash by dropping
// any ":/path" suffix. RNS destination hashes are hex with no colon, so a
// colon marks the start of a path ("c684...:/page/index.mu" -> "c684...").
// Tokens with no colon (a bare hash, or a Go "URL:" value) pass through.
func stripURLPath(token string) string {
	if c := strings.IndexByte(token, ':'); c >= 0 {
		return token[:c]
	}
	return token
}

func firstErrorLine(full string) string {
	errs := []string{
		"No path to destination known",
		"Link establishment timed out",
		"Request failed",
		"Request timed out",
	}
	for _, e := range errs {
		if strings.Contains(full, e) {
			return e
		}
	}
	if i := strings.Index(full, "Invalid URL:"); i >= 0 {
		rest := full[i:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			return strings.TrimSpace(rest[:nl])
		}
		return strings.TrimSpace(rest)
	}
	if i := strings.Index(full, "Could not load partial"); i >= 0 {
		rest := full[i:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			return strings.TrimSpace(rest[:nl])
		}
		return strings.TrimSpace(rest)
	}
	return "unknown"
}

// GuideTopicRendered returns the first visible content line of the Guide
// reader pane — the topic's `>Title` heading when the reader is at the top
// (scroll offset 0). Used to verify a topic actually rendered and to detect
// the scroll-reset bug: if the title is NOT at the top after selecting a new
// topic, the scroll offset leaked from the previous topic (the Go port's
// known bug; the Python reference makes a fresh Scrollable per topic so it
// always resets).
//
// The reader is a LineBox(ScrollBar(Scrollable(...))) — a bordered box whose
// top border `┌──...──┐` sits on a fixed row (it does NOT scroll; only the
// content inside does). So: find the reader's top border row and its left
// edge (the SECOND `┌` on that row — the first `┌` is the Topics LineBox),
// then return the first non-blank content row below it, stripped of the
// leading `│` and trailing scrollbar/border. When the reader is scrolled to
// the bottom, this returns mid-content instead of the title — exactly the
// scroll-reset-bug signal.
func (v *View) GuideTopicRendered() string {
	if v.Screen == nil || len(v.Screen.Rows) == 0 {
		return ""
	}
	rows := v.Screen.Rows
	// Find the row holding BOTH the Topics and reader top borders, and the
	// column of the reader's `┌` (the second box-corner on that row).
	borderY, readerLeft := -1, -1
	for y := 1; y < v.Screen.H && y < len(rows); y++ {
		r := rows[y]
		first := -1
		for x := 0; x < len(r); x++ {
			ch := r[x].Ch
			if ch == '┌' || ch == '├' || ch == '└' {
				if first < 0 {
					first = x
				} else {
					borderY, readerLeft = y, x
					break
				}
			}
		}
		if borderY >= 0 {
			break
		}
	}
	if borderY < 0 || readerLeft < 0 {
		// Fallback: reader begins near W/3, border on row 1.
		borderY = 1
		readerLeft = v.Screen.W / 3
		if readerLeft < 1 {
			readerLeft = 1
		}
	}
	// Scan the rows below the top border for the first content row. A content
	// row begins with │ or ┃ at the reader's left edge; inner border rows
	// (├┬└┴─) and blank rows are skipped.
	for y := borderY + 1; y < v.Screen.H && y < len(rows); y++ {
		r := rows[y]
		if readerLeft >= len(r) {
			continue
		}
		edge := r[readerLeft].Ch
		if edge == 0 {
			edge = ' '
		}
		if edge != '│' && edge != '┃' {
			continue // border / separator / blank
		}
		// Extract content from readerLeft+1 up to the trailing scrollbar (┃)
		// or the LineBox right border (│).
		var b strings.Builder
		started := false
		for x := readerLeft + 1; x < len(r); x++ {
			ch := r[x].Ch
			if ch == 0 {
				ch = ' '
			}
			if ch == '┃' || ch == '│' {
				break // scrollbar thumb or right border
			}
			if !started && ch == ' ' {
				continue
			}
			started = true
			b.WriteRune(ch)
		}
		if t := strings.TrimSpace(b.String()); t != "" {
			return t
		}
	}
	return ""
}

// readerLeftCol returns the column of the Guide reader pane's left edge — the
// SECOND box-corner ('┌'/'├'/'└') on the row holding both the Topics LineBox
// and the reader LineBox top borders (the first corner is the Topics box). It
// is the same anchor GuideTopicRendered uses. Returns -1 if not found.
func (v *View) readerLeftCol() int {
	if v.Screen == nil || len(v.Screen.Rows) == 0 {
		return -1
	}
	rows := v.Screen.Rows
	for y := 1; y < v.Screen.H && y < len(rows); y++ {
		r := rows[y]
		first := -1
		for x := 0; x < len(r); x++ {
			ch := r[x].Ch
			if ch == '┌' || ch == '├' || ch == '└' {
				if first < 0 {
					first = x
				} else {
					return x
				}
			}
		}
	}
	return -1
}

// readerPaneSig returns a signature of the Guide reader pane's visible content
// (every cell at and right of the reader's left edge, for all rows). It changes
// whenever the reader scrolls, so walkToBottom can detect each new screenful and
// (paired with the cursor y) the bottom. The fixed top border row is included
// but does not change, so it does not affect the signal.
func (v *View) readerPaneSig() string {
	if v.Screen == nil {
		return ""
	}
	rl := v.readerLeftCol()
	if rl < 0 {
		rl = v.Screen.W / 3
		if rl < 1 {
			rl = 1
		}
	}
	var b strings.Builder
	for y := 0; y < v.Screen.H && y < len(v.Screen.Rows); y++ {
		row := v.Screen.Rows[y]
		for x := rl; x < len(row); x++ {
			ch := row[x].Ch
			if ch == 0 {
				ch = ' '
			}
			b.WriteRune(ch)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// browserPaneSig returns a signature of the browser right pane's visible content
// (every row's right-pane text, x >= networkLeftWidth). It changes when the
// browser scrolls, so walkToBottom can detect each new screenful and the bottom.
func (v *View) browserPaneSig() string {
	if v.Screen == nil {
		return ""
	}
	var b strings.Builder
	for y := 0; y < v.Screen.H; y++ {
		b.WriteString(v.rightPaneText(y))
		b.WriteByte('\n')
	}
	return b.String()
}

// GuideSelectedTopic returns the index (0..11) of the selected topic in the
// "Topics" list. The PRIMARY signal is the hardware cursor: like the announce
// stream, the Python original signals topic selection via the cursor with no
// background fill, so the topic the cursor is on (in the left third, below the
// "Topics" border title) is the selected one. Falls back to the #aaaaaa-bg row
// (the Go port). ok=false if the Topics list is not on screen.
func (v *View) GuideSelectedTopic() (int, bool) {
	if v.Screen == nil {
		return -1, false
	}
	// Find the "Topics" border title row; the list lives in the rows just
	// below it, in the left ~1/3 of the screen.
	titleY := -1
	for y := 0; y < v.Screen.H; y++ {
		if hasBorderTitle(v.Screen.rowText(y), "Topics") {
			titleY = y
			break
		}
	}
	if titleY < 0 {
		return -1, false
	}
	rightEdge := v.Screen.W / 3
	if rightEdge < 2 {
		rightEdge = 2
	}
	// Cursor-primary: if the cursor is on a topic row (left third, below the
	// Topics title), count it as the Nth non-blank topic row.
	if v.CursorOK && v.CursorY > titleY && v.CursorY < v.Screen.H {
		idx := -1
		for y := titleY + 1; y <= v.CursorY && y < v.Screen.H; y++ {
			row := v.Screen.Rows[y]
			if len(row) == 0 {
				continue
			}
			blank := true
			for x := 0; x < rightEdge && x < len(row); x++ {
				ch := row[x].Ch
				if ch != 0 && ch != ' ' {
					blank = false
					break
				}
			}
			if !blank {
				idx++
			}
		}
		if idx >= 0 {
			return idx, true
		}
	}
	// Fallback: the #aaaaaa-bg row below the title (Go port).
	idx := -1
	for y := titleY + 1; y < v.Screen.H; y++ {
		row := v.Screen.Rows[y]
		if len(row) == 0 {
			continue
		}
		// Is this a selected (highlighted) row in the topics column?
		selected := false
		for x := 0; x < rightEdge && x < len(row); x++ {
			if row[x].BG == listFocusBG {
				selected = true
				break
			}
		}
		if selected {
			return idx + 1, true
		}
		// Count non-blank rows in the topics column as list entries.
		blank := true
		for x := 0; x < rightEdge && x < len(row); x++ {
			ch := row[x].Ch
			if ch != 0 && ch != ' ' {
				blank = false
				break
			}
		}
		if !blank {
			idx++
		}
	}
	return -1, false
}
