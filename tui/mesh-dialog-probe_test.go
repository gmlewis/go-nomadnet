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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestMeshDialogProbe drives the New Conversation dialog key-by-key and logs
// focus + dialog presence after each step, to pin down the live tmux flow.
func TestMeshDialogProbe(t *testing.T) {
	app := newTestApp()
	created := ""
	cd := NewConversationsDisplay(app, nil)
	pages := app.Dialogs.Init(app.Application, cd.Widget())
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(cd.Widget())

	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool {
		created = addrHex + "|" + name + "|" + trust
		return true
	})

	send := func(k *tcell.EventKey, name string) {
		if h := app.GetFocus().InputHandler(); h != nil {
			h(k, func(p tview.Primitive) { app.SetFocus(p) })
		}
		t.Logf("%-8s focus=%T dialog=%v created=%q", name, app.GetFocus(), cd.dialogOpen, created)
	}

	t.Logf("initial   focus=%T dialog=%v", app.GetFocus(), cd.dialogOpen)
	for i := 0; i < 32; i++ {
		send(tcell.NewEventKey(tcell.KeyRune, 'A', tcell.ModNone), "typeA")
	}
	// Walk to rTrusted: Tab1=eName, Tab2=rUntrusted, Tab3=rUnknown, Tab4=rTrusted.
	for i := 1; i <= 4; i++ {
		send(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), "Tab"+string(rune('0'+i)))
	}
	send(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), "Space")
	send(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), "Tab5")
	send(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), "Enter")
}