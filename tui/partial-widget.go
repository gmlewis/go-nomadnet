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
	"fmt"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// PartialState is the lifecycle state of a page-partial widget, mirroring the
// Python partial refresh flow (partialReceived / partialProgressed /
// partialFailed) on the Browser/Frame.
type PartialState int

const (
	// PartialPending is the initial state: the ⧖ placeholder is shown while the
	// partial page has not yet been fetched.
	PartialPending PartialState = iota
	// PartialProgressed means a fetch is in flight and a progress fraction is
	// available (partialProgressed).
	PartialProgressed
	// PartialReceived means the fetch completed and content is available
	// (partialReceived).
	PartialReceived
	// PartialFailed means the fetch failed and an error message is available
	// (partialFailed).
	PartialFailed
)

// PartialWidget is the runtime state of one Micron page partial — the Go
// equivalent of the urwid.Pile produced by parse_partial (MicronParser.py:185-
// 193), carrying the partial id/hash/url/fields/refresh plus the refresh-flow
// state. It is a pure model; rendering wires it to a tview primitive.
type PartialWidget struct {
	URL        string
	ID         string
	Hash       string
	Fields     []string
	Refresh    float64
	HasRefresh bool

	State    PartialState
	Content  string  // received micron markup (PartialReceived)
	Progress float64 // 0..1 (PartialProgressed)
	Error    string  // failure message (PartialFailed)
}

// NewPartialWidgetFromNode builds a Pending partial widget from a parsed
// NodePartial, carrying the url/id/hash/fields/refresh. Mirrors the attribute
// assignment in parse_partial (MicronParser.py:188-192).
func NewPartialWidgetFromNode(node *micron.Node) *PartialWidget {
	pw := &PartialWidget{
		URL:        node.PartialURL,
		ID:         node.PartialID,
		Hash:       micron.PartialHash(node),
		Fields:     node.PartialFields,
		Refresh:    node.PartialRefresh,
		HasRefresh: node.HasRefresh,
		State:      PartialPending,
	}
	if pw.Fields == nil {
		pw.Fields = []string{""}
	}
	return pw
}

// Received transitions the partial to PartialReceived with the fetched micron
// content (Python partialReceived).
func (pw *PartialWidget) Received(content string) {
	pw.State = PartialReceived
	pw.Content = content
}

// Progressed transitions the partial to PartialProgressed with a fetch progress
// fraction in [0,1] (Python partialProgressed).
func (pw *PartialWidget) Progressed(fraction float64) {
	pw.State = PartialProgressed
	pw.Progress = fraction
}

// Failed transitions the partial to PartialFailed with an error message (Python
// partialFailed).
func (pw *PartialWidget) Failed(err string) {
	pw.State = PartialFailed
	pw.Error = err
}

// DisplayText returns the text to render for the partial in its current state:
// the ⧖ placeholder while Pending, the received content once Received, a
// progress line while Progressed, or the error message once Failed.
func (pw *PartialWidget) DisplayText() string {
	switch pw.State {
	case PartialReceived:
		return pw.Content
	case PartialProgressed:
		return fmt.Sprintf("⧖ %v%%", int(pw.Progress*100))
	case PartialFailed:
		if pw.Error == "" {
			return "⧖"
		}
		return pw.Error
	default: // PartialPending
		return "⧖"
	}
}

// PartialsToRefresh returns the subset of partials eligible for refresh
// scheduling — those with a refresh interval (HasRefresh, which already
// excludes intervals < 1). Mirrors the updatePartials selection that reschedules
// only partials carrying a refresh interval.
func PartialsToRefresh(widgets []*PartialWidget) []*PartialWidget {
	var out []*PartialWidget
	for _, pw := range widgets {
		if pw.HasRefresh {
			out = append(out, pw)
		}
	}
	return out
}
