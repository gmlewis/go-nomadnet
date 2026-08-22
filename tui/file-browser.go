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
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

// fileBrowserContent builds the content of the "Attach File" file-browser dialog
// (Python's FileBrowserDialog, Conversations.py:2948-3083): a header (current
// path + "N selected: ..." status + divider), a scrollable list of directory
// entries (parent "..", dirs "▸ name/", files "  name"/"✓ name"), and a footer
// (divider + Done/Cancel buttons). Enter on a dir/parent navigates into it;
// Enter on a file toggles its selection. Down at the bottom of the list moves
// focus to the Done button (Python frame.focus_position body→footer); Up from
// the buttons moves focus back to the list. Done calls onDone with the selected
// files; Cancel/Esc clears the selection and calls onDone(nil). close restores
// the slot.
func fileBrowserContent(app *App, startPath string, onDone func(selected []string), close func()) tview.Primitive {
	if startPath == "" {
		startPath, _ = os.UserHomeDir()
	}
	fbd := NewFileBrowserDialog(startPath)
	g := glyphsUnicode
	if app != nil && app.Glyphs != nil {
		g = app.Glyphs
	}

	pathLabel := tview.NewTextView().SetText("  " + fbd.CurrentPath())
	statusLabel := tview.NewTextView()
	list := tview.NewList()
	list.SetHighlightFullLine(true)
	ApplyListFocusStyle(list, appThemeVal(app))

	doneBtn := NewUrwidButton("Done")
	cancelBtn := NewUrwidButton("Cancel")
	buttonRow := CreateUrwidButtonRow(doneBtn, cancelBtn)

	updateStatus := func() {
		sel := fbd.SelectedFiles()
		if len(sel) > 0 {
			names := make([]string, len(sel))
			for i, p := range sel {
				names[i] = filepath.Base(p)
			}
			statusLabel.SetText("  " + g["file"] + " " + strconv.Itoa(len(sel)) + " selected: " + strings.Join(names, ", "))
		} else {
			statusLabel.SetText("  No files selected")
		}
	}

	repopulate := func(keepFocus int) {
		pathLabel.SetText("  " + fbd.CurrentPath())
		updateStatus()
		list.Clear()
		for _, e := range fbd.Entries() {
			var disp string
			switch {
			case e.IsParent:
				disp = g["arrow_l"] + " .."
			case e.IsDir:
				disp = g["arrow_r"] + " " + e.Name + "/"
			case e.Selected:
				disp = g["check"] + " " + e.Name
			default:
				disp = "  " + e.Name
			}
			list.AddItem(disp, "", 0, nil)
		}
		if keepFocus >= 0 && keepFocus < list.GetItemCount() {
			list.SetCurrentItem(keepFocus)
		} else if list.GetItemCount() > 0 {
			list.SetCurrentItem(0)
		}
	}

	list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		entries := fbd.Entries()
		if i < 0 || i >= len(entries) {
			return
		}
		e := entries[i]
		if e.IsDir || e.IsParent {
			fbd.NavigateTo(e.FullPath)
			repopulate(-1)
		} else {
			fbd.ToggleFileSelection(e.FullPath)
			repopulate(i)
		}
	})

	doneBtn.SetSelectedFunc(func() {
		sel := fbd.SelectedFiles()
		close()
		if onDone != nil {
			onDone(sel)
		}
	})
	cancelBtn.SetSelectedFunc(func() {
		fbd.CancelSelection()
		close()
		if onDone != nil {
			onDone(nil)
		}
	})

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pathLabel, 1, 0, false).
		AddItem(statusLabel, 1, 0, false).
		AddItem(newDividerRow(g["divider1"]), 1, 0, false).
		AddItem(list, 0, 1, true).
		AddItem(newDividerRow(g["divider1"]), 1, 0, false).
		AddItem(buttonRow, 1, 0, false)

	// Down at the bottom of the list → focus Done (Python body→footer); Up from
	// a button → focus the list (footer→body). All other keys pass through to
	// the focused child (list scrolls, button row moves Done↔Cancel, Enter
	// activates).
	content.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyDown:
			if list.HasFocus() && list.GetCurrentItem() >= list.GetItemCount()-1 {
				if app != nil {
					app.SetFocus(doneBtn)
				}
				return nil
			}
		case tcell.KeyUp:
			if (doneBtn.HasFocus() || cancelBtn.HasFocus()) && list.GetItemCount() > 0 {
				if app != nil {
					app.SetFocus(list)
				}
				return nil
			}
		}
		return event
	})

	repopulate(-1)
	return content
}

// appThemeVal returns the app's theme or ThemeDark (for list focus styling when
// the dialog is built without a fully-wired app, e.g. in tests).
func appThemeVal(app *App) int {
	if app != nil {
		return app.Theme
	}
	return ThemeDark
}
