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

//go:build integration

package rrc

// Cross-implementation Channels integration tests: a PYTHON mini-hub
// (testdata/mini_hub.py, run via the parity interpreter against the installed
// nomadnet's RRC module) hosts an rrc.hub destination over a loopback TCP
// interface, and the GO client connects and runs the protocol. The Python hub
// logs every event as JSON lines; the test asserts on both sides of the
// exchange - the Go client must reach Connected+Welcomed from a
// Python-encoded WELCOME, and a message sent from Go must arrive on the
// Python side.

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	nnutils "github.com/gmlewis/go-nomadnet/testutils"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

type miniHubEvent map[string]any

// startPythonMiniHub spawns the Python mini-hub on a free port and waits for
// its event log to carry the hub hash. Returns the hub hash, the log path,
// and a cleanup func.
func startPythonMiniHub(t *testing.T, port int) (string, string, func()) {
	t.Helper()
	if nnutils.PythonNomadnetExe() == "" {
		t.Skip("no python nomadnet")
	}

	dir := testutils.TempDir(t, "nomadnet-rrc-cross")
	logPath := filepath.Join(dir, "events.jsonl")

	script := filepath.Join("testdata", "mini_hub.py")
	if _, err := os.Stat(script); err != nil {
		script = filepath.Join("..", "nomadnet", "rrc", "testdata", "mini_hub.py")
		if _, err2 := os.Stat(script); err2 != nil {
			t.Skipf("mini_hub.py not accessible: %v", err2)
		}
	}
	interp := nnutils.PythonNomadnetExe()
	cmd := exec.Command(interp, script, "--port", fmt.Sprint(port),
		"--log", logPath, "--name", "MiniHub")
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python mini-hub: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	// Poll the log for the hub address (the announce takes a moment).
	var hubHash string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(logPath)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				var ev map[string]any
				if json.Unmarshal([]byte(line), &ev) == nil && ev["event"] == "hub" {
					if s, ok := ev["hash"].(string); ok {
						hubHash = s
					}
				}
			}
		}
		if hubHash != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	// killAndWait terminates the mini hub AND reaps it (the detached Wait
	// goroutine closes done); Kill alone leaked a zombie under parallel load.
	killAndWait := func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	if hubHash == "" {
		killAndWait()
		t.Fatal("python mini-hub never announced its address")
	}

	return hubHash, logPath, killAndWait
}

func freePortRRC(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func readMiniHubEvents(t *testing.T, logPath string) []miniHubEvent {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	var out []miniHubEvent
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev miniHubEvent
		if json.Unmarshal([]byte(line), &ev) == nil {
			out = append(out, ev)
		}
	}
	return out
}

// TestIntegrationCrossImplPythonHub runs the full Channels lifecycle against
// a PYTHON-hosted hub: connect, the hello/welcome handshake with a
// Python-encoded WELCOME, and a message echo round-trip.
func TestIntegrationCrossImplPythonHub(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)
	port := freePortRRC(t)
	hubHash, hubLog, hubCleanup := startPythonMiniHub(t, port)
	defer hubCleanup()

	// The Go client's RNS config: a TCPClientInterface to the Python hub.
	dir, err := os.MkdirTemp("/tmp", "nomadnet-rrc-cross-go")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	cfgDir := filepath.Join(dir, "config")
	writeRNSConfigRRC(t, cfgDir)
	appendTCPClientInterface(t, filepath.Join(cfgDir, "config"), port)

	ts := rns.NewTransportSystem(nil)
	if _, err := rns.NewReticulum(ts, cfgDir); err != nil {
		t.Fatalf("NewReticulum: %v", err)
	}

	mgr := NewManager(filepath.Join(dir, "storage"), func() []byte { return ts.Identity().Hash })
	mgr.SetNickname("GoClient")
	mgr.SetTransport(ts)
	hubHashBytes, err := hex.DecodeString(hubHash)
	if err != nil {
		t.Fatalf("hub hash: %v", err)
	}
	hub := mgr.AddHub(hubHashBytes, "rrc.hub", "RNS Community")
	hub.ConnectAsync()

	// Wait for Connected (the link) AND Welcomed (the Python WELCOME decoded
	// and applied - the cross-implementation handshake).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		status, welcomed := hub.Status, hub.Welcomed
		hub.lock.Unlock()
		if welcomed && status == StatusConnected {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	hub.lock.Lock()
	status, welcomed, hubName, statusText := hub.Status, hub.Welcomed, hub.HubName, hub.StatusText
	hub.lock.Unlock()
	if status != StatusConnected || !welcomed {
		t.Fatalf("hub not welcomed: status=%d text=%q welcomed=%v (expected the Python WELCOME to arrive)", status, statusText, welcomed)
	}
	if hubName == "" {
		t.Error("hub name not populated from the Python WELCOME")
	}
	t.Logf("CONNECTED: hub name=%q status=%d", hubName, status)

	// The message echo round-trip: send to a room, expect the Python hub's
	// NOTICE echo back through HandleData.
	echoCh := make(chan *RRCMessage, 1)
	hub.SendMessage("general", "cross-impl test message")
	deadlineEcho := time.Now().Add(15 * time.Second)
	var echoMsg *RRCMessage
	for time.Now().Before(deadlineEcho) {
		hub.lock.Lock()
		for _, msgs := range hub.Messages {
			for _, msg := range msgs {
				if strings.HasPrefix(msg.Text, "echo: ") {
					echoMsg = msg
				}
			}
		}
		hub.lock.Unlock()
		if echoMsg != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if echoMsg == nil {
		t.Error("no echoed NOTICE received from the Python hub (message delivery failed)")
	} else {
		t.Logf("echo round-trip OK: %q", echoMsg.Text)
	}

	// Assert on the PYTHON side's view: the hub must have received the
	// client's hello with TEXT-string body values and the message.
	events := readMiniHubEvents(t, hubLog)
	var sawHello, sawMsg bool
	for _, ev := range events {
		if ev["event"] == "hello" {
			sawHello = true
			if nameType, _ := ev["name-type"].(string); nameType != "str" {
				t.Errorf("python hub saw hello name as %v; want str (text string)", nameType)
			}
		}
		if ev["event"] == "msg" && strings.Contains(fmt.Sprint(ev["text"]), "cross-impl") {
			sawMsg = true
		}
	}
	if !sawHello {
		t.Error("python hub never received the client hello")
	}
	if !sawMsg {
		t.Error("python hub never received the client message")
	}
	_ = echoCh
}

// appendTCPClientInterface adds a [[TCP Client]] section to an RNS config so
// the transport connects to the given host:port at NewReticulum time.
func appendTCPClientInterface(t *testing.T, configPath string, port int) {
	t.Helper()
	content := `[reticulum]
share_instance = No
enable_transport = No

[logging]
loglevel = 4

[interfaces]
  [[Cross-Impl Hub]]
    type = TCPServerInterface
    enabled = No

  [[TCP Client]]
    type = TCPClientInterface
    enabled = yes
    target_host = 127.0.0.1
    target_port = ` + fmt.Sprint(port) + `
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
