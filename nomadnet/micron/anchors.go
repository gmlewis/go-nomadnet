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

// AnchorMap maps anchor names to the styled-line index that declares them,
// mirroring Python's state["anchors"] dict populated in markup_to_attrmaps
// (MicronParser.py:109-131). Anchors come from `:name declarations and heading
// slugs (both flushed to the current line's row index). The first declaration
// of a name wins (Python: `if name not in anchors`).
type AnchorMap map[string]int

// BuildAnchorMap collects anchors from rendered styled lines. Each line whose
// Anchor field is set contributes that anchor at its line index; the first
// occurrence of a name wins. Mirrors the pending_anchors flush in
// markup_to_attrmaps (MicronParser.py:126-131).
func BuildAnchorMap(lines []*StyledLine) AnchorMap {
	m := AnchorMap{}
	for i, sl := range lines {
		if sl == nil || sl.Anchor == "" {
			continue
		}
		if _, ok := m[sl.Anchor]; !ok {
			m[sl.Anchor] = i
		}
	}
	return m
}

// JumpTarget returns the line index for anchor name, the lookup a "#name" URL
// jump performs (after the caller strips the leading "#"). ok is false when the
// anchor name is unknown, so the caller can fall back to a no-op or error.
func (m AnchorMap) JumpTarget(name string) (int, bool) {
	idx, ok := m[name]
	return idx, ok
}
