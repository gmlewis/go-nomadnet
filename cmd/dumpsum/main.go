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

// Command dumpsum analyzes Go runtime goroutine dumps — SIGQUIT dumps
// (GOTRACEBACK=all "SIGQUIT: quit" files), panic traces, or equivalent
// /debug/pprof/goroutine?debug=2 captures — and prints a forensic summary:
//
//   - total goroutine count and counts grouped by scheduler state;
//   - a role table mapping each goroutine to its first non-runtime frame,
//     so missing subsystem readers are obvious at a glance;
//   - callouts for [running]/[runnable] goroutines (the red-handed CPU
//     burners) and for [syscall] goroutines (potential instant-error spins);
//   - creation-site ("created by") histogram, which reconstructs lineage
//     such as reconnectLoop-spawned-by-failConn transitions;
//   - the highest live goroutine ID, which combined with process uptime
//     estimates lifetime spawn churn.
//
// Usage:
//
//	dumpsum [options] [file]
//
// The file is a saved dump; with no file (or "-"), dumpsum reads stdin. Feed
// it the stderr log a `kill -QUIT <pid>` produced under GOTRACEBACK=all, or a
// curl of http://127.0.0.1:6060/debug/pprof/goroutine?debug=2.
//
// Options:
//
//	-state S   print detail only for goroutines whose state contains S
//	           (e.g. running, syscall); repeatable
//	-created P filter creation sites by substring P
//	-json      emit machine-readable JSON instead of text
//	-v         list every goroutine's header and top frames
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

// goroutine holds the parsed essence of one "goroutine N ... [state]:" block.
type goroutine struct {
	id       int
	header   string   // full original header line
	state    string   // bracketed state field, e.g. "[IO wait, 562 minutes]"
	topFuncs []string // non-runtime user frames, most recent first
	created  string   // "created by ..." line, if any
}

// parseDump splits a dump into goroutine blocks. It is deliberately tolerant:
// register dumps at EOF, "rax rbx" lines, and address suffixes are ignored.
func parseDump(r io.Reader) ([]goroutine, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var out []goroutine
	var cur *goroutine
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "goroutine ") && strings.Contains(line, "[") {
			flush()
			g := goroutine{header: line}
			if i := strings.Index(line, "["); i >= 0 {
				if j := strings.LastIndex(line, "]"); j > i {
					g.state = line[i+1 : j]
				}
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if id, err := strconv.Atoi(fields[1]); err == nil {
					g.id = id
				}
			}
			cur = &g
			continue
		}
		if cur == nil {
			continue // preamble, register dump, blank lines
		}
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "created by "):
			cur.created = strings.TrimSpace(strings.TrimSuffix(line, ":"))
		case strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "main.") ||
			strings.HasPrefix(line, "github.com/") || strings.HasPrefix(line, "net.") ||
			strings.HasPrefix(line, "os/signal.") || strings.HasPrefix(line, "internal/"):
			fn := frameFunc(line)
			if fn != "" && !isRuntimeFrame(fn) {
				cur.topFuncs = append(cur.topFuncs, fn)
			}
		}
	}
	flush()
	return out, nil
}

// frameFunc extracts the function symbol from a stack frame line such as
// "github.com/x/y.(*T).Foo(0x1, ...)". The argument list is always the last
// parenthesized group on the line, so cutting there keeps receiver parens
// such as "(*T)" intact.
func frameFunc(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "/") {
		return ""
	}
	if i := strings.LastIndex(line, "("); i > 0 {
		return strings.TrimRight(line[:i], " \t")
	}
	if i := strings.IndexAny(line, " \t"); i > 0 {
		return line[:i]
	}
	return line
}

func isRuntimeFrame(fn string) bool {
	for _, p := range []string{"runtime.", "internal/runtime/", "internal/poll.", "sync.", "time.Sleep"} {
		if strings.HasPrefix(fn, p) {
			return true
		}
	}
	return false
}

// role returns the best single-line description of what a goroutine does: the
// newest user frame, falling back to the creation site, then to the state.
func (g goroutine) role() string {
	if len(g.topFuncs) > 0 {
		return g.topFuncs[0]
	}
	if g.created != "" {
		return g.created + " (exited-frame)"
	}
	return "?"
}

type summary struct {
	Total     int            `json:"total"`
	MaxID     int            `json:"max_goroutine_id"`
	States    map[string]int `json:"states"`
	Roles     map[string]int `json:"roles"`
	CreatedBy map[string]int `json:"created_by"`
	Running   []goroutine    `json:"running,omitempty"`
	Runnable  []goroutine    `json:"runnable,omitempty"`
	Syscalls  []goroutine    `json:"syscalls,omitempty"`
}

func summarize(gs []goroutine) summary {
	s := summary{Total: len(gs), States: map[string]int{}, Roles: map[string]int{}, CreatedBy: map[string]int{}}
	for _, g := range gs {
		if g.id > s.MaxID {
			s.MaxID = g.id
		}
		key := g.state
		if key == "" {
			key = "?"
		}
		// Collapse per-goroutine noise like ages into the bare state name.
		if i := strings.Index(key, ","); i >= 0 {
			key = key[:i]
		}
		s.States[key]++
		s.Roles[g.role()]++
		if g.created != "" {
			s.CreatedBy[g.created]++
		}
		switch {
		case strings.HasPrefix(g.state, "running"):
			s.Running = append(s.Running, g)
		case strings.HasPrefix(g.state, "runnable"):
			s.Runnable = append(s.Runnable, g)
		case strings.HasPrefix(g.state, "syscall"):
			s.Syscalls = append(s.Syscalls, g)
		}
	}
	sort.Slice(s.Running, func(i, j int) bool { return s.Running[i].id < s.Running[j].id })
	sort.Slice(s.Runnable, func(i, j int) bool { return s.Runnable[i].id < s.Runnable[j].id })
	sort.Slice(s.Syscalls, func(i, j int) bool { return s.Syscalls[i].id < s.Syscalls[j].id })
	return s
}

func sortedPairs(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] || (m[keys[i]] == m[keys[j]] && keys[i] < keys[j]) })
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%5d  %s", m[k], k))
	}
	return out
}

func main() {
	log.SetFlags(0)
	states := flag.String("state", "", "comma-separated state substrings to detail")
	created := flag.String("created", "", "substring filter for created-by sites")
	jsonOut := flag.Bool("json", false, "emit JSON instead of text")
	verbose := flag.Bool("v", false, "list every goroutine")
	flag.Usage = func() { fmt.Fprintln(os.Stderr, "usage: dumpsum [-state S,S] [-created P] [-json] [-v] [file|-]") }
	flag.Parse()

	var r io.Reader = os.Stdin
	if path := flag.Arg(0); path != "" && path != "-" {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dumpsum: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = f.Close() }()
		r = f
	}

	gs, err := parseDump(r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dumpsum: %v\n", err)
		os.Exit(1)
	}
	if len(gs) == 0 {
		fmt.Fprintln(os.Stderr, "dumpsum: no goroutine blocks found in input")
		os.Exit(1)
	}

	s := summarize(gs)
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(s); err != nil {
			fmt.Fprintf(os.Stderr, "dumpsum: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("goroutines: %d   max id: %d\n", s.Total, s.MaxID)
	fmt.Println("\nstates:")
	for _, l := range sortedPairs(s.States) {
		fmt.Println(l)
	}
	fmt.Println("\nroles (newest user frame):")
	for _, l := range sortedPairs(s.Roles) {
		fmt.Println(l)
	}
	if len(s.CreatedBy) > 0 {
		fmt.Println("\ncreated by:")
		for _, l := range sortedPairs(s.CreatedBy) {
			fmt.Println(l)
		}
	}
	for name, group := range map[string][]goroutine{"RUNNING": s.Running, "RUNNABLE": s.Runnable, "SYSCALL": s.Syscalls} {
		if len(group) == 0 {
			continue
		}
		fmt.Printf("\n%s (%d):\n", name, len(group))
		for _, g := range group {
			fmt.Printf("  #%d %s -> %s\n", g.id, g.state, g.role())
		}
	}
	if *verbose || *states != "" || *created != "" {
		want := map[string]bool{}
		for w := range strings.SplitSeq(*states, ",") {
			if w = strings.TrimSpace(w); w != "" {
				want[strings.ToLower(w)] = true
			}
		}
		fmt.Println("\ndetail:")
		for _, g := range gs {
			if *verbose || matchState(g.state, want) || (*created != "" && strings.Contains(g.created, *created)) {
				fmt.Printf("#%d %s\n", g.id, g.header[strings.Index(g.header, "["):])
				for _, fn := range g.topFuncs {
					fmt.Printf("    %s\n", fn)
				}
				if g.created != "" {
					fmt.Printf("    %s\n", g.created)
				}
			}
		}
	}
}

func matchState(state string, want map[string]bool) bool {
	if len(want) == 0 {
		return false
	}
	lower := strings.ToLower(state)
	for w := range want {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}
