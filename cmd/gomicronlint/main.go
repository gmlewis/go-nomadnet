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

// Command gomicronlint checks a Nomad Network page tree for Micron constructs
// that fail silently.
//
// Micron is forgiving in the worst way: a construct it cannot recognize is not
// rejected, it is rendered as ordinary text, and a construct it reads as a
// container can swallow the rest of the page. A mistyped directive therefore
// does not look like a bug in the source -- it looks like a page that prints
// odd punctuation to its visitors, which is how the guestbook shipped a raw
// "`{:/page/hit-counter.wasm`0`quiet=1}" to the web: the partial directive sat at
// the end of a line of text, where Micron only tests for a partial at the START
// of a line, so the directive was never extracted, never fetched (the site
// counter silently stopped advancing), and the visitor read it verbatim.
//
// The linter renders every page through the same parser the client uses and
// reports the constructs that cannot have done what their author intended:
//
//   - a partial directive that does not start its line
//   - a partial directive that survives into the rendered text (a leak)
//   - an unterminated link or field, which renders literally
//   - a link with too many backtick components, which vanishes entirely
//   - an unbalanced `= literal toggle or `t table marker, which swallows the
//     rest of the page
//   - a "#anchor" that no heading or `:name on that page declares
//   - an "anchor=" link field naming an anchor the target page does not declare
//     (the field is how a Markdown "page.md#section" link is expressed, since a
//     Micron link has nowhere to carry a fragment)
//   - a relative ":/page/..." link whose target file does not exist
//
// A comment line, an odd formatting toggle and a literal backtick in the
// rendered text are reported too, but only with -v: each is often deliberate
// (a silent marker, an escape a reader cannot see, a page documenting Micron
// itself).
//
// Usage:
//
//	gomicronlint [-v] [path ...]
//
// Every path is a page file or a directory to walk for *.mu files; with no path
// the tree is the one the local node serves,
// ~/.nomadnetwork/storage/pages. A directory is also the root that relative
// ":/page/..." targets and cross-page anchors resolve against. The exit status
// is 0 when no defect was found and 1 when at least one was, so the command can
// be used directly as a pre-commit hook or a CI step.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// defaultTree is the page tree the local node serves, linted when no path is
// named on the command line.
var defaultTree = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".nomadnetwork", "storage", "pages")
}

func main() {
	log.SetFlags(0)

	verbose := flag.Bool("v", false,
		"also report notes: comment lines, odd formatting toggles, and literal backticks in the rendered text")
	flag.Usage = usage
	flag.Parse()

	defects, err := run(os.Stdout, flag.Args(), *verbose)
	if err != nil {
		log.Fatalf("gomicronlint: %v", err)
	}
	if defects > 0 {
		log.Fatalf("gomicronlint: %v defect(s) found", defects)
	}
}

// usage prints the one-screen help.
func usage() {
	out := flag.CommandLine.Output()
	_, _ = fmt.Fprintf(out, `usage: gomicronlint [-v] [path ...]

  path  a .mu page, or a directory to walk for *.mu pages (default: %v)

Reports Micron constructs that fail silently: a partial directive that does not
start its line, an unterminated link or field, an unbalanced literal or table
marker, an anchor no heading declares, a link whose target file does not exist.
Exit status is 1 when any defect was found.

`, defaultTree())
	flag.PrintDefaults()
}

// run lints the paths named by args, writes the report to w and returns the
// number of defects found. With no args it lints defaultTree. Every path is a
// page file or a directory; a directory is also the root that its pages'
// relative ":/page/..." targets and cross-page anchors resolve against.
func run(w io.Writer, args []string, verbose bool) (int, error) {
	if len(args) == 0 {
		tree := defaultTree()
		if tree == "" {
			return 0, fmt.Errorf("cannot determine the home directory; name a path")
		}
		args = []string{tree}
	}

	var findings []Finding
	pages := 0
	for _, arg := range args {
		found, count, err := scanPath(arg)
		if err != nil {
			return 0, err
		}
		findings = append(findings, found...)
		pages += count
	}

	defects, notes := 0, 0
	for _, f := range findings {
		if f.Defect() {
			defects++
		} else {
			notes++
		}
		if f.Defect() || verbose {
			f.writeTo(w)
		}
	}
	_, _ = fmt.Fprintf(w, "gomicronlint: %v page(s), %v defect(s), %v note(s)\n", pages, defects, notes)
	return defects, nil
}
