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

package main

import (
	"log"
	"net"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux
)

// startPProf starts an HTTP server exposing net/http/pprof endpoints at addr
// (e.g. "127.0.0.1:6060"). It is strictly opt-in via the -pprof-addr flag: when
// addr is empty it is a no-op with zero overhead (no goroutine, no listener,
// no pprof sampling).
//
// It exists so a REAL-terminal CPU profile can be captured while interacting
// with the TUI. The headless tcell.SimulationScreen used by the go-test
// benchmarks (tui/perf-*_test.go) measures the Go-side render/CPU cost but
// cannot reproduce the real-terminal flush/cursor costs that drive the visible
// cursor flicker and scroll sluggishness — only profiling the live binary can.
//
//	gonomadnet -textui -pprof-addr 127.0.0.1:6060
//	# reproduce the flicker/scroll in the terminal, then in another shell:
//	go tool pprof -http :8080 http://127.0.0.1:6060/debug/pprof/profile?seconds=30
//
// Expected hot symbols while scrolling a Guide page: WordWrap, wrappedRowCount,
// ScrollBar.Draw, TextView.Draw. While moving the mouse over the Network page:
// urwidColumns.MouseHandler, Application.SetFocus, Application.draw. While
// idle: Application.draw firing on every background QueueUpdateDraw tick.
func startPProf(addr string) {
	if addr == "" {
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("pprof: listen %s: %v", addr, err)
		return
	}
	go func() {
		log.Printf("pprof: serving on http://%s/debug/pprof/", ln.Addr())
		if err := http.Serve(ln, nil); err != nil {
			log.Printf("pprof: serve %s: %v", addr, err)
		}
	}()
}
