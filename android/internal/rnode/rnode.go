// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package rnode gives the appliance's transport a serial path to a USB RNode
// radio, on a platform that hands an application no serial port at all.
//
// Reticulum's RNodeInterface opens a path — a device node such as
// /dev/ttyACM0 — and speaks the radio's own framed protocol over it. Android
// has no udev and no /dev/serial/by-id, and the kernel creates the CDC-ACM
// node as crw------- root root, which no application may open however it is
// configured. An application reaches a USB device through the host API
// instead, and that hands it a usbfs descriptor — a thing no path can name.
//
// Neither side can do it alone. This package is the half that can allocate
// what Android denies an application: a pseudo-terminal. Open allocates a pty
// pair, holds the slave open so the pair outlives the moment it is created,
// and puts the pair in raw mode so that the radio's framed binary protocol is
// carried byte for byte. The path to that slave is what the transport is
// configured with, and it is published to a file the appliance reads.
//
// The other half is the appliance, which owns the USB device. It dials the
// socket Listen binds (see serve.go) and carries the radio's bytes between
// that socket and the USB endpoints. The appliance is the only thing that
// runs this, and everything it needs is on the command line.
//
// This package only ever runs on Android, and only the radio bridge imports it.
// It lives under android/internal rather than beside the client so that nothing
// in nomadnet/ reads as something a desktop could reuse.
package rnode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gmlewis/go-nomadnet/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// Bridge is a pseudo-terminal pair standing in for a serial port.
//
// Both ends are held for the life of the bridge, and that is deliberate: a pty
// whose slave has no open descriptor reports its master as at end of file, so
// a bridge that let go of the slave would end the instant it was opened, and
// the transport — which opens the published path when it starts, afterwards —
// would find a path that no longer names anything.
type Bridge struct {
	master *os.File
	slave  *os.File
	path   string
}

// Open allocates the pseudo-terminal pair a radio's serial port is built on, retrying an
// allocation that lost a race.
//
// A radio the appliance cannot give a serial path is a radio the appliance does not have, and
// the bridge reports one by exiting: this is the only step that can fail before it has a path
// to publish, and nothing restarts it, so a transient failure here costs the radio until the
// next attach. See retryingOpen for the race being retried and why it is worth waiting out.
func Open() (*Bridge, error) {
	return retryingOpen(openOnce, openAttempts, time.Sleep)
}

// openAttempts bounds how many times Open tries to allocate a pty. openRetryBase and
// openRetryCap bound how long it waits between tries: the wait doubles from the base up to
// the cap, so a race that clears at once costs a millisecond and one that takes longer still
// resolves inside tens of them — far less than the restart a supervising layer would need, and
// below anything a person holding the tablet would notice.
const (
	openAttempts  = 10
	openRetryBase = time.Millisecond
	openRetryCap  = 8 * time.Millisecond
)

// retryingOpen runs open until it succeeds, until it fails for a reason that retrying cannot
// mend, or until attempts run out — in which case it returns the last error.
//
// A pty's /dev/pts/N node is created when its master is opened and removed when the master is
// closed, and the kernel hands back the lowest free N. Two allocations that overlap can land
// on the same N: the open reads the number, the other end's teardown removes the node, and
// opening the slave then fails with ENOENT even though the master was allocated. The pty
// package asks the kernel for the master's own peer instead of resolving that name, which
// cannot lose this race, so what the retry is left with is a kernel older than 4.13 — where
// TIOCGPTPEER does not exist and the name is the only way to the slave — and the rest of this
// allocation: the raw mode, the pollable duplicate, and the published path. The appliance
// has two programs that allocate a pty and can start together — the console that gives the
// client a terminal, and this bridge — and the device's other ptys churn as well. GitHub's
// Ubuntu runner loses the same race under a test suite that allocates eight ptys in parallel
// (run 38166201668, TestPumpEndsWhenEitherEndCloses: "open /dev/pts/0: no such file or
// directory"), while the same bytes pass on the next run. The node is there again a moment
// later, so a bounded retry is the whole fix: what is waited out is a teardown still in
// flight, not a pty that is not there.
//
// Only a "not there" error is retried. Every other failure comes back at once, and so does the
// last not-there error once the attempts are spent, so a genuine failure to allocate a pty is
// reported to the caller rather than waited away.
//
// open and sleep are parameters rather than fixed calls so the retrying can be tested without
// a pty and without waiting: an error is the only thing that decides a retry, and a test can
// supply that directly.
func retryingOpen(open func() (*Bridge, error), attempts int, sleep func(time.Duration)) (*Bridge, error) {
	var err error
	for attempt := range attempts {
		var bridge *Bridge
		if bridge, err = open(); err == nil {
			return bridge, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		// A wait precedes a retry: after the last try there is nothing left to wait for.
		if attempt < attempts-1 {
			sleep(openRetryWait(attempt))
		}
	}
	return nil, err
}

// openRetryWait is how long retryingOpen waits before the retry that follows attempt, counting
// the first retry as zero: the base wait doubled once per attempt, and never past the cap.
func openRetryWait(attempt int) time.Duration {
	// A shift this large is past the cap whatever the base, and refusing it here keeps a
	// raised attempt bound from turning into an overflow.
	if attempt > 16 {
		return openRetryCap
	}
	if wait := openRetryBase << attempt; wait < openRetryCap {
		return wait
	}
	return openRetryCap
}

// openOnce allocates the pseudo-terminal pair a radio's serial port is built on.
//
// The pair is put into raw mode before it is handed over, because a pty in its
// default state is a terminal and not a cable: the line discipline turns a
// carriage return into a newline on the way in and a newline into
// carriage-return newline on the way out, takes 0x03 as an interrupt, and stops
// the output at a 0x13. An RNode's protocol is HDLC-framed binary, which those
// rules would corrupt in ways that look like a radio that hears nothing.
func openOnce() (*Bridge, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, fmt.Errorf("rnode: opening a pseudo-terminal: %w", err)
	}
	path := slave.Name()
	if path == "" {
		_ = master.Close()
		_ = slave.Close()
		return nil, fmt.Errorf("rnode: the pseudo-terminal has no slave path")
	}
	if _, err := term.MakeRaw(int(slave.Fd())); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return nil, fmt.Errorf("rnode: putting the pseudo-terminal into raw mode: %w", err)
	}

	// The descriptor a pty is allocated on is in blocking mode, and Go cannot
	// interrupt a read that is already blocked in one: closing the file does not
	// release the reader, so the goroutine stays in the kernel for the life of
	// the process. A bridge has to be able to put the radio down the moment the
	// appliance goes away, so the end this package reads is a duplicate of that
	// descriptor in non-blocking mode, which is what makes Go register it with
	// its own poller — and what makes Close release a read that is in flight.
	// The allocating descriptor is then released: the duplicate keeps the
	// pseudo-terminal alive, and one handle wants one owner.
	pollable, err := pollableDescriptor(master)
	_ = master.Close()
	if err != nil {
		_ = slave.Close()
		return nil, err
	}
	return &Bridge{master: pollable, slave: slave, path: path}, nil
}

// pollableDescriptor returns a duplicate of f's descriptor that Go's runtime
// poller manages.
func pollableDescriptor(f *os.File) (*os.File, error) {
	fd, err := unix.Dup(int(f.Fd()))
	if err != nil {
		return nil, fmt.Errorf("rnode: duplicating the pseudo-terminal's descriptor: %w", err)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("rnode: making the pseudo-terminal's descriptor non-blocking: %w", err)
	}
	return os.NewFile(uintptr(fd), f.Name()), nil
}

// SlavePath is the serial path the transport is configured with.
func (b *Bridge) SlavePath() string { return b.path }

// Master is the end the appliance's bytes are carried to and from.
//
// It is a duplicate of the descriptor the pair was allocated on, in
// non-blocking mode; see Open.
func (b *Bridge) Master() *os.File { return b.master }

// Close releases the pseudo-terminal pair.
//
// Closing twice is not an error, because the pump closes both ends when either
// of them ends and the caller closes the bridge when it is done with it.
func (b *Bridge) Close() error {
	if b.master != nil {
		_ = b.master.Close()
	}
	if b.slave != nil {
		_ = b.slave.Close()
	}
	return nil
}

// PublishPath writes the slave path where the appliance reads it.
//
// The file is written beside itself and renamed into place, because the
// appliance watches for it and configures the transport with whatever it finds
// there: a partial read would be a transport pointed at a path that is not one.
func PublishPath(path, slavePath string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("rnode: creating %v: %w", dir, err)
		}
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(slavePath), 0o644); err != nil {
		return fmt.Errorf("rnode: writing %v: %w", temporary, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("rnode: publishing %v: %w", path, err)
	}
	return nil
}

// UnpublishPath removes a published slave path.
//
// A path that is not there is not a failure: the bridge removes its own on the
// way out whatever happened to it, and a second removal is what a restart does
// before the first bridge has finished exiting.
func UnpublishPath(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rnode: removing %v: %w", path, err)
	}
	return nil
}
