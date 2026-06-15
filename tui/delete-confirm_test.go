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

import "testing"

func TestDeleteConfirmNew(t *testing.T) {
	t.Parallel()

	dc := NewDeleteConfirm("Delete node?", "node-abc")
	if dc.Message() != "Delete node?" {
		t.Errorf("Message() = %q, want %q", dc.Message(), "Delete node?")
	}
	if dc.Target() != "node-abc" {
		t.Errorf("Target() = %q, want %q", dc.Target(), "node-abc")
	}
	if dc.State() != ConfirmPending {
		t.Errorf("State() = %v, want ConfirmPending", dc.State())
	}
}

func TestDeleteConfirmAccept(t *testing.T) {
	t.Parallel()

	dc := NewDeleteConfirm("Delete?", "item1")
	dc.Accept()

	if dc.State() != ConfirmAccepted {
		t.Errorf("State() = %v, want ConfirmAccepted", dc.State())
	}
	if !dc.IsResolved() {
		t.Error("IsResolved() = false after Accept")
	}
}

func TestDeleteConfirmReject(t *testing.T) {
	t.Parallel()

	dc := NewDeleteConfirm("Delete?", "item1")
	dc.Reject()

	if dc.State() != ConfirmRejected {
		t.Errorf("State() = %v, want ConfirmRejected", dc.State())
	}
	if !dc.IsResolved() {
		t.Error("IsResolved() = false after Reject")
	}
}

func TestDeleteConfirmReset(t *testing.T) {
	t.Parallel()

	dc := NewDeleteConfirm("Delete?", "item1")
	dc.Accept()
	dc.Reset()

	if dc.State() != ConfirmPending {
		t.Errorf("State() = %v, want ConfirmPending after Reset", dc.State())
	}
	if dc.IsResolved() {
		t.Error("IsResolved() = true after Reset")
	}
}

func TestDeleteConfirmIsAccepted(t *testing.T) {
	t.Parallel()

	dc := NewDeleteConfirm("Delete?", "item1")

	if dc.IsAccepted() {
		t.Error("IsAccepted() = true on pending state")
	}

	dc.Accept()

	if !dc.IsAccepted() {
		t.Error("IsAccepted() = false after Accept")
	}
}

func TestDeleteConfirmNewWithTarget(t *testing.T) {
	t.Parallel()

	dc := NewDeleteConfirm("Remove hub?", "hub-123")
	if dc.Message() != "Remove hub?" {
		t.Errorf("Message() = %q", dc.Message())
	}
	if dc.Target() != "hub-123" {
		t.Errorf("Target() = %q", dc.Target())
	}
}
