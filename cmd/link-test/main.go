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

// Command link-test establishes a RNS Link to a remote destination and reports
// whether it succeeds, measuring the full path: path discovery → link request
// → link proof → establishment. It connects to the running shared instance
// (gonomadnet/gornsd) by default, so it shares the cached path table.
//
// Unlike gornprobe (which sends a DATA packet + receipt) or gornsh (which
// targets the rnsh.* destination), link-test creates a proper Link to the
// specified RNS service destination — the same mechanism the nomadnet browser
// uses to fetch pages. Use it to diagnose "Retrieving…" stuck browsers and
// "link establishment timed out" errors.
//
// Usage:
//
//	link-test [options] <destination-hash>
//
// The destination hash is the 32-hex hash of the target's RNS destination.
// For nomadnet nodes, this is the nomadnetwork.node destination hash (the
// entries in known-nodes.txt). The --full-name flag selects the RNS service
// namespace (default "nomadnetwork.node").
//
// Options:
//
//	--config DIR     RNS config directory (default ~/.reticulum)
//	--full-name N    dotted RNS destination name (default "nomadnetwork.node")
//	--timeout SECS   link establishment timeout (default 30)
//	--path-timeout SECS  path discovery timeout (default 30)
//	-v               verbose RNS logging
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

func main() {
	log.SetFlags(0)
	configDir := flag.String("config", "", "RNS config directory (default ~/.reticulum)")
	fullName := flag.String("full-name", "nomadnetwork.node", "dotted RNS destination name")
	timeoutSec := flag.Float64("timeout", 30, "link establishment timeout in seconds")
	_ = flag.Float64("path-timeout", 30, "path discovery timeout in seconds (unused; kept for compat)")
	verbose := flag.Bool("v", false, "verbose RNS logging")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: link-test [options] <destination-hash>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Establishes a RNS Link to <destination-hash> via the")
		fmt.Fprintln(os.Stderr, "running shared instance and reports success/failure.")
		fmt.Fprintln(os.Stderr, "")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	destHashStr := strings.TrimSpace(flag.Arg(0))
	destHash, err := hex.DecodeString(destHashStr)
	if err != nil || len(destHash) != rns.TruncatedHashLength/8 {
		fmt.Fprintf(os.Stderr, "link-test: invalid destination hash %q (want %d hex chars)\n", destHashStr, rns.TruncatedHashLength/4)
		os.Exit(2)
	}

	dir := *configDir
	if dir == "" {
		dir = os.Getenv("HOME") + "/.reticulum"
	}

	logger := rns.NewLogger()
	if *verbose {
		// SetLogCallback alone does nothing: the destination must also switch
		// to the synchronous LogCallback sink for the callback to fire.
		logger.SetLogDest(rns.LogCallback)
		logger.SetLogCallback(func(msg string) { log.Print(msg) })
	}

	ts := rns.NewTransportSystem(logger)
	ret, err := rns.NewReticulumWithLogger(ts, dir, logger)
	if err != nil {
		// Flush the async queue so the init diagnostics explaining the
		// failure are not silently lost.
		logger.Flush()
		fmt.Fprintf(os.Stderr, "link-test: failed to start Reticulum: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ret.Close() }()

	// Wait briefly for the shared-instance LocalClientInterface to connect.
	time.Sleep(500 * time.Millisecond)

	connected := ret.IsConnectedToSharedInstance()
	_ = connected
	fmt.Printf("RNS instance: %s\n", instanceRole(ret))

	// Request a path to the destination, but don't block on it — the link
	// establishment itself will broadcast the link request to all interfaces
	// if no path is known, and a direct TCP connection (e.g. the local hub)
	// can deliver it instantly without needing a path table entry.
	fmt.Printf("Requesting path to %s…\n", destHashStr)
	if err := ts.RequestPath(destHash); err != nil {
		fmt.Printf("Path request error: %v\n", err)
	}
	// Wait briefly for the path response; if it arrives, great. If not,
	// proceed to link establishment anyway (broadcast fallback).
	time.Sleep(3 * time.Second)
	if ts.HasPath(destHash) {
		fmt.Printf("Path found: %d hops\n", ts.HopsTo(destHash))
	} else {
		fmt.Println("No cached path; trying link establishment via broadcast…")
	}

	// Parse the full destination name into app + aspects.
	parts := strings.Split(*fullName, ".")
	if len(parts) < 1 {
		fmt.Fprintf(os.Stderr, "link-test: invalid full-name %q\n", *fullName)
		exit(logger, 2)
	}
	appName := parts[0]
	aspects := parts[1:]

	// Recall the remote identity from known destinations (populated from
	// received announces). This is the same mechanism gornprobe uses:
	// without it, a random identity would produce a different destination
	// hash and the link request would go to a non-existent destination.
	remoteID := rns.RecallIdentity(ts, destHash)
	if remoteID == nil {
		fmt.Println("No known identity for destination; cannot create matching destination")
		exit(logger, 1)
	}
	dest, err := rns.NewDestination(ts, remoteID, rns.DestinationOut, rns.DestinationSingle, appName, aspects...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "link-test: destination error: %v\n", err)
		exit(logger, 1)
	}

	// Create and establish the link.
	link, err := rns.NewLink(ts, dest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "link-test: NewLink error: %v\n", err)
		exit(logger, 1)
	}

	established := make(chan struct{})
	link.SetLinkEstablishedCallback(func(*rns.Link) {
		close(established)
	})

	start := time.Now()
	fmt.Printf("Establishing link to %s (timeout %.0fs)…\n", destHashStr, *timeoutSec)
	if err := link.Establish(); err != nil {
		fmt.Printf("Establish error: %v\n", err)
		exit(logger, 1)
	}

	select {
	case <-established:
		elapsed := time.Since(start)
		fmt.Printf("LINK ESTABLISHED in %v\n", elapsed.Round(time.Millisecond))
		exit(logger, 0)
	case <-time.After(time.Duration(*timeoutSec * float64(time.Second))):
		fmt.Println("LINK TIMED OUT")
		exit(logger, 1)
	}
}

// exit flushes the async rns log queue so queued transport diagnostics reach
// the sink, then exits the process.
func exit(logger *rns.Logger, code int) {
	if logger != nil {
		logger.Close()
	}
	os.Exit(code)
}

func instanceRole(r *rns.Reticulum) string {
	switch {
	case r.IsSharedInstance():
		return "shared instance (server)"
	case r.IsConnectedToSharedInstance():
		return "connected to shared instance (client)"
	case r.IsStandaloneInstance():
		return "standalone"
	default:
		return "unknown"
	}
}
