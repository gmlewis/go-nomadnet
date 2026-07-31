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
)

func TestIngestURIDialogStructure(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	var submittedURI string
	var cancelled bool

	cd := NewConversationsDisplay(app, nil)
	cd.IngestURIDialog(func(uri string) {
		submittedURI = uri
	})

	// Dialog should be open
	if !cd.dialogOpen {
		t.Error("dialog should be open after IngestURIDialog")
	}

	// Simulate submitting a URI
	cd.dialogOpen = false
	onSubmit := func(uri string) {
		submittedURI = uri
	}
	onSubmit("lxmf://abcdef1234567890abcdef1234567890")

	if submittedURI != "lxmf://abcdef1234567890abcdef1234567890" {
		t.Errorf("submitted URI = %q, want lxmf://abcdef1234567890abcdef1234567890", submittedURI)
	}

	// Test cancel
	cd.dialogOpen = true
	onCancel := func() {
		cancelled = true
		cd.dialogOpen = false
	}
	onCancel()
	if !cancelled {
		t.Error("cancel callback not fired")
	}
	if cd.dialogOpen {
		t.Error("dialog should be closed after cancel")
	}
}

func TestIngestURIResultSuccess(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.ShowIngestResult(IngestSuccess)
	if !cd.dialogOpen {
		t.Error("result dialog should be open")
	}
}

func TestIngestURIResultDuplicate(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.ShowIngestResult(IngestDuplicate)
	if !cd.dialogOpen {
		t.Error("result dialog should be open")
	}
}

func TestIngestURIResultError(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.ShowIngestResult(IngestError)
	if !cd.dialogOpen {
		t.Error("result dialog should be open")
	}
}

func TestIngestResultConstants(t *testing.T) {
	t.Parallel()

	if IngestSuccess != 0 {
		t.Errorf("IngestSuccess = %d, want 0", IngestSuccess)
	}
	if IngestDuplicate != 1 {
		t.Errorf("IngestDuplicate = %d, want 1", IngestDuplicate)
	}
	if IngestError != 2 {
		t.Errorf("IngestError = %d, want 2", IngestError)
	}
}
