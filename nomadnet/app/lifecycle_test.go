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

package app

import (
	"strings"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

func TestExitHandler(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Logger = rns.NewLogger()
	a.Logger.SetLogDest(rns.LogStdout)
	a.RRC = nil
	a.ShouldRunJobs = true
	a.ExitHandler()
	if a.ShouldRunJobs {
		t.Fatal("ShouldRunJobs should be false after ExitHandler")
	}
}

func TestExceptionHandler(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Logger = rns.NewLogger()
	a.Logger.SetLogDest(rns.LogCallback)

	// The handler's contract is to report the recovered value and the
	// goroutine's stack at error level (Python's panic() log block).
	var lines []string
	a.Logger.SetLogCallback(func(line string) { lines = append(lines, line) })
	a.ExceptionHandler("something broke")

	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"An unhandled panic occurred:",
		"Value: something broke",
		"Trace:\n",
		"goroutine ",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("exception report missing %q in:\n%v", want, joined)
		}
	}
}

// TestExceptionHandlerWithoutLogger pins the nil-logger guard: a handler
// called before the logger exists must not panic.
func TestExceptionHandlerWithoutLogger(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Logger = nil
	a.ExceptionHandler("something broke")
}
