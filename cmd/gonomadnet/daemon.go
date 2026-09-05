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
	"os/signal"
	"syscall"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
)

// runDaemon starts NomadNet in daemon mode (no UI).
// In daemon mode, the LXMF router and node run without a terminal UI.
func runDaemon(configDir, rnsConfigDir string, console bool) {
	log.Printf("Nomad Network daemon starting...")

	a := app.NewApp(configDir, rnsConfigDir, true, console)
	if err := a.Init(); err != nil {
		// The rns logger writes asynchronously; flush the queue so the init
		// diagnostics explaining the failure are not silently lost.
		a.Logger.Flush()
		log.Fatalf("Failed to initialize: %v", err)
	}

	log.Printf("Daemon mode active")
	log.Printf("Config: %v", a.ConfigPath)
	log.Printf("Storage: %v", a.StoragePath)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutting down...")
	a.Shutdown()
	log.Printf("Daemon stopped")
}
