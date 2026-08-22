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
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

// vterm is a minimal VT100/xterm ANSI terminal emulator that maintains a cell
// grid (rows x cols of rune + tcell.Style) by parsing the byte stream written
// by a child process running on a PTY (e.g. nano/vim). It implements the
// subset of CSI/SGR/cursor/erase/scroll/alt-screen sequences that line editors
// use to paint their UI, enough to render an embedded editor inside a tview
// widget. It is NOT a complete xterm emulator; unhandled sequences are
// ignored (best-effort), which degrades gracefully. The grid is the source of
// truth for EmbeddedTerminal.Draw; a sync.Mutex on the owning widget guards
// all access (the PTY reader goroutine writes bytes; the UI-loop Draw reads).

// vtermCell is one screen cell: the rune plus its tcell style (fg/bg/attrs).
type vtermCell struct {
	char  rune
	style tcell.Style
}

// vtermScreen is the emulated terminal screen.
type vtermScreen struct {
	cols int
	rows int
	grid [][]vtermCell // grid[y][x]

	curX, curY int
	style      tcell.Style // current pen (SGR) state
	cursorVis  bool

	scrollTop, scrollBottom int
	wrapPending             bool // deferred autowrap: cursor at last col, wrap on next printable

	// alt-screen save/restore (CSI ?1049h/l): the main grid+cursor saved on
	// entering the alternate screen and restored on leaving it.
	altScreen bool
	savedGrid [][]vtermCell
	savedCurX int
	savedCurY int

	// mouse reporting modes the child enables (CSI ?1000h/?1002h/?1003h for
	// reporting, ?1006h for SGR-1006 encoding). The widget reads these to decide
	// whether to forward tcell mouse events to the PTY.
	mouseReport bool
	mouseSGR    bool

	state  vtermState
	csiBuf []byte
	oscBuf []byte
	priv   byte
}

type vtermState int

const (
	vtermGround vtermState = iota
	vtermESC
	vtermCSI
	vtermOSC
)

func newVtermScreen(cols, rows int) *vtermScreen {
	s := &vtermScreen{cols: cols, rows: rows, cursorVis: true}
	s.style = tcell.StyleDefault
	s.resetGrid()
	s.scrollTop, s.scrollBottom = 0, rows-1
	return s
}

func (s *vtermScreen) resetGrid() {
	s.grid = make([][]vtermCell, s.rows)
	for y := range s.grid {
		s.grid[y] = make([]vtermCell, s.cols)
		for x := range s.grid[y] {
			s.grid[y][x] = vtermCell{char: ' ', style: tcell.StyleDefault}
		}
	}
}

func (s *vtermScreen) Resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	newGrid := make([][]vtermCell, rows)
	for y := range newGrid {
		newGrid[y] = make([]vtermCell, cols)
		for x := range newGrid[y] {
			newGrid[y][x] = vtermCell{char: ' ', style: tcell.StyleDefault}
		}
	}
	for y := 0; y < min(s.rows, rows); y++ {
		for x := 0; x < min(s.cols, cols); x++ {
			newGrid[y][x] = s.grid[y][x]
		}
	}
	s.grid = newGrid
	s.cols, s.rows = cols, rows
	s.scrollTop, s.scrollBottom = 0, rows-1
	s.clampCursor()
	// If the alternate screen is active, also resize the saved main grid so a
	// later restore matches the new dimensions.
	if s.savedGrid != nil {
		s.savedGrid = resizeGrid(s.savedGrid, cols, rows)
	}
}

func (s *vtermScreen) Cells() [][]vtermCell { return s.grid }

func (s *vtermScreen) Cursor() (x, y int, visible bool) {
	return s.curX, s.curY, s.cursorVis
}

func (s *vtermScreen) Write(data []byte) {
	for i := range len(data) {
		b := data[i]
		if s.state == vtermGround && b < 0x80 {
			s.feedByte(b)
			continue
		}
		if s.state == vtermGround && b >= 0x80 {
			s.utf8Write(b)
			continue
		}
		s.feedByte(b)
	}
}

var utf8Accum []byte

func (s *vtermScreen) utf8Write(b byte) {
	utf8Accum = append(utf8Accum, b)
	if utf8.FullRune(utf8Accum) {
		r, _ := utf8.DecodeRune(utf8Accum)
		utf8Accum = utf8Accum[:0]
		s.putRune(r)
	}
}

func (s *vtermScreen) feedByte(b byte) {
	switch s.state {
	case vtermGround:
		s.groundByte(b)
	case vtermESC:
		s.escByte(b)
	case vtermCSI:
		s.csiByte(b)
	case vtermOSC:
		s.oscByte(b)
	}
}

func (s *vtermScreen) groundByte(b byte) {
	switch b {
	case 0x1b:
		s.state = vtermESC
	case 0x0d:
		s.curX = 0
		s.wrapPending = false
	case 0x0a, 0x0b, 0x0c:
		s.lineFeed()
		s.wrapPending = false
	case 0x08:
		if s.curX > 0 {
			s.curX--
		}
		s.wrapPending = false
	case 0x09:
		s.tab()
		s.wrapPending = false
	case 0x07:
	default:
		if b < 0x20 {
			return
		}
		s.putRune(rune(b))
	}
}

func (s *vtermScreen) escByte(b byte) {
	switch b {
	case '[':
		s.state = vtermCSI
		s.csiBuf = s.csiBuf[:0]
		s.priv = 0
	case ']':
		s.state = vtermOSC
		s.oscBuf = s.oscBuf[:0]
	case 'M':
		s.reverseLineFeed()
		s.state = vtermGround
	default:
		s.state = vtermGround
	}
}

func (s *vtermScreen) csiByte(b byte) {
	if b == '?' && len(s.csiBuf) == 0 {
		s.priv = '?'
		return
	}
	if (b >= '0' && b <= '9') || b == ';' || b == ':' {
		s.csiBuf = append(s.csiBuf, b)
		return
	}
	if b >= 0x40 && b <= 0x7e {
		s.dispatchCSI(b)
		s.state = vtermGround
		return
	}
	s.state = vtermGround
}

func (s *vtermScreen) oscByte(b byte) {
	if b == 0x07 || b == 0x1b {
		s.state = vtermGround
		return
	}
	s.oscBuf = append(s.oscBuf, b)
}

func (s *vtermScreen) paramInt(i int, def int) int {
	parts := splitCsiParams(s.csiBuf)
	if i >= len(parts) {
		return def
	}
	n, ok := atoiSafe(string(parts[i]))
	if !ok {
		return def
	}
	return n
}

func splitCsiParams(buf []byte) [][]byte {
	var parts [][]byte
	start := 0
	for i, b := range buf {
		if b == ';' {
			parts = append(parts, buf[start:i])
			start = i + 1
		}
	}
	parts = append(parts, buf[start:])
	return parts
}

func (s *vtermScreen) dispatchCSI(final byte) {
	s.wrapPending = false
	switch final {
	case 'A':
		s.curY -= s.paramInt(0, 1)
		s.clampCursor()
	case 'B':
		s.curY += s.paramInt(0, 1)
		s.clampCursor()
	case 'C':
		s.curX += s.paramInt(0, 1)
		s.clampCursor()
	case 'D':
		s.curX -= s.paramInt(0, 1)
		s.clampCursor()
	case 'E':
		s.curY += s.paramInt(0, 1)
		s.curX = 0
		s.clampCursor()
	case 'F':
		s.curY -= s.paramInt(0, 1)
		s.curX = 0
		s.clampCursor()
	case 'G', 0x60: // 'G' or backtick (0x60): cursor horizontal absolute
		s.curX = s.paramInt(0, 1) - 1
		s.clampCursor()
	case 'd':
		s.curY = s.paramInt(0, 1) - 1
		s.clampCursor()
	case 'H', 'f':
		s.curY = s.paramInt(0, 1) - 1
		s.curX = s.paramInt(1, 1) - 1
		s.clampCursor()
	case 'J':
		s.eraseDisplay(s.paramInt(0, 0))
	case 'K':
		s.eraseLine(s.paramInt(0, 0))
	case 'S':
		s.scrollUp(s.paramInt(0, 1))
	case 'T':
		s.scrollDown(s.paramInt(0, 1))
	case 'm':
		s.applySGR()
	case 'r':
		top := s.paramInt(0, 1) - 1
		bot := s.paramInt(1, s.rows) - 1
		if top < 0 {
			top = 0
		}
		if bot >= s.rows {
			bot = s.rows - 1
		}
		if top < bot {
			s.scrollTop, s.scrollBottom = top, bot
		} else {
			s.scrollTop, s.scrollBottom = 0, s.rows-1
		}
		s.curX, s.curY = 0, 0
	case 'h':
		s.setMode(true)
	case 'l':
		s.setMode(false)
	case 'L':
		s.insertLines(s.paramInt(0, 1))
	case 'M':
		s.deleteLines(s.paramInt(0, 1))
	case 'P':
		s.deleteChars(s.paramInt(0, 1))
	case '@':
		s.insertChars(s.paramInt(0, 1))
	case 'X':
		s.eraseChars(s.paramInt(0, 1))
	}
}

func (s *vtermScreen) setMode(on bool) {
	if s.priv != '?' {
		return
	}
	switch s.paramInt(0, 0) {
	case 25:
		s.cursorVis = on
	case 1049:
		s.setAltScreen(on, true) // save cursor on enter, restore on leave
	case 47, 1047:
		s.setAltScreen(on, false) // legacy: no cursor save/restore
	case 1000, 1002, 1003:
		s.mouseReport = on
	case 1006:
		s.mouseSGR = on
	case 2004:
	}
}

// setAltScreen switches between the main and alternate screen. On entering
// the alternate screen the main grid + cursor are saved; on leaving they are
// restored (matching xterm ?1049h/l; ?47/?1047 skip the cursor save/restore).
func (s *vtermScreen) setAltScreen(on, saveCursor bool) {
	if on == s.altScreen {
		return
	}
	if on {
		s.savedGrid = copyGrid(s.grid)
		s.savedCurX, s.savedCurY = s.curX, s.curY
		// The alt screen has its own dimensions; clear to the current style.
		s.clearScreen()
		s.curX, s.curY = 0, 0
		s.wrapPending = false
	} else {
		if s.savedGrid != nil {
			s.grid = s.savedGrid
			s.savedGrid = nil
			if saveCursor {
				s.curX, s.curY = s.savedCurX, s.savedCurY
			}
		} else {
			s.clearScreen()
			s.curX, s.curY = 0, 0
		}
		s.wrapPending = false
	}
	s.altScreen = on
}

// MouseModes reports whether the child wants mouse events and whether it uses
// the SGR-1006 encoding. The widget forwards mouse events only when both are
// true (the modern encoding nano/vim request).
func (s *vtermScreen) MouseModes() (report, sgr bool) {
	return s.mouseReport, s.mouseSGR
}

func copyGrid(g [][]vtermCell) [][]vtermCell {
	out := make([][]vtermCell, len(g))
	for y := range g {
		out[y] = make([]vtermCell, len(g[y]))
		copy(out[y], g[y])
	}
	return out
}

// resizeGrid rebuilds g to cols x rows, preserving the upper-left overlap and
// clearing new cells to the default style (used for the saved main grid).
func resizeGrid(g [][]vtermCell, cols, rows int) [][]vtermCell {
	newGrid := make([][]vtermCell, rows)
	for y := range newGrid {
		newGrid[y] = make([]vtermCell, cols)
		for x := range newGrid[y] {
			newGrid[y][x] = vtermCell{char: ' ', style: tcell.StyleDefault}
		}
	}
	for y := 0; y < min(len(g), rows); y++ {
		for x := 0; x < min(len(g[y]), cols); x++ {
			newGrid[y][x] = g[y][x]
		}
	}
	return newGrid
}

func (s *vtermScreen) clampCursor() {
	if s.curX < 0 {
		s.curX = 0
	}
	if s.curY < 0 {
		s.curY = 0
	}
	if s.curX >= s.cols {
		s.curX = s.cols - 1
	}
	if s.curY >= s.rows {
		s.curY = s.rows - 1
	}
}

func (s *vtermScreen) putRune(r rune) {
	if s.wrapPending {
		s.curX = 0
		s.lineFeed()
		s.wrapPending = false
	}
	if s.curY < 0 || s.curY >= s.rows || s.curX < 0 || s.curX >= s.cols {
		return
	}
	s.grid[s.curY][s.curX] = vtermCell{char: r, style: s.style}
	s.curX++
	if s.curX >= s.cols {
		s.wrapPending = true
		s.curX = s.cols - 1 // cursor rests on the last col until the next char wraps
	}
}

func (s *vtermScreen) lineFeed() {
	if s.curY == s.scrollBottom {
		s.scrollUp(1)
		return
	}
	if s.curY < s.rows-1 {
		s.curY++
	}
}

func (s *vtermScreen) reverseLineFeed() {
	if s.curY == s.scrollTop {
		s.scrollDown(1)
		return
	}
	if s.curY > 0 {
		s.curY--
	}
}

func (s *vtermScreen) tab() {
	s.curX = ((s.curX / 8) + 1) * 8
	if s.curX >= s.cols {
		s.curX = s.cols - 1
	}
}

func (s *vtermScreen) clearScreen() {
	for y := range s.grid {
		for x := range s.grid[y] {
			s.grid[y][x] = vtermCell{char: ' ', style: s.style}
		}
	}
}

func (s *vtermScreen) eraseLine(mode int) {
	if s.curY < 0 || s.curY >= s.rows {
		return
	}
	row := s.grid[s.curY]
	switch mode {
	case 0:
		for x := s.curX; x < s.cols; x++ {
			row[x] = vtermCell{char: ' ', style: s.style}
		}
	case 1:
		for x := 0; x <= s.curX && x < s.cols; x++ {
			row[x] = vtermCell{char: ' ', style: s.style}
		}
	case 2:
		for x := range row {
			row[x] = vtermCell{char: ' ', style: s.style}
		}
	}
}

func (s *vtermScreen) eraseDisplay(mode int) {
	switch mode {
	case 0:
		if s.curY >= 0 && s.curY < s.rows {
			for x := s.curX; x < s.cols; x++ {
				s.grid[s.curY][x] = vtermCell{char: ' ', style: s.style}
			}
			for y := s.curY + 1; y < s.rows; y++ {
				for x := range s.grid[y] {
					s.grid[y][x] = vtermCell{char: ' ', style: s.style}
				}
			}
		}
	case 1:
		for y := 0; y < s.curY && y < s.rows; y++ {
			for x := range s.grid[y] {
				s.grid[y][x] = vtermCell{char: ' ', style: s.style}
			}
		}
		if s.curY >= 0 && s.curY < s.rows {
			for x := 0; x <= s.curX && x < s.cols; x++ {
				s.grid[s.curY][x] = vtermCell{char: ' ', style: s.style}
			}
		}
	case 2, 3:
		s.clearScreen()
	}
}

func (s *vtermScreen) eraseChars(n int) {
	if s.curY < 0 || s.curY >= s.rows {
		return
	}
	for i := range n {
		if s.curX+i >= s.cols {
			break
		}
		s.grid[s.curY][s.curX+i] = vtermCell{char: ' ', style: s.style}
	}
}

func (s *vtermScreen) scrollUp(n int) {
	for range n {
		for y := s.scrollTop; y < s.scrollBottom; y++ {
			s.grid[y] = s.grid[y+1]
		}
		s.grid[s.scrollBottom] = make([]vtermCell, s.cols)
		for x := range s.grid[s.scrollBottom] {
			s.grid[s.scrollBottom][x] = vtermCell{char: ' ', style: s.style}
		}
	}
}

func (s *vtermScreen) scrollDown(n int) {
	for range n {
		for y := s.scrollBottom; y > s.scrollTop; y-- {
			s.grid[y] = s.grid[y-1]
		}
		s.grid[s.scrollTop] = make([]vtermCell, s.cols)
		for x := range s.grid[s.scrollTop] {
			s.grid[s.scrollTop][x] = vtermCell{char: ' ', style: s.style}
		}
	}
}

func (s *vtermScreen) insertLines(n int) {
	if s.curY < s.scrollTop || s.curY > s.scrollBottom {
		return
	}
	for range n {
		for y := s.scrollBottom; y > s.curY; y-- {
			s.grid[y] = s.grid[y-1]
		}
		s.grid[s.curY] = make([]vtermCell, s.cols)
		for x := range s.grid[s.curY] {
			s.grid[s.curY][x] = vtermCell{char: ' ', style: s.style}
		}
	}
}

func (s *vtermScreen) deleteLines(n int) {
	if s.curY < s.scrollTop || s.curY > s.scrollBottom {
		return
	}
	for range n {
		for y := s.curY; y < s.scrollBottom; y++ {
			s.grid[y] = s.grid[y+1]
		}
		s.grid[s.scrollBottom] = make([]vtermCell, s.cols)
		for x := range s.grid[s.scrollBottom] {
			s.grid[s.scrollBottom][x] = vtermCell{char: ' ', style: s.style}
		}
	}
}

func (s *vtermScreen) deleteChars(n int) {
	if s.curY < 0 || s.curY >= s.rows {
		return
	}
	row := s.grid[s.curY]
	for x := s.curX; x+n < s.cols; x++ {
		row[x] = row[x+n]
	}
	for x := s.cols - n; x < s.cols; x++ {
		if x >= s.curX {
			row[x] = vtermCell{char: ' ', style: s.style}
		}
	}
}

func (s *vtermScreen) insertChars(n int) {
	if s.curY < 0 || s.curY >= s.rows {
		return
	}
	row := s.grid[s.curY]
	for x := s.cols - 1; x >= s.curX+n; x-- {
		row[x] = row[x-n]
	}
	for x := s.curX; x < s.curX+n && x < s.cols; x++ {
		row[x] = vtermCell{char: ' ', style: s.style}
	}
}

func (s *vtermScreen) applySGR() {
	parts := splitCsiParams(s.csiBuf)
	if len(parts) == 0 || (len(parts) == 1 && len(parts[0]) == 0) {
		s.style = tcell.StyleDefault
		return
	}
	fg, bg, attr := s.style.Decompose()
	bold := attr&tcell.AttrBold != 0
	underline := attr&tcell.AttrUnderline != 0
	reverse := attr&tcell.AttrReverse != 0
	blink := attr&tcell.AttrBlink != 0
	italic := attr&tcell.AttrItalic != 0
	for i := 0; i < len(parts); i++ {
		n, ok := atoiSafe(string(parts[i]))
		switch {
		case !ok:
			continue
		case n == 0:
			fg, bg = tcell.ColorDefault, tcell.ColorDefault
			bold, underline, reverse, blink, italic = false, false, false, false, false
		case n == 1:
			bold = true
		case n == 2:
			bold = false
		case n == 3:
			italic = true
		case n == 4:
			underline = true
		case n == 5:
			blink = true
		case n == 7:
			reverse = true
		case n == 22:
			bold = false
		case n == 23:
			italic = false
		case n == 24:
			underline = false
		case n == 25:
			blink = false
		case n == 27:
			reverse = false
		case n >= 30 && n <= 37:
			// tcell's ANSI colors are ColorValid+offset (ColorBlack=ColorValid+0,
			// ColorMaroon=ColorValid+1=red, ...); a bare Color(n) is invalid and
			// renders as default, so the +ColorValid flag is required.
			fg = tcell.ColorValid + tcell.Color(n-30)
		case n == 38:
			c, consumed, ok := parseSGRColor(parts, i)
			if ok {
				fg = c
			}
			i += consumed
		case n == 39:
			fg = tcell.ColorDefault
		case n >= 40 && n <= 47:
			bg = tcell.ColorValid + tcell.Color(n-40)
		case n == 48:
			c, consumed, ok := parseSGRColor(parts, i)
			if ok {
				bg = c
			}
			i += consumed
		case n == 49:
			bg = tcell.ColorDefault
		case n >= 90 && n <= 97:
			fg = tcell.ColorValid + tcell.Color(n-90+8)
		case n >= 100 && n <= 107:
			bg = tcell.ColorValid + tcell.Color(n-100+8)
		}
	}
	st := tcell.StyleDefault.Foreground(fg).Background(bg)
	if bold {
		st = st.Bold(true)
	}
	if underline {
		st = st.Underline(true)
	}
	if italic {
		st = st.Italic(true)
	}
	if reverse {
		st = st.Reverse(true)
	}
	if blink {
		st = st.Blink(true)
	}
	s.style = st
}

func parseSGRColor(parts [][]byte, i int) (tcell.Color, int, bool) {
	if i+1 >= len(parts) {
		return 0, 0, false
	}
	mode, ok := atoiSafe(string(parts[i+1]))
	if !ok {
		return 0, 0, false
	}
	switch mode {
	case 5:
		if i+2 >= len(parts) {
			return 0, 0, false
		}
		idx, ok := atoiSafe(string(parts[i+2]))
		if !ok {
			return 0, 0, false
		}
		return tcell.PaletteColor(idx), 2, true
	case 2:
		if i+4 >= len(parts) {
			return 0, 0, false
		}
		r, _ := atoiSafe(string(parts[i+2]))
		g, _ := atoiSafe(string(parts[i+3]))
		b, _ := atoiSafe(string(parts[i+4]))
		return tcell.NewRGBColor(int32(r), int32(g), int32(b)), 4, true
	}
	return 0, 0, false
}

func atoiSafe(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
