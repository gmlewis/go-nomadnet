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
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/creack/pty/v2"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EmbeddedTerminal is a tview primitive that runs a child process (e.g. an
// editor like nano/vim) on a PTY and renders its ANSI output as tview cells,
// forwarding keyboard input to the child. It is the Go analogue of Python's
// urwid.Terminal (urwid/vterm.py): the child paints a virtual screen via the
// minimal vterm emulator, and Draw copies that screen onto the tcell screen.
// When the child exits, onClose is invoked on the UI loop so the caller can
// restore the previous body/focus (matching urwid's "closed" signal).
//
// Concurrency: a reader goroutine feeds PTY output into the vterm emulator
// (under mu) and schedules coalesced redraws via App.QueueUpdateDraw. A writer
// goroutine drains input bytes to the PTY master. tview primitives are touched
// only on the UI loop (Draw, InputHandler). The App's GoSafe/onPanic handle
// reader/writer panics.
//
// This is a Phase-1 prototype: it proves render + input for an embedded
// editor. Mouse forwarding, fine-grained resize debouncing, and full xterm
// parity are deferred (see the design plan).
type EmbeddedTerminal struct {
	*tview.Box

	app     *App
	cmd     *exec.Cmd
	master  *os.File
	vt      *vtermScreen
	onClose func()

	mu        sync.Mutex // guards vt (reader writes, Draw reads)
	writeCh   chan []byte
	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once

	cols, rows int
	term       string // TERM env value advertised to the child

	redrawCh chan struct{}
}

// NewEmbeddedTerminal starts cmd on a PTY sized cols x rows and returns a
// widget rendering its output. term is the TERM string set in the child's
// environment (e.g. "xterm"); onClose runs on the UI loop when the child exits.
// The caller must add the widget to the tview app and SetFocus it.
func NewEmbeddedTerminal(app *App, cmd *exec.Cmd, cols, rows int, term string, onClose func()) (*EmbeddedTerminal, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	if term == "" {
		term = "xterm"
	}
	// Inherit the environment but override TERM so the child emits ANSI the
	// vterm emulator supports.
	env := os.Environ()
	env = append(env, "TERM="+term)
	cmd.Env = env
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, err
	}
	et := &EmbeddedTerminal{
		Box:      tview.NewBox(),
		app:      app,
		cmd:      cmd,
		master:   master,
		vt:       newVtermScreen(cols, rows),
		onClose:  onClose,
		writeCh:  make(chan []byte, 64),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		cols:     cols,
		rows:     rows,
		term:     term,
		redrawCh: make(chan struct{}, 1),
	}
	et.start()
	return et, nil
}

// start launches the reader, writer, redraw-coalescer, and child-exit
// goroutines. Each is wrapped in App.GoSafe so a panic is routed to onPanic
// instead of killing the process mid-draw.
func (et *EmbeddedTerminal) start() {
	var wg sync.WaitGroup
	wg.Add(3)
	run := func(fn func()) { et.app.GoSafe(func() { defer wg.Done(); fn() }) }

	// Reader: PTY master -> vterm emulator -> schedule redraw.
	run(func() { et.readLoop() })
	// Writer: input bytes -> PTY master.
	run(func() { et.writeLoop() })
	// Redraw coalescer: drain redrawCh at most every ~16ms and QueueUpdateDraw.
	run(func() { et.redrawLoop() })

	// Child-exit watcher: marshal onClose onto the UI loop.
	et.app.GoSafe(func() {
		_ = et.cmd.Wait()
		et.close()
		select {
		case <-et.stopCh:
		default:
		}
		// Ensure the reader/writer/redrawer stop after the child is gone.
		et.stop()
		wg.Wait()
		close(et.doneCh)
		et.app.QueueUpdateDraw(func() {
			if et.onClose != nil {
				et.onClose()
			}
		})
	})
}

// readLoop reads chunks from the PTY master and feeds them to the vterm
// emulator under mu, then signals a coalesced redraw. It exits on EOF/EIO or
// when stopCh closes.
func (et *EmbeddedTerminal) readLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-et.stopCh:
			return
		default:
		}
		n, err := et.master.Read(buf)
		if n > 0 {
			et.mu.Lock()
			et.vt.Write(buf[:n])
			et.mu.Unlock()
			et.scheduleRedraw()
		}
		if err != nil {
			return // EOF / EIO when the child exits
		}
	}
}

// writeLoop drains writeCh and writes the bytes to the PTY master.
func (et *EmbeddedTerminal) writeLoop() {
	for {
		select {
		case <-et.stopCh:
			return
		case b, ok := <-et.writeCh:
			if !ok {
				return
			}
			for len(b) > 0 {
				n, err := et.master.Write(b)
				if err != nil {
					return
				}
				b = b[n:]
			}
		}
	}
}

// redrawLoop coalesces redraw requests: many small PTY writes collapse into a
// single QueueUpdateDraw at most every ~16ms so a full-screen repaint (e.g.
// nano's startup) does not flood the event loop.
func (et *EmbeddedTerminal) redrawLoop() {
	t := time.NewTimer(0)
	if !t.Stop() {
		<-t.C
	}
	for {
		select {
		case <-et.stopCh:
			return
		case <-et.redrawCh:
			// Drain any back-to-back signals, then wait a short tick.
			for {
				select {
				case <-et.redrawCh:
					continue
				default:
				}
				break
			}
			t.Reset(16 * time.Millisecond)
			select {
			case <-et.stopCh:
				return
			case <-t.C:
			}
			et.app.QueueUpdateDraw(func() {})
		}
	}
}

func (et *EmbeddedTerminal) scheduleRedraw() {
	select {
	case et.redrawCh <- struct{}{}:
	default:
	}
}

// Draw copies the vterm grid onto the tcell screen and shows the child's
// cursor when focused. Runs on the UI loop.
func (et *EmbeddedTerminal) Draw(screen tcell.Screen) {
	et.Box.DrawForSubclass(screen, et)
	// Render into the inner rect so an optional border + title (set by the
	// integrator, e.g. "Editing RNS Config") frames the editor instead of
	// being overwritten by the cell grid.
	x, y, w, h := et.GetInnerRect()
	et.mu.Lock()
	cells := et.vt.Cells()
	cx, cy, vis := et.vt.Cursor()
	et.mu.Unlock()
	for row := range h {
		if row >= len(cells) {
			break
		}
		for col := range w {
			if col >= len(cells[row]) {
				break
			}
			c := cells[row][col]
			screen.SetContent(x+col, y+row, c.char, nil, c.style)
		}
	}
	if vis && et.HasFocus() && cx < w && cy < h {
		screen.ShowCursor(x+cx, y+cy)
	}
}

// InputHandler translates key events to ANSI bytes and enqueues them to the
// PTY writer. It mirrors urwid's keypress key->byte translation.
func (et *EmbeddedTerminal) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return et.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if b := keyToANSI(event); b != nil {
			select {
			case et.writeCh <- b:
			default:
				// drop on full buffer (best-effort for the prototype)
			}
		}
	})
}

// MouseHandler forwards mouse events to the child as SGR-1006 sequences, but
// only when the child has enabled mouse reporting (?1000h/?1002h/?1003h) and the
// SGR-1006 encoding (?1006h) — the combination nano/vim request. Otherwise the
// event is not consumed (so tview can use it, e.g. to focus the widget on
// click). Press/release, drag (move with a button held), and the mouse wheel
// are translated; col/row are widget-relative and 1-based, matching the SGR
// spec the child expects.
func (et *EmbeddedTerminal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return et.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
		rx, ry, _, _ := et.GetRect()
		x, y := event.Position()
		et.mu.Lock()
		report, sgr := et.vt.MouseModes()
		et.mu.Unlock()
		if !report || !sgr {
			return false, nil
		}
		col := x - rx + 1
		row := y - ry + 1
		if col < 1 || row < 1 {
			return false, nil
		}
		var b []byte
		switch action {
		case tview.MouseLeftDown:
			b = mouseToSGR(0, col, row, true)
		case tview.MouseMiddleDown:
			b = mouseToSGR(1, col, row, true)
		case tview.MouseRightDown:
			b = mouseToSGR(2, col, row, true)
		case tview.MouseLeftUp:
			b = mouseToSGR(0, col, row, false)
		case tview.MouseMiddleUp:
			b = mouseToSGR(1, col, row, false)
		case tview.MouseRightUp:
			b = mouseToSGR(2, col, row, false)
		case tview.MouseScrollUp:
			b = mouseToSGR(64, col, row, true)
		case tview.MouseScrollDown:
			b = mouseToSGR(65, col, row, true)
		case tview.MouseMove:
			// Drag (a button held): button code + 32 (motion bit), terminator M.
			btn := -1
			if event.Buttons()&tcell.Button1 != 0 {
				btn = 0
			} else if event.Buttons()&tcell.Button2 != 0 {
				btn = 1
			} else if event.Buttons()&tcell.Button3 != 0 {
				btn = 2
			}
			if btn < 0 {
				return false, nil
			}
			b = mouseToSGR(btn+32, col, row, true)
		default:
			return false, nil
		}
		select {
		case et.writeCh <- b:
			return true, et
		default:
			return false, nil
		}
	})
}

// mouseToSGR builds an SGR-1006 mouse event: ESC[<button;col;row M for a press
// (or motion) and ESC[<button;col;row m for a release. col/row are 1-based.
func mouseToSGR(button, col, row int, press bool) []byte {
	term := byte('M')
	if !press {
		term = 'm'
	}
	b := []byte{0x1b, '[', '<'}
	b = strconv.AppendInt(b, int64(button), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(col), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(row), 10)
	return append(b, term)
}

// PasteHandler forwards pasted text as UTF-8 bytes to the child.
func (et *EmbeddedTerminal) PasteHandler() func(text string, setFocus func(tview.Primitive)) {
	return func(text string, setFocus func(tview.Primitive)) {
		select {
		case et.writeCh <- []byte(text):
		default:
		}
	}
}

// Focus flags the widget focused (so Draw shows the cursor) and is called by
// the application when the widget receives focus.
func (et *EmbeddedTerminal) Focus(delegate func(p tview.Primitive)) {
	et.Box.Focus(delegate)
}

// SetRect resizes the vterm grid and the PTY window to the new cols/rows,
// matching urwid's touch_term + TIOCSWINSZ.
func (et *EmbeddedTerminal) SetRect(x, y, w, h int) {
	et.Box.SetRect(x, y, w, h)
	ix, iy, iw, ih := et.GetInnerRect()
	_ = ix
	_ = iy
	cols, rows := iw, ih
	if cols <= 0 || rows <= 0 {
		return
	}
	et.mu.Lock()
	et.vt.Resize(cols, rows)
	et.cols, et.rows = cols, rows
	et.mu.Unlock()
	_ = pty.Setsize(et.master, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

// Close tears down the widget: stops the goroutines, closes the PTY master, and
// (if the child is still alive) signals it to exit. Safe to call once.
func (et *EmbeddedTerminal) Close() { et.close(); et.stop() }

func (et *EmbeddedTerminal) close() {
	et.closeOnce.Do(func() {
		if et.master != nil {
			_ = et.master.Close()
		}
	})
}

func (et *EmbeddedTerminal) stop() {
	select {
	case <-et.stopCh:
		return
	default:
	}
	close(et.stopCh)
}

// keyToANSI translates a tcell key event to the ANSI byte sequence the child
// editor expects (mirrors urwid's KEY_TRANSLATIONS and ctrl handling).
func keyToANSI(event *tcell.EventKey) []byte {
	k := event.Key()
	switch k {
	case tcell.KeyEnter:
		return []byte{0x0d}
	case tcell.KeyTab:
		return []byte{0x09}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return []byte{0x7f}
	case tcell.KeyEsc:
		return []byte{0x1b}
	case tcell.KeyUp:
		return []byte{0x1b, '[', 'A'}
	case tcell.KeyDown:
		return []byte{0x1b, '[', 'B'}
	case tcell.KeyRight:
		return []byte{0x1b, '[', 'C'}
	case tcell.KeyLeft:
		return []byte{0x1b, '[', 'D'}
	case tcell.KeyHome:
		return []byte{0x1b, '[', 'H'}
	case tcell.KeyEnd:
		return []byte{0x1b, '[', 'F'}
	case tcell.KeyPgUp:
		return []byte{0x1b, '[', '5', '~'}
	case tcell.KeyPgDn:
		return []byte{0x1b, '[', '6', '~'}
	case tcell.KeyDelete:
		return []byte{0x1b, '[', '3', '~'}
	case tcell.KeyInsert:
		return []byte{0x1b, '[', '2', '~'}
	}
	if k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		// Ctrl-X -> byte 1..26 (Ctrl-A=1, Ctrl-B=2, ... Ctrl-Z=26).
		return []byte{byte(int(k) - int(tcell.KeyCtrlA) + 1)}
	}
	if k == tcell.KeyRune {
		r := event.Rune()
		if r < 0x80 {
			return []byte{byte(r)}
		}
		// UTF-8 encode the rune.
		var buf [4]byte
		n := encodeUTF8(r, buf[:])
		return buf[:n]
	}
	return nil
}

func encodeUTF8(r rune, buf []byte) int {
	switch {
	case r < 0x80:
		buf[0] = byte(r)
		return 1
	case r < 0x800:
		buf[0] = 0xc0 | byte(r>>6)
		buf[1] = 0x80 | byte(r&0x3f)
		return 2
	case r < 0x10000:
		buf[0] = 0xe0 | byte(r>>12)
		buf[1] = 0x80 | byte((r>>6)&0x3f)
		buf[2] = 0x80 | byte(r&0x3f)
		return 3
	default:
		buf[0] = 0xf0 | byte(r>>18)
		buf[1] = 0x80 | byte((r>>12)&0x3f)
		buf[2] = 0x80 | byte((r>>6)&0x3f)
		buf[3] = 0x80 | byte(r&0x3f)
		return 4
	}
}
