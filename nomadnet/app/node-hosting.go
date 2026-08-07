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

// Package app implements the NomadNet application core.
//
// node-hosting.go wires the hosted Nomad Network node (nomadnet/node.Node) into
// the App, mirroring Python NomadNetworkApp.py:367-401 where, when enable_node
// is set, the app constructs nomadnet.Node(self) — which creates the
// "nomadnetwork.node" destination, registers page/file request handlers, starts
// the node job thread, and (optionally) announces at start. The Go node package
// splits construction (NewNode), destination+handler registration (Start), the
// job loop (Jobs), and announcing (Announce) into separate methods, so this file
// orchestrates them on the app's transport/identity and runs the job loop in a
// background goroutine.

package app

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/rns"
)

//go:embed default-index.mu
var defaultIndexContent string

// ensureDefaultPages creates the pages directory and a default index.mu file
// if they don't already exist. This gives node operators a starter page to
// customize rather than serving only the in-memory fallback.
func (a *App) ensureDefaultPages() error {
	// Create pages directory if it doesn't exist
	if _, err := os.Stat(a.PagesPath); os.IsNotExist(err) {
		if err := os.MkdirAll(a.PagesPath, 0o755); err != nil {
			return err
		}
	}

	// Create default index.mu if it doesn't exist
	indexPath := filepath.Join(a.PagesPath, "index.mu")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		if err := os.WriteFile(indexPath, []byte(defaultIndexContent), 0o644); err != nil {
			return err
		}
	}

	return nil
}

// startNode constructs, starts, and runs the hosted node when EnableNode is
// true. It mirrors Python NomadNetworkApp.py:399 (self.node = nomadnet.Node(self))
// combined with Node.py:14-51 (Node.__init__): the node name falls back to
// "<peer display name>'s Node" when unset (Node.py:35-36), the node is started
// on the app's transport/identity, its background job loop runs in a goroutine
// (Node.py:49-51), and an announce-at-start fires after START_ANNOUNCE_DELAY
// when NodeAnnounceAtStart is set (Node.py:40-47). It is a no-op when
// EnableNode is false (Python leaves self.node = None). It returns an error if
// the node cannot be started; the production caller (initRNS) logs and
// continues so a node start failure never aborts the client. (InitWithTransport,
// the test harness, does not auto-call startNode — tests call it explicitly.)
func (a *App) startNode() error {
	if !a.EnableNode {
		return nil
	}
	if a.Node != nil {
		return nil
	}
	if a.Transport == nil || a.Identity == nil {
		return errors.New("cannot start node: transport or identity not initialized")
	}

	// Ensure the pages directory exists and has a default index.mu
	if err := a.ensureDefaultPages(); err != nil {
		return err
	}

	name := a.nodeName()

	n := node.NewNode(
		name,
		a.PagesPath,
		a.FilesPath,
		a.NodeAnnounceInterval,
		a.PageRefreshInterval,
		a.FileRefreshInterval,
		a.NodeAnnounceAtStart,
	)
	if err := n.Start(a.Transport, a.Identity); err != nil {
		return err
	}
	a.Node = n

	// Wire the node's event callbacks to persist counters to peer_settings,
	// mirroring Python's peer_connected/serve_page/serve_file/announce paths
	// (Node.py:111-112,194-195,218) which write peer_settings and save.
	a.Node.OnPeerConnected = a.onNodePeerConnected
	a.Node.OnPageServed = a.onNodePageServed
	a.Node.OnFileServed = a.onNodeFileServed
	a.Node.OnAnnounced = a.onNodeAnnounced

	// Announce at start (Node.py:40-47): a daemon thread sleeps
	// START_ANNOUNCE_DELAY then sends the first node announce.
	if a.NodeAnnounceAtStart {
		go func() {
			time.Sleep(startAnnounceDelay)
			if err := n.Announce(); err != nil && a.Logger != nil {
				a.Logger.Error("node announce-at-start failed: %v", err)
			}
		}()
	}

	// Run the node job loop in the background (Node.py:49-51).
	go n.Jobs()

	return nil
}

// onNodePeerConnected persists the incremented node-connects counter, mirroring
// Python Node.peer_connected (Node.py:111-112): peer_settings["node_connects"]
// += 1; save_peer_settings.
func (a *App) onNodePeerConnected() {
	a.psMu.Lock()
	defer a.psMu.Unlock()
	if a.PeerSettings == nil {
		return
	}
	a.PeerSettings.NodeConnects++
	a.savePeerSettingsLocked()
}

// onNodePageServed persists the incremented served-page-requests counter,
// mirroring Python Node.serve_page (Node.py:111-112,194-195).
func (a *App) onNodePageServed() {
	a.psMu.Lock()
	defer a.psMu.Unlock()
	if a.PeerSettings == nil {
		return
	}
	a.PeerSettings.ServedPageRequests++
	a.savePeerSettingsLocked()
}

// onNodeFileServed persists the incremented served-file-requests counter,
// mirroring Python Node.serve_file (Node.py:194-195).
func (a *App) onNodeFileServed() {
	a.psMu.Lock()
	defer a.psMu.Unlock()
	if a.PeerSettings == nil {
		return
	}
	a.PeerSettings.ServedFileRequests++
	a.savePeerSettingsLocked()
}

// onNodeAnnounced persists the node-last-announce timestamp, mirroring Python
// Node.announce (Node.py:218): peer_settings["node_last_announce"] =
// self.last_announce; save_peer_settings.
func (a *App) onNodeAnnounced() {
	a.psMu.Lock()
	defer a.psMu.Unlock()
	if a.PeerSettings == nil {
		return
	}
	a.PeerSettings.NodeLastAnnounce = time.Now().Unix()
	a.savePeerSettingsLocked()
}

// ResetNodeStats zeros the hosted-node stat counters (node_connects,
// served_page_requests, served_file_requests) and persists them, mirroring
// Python NodeInfo.stats_query (Network.py:1391-1394). It also resets the node's
// in-memory counters so the live "Connected Now" / session counts stay
// consistent.
func (a *App) ResetNodeStats() {
	a.psMu.Lock()
	if a.PeerSettings != nil {
		a.PeerSettings.NodeConnects = 0
		a.PeerSettings.ServedPageRequests = 0
		a.PeerSettings.ServedFileRequests = 0
		a.savePeerSettingsLocked()
	}
	a.psMu.Unlock()
	if a.Node != nil {
		a.Node.ResetStats()
	}
}

// nodeName resolves the hosted node's display name, mirroring Node.py:35-36:
// when the configured node name is empty, the node is named after the peer
// settings display name with "'s Node" appended.
func (a *App) nodeName() string {
	if a.NodeName != "" {
		return a.NodeName
	}
	a.psMu.Lock()
	displayName := ""
	if a.PeerSettings != nil {
		displayName = a.PeerSettings.DisplayName
	}
	a.psMu.Unlock()
	if displayName != "" {
		return displayName + "'s Node"
	}
	return ""
}

// stopNode stops the hosted node's background job loop, mirroring the shutdown
// half of Python's exit_handler (NomadNet sets should_run_jobs false). It is a
// no-op when no node is hosted.
func (a *App) stopNode() {
	if a.Node == nil {
		return
	}
	a.Node.Stop()
}

// NodeStats snapshots the hosted-node peer-settings counters under psMu so
// the TUI render goroutine can read them without racing the node job-loop /
// announce callbacks that mutate them. The bool reports whether settings
// exist (so callers can render "None" when absent).
func (a *App) NodeStats() (connects, pages, files int, ok bool) {
	a.psMu.Lock()
	defer a.psMu.Unlock()
	if a.PeerSettings == nil {
		return 0, 0, 0, false
	}
	return a.PeerSettings.NodeConnects, a.PeerSettings.ServedPageRequests, a.PeerSettings.ServedFileRequests, true
}

// NodeLastAnnounceSetting snapshots the persisted node-last-announce value
// under psMu. It returns (nil, false) when settings are absent.
func (a *App) NodeLastAnnounceSetting() (any, bool) {
	a.psMu.Lock()
	defer a.psMu.Unlock()
	if a.PeerSettings == nil {
		return nil, false
	}
	return a.PeerSettings.NodeLastAnnounce, true
}

// NodeDestinationHash returns the "nomadnetwork.node" destination hash for the
// app's identity, or nil before the identity is loaded. This is the hash the
// hosted node announces (Node.py:18) and the Network page shows in the Local
// Peer Info panel; exposing it lets the UI render the node address before the
// node's own destination is constructed (and keeps the app/node dependency
// direction one-way).
func (a *App) NodeDestinationHash() []byte {
	if a.Identity == nil {
		return nil
	}
	return rns.CalculateHash(a.Identity, "nomadnetwork", "node")
}
