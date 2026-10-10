// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package rnode

import (
	"errors"
	"io"
	"net"
	"os"
)

// chunk is the most that is carried in one direction at a time.
const chunk = 32 * 1024

// Pump carries bytes in both directions between the pty's master and the
// appliance's connection, until either end is closed.
//
// Both ends are closed before it returns, whatever ended the transfer: the
// other direction's goroutine is blocked in a read that only that close will
// release, and a bridge that outlived its radio would hold the pty for the life
// of the install. A connection that ended is a normal end and not an error —
// the appliance closing the socket is exactly how a radio is put down.
func Pump(master, app io.ReadWriteCloser) error {
	ended := make(chan error, 2)
	go func() { ended <- carry(master, app) }()
	go func() { ended <- carry(app, master) }()

	first := <-ended
	_ = master.Close()
	_ = app.Close()
	<-ended
	return cleanEnd(first)
}

// carry copies src to dst until src ends.
func carry(dst io.Writer, src io.Reader) error {
	buf := make([]byte, chunk)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// cleanEnd turns the reason a transfer ended into the error Pump reports: a
// closed end is how every one of them ends, and a lost stream is not.
func cleanEnd(err error) error {
	if err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}
