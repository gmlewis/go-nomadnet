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
	"sort"
	"strings"
)

// FileBrowserEntry represents a single entry in the file browser.
type FileBrowserEntry struct {
	Name     string
	FullPath string
	IsDir    bool
	IsParent bool
	Selected bool
}

// FileBrowserDialog provides a filesystem browser for selecting files
// to attach. Matches Python's FileBrowserDialog at Conversations.py:2948.
type FileBrowserDialog struct {
	currentPath string
	selected    map[string]bool
	entries     []FileBrowserEntry
}

// NewFileBrowserDialog creates a file browser starting at the given path.
func NewFileBrowserDialog(startPath string) *FileBrowserDialog {
	fbd := &FileBrowserDialog{
		currentPath: startPath,
		selected:    make(map[string]bool),
	}
	fbd.refresh()
	return fbd
}

// CurrentPath returns the directory the browser is currently displaying.
func (fbd *FileBrowserDialog) CurrentPath() string {
	return fbd.currentPath
}

// Entries returns the current directory listing.
func (fbd *FileBrowserDialog) Entries() []FileBrowserEntry {
	return fbd.entries
}

// SelectedFiles returns the list of selected file paths.
func (fbd *FileBrowserDialog) SelectedFiles() []string {
	paths := make([]string, 0, len(fbd.selected))
	for p := range fbd.selected {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// ToggleFileSelection adds or removes a file from the selection.
func (fbd *FileBrowserDialog) ToggleFileSelection(path string) {
	if fbd.selected[path] {
		delete(fbd.selected, path)
	} else {
		fbd.selected[path] = true
	}
}

// CancelSelection clears all selected files.
func (fbd *FileBrowserDialog) CancelSelection() {
	fbd.selected = make(map[string]bool)
}

// NavigateTo changes the browser to display the given directory.
func (fbd *FileBrowserDialog) NavigateTo(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	fbd.currentPath = path
	fbd.refresh()
}

// NavigateToParent moves the browser up to the parent directory.
func (fbd *FileBrowserDialog) NavigateToParent() {
	parent := filepath.Dir(fbd.currentPath)
	if parent == fbd.currentPath {
		return
	}
	fbd.currentPath = parent
	fbd.refresh()
}

// refresh rebuilds the directory listing from the filesystem.
func (fbd *FileBrowserDialog) refresh() {
	fbd.entries = nil

	parent := filepath.Dir(fbd.currentPath)
	if parent != fbd.currentPath {
		fbd.entries = append(fbd.entries, FileBrowserEntry{
			Name:     "..",
			FullPath: parent,
			IsParent: true,
			IsDir:    true,
		})
	}

	items, err := os.ReadDir(fbd.currentPath)
	if err != nil {
		return
	}

	var dirs, files []FileBrowserEntry
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}
		full := filepath.Join(fbd.currentPath, item.Name())
		info, err := item.Info()
		if err != nil {
			continue
		}
		entry := FileBrowserEntry{
			Name:     item.Name(),
			FullPath: full,
			IsDir:    info.IsDir(),
			Selected: fbd.selected[full],
		}
		if entry.IsDir {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	fbd.entries = append(fbd.entries, dirs...)
	fbd.entries = append(fbd.entries, files...)
}
