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
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tempDir is a short-path temp directory: on macOS a t.TempDir() path is long
// enough to break Unix domain sockets, so the base is /tmp there (see AGENTS.md).
func tempDir(t *testing.T) string {
	t.Helper()
	base := ""
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "gomicronlint-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// kindsOf counts the findings by kind, so a test can pin what was reported
// without depending on the order the report happens to use.
func kindsOf(findings []Finding) map[Kind]int {
	out := map[Kind]int{}
	for _, f := range findings {
		out[f.Kind]++
	}
	return out
}

// TestScanMarkup pins what one page can be told about itself: each construct
// that Micron silently degrades is reported, and a page that uses the same
// constructs correctly is reported clean.
func TestScanMarkup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		markup string
		want   map[Kind]int
	}{
		{
			name:   "clean page",
			markup: ">Title\n\nHello `!bold`! world, see `[Docs`:/page/docs/index.mu].\n\n`{:/page/hit-counter.wasm`0`quiet=1}\n",
			want:   map[Kind]int{},
		},
		{
			name:   "partial at the end of a line",
			markup: "Visited 3 times.`{:/page/hit-counter.wasm`0`quiet=1}\n",
			want:   map[Kind]int{KindPartialOffLine: 1, KindLeakedDirective: 1},
		},
		{
			name:   "partial after a leading space",
			markup: "text\n `{:/page/counter.wasm`0}\n",
			want:   map[Kind]int{KindPartialOffLine: 1, KindLeakedDirective: 1},
		},
		{
			name:   "escaped directive is documentation, not a defect",
			markup: "A partial is written \\`{URL`REFRESH} at the start of its line.\n",
			// The escape is deliberate, so nothing here is a defect; the literal
			// backtick it renders is reported as a note, which -v shows.
			want: map[Kind]int{KindLiteralBacktick: 1},
		},
		{
			name:   "unterminated link",
			markup: "See `[the docs`:/page/docs/index.mu\n",
			want:   map[Kind]int{KindUnterminatedLink: 1},
		},
		{
			name:   "unterminated field",
			markup: "Name: `<name`glenn\n",
			want:   map[Kind]int{KindUnterminatedField: 1},
		},
		{
			name:   "link with too many components",
			markup: "`[label`:/page/x.mu`a`b]\n",
			want:   map[Kind]int{KindLinkComponents: 1},
		},
		{
			name:   "unbalanced literal block",
			markup: ">Title\n`=\nthis is literal\nand so is everything below\n",
			want:   map[Kind]int{KindUnbalancedLiteral: 1},
		},
		{
			name:   "unbalanced table marker",
			markup: "`t\n| a | b |\n| c | d |\n",
			want:   map[Kind]int{KindUnbalancedTable: 1},
		},
		{
			name:   "table with one row renders nothing",
			markup: "`t\n| a | b |\n`t\n",
			want:   map[Kind]int{KindUnbalancedTable: 1},
		},
		{
			name:   "in-page anchor no heading declares",
			markup: ">Title\n\nSee `[below`#nowhere].\n",
			want:   map[Kind]int{KindUnknownAnchor: 1},
		},
		{
			name:   "in-page anchor a heading declares",
			markup: ">Title\n\nSee `[below`#the-section].\n\n>>The Section\n",
			want:   map[Kind]int{},
		},
		{
			name:   "declared anchor name",
			markup: "`:marker text\n\nSee `[here`#marker].\n",
			want:   map[Kind]int{},
		},
		{
			name:   "bare hash is not a defect",
			markup: ">Title\n\nContinue `[on`#].\n",
			want:   map[Kind]int{},
		},
		{
			name:   "comment line and odd toggle are notes",
			markup: ">Title\n# a silent marker\n`!never closed\n",
			want:   map[Kind]int{KindCommentLine: 1, KindOddToggle: 1},
		},
		{
			name:   "literal backtick is a note",
			markup: "Run \\`ls -l\\` to list.\n",
			want:   map[Kind]int{KindLiteralBacktick: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := kindsOf(ScanMarkup("page.mu", tt.markup))
			for kind, want := range tt.want {
				if got[kind] != want {
					t.Errorf("kind %v reported %v time(s), want %v\nreport: %v", kind, got[kind], want, reportOf(ScanMarkup("page.mu", tt.markup)))
				}
			}
			for kind, count := range got {
				if _, wanted := tt.want[kind]; !wanted {
					t.Errorf("unexpected kind %v reported %v time(s)\nreport: %v", kind, count, reportOf(ScanMarkup("page.mu", tt.markup)))
				}
			}
		})
	}
}

// TestScanTreeLinks pins the checks that need the whole tree: a relative page
// target that does not exist, and an "anchor=" field naming an anchor the target
// page does not declare. The second is the defect that made the generated
// documentation's cross-page section links dead.
func TestScanTreeLinks(t *testing.T) {
	t.Parallel()

	root := tempDir(t)
	writePage(t, root, "index.mu", strings.Join([]string{
		">Index",
		"",
		"`[Good`:/page/docs/tool.mu`anchor=the-section]",
		"`[Bad anchor`:/page/docs/tool.mu`anchor=no-such-section]",
		"`[Missing page`:/page/docs/gone.mu]",
		"",
	}, "\n"))
	writePage(t, root, "docs/tool.mu", strings.Join([]string{
		">Tool",
		"",
		">>The Section",
		"body",
		"",
	}, "\n"))

	findings, pages, err := ScanTree(root)
	if err != nil {
		t.Fatalf("ScanTree: %v", err)
	}
	if pages != 2 {
		t.Errorf("pages read = %v, want 2", pages)
	}
	got := kindsOf(findings)
	if got[KindDeadAnchorField] != 1 {
		t.Errorf("dead-anchor-field reported %v time(s), want 1\nreport: %v", got[KindDeadAnchorField], reportOf(findings))
	}
	if got[KindMissingPage] != 1 {
		t.Errorf("missing-page reported %v time(s), want 1\nreport: %v", got[KindMissingPage], reportOf(findings))
	}
	for _, f := range findings {
		if f.Kind == KindDeadAnchorField && !strings.Contains(f.Detail, "no-such-section") {
			t.Errorf("dead-anchor-field detail = %q, want it to name the missing anchor", f.Detail)
		}
	}
}

// TestRunExitStatus pins the contract a pre-commit hook depends on: the exit
// status is decided by defects alone, so a page that only draws a note is
// publishable, and one that would confuse a reader is not.
func TestRunExitStatus(t *testing.T) {
	t.Parallel()

	t.Run("clean tree passes", func(t *testing.T) {
		t.Parallel()

		root := tempDir(t)
		writePage(t, root, "index.mu", ">Index\n\n`[Tool`:/page/tool.mu]\n")
		writePage(t, root, "tool.mu", ">Tool\n\nbody\n")

		var out bytes.Buffer
		defects, err := run(&out, []string{root}, false)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if defects != 0 {
			t.Errorf("defects = %v, want 0\nreport: %v", defects, out.String())
		}
	})

	t.Run("note only passes", func(t *testing.T) {
		t.Parallel()

		root := tempDir(t)
		writePage(t, root, "index.mu", "# a silent marker\n>Index\n\nbody\n")

		var out bytes.Buffer
		defects, err := run(&out, []string{root}, false)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if defects != 0 {
			t.Errorf("defects = %v, want 0 (a comment line is a note)\nreport: %v", defects, out.String())
		}
		if strings.Contains(out.String(), "comment-line") {
			t.Errorf("report names a note without -v: %v", out.String())
		}
	})

	t.Run("defect fails and is reported", func(t *testing.T) {
		t.Parallel()

		root := tempDir(t)
		writePage(t, root, "index.mu", "Visited 3 times.`{/page/counter.wasm`0`quiet=1}\n")

		var out bytes.Buffer
		defects, err := run(&out, []string{root}, false)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if defects == 0 {
			t.Fatalf("defects = 0, want > 0 for the leaked directive\nreport: %v", out.String())
		}
		if !strings.Contains(out.String(), "partial-off-line") {
			t.Errorf("report does not name the defect: %v", out.String())
		}
	})

	t.Run("verbose reports notes", func(t *testing.T) {
		t.Parallel()

		root := tempDir(t)
		writePage(t, root, "index.mu", "# a silent marker\n>Index\n\nbody\n")

		var out bytes.Buffer
		if _, err := run(&out, []string{root}, true); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out.String(), "comment-line") {
			t.Errorf("verbose report does not name the note: %v", out.String())
		}
	})
}

// writePage writes one page into a scanned tree, creating its directory.
func writePage(t *testing.T, root, rel, markup string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(markup), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// reportOf renders findings the way the command does, for a failure message.
func reportOf(findings []Finding) string {
	var b strings.Builder
	for _, f := range findings {
		f.writeTo(&b)
	}
	return b.String()
}
