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

// Command nomadnet is the Nomad Network client CLI.
//
// It provides a terminal-based interface for messaging, RRC chat,
// node page browsing, and network directory management over the
// Reticulum Network Stack.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gmlewis/go-nomadnet/nomadnet/version"
)

func main() {
	log.SetFlags(0)

	var (
		configDir string
		rnsConfig string
		textUI    bool
		daemon    bool
		console   bool
		showVer   bool
		pprofAddr string
	)

	flag.StringVar(&configDir, "config", "", "path to alternative Nomad Network config directory")
	flag.StringVar(&rnsConfig, "rnsconfig", "", "path to alternative Reticulum config directory")
	flag.BoolVar(&textUI, "textui", false, "run Nomad Network in text-UI mode")
	flag.BoolVar(&textUI, "t", false, "run Nomad Network in text-UI mode")
	flag.BoolVar(&daemon, "daemon", false, "run Nomad Network in daemon mode")
	flag.BoolVar(&daemon, "d", false, "run Nomad Network in daemon mode")
	flag.BoolVar(&console, "console", false, "in daemon mode, log to console instead of file")
	flag.BoolVar(&console, "c", false, "in daemon mode, log to console instead of file")
	flag.BoolVar(&showVer, "version", false, "show version and exit")
	flag.StringVar(&pprofAddr, "pprof-addr", "", "if set, serve net/http/pprof on this address (e.g. 127.0.0.1:6060) for live CPU profiling; zero overhead when unset")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Go Nomad Network Client %v\n\n", version.VERSION)
		fmt.Fprintf(os.Stderr, "Usage: gonomadnet [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if showVer {
		fmt.Printf("Go Nomad Network Client %v\n", version.VERSION)
		os.Exit(0)
	}

	// textui overrides daemon
	if textUI {
		daemon = false
	}

	// Default config directory
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".nomadnetwork")
	}

	// Apply memory tuning (soft heap limit / GC percent) before starting. On
	// small devices (< 4 GiB RAM) this auto-sets a soft memory limit so a
	// long-running node can't balloon and starve the system; on larger machines
	// it is a no-op (Go defaults). Override with GONOMADNET_MEMLIMIT (MiB,
	// 0=disable) and GONOMADNET_GOGC (percent). See memlimit.go.
	applyMemoryTuning()

	// Run the appropriate mode
	if daemon {
		startPProf(pprofAddr)
		runDaemon(configDir, rnsConfig, console)
	} else {
		startPProf(pprofAddr)
		runTextUI(configDir, rnsConfig)
	}
}
