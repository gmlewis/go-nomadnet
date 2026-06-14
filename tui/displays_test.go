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

func TestNewConfigDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConfigDisplay(app, "/home/user/.nomadnetwork/config")

	if cd == nil {
		t.Fatal("NewConfigDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestConfigDisplayWidgetType(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConfigDisplay(app, "/test/config")

	// Should be a Flex layout
	_, ok := cd.Widget().(*tview.Flex)
	if !ok {
		t.Error("Widget is not a Flex")
	}
}

func TestNewLogDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	ld := NewLogDisplay(app, "/nonexistent/log", 50)

	if ld == nil {
		t.Fatal("NewLogDisplay returned nil")
	}
	if ld.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestLogDisplayWidgetType(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	ld := NewLogDisplay(app, "/test/log", 50)

	_, ok := ld.Widget().(*tview.Flex)
	if !ok {
		t.Error("Widget is not a Flex")
	}
}

func TestNewIntroDisplay(t *testing.T) {
	t.Parallel()

	id := NewIntroDisplay("Nomad Network", "0.1.0")

	if id == nil {
		t.Fatal("NewIntroDisplay returned nil")
	}
	if id.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestIntroDisplayWidgetType(t *testing.T) {
	t.Parallel()

	id := NewIntroDisplay("Test", "1.0.0")

	_, ok := id.Widget().(*tview.Flex)
	if !ok {
		t.Error("Widget is not a Flex")
	}
}

func TestTailFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := tailFile(path, 3)
	// Content has trailing newline, so split produces ["line1",...,"line5", ""]
	// Last 3 elements: ["line4", "line5", ""]
	if result != "line4\nline5\n" {
		t.Errorf("tailFile(3) = %q, want %q", result, "line4\nline5\n")
	}
}

func TestTailFileFewerLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	content := "line1\nline2\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := tailFile(path, 10)
	if result != content {
		t.Errorf("tailFile(10) = %q, want %q", result, content)
	}
}

func TestTailFileMissing(t *testing.T) {
	t.Parallel()

	result := tailFile("/nonexistent/file.log", 10)
	if result == "" {
		t.Error("tailFile for missing file returned empty string")
	}
}
