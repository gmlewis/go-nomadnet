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

import "sync"

// The editor seam.
//
// The Config page's "Open Editor" button and the Interfaces page's editor
// action open the configured editor on a file by starting a program and
// embedding its terminal. A host that cannot start a program at all — the iOS
// appliance, where the platform forbids fork and exec outright — cannot do
// that, and a button that quietly does nothing is worse than one that says
// why. This is where such a host says so.
//
// It is not a way to open a different editor: a host that installs a check is
// one that has no editor program to offer, and the check exists so the failure
// names the thing that does the job instead of a command that was never going
// to run.

var (
	// editorCheckMu guards the installed check, which a host registers from its own
	// init and the event loop reads from whichever goroutine is drawing.
	editorCheckMu sync.Mutex
	editorCheck   func(editorCmd string) error
)

// SetEditorCheck installs the check the client's editor screens ask before they start
// a program, or clears it when fn is nil.
//
// It must be called before the editor screens are built. A nil check, which is the
// default, means this host starts the editor the way a desktop does: every other
// build of this binary is unchanged.
func SetEditorCheck(fn func(editorCmd string) error) {
	editorCheckMu.Lock()
	editorCheck = fn
	editorCheckMu.Unlock()
}

// checkEditor returns the reason this host cannot open the configured editor, or nil
// when it can. A caller that gets a reason reports it instead of starting anything.
func checkEditor(editorCmd string) error {
	editorCheckMu.Lock()
	fn := editorCheck
	editorCheckMu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(editorCmd)
}
