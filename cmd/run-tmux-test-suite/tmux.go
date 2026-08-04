// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// tmux.go wraps tmux remote-control: creating/destroying a session, sending
// keys, capturing the pane (plain and with ANSI escapes), reading the terminal
// cursor position, and polling a View (screen + cursor) until a condition holds
// or a timeout elapses.

// Session is a tmux session under test.
type Session struct {
	Name string
}

// NewSession creates a detached tmux session at the given size running the
// launch command in the given working directory, with window-size manual so a
// later attach cannot resize it. The session is created fresh; an existing
// session of the same name is killed first.
func NewSession(name, dir, launchCmd string, w, h int) (*Session, error) {
	// Kill any stale session of the same name.
	_ = exec.Command("tmux", "kill-session", "-t", name).Run()

	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-n", "main",
		"-x", strconv.Itoa(w), "-y", strconv.Itoa(h),
		"--", "bash", "-lc", launchCmd)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("tmux new-session: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	// Pin the window size so attaching (or the client) cannot resize it away
	// from the fixed size we drive against.
	_ = exec.Command("tmux", "set-option", "-t", name, "-w", "window-size", "manual").Run()
	_ = exec.Command("tmux", "resize-window", "-t", name, "-x", strconv.Itoa(w), "-y", strconv.Itoa(h)).Run()
	return &Session{Name: name}, nil
}

// SendKeys sends tmux key names (Up/Down/Left/Right/Enter/Home/End/PageUp/
// PageDown/Escape/Space/C-l/C-d/...). Each is interpreted by tmux as a key.
func (s *Session) SendKeys(keys ...string) error {
	args := append([]string{"send-keys", "-t", s.Name}, keys...)
	return exec.Command("tmux", args...).Run()
}

// SendLiteral sends literal text (no tmux key-name interpretation).
func (s *Session) SendLiteral(text string) error {
	return exec.Command("tmux", "send-keys", "-t", s.Name, "-l", text).Run()
}

// Capture returns the visible pane content as plain text (no escapes).
func (s *Session) Capture() (string, error) {
	out, err := exec.Command("tmux", "capture-pane", "-t", s.Name, "-p").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CaptureScreen returns the visible pane parsed into a cell grid (with ANSI
// SGR escapes from `capture-pane -e -p`).
func (s *Session) CaptureScreen() (*Screen, error) {
	out, err := exec.Command("tmux", "capture-pane", "-t", s.Name, "-e", "-p").Output()
	if err != nil {
		return nil, err
	}
	return parseScreen(out), nil
}

// CursorPos returns the terminal cursor position (column, row) reported by
// tmux for the pane. ok=false if tmux could not report it. This is the sole
// reliable indicator of menu/tab/button focus in this TUI (those elements are
// color-neutral; focus is shown only by the hardware cursor).
func (s *Session) CursorPos() (x, y int, ok bool) {
	out, err := exec.Command("tmux", "display-message", "-t", s.Name, "-p", "#{cursor_x},#{cursor_y}").Output()
	if err != nil {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	xv, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	yv, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return xv, yv, true
}

// View captures the screen and cursor position together.
func (s *Session) View() (*View, error) {
	sc, err := s.CaptureScreen()
	if err != nil {
		return nil, err
	}
	cx, cy, ok := s.CursorPos()
	return &View{Screen: sc, CursorX: cx, CursorY: cy, CursorOK: ok}, nil
}

// Resize sets the session window size.
func (s *Session) Resize(w, h int) error {
	return exec.Command("tmux", "resize-window", "-t", s.Name,
		"-x", strconv.Itoa(w), "-y", strconv.Itoa(h)).Run()
}

// HasSession reports whether the tmux session still exists.
func (s *Session) HasSession() bool {
	return exec.Command("tmux", "has-session", "-t", s.Name).Run() == nil
}

// KillSession kills the session.
func (s *Session) KillSession() error {
	return exec.Command("tmux", "kill-session", "-t", s.Name).Run()
}

// pollInterval is the cadence at which Wait re-captures the view.
const pollInterval = 120 * time.Millisecond

// Wait polls the View (screen + cursor) every pollInterval until cond returns
// true or timeout elapses. Returns the last view and whether cond succeeded.
func (s *Session) Wait(cond func(*View) bool, timeout time.Duration) (*View, bool) {
	deadline := time.Now().Add(timeout)
	for {
		v, err := s.View()
		if err == nil && cond(v) {
			return v, true
		}
		if time.Now().After(deadline) {
			return v, false
		}
		time.Sleep(pollInterval)
	}
}

// WaitStable polls region(view) until it is identical across two captures
// `settle` apart, or timeout elapses. Returns the stable region value and
// whether it settled. Used to confirm a page has stopped changing.
func (s *Session) WaitStable(region func(*View) string, settle, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	var prev string
	for {
		v, err := s.View()
		if err != nil {
			if time.Now().After(deadline) {
				return "", false
			}
			time.Sleep(pollInterval)
			continue
		}
		cur := region(v)
		if cur == prev && cur != "" {
			return cur, true
		}
		prev = cur
		if time.Now().After(deadline) {
			return cur, false
		}
		time.Sleep(settle)
	}
}
