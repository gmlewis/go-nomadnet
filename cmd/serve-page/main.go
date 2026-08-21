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

// Command serve-page runs a headless gonomadnet node that serves a directory of
// micron pages (.mu) over a TCP RNS interface on the loopback address, and
// prints its nomadnetwork.node destination hash. It is the fixture-serving node
// for the local-loopback parity comparator (workflow C): both the Python
// nomadnet client and the gonomadnet client connect to this server over a
// TCPClientInterface and browse the same served page, so their renders can be
// diffed with no real network and deterministic content.
//
// The server uses a regular TCPServerInterface (the standard cross-implementation
// RNS transport), not the shared-instance local interface, so external Python
// clients can connect. The fixture .mu must be at <pages>/index.mu for the
// default /page/index.mu request.
//
// Usage:
//
//	serve-page -pages <dir> [-files <dir>] [-port N] [-bind ADDR] [-name NAME]
//
// On startup it prints two lines to stdout:
//
//	NODE_HASH=<32-hex>
//	PORT=<tcp-port>
//
// and runs until interrupted (SIGINT/SIGTERM). Diagnostics go to stderr.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
)

func main() {
	log.SetFlags(0)

	pages := flag.String("pages", "", "directory of .mu pages to serve (index.mu is the home page)")
	files := flag.String("files", "", "directory of served files (defaults to -pages)")
	port := flag.Int("port", 0, "TCP port to listen on (0 = auto-pick a free loopback port)")
	bind := flag.String("bind", "127.0.0.1", "bind address for the TCP interface")
	name := flag.String("name", "parity-node", "node name (announced aspect)")
	rnsConfig := flag.String("rnsconfig", "", "RNS config dir (default: a fresh temp dir under /tmp)")
	verbose := flag.Bool("v", false, "verbose RNS logging")
	flag.Parse()

	if *pages == "" {
		flag.Usage()
		os.Exit(2)
	}
	if _, err := os.Stat(*pages); err != nil {
		log.Printf("-pages dir not usable: %v", err)
		os.Exit(2)
	}
	filesDir := *files
	if filesDir == "" {
		filesDir = *pages
	}
	cfgDir := *rnsConfig
	if cfgDir == "" {
		d, err := os.MkdirTemp("/tmp", "serve-page-rns-*")
		if err != nil {
			log.Printf("create rns config dir: %v", err)
			os.Exit(1)
		}
		cfgDir = d
		defer func() { _ = os.RemoveAll(cfgDir) }()
	}
	if err := writeRNSConfig(cfgDir, *verbose); err != nil {
		log.Printf("write rns config: %v", err)
		os.Exit(1)
	}

	// Route all RNS logs to stderr so stdout carries only the NODE_HASH/PORT
	// lines the harness parses.
	logger := rns.NewLogger()
	level := rns.LogError
	if *verbose {
		level = rns.LogInfo
	}
	logger.SetLogCallback(func(msg string) { log.Print(msg) })
	logger.SetLogDest(rns.LogCallback)
	logger.SetLogLevel(level)

	ts := rns.NewTransportSystem(logger)
	ret, err := rns.NewReticulum(ts, cfgDir)
	if err != nil {
		log.Printf("Could not initialize Reticulum: %v", err)
		os.Exit(1)
	}
	logger.SetLogLevel(level) // applyConfig may have reset it from the config file.
	defer func() {
		ts.Stop()
		if err := ret.Close(); err != nil {
			log.Printf("Could not close Reticulum: %v", err)
		}
	}()

	// TCPServerInterface.BindPort reports the configured port, not the actually
	// listened one, so a -port of 0 (auto) would print PORT=0. Resolve a free
	// loopback port up front via a throwaway listener when none was given.
	listenPort := *port
	if listenPort == 0 {
		free, err := freeLoopbackPort(*bind)
		if err != nil {
			log.Printf("pick free port: %v", err)
			os.Exit(1)
		}
		listenPort = free
	}

	server, err := interfaces.NewTCPServerInterface(
		"parity-tcp", *bind, listenPort,
		func(data []byte, iface interfaces.Interface) { ts.Inbound(data, iface) },
		nil,
	)
	if err != nil {
		log.Printf("NewTCPServerInterface: %v", err)
		os.Exit(1)
	}
	ts.RegisterInterface(server)
	defer func() { _ = server.Detach() }()

	n := node.NewNode(*name, *pages, filesDir, 720, 0, 0, true)
	if err := n.Start(ts, ts.Identity()); err != nil {
		log.Printf("node Start: %v", err)
		os.Exit(1)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		log.Printf("node Announce: %v", err)
		os.Exit(1)
	}

	fmt.Printf("NODE_HASH=%x\n", n.Destination().Hash)
	fmt.Printf("PORT=%d\n", server.BindPort())
	_ = os.Stdout.Sync()

	// Keep serving until interrupted.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

// writeRNSConfig writes a minimal standalone RNS config (share_instance = No)
// so the node gets its own identity and does not collide with the user's
// ~/.reticulum or any running shared instance.
func writeRNSConfig(cfgDir string, verbose bool) error {
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	level := "4" // ERROR
	if verbose {
		level = "7" // DEBUG
	}
	content := "[reticulum]\nshare_instance = No\n\n[logging]\nloglevel = " + level + "\n"
	return os.WriteFile(cfgDir+"/config", []byte(content), 0o600)
}

// freeLoopbackPort returns a free TCP port on the given bind address by opening
// a throwaway listener and closing it. There is a small race window before the
// real interface binds, which is acceptable for a local loopback test harness.
func freeLoopbackPort(bind string) (int, error) {
	l, err := net.Listen("tcp", bind+":0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
