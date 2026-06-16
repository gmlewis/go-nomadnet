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
	"os"
	"path/filepath"
	"testing"

	"github.com/rivo/tview"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "gonomadnet-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestFileBrowserDialogCreation(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	fbd := NewFileBrowserDialog(dir)
	if fbd == nil {
		t.Fatal("NewFileBrowserDialog returned nil")
	}
}

func TestFileBrowserDialogCurrentPath(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	fbd := NewFileBrowserDialog(dir)

	if fbd.CurrentPath() != dir {
		t.Errorf("CurrentPath = %q, want %q", fbd.CurrentPath(), dir)
	}
}

func TestFileBrowserDialogNavigateUp(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	fbd := NewFileBrowserDialog(subdir)
	fbd.NavigateToParent()

	if fbd.CurrentPath() != dir {
		t.Errorf("after NavigateToParent, CurrentPath = %q, want %q", fbd.CurrentPath(), dir)
	}
}

func TestFileBrowserDialogNavigateIntoDir(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	fbd := NewFileBrowserDialog(dir)
	fbd.NavigateTo(subdir)

	if fbd.CurrentPath() != subdir {
		t.Errorf("after NavigateTo, CurrentPath = %q, want %q", fbd.CurrentPath(), subdir)
	}
}

func TestFileBrowserDialogSelectFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	fbd := NewFileBrowserDialog(dir)
	fbd.ToggleFileSelection(filePath)

	selected := fbd.SelectedFiles()
	if len(selected) != 1 || selected[0] != filePath {
		t.Errorf("SelectedFiles = %v, want [%q]", selected, filePath)
	}
}

func TestFileBrowserDialogDeselectFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	fbd := NewFileBrowserDialog(dir)
	fbd.ToggleFileSelection(filePath)
	fbd.ToggleFileSelection(filePath)

	selected := fbd.SelectedFiles()
	if len(selected) != 0 {
		t.Errorf("after deselect, SelectedFiles = %v, want empty", selected)
	}
}

func TestFileBrowserDialogCancelClearsSelection(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	fbd := NewFileBrowserDialog(dir)
	fbd.ToggleFileSelection(filePath)
	fbd.CancelSelection()

	selected := fbd.SelectedFiles()
	if len(selected) != 0 {
		t.Errorf("after CancelSelection, SelectedFiles = %v, want empty", selected)
	}
}

func TestFileBrowserDialogListEntries(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "afile.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fbd := NewFileBrowserDialog(dir)
	entries := fbd.Entries()

	if len(entries) < 2 {
		t.Errorf("Entries returned %d, want at least 2", len(entries))
	}

	var hasDir, hasFile bool
	for _, e := range entries {
		if e.Name == "adir" && e.IsDir {
			hasDir = true
		}
		if e.Name == "afile.txt" && !e.IsDir {
			hasFile = true
		}
	}
	if !hasDir {
		t.Error("Entries missing directory 'adir'")
	}
	if !hasFile {
		t.Error("Entries missing file 'afile.txt'")
	}
}

func TestFileBrowserDialogHiddenFilesExcluded(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fbd := NewFileBrowserDialog(dir)
	entries := fbd.Entries()

	for _, e := range entries {
		if e.Name == ".hidden" {
			t.Error("hidden files should not appear in entries")
		}
	}
}

func TestConversationWidgetAttachFile(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")

	var attached []string
	cw.OnAttachFiles = func(paths []string) { attached = paths }

	dir := tempDir(t)
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	cw.OpenAttachFileDialog(dir)

	if !cw.DialogOpen() {
		t.Error("OpenAttachFileDialog should set dialog open")
	}

	cw.ConfirmAttachFile([]string{filePath})
	if cw.DialogOpen() {
		t.Error("ConfirmAttachFile should close dialog")
	}
	if len(attached) != 1 || attached[0] != filePath {
		t.Errorf("OnAttachFiles = %v, want [%q]", attached, filePath)
	}
}
