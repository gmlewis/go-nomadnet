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

// The editor: the buffer, the keys, and the two ways the file is read and
// written.
//
// It is a type rather than a main function so that everything it decides — what
// a key press means, when a file is written, and what happens when the write
// fails — can be asserted without a terminal: [Editor.Run] and [main] are the
// only parts that need one.
package main

import (
	"fmt"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// Key hints, in the footer's own words. They are also what the footer returns
// to when a message has been shown and the next key press clears it.
const hints = "Ctrl-S Save    Ctrl-Q Quit    Ctrl-Z Undo"

// Files is the file the editor reads and writes.
//
// It is an interface because the editor's decisions are about text and about
// what to do when a write fails, and neither needs a disk to be tested. See
// OSFiles for the real one.
type Files interface {
	// Read returns the file's contents. A file that does not exist is the
	// empty string and no error: an editor started on a path that is not there
	// yet opens an empty buffer, and creates the file when it is first saved.
	Read(path string) (string, error)

	// Write replaces the file's contents, creating it if it does not exist.
	Write(path, text string) error
}

// Editor is one file being edited.
//
// Its state is the text in the area, the text as it stands on disk, and
// whether the person has been asked about unsaved changes. Everything else is
// the tview widgets that show those three things.
//
// An Editor is used from the UI loop of its application, like every tview
// primitive: the key handlers and the redraws both run there.
type Editor struct {
	path   string
	files  Files
	app    *tview.Application
	area   *tview.TextArea
	title  *tview.TextView
	footer *tview.TextView

	// onDisk is the file's contents as this editor last read or wrote them.
	// The difference between it and the area's text is the whole of what
	// "modified" means, and it is why the comparison is on text rather than on
	// a flag set by a key press: text typed and then undone is not a change.
	onDisk string

	// asking is whether the person has been asked once about unsaved changes
	// and has not answered. It is cleared by anything that answers the
	// question — a save, or a second quit.
	asking bool

	// stop ends the application. It is a field rather than a call to
	// app.Stop because the decision to stop is the part worth testing, and a
	// test has no application to stop.
	stop func()

	// note is what the footer is saying instead of the key hints, or empty
	// when it is saying the hints.
	note string
}

// newEditor builds an editor for path, reading the file through files.
//
// The file's contents are the buffer's contents; a read that fails is returned
// rather than shown as an empty buffer, because an editor that silently opens
// nothing is an editor whose first save writes over a file it never read.
func newEditor(path string, files Files) (*Editor, error) {
	text, err := files.Read(path)
	if err != nil {
		return nil, fmt.Errorf("reading %v: %w", path, err)
	}

	editor := &Editor{
		path:   path,
		files:  files,
		app:    tview.NewApplication(),
		onDisk: text,
	}
	editor.area = tview.NewTextArea().
		SetText(text, false).
		SetWrap(true).
		SetWordWrap(true).
		SetChangedFunc(editor.changed)
	editor.title = tview.NewTextView().SetDynamicColors(true)
	editor.footer = tview.NewTextView().SetDynamicColors(true)
	editor.stop = editor.app.Stop
	editor.show()
	return editor, nil
}

// Run draws the editor and reads keys until the person leaves.
func (e *Editor) Run() error {
	e.app.SetRoot(e.layout(), true)
	// The editor's own keys are taken before the text area sees them, because
	// two of them are keys the widget would otherwise spend on something else:
	// Ctrl-Q copies in tview's TextArea, and Ctrl-C is the application's own
	// way out, which would leave the unsaved-changes question unasked.
	e.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if e.Key(event) {
			return nil
		}
		return event
	})
	return e.app.Run()
}

// layout is the screen: the file's name, the text, and the footer.
//
// The name is the title row rather than a border's title because the editor is
// already drawn inside one — the client embeds it in a LineBox, as Python
// embeds the same editor in a urwid LineBox — and a second frame would be two
// borders around one file.
func (e *Editor) layout() tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(e.title, 1, 0, false).
		AddItem(e.area, 0, 1, true).
		AddItem(e.footer, 1, 0, false)
}

// Key handles one key press and reports whether the editor has taken it.
//
// The keys it takes are the ones that are about the file rather than about the
// text: save, leave, and the question a leave asks. Everything else — the
// arrows, Home and End, Page Up and Page Down, Backspace and Delete, Tab,
// Ctrl-Z — is the text area's, and is left to it.
func (e *Editor) Key(event *tcell.EventKey) bool {
	switch event.Key() {
	case tcell.KeyCtrlS:
		e.Save()
		return true
	case tcell.KeyCtrlQ, tcell.KeyEscape, tcell.KeyCtrlC:
		e.Quit()
		return true
	}
	return false
}

// Modified reports whether the text differs from what is on disk.
func (e *Editor) Modified() bool { return e.area.GetText() != e.onDisk }

// Text is the text as it stands in the buffer.
func (e *Editor) Text() string { return e.area.GetText() }

// Save writes the buffer to the file.
//
// A write that fails is shown and changes nothing: the editor still holds the
// text, still knows it is unsaved, and can be saved again — which is what
// makes a read-only file or a full disk a message rather than a lost edit.
func (e *Editor) Save() {
	text := e.area.GetText()
	if err := e.files.Write(e.path, text); err != nil {
		e.message("Could not save: " + err.Error())
		return
	}
	e.onDisk = text
	e.asking = false
	e.message("Saved " + e.path)
}

// Quit leaves the editor, asking once about unsaved changes.
//
// The question is asked rather than assumed in either direction: quitting
// silently would lose a person's edit, and refusing to quit would be a program
// they cannot leave. So the first quit with unsaved changes asks and stays, and
// the second one goes — and any save in between makes the question moot.
func (e *Editor) Quit() {
	if e.Modified() && !e.asking {
		e.asking = true
		e.message("Unsaved changes: Ctrl-S saves them, Ctrl-Q again discards them")
		return
	}
	if e.stop != nil {
		e.stop()
	}
}

// changed is what the text area calls when the text changes: the title's
// marker and the footer both describe the buffer, so both are redrawn, and an
// unsaved-changes question the person has answered by typing is withdrawn, as
// is a message about an earlier state of the file.
func (e *Editor) changed() {
	e.asking = false
	e.note = ""
	e.show()
}

// show redraws the title and the footer.
//
// The title carries the file's name, and a marker while the buffer differs
// from it. The footer carries the last thing the editor had to say, or the
// keys when it has nothing to say: a footer that kept an old message would be
// a footer that is wrong for the rest of the session.
func (e *Editor) show() {
	marker := ""
	if e.Modified() {
		marker = "*"
	}
	e.title.SetText("[" + marker + e.path + "]")
	if e.note == "" {
		e.footer.SetText(hints)
		return
	}
	e.footer.SetText("[yellow]" + e.note + "[-]")
}

// message says something in the footer, in place of the key hints until the
// person does something that makes it stale.
func (e *Editor) message(text string) {
	e.note = text
	e.show()
}
