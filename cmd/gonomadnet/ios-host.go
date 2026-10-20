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

//go:build ios

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gmlewis/go-nomadnet/ios/bridge"
	"github.com/gmlewis/go-nomadnet/tui"
	"github.com/gmlewis/tcell/v2"
)

// This file is the appliance's host wiring, and it is the one piece of the iOS
// appliance that does not live under ios/.
//
// It has to be here because the NomadNet client is `package main`, Go forbids
// importing a main package, and this file is what calls runTextUI. The build
// tag confines it to GOOS=ios, so every other build of this binary is
// unaffected.

// terminalName is the terminal the appliance's console claims to be.
//
// The name lives in the bridge package beside the console that is the terminal,
// so the entry the client renders through and the entry the appliance's Swift
// half parses cannot drift apart. It is a direct-color entry: the client draws
// in 24-bit color on the device's own terminal.
const terminalName = bridge.TerminalName

func init() {
	// The client is registered before anything can ask for a console, so a console
	// is never bound to a socket that would accept a connection and draw nothing.
	bridge.Appliance().SetClientRunner(runClientOnConsole)

	// An iOS application may not start a program, so the client's own editor button
	// has nothing to start. It is told why, and where the editing happens instead:
	// the appliance's Config editor, which is a screen in the app.
	tui.SetEditorCheck(editorUnavailable)

	// A console that ends must not take the app with it: the reader closes the
	// console, the session ends, and the Swift half is told with an EXIT frame.
	reportTUIError = func(err error) {
		log.Printf("gonomadnet: the console session ended: %v", err)
	}
}

// editorUnavailable is why the client cannot open an editor on the appliance.
//
// The client's Config page opens the editor named in its `[textui] editor`
// setting by starting that program, and an iOS application is forbidden from
// starting a program at all, so the setting is empty and there is nothing for
// the button to run. The message names the screen that does the job instead.
func editorUnavailable(editorCmd string) error {
	return fmt.Errorf(
		"the appliance edits its configuration in the gonomadnet app, not on the console: "+
			"iOS does not let an application start a program, so this console has no editor "+
			"to open (its editor setting is %q). Open gonomadnet's Config editor from the "+
			"sidebar, which edits the same file",
		editorCmd,
	)
}

// runClientOnConsole runs the NomadNet client on the terminal the bridge built.
//
// The terminal is a tcell.Tty rather than a file descriptor, because iOS gives the
// process no controlling terminal to open: the bridge's console *is* the terminal,
// and the app's own parser is what renders it. Registering it as the screen
// provider is what makes the client draw into it instead of reaching for /dev/tty.
func runClientOnConsole(ctx context.Context, tty tcell.Tty, configDir, rnsConfigDir string) error {
	ti, err := tcell.LookupTerminfo(terminalName)
	if err != nil {
		return fmt.Errorf("looking up the %v terminal description: %w", terminalName, err)
	}
	screen, err := tcell.NewTerminfoScreenFromTtyTerminfo(tty, ti)
	if err != nil {
		return fmt.Errorf("building a %v terminal on the console: %w", terminalName, err)
	}

	tui.SetScreenProvider(func() (tcell.Screen, error) { return screen, nil })
	defer tui.SetScreenProvider(nil)

	// The console is also where the client's last words go. There is no
	// controlling terminal here for the restore to open — the console is the
	// terminal — so the client writes its restore sequence into the same tty it
	// draws through. Otherwise a console whose client has quit would keep
	// reporting the mouse to the app's own parser, printing a row of escape
	// codes for every movement of the user's finger.
	tui.SetTerminalRestoreWriter(tty)
	defer tui.SetTerminalRestoreWriter(nil)

	// The console's size is the bridge's to report, not a descriptor's: there is
	// no pty here for the built-in probe to read, and the app's RESIZE frames —
	// whose pixel fields say how much room the console has, which the client needs
	// to know whether it is standing up — are the tty's own answer.
	tui.SetTerminalPixels(func() (int, int, bool) {
		ws, err := tty.WindowSize()
		if err != nil || ws.PixelWidth <= 0 || ws.PixelHeight <= 0 {
			return 0, 0, false
		}
		return ws.PixelWidth, ws.PixelHeight, true
	})
	defer tui.SetTerminalPixels(nil)

	// Where the app's camera is, told to the client in its environment. It is the
	// same pair of variables the Android appliance passes on the console host's
	// command line, so the client reads one thing on both platforms — and the
	// client is one process here, so setting them is the whole of the handover.
	//
	// An empty endpoint is no scanner, and the client then draws no Scan button.
	if endpoint, token := bridge.Appliance().ScanEndpoint(); endpoint != "" {
		if err := os.Setenv(tui.ScanAddrEnv, endpoint); err != nil {
			log.Printf("gonomadnet: could not tell the client where the scanner is: %v", err)
		}
		if err := os.Setenv(tui.ScanTokenEnv, token); err != nil {
			log.Printf("gonomadnet: could not tell the client the scanner's token: %v", err)
		}
	}

	// runTextUI runs until the reader closes the console. It never returns an
	// error: a failure to run the interface is reported through reportTUIError,
	// which this build replaced with a report rather than a process exit.
	runTextUI(configDir, rnsConfigDir)

	// The context is what the engine cancelled; returning its reason lets the
	// session report why it ended.
	if err := ctx.Err(); err != nil && ctx.Err() != context.Canceled {
		return err
	}
	return nil
}
