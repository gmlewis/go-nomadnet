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

// Command gonomadnetedit is the text editor the appliance carries with it.
//
// Nomad Network edits its configuration by running an editor on it — the same
// program Python's EditorTerminal starts — and an appliance has none to run:
// Android ships no editor an application may execute, and a bare program name
// is not something an Android process may look up at all. So the appliance
// carries one, and the client's [textui] editor setting names it.
//
// It is deliberately small: a file, a cursor, a save and a quit. What it edits
// is a configuration file that is read at startup and read by a person, so the
// features that matter are that it opens, that it shows the whole file, and
// that closing it cannot lose an edit by accident.
//
// It lives under android/ with the rest of the appliance because that is the
// only place it is ever run: it is not a command for a laptop, and nothing on a
// desktop should grow a second editor.
package main

import (
	"flag"
	"fmt"
	"log"
)

// usage is the documented form, which is the whole of this program's interface.
const usage = `gonomadnetedit edits one text file on a terminal.

Usage:
  gonomadnetedit FILE

Keys:
  Ctrl-S   save
  Ctrl-Q   quit (asks once if there are unsaved changes)
  Esc      quit, the same way
  Ctrl-Z   undo
`

func main() {
	log.SetFlags(0)
	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		log.Fatalf("gonomadnetedit: one file to edit is required")
	}

	editor, err := newEditor(flag.Arg(0), OSFiles{})
	if err != nil {
		log.Fatal(err)
	}
	if err := editor.Run(); err != nil {
		log.Fatal(err)
	}
}
