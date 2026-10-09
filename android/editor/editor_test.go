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

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/tcell/v2"
)

// memoryFiles is a Files that keeps its text in memory, so that what the
// editor writes can be read back without a disk and a failing write can be
// asked for instead of arranged with permissions.
type memoryFiles struct {
	files   map[string]string
	writeTo func(path, text string) error
}

func newMemoryFiles() *memoryFiles { return &memoryFiles{files: map[string]string{}} }

func (m *memoryFiles) Read(path string) (string, error) { return m.files[path], nil }

func (m *memoryFiles) Write(path, text string) error {
	if m.writeTo != nil {
		return m.writeTo(path, text)
	}
	m.files[path] = text
	return nil
}

// press is one key press, as the application's input capture would deliver it.
func press(e *Editor, key tcell.Key) bool {
	return e.Key(tcell.NewEventKey(key, 0, tcell.ModNone))
}

// typing replaces the buffer, which is what the text area does as characters
// arrive. Going through the widget rather than through a field is deliberate:
// the editor's idea of "modified" is the widget's text, so a test that did not
// set the widget's text would not be testing the editor.
func typing(t *testing.T, e *Editor, text string) {
	t.Helper()
	e.area.SetText(text, true)
}

func TestNewOpensTheFilesContents(t *testing.T) {
	t.Parallel()

	files := newMemoryFiles()
	files.files["/etc/config"] = "loglevel = 4\n"

	e, err := newEditor("/etc/config", files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := e.Text(), "loglevel = 4\n"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if e.Modified() {
		t.Error("a file just read is reported as modified")
	}
}

func TestNewRefusesAFileItCouldNotRead(t *testing.T) {
	t.Parallel()

	// An editor that opened an unreadable file as empty would write over it on
	// the first save, which is the one failure an editor must not have.
	files := &failingRead{err: errors.New("permission denied")}
	if _, err := newEditor("/etc/config", files); err == nil {
		t.Fatal("New accepted a file it could not read")
	} else if !strings.Contains(err.Error(), "/etc/config") {
		t.Errorf("the failure does not name the file: %v", err)
	}
}

type failingRead struct{ err error }

func (f *failingRead) Read(string) (string, error) { return "", f.err }
func (f *failingRead) Write(string, string) error  { return nil }

func TestModifiedFollowsTheTextRatherThanTheKeys(t *testing.T) {
	t.Parallel()

	files := newMemoryFiles()
	files.files["/etc/config"] = "one\n"
	e, err := newEditor("/etc/config", files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	typing(t, e, "one\ntwo\n")
	if !e.Modified() {
		t.Error("typing did not mark the buffer modified")
	}
	typing(t, e, "one\n")
	if e.Modified() {
		t.Error("text typed and then taken back is still reported as a change")
	}
}

func TestSaveWritesTheBufferAndClearsTheQuestion(t *testing.T) {
	t.Parallel()

	files := newMemoryFiles()
	files.files["/etc/config"] = "one\n"
	e, err := newEditor("/etc/config", files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.stop = func() { t.Error("save quit the editor") }

	typing(t, e, "one\ntwo\n")
	press(e, tcell.KeyCtrlQ) // asks, because there are unsaved changes
	press(e, tcell.KeyCtrlS)

	if got, want := files.files["/etc/config"], "one\ntwo\n"; got != want {
		t.Errorf("the file holds %q, want %q", got, want)
	}
	if e.Modified() {
		t.Error("a saved buffer is still reported as modified")
	}
	if e.asking {
		t.Error("the unsaved-changes question survived the save that answered it")
	}
}

func TestSaveFailureIsShownAndKeepsTheText(t *testing.T) {
	t.Parallel()

	files := newMemoryFiles()
	files.files["/etc/config"] = "one\n"
	files.writeTo = func(string, string) error { return errors.New("read-only file system") }
	e, err := newEditor("/etc/config", files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.stop = func() { t.Error("a failed save quit the editor, losing the edit") }

	typing(t, e, "one\ntwo\n")
	press(e, tcell.KeyCtrlS)

	if got := e.footer.GetText(false); !strings.Contains(got, "read-only file system") {
		t.Errorf("the failure was not shown: footer = %q", got)
	}
	if !e.Modified() {
		t.Error("a buffer whose save failed is no longer reported as modified")
	}
	if got, want := e.Text(), "one\ntwo\n"; got != want {
		t.Errorf("the text was lost: %q, want %q", got, want)
	}
	if got, want := files.files["/etc/config"], "one\n"; got != want {
		t.Errorf("the file changed despite the failure: %q, want %q", got, want)
	}
}

func TestQuitAsksOnceAboutUnsavedChanges(t *testing.T) {
	t.Parallel()

	files := newMemoryFiles()
	files.files["/etc/config"] = "one\n"
	e, err := newEditor("/etc/config", files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stopped := false
	e.stop = func() { stopped = true }

	typing(t, e, "one\ntwo\n")

	press(e, tcell.KeyCtrlQ)
	if stopped {
		t.Fatal("the first quit left without asking about unsaved changes")
	}
	if got := e.footer.GetText(false); !strings.Contains(got, "Unsaved changes") {
		t.Errorf("the first quit asked nothing: footer = %q", got)
	}

	press(e, tcell.KeyCtrlQ)
	if !stopped {
		t.Error("the second quit did not leave")
	}
}

func TestQuitLeavesWhenThereIsNothingToSave(t *testing.T) {
	t.Parallel()

	for _, key := range []tcell.Key{tcell.KeyCtrlQ, tcell.KeyEscape, tcell.KeyCtrlC} {
		t.Run(fmt.Sprintf("key %v", key), func(t *testing.T) {
			t.Parallel()

			files := newMemoryFiles()
			files.files["/etc/config"] = "one\n"
			e, err := newEditor("/etc/config", files)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			stopped := false
			e.stop = func() { stopped = true }

			// An unmodified buffer is not a question: quitting on the file that
			// has just been read is the ordinary way out.
			press(e, key)
			if !stopped {
				t.Errorf("key %v did not leave an unmodified file", key)
			}
		})
	}
}

func TestTypingWithdrawsTheUnsavedChangesQuestion(t *testing.T) {
	t.Parallel()

	files := newMemoryFiles()
	files.files["/etc/config"] = "one\n"
	e, err := newEditor("/etc/config", files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stopped := false
	e.stop = func() { stopped = true }

	typing(t, e, "one\ntwo\n")
	press(e, tcell.KeyCtrlQ)
	typing(t, e, "one\ntwo\nthree\n")
	if e.asking {
		t.Error("the editor is still asking a question the person has answered by typing")
	}
	if got := e.footer.GetText(false); strings.Contains(got, "Unsaved changes") {
		t.Errorf("a stale question is still in the footer: %q", got)
	}

	// And the next quit asks again rather than leaving on the strength of the
	// abandoned question.
	press(e, tcell.KeyCtrlQ)
	if stopped {
		t.Error("the quit left without asking again")
	}
}

func TestTheEditorsKeysAreItsOwnAndTheRestAreTheTextAreas(t *testing.T) {
	t.Parallel()

	files := newMemoryFiles()
	e, err := newEditor("/etc/config", files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.stop = func() {}

	for _, key := range []tcell.Key{
		tcell.KeyCtrlS, tcell.KeyCtrlQ, tcell.KeyEscape, tcell.KeyCtrlC,
	} {
		if !press(e, key) {
			t.Errorf("key %v was left to the text area", key)
		}
	}
	// Everything else belongs to the text area: the cursor keys, the ones the
	// text is made of, Tab (which inserts a tab), Backspace and Delete, and
	// Ctrl-Z, which is the area's undo.
	for _, key := range []tcell.Key{
		tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
		tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn,
		tcell.KeyRune, tcell.KeyTab, tcell.KeyBackspace, tcell.KeyDelete, tcell.KeyCtrlZ,
	} {
		if press(e, key) {
			t.Errorf("key %v was taken from the text area", key)
		}
	}
}

func TestTheNameAndTheMarkerDescribeTheFile(t *testing.T) {
	t.Parallel()

	files := newMemoryFiles()
	files.files["/etc/config"] = "one\n"
	e, err := newEditor("/etc/config", files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := e.title.GetText(false); !strings.Contains(got, "/etc/config") {
		t.Errorf("the title does not name the file: %q", got)
	}
	if got := e.title.GetText(false); strings.Contains(got, "*") {
		t.Errorf("an unmodified file is marked as modified: %q", got)
	}

	typing(t, e, "one\ntwo\n")
	if got := e.title.GetText(false); !strings.Contains(got, "*") {
		t.Errorf("a modified file is not marked: %q", got)
	}
}

func TestOSFilesReadsAMissingFileAsEmptyAndWritesItOnSave(t *testing.T) {
	t.Parallel()

	path := filepath.Join(tempDir(t), "config")
	var files OSFiles
	text, err := files.Read(path)
	if err != nil {
		t.Fatalf("Read of a missing file: %v", err)
	}
	if text != "" {
		t.Errorf("a missing file read as %q, want empty", text)
	}

	e, err := newEditor(path, files)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	typing(t, e, "enable_node = yes\n")
	e.Save()

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file was not created: %v", err)
	}
	if got, want := string(onDisk), "enable_node = yes\n"; got != want {
		t.Errorf("the file holds %q, want %q", got, want)
	}
	if e.Modified() {
		t.Error("the buffer is still reported as modified after a save")
	}
}

func TestOSFilesLeavesNothingBesideTheFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("one\n"), FileMode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var files OSFiles
	if err := files.Write(path, "one\ntwo\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config" {
		t.Errorf("the directory holds %v, want just the file", entries)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got, want := string(onDisk), "one\ntwo\n"; got != want {
		t.Errorf("the file holds %q, want %q", got, want)
	}
}

// tempDir is a scratch directory for one test, removed with it.
//
// It is under /tmp rather than t.TempDir() because the macOS temporary
// directory is long enough that sockets created beneath it fail.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "android-editor-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
