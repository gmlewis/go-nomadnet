// Copyright 2026 Glenn Lewis. All rights reserved.

// Command test-conversations drives TWO headless gonomadnet TUIs (in separate
// tmux sessions, each with its own private config + RNS config) over tmux
// remote-control, connects them over a localhost RNS TCP link (A = TCP server,
// B = TCP client), starts a conversation between them, and exercises every
// Conversations-panel feature and keyboard shortcut — logging all tmux
// operations to a single timestamped log file for analysis, exactly like
// cmd/run-tmux-test-suite.
//
// Connectivity is proven by a real LXMF message round-trip, not by scraping
// stdout: each instance reads the OTHER's LXMF address off its screen and feeds
// it to a "New Conversation" dialog, then they exchange messages.
//
// Usage:
//
//	go run ./cmd/test-conversations [--log FILE] [--size 135x32]
//	    [--step-delay 400ms] [--announce-wait 30s] [--msg-wait 30s]
//	    [--keep-config]
//
// Watch either instance live in another terminal:
//
//	tmux attach -t gtc-A-<unix>   # or gtc-B-<unix>
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
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "test-conversations: %v\n", err)
		os.Exit(1)
	}
}

// config holds the parsed command-line flags.
type config struct {
	logFile      string
	size         string
	stepDelay    time.Duration
	announceWait time.Duration
	msgWait      time.Duration
	keepConfig   bool
}

func parseFlags(args []string) (*config, error) {
	fs := flag.NewFlagSet("test-conversations", flag.ContinueOnError)
	c := &config{
		stepDelay:    envDur("GNOMADNET_STEP_DELAY", 400*time.Millisecond),
		announceWait: envDur("GNOMADNET_ANNOUNCE_WAIT", 30*time.Second),
		msgWait:      envDur("GNOMADNET_MSG_WAIT", 30*time.Second),
		size:         "135x32",
	}
	fs.StringVar(&c.logFile, "log", "", "log file path (default /tmp/gonomadnet-test-conversations-NNN.log)")
	fs.StringVar(&c.size, "size", c.size, "pane size WxH (default 135x32)")
	fs.DurationVar(&c.stepDelay, "step-delay", c.stepDelay, "delay between keystrokes")
	fs.DurationVar(&c.announceWait, "announce-wait", c.announceWait, "how long to wait for announce/path propagation")
	fs.DurationVar(&c.msgWait, "msg-wait", c.msgWait, "how long to wait for a message to arrive")
	fs.BoolVar(&c.keepConfig, "keep-config", false, "keep the temp config/rns dirs for debugging")
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

func requireTools() error {
	for _, tool := range []string{"tmux", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found on PATH", tool)
		}
	}
	return nil
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
	logFile := cfg.logFile
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/gonomadnet-test-conversations-%s.log", nnn)
	}
	logf, closeLog, err := openLog(logFile)
	if err != nil {
		return err
	}
	defer closeLog()

	w, hh, ok := utils.ParseSize(cfg.size)
	if !ok {
		return fmt.Errorf("invalid --size %q (want WxH)", cfg.size)
	}

	repo, _ := os.Getwd()

	both := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
		logf(format, a...)
	}

	both("gonomadnet conversations test harness")
	both("  log     : %s", logFile)
	both("  repo    : %s", repo)
	both("  size    : %dx%d", w, hh)
	both("  waits   : announce=%s msg=%s step=%s", cfg.announceWait, cfg.msgWait, cfg.stepDelay)
	both("  keep    : %t", cfg.keepConfig)
	if out, err := exec.Command("tmux", "-V").Output(); err == nil {
		both("  tmux    : %s", strings.TrimSpace(string(out)))
	}
	both("")

	// One temp root holds the built binary + both instances' config/rns dirs, so
	// a single RemoveAll cleans everything up (unless --keep-config). Always
	// create it under /tmp (NOT os.MkdirTemp("", ...) whose "" resolves to
	// $TMPDIR — on macOS that is a per-user /var/folders/.../T path, so kept-config
	// dirs "vanish" when you look in /tmp and are fiddly to find/attach to).
	root, err := os.MkdirTemp("/tmp", "gonomadnet-tc-XXXXXX")
	if err != nil {
		return fmt.Errorf("temp root: %w", err)
	}
	if !cfg.keepConfig {
		defer func() { _ = os.RemoveAll(root) }()
	}
	binPath := filepath.Join(root, "gonomadnet")
	cfgA := filepath.Join(root, "cfgA")
	rnsA := filepath.Join(root, "rnsA")
	cfgB := filepath.Join(root, "cfgB")
	rnsB := filepath.Join(root, "rnsB")

	// Build the gonomadnet binary once (faster + avoids two `go run` compiles).
	both("building gonomadnet binary...")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/gonomadnet")
	build.Dir = repo
	build.Env = append(os.Environ(), "GOCACHE=/tmp/go-cache")
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("go build gonomadnet: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// Connectivity: A hosts a TCP server on a free localhost port; B connects.
	port, err := freePort()
	if err != nil {
		return err
	}
	if err := writeNomadNetConfig(cfgA); err != nil {
		return err
	}
	if err := writeRNSConfigServer(rnsA, port); err != nil {
		return err
	}
	if err := writeNomadNetConfig(cfgB); err != nil {
		return err
	}
	if err := writeRNSConfigClient(rnsB, port); err != nil {
		return err
	}
	both("network  : A=TCP server 127.0.0.1:%d   B=TCP client -> 127.0.0.1:%d", port, port)

	// tcell truecolor is robust regardless of $TERM; the tmux sessions inherit
	// the parent env so this reaches both TUIs.
	_ = os.Setenv("COLORTERM", "truecolor")

	launch := func(cfgDir, rnsDir string) string {
		return fmt.Sprintf("exec '%s' -t -config '%s' -rnsconfig '%s'", binPath, cfgDir, rnsDir)
	}
	nameA := "gtc-A-" + nnn
	nameB := "gtc-B-" + nnn

	sessA, err := utils.NewSession(nameA, repo, launch(cfgA, rnsA), w, hh)
	if err != nil {
		return fmt.Errorf("FATAL session A: %w", err)
	}
	sessB, err := utils.NewSession(nameB, repo, launch(cfgB, rnsB), w, hh)
	if err != nil {
		_ = sessA.KillSession()
		return fmt.Errorf("FATAL session B: %w", err)
	}
	defer func() {
		for _, s := range []*utils.Session{sessA, sessB} {
			if s.HasSession() {
				logf("cleanup: killing tmux session %s", s.Name)
				_ = s.KillSession()
			}
		}
	}()

	both("sessions : %s  %s  (watch live: tmux attach -t %s | %s)", nameA, nameB, nameA, nameB)
	both("")

	h := &harness{
		dA:           newDriver(sessA, "[A]", logf, cfg),
		dB:           newDriver(sessB, "[B]", logf, cfg),
		logf:         logf,
		announceWait: cfg.announceWait,
		msgWait:      cfg.msgWait,
	}
	h.dA.peer = h.dB
	h.dB.peer = h.dA

	runAllPhases(h, func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
		logf(format, a...)
	})

	both("")
	both("================ DONE ================")
	both("log    : %s", logFile)
	ta, pa, fa := h.dA.summary()
	tb, pb, fb := h.dB.summary()
	both("summary : A asserts=%d pass=%d fail=%d | B asserts=%d pass=%d fail=%d | total asserts=%d pass=%d fail=%d",
		ta, pa, fa, tb, pb, fb, ta+tb, pa+pb, fa+fb)
	return nil
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
		_, _ = fmt.Fprintf(f, "[%s] %s\n", ts, msg)
	}
	return logf, func() { _ = f.Close() }, nil
}
