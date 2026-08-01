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
	"strings"
	"testing"
)

// TestBuildAnchorMapAnchorDeclaration asserts a `:name anchor on a content line
// maps to that line's index, mirroring Python's pending_anchors flush to
// row_index (MicronParser.py:126-131, 657-665).
func TestBuildAnchorMapAnchorDeclaration(t *testing.T) {
	t.Parallel()
	lines := RenderToStyledLines("`:intro hello world", ThemeDark)
	m := BuildAnchorMap(lines)
	idx, ok := m.JumpTarget("intro")
	if !ok {
		t.Fatal(`JumpTarget("intro") not found`)
	}
	if idx != 0 {
		t.Errorf("intro index = %v, want 0", idx)
	}
}

// TestBuildAnchorMapHeadingSlug asserts a heading auto-generates a slug anchor
// pointing at the heading line (MicronParser.py:308-310).
func TestBuildAnchorMapHeadingSlug(t *testing.T) {
	t.Parallel()
	lines := RenderToStyledLines(">Section Title\nbody text", ThemeDark)
	m := BuildAnchorMap(lines)
	idx, ok := m.JumpTarget("section-title")
	if !ok {
		t.Fatal(`JumpTarget("section-title") not found`)
	}
	if idx != 0 {
		t.Errorf("heading slug index = %v, want 0", idx)
	}
}

// TestBuildAnchorMapMultipleAnchors asserts each distinct anchor maps to its own
// line index.
func TestBuildAnchorMapMultipleAnchors(t *testing.T) {
	t.Parallel()
	lines := RenderToStyledLines("`:a line a\n`:b line b\n`:c line c", ThemeDark)
	m := BuildAnchorMap(lines)
	want := map[string]int{"a": 0, "b": 1, "c": 2}
	for name, wantIdx := range want {
		gotIdx, ok := m.JumpTarget(name)
		if !ok {
			t.Errorf("anchor %q not found", name)
			continue
		}
		if gotIdx != wantIdx {
			t.Errorf("anchor %q index = %v, want %v", name, gotIdx, wantIdx)
		}
	}
}

// TestBuildAnchorMapFirstWriteWins asserts the first declaration of a duplicate
// anchor name wins (Python: `if name not in anchors` — MicronParser.py:129).
func TestBuildAnchorMapFirstWriteWins(t *testing.T) {
	t.Parallel()
	lines := RenderToStyledLines("`:dup first\n`:dup second", ThemeDark)
	m := BuildAnchorMap(lines)
	idx, ok := m.JumpTarget("dup")
	if !ok {
		t.Fatal(`JumpTarget("dup") not found`)
	}
	if idx != 0 {
		t.Errorf("duplicate anchor index = %v, want 0 (first wins)", idx)
	}
}

// TestJumpTargetUnknown asserts an unknown anchor name reports not-found.
func TestJumpTargetUnknown(t *testing.T) {
	t.Parallel()
	m := BuildAnchorMap(RenderToStyledLines("no anchors here", ThemeDark))
	if _, ok := m.JumpTarget("nope"); ok {
		t.Error(`JumpTarget("nope") found, want not found`)
	}
}

// TestJumpTargetFromHashURL asserts a "#name" URL jump resolves to the anchor's
// line index after stripping the leading "#", matching Python's handle_link
// anchor-jump branch (Browser.py) fed by the anchors map.
func TestJumpTargetFromHashURL(t *testing.T) {
	t.Parallel()
	lines := RenderToStyledLines("`:target the line\nsecond line", ThemeDark)
	m := BuildAnchorMap(lines)

	target := "#target"
	name := strings.TrimPrefix(target, "#")
	idx, ok := m.JumpTarget(name)
	if !ok {
		t.Fatal(`JumpTarget("target") not found`)
	}
	if idx != 0 {
		t.Errorf("hash-URL jump target index = %v, want 0", idx)
	}
}
