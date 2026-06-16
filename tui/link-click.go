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

package tui

import (
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// LinkPosition represents a link's start and end character positions
// in rendered text, along with its target URL.
type LinkPosition struct {
	Start  int
	End    int
	Target string
}

// LinkClickTracker tracks link positions in rendered Micron text and
// determines which link was clicked based on character position.
// Matches Python's LinkableText.find_item_at_pos at MicronParser.py:895.
type LinkClickTracker struct {
	links []LinkPosition
}

// NewLinkClickTracker creates a tracker by parsing Micron AST nodes
// and extracting link positions. The position calculation counts
// rendered characters. tview color tags are zero-width so they don't
// affect the position mapping.
func NewLinkClickTracker(nodes []*micron.Node) *LinkClickTracker {
	tracker := &LinkClickTracker{}
	tracker.extractLinks(nodes, &pos{0})
	return tracker
}

type pos struct{ v int }

func (t *LinkClickTracker) extractLinks(nodes []*micron.Node, p *pos) {
	for _, node := range nodes {
		switch node.Type {
		case micron.NodeLink:
			visibleLen := len([]rune(node.LinkURL))
			t.links = append(t.links, LinkPosition{
				Start:  p.v,
				End:    p.v + visibleLen,
				Target: node.LinkURL,
			})
			p.v += visibleLen
		case micron.NodeHeading:
			for _, child := range node.Children {
				t.extractLinks([]*micron.Node{child}, p)
			}
		case micron.NodeText:
			p.v += len([]rune(node.Text))
		case micron.NodeDivider:
			p.v += 31
		case micron.NodeField:
			p.v += len([]rune(node.FieldName)) + 4
		case micron.NodePartial:
			p.v += len([]rune(node.PartialID)) + 12
		case micron.NodeAnchor:
			p.v += len([]rune(node.AnchorName)) + 2
		default:
			p.v += len([]rune(node.Text))
		}
	}
}

// FindLinkAtPosition returns the link target if the given character
// position falls within a link span, or empty string otherwise.
func (t *LinkClickTracker) FindLinkAtPosition(pos int) string {
	for _, link := range t.links {
		if pos >= link.Start && pos < link.End {
			return link.Target
		}
	}
	return ""
}

// SetLinks sets the link positions for the tracker.
func (t *LinkClickTracker) SetLinks(links []LinkPosition) {
	t.links = links
}

// Links returns the current link positions.
func (t *LinkClickTracker) Links() []LinkPosition {
	return t.links
}
