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

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-nomadnet/nomadnet/rrc"
	"github.com/gmlewis/go-nomadnet/tui"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/rivo/tview"
)

// diagFile appends a line to a diagnostic file (TEMP debug for the input-box
// reliability investigation).
func diagFile(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(line + "\n")
}

// runTextUI starts NomadNet with the terminal UI.
func runTextUI(configDir, rnsConfigDir string) {
	// Ensure the log directory exists
	logDir := filepath.Join(configDir, "logs")
	_ = os.MkdirAll(logDir, 0o755)

	// Redirect standard log to file BEFORE any logging happens
	// (matches gornphone pattern to prevent log output destroying TUI)
	logPath := filepath.Join(logDir, "nomadnet.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		defer func() { _ = logFile.Close() }()
		log.SetOutput(logFile)
		log.SetFlags(0)
	}

	log.Printf("Nomad Network text UI starting...")

	// Initialize the app
	a := app.NewApp(configDir, rnsConfigDir, false, false)
	if err := a.Init(); err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	// Determine theme from config
	theme := tui.ThemeDark
	if a.Config != nil && a.Config.TextUI.Theme == "light" {
		theme = tui.ThemeLight
	}

	// Determine glyph set
	glyphSet := tui.GlyphUnicode
	if a.Config != nil {
		switch a.Config.TextUI.Glyphs {
		case "plain":
			glyphSet = tui.GlyphPlain
		case "nerdfont":
			glyphSet = tui.GlyphNerd
		}
	}

	// Determine color depth from config (default 24-bit true color).
	colorMode := tui.ColorModeTrue
	if a.Config != nil {
		colorMode = tui.ParseColorMode(a.Config.TextUI.ColorMode)
	}

	// Create and run the TUI
	tuiApp := tui.NewApp(theme, glyphSet, colorMode)

	// Crash recovery. An unrecovered panic in a gonomadnet goroutine restores
	// the terminal and writes the stack to a crash file instead of letting the
	// runtime spray a GOTRACEBACK dump at the (raw, alt-screen) terminal — which
	// is what leaves the tty in raw mode spewing escape-sequence garbage and
	// forces a manual `reset`. tview restores the tty ONLY for panics in its own
	// event-loop goroutine (and then re-panics); this defer catches that
	// re-panic, and App.GoSafe/drainUpdates route background-goroutine panics
	// here too. The launcher's EXIT trap remains the final backstop for panics
	// in go-reticulum's own goroutines (which we cannot wrap from here).
	handleCrash := func(r any) {
		// Best-effort restore the tty so a bare-run user isn't left in raw mode.
		go func() { defer func() { _ = recover() }(); tuiApp.RestoreTerminal() }()
		time.Sleep(300 * time.Millisecond) // let Stop emit ExitCA + restore termios
		crashDir := filepath.Join(configDir, "logs")
		_ = os.MkdirAll(crashDir, 0o755)
		path := filepath.Join(crashDir, "crash-"+time.Now().Format("20060102-150405")+".log")
		if f, err := os.Create(path); err == nil {
			_, _ = fmt.Fprintf(f, "gonomadnet panic: %v\n\n", r)
			_, _ = f.Write(debug.Stack())
			_ = f.Close()
		}
		os.Exit(1)
	}
	tuiApp.SetOnPanic(handleCrash)
	defer func() {
		if r := recover(); r != nil {
			handleCrash(r)
		}
	}()

	// Wire up real displays BEFORE setting root. The returned cleanup releases
	// background resources (log tail) on shutdown.
	logCleanup := wireDisplays(tuiApp, a)

	// quitFn is the single graceful-exit path, shared by Ctrl-Q / the [ Quit ]
	// menubar item (SetQuitCallback) and external signals below. The sync.Once
	// lets both fire concurrently (Ctrl-Q on the event loop while a signal
	// arrives) without double-running a.Shutdown.
	var quitOnce sync.Once
	quitFn := func() {
		quitOnce.Do(func() {
			tuiApp.Main.StopUnreadBlink()
			if logCleanup != nil {
				logCleanup()
			}
			a.Shutdown()
			tuiApp.Stop()
		})
	}
	tuiApp.SetQuitCallback(quitFn)

	// External signals (SIGINT/SIGTERM — `kill`, the recovery path when the UI
	// becomes unresponsive) run the SAME graceful exit as Ctrl-Q, so the
	// directory and RNS state are saved and tcell restores the terminal,
	// instead of the default immediate-death behavior that leaves the tty in
	// raw mode on the alt screen. While the tty is in raw mode (ISIG cleared)
	// Ctrl-C reaches the app as a key event, not a signal — this path covers a
	// lost raw mode or an explicit kill.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received %v — shutting down gracefully", sig)
		quitFn()
	}()

	// Probe unread conversations every 2 s and swap the menu indicator glyph
	// (Python MenuDisplay.update_display job, Main.py:216-230). The app injects
	// the probe so the tui package need not import nomadnet/app.
	tuiApp.Main.SetUnreadCheck(func() bool { return a.HasUnreadConversations() })
	tuiApp.Main.StartUnreadBlink()

	// Show the intro splash for intro_time seconds (default 1), then swap to
	// the main display — matching Python's TextUI.py:223-232. intro_time <= 0
	// skips the splash and shows the main display immediately.
	introTime := 1.0
	if a.Config != nil {
		introTime = a.Config.TextUI.IntroTime
	}
	intro := tui.NewIntroDisplay("Nomad Network", a.Version)
	tuiApp.ShowIntro(intro.Widget(), introTime)

	if err := tuiApp.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}

// wireDisplays connects the app data to the TUI display widgets.
// wireDisplays builds and registers all body-page displays. It returns a
// cleanup function that must be invoked on shutdown to release background
// resources (e.g. the log live-tail goroutine).
func wireDisplays(tuiApp *tui.App, a *app.App) func() {
	main := tuiApp.Main

	// Network display — use real announces from the app
	networkDisplay := tui.NewNetworkDisplay(tuiApp, nil, nil)
	networkDisplay.SanitizeNames = a.Config.TextUI.SanitizeNames

	// Seed the name edit ONCE with the current display name, mirroring Python's
	// LocalPeer.__init__ which sets e_name.edit_text at construction
	// (Network.py:1271) and never re-sets it. PeerSettings is loaded
	// synchronously during Init, so GetDisplayName is available here. The name
	// is deliberately not part of the UpdateLocalPeer refresh path — including
	// it there would clobber the user's in-progress typing on every
	// UIChangeCallback (incoming announces/messages). See LocalPeerDisplay.SetName.
	networkDisplay.SetLocalPeerName(a.GetDisplayName())

	// refreshLXMFPeers rebuilds the LXMF Propagation Peers list from the app's
	// LXMF message router (Python reinit_lxmf_peers, Network.py:1717 +
	// make_peer_widgets 1863-1869): peers sorted by (pn_trust_level,
	// sync_transfer_rate) descending; each row is the peer_info_str from
	// FormatLXMFPeerEntry. No router (early boot) ⇒ empty (no-content branch).
	var refreshConvs func()
	refreshLXMFPeers := func() {
		if a.Router == nil {
			networkDisplay.UpdateLXMFPeers(nil)
			return
		}
		peers := a.Router.Peers()
		sym := tuiApp.Glyphs["sent"]
		entries := make([]tui.LXMFPeerEntry, 0, len(peers))
		// Sort by (pn_trust_level, sync_transfer_rate) descending (Python: sorted
		// ... reverse=True). Trust bytes: Trusted=0xFF > Unknown=0x02 >
		// Untrusted=0x01 > Warning=0x00, so plain descending byte order matches.
		sort.SliceStable(peers, func(i, j int) bool {
			hi := peers[i].DestinationHash()
			hj := peers[j].DestinationHash()
			ti, _ := a.Dir.PNTrustLevel(hi)
			tj, _ := a.Dir.PNTrustLevel(hj)
			if ti != tj {
				return ti > tj
			}
			return peers[i].SyncTransferRate() > peers[j].SyncTransferRate()
		})
		for _, peer := range peers {
			displayStr := tui.ResolvePeerDisplayStr(peer.DestinationHash(), peer.Identity(), a.Dir.AllegedDisplayStr)
			data := tui.BuildPeerEntryData(peer, displayStr, sym)
			entries = append(entries, tui.LXMFPeerEntry{
				DisplayText:     tui.FormatLXMFPeerEntry(data, time.Now()),
				DestinationHash: peer.DestinationHash(),
			})
		}
		networkDisplay.UpdateLXMFPeers(entries)
	}
	// localPeerInfo formats the app's identity/LXMF hashes + last-announce time
	// for the Local Peer Info panel refresh (Python LocalPeer, Network.py:1259).
	// The name is intentionally excluded — it is seeded once via
	// SetLocalPeerName above and never refreshed (Python never re-sets
	// e_name.edit_text after construction). prettyhexrep = "<" + lowercase hex + ">".
	localPeerInfo := func() (lxmfAddr, identityHash string, lastAnnounce time.Time) {
		lxmfAddr = "<" + a.LXMFAddressHex() + ">"
		if a.Identity != nil {
			identityHash = "<" + fmt.Sprintf("%x", a.Identity.Hash) + ">"
		}
		return lxmfAddr, identityHash, a.LastAnnounce
	}
	lxmfAddr, idhash, lann := localPeerInfo()
	networkDisplay.UpdateLocalPeer(lxmfAddr, idhash, lann)

	// navigateTo is assigned once the browser display is constructed (below),
	// then invoked both by the network list's connect handler and by the Local
	// Node Info "Browse" button (Python connect_query,
	// Network.py:1402-1404/browse_own). It is captured here so the node-info
	// callback — wired before the browser exists — can close over it and call
	// the live value at click time.
	navigateTo := func(string) {}
	_ = navigateTo

	// showURLDialog shows the "Enter URL" dialog as a slot overlay on the
	// Network browser pane (Python Browser.url_dialog, Browser.py:1135-1182:
	// columns.contents[1] = urwid.Overlay(..., width=("relative", 65),
	// height=PACK, left=2, right=2), title "Enter URL", "URL : " edit, Cancel/
	// Go buttons). The browser frame shows through around the 65%-width dialog.
	// Enter on the field or "Go" submits; Esc/Cancel dismisses.
	showURLDialog := func(prefill string, onGo func(string)) {
		input := tview.NewInputField()
		input.SetLabel("URL : ")
		input.SetText(prefill)
		input.SetFieldBackgroundColor(tcell.ColorDefault)
		input.SetFieldTextColor(tcell.ColorDefault)
		close := func() { networkDisplay.CloseBrowserSlotDialog() }
		submit := func() {
			text := strings.TrimSpace(input.GetText())
			close()
			if text != "" {
				onGo(text)
			}
		}
		goBtn := tui.NewUrwidButton("Go").SetSelectedFunc(submit)
		cancelBtn := tui.NewUrwidButton("Cancel").SetSelectedFunc(close)
		// Python button order: Cancel(0.45), spacer(0.10), Go(0.45).
		row := tui.CreateUrwidButtonRow(cancelBtn, goBtn)
		input.SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				submit()
			}
		})
		layout := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(input, 1, 0, true).
			AddItem(row, 1, 0, false)
		dialog := tui.NewDialogLineBox("Enter URL", layout, nil)
		// input 1 + button row 1 + border 2 = 4 PACK height; 65% pane width.
		networkDisplay.ShowBrowserSlotDialog(dialog, 65, 4)
		tui.WireDialogNav(tuiApp, close, []tview.Primitive{input, goBtn, cancelBtn})
	}

	// showBrowserConfirm shows a Cancel/Confirm dialog as a slot overlay on the
	// Network browser pane (Python Browser.save_node_dialog, Browser.py:1184-
	// 1234: columns.contents[1] = urwid.Overlay(..., width=("relative", 50),
	// height=PACK), title "Save Node"). The browser frame shows through around
	// the widthPct%-width dialog. Python's urwid.Text wraps the message to the
	// dialog's content width and height=PACK adapts; we pre-wrap via
	// tview.WordWrap to the known pane width so the Go dialog matches.
	showBrowserConfirm := func(title, message, cancelLabel, confirmLabel string, widthPct int, onConfirm func()) {
		close := func() { networkDisplay.CloseBrowserSlotDialog() }
		confirmBtn := tui.NewUrwidButton(confirmLabel).SetSelectedFunc(func() {
			close()
			if onConfirm != nil {
				onConfirm()
			}
		})
		cancelBtn := tui.NewUrwidButton(cancelLabel).SetSelectedFunc(close)
		row := tui.CreateUrwidButtonRow(cancelBtn, confirmBtn)
		// Compute the dialog content width so we can pre-wrap the message
		// to the same width Python's urwid.Text would wrap to. The dialog
		// is widthPct% of the browser pane; the LineBox border takes 2 cols.
		paneW := networkDisplay.BrowserPaneWidth()
		contentW := 0
		if paneW > 4 {
			contentW = paneW*widthPct/100 - 2
		}
		// Split on explicit newlines, word-wrap each segment, then rejoin
		// so the urwidLeftText draws one wrapped line per row (matching
		// urwid.Text's pack-height behavior).
		var wrappedLines []string
		for seg := range strings.SplitSeq(message, "\n") {
			if contentW > 0 {
				rows := tview.WordWrap(seg, contentW)
				wrappedLines = append(wrappedLines, rows...)
			} else {
				wrappedLines = append(wrappedLines, seg)
			}
		}
		wrappedMsg := strings.Join(wrappedLines, "\n")
		msgRows := max(len(wrappedLines), 1)
		msg := tui.NewUrwidLeftText(wrappedMsg)
		layout := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(msg, msgRows, 0, false).
			AddItem(row, 1, 0, true)
		dialog := tui.NewDialogLineBox(title, layout, nil)
		networkDisplay.ShowBrowserSlotDialog(dialog, widthPct, msgRows+1+2)
		tui.WireDialogNav(tuiApp, close, []tview.Primitive{confirmBtn, cancelBtn})
	}

	networkDisplay.SetLocalPeerHandlers(
		func(name string) {
			a.SetDisplayName(name)
			// Python LocalPeer.save_query (Network.py:1282-1295): the "Saved"
			// notice is a LineBox swapped into the left_pile LocalPeer slot
			// (contents[1]) at PACK height, titled with the info glyph — NOT a
			// centered modal. "\n\n\nSaved\n\n" = 6 message rows.
			networkDisplay.ShowLocalPeerStatus("\n\n\nSaved\n\n", 6)
		},
		func() {
			a.AnnounceNow()
			lxmfAddr, idhash, lann = localPeerInfo()
			networkDisplay.UpdateLocalPeer(lxmfAddr, idhash, lann)
			// Python LocalPeer.announce_query (Network.py:1305-1319): "Announce
			// Sent" LineBox in the LocalPeer slot, PACK height. 7 message rows.
			networkDisplay.ShowLocalPeerStatus("\n\n\nAnnounce Sent\n\n\n", 7)
		},
		func() {
			// Swap the left pile's PACK slot from Local Peer Info to the Local
			// Node Info panel (Python node_info_query, Network.py:1399-1401),
			// building the panel from the hosted node's live state. When no node
			// is hosted (EnableNode false) the panel renders the "not hosting a
			// node" branch (Python NodeInfo else-branch, Network.py:1541-1551).
			data := buildNodeInfoData(a, navigateTo, func() {
				networkDisplay.ShowLocalPeerStatus("\n\n\nAnnounce Sent\n\n\n", 7)
			})
			networkDisplay.ShowNodeInfo(data)
		},
	)

	// refreshAnnounces re-fetches the announce stream from the app and updates
	// the network display's left pane. It reads the persisted directory
	// announce stream (a.DirAnnounceEvents, mirroring Python's AnnounceStream
	// widget iterating app.directory.announce_stream, Network.py:489), which
	// is the single source of truth, so the panel populates at boot from the
	// previous run's discovered nodes loaded by Dir.LoadFromDisk.
	refreshAnnounces := func() {
		anns := a.DirAnnounceEvents()
		tuiConvs := make([]tui.AnnounceEntry, len(anns))
		for i, ann := range anns {
			tuiConvs[i] = tui.AnnounceEntry{
				Timestamp:   ann.Timestamp,
				TimestampF:  ann.TimestampF,
				SourceHash:  fmt.Sprintf("%x", ann.SourceHash),
				AppData:     string(ann.AppData),
				Type:        ann.AnnounceType,
				DisplayName: ann.DisplayName,
			}
		}
		networkDisplay.UpdateAnnounces(tuiConvs)
	}
	// refreshNodes re-fetches the saved-node list from the app directory.
	refreshNodes := func() {
		entries := a.Dir.KnownNodes()
		nodes := make([]tui.NodeEntry, 0, len(entries))
		for _, e := range entries {
			trustStr := "unknown"
			switch e.TrustLevel {
			case directory.TrustTrusted:
				trustStr = "trusted"
			case directory.TrustUntrusted:
				trustStr = "untrusted"
			case directory.TrustWarning:
				trustStr = "warning"
			}
			delivery := "direct"
			if e.PreferredDelivery == directory.DeliveryPropagated {
				delivery = "propagated"
			}
			nodes = append(nodes, tui.NodeEntry{
				SourceHash:  fmt.Sprintf("%x", e.SourceHash),
				DisplayName: e.DisplayName,
				TrustLevel:  trustStr,
				HostsNode:   e.HostsNode,
				Delivery:    delivery,
			})
		}
		networkDisplay.UpdateNodes(nodes)
	}
	// refreshAll is the full Network/Conversations/LocalPeer refresh, run on
	// the event loop. It is invoked through the debouncer below so a burst of
	// announces/messages collapses into a single refresh instead of one refresh
	// per event. Each refresh does an os.ReadDir for the conversation list plus
	// a full screen redraw, so N simultaneous announces would otherwise queue N
	// redundant refreshes (each spawning its own goroutine via the non-blocking
	// QueueUpdateDraw wrapper) and keep the event loop busy long enough to make
	// the UI appear hung — key events sit in the tcell queue while the loop
	// drains the pile. Python's directory_change_callback (Network.py:1744)
	// avoids this by doing only cheap in-memory widget rebuilds per announce;
	// the Go refresh is heavier, so coalescing is necessary for parity under a
	// burst (e.g. the path-response storm that follows announce-at-start).
	refreshAll := func() {
		tuiApp.QueueUpdateDraw(func() {
			refreshAnnounces()
			refreshNodes()
			refreshConvs()
			// RNS init runs asynchronously in a goroutine, so the identity/LXMF
			// destination are nil when wireDisplays first runs. Re-filling the
			// Local Peer Info panel on each UI change picks them up once initRNS
			// completes (it fires UIChangeCallback at the end), and also refreshes
			// the "Announced : …" line as the announce age advances. The name is
			// not part of this refresh — it was seeded once via SetLocalPeerName.
			lxmfAddr, idhash, lann := localPeerInfo()
			networkDisplay.UpdateLocalPeer(lxmfAddr, idhash, lann)
		})
	}

	// Debounce UIChangeCallback (fired from transport goroutines: one call per
	// incoming announce/message, plus the end of async initRNS). Resetting a
	// short timer on each fire coalesces a burst into one refreshAll, bounding
	// the refresh rate to ~1 per refreshCoalesceWindow regardless of how fast
	// announces arrive. An 80 ms window is imperceptible for a single event but
	// collapses a tight path-response storm (hundreds of announces within a
	// second or two) to a handful of refreshes.
	// maxWait bounds the SUSTAINED-storm case the plain debounce cannot: with
	// announces arriving faster than the window elapses, retriggering would
	// postpone refreshAll indefinitely (the UI freezes on stale data). The
	// 500 ms cap keeps the eventual refresh rate at ~2 Hz under a firehose
	// while leaving quiet and burst-y traffic at the plain 80 ms trailing-edge
	// behavior.
	const (
		refreshCoalesceWindow = 80 * time.Millisecond
		refreshMaxWait        = 500 * time.Millisecond
	)
	refreshTrigger := tui.NewDebouncedCallWithMaxWait(refreshCoalesceWindow, refreshMaxWait, refreshAll)
	a.SetUIChangeCallback(refreshTrigger.Trigger)
	main.SetDisplay("network", networkDisplay.Widget())
	main.SetShortcut("network", "[C-l] Nodes/Announces  [C-x] Remove  [C-w] Disconnect  [C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-g] Fullscreen  [C-s / C-b] Save Node")

	// Wire Esc to go back from AnnounceInfo before quitting.
	main.SetEscCallback(func() bool {
		return networkDisplay.HandleEsc()
	})

	// Wire network keyboard shortcuts
	networkDisplay.OnDeleteSelected = func() {
		if networkDisplay.ShowingNodes() {
			// Python KnownNodes.delete_selected_entry (Network.py:921-961): a
			// "?" dialog overlaid on the left_pile list slot, message
			// "Delete Node\n<display>\n", Yes/No, RELATIVE_100/PACK.
			node, ok := networkDisplay.SelectedNode()
			if !ok {
				return
			}
			displayStr := node.DisplayName
			if hash, ok := app.SourceHashFromHex(node.SourceHash); ok {
				displayStr = a.Dir.SimplestDisplayStr(hash)
			}
			if displayStr == "" {
				displayStr = "<" + node.SourceHash + ">"
			}
			networkDisplay.ShowListSlotConfirm("?", "Delete Node\n"+displayStr+"\n", func() {
				if hash, ok := app.SourceHashFromHex(node.SourceHash); ok {
					a.ForgetNode(hash)
				}
				refreshNodes()
			}, nil)
		} else {
			// Python AnnounceStream.delete_selected_entry (Network.py:468-473):
			// direct removal — NO confirmation dialog.
			ann, ok := networkDisplay.SelectedAnnounce()
			if !ok {
				return
			}
			a.RemoveAnnounce(ann.TimestampF)
			refreshAnnounces()
		}
	}
	networkDisplay.OnShowPeers = func() {
		// Python reinit_lxmf_peers (Network.py:1717): rebuild the peer list
		// from the LXMF message router before show_peers swaps it in.
		refreshLXMFPeers()
	}
	networkDisplay.OnLXMFPeerUnpeer = func(destinationHash []byte) {
		// Python LXMFPeers.delete_selected_entry (Network.py:1800-1806):
		// unpeer then reinit+show. The router guards out-of-order unpeers by
		// the peer's peering_timebase. The peers slot is already displayed
		// (C-x is pressed on it), so UpdateLXMFPeers refreshes it in place.
		if a.Router != nil {
			a.Router.Unpeer(destinationHash)
		}
		refreshLXMFPeers()
	}
	networkDisplay.OnLXMFPeerSync = func(destinationHash []byte) {
		// Python LXMFPeers.sync_selected_entry (Network.py:1808-1834): honor a
		// 10 s sync-grace window, then trigger peer.sync() in a goroutine and
		// show the "delivery sync requested" dialog (OK dismisses via
		// close_list_dialogs = reinit+show_peers).
		if a.Router == nil {
			return
		}
		peer := a.Router.PeerByHash(destinationHash)
		if peer == nil {
			return
		}
		const syncGrace = 10.0
		now := float64(time.Now().UnixNano()) / float64(time.Second)
		if now <= peer.LastSyncAttempt()+syncGrace {
			return
		}
		// Peer.Sync blocks on link requests; mirror Python's
		// threading.Thread(target=peer.sync).
		go peer.Sync()
		msg := tui.NewUrwidCenterText("\nA delivery sync of all unhandled LXMs was manually requested for the selected node\n")
		okBtn := tui.NewUrwidButton("OK").SetSelectedFunc(func() {
			tuiApp.Dialogs.DismissTop()
			refreshLXMFPeers()
		})
		buttons := tui.CreateUrwidButtonRow(okBtn)
		layout := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(msg, 0, 1, false).
			AddItem(buttons, 1, 0, true)
		tuiApp.Dialogs.ShowDialog("!", layout, 0, 6, nil)
	}
	networkDisplay.OnURLDialog = func() {
		// Python Browser.url_dialog (Browser.py:1135-1182): title "Enter URL",
		// caption "URL : ", Cancel/Go, overlaid on the browser pane at 65% width.
		// D2: with a browser frame present, nd.handleInput delegates C-u to the
		// frame's bd.OnURLDialog (which pre-fills with the live URL), so this
		// display-level handler now only fires when no browser frame exists —
		// the field is empty either way, matching Python's current_url() == "".
		showURLDialog("", func(text string) {
			navigateTo(browser.NormalizeEnteredURL(text))
		})
	}
	networkDisplay.OnSaveNode = func() {
		// Python NetworkLeftPile.keypress "ctrl s" (Network.py:1613-1614)
		// and BrowserFrame.keypress "ctrl s" (Browser.py:32-33) BOTH forward
		// to browser.save_node_dialog(), which shows a 50%-width overlay
		// dialog asking "Save connected node … to Known Nodes?" with
		// Cancel/Save buttons (Browser.py:1184-1234). The save only happens
		// on user confirmation — it is NOT an immediate silent save.
		//
		// When the browser has a connected destination, delegate to
		// bd.OnSaveNode which calls showBrowserConfirm to display that
		// dialog (matching Python). The handleInput InputCapture on mainCols
		// intercepts Ctrl-s before the browser's HandleKey sees it, so
		// bd.OnSaveNode would never fire without this delegation.
		if bd := networkDisplay.BrowserDisplay(); bd != nil {
			dest := bd.CurrentDest()
			if len(dest) > 0 {
				bd.OnSaveNode()
				return
			}
		}
		// Fall back to saving the selected announce/list entry when no
		// browser node is connected (a Go-specific extension; Python's
		// save_node_dialog is a no-op when no destination is connected).
		if ann, ok := networkDisplay.SelectedAnnounce(); ok {
			hash, ok := app.SourceHashFromHex(ann.SourceHash)
			if ok {
				a.SaveNode(hash, ann.DisplayName)
				refreshNodes()
				return
			}
		}
		if node, ok := networkDisplay.SelectedNode(); ok {
			hash, ok := app.SourceHashFromHex(node.SourceHash)
			if ok {
				a.SaveNode(hash, node.DisplayName)
				refreshNodes()
			}
		}
	}
	// OnSaveSpecificNode saves a specific announce entry (from the
	// AnnounceInfo "Save" button). It uses the entry's SourceHash and
	// DisplayName directly, instead of relying on the stream list selection.
	networkDisplay.OnSaveSpecificNode = func(ann tui.AnnounceEntry) {
		hash, ok := app.SourceHashFromHex(ann.SourceHash)
		if !ok {
			return
		}
		a.SaveNode(hash, ann.DisplayName)
		refreshNodes()
	}
	// Resolve directory-backed fields for the AnnounceInfo view (Python
	// AnnounceInfo __init__: trust_level, simplest_display_str, op_str). The
	// operator string for a node announce recalls the identity and derives the
	// lxmf.delivery hash (Network.py:138-144); a non-recallable identity falls
	// back to "Unknown" (Python raises; the TUI surfaces "Unknown" gracefully).
	networkDisplay.OnResolveAnnounceInfo = func(ann tui.AnnounceEntry) (tui.AnnounceInfoData, bool) {
		data := tui.AnnounceInfoData{OpStr: "Unknown"}
		hash, ok := app.SourceHashFromHex(ann.SourceHash)
		if !ok {
			data.DisplayStr = ann.DisplayName
			data.TrustStr = "Unknown"
			data.TrustStyle = "list_unknown"
			return data, true
		}
		data.DisplayStr = a.Dir.SimplestDisplayStr(hash)
		if ann.Type == "node" {
			data.OpStr = a.NodeOperatorDisplay(hash)
			if opHash := a.NodeOperatorHash(hash); opHash != nil {
				data.OpHash = fmt.Sprintf("%x", opHash)
			}
		}
		switch a.Dir.TrustLevel(hash, nil) {
		case directory.TrustTrusted:
			data.TrustStr = "Trusted"
			data.TrustStyle = "list_trusted"
		case directory.TrustUntrusted:
			data.TrustStr = "Untrusted"
			data.TrustStyle = "list_untrusted"
		case directory.TrustWarning:
			data.TrustStr = "Warning"
			data.TrustStyle = "list_untrusted"
		default:
			data.TrustStr = "Unknown"
			data.TrustStyle = "list_unknown"
		}
		return data, true
	}

	// OnResolveKnownNodeInfo resolves the directory-backed fields the
	// KnownNodeInfo form needs (Python KnownNodeInfo __init__, Network.py:612-
	// 740). The RNS-dependent fields (operator string via Identity.recall, hop
	// distance via Transport.hops_to, the PN address hash, the current
	// user-selected PN) need RNS identity recall and are stubbed here; identify-on-
	// connect, display name, sort rank and trust come from the directory.
	networkDisplay.OnResolveKnownNodeInfo = func(nodeHash string) (tui.KnownNodeInfoData, bool) {
		data := tui.KnownNodeInfoData{
			OpStr:       "Unknown",
			HopsStr:     "Unknown",
			LXMFAddrStr: "No associated Propagation Node known",
		}
		hash, ok := app.SourceHashFromHex(nodeHash)
		if !ok {
			data.DisplayStr = "<" + nodeHash + ">"
			data.SortStr = "None"
			data.TrustLevel = "unknown"
			return data, true
		}
		data.DisplayStr = a.Dir.SimplestDisplayStr(hash)
		if sr := a.Dir.SortRank(hash); sr == nil {
			data.SortStr = "None"
		} else {
			data.SortStr = strconv.Itoa(*sr)
		}
		data.IdentifyOnConnect = a.Dir.ShouldIdentifyOnConnect(hash)
		// RNS-derived fields (Network.py:629-634,681-688,791-800): operator display
		// (lxmf.delivery hash), hop distance (Transport.hops_to), the centered
		// LXMF Propagation Node address line (lxmf.propagation hash), and the
		// default-PN checkbox preselection (current user-selected PN == pn_hash).
		data.OpStr = a.NodeOperatorDisplay(hash)
		if opHash := a.NodeOperatorHash(hash); opHash != nil {
			data.OpHash = fmt.Sprintf("%x", opHash)
		}
		data.HopsStr = tui.FormatHopsStr(a.PeerHops(hash))
		pnHash := a.NodePropagationHash(hash)
		data.LXMFAddrStr = tui.FormatLXMFAddrStr(pnHash, tuiApp.Glyphs["sent"])
		if pnHash != nil {
			if sel := a.GetUserSelectedPropagationNode(); sel != nil && bytes.Equal(sel, pnHash) {
				data.UseAsPN = true
			}
		}
		switch a.Dir.TrustLevel(hash, nil) {
		case directory.TrustTrusted:
			data.TrustLevel = "trusted"
		case directory.TrustUntrusted:
			data.TrustLevel = "untrusted"
		case directory.TrustWarning:
			data.TrustLevel = "warning"
		default:
			data.TrustLevel = "unknown"
		}
		return data, true
	}

	// OnKnownNodeSave writes the edited KnownNodeInfo form to the directory
	// (Python save_node, Network.py:755-785). The default-PN
	// toggle and autoselect need RNS (set_user_selected_propagation_node,
	// autoselect_propagation_node) and are stubbed; trust/name/sort/identify
	// are written to the directory entry.
	networkDisplay.OnKnownNodeSave = func(nodeHash string, fd tui.KnownNodeInfoFormData) {
		hash, ok := app.SourceHashFromHex(nodeHash)
		if !ok {
			return
		}
		entry := directory.NewEntry(hash)
		entry.DisplayName = fd.Name
		entry.HostsNode = true
		entry.IdentifyOnConnect = fd.IdentifyOnConnect
		switch fd.TrustLevel {
		case "trusted":
			entry.TrustLevel = directory.TrustTrusted
		case "untrusted":
			entry.TrustLevel = directory.TrustUntrusted
		case "unknown":
			entry.TrustLevel = directory.TrustUnknown
		default:
			entry.TrustLevel = directory.TrustWarning
		}
		if n, err := strconv.Atoi(fd.SortRank); err == nil && n >= 0 {
			entry.SortRank = &n
		}
		a.Dir.Remember(entry)
		// Default-PN toggle (Python save_node, Network.py:730-734): only act if
		// the user toggled the checkbox. Checked → set the PN to this node's
		// lxmf.propagation hash; unchecked → clear the selection.
		if fd.UseAsPNChanged {
			if fd.UseAsPN {
				if pn := a.NodePropagationHash(hash); pn != nil {
					a.SetUserSelectedPropagationNode(pn)
				}
			} else {
				a.SetUserSelectedPropagationNode(nil)
			}
		}
		// autoselect runs when the saved trust becomes Trusted (Network.py:757-758).
		if entry.TrustLevel == directory.TrustTrusted {
			a.AutoSelectPropagationNode()
		}
	}

	// Ctrl-E opens the KnownNodeInfo form for the selected saved node (Python
	// NetworkLeftPile.keypress ctrl e → selected_node_info, Network.py:1603).
	networkDisplay.OnEditNode = func() {
		if node, ok := networkDisplay.SelectedNode(); ok {
			networkDisplay.ShowKnownNodeInfo(node.SourceHash)
		}
	}

	// Conversations display
	convs := a.ConversationList()
	tuiConvs := make([]tui.ConversationInfo, len(convs))
	for i, c := range convs {
		trustStr := "unknown"
		switch c.TrustLevel {
		case 0xFF:
			trustStr = "trusted"
		case 0x01:
			trustStr = "untrusted"
		case 0x00:
			trustStr = "warning"
		}

		var lastTime time.Time
		if c.LastActivity > 0 {
			lastTime = time.Unix(int64(c.LastActivity), 0)
		}

		tuiConvs[i] = tui.ConversationInfo{
			SourceHash:  c.SourceHash,
			DisplayName: c.DisplayName,
			TrustLevel:  trustStr,
			LastTime:    lastTime,
			Unread:      c.Unread,
			UnreadCount: c.UnreadCount,
			Failed:      c.Failed,
			FailedCount: c.FailedCount,
			Pinned:      c.SortRank != nil,
		}
	}
	conversationsDisplay := tui.NewConversationsDisplay(tuiApp, tuiConvs)
	main.SetDisplay("conversations", conversationsDisplay.Widget())

	// Conversations supplies its shortcut bar dynamically (it switches between
	// the list/editor/body bars by focus region). Other pages use their static
	// SetShortcut text. Registered per-page so it only applies to conversations.
	main.SetShortcutCallback("conversations", func() string {
		return conversationsDisplay.GetShortcutText()
	})

	// refreshConvs updates the conversation list from the app.
	refreshConvs = func() {
		newConvs := a.ConversationList()
		tuiConvs = make([]tui.ConversationInfo, 0, len(newConvs)+len(a.IgnoredList))
		seenIgnored := make(map[string]bool)
		for _, c := range newConvs {
			trustStr := "unknown"
			switch c.TrustLevel {
			case 0xFF:
				trustStr = "trusted"
			case 0x01:
				trustStr = "untrusted"
			case 0x00:
				trustStr = "warning"
			}
			// An ignored (blocked) destination renders Python's dedicated
			// "[blocked]" row (Conversations.py:332-341 + update_listbox
			// blocked-row append, Conversations.py:483-488) instead of its
			// trust-derived row, so activating it offers the Unblock dialog.
			if hashBytes, ok := app.SourceHashFromHex(c.SourceHash); ok && a.IsIgnored(hashBytes) {
				trustStr = "blocked"
				seenIgnored[c.SourceHash] = true
			}
			var lastTime time.Time
			if c.LastActivity > 0 {
				lastTime = time.Unix(int64(c.LastActivity), 0)
			}
			tuiConvs = append(tuiConvs, tui.ConversationInfo{
				SourceHash:  c.SourceHash,
				DisplayName: c.DisplayName,
				TrustLevel:  trustStr,
				LastTime:    lastTime,
				Unread:      c.Unread,
				UnreadCount: c.UnreadCount,
				Failed:      c.Failed,
				FailedCount: c.FailedCount,
				Pinned:      c.SortRank != nil,
			})
		}
		// Python's update_listbox appends a visible row for EVERY entry of
		// app.ignored_list (Conversations.py:483-488), including peers with no
		// conversation on disk — without those rows a peer blocked before the
		// first conversation could never be unblocked.
		for _, h := range a.IgnoredList {
			hashHex := fmt.Sprintf("%x", h)
			if seenIgnored[hashHex] {
				continue
			}
			tuiConvs = append(tuiConvs, tui.ConversationInfo{
				SourceHash:  hashHex,
				DisplayName: a.Dir.SimplestDisplayStr(h),
				TrustLevel:  "blocked",
			})
		}
		conversationsDisplay.SetConversations(tuiConvs)
	}

	// "Last sync:" footer reads the persisted peer_settings["last_lxmf_sync"]
	// live (Python _sync_status_line, Conversations.py:517-545). Refresh every
	// 30 s to age the relative time, matching Python's set_alarm_in(30,
	// _refresh_sync_status). Also refreshed after a sync completes (below).
	// The same tick re-renders the open conversation's message list when any
	// per-message relative-time header ("1m ago") changed on the wall clock —
	// those labels are computed at render time and would otherwise freeze
	// until the next event-driven reload.
	conversationsDisplay.LastSyncInfo = a.LastSyncInfo
	conversationsDisplay.RefreshSyncStatus()
	tuiApp.GoSafe(func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			conversationsDisplay.RefreshSyncStatus()
			tuiApp.QueueUpdateDraw(func() {
				conversationsDisplay.RefreshRelativeTimes()
			})
		}
	})

	// Wire conversation keyboard shortcuts. OnDeleteConv forwards the SELECTED
	// conversation as resolved by the display against the active tab's
	// RENDERED rows (Python reads source_hash from
	// self.ilb.get_selected_item(), Conversations.py:561-566). The former
	// wiring resolved it here by list index against the UNFILTERED
	// conversation list; with the Untrusted tab active the filtered row
	// index maps onto a trusted row of the full model, so Ctrl-X offered —
	// and on "Yes" deleted — the wrong (trusted) conversation.
	conversationsDisplay.OnDeleteConv = func(conv tui.ConversationInfo) {
		// Name the peer as Python does: directory.simplest_display_str of the
		// source hash (Conversations.py:579-581), falling back to the row's
		// display name for a malformed hash.
		name := conv.DisplayName
		if hash, ok := app.SourceHashFromHex(conv.SourceHash); ok {
			name = a.Dir.SimplestDisplayStr(hash)
		}
		tuiApp.Dialogs.ShowConfirmDialog("Delete conversation with "+name+"?",
			func() {
				a.DeleteConversation(conv.SourceHash)
				refreshConvs()
			},
			func() {},
		)
	}
	conversationsDisplay.OnNewConv = func() {
		conversationsDisplay.ShowNewConversationDialog(func(addrHex, name, trust string) bool {
			if addrHex == "" {
				return false
			}
			// Python new_conversation confirmed() (Conversations.py:1063-1093):
			// the entry and the on-disk conversation directory are created ONLY
			// when the peer does not have a conversation yet — a peer that
			// already has one is left untouched, so re-creating it with an
			// empty Name field cannot wipe the stored display name, and the
			// chosen trust applies only to a genuinely new peer. OpenLXMFLink
			// provides that create path: it guards on existing conversations,
			// recalls the announced app-data display name (falling back to the
			// typed name), remembers the entry, and creates the conversation
			// directory (Python Conversation(source_hash, initiator=True)).
			isNew, err := a.OpenLXMFLink(addrHex, name)
			if err != nil {
				// Python catches the parse failure and shows the dialog's
				// centered "Could not start conversation. Check your input."
				return false
			}
			if isNew {
				// Python's confirmed() sets the typed display name and the
				// chosen trust level only on the create path.
				if name != "" {
					if hash, ok := app.SourceHashFromHex(addrHex); ok {
						a.SetPeerDisplayName(hash, name)
					}
				}
				var trustByte byte
				switch trust {
				case "trusted":
					trustByte = directory.TrustTrusted
				case "unknown":
					trustByte = directory.TrustUnknown
				default:
					trustByte = directory.TrustUntrusted
				}
				if hash, ok := app.SourceHashFromHex(addrHex); ok {
					a.SetPeerTrustLevel(hash, trustByte)
				}
			}
			// Reveal the new entry: switch to the Untrusted tab unless the
			// entry was created trusted (Conversations.py:1066-1068).
			if trust != "trusted" {
				conversationsDisplay.SetShowTrusted(false)
			}
			refreshConvs()
			// B1: nomadnet 1.2.8 does NOT auto-open the conversation after
			// Create — it returns to the list with the right pane showing
			// "No conversation selected". The user must select the peer and
			// press Enter to open it. Do NOT call DisplayConversation here.
			return true
		})
	}
	conversationsDisplay.OnToggleSort = func() {
		conversationsDisplay.ToggleSort()
	}
	conversationsDisplay.OnShowQR = func() {
		// C5: the My LXMF dialog is a WHOLE-display 70%-relative overlay with
		// the QR, "< addr >" and a Close button (Python show_my_qr →
		// show_qr_dialog, Conversations.py:630-692) — built inside the tui
		// package, which owns the slot mechanics.
		conversationsDisplay.ShowMyQRDialog(a.LXMFAddressHex())
	}
	conversationsDisplay.OnEditPeerInfo = func() {
		conv, ok := conversationsDisplay.GetSelectedConversation()
		if !ok {
			return
		}
		hash, ok := app.SourceHashFromHex(conv.SourceHash)
		if !ok {
			return
		}
		// Load the current directory entry to pre-fill the dialog (Python
		// edit_selected_in_directory existing_entry lookup, Conversations.py:
		// 844-876).
		data := a.PeerInfoLoad(hash)
		entry := tui.PeerInfoEntry{
			SourceHash:  conv.SourceHash,
			DisplayName: data.DisplayName,
			Pinned:      data.Pinned,
			Notes:       data.Notes,
		}
		switch data.TrustLevel {
		case directory.TrustTrusted:
			entry.TrustLevel = tui.TrustTrusted
		case directory.TrustUntrusted:
			entry.TrustLevel = tui.TrustUntrusted
		default:
			entry.TrustLevel = tui.TrustUnknown
		}
		if data.PreferredDelivery == directory.DeliveryPropagated {
			entry.PreferredDelivery = "propagated"
		} else {
			entry.PreferredDelivery = "direct"
		}

		conversationsDisplay.ShowPeerInfoDialog(entry, tui.PeerInfoDialogHooks{
			IsKnown: func(sourceHash string) bool {
				if h, ok := app.SourceHashFromHex(sourceHash); ok {
					return a.Dir.IsKnown(h)
				}
				return false
			},
			OnQueryKeys: func(sourceHash string) {
				tuiApp.Dialogs.DismissTop()
				if h, ok := app.SourceHashFromHex(sourceHash); ok {
					a.QueryForPeer(h)
				}
			},
			// Ping opens an outbound lxmf.delivery link to the peer and reports
			// "Pong in N ms (M hops)" on establishment (Python
			// _ping_peer_from_dialog, Conversations.py:705-768). PingPeer's link
			// established/closed callbacks run on the RNS worker goroutine, but
			// setStatus updates a tview TextView, so marshal every update onto
			// the UI thread via QueueUpdateDraw (matches Python's schedule_ui).
			OnPing: func(sourceHash string, setStatus func(string)) {
				marshaled := func(s string) {
					tuiApp.QueueUpdateDraw(func() { setStatus(s) })
				}
				a.PingPeer(sourceHash, marshaled)
			},
			OnBlock: func(sourceHash string) {
				// Python _block_peer_from_dialog shows a confirm sub-dialog,
				// then block_destination + delete_conversation (Conversations.py:
				// 769-800). We confirm then block + delete the conversation.
				who := sourceHash
				tuiApp.Dialogs.DismissTop()
				tuiApp.Dialogs.ShowConfirmDialog("Block "+who+"?", func() {
					if h, ok := app.SourceHashFromHex(sourceHash); ok {
						a.BlockPeer(h, "user-blocked from peer info dialog")
						a.DeleteConversation(sourceHash)
						refreshConvs()
					}
				}, func() {})
			},
			OnLXMFQR: func(sourceHash, title string) {
				tuiApp.Dialogs.DismissTop()
				dialogTitle := "LXMF"
				if title != "" {
					dialogTitle = title
				}
				qr, err := tui.GenerateQRASCII(sourceHash)
				body := tview.NewTextView().
					SetDynamicColors(true).
					SetTextAlign(tview.AlignCenter)
				if err != nil || qr == "" {
					body.SetText("[gray]" + sourceHash + "[-]")
				} else {
					body.SetText("[white]" + qr + "[-]")
				}
				tuiApp.Dialogs.ShowDialog(dialogTitle, body, 50, 16, nil)
			},
		}, func(result tui.PeerInfoEntry) {
			h, ok := app.SourceHashFromHex(result.SourceHash)
			if !ok {
				return
			}
			var trust byte
			switch result.TrustLevel {
			case tui.TrustTrusted:
				trust = directory.TrustTrusted
			case tui.TrustUntrusted:
				trust = directory.TrustUntrusted
			default:
				trust = directory.TrustUnknown
			}
			delivery := directory.DeliveryDirect
			if result.PreferredDelivery == "propagated" {
				delivery = directory.DeliveryPropagated
			}
			a.RememberPeerInfo(h, app.PeerInfoData{
				DisplayName:       result.DisplayName,
				TrustLevel:        trust,
				PreferredDelivery: delivery,
				Pinned:            result.Pinned,
				Notes:             result.Notes,
			})
			// Python confirmed(): refresh the open conversation widget, the
			// conversation list, and fire directory_change_callback (which
			// reloads the network announce/nodes panes).
			refreshConvs()
			refreshAnnounces()
			refreshNodes()
			tuiApp.Dialogs.DismissTop()
		})
	}
	conversationsDisplay.OnIngestURI = func() {
		// The ingest dialog is a LIST-SLOT overlay in the left column with
		// Python's exact strings (title "Ingest message URI", "URI : " caption,
		// Ingest/Back buttons) — IngestURIDialog (Conversations.py:1118-1268);
		// the generic DialogManager input dialog centered on the whole screen
		// with Save/Cancel labels is the old, non-parity form.
		conversationsDisplay.IngestURIDialog(func(uri string) {
			if a.Router == nil {
				conversationsDisplay.ShowIngestResult(tui.IngestError)
				return
			}
			outcome, err := a.Router.IngestLXMURIOutcome(uri)
			if err != nil || outcome == lxmf.IngestOutcomeNone {
				conversationsDisplay.ShowIngestResult(tui.IngestError)
				return
			}
			var result tui.IngestResult
			switch outcome {
			case lxmf.IngestOutcomeLocalDelivery:
				result = tui.IngestSuccess
			case lxmf.IngestOutcomeDuplicate:
				result = tui.IngestDuplicate
			case lxmf.IngestOutcomePropagated:
				result = tui.IngestPropagated
			case lxmf.IngestOutcomeDiscarded:
				result = tui.IngestDiscarded
			default:
				result = tui.IngestError
			}
			conversationsDisplay.ShowIngestResult(result)
			if outcome == lxmf.IngestOutcomeLocalDelivery {
				refreshConvs()
				conversationsDisplay.ReloadCurrentMessages()
			}
		})
	}
	conversationsDisplay.OnSync = func() {
		// Propagation node display label: the node hash currently in use, if any.
		currentPN := ""
		if h := a.GetDefaultPropagationNode(); len(h) > 0 {
			currentPN = fmt.Sprintf("%x", h)
		}
		conversationsDisplay.ShowSyncDialog(
			currentPN,
			nil,
			tui.SyncDialogHooks{
				Progress:    a.GetSyncProgress,
				Status:      a.GetSyncStatus,
				ShowPercent: a.SyncStatusShowPercent,
			},
			func(result tui.SyncDialogResult) {
				switch result.Action {
				case "sync":
					limit := 0
					if result.Mode == tui.SyncLimited {
						limit = result.Limit
					}
					a.RequestLXMFSync(limit)
					conversationsDisplay.RefreshSyncStatus()
				case "cancel":
					a.CancelLXMFSync()
				case "dismiss":
					// Python dismiss_dialog cancels the sync if it had already
					// completed (transfer_state >= PR_COMPLETE), since there is
					// nothing left to receive but the state is non-idle.
					if a.Router != nil {
						st := a.Router.PropagationTransferState()
						if st >= lxmf.PRComplete {
							a.CancelLXMFSync()
						}
					}
					refreshConvs()
				}
			},
		)
		// Start the live progress refresh (200ms) on the running event loop.
		conversationsDisplay.StartSyncRefresh(true)
	}

	// Block/Unblock peer callbacks — used in conversation context
	blockPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowConfirmDialog("Block this peer?\nThis will blackhole their identity and delete this conversation.",
			func() {
				hash, ok := app.SourceHashFromHex(sourceHash)
				if !ok {
					return
				}
				a.BlockDestination(hash, "blocked from conversations")
				a.DeleteConversation(sourceHash)
				refreshConvs()
			},
			func() {},
		)
	}
	unblockPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowConfirmDialog("Unblock this peer?\nThis lifts the blackhole and removes them from your ignored list.",
			func() {
				hash, ok := app.SourceHashFromHex(sourceHash)
				if !ok {
					return
				}
				a.UnblockDestination(hash)
				refreshConvs()
			},
			func() {},
		)
	}
	pingPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowDialog("Ping",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText(fmt.Sprintf("[gray]Pinging %v...[-]", sourceHash[:8])),
			40, 5, nil)
		// TODO: Actual ping via RNS transport
	}
	_ = blockPeer
	_ = pingPeer

	// A "[blocked]" row in the Untrusted tab runs the unblock flow (Python's
	// blocked-row click → _unblock_dialog, Conversations.py:332-347).
	conversationsDisplay.OnUnblockPeer = unblockPeer

	// Trust banner button wiring (Python _on_trust_click/_on_block_click/
	// _on_ignore_click). Defined after blockPeer so it can delegate to it.
	conversationsDisplay.OnTrustPeer = func(sourceHash string) {
		hash, ok := app.SourceHashFromHex(sourceHash)
		if !ok {
			return
		}
		a.SetPeerTrustLevel(hash, directory.TrustTrusted)
		refreshConvs()
	}
	conversationsDisplay.OnBlockPeer = func(sourceHash string) {
		blockPeer(sourceHash)
	}
	conversationsDisplay.OnIgnorePeer = func(sourceHash string) {
		// "Do nothing" just dismisses the banner; no app action.
		_ = sourceHash
	}

	// Go-only enhancement (no Python SOT counterpart — Python nomadnet has no
	// way to block a node): the AnnounceInfo "Block" button blackholes the
	// node's identity (App.BlockDestination → Transport.BlackholeIdentity +
	// ignored list + LXMF router ignore) and the directory's Go-only blocked
	// filter drops its announces from the Announce Stream on the refresh below.
	// Keep this hook (and the dialog/blocked-filter comments) when auditing
	// parity against the Python original.
	networkDisplay.OnBlockNode = func(nodeHash string) {
		hash, ok := app.SourceHashFromHex(nodeHash)
		if !ok {
			return
		}
		a.BlockDestination(hash, "user-blocked from node announce info (Go enhancement)")
		refreshAll()
	}

	// Send hook: C-d in the open conversation's composer builds the outbound
	// LXMF message (App.SendConversation wires Conversation.SetSendDeps and
	// dispatches via the router) and refreshes the list so the new message's
	// last-activity/unread state shows. This closes the "Wire conversation
	// send" gap (TODO).
	conversationsDisplay.OnSend = func(sourceHash, content, title string, attachments []string) {
		a.SendConversation(sourceHash, content, title, attachments...)
		refreshConvs()
		conversationsDisplay.ReloadCurrentMessages()
	}

	// Message-loading hooks: supply the open conversation's messages (parsed
	// from disk via App.ConversationMessages), this app's own LXMF hash (so
	// the LXMessageWidget header can tell outbound from inbound), and the
	// configured time format. Together these let the ConversationWidget render
	// Python-parity message headers + bodies ("ConversationWidget
	// — messages").
	conversationsDisplay.OnLoadMessages = func(sourceHash string) []tui.ConversationMessage {
		return toConversationMessages(a.ConversationMessages(sourceHash))
	}
	conversationsDisplay.OnOwnHash = func() []byte {
		if a.LXMFDest == nil {
			return nil
		}
		return a.LXMFDest.Hash
	}
	// Peer-info bar RNS fields (Python _update_peer_info, Conversations.py:
	// 2103-2112): the outbound stamp cost (router + recalled-app-data fallback)
	// and the transport hop count. nil leaves the "Stamp:" segment off / hops
	// as "unknown", exactly as Python renders when the data is unavailable.
	conversationsDisplay.OnStampCost = func(sourceHash string) *int {
		if h, ok := app.SourceHashFromHex(sourceHash); ok {
			return a.PeerStampCost(h)
		}
		return nil
	}
	conversationsDisplay.OnHops = func(sourceHash string) *int {
		if h, ok := app.SourceHashFromHex(sourceHash); ok {
			return a.PeerHops(h)
		}
		return nil
	}
	// Live trust resolver: the conversation widget's trust banner reads the
	// directory's current trust on each render (Python has_visible_trust_banner
	// calls self.app.directory.trust_level live, Conversations.py:1957-1960),
	// instead of relying on the TrustLevel snapshot captured at
	// DisplayConversation time — which can be stale (e.g. "" because the
	// conversation dir didn't exist yet when the New Conversation dialog created
	// the trusted entry, so the banner wrongly showed for a trusted peer).
	conversationsDisplay.OnResolveTrust = func(sourceHash string) string {
		h, ok := app.SourceHashFromHex(sourceHash)
		if !ok {
			return ""
		}
		switch a.Dir.TrustLevel(h, nil) {
		case directory.TrustTrusted:
			return "trusted"
		case directory.TrustUntrusted:
			return "untrusted"
		case directory.TrustWarning:
			return "warning"
		default:
			return "unknown"
		}
	}
	// Receive → open-conversation refresh. Ingest fires OnChanged from the LXMF
	// delivery goroutine when a message lands on disk (mirrors Python's
	// Conversation.ingest → scan_storage → __changed_callback →
	// ConversationWidget.conversation_changed → update_message_widgets,
	// Conversation.py:71-80 + 271-272, Conversations.py:1896 + 2246-2252).
	// Marshal to the UI thread, refresh the conversation LIST (so the unread
	// badge updates — Python created_callback → update_conversation_list), then
	// reload the OPEN conversation's message list + trust banner so an inbound
	// message appears in the already-open view without a manual interaction.
	// Without this the list refreshed on receive but the open body stayed "No
	// messages yet" until the next send/interaction triggered a reload.
	if a.ConversationCache != nil {
		a.ConversationCache.OnChanged = func(sourceHex string) {
			tuiApp.QueueUpdateDraw(func() {
				refreshConvs()
				conversationsDisplay.RefreshOpenConversation(sourceHex)
			})
		}
	}
	// Paper message hook: C-p in the open conversation's composer builds the
	// paper (offline) LXMF message via App.PaperMessage (print_qr / save_qr /
	// save_uri). On success the saved-path / failure dialogs are rendered by the
	// display; a successful send also refreshes the conversation list so the
	// ingested paper message shows.
	conversationsDisplay.OnPaperMessage = func(sourceHash, action, content, title string) (string, bool) {
		path, ok := a.PaperMessage(sourceHash, action, content, title)
		if ok {
			refreshConvs()
			conversationsDisplay.ReloadCurrentMessages()
		}
		return path, ok
	}
	conversationsDisplay.OnPaperMessageSaved = func(path string) {
		conversationsDisplay.PaperMessageSaved(path)
	}
	conversationsDisplay.OnPaperMessageFailed = func() {
		conversationsDisplay.PaperMessageFailed()
	}
	// Save-attachments hook: C-s collects received-attachment refs (each
	// carrying the owning message hash + field index); "Copy to Downloads" in
	// the save dialog copies the extracted attachment files to the download
	// directory via App.SaveConversationAttachments (Python do_save,
	// Conversations.py:2368-2391).
	conversationsDisplay.OnSaveAttachments = func(sourceHash string, refs []tui.AttachmentRef) ([]string, int) {
		selections := make([]conversation.SaveAttachmentSelection, len(refs))
		for i, r := range refs {
			selections[i] = conversation.SaveAttachmentSelection{
				MessageHash: r.MessageHash,
				FieldType:   r.Type,
				FieldIndex:  r.FieldIndex,
				Name:        r.Name,
			}
		}
		return a.SaveConversationAttachments(sourceHash, selections)
	}

	// Network "Converse" / "Msg Op" (Python converse, Network.py:168-187, and
	// msg_op, Network.py:146-166): create a directory entry + on-disk
	// conversation for the target LXMF address, refresh the conversation list,
	// display the conversation, and switch to the Conversations page. The
	// target is the announce's own address for Converse and the operator's
	// derived "lxmf.delivery" hash for Msg Op — an empty target (the operator
	// identity was not recallable) skips the action, as Python's KeyError
	// branch does.
	ensureConversationAndShow := func(targetHashHex, displayName string) {
		if targetHashHex == "" {
			return
		}
		if _, err := a.OpenLXMFLink(targetHashHex, displayName); err != nil {
			a.Logger.Error("could not start conversation from announce: %v", err)
			return
		}
		refreshConvs()
		conversationsDisplay.DisplayConversation(targetHashHex)
		main.SelectPage("conversations")
	}
	networkDisplay.OnConverse = func() {
		ann, ok := networkDisplay.SelectedAnnounce()
		if !ok {
			return
		}
		ensureConversationAndShow(ann.SourceHash, ann.DisplayName)
	}
	networkDisplay.OnMsgOp = func(opHashHex string) {
		opHash, ok := app.SourceHashFromHex(opHashHex)
		if !ok {
			return
		}
		ensureConversationAndShow(opHashHex, a.Dir.SimplestDisplayStr(opHash))
	}

	// Channels display
	channelsDisplay := tui.NewChannelsDisplay(tuiApp, nil)
	main.SetDisplay("channels", channelsDisplay.Widget())
	main.SetShortcut("channels", "[C-n] New Hub  [C-a] Add Room  [C-r] Connect  [C-w] Disconnect  [C-t] Auto-reconnect  [C-e] Edit Hub  [C-x] Remove")

	// Populate the hub/room list from the RRC manager (mirrors Python
	// _compose_list_widgets, Channels.py:1599-1662). The initial populate runs
	// directly on the UI goroutine before tuiApp.Run starts the event loop, so
	// QueueUpdateDraw (which blocks until the loop drains it) cannot be used
	// here. The RRC change callback (fired on the RRC worker goroutine when a
	// hub is added/removed/connected/status-changed) re-populates via
	// QueueUpdateDraw, marshaling onto the UI loop.
	refreshChannels := func() { channelsDisplay.SetHubs(a.HubViews()) }
	refreshChannels()
	if a.RRC != nil {
		a.RRC.SetChangeCallback(func() {
			tuiApp.QueueUpdateDraw(refreshChannels)
		})
	}

	// Wire channel keyboard shortcuts
	channelsDisplay.OnNewHub = func() {
		channelsDisplay.NewHubDialog()
	}
	channelsDisplay.OnJoinRoom = func() {
		channelsDisplay.JoinRoomDialog()
	}
	channelsDisplay.OnRemoveHub = func() {
		channelsDisplay.RemoveSelectedDialog()
	}
	channelsDisplay.OnEditHub = func() {
		channelsDisplay.EditHubDialog()
	}
	channelsDisplay.OnConnect = func() {
		if entry, ok := channelsDisplay.SelectedEntry(); ok && a.RRC != nil {
			hubs := a.RRC.HubsSnapshot()
			if entry.HubIdx >= 0 && entry.HubIdx < len(hubs) {
				hubs[entry.HubIdx].ConnectAsync()
			}
		}
	}
	channelsDisplay.OnDisconnect = func() {
		if entry, ok := channelsDisplay.SelectedEntry(); ok && a.RRC != nil {
			hubs := a.RRC.HubsSnapshot()
			if entry.HubIdx >= 0 && entry.HubIdx < len(hubs) {
				hubs[entry.HubIdx].Disconnect()
			}
		}
	}
	channelsDisplay.OnToggleAutoReconnect = func() {
		if entry, ok := channelsDisplay.SelectedEntry(); ok && a.RRC != nil {
			hubs := a.RRC.HubsSnapshot()
			if entry.HubIdx >= 0 && entry.HubIdx < len(hubs) {
				hub := hubs[entry.HubIdx]
				hub.SetAutoReconnect(!hub.AutoReconnect, true)
			}
		}
	}

	// Config display
	configPath := a.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(a.ConfigDir, "config")
	}
	configDisplay := tui.NewConfigDisplay(tuiApp, configPath)
	configDisplay.SetEditorCmd(tui.ResolveEditorCmd(a.Config.TextUI.Editor))
	main.SetDisplay("config", configDisplay.Widget())
	main.SetShortcut("config", "")

	// Log display
	logPath := a.LogFilePath
	if logPath == "" {
		logPath = filepath.Join(a.ConfigDir, "logfile")
	}
	logDisplay := tui.NewLogDisplay(tuiApp, logPath, 50)
	main.SetDisplay("log", logDisplay.Widget())
	main.SetShortcut("log", "")

	// Route focus-invariant violation dumps (a nil a.focus — the "arrow keys do
	// nothing" lock-up root cause) into the application logger, which writes
	// logPath — the same file the "[ Log ]" menu tails — so a stack trace
	// surfaces in-menu instead of only in a /tmp scratch file. The dump still
	// fires from MainDisplay.handleInput / App.SetFocus; this just chooses the
	// destination. a.Logger writes at or above Notice, so Error always emits.
	logger := a.Logger
	tui.SetFocusInvariantSink(func(msg string, stack []byte) {
		if logger != nil {
			logger.Error("FOCUS INVARIANT VIOLATION: %s\n%s", msg, stack)
		}
	})

	// Begin live tailing (Python's LogTerminal runs `tail -fn50` continuously
	// while the page exists). It is a no-op when the log file is absent. The
	// returned cleanup stops the goroutine on shutdown.
	logDisplay.StartTailing()

	// Guide display
	guideDisplay := tui.NewGuideDisplay(tuiApp)
	main.SetDisplay("guide", guideDisplay.Widget())
	main.SetShortcutCallback("guide", guideDisplay.Shortcuts)
	// Wire Guide link activation to the Network browser, mirroring Python's
	// GuideLinkDelegate.handle_link (Guide.py:103-118): an "#anchor" stays in
	// the Guide (handled in GuideDisplay.handleLink before this callback), any
	// other target switches to the Network page and dispatches through the
	// browser's handle_link (here BrowserDisplay.HandleLink), which routes
	// nomadnetwork.node page URLs → OnRetrieveURL (the fetch backend), lxmf@ →
	// OnOpenLXMF, rrc:// → OnOpenRRC, p: → OnPartialUpdate. Without this the
	// Guide reader's OnHandleLink was nil, so clicking/Enter on a non-anchor
	// link like the Introduction's "Aleph git" page link did NOTHING — the user
	// could not click it (Python's mouse_event fires handle_link directly, so
	// nomadnet could). The link's field-names component is now threaded through
	// to HandleLink so a Guide submit link can collect form fields like a page
	// link (Python recurse_down).
	guideDisplay.OnHandleLink = func(target, fields string) {
		main.SelectPage("network")
		if ndBd := networkDisplay.BrowserDisplay(); ndBd != nil {
			ndBd.HandleLink(target, fields)
		}
	}

	// Interfaces display — wired to the live RNS transport. Python builds the
	// list from the RNS config + app.rns.get_interface_stats() (Interfaces.py:
	// 2864-2897) and refreshes TX/RX every 1 s (poll_stats, set_alarm_in(1)).
	// Go reads App.InterfaceStats() (Transport.GetInterfaces: name/type, live
	// Status, BytesSent/Received) which returns the running interfaces. RNS init
	// is asynchronous, so the list is empty until initRNS completes; a 1 s ticker
	// then populates it and keeps TX/RX/status live. Disabled-in-config
	// interfaces are skipped by RNS (rns.go: ifaceEnabled gate) and so do not
	// appear here — a known gap vs Python, which enumerates them from the config.
	interfacesDisplay := tui.NewInterfacesDisplay(tuiApp, nil)
	main.SetDisplay("interfaces", interfacesDisplay.Widget())
	main.SetShortcut("interfaces", "[C-a] Add Interface [C-e] Edit Interface [C-x] Remove Interface [Enter] Show Interface [C-w] Open Text Editor")

	// interfaceInfos snapshots the live transport interfaces into the
	// InterfaceInfo shape the display consumes.
	interfaceInfos := func() []tui.InterfaceInfo {
		stats := a.InterfaceStats()
		infos := make([]tui.InterfaceInfo, 0, len(stats))
		for _, s := range stats {
			infos = append(infos, tui.InterfaceInfo{
				Name:      s.Name,
				Type:      s.Type,
				Enabled:   s.Enabled,
				Connected: s.Connected,
				TX:        s.TX,
				RX:        s.RX,
				Bitrate:   s.Bitrate,
			})
		}
		return infos
	}
	// refreshInterfaces is called from the 1 s ticker goroutine, so it must
	// marshal the SetInterfaces call onto the UI loop via QueueUpdateDraw.
	refreshInterfaces := func() {
		infos := interfaceInfos()
		tuiApp.QueueUpdateDraw(func() { interfacesDisplay.SetInterfaces(infos) })
	}
	// Initial populate. wireDisplays runs on the goroutine that will become the
	// UI loop, BEFORE tuiApp.Run starts the event loop, so QueueUpdateDraw
	// (which blocks until the loop drains it) cannot be used here — it would
	// deadlock and Run would never start. Populate directly instead; the
	// transport may still be nil at this point (initRNS is async), in which
	// case this is a no-op and the ticker below picks the list up once initRNS
	// completes.
	interfacesDisplay.SetInterfaces(interfaceInfos())
	ifaceTicker := time.NewTicker(1 * time.Second)
	tuiApp.GoSafe(func() {
		for range ifaceTicker.C {
			refreshInterfaces()
		}
	})

	// Wire interfaces keyboard shortcuts
	interfacesDisplay.OnReleaseFocus = func() { main.FocusMenu() }
	interfacesDisplay.OnAddInterface = func() {
		tuiApp.Dialogs.ShowInputDialog("Add Interface",
			"Interface name:", "",
			func(name string) {
				name = strings.TrimSpace(name)
				if name == "" {
					return
				}
				tuiApp.Dialogs.ShowInputDialog("Interface Type",
					"Type (e.g. AutoInterface, TCPClientInterface):", "AutoInterface",
					func(ifType string) {
						ifType = strings.TrimSpace(ifType)
						if ifType == "" {
							ifType = "AutoInterface"
						}
						formData := tui.NewInterfaceFormData(ifType)
						formData.Fields["name"].Value = name
						cfg := formData.BuildConfig()
						if err := a.AddInterfaceConfig(name, cfg); err != nil {
							interfacesDisplay.ShowInterfaceError(err.Error())
							return
						}
						refreshInterfaces()
						interfacesDisplay.ShowRestartRequired()
					},
					func() {},
				)
			},
			func() {},
		)
	}
	interfacesDisplay.OnConfigEditor = func() {
		// Python's open_config_editor (Interfaces.py:3160-3185) runs $EDITOR
		// on the RNS config (self.app.rns.configpath), embedded in the body as a
		// LineBox titled "Editing RNS Config". gonomadnet embeds the editor via
		// the EmbeddedTerminal widget (the tview analogue of urwid.Terminal) so
		// the menu bar + footer stay visible while editing; on editor exit the
		// interface list is restored.
		rnsPath := a.RNSConfigPath()
		if rnsPath == "" {
			tuiApp.Dialogs.ShowDialog("Config Editor",
				tview.NewTextView().
					SetDynamicColors(true).
					SetText("[gray]The Reticulum config is not available yet — RNS is still initializing. Try again in a moment.[-]"),
				60, 6, nil)
			return
		}
		editor := tui.ResolveEditorCmd(a.Config.TextUI.Editor)
		interfacesDisplay.ShowEditor(editor, rnsPath)
	}
	interfacesDisplay.OnEditInterface = func() {
		idx := interfacesDisplay.SelectedIndex()
		items := interfacesDisplay.Items()
		if idx < 0 || idx >= len(items) {
			return
		}
		iface := items[idx]
		tuiApp.Dialogs.ShowInputDialog("Edit Interface: "+iface.Name,
			"New interface name:", iface.Name,
			func(newName string) {
				newName = strings.TrimSpace(newName)
				if newName == "" {
					newName = iface.Name
				}
				existingCfg, err := a.GetInterfaceConfigMap(iface.Name)
				if err != nil {
					interfacesDisplay.ShowInterfaceError(err.Error())
					return
				}
				formData := tui.NewInterfaceFormData(iface.Type)
				formData.PopulateFromConfig(iface.Name, existingCfg)
				formData.Fields["name"].Value = newName
				cfg := formData.BuildConfig()
				if err := a.EditInterfaceConfig(iface.Name, newName, cfg); err != nil {
					interfacesDisplay.ShowInterfaceError(err.Error())
					return
				}
				refreshInterfaces()
				interfacesDisplay.ShowRestartRequired()
			},
			func() {},
		)
	}
	interfacesDisplay.OnRemoveInterface = func() {
		idx := interfacesDisplay.SelectedIndex()
		items := interfacesDisplay.Items()
		if idx < 0 || idx >= len(items) {
			return
		}
		iface := items[idx]
		tuiApp.Dialogs.ShowConfirmDialog("Remove interface "+iface.Name+"?",
			func() {
				if err := a.RemoveInterfaceConfig(iface.Name); err != nil {
					interfacesDisplay.ShowInterfaceError(err.Error())
					return
				}
				refreshInterfaces()
				interfacesDisplay.ShowRestartRequired()
			},
			func() {},
		)
	}
	interfacesDisplay.OnShowInterface = func(idx int) {
		// G1: Enter opens Python's FULL-PAGE ShowInterface view (title header,
		// info rows, RX/TX charts, parameter blocks, footer Back/Toggle/Edit
		// row, Interfaces.py:2198-2620) — not a tiny modal.
		items := interfacesDisplay.Items()
		if idx < 0 || idx >= len(items) {
			return
		}
		interfacesDisplay.ShowInterfaceDetail(items[idx])
	}
	interfacesDisplay.OnToggleInterface = func(info tui.InterfaceInfo) {
		// Python on_toggle_enabled (Interfaces.py:2516-2600): flip
		// interface_enabled in the RNS config and write it. The Go config file
		// is the source of truth for the interface list, so flip the value in
		// place. The confirm dialog + restart-required notice are a follow-up
		// (deviation noted here).
		if err := a.ToggleInterfaceEnabled(info.Name); err != nil {
			tuiApp.QueueUpdateDraw(func() {
				interfacesDisplay.ShowInterfaceError(err.Error())
			})
			return
		}
		refreshInterfaces()
	}

	// Browser display (hidden by default, shown when navigating from network)
	browserDisplay := tui.NewBrowserDisplay(tuiApp)
	main.SetDisplay("browser", browserDisplay.Widget())
	main.SetShortcut("browser", "[C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-s] Save  [C-y] Copy URL  [C-g] Fullscreen")

	// On-disk page cache (Python app.cachepath = configdir/storage/cache,
	// NomadNetworkApp.py:113). Browser.load_page checks get_cached when no
	// request_data is attached (Browser.py:1237-1244); response_received caches
	// a freshly fetched page for #!c=<seconds> (Browser.py:1524-1546). Reload
	// uncaches the current URL first (Browser.py:1100) to force a re-fetch.
	pageCache := tui.NewBrowserCache(a.CachePath)

	// The per-target callbacks (OnDisconnect, OnReleaseFocus) are wired inline
	// below; everything else (OnBack/Forward/Reload/CopyURL/URLDialog/SaveNode/
	// JumpAnchor/OpenLXMF/OpenRRC/PartialUpdate/BrowserError/ToggleFullscreen/
	// FetchPartial/RetrieveURL) is shared by BOTH the standalone browser page
	// and the Network right pane via wireBrowser, so the two no longer diverge
	// (the Network pane previously had most of these nil — e.g. Ctrl-d / Back
	// was a no-op, stranding the user after a malformed-link error).
	browserDisplay.OnDisconnect = func() {
		// Python Browser.disconnect (Browser.py:862-881) clears history, resets
		// the pointer, and drops the current-destination hint — the BrowserDisplay
		// owns that state; the link itself is one-shot per fetch in the Go port
		// (no persistent self.link to tear down).
		browserDisplay.Disconnect()
	}
	// identifyOnConnect mirrors Python link_established (Browser.py:1454-1459):
	// when the directory entry requests it, identify to the remote node over the
	// freshly established link. Shared by wireBrowser's OnFetchPartial and
	// OnRetrieveURL, so defined before wireBrowser.
	identifyOnConnect := func(destHash []byte) func(*rns.Link) {
		if a.Dir == nil || !a.Dir.ShouldIdentifyOnConnect(destHash) || a.Identity == nil {
			return nil
		}
		identity := a.Identity
		return func(link *rns.Link) { _ = link.Identify(identity) }
	}

	// activeRetainedLink returns the browser's retained *rns.Link if it is still
	// ACTIVE (LinkActive), nil otherwise. The retained link is stored opaquely
	// on the BrowserDisplay (the tui package does not import rns), so the wiring
	// layer owns the activeness check. Used both to gate the page cache (a cached
	// page is served only when a link is already active — see OnRetrieveURL) and
	// to pass the reusable link into FetchPageReuseLink.
	activeRetainedLink := func(bd *tui.BrowserDisplay) *rns.Link {
		l, ok := bd.RetainedLink().(*rns.Link)
		if !ok || l == nil {
			return nil
		}
		if l.GetStatus() != rns.LinkActive {
			return nil
		}
		return l
	}

	// OnRetrieveURL runs the real fetch backend (Browser.retrieve_url →
	// load_page → __load): parse the RNS address (resolving relative ":<path>"
	// URLs against the current destination), check the on-disk cache when no
	// request_data is attached (Python load_page, Browser.py:1237-1244), and
	// otherwise establish a link, request the page on a goroutine (UI stays
	// responsive — Python runs __load on a thread), and marshal the rendered
	// Micron page (or an error) back to the browser via QueueUpdateDraw. On a
	// successful fetch the page is cached for its #!c=<seconds> header
	// (response_received, Browser.py:1524-1546) unless that header is 0.
	//
	// Race fix: beginRequest cancels any in-flight fetch and bumps a request
	// sequence BEFORE this fetch spawns. The goroutine captures ctx+seq by value
	// and, at the top of its QueueUpdateDraw callback, drops the render when seq
	// no longer equals the current sequence or ctx was cancelled — so a slow
	// fetch from a superseded Connect can never overwrite the page the user is
	// now viewing. This is the single convergence point for both displayURL →
	// OnRetrieveURL and HandleLink → OnRetrieveURL, so link-click racing is
	// covered too.
	wireBrowser := func(bd *tui.BrowserDisplay) {
		if bd == nil {
			return
		}
		// Register the retained-link teardown so BrowserDisplay can tear down
		// the per-destination *rns.Link (Python self_link) when the destination
		// changes or the browser disconnects. BrowserDisplay stores the link
		// opaquely (any) to avoid importing go-reticulum into the tui package;
		// this closure is the one side that knows the concrete type.
		bd.SetRetainedLinkTeardown(func(link any) {
			if l, ok := link.(*rns.Link); ok && l != nil {
				l.Teardown()
			}
		})
		// Navigation + link-handling callbacks are shared by BOTH the standalone
		// browser page and the Network right pane (Python's BrowserFrame and
		// NetworkDisplay both drive the same Browser instance, Browser.py:21-40 /
		// Network.py:1609-1610). Previously these were wired only on the standalone
		// browserDisplay, so the Network pane's browser had OnBack/OnForward/etc.
		// nil — Ctrl-d (Back) was a no-op there, stranding the user after a
		// malformed-link error with no way back.
		bd.OnBack = func() { bd.GoBack() }
		bd.OnForward = func() { bd.GoForward() }
		// Go-only enhancement (no Python SOT counterpart — Python nomadnet
		// cannot block nodes, so never remove this guard as a parity
		// divergence): before ANY navigation (URL dialogs, node Connect, link
		// clicks) the browser asks the app whether the destination is on the
		// ignored list. Blocked destinations raise the "Blocked node" warning
		// modal (default focus = Cancel, so a bare Enter cancels) loaded from
		// tui.ShowBlockedNodeConfirmDialog, and only an explicit Connect
		// proceeds.
		bd.OnBlockedConnectCheck = func(nodeHashHex string) (string, bool) {
			hash, ok := app.SourceHashFromHex(nodeHashHex)
			if !ok || !a.IsIgnored(hash) {
				return "", false
			}
			return a.Dir.SimplestDisplayStr(hash), true
		}
		bd.OnReload = func() {
			// Python reload() uncaches the current URL then re-loads, so a reload
			// always re-fetches instead of returning a stale cache hit.
			pageCache.UncachePage(bd.CurrentURL())
			bd.Reload()
		}
		bd.OnCopyURL = func() {
			// Python BrowserFrame.keypress ctrl y (Browser.py:38-40) only calls
			// copy_url when config.textui.clipboard_copy is enabled, and copy_url
			// (Browser.py:1103-1133) copies the current URL via the OSC 52 escape
			// sequence (osc52_copy) — no platform clipboard tool. With no URL or
			// the feature disabled, C-y is a no-op.
			if a.Config == nil || !a.Config.TextUI.ClipboardCopy {
				return
			}
			url := bd.CurrentURL()
			if url == "" {
				return
			}
			_ = tui.OSC52Copy(url)
		}
		bd.OnURLDialog = func() {
			// Python Browser.url_dialog (Browser.py:1135-1182): title "Enter URL",
			// caption "URL : ", pre-fill current_url, Cancel/Go buttons, overlaid
			// on the browser pane at 65% width. On "Go" apply the "|"`→"`"
			// normalization (a user can type "hash:path|x=1" instead of using a
			// backtick) then retrieve_url.
			showURLDialog(bd.CurrentURL(), func(text string) {
				// Route the submit through OnLoadURL when the wiring layer
				// provides one (the Network right pane must mount its display
				// before a load renders into the visible frame), else load
				// directly (the standalone browser page is always mounted).
				if bd.OnLoadURL != nil {
					bd.OnLoadURL(browser.NormalizeEnteredURL(text))
					return
				}
				bd.LoadURL(browser.NormalizeEnteredURL(text))
			})
		}
		bd.OnSaveNode = func() {
			// Python Browser.save_node_dialog (Browser.py:1184-1234): only when a
			// destination is connected; recall the announced app_data display name;
			// confirm "Save connected node "<name>" <prettyhex> to Known Nodes?";
			// on Save, remember a DirectoryEntry(hosts_node=True) and fire the
			// directory-change callback (refresh the network known-nodes pane).
			dest := bd.CurrentDest()
			if len(dest) == 0 {
				return
			}
			dispName := ""
			if id := a.Transport.Recall(dest); id != nil && len(id.AppData) > 0 {
				dispName = string(id.AppData)
			}
			namePart := ""
			if dispName != "" {
				namePart = " \"" + dispName + "\""
			}
			msg := "Save connected node" + namePart + " " + rns.PrettyHexRep(dest) + " to Known Nodes?\n"
			// Python Browser.save_node_dialog (Browser.py:1184-1234): overlaid on
			// the browser pane at 50% width, title "Save Node", Cancel/Save.
			showBrowserConfirm("Save Node", msg, "Cancel", "Save", 50, func() {
				a.SaveConnectedNode(dest, dispName)
				refreshNodes()
			})
		}
		bd.OnJumpAnchor = func(name string) {
			// Python Browser._jump_to_anchor (Browser.py:324-357): scroll the page
			// content to the named anchor (or the next heading below the cursor for
			// a bare "#"). Pure local UI — no network involved.
			bd.JumpToAnchor(name)
		}
		bd.OnOpenLXMF = func(hashHex string) {
			// Python Browser.handle_lxmf_link (Browser.py:383-423): validate the LXMF
			// destination hash, recall the announced display name, and for a new
			// source create a directory entry + on-disk conversation directory; then
			// display the conversation and switch to the Conversations page. The
			// browser-side HandleLXMFLink has already validated, but the app method
			// re-validates defensively.
			if _, err := a.OpenLXMFLink(hashHex); err != nil {
				tuiApp.Dialogs.ShowConfirmDialog(
					"Could not open LXMF link: "+err.Error(),
					func() {}, func() {})
				return
			}
			refreshConvs()
			conversationsDisplay.DisplayConversation(hashHex)
			main.SelectPage("conversations")
		}
		bd.OnOpenRRC = func(hubHex, room string) {
			hubHex = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(hubHex)), "0x")
			hashBytes, err := hex.DecodeString(hubHex)
			if err != nil || len(hashBytes) != 16 {
				tuiApp.Dialogs.ShowConfirmDialog(
					"Could not open RRC link: invalid hub address",
					func() {}, func() {})
				return
			}
			if a.RRC != nil {
				hub := a.RRC.FindHub(hashBytes, "rrc.hub")
				if hub == nil {
					hub = a.RRC.AddHub(hashBytes, "rrc.hub", "")
				}
				if room != "" {
					hub.AddRoom(room)
					if hub.Status == rrc.StatusConnected {
						hub.JoinRoom(room, false)
					}
				}
				tuiApp.QueueUpdateDraw(func() {
					channelsDisplay.SetHubs(a.HubViews())
				})
			}
			main.SelectPage("channels")
		}
		bd.OnPartialUpdate = func(ids []string) {
			// Python Browser.handle_partial_updates (Browser.py:823-834): a
			// "p:<id>:<id>" link forces an immediate re-fetch of the named partials.
			// The BrowserDisplay owns the partial-refresh loop and substitution; this
			// just selects the matching partials and re-fetches them off-thread.
			bd.RefreshPartials(ids)
		}
		bd.OnBrowserError = func(msg string) {
			// Python surfaces link-dispatch failures (handle_link "No known handler",
			// handle_lxmf_link/handle_rrc_link errors, Browser.py:303/321/422/465) in
			// the browser footer. The Go BrowserDisplay has no footer slot, so the app
			// surfaces them as a centered dismissible dialog. Page-load fetch errors
			// are NOT routed here — they render in the content area (the faithful
			// single-place surface, since Python shows those in the footer too but
			// the Go content is the equivalent prominent surface).
			tuiApp.Dialogs.ShowDialog("Browser",
				tview.NewTextView().
					SetDynamicColors(true).
					SetTextAlign(tview.AlignCenter).
					SetText("[red]"+tview.Escape(msg)+"[-]"),
				50, 5, nil)
		}
		bd.OnToggleFullscreen = func() {
			// Python BrowserFrame C-g (Browser.py:36-37) calls
			// network_display.toggle_fullscreen(), which hides the network page's
			// left pane so the in-page browser fills the width. The Go port mounts
			// the browser as its OWN full-screen page (navigating switches to the
			// "browser" page), so it is already full-width with no left pane to
			// hide — the toggle has no chrome to collapse here. A no-op preserves
			// the keybinding without a visible change; the in-network-pane browser
			// placement (which would make this toggle meaningful) is a larger
			// architecture change deferred beyond this phase.
		}
		bd.OnFetchPartial = func(p browser.Partial) ([]byte, error) {
			// This closure runs on the runPartialRefresh goroutine (browser.go), NOT
			// the tview event loop, so it must NOT read bd.reqCtx (it would race the
			// event loop). Pass context.Background(); partials already have their
			// own partialCancel staleness gate (browser.go fetchAndSubstitute).
			return browser.FetchPartial(context.Background(), a.Transport, p, bd.CurrentDest(),
				time.Duration(browser.DefaultTimeout)*time.Second, nil,
				identifyOnConnect(bd.CurrentDest()))
		}
		bd.OnRetrieveURL = func(url string, requestData map[string]string) {
			diagFile("/tmp/fetch-diag.log", fmt.Sprintf("[%s] OnRetrieveURL bd=%p url=%q rdNil=%v", time.Now().Format("15:04:05.000"), bd, url, requestData == nil))
			ctx, seq := bd.BeginRequest()
			// requestData carries the live form-field values collected by
			// HandleLink's collectFields (Python recurse_down), or nil for a
			// plain link / typed URL. ParseURL merges any backtick var_*
			// field-suffix embedded in the URL into requestData and returns the
			// combined map; the cache is only consulted when rd is nil (Python
			// load_page caches only when no request_data is attached), so a form
			// submit always re-fetches.
			dest, path, rd, err := browser.ParseURL(url, bd.CurrentDest(), requestData)
			if err != nil {
				tuiApp.QueueUpdateDraw(func() {
					if seq != bd.CurrentRequestSeq() || ctx.Err() != nil {
						return
					}
					// Malformed URL (e.g. an https:// link): Python's retrieve_url
					// raises ValueError BEFORE touching status/destination/history,
					// and handle_link / url_dialog catch it into the FOOTER
					// ("Could not open link: ...") leaving the current page intact
					// (Browser.py:300-304, 1142-1150). Mirroring that keeps the user
					// on the page they were viewing so Back (Ctrl-d) works as normal
					// — the previous SetContent overwrote the page with the error
					// and, since the failed link never pushed history, stranded the
					// user with no way back (Ctrl-d did nothing).
					bd.NotifyLinkError(fmt.Sprintf("Could not open link: %v", err))
				})
				return
			}
			bd.SetCurrentDest(dest)
			canonURL := fmt.Sprintf("%x:%v", dest, path)
			// Serve from the page cache whenever no request_data is attached,
			// matching Python's load_page (Browser.py:1237-1244), which checks
			// get_cached based ONLY on request_data — NOT on whether an RNS link
			// is active. The former Go-only gate (cache only when an active
			// retained link existed) made Back/Forward re-fetch over the network
			// whenever the retained link had gone stale, so Ctrl-d back to a cached
			// page (e.g. RetiBooks' index after visiting an Author page) was slow
			// where Python returns instantly from the cache. A form submit (rd !=
			// nil) still always re-fetches. The retained link is reused for the
			// next fetch via activeRetainedLink below; a cache hit simply does not
			// establish one, exactly as in Python (Python's connect cache-hits and
			// leaves link establishment to the next click).
			if tui.ServeFromCache(rd) {
				if cached := pageCache.GetCached(canonURL); cached != nil {
					tuiApp.QueueUpdateDraw(func() {
						if seq != bd.CurrentRequestSeq() || ctx.Err() != nil {
							return
						}
						bd.SetTransferStats(int64(len(cached)), 0, 0, true)
						bd.RenderPage(string(cached))
					})
					return
				}
			}
			// Local loopback: if the destination is THIS node's own
			// destination, serve the page directly from the local pages
			// directory instead of going through RNS transport. Mirrors
			// Python Browser.load_page's loopback branch (Browser.py:1296
			// -1372): a single nomadnet instance hosts the node AND the
			// browser in one process, so browsing the local node reads its
			// served pages from disk — it never establishes an RNS link to
			// itself. RNS has no self-loopback (Transport.has_path(own) is
			// false even in Python), so without this Browse on the local
			// node fails at the path-resolution gate with "No path to
			// destination known". Local file downloads (/file/...) route
			// through ServeLocalFile the same way; remote /file/ downloads
			// are a separate, not-yet-wired path.
			if a.Node != nil {
				if nd := a.Node.Destination(); nd != nil && bytes.Equal(dest, nd.Hash) {
					go func() {
						start := time.Now()
						var data []byte
						if strings.HasPrefix(path, "/file/") {
							savedName, _, _ := browser.ServeLocalFile(a.FilesPath, path, a.DownloadsPath)
							elapsed := time.Since(start).Seconds()
							tuiApp.QueueUpdateDraw(func() {
								if seq != bd.CurrentRequestSeq() || ctx.Err() != nil {
									return
								}
								if savedName != "" {
									bd.SetTransferStats(0, 0, elapsed, false)
									bd.SetContent(fmt.Sprintf("Saved file: %s", savedName))
								} else {
									bd.SetContent("[red]The requested local download file does not exist[-]")
								}
							})
							return
						}
						data = browser.ServeLocalPage(a.PagesPath, path)
						elapsed := time.Since(start).Seconds()
						tuiApp.QueueUpdateDraw(func() {
							if seq != bd.CurrentRequestSeq() || ctx.Err() != nil {
								return
							}
							bd.SetTransferStats(int64(len(data)), int64(len(data)), elapsed, false)
							bd.RenderPage(string(data))
						})
					}()
					return
				}
			}
			go func() {
				start := time.Now()
				// Reuse the per-destination retained link (Python self_link,
				// Browser.py:1375-1451) so a form-submit re-fetch rides the
				// already-ACTIVE link instead of re-establishing — the reliability
				// gap that made the search-results fetch flaky over a remote
				// multi-hop path. activeRetainedLink returns nil when the retained
				// link is absent or has gone stale (remote closed it), in which
				// case FetchPageReuseLink establishes a fresh one.
				existing := activeRetainedLink(bd)
				data, link, ferr := browser.FetchPageReuseLink(ctx, a.Transport, dest, path, rd,
					time.Duration(browser.DefaultTimeout)*time.Second, nil,
					identifyOnConnect(dest), existing)
				elapsed := time.Since(start).Seconds()
				tuiApp.QueueUpdateDraw(func() {
					if seq != bd.CurrentRequestSeq() || ctx.Err() != nil {
						// A superseding fetch superseded this one. Tear down this
						// result's link ONLY if it is not the currently retained
						// link: a superseded fetch that REUSED the retained link
						// shares it with the newer fetch (which may still be using
						// it), so tearing it down would break the newer fetch. A
						// fresh link (the common case — e.g. the double-fetch from
						// the URL dialog) is an orphan and is torn down. The
						// retained-link comparison is by pointer; both callbacks
						// run serialized on the event loop, so there is no race
						// with the newer fetch's SetRetainedLink.
						if link != nil && bd.RetainedLink() != link {
							link.Teardown()
						}
						return
					}
					if ferr != nil {
						// On error drop the retained link so a retry re-establishes
						// (a stale/closed link would fail the GetStatus reuse check
						// anyway, but clearing is explicit and avoids leaking a link
						// whose request failed).
						bd.SetRetainedLink(nil)
						if link != nil {
							link.Teardown()
						}
						bd.SetContent(fmt.Sprintf("[red]%v[-]", browser.StatusText(browser.ErrToStatus(ferr))))
						return
					}
					if rd == nil {
						if ct := tui.CacheTimeFromMarkup(string(data)); ct != 0 {
							pageCache.CachePage(canonURL, data,
								float64(time.Now().UnixNano())/1e9+float64(ct))
						}
					}
					// Retain the link for the next fetch to this destination
					// (Python self_link). A fresh link is stored here on first
					// fetch; a reused link is re-stored (no-op). The link is torn
					// down via SetCurrentDest/Disconnect when the destination
					// changes or the browser disconnects.
					bd.SetRetainedLink(link)
					bd.SetTransferStats(int64(len(data)), int64(len(data)), elapsed, false)
					bd.RenderPage(string(data))
				})
			}()
		}
	}

	wireBrowser(browserDisplay)
	// Release-focus: Left at the start of the focused line in the page body
	// returns focus to the owning view (Python delegate.micron_released_focus →
	// focus_lists / the menu, MicronParser.py:972-974). The standalone browser
	// page hands focus back to the menu bar; the Network right pane hands it to
	// the left node/announce list.
	browserDisplay.OnReleaseFocus = func() { main.FocusMenu() }
	if ndBd := networkDisplay.BrowserDisplay(); ndBd != nil {
		wireBrowser(ndBd)
		ndBd.OnReleaseFocus = func() { networkDisplay.FocusLists() }
		// URL-dialog submits from the Network right-pane browser must mount the
		// pane before loading: navigateTo → BrowserPane.LoadURL swaps the
		// BrowserDisplay's widget into the "Remote Node" frame AND loads the
		// URL — bd.LoadURL alone renders into the still-unmounted display and
		// the visible pane keeps its disconnected placeholder.
		ndBd.OnLoadURL = func(url string) { navigateTo(url) }
		// C-w while the Network right-pane browser holds focus routes through
		// the BrowserDisplay's own key handler (browser.go handleInput KeyCtrlW
		// → OnDisconnect). Python's BrowserFrame.keypress (Browser.py:21-22) calls
		// self.delegate.disconnect() — the same Browser.disconnect the Network
		// page shortcut calls — so wire it to the pane's Disconnect (which cancels
		// the in-flight fetch and resets the Remote Node pane to the disconnected
		// view). Without this, C-w while the browser is focused is a no-op and the
		// pane stays stuck on "Retrieving" after a connect to a non-responding
		// node (the connect-timeout regression).
		ndBd.OnDisconnect = func() {
			if bp := networkDisplay.BrowserPane(); bp != nil {
				bp.Disconnect()
			}
		}
	}

	// C-w on the Network page with focus on the left list routes through
	// NetworkDisplay.handleInput (network.go KeyCtrlW → OnDisconnect). Python's
	// NetworkDisplay.keypress (Network.py:1609-1610) calls
	// self.parent.browser.disconnect() directly; the Go port must wire the
	// callback or C-w is a no-op (the in-flight fetch is never cancelled, the
	// "Retrieving" loading body is never swapped out — the pane hangs).
	networkDisplay.OnDisconnect = func() {
		if bp := networkDisplay.BrowserPane(); bp != nil {
			bp.Disconnect()
		}
	}

	// Wire network connect to the Network display's own browser pane only.
	// Python's Network page routes every navigation (Ctrl-u url_dialog,
	// node/announce Enter, NodeInfo Browse) to its own browser
	// (Network.py:1612/129/695/893/1421 → self.parent.browser.retrieve_url) —
	// there is no standalone "browser" sub-display in Python (SubDisplays lists
	// network/conversations/channels/directory/config/interface/map/log/guide
	// only). Loading the standalone browserDisplay here too spawned a SECOND
	// concurrent link establishment to the same destination (two fresh
	// existing=false handshakes racing), which intermittently made both time
	// out — the last flakiness source. The standalone browserDisplay keeps
	// its own Ctrl-u (bd.OnURLDialog → bd.LoadURL) for when it is shown.
	navigateTo = func(url string) {
		if ndBp := networkDisplay.BrowserPane(); ndBp != nil {
			ndBp.LoadURL(url)
		}
	}
	networkDisplay.SetNavigateCallback(navigateTo)

	// Enter on a Saved Nodes list row opens the "Connect to node?" dialog
	// (Python KnownNodes NodeEntry "click" signal → connect_node,
	// Network.py:881-919): a Yes/No/Info button row. Yes connects by loading
	// the node's page URL in the Network browser pane (Python confirmed →
	// browser.retrieve_url(RNS.hexrep(source_hash, delimit=False)) +
	// close_list_dialogs); No dismisses; Info opens the editable KnownNodeInfo
	// form (Python show_info → KnownNodeInfo). The display string mirrors
	// Python's simplest_display_str(source_hash).
	networkDisplay.OnConnectNode = func(node tui.NodeEntry) {
		displayStr := node.DisplayName
		if hash, ok := app.SourceHashFromHex(node.SourceHash); ok {
			displayStr = a.Dir.SimplestDisplayStr(hash)
		}
		if displayStr == "" {
			displayStr = "<" + node.SourceHash + ">"
		}
		yesBtn := tui.NewUrwidButton("Yes").SetSelectedFunc(func() {
			networkDisplay.CloseListSlotDialog()
			navigateTo(node.SourceHash)
		})
		noBtn := tui.NewUrwidButton("No").SetSelectedFunc(func() {
			networkDisplay.CloseListSlotDialog()
		})
		infoBtn := tui.NewUrwidButton("Info").SetSelectedFunc(func() {
			networkDisplay.CloseListSlotDialog()
			networkDisplay.ShowKnownNodeInfo(node.SourceHash)
		})
		buttons := tui.CreateUrwidButtonRow(yesBtn, noBtn, infoBtn)
		msg := tui.NewUrwidCenterText("Connect to node\n" + displayStr + "\n")
		layout := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(msg, 3, 0, false).
			AddItem(buttons, 1, 0, true)
		// Python connect_node (Network.py:881-919): urwid.Overlay over the
		// left_pile list slot, width=RELATIVE_100, left=2/right=2, height=PACK,
		// title "?". Render it in the list slot (show-through), not a centered
		// modal. msg 3 + button row 1 + border 2 = 6 PACK height.
		dialog := tui.NewDialogLineBox("?", layout, nil)
		networkDisplay.ShowListSlotDialog(dialog, 6)
	}

	// The Quit menu item triggers graceful shutdown (selectMenu special-cases
	// key "quit" → onQuit, mirroring Python's handler.quit raise ExitMainLoop →
	// atexit exit_handler). It shows no page, so there is no "quit" body display
	// to register. The startup splash is shown separately via ShowIntro above.

	// On first run, the original opens the Guide on the "First Run" topic
	// (Main.py:27 `if app.firstrun: active_display = guide_display` +
	// Guide.py:221-224 first_run_entry.display_topic). NewMainDisplay defaults
	// to the Conversations page, so override that here.
	if a.FirstRun {
		guideDisplay.ShowFirstRun()
		main.SelectPage("guide")
	}

	return func() {
		logDisplay.StopTailing()
		ifaceTicker.Stop()
		// Stop the partial-refresh goroutines BEFORE a.Shutdown() closes the
		// RNS transport: each tick dereferences a.Transport via the
		// OnFetchPartial closure, and a closed TransportSystem panics. The
		// standalone browser page and the Network pane's in-page browser each
		// own their own partial loops.
		browserDisplay.StopPartials()
		if ndBd := networkDisplay.BrowserDisplay(); ndBd != nil {
			ndBd.StopPartials()
		}
	}
}

// toConversationMessages maps the app's pure-data message view
// (conversation.MessageDisplayData, parsed from each message's LXMF envelope
// on disk) onto the TUI's ConversationMessage, which drives the
// LXMessageWidget header (LXMessageHeader) and indented body rendering. The
// raw LXMF state int and method are passed through unchanged so the header
// logic compares against the raw LXMF constants, exactly like the Python
// original (Conversations.py:2609-2626).
func toConversationMessages(in []conversation.MessageDisplayData) []tui.ConversationMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]tui.ConversationMessage, len(in))
	for i, m := range in {
		var ts time.Time
		if m.Timestamp > 0 {
			// Timestamp is Unix seconds (float, Python get_timestamp).
			sec := int64(m.Timestamp)
			nsec := int64((m.Timestamp - float64(sec)) * 1e9)
			ts = time.Unix(sec, nsec)
		}
		out[i] = tui.ConversationMessage{
			Content:              m.Content,
			Title:                m.Title,
			Timestamp:            ts,
			State:                m.State,
			Method:               m.Method,
			SourceHash:           m.SourceHash,
			Hash:                 m.Hash,
			TransportEncrypted:   m.TransportEncrypted,
			SignatureValidated:   m.SignatureValidated,
			SignatureDescription: m.SignatureDescription,
			HasAttach:            m.HasAttachments,
			AttachCount:          len(m.AttachmentNames),
			AttachmentTypes:      m.AttachmentTypes,
			AttachmentNames:      m.AttachmentNames,
		}
	}
	return out
}

// buildNodeInfoData constructs the Local Node Info panel data from the app's
// hosted node + peer settings + LXMF router, mirroring Python's NodeInfo pile
// (Network.py:1357-1554). When no node is hosted (EnableNode false / a.Node nil)
// the returned HasNode is false and the panel renders the "not hosting a node"
// branch. Otherwise every stat line is a live provider that re-reads the
// app/node/peer-settings each refresh, exactly like the Python UpdatingText
// widgets (NodeLastAnnounce/NodeStorageStats/NodeActiveConnections/
// NodeTotalConnections/NodeTotalPages/NodeTotalFiles, Network.py:1059-1256):
//
//   - Last Announce  : pretty_date(node_last_announce) or "Never"
//   - LXMF Storage   : "<pct>%, <used> of <limit>" (when not disable_propagation)
//     else "None"; "<used>" only when no limit
//   - Connected Now  : len(node.destination.links) — Go tracks a.Node.ActiveLinks
//   - Total Connects : peer_settings["node_connects"]
//   - Served Pages   : peer_settings["served_page_requests"]
//   - Served Files   : peer_settings["served_file_requests"]
//
// The Browse button browses the node's own page (connect_query), Announce sends
// a node announce + shows the "Announce Sent" dialog (announce_query), and Rst
// Stats zeros the persisted counters (stats_query).
func buildNodeInfoData(a *app.App, navigateTo func(string), showAnnounceSent func()) tui.NodeInfoData {
	data := tui.NodeInfoData{HasNode: a.Node != nil}
	if a.Node == nil {
		return data
	}

	// Addr = RNS.hexrep(node.destination.hash, delimit=False) = lowercase hex.
	if dest := a.Node.Destination(); dest != nil {
		data.Addr = fmt.Sprintf("%x", dest.Hash)
	}
	// Name: the hosted node's announce name (Python Network.py:1381-1386
	// reads app.node.name, the same value Node.announce sends as app_data —
	// Node.py:216). nodeName mirrors Python Node.py:28-36: the configured
	// node_name, falling back to the peer display name with "'s Node"
	// appended, so the panel always shows exactly what peers receive.
	data.Name = a.ResolveNodeName()
	data.DisablePropagation = a.DisablePropagation
	if !a.DisablePropagation && a.Identity != nil {
		// Python: RNS.prettyhexrep(RNS.Destination.hash_from_name_and_identity(
		// "lxmf.propagation", node.destination.identity)).
		propHash := rns.CalculateHash(a.Identity, "lxmf", "propagation")
		data.LXMFPropAddr = rns.PrettyHexRep(propHash)
	}

	// Last Announce (NodeAnnounceTime, Network.py:1060-1097): "Never" until the
	// node announces once, then pretty_date(unix_seconds). NodeLastAnnounce is
	// msgpack-typed `any` (int64 from onNodeAnnounced, possibly uint64/CBOR on
	// reload) — coerce defensively.
	data.LastAnnounce = func() string {
		last, ok := a.NodeLastAnnounceSetting()
		if !ok || last == nil {
			return "Never"
		}
		ts, ok := peerAnnounceUnix(last)
		if !ok || ts <= 0 {
			return "Never"
		}
		return tui.PrettyDate(time.Unix(ts, 0).UTC())
	}

	// LXMF Storage (NodeStorageStats, Network.py:1130-1147): "<pct>%, <used> of
	// <limit>"; "None" when propagation disabled or no router; "<used>" only when
	// the limit is unset (Python message_storage_limit is None).
	data.StorageStats = func() string {
		if a.DisablePropagation || a.Router == nil {
			return "None"
		}
		used := a.Router.MessageStorageSize()
		limit := a.Router.MessageStorageLimit()
		if limit > 0 {
			pct := strconv.FormatFloat(used/limit*100, 'f', 1, 64)
			return pct + "%, " + tui.Prettysize(used) + " of " + tui.Prettysize(limit)
		}
		return tui.Prettysize(used)
	}

	// Connected Now (NodeActiveConnections, Network.py:1116-1127):
	// len(node.destination.links) — Go tracks a.Node.ActiveLinks.
	data.ActiveLinks = func() string { return strconv.Itoa(a.Node.ActiveLinkCount()) }

	// Total Connects / Served Pages / Served Files (NodeTotalConnections/Pages/
	// Files, Network.py:1163-1256): the persisted peer-settings counters.
	data.TotalConnects = func() string {
		connects, _, _, ok := a.NodeStats()
		if !ok {
			return "None"
		}
		return strconv.Itoa(connects)
	}
	data.TotalPages = func() string {
		_, pages, _, ok := a.NodeStats()
		if !ok {
			return "None"
		}
		return strconv.Itoa(pages)
	}
	data.TotalFiles = func() string {
		_, _, files, ok := a.NodeStats()
		if !ok {
			return "None"
		}
		return strconv.Itoa(files)
	}

	// Browse → load the node's own page in the browser (connect_query,
	// Network.py:1402-1404). The RNS page fetch is wired through the
	// same navigate callback the network list uses.
	data.OnBrowse = func() { navigateTo(data.Addr) }

	// Announce → send a node announce + show "Announce Sent" (announce_query,
	// Network.py:1406-1439). The OnAnnounced callback persists the new
	// node_last_announce, which the 1s refresh ticker surfaces.
	data.OnAnnounce = func() {
		if err := a.Node.Announce(); err != nil && a.Logger != nil {
			a.Logger.Error("node announce failed: %v", err)
		}
		if showAnnounceSent != nil {
			showAnnounceSent()
		}
	}

	// Rst Stats → zero the persisted counters (stats_query, Network.py:1383-1387).
	data.OnResetStats = func() { a.ResetNodeStats() }

	return data
}

// peerAnnounceUnix coerces a peer_settings["node_last_announce"] value (msgpack
// `any`) to a unix-seconds int64. It accepts the int64 written by onNodeAnnounced
// and the int64/uint64/msgpack-int forms a reloaded settings file may carry.
func peerAnnounceUnix(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case uint64:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}
