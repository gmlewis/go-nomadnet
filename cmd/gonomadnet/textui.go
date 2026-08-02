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
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-nomadnet/tui"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/rivo/tview"
)

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

	// Wire up real displays BEFORE setting root. The returned cleanup releases
	// background resources (log tail) on shutdown.
	logCleanup := wireDisplays(tuiApp, a)

	tuiApp.SetQuitCallback(func() {
		tuiApp.Main.StopUnreadBlink()
		if logCleanup != nil {
			logCleanup()
		}
		a.Shutdown()
		tuiApp.Stop()
	})

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

	// localPeerInfo formats the app's identity/LXMF hashes + display name +
	// last-announce time for the Local Peer Info panel (Python LocalPeer,
	// Network.py:1259). prettyhexrep = "<" + lowercase hex + ">".
	localPeerInfo := func() (lxmfAddr, identityHash, name string, lastAnnounce time.Time) {
		lxmfAddr = "<" + a.LXMFAddressHex() + ">"
		if a.Identity != nil {
			identityHash = "<" + fmt.Sprintf("%x", a.Identity.Hash) + ">"
		}
		return lxmfAddr, identityHash, a.GetDisplayName(), a.LastAnnounce
	}
	lxmfAddr, idhash, dname, lann := localPeerInfo()
	networkDisplay.UpdateLocalPeer(lxmfAddr, idhash, dname, lann)
	networkDisplay.SetLocalPeerHandlers(
		func(name string) {
			a.SetDisplayName(name)
			tuiApp.Dialogs.ShowDialog("Saved",
				tview.NewTextView().
					SetDynamicColors(true).
					SetTextAlign(tview.AlignCenter).
					SetText("\n\n\nSaved\n\n"),
				40, 9, nil)
		},
		func() {
			a.AnnounceNow()
			lxmfAddr, idhash, dname, lann = localPeerInfo()
			networkDisplay.UpdateLocalPeer(lxmfAddr, idhash, dname, lann)
			tuiApp.Dialogs.ShowDialog("Announce Sent",
				tview.NewTextView().
					SetDynamicColors(true).
					SetTextAlign(tview.AlignCenter).
					SetText("\n\n\nAnnounce Sent\n\n\n"),
				40, 10, nil)
		},
		func() {
			// Swap the left pile's PACK slot from Local Peer Info to the Local
			// Node Info panel (Python node_info_query, Network.py:1399-1401).
			// Node hosting is not yet wired in the Go port (no app.Node server;
			// Phase 5), so the panel renders the "not hosting a node" branch.
			networkDisplay.ShowNodeInfo(tui.NodeInfoData{HasNode: false})
		},
	)

	// refreshAnnounces re-fetches the announce stream from the app and updates
	// the network display's left pane.
	refreshAnnounces := func() {
		anns := a.GetAnnounces()
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
	// Wire up real announce data via callback
	a.SetUIChangeCallback(func() {
		refreshAnnounces()
		refreshNodes()
		// RNS init runs asynchronously in a goroutine, so the identity/LXMF
		// destination are nil when wireDisplays first runs. Re-filling the
		// Local Peer Info panel on each UI change picks them up once initRNS
		// completes (it fires UIChangeCallback at the end), and also refreshes
		// the "Announced : …" line as the announce age advances.
		lxmfAddr, idhash, dname, lann := localPeerInfo()
		networkDisplay.UpdateLocalPeer(lxmfAddr, idhash, dname, lann)
	})
	main.SetDisplay("network", networkDisplay.Widget())
	main.SetShortcut("network", "[C-l] Nodes/Announces  [C-x] Remove  [C-w] Disconnect  [C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-g] Fullscreen  [C-s / C-b] Save Node")

	// Wire Esc to go back from AnnounceInfo before quitting.
	main.SetEscCallback(func() bool {
		return networkDisplay.HandleEsc()
	})

	// Wire node connect to open the browser.
	networkDisplay.SetNavigateCallback(func(url string) {
		// TODO: Switch to browser display and load the URL.
		_ = url
	})

	// Wire network keyboard shortcuts
	networkDisplay.OnDeleteSelected = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Delete selected entry?",
			func() {
				if networkDisplay.ShowingNodes() {
					node, ok := networkDisplay.SelectedNode()
					if !ok {
						return
					}
					hash, ok := app.SourceHashFromHex(node.SourceHash)
					if ok {
						a.ForgetNode(hash)
					}
					refreshNodes()
				} else {
					ann, ok := networkDisplay.SelectedAnnounce()
					if !ok {
						return
					}
					a.RemoveAnnounce(ann.TimestampF)
					refreshAnnounces()
				}
			},
			func() {},
		)
	}
	networkDisplay.OnShowPeers = func() {
		// Python reinit_lxmf_peers (Network.py:1717): rebuild the peer list
		// from the LXMF message router before show_peers swaps it in. The
		// message router's peer set is not wired until Phase 5, so refresh
		// with the (currently empty) set; the no-content branch renders.
		networkDisplay.UpdateLXMFPeers(nil)
	}
	networkDisplay.OnURLDialog = func() {
		tuiApp.Dialogs.ShowInputDialog("Navigate",
			"Enter URL:", "",
			func(text string) {
				if text != "" {
					networkDisplay.SetNavigateCallback(func(url string) { _ = url })
				}
			},
			func() {},
		)
	}
	networkDisplay.OnSaveNode = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Save selected node?",
			func() {
				if ann, ok := networkDisplay.SelectedAnnounce(); ok {
					hash, ok := app.SourceHashFromHex(ann.SourceHash)
					if ok {
						a.SaveNode(hash, ann.DisplayName)
					}
				} else if node, ok := networkDisplay.SelectedNode(); ok {
					hash, ok := app.SourceHashFromHex(node.SourceHash)
					if ok {
						a.SaveNode(hash, node.DisplayName)
					}
				}
			},
			func() {},
		)
	}
	// Resolve directory-backed fields for the AnnounceInfo view (Python
	// AnnounceInfo __init__: trust_level, simplest_display_str, op_str). The
	// operator string needs RNS identity recall to compute the lxmf.delivery
	// hash (Phase 5), so it is "Unknown" until then.
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
	// user-selected PN) need Phase 5 RNS and are stubbed here; identify-on-
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
	// (Python save_node, Network.py:755-785). Phase 5 wiring: the default-PN
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
	refreshConvs := func() {
		newConvs := a.ConversationList()
		tuiConvs = make([]tui.ConversationInfo, len(newConvs))
		for i, c := range newConvs {
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
			}
		}
		conversationsDisplay.SetConversations(tuiConvs)
	}

	// Wire conversation keyboard shortcuts
	conversationsDisplay.OnDeleteConv = func() {
		idx := conversationsDisplay.GetSelectedIndex()
		if idx < 0 || idx >= len(tuiConvs) {
			return
		}
		conv := tuiConvs[idx]
		tuiApp.Dialogs.ShowConfirmDialog("Delete conversation with "+conv.DisplayName+"?",
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
			hash, ok := app.SourceHashFromHex(addrHex)
			if !ok {
				return false
			}
			a.CreateDirectoryEntry(hash, name)
			var trustByte byte
			switch trust {
			case "trusted":
				trustByte = directory.TrustTrusted
			case "unknown":
				trustByte = directory.TrustUnknown
			default:
				trustByte = directory.TrustUntrusted
			}
			a.SetPeerTrustLevel(hash, trustByte)
			// Reveal the new entry: switch to the Untrusted tab unless the
			// entry was created trusted (Conversations.py:1066-1068).
			if trust != "trusted" {
				conversationsDisplay.SetShowTrusted(false)
			}
			refreshConvs()
			conversationsDisplay.DisplayConversation(addrHex)
			return true
		})
	}
	conversationsDisplay.OnToggleSort = func() {
		conversationsDisplay.ToggleSort()
	}
	conversationsDisplay.OnShowQR = func() {
		addr := a.LXMFAddressHex()
		text := "[gray]LXMF address display — not available[-]"
		if addr != "" {
			text = fmt.Sprintf("[lightblue]LXMF Address[-]\n\n[white]<%s>[-]", addr)
		}
		tuiApp.Dialogs.ShowDialog("LXMF Address",
			tview.NewTextView().
				SetDynamicColors(true).
				SetTextAlign(tview.AlignCenter).
				SetText(text),
			50, 8, nil)
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
			// Ping requires a full RNS link-establishment flow with async
			// established/closed callbacks and a timeout (Python
			// _ping_peer_from_dialog, Conversations.py:705-768). It is wired in
			// a later parity pass; the button is drawn for layout parity.
			OnPing: nil,
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
		tuiApp.Dialogs.ShowInputDialog("Ingest LXM URI",
			"URI:", "",
			func(text string) {
				if text == "" {
					return
				}
				if a.Router == nil {
					conversationsDisplay.ShowIngestResult(tui.IngestError)
					return
				}
				outcome, err := a.Router.IngestLXMURIOutcome(text)
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
			},
			func() {},
		)
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
			},
			func() {},
		)
	}
	pingPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowDialog("Ping",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText(fmt.Sprintf("[gray]Pinging %s...[-]", sourceHash[:8])),
			40, 5, nil)
		// TODO: Actual ping via RNS transport
	}
	_ = blockPeer
	_ = unblockPeer
	_ = pingPeer

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

	// Send hook: C-d in the open conversation's composer builds the outbound
	// LXMF message (App.SendConversation wires Conversation.SetSendDeps and
	// dispatches via the router) and refreshes the list so the new message's
	// last-activity/unread state shows. This closes the "Wire conversation
	// send" gap (TODO Phase 1).
	conversationsDisplay.OnSend = func(sourceHash, content, title string, attachments []string) {
		a.SendConversation(sourceHash, content, title, attachments...)
		refreshConvs()
		conversationsDisplay.ReloadCurrentMessages()
	}

	// Message-loading hooks: supply the open conversation's messages (parsed
	// from disk via App.ConversationMessages), this app's own LXMF hash (so
	// the LXMessageWidget header can tell outbound from inbound), and the
	// configured time format. Together these let the ConversationWidget render
	// Python-parity message headers + bodies (TODO Phase 1 "ConversationWidget
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

	// Network "Converse" / "Msg Op": open a conversation with the selected
	// announce's identity — create a directory entry, refresh the conversation
	// list, and switch to the Conversations page. (Python converse(msg) /
	// msg_op resolve the operator's LXMF address; until RNS identity resolution
	// is wired, both open a conversation with the announce's own identity.)
	openConversationFromAnnounce := func() {
		ann, ok := networkDisplay.SelectedAnnounce()
		if !ok {
			return
		}
		hash, ok := app.SourceHashFromHex(ann.SourceHash)
		if !ok {
			return
		}
		a.CreateDirectoryEntry(hash, ann.DisplayName)
		refreshConvs()
		main.SelectPage("conversations")
	}
	networkDisplay.OnConverse = openConversationFromAnnounce
	networkDisplay.OnMsgOp = openConversationFromAnnounce

	// Channels display
	channelsDisplay := tui.NewChannelsDisplay(tuiApp, nil)
	main.SetDisplay("channels", channelsDisplay.Widget())
	main.SetShortcut("channels", "[C-n] New Hub  [C-a] Add Room  [C-r] Connect  [C-w] Disconnect  [C-t] Auto-reconnect  [C-e] Edit Hub  [C-x] Remove")

	// Wire channel keyboard shortcuts
	channelsDisplay.OnNewHub = func() {
		tuiApp.Dialogs.ShowInputDialog("New Hub",
			"Hub address (hex hash):", "",
			func(text string) {
				if text == "" {
					return
				}
				name := tui.TruncateString(text, 8)
				_ = a
				// TODO: Call a.AddHub when app method exists
				_ = text
				_ = name
			},
			func() {},
		)
	}
	channelsDisplay.OnJoinRoom = func() {
		tuiApp.Dialogs.ShowInputDialog("Join Room",
			"Room name:", "",
			func(text string) {
				if text == "" {
					return
				}
				_ = text
				// TODO: Join room via RRC hub
			},
			func() {},
		)
	}
	channelsDisplay.OnRemoveHub = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Remove selected hub/room?",
			func() {
				// TODO: Remove hub/room via RRC hub
			},
			func() {},
		)
	}
	channelsDisplay.OnEditHub = func() {
		tuiApp.Dialogs.ShowInputDialog("Edit Hub",
			"Display name:", "",
			func(text string) {
				if text == "" {
					return
				}
				// TODO: Update hub display name
				_ = text
			},
			func() {},
		)
	}
	channelsDisplay.OnConnect = func() {
		tuiApp.Dialogs.ShowDialog("Connect",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText("[gray]Connecting to hub...[-]"),
			40, 5, nil)
		// TODO: Connect to hub via RRC
	}
	channelsDisplay.OnDisconnect = func() {
		tuiApp.Dialogs.ShowDialog("Disconnect",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText("[gray]Disconnected from hub.[-]"),
			40, 5, nil)
		// TODO: Disconnect from hub
	}
	channelsDisplay.OnToggleAutoReconnect = func() {
		// TODO: Toggle auto-reconnect setting
	}

	// Config display
	configPath := a.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(a.ConfigDir, "config")
	}
	configDisplay := tui.NewConfigDisplay(tuiApp, configPath)
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

	// Begin live tailing (Python's LogTerminal runs `tail -fn50` continuously
	// while the page exists). It is a no-op when the log file is absent. The
	// returned cleanup stops the goroutine on shutdown.
	logDisplay.StartTailing()

	// Guide display
	guideDisplay := tui.NewGuideDisplay(tuiApp)
	main.SetDisplay("guide", guideDisplay.Widget())
	main.SetShortcut("guide", "")

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
	go func() {
		for range ifaceTicker.C {
			refreshInterfaces()
		}
	}()

	// Wire interfaces keyboard shortcuts
	interfacesDisplay.OnAddInterface = func() {
		tuiApp.Dialogs.ShowInputDialog("Add Interface",
			"Interface type:", "",
			func(text string) {
				_ = text
				// TODO: Add interface via RNS config
			},
			func() {},
		)
	}
	interfacesDisplay.OnConfigEditor = func() {
		tuiApp.Dialogs.ShowDialog("Config Editor",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText(fmt.Sprintf("[gray]Config file: %s[-]\n\n[gray]Edit this file with your text editor.[-]", configPath)),
			60, 8, nil)
	}
	interfacesDisplay.OnEditInterface = func() {
		tuiApp.Dialogs.ShowInputDialog("Edit Interface",
			"Interface name:", "",
			func(text string) {
				_ = text
				// TODO: Edit interface via RNS config
			},
			func() {},
		)
	}
	interfacesDisplay.OnRemoveInterface = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Remove selected interface?",
			func() {
				// TODO: Remove interface via RNS config
			},
			func() {},
		)
	}
	interfacesDisplay.OnShowInterface = func(idx int) {
		items := interfacesDisplay.Items()
		if idx < 0 || idx >= len(items) {
			return
		}
		iface := items[idx]
		status := "disconnected"
		if iface.Connected {
			status = "connected"
		}
		tuiApp.Dialogs.ShowDialog("Interface: "+iface.Name,
			tview.NewTextView().
				SetDynamicColors(true).
				SetText(fmt.Sprintf("[::b]%s[-]\n\nType: %s\nStatus: %s\nTarget: %s",
					iface.Name, iface.Type, status, iface.Target)),
			50, 9, nil)
	}

	// Browser display (hidden by default, shown when navigating from network)
	browserDisplay := tui.NewBrowserDisplay(tuiApp)
	main.SetDisplay("browser", browserDisplay.Widget())
	main.SetShortcut("browser", "[C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-s] Save  [C-y] Copy URL  [C-g] Fullscreen")

	// Wire browser keyboard shortcuts
	browserDisplay.OnBack = func() { browserDisplay.GoBack() }
	browserDisplay.OnForward = func() { browserDisplay.GoForward() }
	browserDisplay.OnReload = func() { browserDisplay.Reload() }
	browserDisplay.OnCopyURL = func() {
		url := browserDisplay.CurrentURL()
		if url != "" {
			_ = tui.CopyToClipboard(url)
		}
	}
	browserDisplay.OnURLDialog = func() {
		tuiApp.Dialogs.ShowInputDialog("Navigate",
			"Enter URL:", "",
			func(text string) {
				if text != "" {
					browserDisplay.LoadURL(text)
				}
			},
			func() {},
		)
	}
	browserDisplay.OnSaveNode = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Save connected node to directory?",
			func() { /* TODO */ },
			func() {},
		)
	}
	browserDisplay.OnDisconnect = func() {
		browserDisplay.SetContent("[gray]Disconnected.[-]")
	}
	browserDisplay.OnToggleFullscreen = func() {
		// TODO: Toggle fullscreen
	}

	// Wire network connect to browser
	networkDisplay.SetNavigateCallback(func(url string) {
		main.SetDisplay("browser", browserDisplay.Widget())
		browserDisplay.LoadURL(url)
	})

	// Intro/splash display
	introDisplay := tui.NewIntroDisplay("Nomad Network", a.Version)
	main.SetDisplay("quit", introDisplay.Widget())

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
