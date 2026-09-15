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

	"github.com/rivo/tview"
)

func TestNewDialogLineBox(t *testing.T) {
	t.Parallel()

	content := tview.NewTextView().SetText("test")
	dismissed := false
	d := NewDialogLineBox("Test Dialog", content, func() { dismissed = true })

	if d == nil {
		t.Fatal("NewDialogLineBox returned nil")
	}
	if d.title != "Test Dialog" {
		t.Errorf("title = %q, want %q", d.title, "Test Dialog")
	}
	if d.onDismiss == nil {
		t.Error("onDismiss is nil")
	}
	// Call dismiss
	d.onDismiss()
	if !dismissed {
		t.Error("onDismiss callback not invoked")
	}
}

func TestNewConfirmationDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	yesCalled := false
	noCalled := false

	cd := NewConfirmationDialog(app.Dialogs, "Are you sure?", func() { yesCalled = true }, func() { noCalled = true })

	if cd == nil {
		t.Fatal("NewConfirmationDialog returned nil")
	}
	if cd.message != "Are you sure?" {
		t.Errorf("message = %q, want %q", cd.message, "Are you sure?")
	}
	if !yesCalled && !noCalled {
		// Both should be callable
		cd.onYes()
		if !yesCalled {
			t.Error("onYes not called")
		}
		cd.onNo()
		if !noCalled {
			t.Error("onNo not called")
		}
	}
}

func TestDialogLineBoxDismiss(t *testing.T) {
	t.Parallel()

	dismissed := false
	d := NewDialogLineBox("Test", nil, func() { dismissed = true })

	// Simulate dismiss
	if d.onDismiss != nil {
		d.onDismiss()
	}
	if !dismissed {
		t.Error("onDismiss not called")
	}
}

func TestDialogLineBoxNilDismiss(t *testing.T) {
	t.Parallel()

	d := NewDialogLineBox("Test", nil, nil)
	// Should not panic
	if d.onDismiss != nil {
		d.onDismiss()
	}
}
