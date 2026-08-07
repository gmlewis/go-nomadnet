// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// config.go writes the private NomadNet + RNS configs for the two headless
// gonomadnet instances driven by the conversations harness. Each instance gets
// its own -config dir (isolates storage/identity, conversations, directory,
// attachments) and its own -rnsconfig dir (isolates the RNS identity), per
// nomadnet/app/app.go:setupPaths and RNS reading <rnsdir>/config (go-reticulum
// rns/rns.go:243).

// freePort returns a free localhost TCP port by opening a listener on :0 and
// closing it. Used for A's TCPServerInterface listen_port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("free port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return 0, fmt.Errorf("close free port listener: %w", err)
	}
	return port, nil
}

// writeNomadNetConfig writes a minimal private NomadNet config to <dir>/config
// with node hosting disabled, so the instance starts with no hosted node and
// no spurious announce-at-start. The Conversations/LXMF-router functionality
// does not require node hosting. Mirrors nomadnet/app/testconfig_test.go.
func writeNomadNetConfig(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	const contents = "[node]\nenable_node = no\n"
	return os.WriteFile(filepath.Join(dir, "config"), []byte(contents), 0o644)
}

// writeRNSConfigServer writes an RNS config to <dir>/config that hosts a TCP
// server interface on 127.0.0.1:<port>. share_instance=No so this Reticulum is
// standalone. loglevel=4 (warning+). Format matches go-reticulum rns/rns.go
// initInterfaces (the [interfaces] section + [[...]] subsections with a "type"
// property) and cmd/gornsd's shipped config.
func writeRNSConfigServer(dir string, port int) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(`[reticulum]
  share_instance = No

[logging]
  loglevel = 4

[interfaces]

  [[TCP Server Interface]]
    type = TCPServerInterface
    enabled = yes
    listen_ip = 127.0.0.1
    listen_port = %d
`, port)
	return os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o600)
}

// writeRNSConfigClient writes an RNS config to <dir>/config that connects a
// TCP client interface to 127.0.0.1:<port> (A's server).
func writeRNSConfigClient(dir string, port int) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(`[reticulum]
  share_instance = No

[logging]
  loglevel = 4

[interfaces]

  [[TCP Client Interface]]
    type = TCPClientInterface
    enabled = yes
    target_host = 127.0.0.1
    target_port = %d
`, port)
	return os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o600)
}
