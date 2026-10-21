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

//go:build !no_tui && !embedded && !pocket_communicator && !pocket_hub

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

// TestStdLogRouteNeverWritesToTheTerminal pins the invariant the terminal UI
// depends on: once the app logger exists, no standard-log line may be written
// to the console, whatever state the logger is in.
func TestStdLogRouteNeverWritesToTheTerminal(t *testing.T) {
	t.Parallel()

	newWriter := func(logger *rns.Logger) (*stdLogLineWriter, *bytes.Buffer) {
		var console bytes.Buffer
		w := &stdLogLineWriter{errOut: &console}
		if logger != nil {
			w.logger.Store(logger)
		}
		return w, &console
	}

	t.Run("before the logger exists the line goes to stderr", func(t *testing.T) {
		t.Parallel()
		w, console := newWriter(nil)
		w.route("startup line")
		if got := console.String(); !strings.Contains(got, "startup line") {
			t.Fatalf("console = %q, want the startup line", got)
		}
	})

	t.Run("with a working logger nothing reaches the console", func(t *testing.T) {
		t.Parallel()
		logPath := filepath.Join(tempDir(t), "logfile")
		logger := rns.NewLogger()
		logger.SetLogLevel(rns.LogInfo)
		logger.SetLogDest(rns.LogDestFile)
		logger.SetLogFilePath(logPath)
		w, console := newWriter(logger)
		w.route("file line")
		if !logger.Flush() {
			t.Fatal("Flush() timed out")
		}
		if got := console.String(); got != "" {
			t.Fatalf("console = %q, want nothing", got)
		}
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if !strings.Contains(string(data), "file line") {
			t.Fatalf("log file %q missing the line", data)
		}
	})

	t.Run("a latched console override is not re-fed or drawn", func(t *testing.T) {
		t.Parallel()
		logger := rns.NewLogger()
		logger.SetLogLevel(rns.LogInfo)
		logger.SetLogDest(rns.LogDestFile)
		logger.SetLogFilePath(filepath.Join(tempDir(t), "logfile"))
		logger.SetAlwaysOverride(true)
		w, console := newWriter(logger)
		// A re-fed logger would loop here forever; dropping it returns.
		w.route("latched line")
		if got := console.String(); got != "" {
			t.Fatalf("console = %q, want nothing", got)
		}
	})

	t.Run("a closed logger drops the line", func(t *testing.T) {
		t.Parallel()
		logger := rns.NewLogger()
		logger.Close()
		w, console := newWriter(logger)
		w.route("late line")
		if got := console.String(); got != "" {
			t.Fatalf("console = %q, want nothing", got)
		}
	})
}

// TestTUILogDestError verifies the terminal UI refuses a console log
// destination, which would be drawn over the screen, and accepts a file one.
func TestTUILogDestError(t *testing.T) {
	t.Parallel()

	const path = "/home/user/.nomadnetwork/config"
	if err := tuiLogDestError(rns.LogStdout, path); err == nil {
		t.Fatal("tuiLogDestError(LogStdout) = nil, want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q does not name the config file", err)
	}
	for _, dest := range []int{rns.LogDestFile, rns.LogCallback} {
		if err := tuiLogDestError(dest, path); err != nil {
			t.Fatalf("tuiLogDestError(%v) = %v, want nil", dest, err)
		}
	}
}
