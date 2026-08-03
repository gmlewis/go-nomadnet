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
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-nomadnet/nomadnet/rrc"
	"github.com/gmlewis/go-nomadnet/tui"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
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

	// navigateTo is assigned once the browser display is constructed (below),
	// then invoked both by the network list's connect handler and by the Local
	// Node Info "Browse" button (Python connect_query,
	// Network.py:1402-1404/browse_own). It is captured here so the node-info
	// callback — wired before the browser exists — can close over it and call
	// the live value at click time.
	navigateTo := func(string) {}
	_ = navigateTo

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
			// Node Info panel (Python node_info_query, Network.py:1399-1401),
			// building the panel from the hosted node's live state. When no node
			// is hosted (EnableNode false) the panel renders the "not hosting a
			// node" branch (Python NodeInfo else-branch, Network.py:1541-1551).
			data := buildNodeInfoData(a, navigateTo, func() {
				tuiApp.Dialogs.ShowDialog("Announce Sent",
					tview.NewTextView().
						SetDynamicColors(true).
						SetTextAlign(tview.AlignCenter).
						SetText("\n\n\nAnnounce Sent\n\n\n"),
					40, 10, nil)
			})
			networkDisplay.ShowNodeInfo(data)
		},
	)

	// refreshAnnounces re-fetches the announce stream from the app and updates
	// the network display's left pane. It reads the persisted directory
	// announce stream (a.DirAnnounceEvents, mirroring Python's AnnounceStream
	// widget iterating app.directory.announce_stream, Network.py:489) rather
	// than the ephemeral a.Announces feed, so the panel populates at boot from
	// the previous run's discovered nodes loaded by Dir.LoadFromDisk.
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
	// Wire up real announce data via callback
	a.SetUIChangeCallback(func() {
		tuiApp.QueueUpdateDraw(func() {
			refreshAnnounces()
			refreshNodes()
			refreshConvs()
			// RNS init runs asynchronously in a goroutine, so the identity/LXMF
			// destination are nil when wireDisplays first runs. Re-filling the
			// Local Peer Info panel on each UI change picks them up once initRNS
			// completes (it fires UIChangeCallback at the end), and also refreshes
			// the "Announced : …" line as the announce age advances.
			lxmfAddr, idhash, dname, lann := localPeerInfo()
			networkDisplay.UpdateLocalPeer(lxmfAddr, idhash, dname, lann)
		})
	})
	main.SetDisplay("network", networkDisplay.Widget())
	main.SetShortcut("network", "[C-l] Nodes/Announces  [C-x] Remove  [C-w] Disconnect  [C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-g] Fullscreen  [C-s / C-b] Save Node")

	// Wire Esc to go back from AnnounceInfo before quitting.
	main.SetEscCallback(func() bool {
		return networkDisplay.HandleEsc()
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
		msg := tview.NewTextView().
			SetDynamicColors(true).
			SetTextAlign(tview.AlignCenter).
			SetText("\nA delivery sync of all unhandled LXMs was manually requested for the selected node\n")
		okBtn := tview.NewButton("OK").SetSelectedFunc(func() {
			tuiApp.Dialogs.DismissTop()
			refreshLXMFPeers()
		})
		layout := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(msg, 0, 1, false).
			AddItem(okBtn, 1, 0, true)
		tuiApp.Dialogs.ShowDialog("!", layout, 60, 6, nil)
	}
	networkDisplay.OnURLDialog = func() {
		tuiApp.Dialogs.ShowInputDialog("Navigate",
			"Enter URL:", "",
			func(text string) {
				if text != "" {
					navigateTo(browser.NormalizeEnteredURL(text))
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
		// RNS-derived fields (Network.py:629-634,681-688,791-800): operator display
		// (lxmf.delivery hash), hop distance (Transport.hops_to), the centered
		// LXMF Propagation Node address line (lxmf.propagation hash), and the
		// default-PN checkbox preselection (current user-selected PN == pn_hash).
		data.OpStr = a.NodeOperatorDisplay(hash)
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
			text = fmt.Sprintf("[lightblue]LXMF Address[-]\n\n[white]<%v>[-]", addr)
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
				SetText(fmt.Sprintf("[gray]Pinging %v...[-]", sourceHash[:8])),
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
	main.SetShortcutCallback("guide", guideDisplay.Shortcuts)

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
		// on the RNS config (self.app.rns.configpath), titled "Editing RNS
		// Config". tview has no embedded terminal widget, so suspend the screen
		// and let the editor own the real terminal — the same path the Config
		// page's "Open Editor" button uses (ConfigDisplay.openEditor).
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
		tuiApp.Application.Suspend(func() {
			cmd := exec.Command(editor, rnsPath)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
		})
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
				SetText(fmt.Sprintf("[::b]%v[-]\n\nType: %v\nStatus: %v\nTarget: %v",
					iface.Name, iface.Type, status, iface.Target)),
			50, 9, nil)
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

	// Wire browser keyboard shortcuts
	browserDisplay.OnBack = func() { browserDisplay.GoBack() }
	browserDisplay.OnForward = func() { browserDisplay.GoForward() }
	browserDisplay.OnReload = func() {
		// Python reload() uncaches the current URL then re-loads, so a reload
		// always re-fetches instead of returning a stale cache hit.
		pageCache.UncachePage(browserDisplay.CurrentURL())
		browserDisplay.Reload()
	}
	browserDisplay.OnCopyURL = func() {
		// Python BrowserFrame.keypress ctrl y (Browser.py:38-40) only calls
		// copy_url when config.textui.clipboard_copy is enabled, and copy_url
		// (Browser.py:1103-1133) copies the current URL via the OSC 52 escape
		// sequence (osc52_copy) — no platform clipboard tool. With no URL or
		// the feature disabled, C-y is a no-op.
		if a.Config == nil || !a.Config.TextUI.ClipboardCopy {
			return
		}
		url := browserDisplay.CurrentURL()
		if url == "" {
			return
		}
		_ = tui.OSC52Copy(url)
	}
	browserDisplay.OnURLDialog = func() {
		// Python Browser.url_dialog (Browser.py:1135-1182): pre-fill the
		// current URL, let the user edit it, and on "Go" apply the "|"`→"`"
		// normalization (a user can type "hash:path|x=1" instead of using a
		// backtick) then retrieve_url.
		tuiApp.Dialogs.ShowInputDialog("Enter URL",
			"URL : ", browserDisplay.CurrentURL(),
			func(text string) {
				text = strings.TrimSpace(text)
				if text == "" {
					return
				}
				browserDisplay.LoadURL(browser.NormalizeEnteredURL(text))
			},
			func() {},
		)
	}
	browserDisplay.OnSaveNode = func() {
		// Python Browser.save_node_dialog (Browser.py:1184-1234): only when a
		// destination is connected; recall the announced app_data display name;
		// confirm "Save connected node "<name>" <prettyhex> to Known Nodes?";
		// on Save, remember a DirectoryEntry(hosts_node=True) and fire the
		// directory-change callback (refresh the network known-nodes pane).
		dest := browserDisplay.CurrentDest()
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
		tuiApp.Dialogs.ShowConfirmDialog(msg, func() {
			a.SaveConnectedNode(dest, dispName)
			refreshNodes()
		}, func() {})
	}
	browserDisplay.OnDisconnect = func() {
		// Python Browser.disconnect (Browser.py:862-881) clears history, resets
		// the pointer, and drops the current-destination hint — the BrowserDisplay
		// owns that state; the link itself is one-shot per fetch in the Go port
		// (no persistent self.link to tear down).
		browserDisplay.Disconnect()
	}
	browserDisplay.OnJumpAnchor = func(name string) {
		// Python Browser._jump_to_anchor (Browser.py:324-357): scroll the page
		// content to the named anchor (or the next heading below the cursor for
		// a bare "#"). Pure local UI — no network involved.
		browserDisplay.JumpToAnchor(name)
	}
	browserDisplay.OnOpenLXMF = func(hashHex string) {
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
	browserDisplay.OnOpenRRC = func(hubHex, room string) {
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
	browserDisplay.OnPartialUpdate = func(ids []string) {
		// Python Browser.handle_partial_updates (Browser.py:823-834): a
		// "p:<id>:<id>" link forces an immediate re-fetch of the named partials.
		// The BrowserDisplay owns the partial-refresh loop and substitution; this
		// just selects the matching partials and re-fetches them off-thread.
		browserDisplay.RefreshPartials(ids)
	}
	browserDisplay.OnBrowserError = func(msg string) {
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
	browserDisplay.OnToggleFullscreen = func() {
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
	// OnFetchPartial fetches one page partial over RNS, resolving its (possibly
	// relative) URL against the page's current destination hash. Wired to the
	// browser fetch backend; the BrowserDisplay runs the refresh loop and
	// substitutes the result into the page (Python Browser.__load_partial +
	// partial_received). identifyOnConnect mirrors Python link_established
	// (Browser.py:1454-1459): when the directory entry requests it, identify to
	// the remote node over the freshly established link.
	identifyOnConnect := func(destHash []byte) func(*rns.Link) {
		if a.Dir == nil || !a.Dir.ShouldIdentifyOnConnect(destHash) || a.Identity == nil {
			return nil
		}
		identity := a.Identity
		return func(link *rns.Link) { _ = link.Identify(identity) }
	}
	browserDisplay.OnFetchPartial = func(p browser.Partial) ([]byte, error) {
		return browser.FetchPartial(a.Transport, p, browserDisplay.CurrentDest(),
			time.Duration(browser.DefaultTimeout)*time.Second, nil,
			identifyOnConnect(browserDisplay.CurrentDest()))
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
	wireBrowser := func(bd *tui.BrowserDisplay) {
		if bd == nil {
			return
		}
		bd.OnRetrieveURL = func(url string) {
			dest, path, rd, err := browser.ParseURL(url, bd.CurrentDest(), nil)
			if err != nil {
				tuiApp.QueueUpdateDraw(func() {
					bd.SetContent(fmt.Sprintf("[red]Invalid URL: %v[-]", err))
				})
				return
			}
			bd.SetCurrentDest(dest)
			canonURL := fmt.Sprintf("%x:%v", dest, path)
			if rd == nil {
				if cached := pageCache.GetCached(canonURL); cached != nil {
					tuiApp.QueueUpdateDraw(func() {
						bd.RenderPage(string(cached))
					})
					return
				}
			}
			go func() {
				data, ferr := browser.FetchPage(a.Transport, dest, path, rd,
					time.Duration(browser.DefaultTimeout)*time.Second, nil,
					identifyOnConnect(dest))
				tuiApp.QueueUpdateDraw(func() {
					if ferr != nil {
						bd.SetContent(fmt.Sprintf("[red]%v[-]", browser.StatusText(browser.ErrToStatus(ferr))))
						return
					}
					if rd == nil {
						if ct := tui.CacheTimeFromMarkup(string(data)); ct != 0 {
							pageCache.CachePage(canonURL, data,
								float64(time.Now().UnixNano())/1e9+float64(ct))
						}
					}
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
	}

	// Wire network connect to browser (loads page in Network display's Remote Node pane and full browser)
	navigateTo = func(url string) {
		if ndBp := networkDisplay.BrowserPane(); ndBp != nil {
			ndBp.LoadURL(url)
		}
		browserDisplay.LoadURL(url)
	}
	networkDisplay.SetNavigateCallback(navigateTo)

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
	data.Name = a.Node.Name
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
		if a.PeerSettings == nil || a.PeerSettings.NodeLastAnnounce == nil {
			return "Never"
		}
		ts, ok := peerAnnounceUnix(a.PeerSettings.NodeLastAnnounce)
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
	data.ActiveLinks = func() string { return strconv.Itoa(a.Node.ActiveLinks) }

	// Total Connects / Served Pages / Served Files (NodeTotalConnections/Pages/
	// Files, Network.py:1163-1256): the persisted peer-settings counters.
	data.TotalConnects = func() string {
		if a.PeerSettings == nil {
			return "None"
		}
		return strconv.Itoa(a.PeerSettings.NodeConnects)
	}
	data.TotalPages = func() string {
		if a.PeerSettings == nil {
			return "None"
		}
		return strconv.Itoa(a.PeerSettings.ServedPageRequests)
	}
	data.TotalFiles = func() string {
		if a.PeerSettings == nil {
			return "None"
		}
		return strconv.Itoa(a.PeerSettings.ServedFileRequests)
	}

	// Browse → load the node's own page in the browser (connect_query,
	// Network.py:1402-1404). The Phase 2 RNS page fetch is wired through the
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
