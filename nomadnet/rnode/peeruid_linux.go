// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build linux

package rnode

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// platformPeerUID reads the uid at the other end of a Unix connection.
//
// This is the call the appliance uses: every peer of this socket is a process
// on the same device, so the kernel's own record of who connected is the whole
// of the identification that is needed, and it cannot be forged by the peer.
func platformPeerUID(conn net.Conn) (int, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("a %T is not a Unix connection", conn)
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("reaching the connection's file descriptor: %w", err)
	}

	var (
		credentials *unix.Ucred
		controlErr  error
	)
	if err := raw.Control(func(fd uintptr) {
		credentials, controlErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, fmt.Errorf("reading the peer's credentials: %w", err)
	}
	if controlErr != nil {
		return 0, fmt.Errorf("reading the peer's credentials: %w", controlErr)
	}
	return int(credentials.Uid), nil
}
