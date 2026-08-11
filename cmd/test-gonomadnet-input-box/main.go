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

// Command test-gonomadnet-input-box fires up a gonomadnet TUI inside a tmux
// session (exactly like run-tmux-test-suite), connects to a remote node that
// hosts a search form, drives the form's text input box and "Search" button via
// remote-control keystrokes, waits for the response, prints the rendered search
// results to stdout, and exits.
//
// Its sole purpose is to be the autonomous regression harness for the gonomadnet
// browser input-box, which is currently completely broken (micron <fields are
// rendered as static text instead of focusable ReadlineEdit widgets, so typing
// into them does nothing). The harness drives the canonical interaction
// sequence and reports PASS/FAIL at each checkpoint with full screen snapshots,
// so once the input box is fixed the harness passes end-to-end — and until then
// it pinpoints exactly which step is broken.
//
// Because the input box works in the Python source-of-truth, pass --nomadnet to
// drive /opt/homebrew/bin/nomadnet instead of the Go port: the harness then runs
// the identical sequence against the reference implementation, proving the
// sequence itself is correct and giving a working baseline to diff the Go port
// against.
//
// Usage:
//
//	go run ./cmd/test-gonomadnet-input-box
//	go run ./cmd/test-gonomadnet-input-box --nomadnet
//	go run ./cmd/test-gonomadnet-input-box --dest 1bf29468f7d10cfed65c7d0fd9717634 --text 'Go port of NomadNet'
//	go run ./cmd/test-gonomadnet-input-box --nomadnet --text 'hello world'
//
// The rendered search-result page is written to stdout; all progress, asserts,
// and snapshots go to stderr and the log file.
//
// Flags:
//
//	--nomadnet          drive the Python nomadnet (/opt/homebrew/bin/nomadnet) instead of the Go port
//	--dest HASH         16-byte (32 hex char) destination hash to connect to (default: ICP Board)
//	--text TEXT         text to type into the search input box (default: "Go port of NomadNet")
//	--search-label LAB  label of the submit link/button to press (default: "Search")
//	--config DIR        NomadNet config dir (default: ~/.nomadnetwork)
//	--rnsconfig DIR     Reticulum config dir passed as -rnsconfig (default: ~/.reticulum)
//	--announce-wait D   how long to wait for the local node to announce (network ready) before connecting (default 25s)
//	--connect-wait D    how long to wait for a node page to render (default 15s)
//	--response-wait D   how long to wait for the search response to render (default 20s)
//	--step-delay D      delay between keystrokes (default 250ms)
//	--max-field-downs N max Down strokes when locating the input field (default 40)
//	--max-button-downs N max Down strokes when locating the Search button (default 40)
//	--size WxH          pane size (default 135x32)
//	--headed            attach the tmux session to this terminal so the run is watchable
//	--log FILE          log file path (default /tmp/<target>-input-box-NNN.log)
//	-h, --help          show help and exit
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gmlewis/go-nomadnet/utils"
	"golang.org/x/term"
)

// defaultDest is the ICP Board search node — a nomadnetwork.node that hosts a
// search form (a micron text input field plus a "Search" submit link).
const defaultDest = "1bf29468f7d10cfed65c7d0fd9717634"

const defaultText = "Go port of NomadNet"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "test-gonomadnet-input-box: %v\n", err)
		os.Exit(1)
	}
}

// config holds the parsed command-line flags.
type config struct {
	nomadnet       bool
	dest           string
	text           string
	searchLabel    string
	configDir      string
	rnsconfigDir   string
	announceWait   time.Duration
	connectWait    time.Duration
	responseWait   time.Duration
	stepDelay      time.Duration
	maxFieldDowns  int
	maxButtonDowns int
	size           string
	headed         bool
	logFile        string
}

// target returns "nomadnet" for the Python original, "go" for the port.
func (c *config) target() string {
	if c.nomadnet {
		return "nomadnet"
	}
	return "go"
}

func parseFlags(args []string) (*config, error) {
	c := &config{
		dest:           defaultDest,
		text:           defaultText,
		searchLabel:    "Search",
		announceWait:   envDur("GNOMADNET_ANNOUNCE_WAIT", 25*time.Second),
		connectWait:    envDur("GNOMADNET_CONNECT_WAIT", 15*time.Second),
		responseWait:   20 * time.Second,
		stepDelay:      envDur("GNOMADNET_STEP_DELAY", 250*time.Millisecond),
		maxFieldDowns:  40,
		maxButtonDowns: 40,
		size:           "135x32",
	}
	if home, err := os.UserHomeDir(); err == nil {
		c.configDir = filepath.Join(home, ".nomadnetwork")
	}
	fs := flag.NewFlagSet("test-gonomadnet-input-box", flag.ContinueOnError)
	fs.BoolVar(&c.nomadnet, "nomadnet", false, "drive the Python `nomadnet` (/opt/homebrew/bin/nomadnet) instead of the Go port")
	fs.StringVar(&c.dest, "dest", c.dest, "destination hash (32 hex chars) of the search node to connect to")
	fs.StringVar(&c.text, "text", c.text, "text to type into the search input box")
	fs.StringVar(&c.searchLabel, "search-label", c.searchLabel, "label of the submit link/button to press")
	fs.StringVar(&c.configDir, "config", c.configDir, "NomadNet config dir")
	fs.StringVar(&c.rnsconfigDir, "rnsconfig", "", "RNS config dir passed as -rnsconfig (default ~/.reticulum)")
	fs.DurationVar(&c.announceWait, "announce-wait", c.announceWait, "how long to wait for the local node to announce (network ready) before connecting via URL")
	fs.DurationVar(&c.connectWait, "connect-wait", c.connectWait, "how long to wait for the node page to render after Connect")
	fs.DurationVar(&c.responseWait, "response-wait", c.responseWait, "how long to wait for the search response to render")
	fs.DurationVar(&c.stepDelay, "step-delay", c.stepDelay, "delay between keystrokes")
	fs.IntVar(&c.maxFieldDowns, "max-field-downs", c.maxFieldDowns, "max Down strokes when locating the input field")
	fs.IntVar(&c.maxButtonDowns, "max-button-downs", c.maxButtonDowns, "max Down strokes when locating the Search button")
	fs.StringVar(&c.size, "size", c.size, "pane size WxH")
	fs.BoolVar(&c.headed, "headed", false, "attach the tmux session to this terminal so the run is watchable")
	fs.StringVar(&c.logFile, "log", "", "log file path (default /tmp/<target>-input-box-NNN.log)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return c, nil
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return time.Duration(f * float64(time.Second))
		}
	}
	return def
}

func run(args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	if err := requireTools(cfg); err != nil {
		return err
	}
	if err := validateDest(cfg.dest); err != nil {
		return err
	}

	nnn := strconv.FormatInt(time.Now().Unix(), 10)
	target := cfg.target()
	sessionPrefix := "gib-"
	logPrefix := "gonomadnet-input-box"
	if cfg.nomadnet {
		sessionPrefix = "nib-"
		logPrefix = "nomadnet-input-box"
	}
	sessionName := sessionPrefix + nnn
	logFile := cfg.logFile
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/%s-%s.log", logPrefix, nnn)
	}
	logf, closeLog, err := openLog(logFile)
	if err != nil {
		return err
	}
	defer closeLog()

	w, h, err := parseSize(cfg)
	if err != nil {
		return err
	}

	repo, _ := os.Getwd()
	rnsFlag := ""
	rnsDir := cfg.rnsconfigDir
	if rnsDir == "" {
		if home, e := os.UserHomeDir(); e == nil {
			rnsDir = filepath.Join(home, ".reticulum")
		}
	}
	if rnsDir != "" {
		if cfg.nomadnet {
			rnsFlag = fmt.Sprintf(" --rnsconfig '%s'", rnsDir)
		} else {
			rnsFlag = fmt.Sprintf(" -rnsconfig '%s'", rnsDir)
		}
	}
	var launchCmd string
	if cfg.nomadnet {
		launchCmd = fmt.Sprintf("exec nomadnet -t --config '%s'%s", cfg.configDir, rnsFlag)
	} else {
		launchCmd = fmt.Sprintf("cd '%s' && exec go run ./cmd/gonomadnet -t -config '%s'%s", repo, cfg.configDir, rnsFlag)
	}

	both := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
		logf(format, args...)
	}

	both("gonomadnet input-box test (%s)", target)
	both("  log     : %s", logFile)
	both("  session : %s  (watch live: tmux attach -t %s)", sessionName, sessionName)
	both("  config  : %s", cfg.configDir)
	both("  rns     : %s", rnsDir)
	both("  repo    : %s", repo)
	both("  target  : %s  (launch: %s)", target, launchCmd)
	both("  dest    : %s", cfg.dest)
	both("  text    : %q", cfg.text)
	both("  size    : %dx%d", w, h)
	if out, err := exec.Command("tmux", "-V").Output(); err == nil {
		both("  tmux    : %s", strings.TrimSpace(string(out)))
	}
	both("")

	// tcell truecolor is robust regardless of $TERM.
	_ = os.Setenv("COLORTERM", "truecolor")

	sess, err := utils.NewSession(sessionName, repo, launchCmd, w, h)
	if err != nil {
		return fmt.Errorf("FATAL: %w", err)
	}
	defer func() {
		if sess.HasSession() {
			logf("cleanup: killing tmux session %s", sessionName)
			_ = sess.KillSession()
		}
	}()

	d := &driver{
		sess:       sess,
		logf:       logf,
		stepDelay:   cfg.stepDelay,
		announceWait: cfg.announceWait,
		connectWait:  cfg.connectWait,
		responseWait: cfg.responseWait,
	}

	if cfg.headed {
		done := make(chan struct{})
		go func() {
			d.runSequence(cfg)
			close(done)
		}()
		both("attaching tmux session so you can watch (detach with Ctrl-b d)...")
		both("progress is being logged to: %s", logFile)
		both("")
		attach := exec.Command("tmux", "attach", "-t", sessionName)
		attach.Stdin = os.Stdin
		attach.Stdout = os.Stdout
		attach.Stderr = os.Stderr
		if err := attach.Run(); err != nil {
			both("WARN: tmux attach exited (%v); the test is still running in the background", err)
		}
		<-done
	} else {
		both("running detached. Watch live in another terminal with:")
		both("  tmux attach -t %s", sessionName)
		both("progress is being logged to: %s", logFile)
		both("")
		d.runSequence(cfg)
	}

	// The app has now been asked to quit (runSequence's final step) and the tmux
	// session has ended — so under --headed the user's terminal has been restored
	// to its normal (cooked) state by tmux. ONLY NOW print the captured browser
	// results to stdout: printing earlier (while the TUI was still attached)
	// would interleave the results with the nomadnet display, which is what made
	// the results hard to read under --headed. stdout carries ONLY the results;
	// all progress/asserts/snapshots went to stderr and the log file.
	printResults(d)

	both("")
	both("================ DONE ================")
	both("log    : %s", logFile)
	both("summary: asserts=%d pass=%d fail=%d  field-found=%t text-accepted=%t search-submitted=%t",
		d.asserts, d.assertOK, d.failures, d.fieldFound, d.textAccepted, d.searchSubmitted)
	return nil
}

// printResults writes the captured browser output to stdout. It is called AFTER
// the app has quit and the terminal has been reset, so the results appear
// cleanly, not interleaved with the nomadnet TUI. stdout is the ONLY place the
// results go (so it can be piped/redirected); any notice about a missing
// capture goes to stderr instead.
func printResults(d *driver) {
	if d.results == "" {
		fmt.Fprintln(os.Stderr, "test-gonomadnet-input-box: no search results captured")
		d.logf("no search results captured")
		return
	}
	fmt.Print(d.results)
	if !strings.HasSuffix(d.results, "\n") {
		fmt.Println()
	}
	d.logf("printed %d bytes of search results to stdout", len(d.results))
}

// ---------- driver ----------

// driver holds the test run state and the shared keystroke/verify helpers.
// The model is the run-tmux-test-suite driver: every keystroke is followed by a
// Wait on a View condition (parsed screen + cursor), failures are logged but
// never fatal, and the run continues so one log records the full sequence.
type driver struct {
	sess         *utils.Session
	logf         func(format string, args ...any)
	stepDelay    time.Duration
	announceWait time.Duration
	connectWait  time.Duration
	responseWait time.Duration

	// checkpoints, reported in the final summary.
	asserts        int
	assertOK       int
	failures       int
	fieldFound     bool
	textAccepted   bool
	searchSubmitted bool

	// preSubmitSig is the browser right-pane signature captured AFTER the submit
	// button is focused (so its footer link indicator is already baked in) but
	// BEFORE Enter is pressed. The response-wait compares against THIS, so the
	// only subsequent pane change it can match is the actual search results — not
	// the footer merely appearing as the cursor lands on the Search link (which
	// otherwise satisfied a naive "sig changed" check in ~23ms, before any mesh
	// search could return).
	preSubmitSig string

	// results holds the captured search-result page written to stdout.
	results string
}

// send sends tmux key names then pauses stepDelay for the app to redraw.
func (d *driver) send(keys ...string) {
	d.logf("send: %s", strings.Join(keys, " "))
	if err := d.sess.SendKeys(keys...); err != nil {
		d.logf("  ERROR send-keys: %v", err)
	}
	time.Sleep(d.stepDelay)
}

// sendLiteral sends literal text (no tmux key-name interpretation).
func (d *driver) sendLiteral(text string) {
	d.logf("send-literal: %q", text)
	if err := d.sess.SendLiteral(text); err != nil {
		d.logf("  ERROR send-literal: %v", err)
	}
	time.Sleep(d.stepDelay)
}

// snapshot logs the current pane (plain text) under a label.
func (d *driver) snapshot(label string) {
	out, err := d.sess.Capture()
	if err != nil {
		d.logf("===== SNAPSHOT: %s (capture error: %v) =====", label, err)
		return
	}
	d.logf("===== SNAPSHOT: %s =====\n%s===== END SNAPSHOT =====", label, out)
}

// view captures the current View, logging on error.
func (d *driver) view() *utils.View {
	v, err := d.sess.View()
	if err != nil {
		d.logf("  ERROR capturing view: %v", err)
		return &utils.View{}
	}
	return v
}

// waitFor polls until cond(v) is true or timeout. Returns the last view + ok.
func (d *driver) waitFor(cond func(*utils.View) bool, timeout time.Duration) (*utils.View, bool) {
	return d.sess.Wait(cond, timeout)
}

// assert waits up to timeout for cond(v), then logs PASS/FAIL with the observed
// state. Never fatal. Returns whether the condition held.
func (d *driver) assert(cond func(*utils.View) bool, timeout time.Duration, format string, args ...any) bool {
	d.asserts++
	v, ok := d.waitFor(cond, timeout)
	msg := fmt.Sprintf(format, args...)
	if ok {
		d.assertOK++
		d.logf("  ASSERT PASS: %s", msg)
		return true
	}
	d.failures++
	d.logf("  ASSERT FAIL: %s | observed: %s", msg, observedState(v))
	return false
}

// observedState summarizes the view for failure logs.
func observedState(v *utils.View) string {
	if v == nil || v.Screen == nil {
		return "<no view>"
	}
	page := v.ActivePage()
	midx, mok := v.MenuFocusedButton()
	cur := "cursor=?"
	if v.CursorOK {
		cur = fmt.Sprintf("cursor=(%d,%d)", v.CursorX, v.CursorY)
	}
	btn := ""
	if b, ok := v.FocusedActionButton(); ok {
		btn = " actionBtn=" + b
	}
	st, url := v.BrowserState()
	footer := v.BrowserFooterLink()
	return fmt.Sprintf("page=%q menu=%v(%t) %s browser=%s/%q%s footer-link=%q", page, midx, mok, cur, st, url, btn, footer)
}

// step logs a section header.
func (d *driver) step(msg string) {
	d.logf("\n########## %s ##########", msg)
}

// ---------- the input-box sequence ----------

// runSequence is the end-to-end test: boot, reach the Network page, connect to
// the --dest by URL (Ctrl-u), drive the search form, and quit the app. The
// captured browser results are printed by the caller AFTER the app has exited
// and the terminal has been reset (so the results are not interleaved with the
// TUI, especially under --headed).
func (d *driver) runSequence(cfg *config) {
	d.step("Phase 1: boot the app")
	if !d.boot() {
		d.snapshot("final state (boot failed)")
		return
	}

	d.step("Phase 2: reach the Network page (browser pane)")
	if !d.reachNetworkPage() {
		d.snapshot("final state (could not reach the Network page)")
		return
	}

	d.step(fmt.Sprintf("Phase 3: connect to %s via URL dialog (Ctrl-u)", cfg.dest))
	connected := d.connectToTarget(cfg.dest)
	if !connected {
		d.snapshot("final state (connect failed)")
		return
	}

	d.step("Phase 4: drive the search form")
	d.driveSearchForm(cfg)

	d.step("Phase 5: quit the app (results are printed after the terminal resets)")
	d.quit()
}

// boot waits for the menu bar to appear (the "[ Conversations ]" button).
func (d *driver) boot() bool {
	d.logf("waiting for the app to start (compile + intro splash)...")
	if _, ok := d.waitFor(appStarted(), 150*time.Second); !ok {
		d.logf("FATAL: app did not start within 150s (no menu bar)")
		d.snapshot("app-start-timeout")
		return false
	}
	time.Sleep(d.stepDelay)
	d.snapshot("app started (main display)")
	return true
}

// appStarted is a Wait condition for the menu bar appearing.
func appStarted() func(*utils.View) bool {
	return func(v *utils.View) bool {
		if v.Screen == nil || len(v.Screen.Rows) == 0 {
			return false
		}
		return strings.Contains(v.Screen.RowText(0), "Conversations")
	}
}

// reachNetworkPage moves from the boot body to the Network page and focuses the
// browser pane (the right column), ready for the Ctrl-u URL dialog. It does NOT
// enter or scan the Announce Stream: the target node is reached directly by URL
// (Ctrl-u), which connects via the cached path table and so does NOT require the
// node to have announced recently (paths persist for days). Scanning the stream
// was both slow and fragile — a node that last announced hours ago is not in the
// recent stream but is still routable.
func (d *driver) reachNetworkPage() bool {
	// Boot focus is the body. Go to the top, then Up escapes to the menu.
	d.send("Home")
	d.send("Up")
	d.assert(menuFocused(), 3*time.Second, "menu focused after Home+Up")

	// Network is menu index 1.
	d.moveMenuTo(1, 8)
	d.assert(menuAt(1), 3*time.Second, "menu on Network (index 1)")
	d.send("Enter")
	d.assert(func(v *utils.View) bool { return v.ActivePage() == "network" }, 5*time.Second, "Network page active")
	d.snapshot("Network page selected")

	// Down drops focus from the menu to the body (the left pane).
	d.send("Down")
	d.assert(func(v *utils.View) bool {
		_, ok := v.MenuFocusedButton()
		return !ok
	}, 3*time.Second, "focus left the menu (Down to body)")

	// Wait for the local node to have announced — the "Local Peer Info" box
	// shows "Announced : Never" until the first announce, then "Announced : N
	// ... ago". An announce means the transport interfaces are up, so the Ctrl-u
	// link to the cached --dest path can establish. Non-fatal: if it never
	// announces, connectToTarget still retries.
	d.logf("waiting up to %v for the local node to announce (network ready)...", d.announceWait)
	d.assert(func(v *utils.View) bool {
		if v.Screen == nil {
			return false
		}
		full := v.Screen.FullText()
		return strings.Contains(full, "Announced") && !strings.Contains(full, "Never")
	}, d.announceWait, "local node announced (network ready)")

	// Focus the browser pane (the right column) so browser keys — Ctrl-u in
	// particular — are delivered to the BrowserFrame, not the left list.
	d.send("Right")
	d.snapshot("browser pane focused (ready for Ctrl-u)")
	return true
}

// connectToTarget opens the browser URL dialog (Ctrl-u), types the destination
// hash, and submits — connecting directly to the node via the cached path table,
// with no dependence on the Announce Stream (the node need not have announced
// recently; a path persists for days once learned). This replaces the old
// Announce-Stream scan, which only worked when the target had announced within
// the stream's recent window.
//
// Retries a few times so a slow transport-interface startup (the local node
// announcing just before this) does not cause a spurious "Link establishment
// timed out" failure: each retry re-opens the URL dialog after a disconnect.
func (d *driver) connectToTarget(dest string) bool {
	want := strings.ToLower(dest)
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if !d.sess.HasSession() {
			d.logf("session died during connect; aborting")
			return false
		}
		d.logf("connect attempt %d/%d: opening URL dialog for %s", attempt, maxAttempts, want)

		// Ensure the browser pane is focused (Right is idempotent on the right
		// column), then open the URL dialog. Ctrl-u is handled by both the
		// browser frame (bd.OnURLDialog, when the browser is focused) and the
		// network display (networkDisplay.OnURLDialog, when the empty browser
		// frame lets the key bubble up). Both now open the same Python-parity
		// dialog titled "Enter URL" with caption "URL : ", so either path works.
		d.send("Right")
		d.send("C-u")
		if !d.assert(urlDialogOpen, 5*time.Second, "URL dialog open (attempt %d)", attempt) {
			d.logf("  URL dialog did not open (browser not focused?); retrying")
			d.send("Escape")
			if attempt < maxAttempts {
				time.Sleep(2 * time.Second)
			}
			continue
		}

		// The dialog's edit field is pre-filled with the browser's current URL,
		// which is empty while disconnected (and after a C-w reset), so typing
		// the bare destination hash is the whole URL. parse_url accepts a single
		// 32-hex-char component and loads the node's default page (/page/index.mu).
		d.sendLiteral(dest)
		d.snapshot("connect: destination entered in URL dialog")
		d.send("Enter")

		// Wait for the target page to render OR a connect error.
		d.logf("  URL submitted; waiting for render or failure...")
		_, _ = d.waitFor(func(v *utils.View) bool {
			st, url := v.BrowserState()
			if strings.HasPrefix(st, "error:") {
				return true
			}
			return st == utils.BSRendered && url == want
		}, d.connectWait+6*time.Second)
		st, url := d.view().BrowserState()
		if st == utils.BSRendered && url == want {
			d.logf("  page RENDERED OK (url=%q)", url)
			d.snapshot("connect: target page rendered")
			return true
		}
		switch {
		case strings.HasPrefix(st, "error:"):
			d.logf("  connect FAILED (attempt %d): %s", attempt, st)
		default:
			d.logf("  connect TIMEOUT (attempt %d) (state=%q url=%q)", attempt, st, url)
		}
		d.snapshot("connect: failed")
		// Disconnect to reset the browser pane (and clear the dialog's pre-fill)
		// before the next attempt.
		d.send("C-w")
		if attempt < maxAttempts {
			d.logf("  pausing 5s before retry (letting interfaces finish coming up)...")
			time.Sleep(5 * time.Second)
		}
	}
	d.logf("  could not connect to %s after %d attempts", want, maxAttempts)
	return false
}

// driveSearchForm runs the canonical input-box sequence on the connected node's
// page: move focus to the browser, navigate to the search page (if the field
// is not on the landing page), walk Down to the text input field, type the
// query, walk Down to the "Search" submit link, press Enter, wait for the
// response, and capture the rendered results.
//
// ICP Board's structure (the default --dest) is two pages deep: the landing
// page /page/index.mu has an "OPEN LIVE SEARCH" link (a micron link whose URL
// targets /page/search.mu and declares a `query` field); the editable field
// itself lives on /page/search.mu. So the harness first tries the field on the
// current page, and if it is not there, follows a search link to the search
// page and retries there. For a node that hosts the field directly on its landing
// page, the first attempt succeeds and no navigation happens.
func (d *driver) driveSearchForm(cfg *config) {
	// After the Ctrl-u connect, the URL dialog has closed and focus is on the
	// browser page (the BrowserFrame / its page Pile), so the page body is
	// already driven — no list->browser Right is needed (that was for the old
	// Announce-Stream connect, which returned focus to the list).
	d.snapshot("search form: browser pane (before driving)")

	// --- Locate the input field and type the query (exactly once). ---
	//
	// findAndTypeField walks Down through the browser. If the field is on the
	// landing page it is found and typed there. If instead a search link (a
	// micron link whose target contains "search", e.g. ICP Board's "OPEN LIVE
	// SEARCH" -> /page/search.mu) is encountered BEFORE any field, the scan
	// follows it (Enter) to the search page and continues scanning there, where
	// the editable field lives. This handles both a node whose field is on its
	// landing page and ICP Board, whose field is one link deep — without a
	// focus-breaking Home reset between the two pages.
	d.findAndTypeField(cfg)
	if !d.fieldFound {
		d.failures++
		d.logf("  ASSERT FAIL: input field not focusable — typed %q never appeared in the browser pane", cfg.text)
		d.logf("  (this is the known-broken gonomadnet browser input box: micron <fields are static text, not editable ReadlineEdit widgets)")
		d.snapshot("search form: input field NOT found (text entry broken)")
		// Fall through best-effort: keep the run going so the final state is
		// captured and the log records what the page looked like.
	}

	// --- Locate the Search/submit link and press Enter. ---
	//
	// findAndPressSubmit focuses the Search link (its footer indicator appears,
	// ~100ms after the cursor rests on it) and captures d.preSubmitSig RIGHT
	// BEFORE pressing Enter — with the footer already baked into the baseline.
	// The response-wait then compares against d.preSubmitSig, so the footer
	// appearing cannot itself satisfy "sig changed"; only a real results-page
	// render (or a "Results for:" line, or an error) ends the wait. Capturing the
	// baseline before typing (or before the button is focused) was wrong: the
	// cursor moving onto the Search link changes the pane (the footer appears in
	// the right pane) and satisfied the wait in ~23ms, before any mesh search
	// could return.
	d.findAndPressSubmit(cfg)

	// --- Wait for the search response. ---
	//
	// Submitting collects the field value and navigates the browser to the
	// search URL, so the pane goes to "Retrieving" then re-renders the results.
	// We wait for a terminal state: rendered with a DIFFERENT right-pane
	// signature than the pre-submit form (the results page), OR the appearance
	// of a "Results for:" line (the definitive search-executed signal), OR a
	// connect/fetch error. Because d.preSubmitSig already includes the Search
	// link's footer indicator, the wait cannot fire on the footer merely
	// appearing — only on a genuine content change.
	d.logf("  waiting up to %v for the search response to render...", d.responseWait)
	_, ok := d.waitFor(func(v *utils.View) bool {
		st, _ := v.BrowserState()
		if strings.HasPrefix(st, "error:") {
			return true
		}
		if st == utils.BSRendered {
			if strings.Contains(v.BrowserPaneSig(), "Results for:") {
				return true
			}
			return v.BrowserPaneSig() != d.preSubmitSig
		}
		return false
	}, d.responseWait)
	st, _ := d.view().BrowserState()
	if ok {
		d.logf("  search response RENDERED (state=%q)", st)
	} else {
		d.failures++
		d.logf("  ASSERT FAIL: no search response within %v (state=%q)", d.responseWait, st)
	}
	d.snapshot("search form: search response (final page)")

	// Capture the ENTIRE search response, not just the first viewport. The
	// browser page is a scrollable TextView, so a long results page only shows
	// its top screenful; we page top-to-bottom (Home then repeated PageDown)
	// and stitch each viewport's body rows into one string. This is captured
	// while the app is still running; the caller prints it only after the app
	// has quit and the terminal has reset, so the output is not interleaved with
	// the TUI.
	d.results = d.captureResultsScrolled()
	d.logf("  captured %d bytes of browser output (scrolled)", len(d.results))
}

// captureResultsScrolled pages the browser top-to-bottom and collects every
// distinct line of the search response, so a results page longer than one
// screen is captured in full. It scrolls with Home/PageDown (the browser's
// page-body nav keys, browser-nav.go) and reads BrowserContentLines — the
// scrollable body region between the URL header and the transfer-status footer
// (the fixed chrome that does not scroll).
//
// Lines are matched on a normalized form (box-drawing borders stripped and
// trimmed) rather than the raw capture, because consecutive frames do not render
// byte-identically: the per-line cursor/focus highlight and the inner micron
// box borders shift between frames, so an exact-line overlap stitch would miss
// the overlap and duplicate the whole top of the page. Normalizing makes the
// same logical line compare equal across frames, so each distinct result line
// is kept exactly once in first-seen order. The loop stops when a PageDown
// brings no new normalized line into view (the bottom of the page).
func (d *driver) captureResultsScrolled() string {
	// Start from the top of the results so the capture begins at the first line.
	d.send("Home")

	seen := make(map[string]bool)
	var out []string
	const maxScrolls = 80
	for i := range maxScrolls {
		if !d.sess.HasSession() {
			d.logf("  scroll capture: session died at frame %d", i)
			break
		}
		lines := d.view().BrowserContentLines()
		if len(lines) == 0 {
			if i == 0 {
				d.logf("  scroll capture: no browser body region found; falling back to whole-pane text")
				return d.view().BrowserPaneText()
			}
			break
		}
		added := 0
		for _, line := range lines {
			n := normLine(line)
			if n == "" {
				continue
			}
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
				added++
			}
		}
		// A frame that added no new normalized line means the page-down brought
		// nothing new into view — we have reached the bottom.
		if i > 0 && added == 0 {
			d.logf("  scroll capture: reached bottom after %d frame(s), %d distinct line(s)", i, len(out))
			break
		}
		d.send("PageDown")
	}
	return strings.Join(out, "\n")
}

// normLine strips the browser/micron box-drawing borders and surrounding
// whitespace from a captured line so the same logical line compares equal across
// frames that render it with different borders (outer ┃ vs inner │, presence of
// a side border depending on scroll position, the cursor highlight, etc.).
func normLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "│┃┌┐└┘─━┄═║├┤┬┴┼")
	return strings.TrimSpace(s)
}

// findAndTypeField walks Down through the browser and types the query at each
// non-link row until one accepts it (the editable input field). It stops on the
// FIRST acceptance so the query is typed exactly once. It sets d.fieldFound.
//
// If a SEARCH link (a micron link whose footer target contains "search", e.g.
// ICP Board's "OPEN LIVE SEARCH" -> /page/search.mu) is encountered BEFORE any
// field accepts text, the scan follows it (Enter) to the search page and
// continues scanning there, where the editable field lives. This handles a node
// whose field is on its landing page (found before any search link) AND ICP
// Board, whose field is one link deep — without a focus-breaking Home reset
// between the two pages (Home moves focus to the menu, not the browser top).
//
// Detection is scoped to the browser RIGHT pane (BrowserPaneSig), NOT the whole
// pane: the Network page's LEFT pane shows the Local Peer Info box whose "Name :"
// row is the LOCAL node's name — and the default --text IS the local node name
// "Go port of NomadNet". A whole-pane Contains check would always find the text
// already present, so the "newly appeared" test would never fire and the loop
// would append the query to the field at every row. BrowserPaneSig excludes the
// left pane, so the text only counts when it is actually typed into the browser
// field.
//
// Link rows are skipped (not typed into): typing on a micron link is not field
// entry and in the Python original can ACTIVATE it, navigating away from the
// form mid-scan. A search link is followed via Enter (not by typing).
//
// This is the regression sentinel: while the gonomadnet input box is broken
// (fields rendered as static text), no row accepts the text, the field is never
// found, and the harness reports exactly that.
func (d *driver) findAndTypeField(cfg *config) {
	navigated := false // follow at most one search link to the search page
	for i := range cfg.maxFieldDowns {
		v := d.view()
		footer := v.BrowserFooterLink()
		if footer != "" {
			// On a link. If it targets a search page and we have not yet
			// navigated (and not yet found the field), follow it to the search
			// page and keep scanning there.
			if !d.fieldFound && !navigated && strings.Contains(strings.ToLower(footer), "search") {
				d.logf("  field scan at %d Down(s): following search link to search page (footer=%q)", i, footer)
				d.snapshot("search form: navigating to search page")
				landedSig := v.BrowserPaneSig()
				d.send("Enter")
				// Wait for the search page to render. The landing page also
				// contains "Search" ("LIVE NOMAD SEARCH", "OPEN LIVE SEARCH"),
				// so a mere "Search" Contains check would match the page we are
				// leaving. Require the signature to DIFFER from the landing page
				// (navigation happened) AND the search page's distinctive prompt
				// "one or more search words" to be present (or a connect error).
				_, _ = d.waitFor(func(v *utils.View) bool {
					st, _ := v.BrowserState()
					if strings.HasPrefix(st, "error:") {
						return true
					}
					if st != utils.BSRendered {
						return false
					}
					sig := v.BrowserPaneSig()
					return sig != landedSig && strings.Contains(sig, "one or more search words")
				}, d.connectWait)
				st, _ := d.view().BrowserState()
				d.snapshot("search form: search page rendered")
				if strings.HasPrefix(st, "error:") {
					d.logf("  navigation to search page FAILED: %s", st)
					return
				}
				d.logf("  search page rendered (state=%q); resuming field scan", st)
				navigated = true
				continue // resume scanning on the new page (cursor at its top)
			}
			// Other link (or already navigated): skip, do not type.
			d.send("Down")
			continue
		}
		// Non-link row: type the query and check whether it was accepted.
		before := v.BrowserPaneSig()
		d.sendLiteral(cfg.text)
		after := d.view().BrowserPaneSig()
		if !strings.Contains(before, cfg.text) && strings.Contains(after, cfg.text) {
			d.fieldFound = true
			d.textAccepted = true
			d.logf("  input field FOUND after %d Down(s): query %q accepted into the field", i, cfg.text)
			d.snapshot("search form: text accepted into input field")
			return
		}
		// Text was not accepted by this line; move to the next selectable.
		d.send("Down")
	}
	d.logf("  no input field accepting text found after %d Down(s)", cfg.maxFieldDowns)
}

// findAndPressSubmit walks Down to the "Search" submit link (a micron link whose
// footer target contains "search" and which collects the field value) and
// presses Enter to submit. The submit lives on the search page alongside the
// field (e.g. the "Search" link with footer ":/page/search.mu`query").
func (d *driver) findAndPressSubmit(cfg *config) {
	label := strings.ToLower(cfg.searchLabel)
	for i := range cfg.maxButtonDowns {
		v := d.view()
		footer := strings.ToLower(v.BrowserFooterLink())
		rowText := ""
		if v.Screen != nil && v.CursorOK && v.CursorY >= 0 && v.CursorY < v.Screen.H {
			rowText = strings.ToLower(v.Screen.RowText(v.CursorY))
		}
		if footer != "" && (strings.Contains(footer, "search") || strings.Contains(rowText, label)) {
			// Give the footer's 100ms marked-link alarm a moment in case the
			// cursor just landed but the alarm hasn't fired yet.
			if !strings.Contains(footer, "search") {
				time.Sleep(d.stepDelay)
				footer = strings.ToLower(d.view().BrowserFooterLink())
			}
			if strings.Contains(footer, "search") {
				d.searchSubmitted = true
				d.logf("  search button FOUND after %d Down(s) (footer=%q); pressing Enter to submit", i, footer)
				d.snapshot("search form: Search button focused (footer link)")
				// Capture the pre-submit signature NOW: the cursor is resting on
				// the Search link so its footer indicator is already part of the
				// pane. Baking the footer into the baseline means the response-wait
				// cannot be fooled by the footer merely appearing — only a real
				// results-page render (different content) will differ from this.
				d.preSubmitSig = d.view().BrowserPaneSig()
				d.send("Enter")
				return
			}
		}
		d.send("Down")
	}
	d.failures++
	d.logf("  ASSERT FAIL: Search button %q not found after %d Down(s); pressing Enter at the cursor as a best effort", cfg.searchLabel, cfg.maxButtonDowns)
	d.snapshot("search form: Search button NOT found")
	d.preSubmitSig = d.view().BrowserPaneSig()
	d.send("Enter")
}

// quit navigates to the Quit menu item and selects it, then waits for the app
// to exit. If the app does not exit within 15s (e.g. the Quit menu could not be
// reached — a known rough edge), it KILLS the tmux session itself so the
// terminal is reset before the caller prints the results. Best-effort: failures
// here do not abort the run.
func (d *driver) quit() {
	d.step("Phase 5: quit")
	if !d.sess.HasSession() {
		d.logf("session already gone")
		return
	}
	d.escapeToMenu(15)
	d.moveMenuTo(7, 8) // Quit is menu index 7.
	d.assert(menuAt(7), 3*time.Second, "cursor on Quit (index 7)")
	d.send("Enter")
	d.logf("Quit activated; waiting for the app to exit...")
	elapsed := 0 * time.Second
	for elapsed < 15*time.Second {
		if !d.sess.HasSession() {
			d.logf("app exited cleanly after %v", elapsed)
			return
		}
		time.Sleep(1 * time.Second)
		elapsed += 1 * time.Second
	}
	if d.sess.HasSession() {
		d.logf("WARN: app did not exit within 15s of Quit; killing the tmux session to reset the terminal")
		_ = d.sess.KillSession()
	}
}

// ---------- minimal navigation helpers (mirrors run-tmux-test-suite) ----------

// menuAt is a Wait condition: the menu bar is focused and the focused button is
// the given index.
func menuAt(idx int) func(*utils.View) bool {
	return func(v *utils.View) bool {
		i, ok := v.MenuFocusedButton()
		return ok && i == idx
	}
}

// menuFocused is a Wait condition: any menu button is focused.
func menuFocused() func(*utils.View) bool {
	return func(v *utils.View) bool {
		_, ok := v.MenuFocusedButton()
		return ok
	}
}

// urlDialogOpen is a Wait condition: the Ctrl-u URL dialog is on screen. Both
// the browser (bd.OnURLDialog) and network (networkDisplay.OnURLDialog)
// handlers now show the same Python-parity dialog (title "Enter URL", caption
// "URL : "), so either the border title or the caption matching is enough —
// this stays robust even if a focus-routing quirk fires one handler instead of
// the other, or if the border title render varies between the Go and Python
// ports.
func urlDialogOpen(v *utils.View) bool {
	if v.Screen == nil {
		return false
	}
	full := v.Screen.FullText()
	return utils.HasBorderTitle(full, "Enter URL") || strings.Contains(full, "URL : ")
}

// moveMenuTo moves the menu focus to the target button index by sending Right
// (the menu wraps) until MenuFocusedButton matches, capped at `cap` sends.
func (d *driver) moveMenuTo(target, cap int) bool {
	for range cap {
		if idx, ok := d.view().MenuFocusedButton(); ok && idx == target {
			return true
		}
		d.send("Right")
	}
	idx, ok := d.view().MenuFocusedButton()
	return ok && idx == target
}

// escapeToMenu moves focus from wherever it is (browser content, the announce
// list, the tab/filter bar, a dialog) up to the menu bar. Caps total steps.
func (d *driver) escapeToMenu(maxSteps int) {
	homeSent := false
	for range maxSteps {
		v := d.view()
		if _, ok := v.MenuFocusedButton(); ok {
			return
		}
		if v.Screen != nil && utils.HasBorderTitle(v.Screen.FullText(), "Announce Info") {
			d.send("Escape")
			homeSent = false
			continue
		}
		if btn, ok := v.FocusedActionButton(); ok && (btn == "Save" || btn == "Node Info") {
			d.send("Up")
			homeSent = false
			continue
		}
		// Cursor in the right pane (browser) and not on a node row -> Left
		// releases focus back to the left list.
		if v.CursorOK && v.CursorX >= utils.NetworkLeftWidth && !v.CursorOnAnnounceNodeRow() {
			d.send("Left")
			homeSent = false
			continue
		}
		if v.ActivePage() == "network" {
			if !homeSent {
				d.send("Home")
				homeSent = true
				continue
			}
			d.send("Up")
			continue
		}
		d.send("Up")
	}
}

// ---------- launch helpers ----------

func parseSize(cfg *config) (w, h int, err error) {
	if cfg.size != "" {
		if ww, hh, ok := utils.ParseSize(cfg.size); ok {
			return ww, hh, nil
		}
		return 0, 0, fmt.Errorf("invalid --size %q (want WxH)", cfg.size)
	}
	if cfg.headed {
		if ww, hh, ok := terminalSize(); ok {
			return ww, hh, nil
		}
	}
	return 135, 32, nil
}

func requireTools(cfg *config) error {
	tools := []string{"tmux"}
	if cfg.nomadnet {
		tools = append(tools, "nomadnet")
	} else {
		tools = append(tools, "go")
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found on PATH", tool)
		}
	}
	return nil
}

func validateDest(dest string) error {
	d := strings.TrimSpace(dest)
	d = strings.TrimPrefix(d, "0x")
	d = strings.TrimPrefix(d, "0X")
	if len(d) != 32 {
		return fmt.Errorf("--dest must be a 16-byte (32 hex char) destination hash, got %q", dest)
	}
	for _, c := range d {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fmt.Errorf("--dest must be hex, got %q", dest)
		}
	}
	return nil
}

// openLog opens the log file and returns a timestamped line writer and a closer.
func openLog(path string) (func(string, ...any), func(), error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create log %s: %w", path, err)
	}
	start := time.Now()
	logf := func(format string, args ...any) {
		ts := time.Since(start).Truncate(time.Millisecond)
		msg := fmt.Sprintf(format, args...)
		_, _ = fmt.Fprintf(f, "[%s] %s\n", ts, msg)
	}
	return logf, func() { _ = f.Close() }, nil
}

// terminalSize queries the actual terminal size for --headed mode.
func terminalSize() (w, h int, ok bool) {
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil && w > 0 && h > 0 {
		return w, h, true
	}
	if f, err := os.Open("/dev/tty"); err == nil {
		if w, h, err := term.GetSize(int(f.Fd())); err == nil && w > 0 && h > 0 {
			_ = f.Close()
			return w, h, true
		}
		_ = f.Close()
	}
	cmd := exec.Command("stty", "size")
	if f, err := os.Open("/dev/tty"); err == nil {
		cmd.Stdin = f
		out, err := cmd.Output()
		_ = f.Close()
		if err == nil {
			if ww, hh, ok2 := utils.ParseSize(strings.TrimSpace(string(out))); ok2 {
				return ww, hh, ok2
			}
		}
	}
	return 0, 0, false
}