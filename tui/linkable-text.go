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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LinkSpec holds metadata for a clickable link within LinkableText.
// Matches Python's LinkSpec at MicronParser.py:856.
type LinkSpec struct {
	Label  string
	Target string
	Fields string
}

// LinkableText displays text with embedded clickable link regions.
// It wraps tview.TextView and tracks link positions so that mouse
// clicks can resolve and dispatch the correct link action.
// Matches Python's LinkableText at MicronParser.py:866.
type LinkableText struct {
	*tview.TextView
	links    []LinkSpec
	onHandle func(target, fields string)
}

// NewLinkableText creates a selectable text view with link support.
// The onHandle callback is invoked when a link is activated;
// it receives the link target and optional fields string.
func NewLinkableText(onHandle func(target, fields string)) *LinkableText {
	lt := &LinkableText{
		TextView: tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetRegions(true).
			SetTextColor(tcell.NewHexColor(0xbbbbbb)),
		onHandle: onHandle,
	}

	lt.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			region := lt.GetRegionByMouse(event)
			if region != "" {
				lt.activateRegion(region)
			}
		}
		return action, event
	})

	return lt
}

// AddLink registers a link with a label and target. Links are
// numbered sequentially starting from 0 and rendered as tview
// region tags [\"N\"]...[\"\"] within the text.
func (lt *LinkableText) AddLink(label, target string) {
	lt.links = append(lt.links, LinkSpec{
		Label:  label,
		Target: target,
	})
}

// AddLinkWithFields registers a link with an additional fields
// string that is passed to the handler on activation.
func (lt *LinkableText) AddLinkWithFields(label, target, fields string) {
	lt.links = append(lt.links, LinkSpec{
		Label:  label,
		Target: target,
		Fields: fields,
	})
}

// Links returns a copy of the current link list.
func (lt *LinkableText) Links() []LinkSpec {
	result := make([]LinkSpec, len(lt.links))
	copy(result, lt.links)
	return result
}

// PlainText returns the text without tview color/region tags.
func (lt *LinkableText) PlainText() string {
	return lt.GetText(false)
}

// RenderedText returns the text including all tview tags.
func (lt *LinkableText) RenderedText() string {
	return lt.GetText(true)
}

// SetText sets the display text. Callers should embed tview region
// tags like [\"0\"]...[\"\"] that reference link indices added
// via AddLink.
func (lt *LinkableText) SetText(text string) {
	lt.TextView.SetText(text)
}

// Clear removes all text and registered links.
func (lt *LinkableText) Clear() {
	lt.TextView.SetText("")
	lt.links = nil
}

// HandleLinkByIndex activates the link at the given index.
// This is the programmatic equivalent of clicking a link.
func (lt *LinkableText) HandleLinkByIndex(idx int) {
	if idx < 0 || idx >= len(lt.links) {
		return
	}
	link := lt.links[idx]
	if lt.onHandle != nil {
		lt.onHandle(link.Target, link.Fields)
	}
}

// GetRegionByMouse resolves a mouse event to a tview region tag ID.
// It returns the currently highlighted region ID, which tview
// sets when a region is clicked or navigated to.
func (lt *LinkableText) GetRegionByMouse(event *tcell.EventMouse) string {
	highlights := lt.GetHighlights()
	if len(highlights) > 0 {
		return highlights[0]
	}
	return ""
}

// activateRegion dispatches the link handler for a region tag.
// Region tags use numeric IDs matching the link index.
func (lt *LinkableText) activateRegion(regionID string) {
	if regionID == "" {
		return
	}
	idx := 0
	for _, c := range regionID {
		if c >= '0' && c <= '9' {
			idx = idx*10 + int(c-'0')
		} else {
			return
		}
	}
	lt.HandleLinkByIndex(idx)
}
