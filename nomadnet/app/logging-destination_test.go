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

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

// TestLoggingDestinationFromConfig pins [logging] destination, which Python
// selects as (NomadNetworkApp.py:811-818):
//
//	if value.lower() == "file" and not self.force_console_log:
//	    RNS.logdest = RNS.LOG_FILE      # + [logging] logfile if present
//	else:
//	    RNS.logdest = RNS.LOG_STDOUT
//
// The port chose the log FILE unless -c was given, so `destination = stdout` was
// silently ignored and a user who asked for console logging got a file instead.
func TestLoggingDestinationFromConfig(t *testing.T) {
	// Not parallel — Init() starts real RNS/LXMF subsystems.

	tests := []struct {
		name         string
		config       string
		forceConsole bool
		want         int
		wantLogFile  bool
	}{
		{
			name:   "destination stdout selects stdout",
			config: "[logging]\n  destination = stdout\n\n[node]\n  enable_node = no\n",
			want:   rns.LogStdout,
		},
		{
			name:        "destination file selects the file",
			config:      "[logging]\n  destination = file\n\n[node]\n  enable_node = no\n",
			want:        rns.LogDestFile,
			wantLogFile: true,
		},
		{
			name:         "file with forced console logging selects stdout",
			config:       "[logging]\n  destination = file\n\n[node]\n  enable_node = no\n",
			forceConsole: true,
			want:         rns.LogStdout,
		},
		{
			name:   "any other value selects stdout",
			config: "[logging]\n  destination = syslog\n\n[node]\n  enable_node = no\n",
			want:   rns.LogStdout,
		},
		{
			name:        "absent destination defaults to the file",
			config:      "[node]\n  enable_node = no\n",
			want:        rns.LogDestFile,
			wantLogFile: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tempDir(t)
			if err := os.WriteFile(filepath.Join(dir, "config"), []byte(tc.config), 0o644); err != nil {
				t.Fatal(err)
			}
			rnsDir := writeTestRNSConfig(t)
			a := NewApp(dir, rnsDir, false, tc.forceConsole)

			if err := a.Init(); err != nil {
				t.Fatalf("Init: %v", err)
			}
			defer a.Shutdown()

			if got := a.Logger.GetLogDest(); got != tc.want {
				t.Errorf("GetLogDest = %v, want %v", got, tc.want)
			}

			logPath := a.LogFilePath
			_, statErr := os.Stat(logPath)
			if tc.wantLogFile && statErr != nil {
				t.Errorf("log file %v was not created: %v", logPath, statErr)
			}
			if !tc.wantLogFile && statErr == nil {
				t.Errorf("log file %v was created although the destination is stdout", logPath)
			}
		})
	}
}

// TestLoggingDestinationLogFileOverride pins the companion rule in the same
// block: a [logging] logfile entry names the file, overriding the default
// <configdir>/logfile (NomadNetworkApp.py:813-815 sets RNS.logfile from it).
func TestLoggingDestinationLogFileOverride(t *testing.T) {
	dir := tempDir(t)
	custom := filepath.Join(dir, "custom.log")
	contents := "[logging]\n  destination = file\n  logfile = " + custom + "\n\n[node]\n  enable_node = no\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	rnsDir := writeTestRNSConfig(t)
	a := NewApp(dir, rnsDir, false, false)

	if err := a.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer a.Shutdown()

	if a.LogFilePath != custom {
		t.Errorf("LogFilePath = %v, want the configured %v", a.LogFilePath, custom)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("configured log file %v was not created: %v", custom, err)
	}
}
