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

// Command embedded-terminal-demo is a Phase-1/2 prototype harness for the
// EmbeddedTerminal widget: it runs a bare tview app whose sole primitive is an
// EmbeddedTerminal running an editor (nano by default) on a temp file, so the
// widget's render, input, resize, and mouse forwarding can be verified live in
// a real terminal/tmux PTY.
//
// Usage: embedded-terminal-demo [editor] [args...]
// All args after the editor are passed to it (e.g. "nano -m /tmp/file"). The
// demo seeds the first non-flag arg (the file) if it is empty. Press the
// editor's quit keys (e.g. nano Ctrl-X, vim :q) to exit; the app stops when the
// child exits.
package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/gmlewis/go-nomadnet/tui"
)

func main() {
	editor := "nano"
	if e := os.Getenv("EDITOR"); e != "" {
		editor = e
	}
	if len(os.Args) > 1 && os.Args[1] != "" {
		editor = os.Args[1]
	}
	editorArgs := []string{}
	file := "/tmp/embedded-terminal-demo.txt"
	if len(os.Args) > 2 {
		editorArgs = os.Args[2:]
		for _, a := range editorArgs {
			if len(a) > 0 && a[0] != '-' {
				file = a
				break
			}
		}
	}
	if err := os.WriteFile(file, []byte("EmbeddedTerminal prototype.\nEdit me, then quit the editor to exit the demo.\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "seed file: %v\n", err)
		os.Exit(1)
	}

	app := tui.NewApp(tui.ThemeDark, tui.GlyphUnicode, tui.ColorModeTrue)
	editor = tui.ResolveEditorCmd(editor)
	cmd := exec.Command(editor, editorArgs...)

	cols, rows := 100, 32
	et, err := tui.NewEmbeddedTerminal(app, cmd, cols, rows, "xterm", func() {
		app.Stop()
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewEmbeddedTerminal: %v\n", err)
		os.Exit(1)
	}

	app.Application.SetRoot(et, true)
	app.Application.SetFocus(et)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
}
