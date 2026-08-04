// Copyright 2026 Glenn Lewis. All rights reserved.

// Command run-tmux-test-suite drives a live gonomadnet TUI (go run
// ./cmd/gonomadnet) inside a tmux session via remote-control keystrokes, and
// ACTIVELY VERIFIES each step by parsing the pane's ANSI-escape screen capture
// plus the terminal cursor position — rather than blindly sleeping and grepping
// like its bash predecessor (scripts/run-tmux-test-suite.sh).
//
// It runs five phases: (1) tour every main-menu command except Quit; (2) select
// Network, Down, Ctrl-L to the Announce Stream; (3) connect to up to N nodes,
// wait for the index.mu to actually render (or detect an error), follow a couple
// of links, repeat; (4) go to the Guide, walk all 12 topics, exercising the
// known scroll-not-reset bug; (5) navigate to Quit and select it.
//
// Everything — every keystroke, every assertion, every screen snapshot — is
// logged to /tmp/gonomadnet-tmux-test-suite-NNN.log (NNN = seconds since epoch).
//
// Usage:
//
//	go run ./cmd/run-tmux-test-suite [--headed] [--copy-config] [--fresh]
//	    [--config DIR] [--rnsconfig DIR] [--announce-wait 25s] [--connect-wait 12s]
//	    [--node-iters 7] [--step-delay 400ms] [--size 135x32] [--log FILE]
//
// In --headed mode the program queries the ACTUAL terminal size and uses the
// FULL terminal (not a fixed sub-window), then attaches the tmux session to
// this shell so you can watch the scripted run live.
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

	"golang.org/x/term"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "run-tmux-test-suite: %v\n", err)
		os.Exit(1)
	}
}

// config holds the parsed command-line flags.
type config struct {
	headed       bool
	copyConfig   bool
	fresh        bool
	configDir    string
	rnsconfigDir string
	announceWait time.Duration
	connectWait  time.Duration
	nodeIters    int
	stepDelay    time.Duration
	size         string // WxH, "" = auto for headed / 135x32 for detached
	logFile      string
}

func parseFlags(args []string) (*config, error) {
	fs := flag.NewFlagSet("run-tmux-test-suite", flag.ContinueOnError)
	c := &config{
		configDir:    os.Getenv("GNOMADNET_CONFIG"),
		announceWait: envDur("GNOMADNET_ANNOUNCE_WAIT", 25*time.Second),
		connectWait:  envDur("GNOMADNET_CONNECT_WAIT", 12*time.Second),
		nodeIters:    envInt("GNOMADNET_NODE_ITERATIONS", 7),
		stepDelay:    envDur("GNOMADNET_STEP_DELAY", 400*time.Millisecond),
	}
	if c.configDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.configDir = filepath.Join(home, ".nomadnetwork")
		}
	}
	fs.BoolVar(&c.headed, "headed", false, "attach the tmux session to this terminal so the test is watchable (uses the full terminal size)")
	fs.BoolVar(&c.copyConfig, "copy-config", false, "copy the config dir to a temp dir so the real config is not modified")
	fs.BoolVar(&c.fresh, "fresh", false, "run with an empty config dir (first-run Guide boot; no live nodes)")
	fs.StringVar(&c.configDir, "config", c.configDir, "config directory to use (as-is)")
	fs.StringVar(&c.rnsconfigDir, "rnsconfig", "", "RNS config dir passed as -rnsconfig")
	fs.DurationVar(&c.announceWait, "announce-wait", c.announceWait, "how long to wait for announcing nodes before connecting")
	fs.DurationVar(&c.connectWait, "connect-wait", c.connectWait, "how long to wait for a node page to render")
	fs.IntVar(&c.nodeIters, "node-iters", c.nodeIters, "number of node connect iterations")
	fs.DurationVar(&c.stepDelay, "step-delay", c.stepDelay, "delay between keystrokes")
	fs.StringVar(&c.size, "size", "", "pane size WxH (default: full terminal in --headed, else 135x32)")
	fs.StringVar(&c.logFile, "log", "", "log file path (default /tmp/gonomadnet-tmux-test-suite-NNN.log)")
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

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func run(args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	if err := requireTools(); err != nil {
		return err
	}

	nnn := strconv.FormatInt(time.Now().Unix(), 10)
	sessionName := "gonet-" + nnn
	logFile := cfg.logFile
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/gonomadnet-tmux-test-suite-%s.log", nnn)
	}
	logf, closeLog, err := openLog(logFile)
	if err != nil {
		return err
	}
	defer closeLog()

	// Resolve the config dir to run against.
	configDir, tempConfig, err := resolveConfig(cfg)
	if err != nil {
		return err
	}
	if tempConfig != "" {
		defer os.RemoveAll(tempConfig)
	}

	// Determine the pane size. In --headed, query the ACTUAL terminal and use
	// the FULL terminal (not a fixed sub-window). In detached mode, default to
	// 135x32 (the size the app is tuned for) for reproducibility.
	w, h := 135, 32
	if cfg.size != "" {
		if ww, hh, ok := parseSize(cfg.size); ok {
			w, h = ww, hh
		} else {
			return fmt.Errorf("invalid --size %q (want WxH)", cfg.size)
		}
	} else if cfg.headed {
		if ww, hh, ok := terminalSize(); ok {
			w, h = ww, hh
		} else {
			logf("WARN: could not query terminal size; falling back to 135x32")
		}
	}

	// Build the launch command.
	rnsFlag := ""
	if cfg.rnsconfigDir != "" {
		rnsFlag = fmt.Sprintf(" -rnsconfig '%s'", cfg.rnsconfigDir)
	}
	repo, _ := os.Getwd()
	launchCmd := fmt.Sprintf("cd '%s' && exec go run ./cmd/gonomadnet -t -config '%s'%s", repo, configDir, rnsFlag)

	both := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
		logf(format, args...)
	}

	both("gonomadnet tmux test suite (Go)")
	both("  log     : %s", logFile)
	both("  session : %s  (watch live: tmux attach -t %s)", sessionName, sessionName)
	both("  config  : %s", configSummary(cfg, configDir))
	both("  repo    : %s", repo)
	both("  size    : %dx%d", w, h)
	both("  flags   : announce-wait=%s connect-wait=%s node-iters=%d step-delay=%s headed=%t",
		cfg.announceWait, cfg.connectWait, cfg.nodeIters, cfg.stepDelay, cfg.headed)
	if out, err := exec.Command("tmux", "-V").Output(); err == nil {
		both("  tmux    : %s", strings.TrimSpace(string(out)))
	}
	both("")

	// tcell truecolor is robust regardless of $TERM.
	_ = os.Setenv("COLORTERM", "truecolor")

	sess, err := NewSession(sessionName, repo, launchCmd, w, h)
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
		sess:         sess,
		logf:         logf,
		stepDelay:    cfg.stepDelay,
		announceWait: cfg.announceWait,
		connectWait:  cfg.connectWait,
		nodeIters:    cfg.nodeIters,
	}

	if cfg.headed {
		// Driver runs in a goroutine logging only to the file; the foreground
		// is reserved for `tmux attach` so the user watches live.
		done := make(chan struct{})
		go func() {
			runAllPhases(d, logf)
			close(done)
		}()
		both("attaching tmux session so you can watch (detach with Ctrl-b d to leave it running)...")
		both("progress is being logged to: %s", logFile)
		both("")
		// NOTE: exec.Command connects unset Stdin/Stdout/Stderr to /dev/null
		// (os/exec exec.go: "If Stdin is nil, the process reads from the null
		// device"), so we MUST explicitly hand tmux attach the real terminal
		// fds or it cannot take over the screen and exits immediately.
		attach := exec.Command("tmux", "attach", "-t", sessionName)
		attach.Stdin = os.Stdin
		attach.Stdout = os.Stdout
		attach.Stderr = os.Stderr
		if err := attach.Run(); err != nil {
			both("WARN: tmux attach exited (%v). The test is still running in the", err)
			both("      background; re-attach to watch: tmux attach -t %s", sessionName)
		}
		<-done
	} else {
		both("running detached. Watch live in another terminal with:")
		both("  tmux attach -t %s", sessionName)
		both("progress is being logged to: %s", logFile)
		both("")
		runAllPhases(d, func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
			logf(format, args...)
		})
	}

	both("")
	both("================ DONE ================")
	both("log    : %s", logFile)
	both("summary: asserts=%d pass=%d fail=%d guide-scroll-bug=%d",
		d.asserts, d.assertOK, d.failures, d.scrollBug)
	if sess.HasSession() {
		both("(tmux session still alive; it will be killed on exit.)")
	}
	return nil
}

// runAllPhases runs the five phases in order, each guarded so a session death
// does not abort the logging of later phases' state.
func runAllPhases(d *driver, logf func(string, ...any)) {
	logf("waiting for the app to start (go compile + intro splash)...")
	d.phase1()
	d.phase2()
	d.phase3()
	d.phase4()
	d.phase5()
}

// openLog opens (creating/truncating) the log file and returns a timestamped
// line writer and a closer.
func openLog(path string) (func(string, ...any), func(), error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create log %s: %w", path, err)
	}
	start := time.Now()
	logf := func(format string, args ...any) {
		ts := time.Since(start).Truncate(time.Millisecond)
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(f, "[%s] %s\n", ts, msg)
	}
	return logf, func() { _ = f.Close() }, nil
}

// resolveConfig returns the config dir to run against and a temp dir to clean
// up (empty if none was created).
func resolveConfig(cfg *config) (dir, temp string, err error) {
	switch {
	case cfg.fresh:
		temp, err = os.MkdirTemp("", "gonet-fresh-XXXXXX")
		if err != nil {
			return "", "", err
		}
		return temp, temp, nil
	case cfg.copyConfig:
		temp, err = os.MkdirTemp("", "gonet-copy-XXXXXX")
		if err != nil {
			return "", "", err
		}
		if fi, statErr := os.Stat(cfg.configDir); statErr == nil && fi.IsDir() {
			// cp -a cfg.configDir/. temp
			cmd := exec.Command("cp", "-a", cfg.configDir+"/.", temp+"/")
			if err := cmd.Run(); err != nil {
				return "", temp, fmt.Errorf("copy config: %w", err)
			}
			return temp, temp, nil
		}
		// Source missing: use an empty temp dir.
		return temp, temp, nil
	default:
		return cfg.configDir, "", nil
	}
}

func configSummary(cfg *config, dir string) string {
	switch {
	case cfg.fresh:
		return fmt.Sprintf("fresh config (first-run Guide); no live nodes — %s", dir)
	case cfg.copyConfig:
		return fmt.Sprintf("copied %s to an isolated temp dir — %s", cfg.configDir, dir)
	default:
		return fmt.Sprintf("REAL config %s (may be modified by the test)", dir)
	}
}

// terminalSize queries the actual terminal size for --headed full-terminal mode.
// It tries golang.org/x/term on stdin, then /dev/tty, then `stty size </dev/tty`.
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
	// Fallback: stty size </dev/tty.
	cmd := exec.Command("stty", "size")
	if f, err := os.Open("/dev/tty"); err == nil {
		cmd.Stdin = f
		out, err := cmd.Output()
		_ = f.Close()
		if err == nil {
			if ww, hh, ok2 := parseSize(strings.TrimSpace(string(out))); ok2 {
				return ww, hh, ok2
			}
		}
	}
	return 0, 0, false
}

// parseSize parses "WxH" (or "H W" from stty size).
func parseSize(s string) (w, h int, ok bool) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "x") {
		parts := strings.SplitN(s, "x", 2)
		if len(parts) != 2 {
			return 0, 0, false
		}
		ww, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		hh, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || ww <= 0 || hh <= 0 {
			return 0, 0, false
		}
		return ww, hh, true
	}
	// stty size: "rows cols".
	parts := strings.Fields(s)
	if len(parts) == 2 {
		hh, err1 := strconv.Atoi(parts[0])
		ww, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || ww <= 0 || hh <= 0 {
			return 0, 0, false
		}
		return ww, hh, true
	}
	return 0, 0, false
}

func requireTools() error {
	for _, tool := range []string{"tmux", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found on PATH", tool)
		}
	}
	return nil
}
