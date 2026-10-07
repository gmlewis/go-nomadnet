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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// Kind classifies a finding. The names are the report's own vocabulary: they say
// what Micron does with the construct, which is what a page author has to reason
// about -- not what the linter's code happens to call the case.
type Kind string

const (
	// KindPartialOffLine is a partial directive that does not start its line.
	// Micron tests for a partial only at the beginning of a line, so the
	// directive is ordinary text: the visitor reads it and the partial is never
	// fetched (the guestbook's quiet site counter shipped this way).
	KindPartialOffLine Kind = "partial-off-line"
	// KindLeakedDirective is a partial directive that survived into the rendered
	// text, which is the same defect seen from the reader's side.
	KindLeakedDirective Kind = "leaked-directive"
	// KindUnterminatedLink is a link with no closing bracket, which renders
	// literally.
	KindUnterminatedLink Kind = "unterminated-link"
	// KindUnterminatedField is a field with no closing angle bracket, which
	// renders literally.
	KindUnterminatedField Kind = "unterminated-field"
	// KindLinkComponents is a link with more than three backtick components,
	// which renders as nothing at all.
	KindLinkComponents Kind = "link-too-many-components"
	// KindUnbalancedLiteral is a page left inside a `= literal block, so
	// everything after the last toggle is rendered raw.
	KindUnbalancedLiteral Kind = "unbalanced-literal"
	// KindUnbalancedTable is a page left inside a `t table block, so the rest of
	// the page is swallowed by the table (and a table closing with fewer than two
	// rows renders nothing).
	KindUnbalancedTable Kind = "unbalanced-table"
	// KindUnknownAnchor is a "#anchor" link naming an anchor the page does not
	// declare, so the jump does nothing.
	KindUnknownAnchor Kind = "unknown-anchor"
	// KindDeadAnchorField is an "anchor=<name>" link field naming an anchor the
	// target page does not declare. The field is how a cross-page section link is
	// expressed, because a Micron link has nowhere to carry a fragment.
	KindDeadAnchorField Kind = "dead-anchor-field"
	// KindMissingPage is a relative ":/page/..." link whose target file does not
	// exist under the scanned root.
	KindMissingPage Kind = "missing-page"
	// KindCommentLine is a line starting with "#", which renders as nothing.
	KindCommentLine Kind = "comment-line"
	// KindOddToggle is a formatting toggle that is never closed, so it runs to
	// the end of the page.
	KindOddToggle Kind = "odd-toggle"
	// KindLiteralBacktick is a backtick in the rendered text: usually a
	// deliberate escape, occasionally a construct that did not parse.
	KindLiteralBacktick Kind = "literal-backtick"
)

// Defect reports whether a finding is a defect -- a construct that cannot be
// doing what its author intended -- rather than an observation that may be
// deliberate. Only defects decide the exit status.
func (k Kind) Defect() bool {
	switch k {
	case KindCommentLine, KindOddToggle, KindLiteralBacktick:
		return false
	default:
		return true
	}
}

// Finding is one thing the linter noticed about one page.
type Finding struct {
	// Path is the page path as the report spells it: relative to the scanned
	// root for a directory scan, as given for a single file.
	Path string
	// Line is the 1-based source line the finding is on, or 0 when the finding
	// is about the page as a whole or about the rendered text.
	Line int
	// Kind classifies the finding.
	Kind Kind
	// Detail says what Micron does with the construct.
	Detail string
	// Text is the offending source or rendered line, when there is one.
	Text string
}

// Defect reports whether f is a defect rather than an observation.
func (f Finding) Defect() bool { return f.Kind.Defect() }

// String renders the report line, without the offending source line under it.
func (f Finding) String() string {
	where := f.Path
	if f.Line > 0 {
		where = fmt.Sprintf("%v:%v", f.Path, f.Line)
	}
	return fmt.Sprintf("%v: %v: %v", where, f.Kind, f.Detail)
}

// writeTo writes the finding, and the line it is about indented beneath it.
func (f Finding) writeTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, f.String())
	if f.Text != "" {
		_, _ = fmt.Fprintf(w, "    %v\n", f.Text)
	}
}

// anchorsOf returns the anchor names a page exposes: every heading slug and
// every explicit `:name declaration, exactly as the client's anchor map sees
// them (micron.BuildAnchorMap over the rendered lines).
func anchorsOf(markup string) map[string]bool {
	out := map[string]bool{}
	for name := range micron.BuildAnchorMap(micron.RenderToStyledLines(markup, micron.ThemeDark)) {
		out[name] = true
	}
	return out
}

// renderedLines returns the text of each rendered line, which is what a visitor
// reads: a construct that did not parse is visible here even though it is
// invisible in the source. Spans are concatenated because styling carries no
// text of its own.
func renderedLines(markup string) []string {
	lines := micron.RenderToStyledLines(markup, micron.ThemeDark)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		var b strings.Builder
		for _, span := range line.Spans {
			b.WriteString(span.Text)
		}
		out = append(out, b.String())
	}
	return out
}

// escapedAt reports whether the character at idx is backslash-escaped, so a
// page that documents Micron ("\`{") is not reported as a broken directive.
func escapedAt(line string, idx int) bool {
	backslashes := 0
	for i := idx - 1; i >= 0 && line[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

// ScanMarkup reports every finding that one page's markup shows on its own.
// name is the page path the report should spell. Cross-page checks (a link's
// target file, an anchor on the target page) need the whole tree and live in
// scanTree instead.
func ScanMarkup(name, markup string) []Finding {
	var out []Finding
	add := func(line int, kind Kind, detail, text string) {
		out = append(out, Finding{Path: name, Line: line, Kind: kind, Detail: detail, Text: text})
	}

	// A `= literal toggle and a `t table marker both make Micron stop parsing
	// what follows, so every check below runs only outside them.
	literal := false
	tableOpen := false
	tableRows := 0
	toggles := map[string]int{}

	for i, line := range strings.Split(markup, "\n") {
		lineNo := i + 1
		if line == "`=" {
			literal = !literal
			continue
		}
		if literal {
			continue
		}
		if strings.HasPrefix(line, "`t") {
			if tableOpen && tableRows < 2 {
				add(lineNo, KindUnbalancedTable,
					"table closes before it has two rows, so it renders as nothing", line)
			}
			tableOpen = !tableOpen
			tableRows = 0
			continue
		}
		if tableOpen {
			if strings.TrimSpace(line) != "" {
				tableRows++
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			add(lineNo, KindCommentLine, "comment line: renders as nothing", line)
		}
		if idx := strings.Index(line, "`{"); idx > 0 && !escapedAt(line, idx) {
			add(lineNo, KindPartialOffLine,
				"partial directive does not start its line: Micron treats it as ordinary text, so the visitor reads the directive and the partial is never fetched",
				line)
		}
		for _, c := range []struct {
			open, close string
			kind        Kind
			what        string
		}{
			{"`[", "]", KindUnterminatedLink, "link"},
			{"`<", ">", KindUnterminatedField, "field"},
		} {
			for from := 0; ; {
				idx := strings.Index(line[from:], c.open)
				if idx < 0 {
					break
				}
				idx += from
				rest := line[idx+len(c.open):]
				end := strings.Index(rest, c.close)
				if end < 0 {
					add(lineNo, c.kind, "unterminated "+c.what+": renders literally", line)
					break
				}
				if c.what == "link" && strings.Count(rest[:end], "`") > 2 {
					add(lineNo, KindLinkComponents,
						"link with more than three backtick components: renders as nothing", line)
				}
				from = idx + len(c.open)
			}
		}
		for _, toggle := range []string{"`!", "`_", "`*"} {
			toggles[toggle] += strings.Count(line, toggle)
		}
	}

	if literal {
		out = append(out, Finding{Path: name, Kind: KindUnbalancedLiteral,
			Detail: "`= literal block is never closed: everything after it renders raw"})
	}
	if tableOpen {
		out = append(out, Finding{Path: name, Kind: KindUnbalancedTable,
			Detail: "`t table block is never closed: the rest of the page is swallowed by the table"})
	}
	var odd []string
	for toggle, count := range toggles {
		if count%2 == 1 {
			odd = append(odd, fmt.Sprintf("%v x%v", toggle, count))
		}
	}
	sort.Strings(odd)
	for _, o := range odd {
		out = append(out, Finding{Path: name, Kind: KindOddToggle,
			Detail: "formatting toggle " + o + " is never closed: it runs to the end of the page"})
	}

	// A "#anchor" link only moves a reader who is already on the page, so an
	// anchor this page does not declare is a dead link. A bare "#" is not: it
	// means "the next heading below".
	anchors := anchorsOf(markup)
	for _, node := range micron.Parse(markup) {
		if node.Type != micron.NodeLink {
			continue
		}
		target, ok := strings.CutPrefix(node.LinkURL, "#")
		if !ok || target == "" {
			continue
		}
		if !anchors[target] {
			out = append(out, Finding{Path: name, Kind: KindUnknownAnchor,
				Detail: fmt.Sprintf("link to #%v names no heading slug or `:name on this page", target)})
		}
	}

	// What the visitor actually reads.
	for _, text := range renderedLines(markup) {
		if strings.Contains(text, "{:") {
			out = append(out, Finding{Path: name, Kind: KindLeakedDirective,
				Detail: "a partial directive reached the rendered text: it did not parse as a partial where it sits",
				Text:   text})
		}
		if strings.Contains(text, "`") {
			out = append(out, Finding{Path: name, Kind: KindLiteralBacktick,
				Detail: "rendered text carries a literal backtick: an escape the reader cannot see, or a construct that did not parse",
				Text:   text})
		}
	}
	return out
}

// scanPath lints one path: a directory is walked for *.mu pages and is the root
// its pages' relative targets resolve against; a file is linted on its own, with
// its own directory as the root. It returns the findings and the number of pages
// read.
func scanPath(path string) ([]Finding, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	if info.IsDir() {
		return ScanTree(path)
	}
	markup, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	return ScanMarkup(filepath.Base(path), string(markup)), 1, nil
}

// ScanTree lints every *.mu file under root, in lexical order, and adds the
// checks that need the whole tree: a relative ":/page/..." target must exist,
// and an "anchor=" field must name an anchor the target page declares. It
// returns the findings and the number of pages read.
func ScanTree(root string) ([]Finding, int, error) {
	var names []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".mu") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(names)

	// anchors caches each target page's anchor set, so a page that many others
	// link into is rendered once rather than once per link.
	anchors := map[string]map[string]bool{}
	targetAnchors := func(rel string) map[string]bool {
		if set, ok := anchors[rel]; ok {
			return set
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			anchors[rel] = nil
			return nil
		}
		set := anchorsOf(string(data))
		anchors[rel] = set
		return set
	}

	var out []Finding
	for _, rel := range names {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, 0, err
		}
		markup := string(data)
		out = append(out, ScanMarkup(rel, markup)...)

		for _, node := range micron.Parse(markup) {
			if node.Type != micron.NodeLink {
				continue
			}
			target, ok := strings.CutPrefix(node.LinkURL, ":/page/")
			if !ok {
				continue
			}
			full := filepath.Join(root, filepath.FromSlash(target))
			info, err := os.Stat(full)
			if err != nil || info.IsDir() {
				out = append(out, Finding{Path: rel, Kind: KindMissingPage,
					Detail: fmt.Sprintf("link target :/page/%v does not exist under the scanned tree", target)})
				continue
			}
			for field := range strings.SplitSeq(node.LinkFields, "|") {
				name, ok := strings.CutPrefix(field, "anchor=")
				if !ok || name == "" {
					continue
				}
				if !targetAnchors(target)[name] {
					out = append(out, Finding{Path: rel, Kind: KindDeadAnchorField,
						Detail: fmt.Sprintf("link to :/page/%v asks for anchor %q, which no heading slug or `:name on that page declares", target, name)})
				}
			}
		}
	}
	return out, len(names), nil
}
