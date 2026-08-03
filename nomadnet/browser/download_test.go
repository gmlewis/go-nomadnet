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

package browser

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUniqueDownloadPath pins the Go port of Python Browser.file_received /
// download_local_file's collision-avoidance logic (Browser.py:1641-1646 +
// 969-974): take the basename, and on collision append ".N" (counter starts at
// 0, first collision → ".1", then ".2", ...). Golden values are derived
// directly from the Python source's counter loop.
func TestUniqueDownloadPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		setup    func(dir string) // pre-create colliding files
		inName   string
		wantBase string // expected basename of the returned path
	}{
		{"no collision", nil, "report.pdf", "report.pdf"},
		{"basename from path", nil, "/file/docs/report.pdf", "report.pdf"},
		{"no extension", nil, "data", "data"},
		{"multiple dots", nil, "a.b.c", "a.b.c"},
		{"one collision", func(d string) { touch(d, "report.pdf") }, "report.pdf", "report.pdf.1"},
		{"two collisions", func(d string) { touch(d, "report.pdf"); touch(d, "report.pdf.1") }, "report.pdf", "report.pdf.2"},
		{"no ext collisions", func(d string) { touch(d, "data"); touch(d, "data.1") }, "data", "data.2"},
		{"gap in counter", func(d string) { touch(d, "report.pdf"); touch(d, "report.pdf.2") }, "report.pdf", "report.pdf.1"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d := t.TempDir()
			if c.setup != nil {
				c.setup(d)
			}
			got := UniqueDownloadPath(d, c.inName)
			if filepath.Base(got) != c.wantBase {
				t.Errorf("UniqueDownloadPath(%q) base = %q, want %q (full=%q)",
					c.inName, filepath.Base(got), c.wantBase, got)
			}
			if filepath.Dir(got) != d {
				t.Errorf("UniqueDownloadPath(%q) dir = %q, want %q", c.inName, filepath.Dir(got), d)
			}
		})
	}
}

// TestSaveDownload verifies SaveDownload writes the file under downloadsDir with
// the basename of the supplied name, returns the relative name + byte size, and
// avoids collisions across repeated saves — mirroring Python file_received's
// saved_file_name / saved_file_size.
func TestSaveDownload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// First save: report.pdf from a /file/ path.
	name, size, err := SaveDownload(dir, "/file/docs/report.pdf", []byte("hello"))
	if err != nil {
		t.Fatalf("SaveDownload: %v", err)
	}
	if name != "report.pdf" {
		t.Errorf("first save name = %q, want report.pdf", name)
	}
	if size != 5 {
		t.Errorf("first save size = %v, want 5", size)
	}
	assertContent(t, filepath.Join(dir, "report.pdf"), "hello")

	// Second save of the same name collides → report.pdf.1.
	name2, _, err := SaveDownload(dir, "/file/docs/report.pdf", []byte("world!"))
	if err != nil {
		t.Fatalf("SaveDownload 2: %v", err)
	}
	if name2 != "report.pdf.1" {
		t.Errorf("second save name = %q, want report.pdf.1", name2)
	}
	assertContent(t, filepath.Join(dir, "report.pdf.1"), "world!")

	// Third save collides with both → report.pdf.2.
	name3, _, err := SaveDownload(dir, "/file/docs/report.pdf", []byte("third"))
	if err != nil {
		t.Fatalf("SaveDownload 3: %v", err)
	}
	if name3 != "report.pdf.2" {
		t.Errorf("third save name = %q, want report.pdf.2", name3)
	}

	// A different name doesn't collide.
	name4, _, err := SaveDownload(dir, "notes.txt", []byte("x"))
	if err != nil {
		t.Fatalf("SaveDownload 4: %v", err)
	}
	if name4 != "notes.txt" {
		t.Errorf("notes save name = %q, want notes.txt", name4)
	}
}

func touch(dir, name string) {
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		panic(err)
	}
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("content of %s = %q, want %q", path, got, want)
	}
}
