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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// TestPartialWidgetFromNode asserts a partial widget starts in the Pending state
// showing the ⧖ placeholder, carrying the parsed URL/ID/hash/fields/refresh.
func TestPartialWidgetFromNode(t *testing.T) {
	t.Parallel()
	nodes := micron.Parse("`{page_name`5`pid=foo}")
	if len(nodes) != 1 {
		t.Fatalf("len(nodes) = %v, want 1", len(nodes))
	}
	pw := NewPartialWidgetFromNode(nodes[0])

	if pw.State != PartialPending {
		t.Errorf("State = %v, want PartialPending", pw.State)
	}
	if pw.URL != "page_name" {
		t.Errorf("URL = %q, want page_name", pw.URL)
	}
	if pw.ID != "foo" {
		t.Errorf("ID = %q, want foo", pw.ID)
	}
	if pw.Hash == "" {
		t.Error("Hash = empty, want the descriptor SHA-256")
	}
	if !pw.HasRefresh || pw.Refresh != 5 {
		t.Errorf("Refresh = %v (HasRefresh %v), want 5", pw.Refresh, pw.HasRefresh)
	}
	if got := pw.DisplayText(); got != "⧖" {
		t.Errorf("Pending DisplayText = %q, want ⧖", got)
	}
}

// TestPartialWidgetReceived asserts partialReceived transitions to Received and
// surfaces the fetched content as the display text.
func TestPartialWidgetReceived(t *testing.T) {
	t.Parallel()
	pw := NewPartialWidgetFromNode(micron.Parse("`{page}")[0])

	pw.Received(">Updated content")

	if pw.State != PartialReceived {
		t.Errorf("State = %v, want PartialReceived", pw.State)
	}
	if pw.Content != ">Updated content" {
		t.Errorf("Content = %q, want >Updated content", pw.Content)
	}
	if got := pw.DisplayText(); got != ">Updated content" {
		t.Errorf("Received DisplayText = %q, want the content", got)
	}
}

// TestPartialWidgetProgressed asserts partialProgressed transitions to Progressed
// and records the progress fraction.
func TestPartialWidgetProgressed(t *testing.T) {
	t.Parallel()
	pw := NewPartialWidgetFromNode(micron.Parse("`{page}")[0])

	pw.Progressed(0.5)

	if pw.State != PartialProgressed {
		t.Errorf("State = %v, want PartialProgressed", pw.State)
	}
	if pw.Progress != 0.5 {
		t.Errorf("Progress = %v, want 0.5", pw.Progress)
	}
}

// TestPartialWidgetFailed asserts partialFailed transitions to Failed and records
// the error message.
func TestPartialWidgetFailed(t *testing.T) {
	t.Parallel()
	pw := NewPartialWidgetFromNode(micron.Parse("`{page}")[0])

	pw.Failed("request timed out")

	if pw.State != PartialFailed {
		t.Errorf("State = %v, want PartialFailed", pw.State)
	}
	if pw.Error != "request timed out" {
		t.Errorf("Error = %q, want request timed out", pw.Error)
	}
	if got := pw.DisplayText(); got == "" {
		t.Error("Failed DisplayText = empty, want an error message")
	}
}

// TestPartialsToRefresh asserts updatePartials selects only partials that have a
// refresh interval (HasRefresh), the ones eligible for rescheduling.
func TestPartialsToRefresh(t *testing.T) {
	t.Parallel()
	noRefresh := NewPartialWidgetFromNode(micron.Parse("`{static}")[0])
	withRefresh := NewPartialWidgetFromNode(micron.Parse("`{live`10}")[0])
	staleRefresh := NewPartialWidgetFromNode(micron.Parse("`{slow`0.5}")[0]) // <1 → no refresh

	got := PartialsToRefresh([]*PartialWidget{noRefresh, withRefresh, staleRefresh})
	if len(got) != 1 {
		t.Fatalf("len(PartialsToRefresh) = %v, want 1", len(got))
	}
	if got[0] != withRefresh {
		t.Error("the refreshable partial selected was not the live one")
	}
}
