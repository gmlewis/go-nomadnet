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

package micron

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// alignName mirrors cmd/view-mu's alignName so the test can serialize a
// StyledLine's Alignment into the parity schema's string form.
func alignName(a Alignment) string {
	switch a {
	case AlignCenter:
		return "center"
	case AlignRight:
		return "right"
	default:
		return "left"
	}
}

// parityDoc mirrors the logical-line JSON schema emitted by both cmd/view-mu
// -json and tooling/render-mu.py --json, so the Go renderer and the Python
// MicronParser can be diffed field by field with no network and no tmux.
type parityDoc struct {
	Source string       `json:"source"`
	Theme  string       `json:"theme"`
	Lines  []parityLine `json:"lines"`
}

type parityLine struct {
	Index        int          `json:"index"`
	Align        string       `json:"align"`
	Indent       int          `json:"indent"`
	HeadingLevel int          `json:"heading_level"`
	Divider      bool         `json:"divider"`
	DividerChar  string       `json:"divider_char,omitempty"`
	DividerRight int          `json:"divider_right,omitempty"`
	Anchor       string       `json:"anchor,omitempty"`
	Spans        []paritySpan `json:"spans"`
}

type paritySpan struct {
	Text      string       `json:"text"`
	FG        string       `json:"fg"`
	BG        string       `json:"bg"`
	Bold      bool         `json:"bold"`
	Underline bool         `json:"underline"`
	Italic    bool         `json:"italic"`
	Link      *parityLink  `json:"link,omitempty"`
	Field     *parityField `json:"field,omitempty"`
}

type parityLink struct {
	Label  string `json:"label"`
	URL    string `json:"url"`
	Fields string `json:"fields"`
}

type parityField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Data       string `json:"data"`
	Value      string `json:"value"`
	Width      int    `json:"width"`
	Masked     bool   `json:"masked"`
	Prechecked bool   `json:"prechecked"`
}

// renderGoDoc renders markup through the Go micron renderer and marshals the
// styled lines to the shared parity JSON schema.
func renderGoDoc(t *testing.T, markup []byte, source string) parityDoc {
	t.Helper()
	lines := RenderToStyledLines(string(markup), ThemeDark)
	doc := parityDoc{Source: source, Theme: "dark", Lines: make([]parityLine, 0, len(lines))}
	for i, line := range lines {
		jl := parityLine{Index: i, Align: "left", Spans: []paritySpan{}}
		if line != nil {
			jl = parityLine{
				Index:        i,
				Align:        alignName(line.Align),
				Indent:       line.Indent,
				HeadingLevel: line.HeadingLevel,
				Divider:      line.Divider,
				DividerChar:  line.DividerChar,
				DividerRight: line.DividerRight,
				Anchor:       line.Anchor,
				Spans:        make([]paritySpan, 0, len(line.Spans)),
			}
			for _, sp := range line.Spans {
				js := paritySpan{
					Text: sp.Text, FG: sp.FG, BG: sp.BG,
					Bold: sp.Bold, Underline: sp.Underline, Italic: sp.Italic,
				}
				if sp.Link != nil {
					js.Link = &parityLink{Label: sp.Link.Label, URL: sp.Link.URL, Fields: sp.Link.Fields}
				}
				if sp.Field != nil {
					js.Field = &parityField{
						Name: sp.Field.Name, Type: sp.Field.Type, Data: sp.Field.Data,
						Value: sp.Field.Value, Width: sp.Field.Width, Masked: sp.Field.Masked,
						Prechecked: sp.Field.Prechecked,
					}
				}
				jl.Spans = append(jl.Spans, js)
			}
		}
		doc.Lines = append(doc.Lines, jl)
	}
	return doc
}

// runPythonRender feeds markup to tooling/render-mu.py - --json (the headless
// Python MicronParser renderer) and decodes the parity JSON it emits. Skipped,
// not failed, when the nomadnet-capable Python interpreter is unavailable.
func runPythonRender(t *testing.T, scriptPath string, markup []byte, source string) parityDoc {
	t.Helper()
	testutils.SkipIfNoPythonNomadnet(t)
	cmd := exec.Command(testutils.PythonNomadnetExe(), scriptPath, "-", "--json", "--source", source)
	cmd.Stdin = bytes.NewReader(markup)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("render-mu.py failed: %v\nstderr:\n%s", err, stderr.String())
	}
	var doc parityDoc
	if err := json.Unmarshal(stdout, &doc); err != nil {
		t.Fatalf("decode render-mu.py output: %v\nstderr:\n%s\nraw:\n%s", err, stderr.String(), stdout)
	}
	return doc
}

// compareParityDocs returns a sorted list of human-readable diff signatures
// between the Go and Python parity documents.
func compareParityDocs(goDoc, pyDoc parityDoc) []string {
	var diffs []string
	gl, pl := goDoc.Lines, pyDoc.Lines
	if len(gl) != len(pl) {
		diffs = append(diffs, "line_count: go=%d py=%d")
	}
	n := max(len(gl), len(pl))
	for i := range n {
		var g, p *parityLine
		if i < len(gl) {
			g = &gl[i]
		}
		if i < len(pl) {
			p = &pl[i]
		}
		if g == nil {
			diffs = append(diffs, "line %d: missing_in_go")
			continue
		}
		if p == nil {
			diffs = append(diffs, "line %d: missing_in_py")
			continue
		}
		if g.Align != p.Align {
			diffs = append(diffs, "line %d: align go=%s py=%s")
		}
		if g.Indent != p.Indent {
			diffs = append(diffs, "line %d: indent go=%d py=%d")
		}
		if g.HeadingLevel != p.HeadingLevel {
			diffs = append(diffs, "line %d: heading_level go=%d py=%d")
		}
		if g.Divider != p.Divider {
			diffs = append(diffs, "line %d: divider go=%v py=%v")
		}
		if g.Anchor != p.Anchor {
			diffs = append(diffs, "line %d: anchor go=%q py=%q")
		}
		if g.DividerChar != p.DividerChar {
			diffs = append(diffs, "line %d: divider_char go=%q py=%q")
		}
		if len(g.Spans) != len(p.Spans) {
			diffs = append(diffs, "line %d: span_count go=%d py=%d")
		}
		for j := range min(len(g.Spans), len(p.Spans)) {
			gs, ps := g.Spans[j], p.Spans[j]
			if gs.Text != ps.Text {
				diffs = append(diffs, "line %d span %d: text go=%q py=%q")
			}
			if gs.FG != ps.FG {
				diffs = append(diffs, "line %d span %d: fg go=%q py=%q")
			}
			if gs.BG != ps.BG {
				diffs = append(diffs, "line %d span %d: bg go=%q py=%q")
			}
			if gs.Bold != ps.Bold || gs.Underline != ps.Underline || gs.Italic != ps.Italic {
				diffs = append(diffs, "line %d span %d: attrs go=%v/%v/%v py=%v/%v/%v")
			}
			if (gs.Link == nil) != (ps.Link == nil) {
				diffs = append(diffs, "line %d span %d: link presence go=%v py=%v")
			} else if gs.Link != nil {
				if gs.Link.URL != ps.Link.URL {
					diffs = append(diffs, "line %d span %d: link.url go=%q py=%q")
				}
			}
			if (gs.Field == nil) != (ps.Field == nil) {
				diffs = append(diffs, "line %d span %d: field presence go=%v py=%v")
			}
		}
	}
	return diffs
}

// renderMuScriptPath is the path to the headless Python renderer, resolved from
// the test working directory (the micron package dir) up to the repo root.
var renderMuScriptPath = filepath.Join("..", "..", "tooling", "render-mu.py")

// acceptedParityDiffs lists known, reviewed Go-vs-Python divergences per
// fixture that the regression test does NOT treat as failures. Each entry must
// carry a one-line reason so a future reader knows whether to fix it or leave
// it. A diff that is NOT listed here fails the test, so new regressions are
// caught immediately.
var acceptedParityDiffs = map[string]map[string]string{
	"retibooks-index.mu": {
		"line_count: go=%d py=%d": "Go RenderToStyledLines does not strip trailing whitespace; Python strip_modifiers does, so Go emits one extra trailing blank line.",
		"line %d: missing_in_py":  "Consequence of the trailing-blank-line divergence (Go has one more line than Python).",
	},
}

// TestRenderParityFixtures diffs the Go micron renderer against the Python
// MicronParser for every checked-in .mu fixture under testdata/parity. Expected
// output is derived fresh from Python on every run (no stale golden files), so
// a Python nomadnet update or a Go renderer change surfaces immediately.
func TestRenderParityFixtures(t *testing.T) {
	// Resolve the absolute script path once and skip the whole table when the
	// renderer script is not present (e.g. a stripped export without tooling).
	scriptAbs, err := filepath.Abs(renderMuScriptPath)
	if err != nil || !fileExists(scriptAbs) {
		t.Skipf("skipping parity fixtures: render-mu.py not found at %s", renderMuScriptPath)
	}
	fixtures, err := filepath.Glob(filepath.Join("testdata", "parity", "*.mu"))
	if err != nil || len(fixtures) == 0 {
		t.Skipf("skipping parity fixtures: no .mu fixtures in testdata/parity (err=%v)", err)
	}
	for _, fx := range fixtures {
		name := filepath.Base(fx)
		t.Run(name, func(t *testing.T) {
			markup, err := os.ReadFile(fx)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			goDoc := renderGoDoc(t, markup, name)
			pyDoc := runPythonRender(t, scriptAbs, markup, name)
			diffs := compareParityDocs(goDoc, pyDoc)
			if len(diffs) == 0 {
				return
			}
			accepted := acceptedParityDiffs[name]
			var unaccepted []string
			for _, d := range diffs {
				if _, ok := accepted[d]; ok {
					t.Logf("accepted diff: %s — %s", d, accepted[d])
					continue
				}
				unaccepted = append(unaccepted, d)
			}
			if len(unaccepted) > 0 {
				t.Errorf("parity divergences vs Python MicronParser:\n  %s", strings.Join(unaccepted, "\n  "))
			}
		})
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
