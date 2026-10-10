// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package rnode

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/gmlewis/go-nomadnet/nomadnet/console"
)

// Server carries a radio's bytes between a bridge's pty master and the one
// appliance that connects to it.
//
// A peer is served only if it can be shown to be this application — see
// console.CheckPeerUID, the same rule the console bridge keeps — because what
// this socket reaches is a radio's serial stream, and anything that can write
// there can transmit. A peer that cannot be identified is refused rather than
// trusted, so a platform with no way to read a peer's credentials serves nobody
// at all.
type Server struct {
	// Listener is the bound socket the appliance dials.
	Listener net.Listener

	// Bridge is the pty pair whose slave the transport opens.
	Bridge *Bridge

	// OurUID is the uid a peer has to share to be served.
	OurUID int

	// PeerUID reads a connection's peer uid. A nil value reads it with the
	// platform's own call, which is the only way to do it in production and is
	// not something a test can arrange, so a test supplies its own.
	PeerUID func(net.Conn) (int, error)

	// Logf reports a refused or an ended connection. A nil value logs to the
	// standard logger.
	Logf func(format string, args ...any)
}

// Serve accepts the appliance, carries the radio's bytes until either end is
// closed, and returns.
//
// A refused peer does not end the server: a stranger that found the name must
// not be able to take the radio away from the app by being refused, so the
// server keeps listening for the connection it will serve. Cancelling ctx
// releases the listener, the bridge, and any transfer in flight.
func (s *Server) Serve(ctx context.Context) error {
	peerUID := s.PeerUID
	if peerUID == nil {
		peerUID = platformPeerUID
	}
	logf := s.Logf
	if logf == nil {
		logf = log.Printf
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Listener.Close()
			_ = s.Bridge.Close()
		case <-done:
		}
	}()

	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("rnode: accepting a connection: %w", err)
		}

		uid, err := peerUID(conn)
		if err != nil {
			logf("rnode: refusing a connection to the radio: the peer's uid could not be read: %v", err)
			_ = conn.Close()
			continue
		}
		if err := console.CheckPeerUID(s.OurUID, uid); err != nil {
			logf("rnode: refusing a connection to the radio: %v", err)
			_ = conn.Close()
			continue
		}

		return Pump(s.Bridge.Master(), conn)
	}
}
