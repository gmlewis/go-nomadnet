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
	"strings"
	"testing"

	"github.com/rivo/tview"
)

// Regression tests for the remote-data bracket escapes outside the RRC room
// widget: the Network/announce views, the peer/directory helpers, and the
// LXMF/conversation surfaces. A remote display name, announce app-data, or
// attachment name containing "[x]" must reach the tag-parsing widget escaped.
//
// List rows expose the raw (escaped) item text via GetItemText; dynamic-colors
// TextViews are checked through GetText(true), which resolves the escape.

func TestFormatNodeEntryRowEscapesBrackets(t *testing.T) {
	t.Parallel()

	got := FormatNodeEntryRow(NodeEntry{
		DisplayName: "[node]",
		SourceHash:  "0123456789abcdef0123456789abcdef",
		TrustLevel:  "unknown",
	}, glyphsUnicode)
	if !strings.Contains(got, "[node[]") {
		t.Errorf("FormatNodeEntryRow not escaped: %q", got)
	}
}

func TestNewTrustListItemEscapesBrackets(t *testing.T) {
	t.Parallel()

	got := NewTrustListItem("[peer]", "trusted")
	if !strings.Contains(got, "[peer[]") {
		t.Errorf("NewTrustListItem not escaped: %q", got)
	}
}

func TestNewNodeInfoEscapesBrackets(t *testing.T) {
	t.Parallel()

	ni := NewNodeInfo("0123456789abcdef0123456789abcdef", "[Node]")
	tv, ok := ni.widget.(*tview.TextView)
	if !ok {
		t.Fatal("NewNodeInfo widget is not a *tview.TextView")
	}
	if got := tv.GetText(true); !strings.Contains(got, "[Node]") {
		t.Errorf("node info lost brackets: %q", got)
	}
}

func TestNewLocalPeerEscapesBrackets(t *testing.T) {
	t.Parallel()

	lp := NewLocalPeer("0123456789abcdef0123456789abcdef", "[Me]", "[now]")
	tv, ok := lp.widget.(*tview.TextView)
	if !ok {
		t.Fatal("NewLocalPeer widget is not a *tview.TextView")
	}
	got := tv.GetText(true)
	for _, want := range []string{"[Me]", "[now]"} {
		if !strings.Contains(got, want) {
			t.Errorf("local peer lost %q: %q", want, got)
		}
	}
}

func TestNewLXMFPeersViewEscapesBrackets(t *testing.T) {
	t.Parallel()

	lv := NewLXMFPeersView([]LXMFPeerEntry{{
		Name:  "[peer]",
		Hash:  "0123456789abcdef0123456789abcdef",
		Alive: true,
	}})
	main, _ := lv.list.GetItemText(0)
	if !strings.Contains(main, "[peer[]") {
		t.Errorf("peer row not escaped: %q", main)
	}
}
