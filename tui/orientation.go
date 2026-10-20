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

import "sync"

// TerminalPixels reports the physical size of the surface the console is drawn
// on, and whether there is one to report.
//
// The size is the terminal's own statement about itself, the way a pty's
// ws_xpixel and ws_ypixel are: an appliance whose display is 1200 by 1920 pixels
// says so, and a terminal that keeps no font metrics says nothing and leaves the
// question to its cell grid.
type TerminalPixels func() (width, height int, ok bool)

var (
	// terminalPixelsMu guards the installed probe. It is read on every draw and
	// written once at start-up, so it is a mutex rather than an atomic: a probe
	// that is a closure over the caller's state has to be published to the
	// goroutine that draws, and a mutex says that out loud.
	terminalPixelsMu sync.RWMutex
	terminalPixelsFn TerminalPixels
)

// SetTerminalPixels installs the probe the client asks for its terminal's
// physical size, or restores the built-in probe when fn is nil.
//
// A host whose terminal is not a file descriptor needs this: the appliance's iOS
// client draws on a synthetic tcell.Tty its own bridge built, so there is no
// descriptor to ask and the bridge answers for it instead. A host that leaves it
// unset gets the built-in probe, which asks the client's own terminal — see
// stdinTerminalPixels.
//
// It must be called before the interface draws, and a host that is done with it
// passes nil back, which is the shape every seam in this package keeps.
func SetTerminalPixels(fn TerminalPixels) {
	terminalPixelsMu.Lock()
	terminalPixelsFn = fn
	terminalPixelsMu.Unlock()
}

// terminalPixels reports the terminal's physical size, preferring an installed
// probe over the built-in one.
func terminalPixels() (width, height int, ok bool) {
	terminalPixelsMu.RLock()
	fn := terminalPixelsFn
	terminalPixelsMu.RUnlock()
	if fn != nil {
		return fn()
	}
	return stdinTerminalPixels()
}

// terminalIsPortrait reports whether a terminal laid out as cols cells wide and
// rows cells tall is physically taller than it is wide.
//
// Cells are not square — the appliance's console font draws one about twice as
// tall as it is wide — so a terminal can hold more columns than rows and still
// be standing up. A tablet whose display is 1200 by 1920 pixels holds 100 columns
// of 12 pixels and 75 rows of 24, which is portrait by every measure except the
// one the cell counts alone can make. A terminal that reports its physical size is
// therefore believed; one that reports none falls back to its cell grid.
func terminalIsPortrait(cols, rows int) bool {
	width, height, havePixels := terminalPixels()
	return portraitGrid(cols, rows, width, height, havePixels)
}

// portraitGrid is the rule itself, as a function of the two sizes it compares,
// so that every combination can be asserted without a terminal to ask.
//
// A reported physical size that covers the cell grid is believed; anything else —
// no report, or a report too small to be this terminal's — leaves the decision to
// the grid.
func portraitGrid(cols, rows, width, height int, havePixels bool) bool {
	if havePixels && pixelsCoverGrid(width, height, cols, rows) {
		return width < height
	}
	return cols < rows
}

// pixelsCoverGrid reports whether a reported physical size could belong to a
// cols-by-rows cell grid.
//
// A terminal is at least one pixel per cell, so a size smaller than the grid is
// not a physical size at all — it is a terminal reporting something else, such
// as a multiplexer handing a pane the pixel size of the window the pane lives
// in. Such a report is refused and the cell grid decides, which is a collapse
// made for the wrong reason at worst and never a crash: the numbers are only
// ever compared with one another.
func pixelsCoverGrid(width, height, cols, rows int) bool {
	return width >= cols && height >= rows
}
