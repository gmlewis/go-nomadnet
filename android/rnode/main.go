// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Command gornnode gives the appliance's Reticulum transport a serial path to a
// USB RNode radio.
//
// Reticulum's RNodeInterface opens a path such as /dev/ttyACM0, and Android
// gives an application no serial port at all: the kernel's node is root-only,
// and the supported way to reach a USB device hands back a usbfs descriptor
// that no path can name. So this program allocates the one thing Android does
// let an application have — a pseudo-terminal — publishes the path to its
// slave as the radio's port, and carries the radio's bytes between that pty and
// the appliance, which owns the USB device and dials the socket bound here.
//
// Everything it needs is on its command line, because the appliance is the only
// thing that runs it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gmlewis/go-nomadnet/nomadnet/rnode"
)

// usage is the documented form, which is the whole of this program's interface.
const usage = `gornnode gives the appliance's transport a serial path to a USB RNode radio.

Usage:
  gornnode --socket NAME --tty PATH [--uid N]

Options:
  --socket NAME  the abstract-socket name to bind for the appliance to dial,
                 without the "@" that Go's net package spells it with
  --tty PATH     the file to write the pseudo-terminal's slave path to. The
                 transport is configured with whatever it finds there.
  --uid N        the uid a connecting peer must have (default: this process's)
`

func main() {
	log.SetFlags(0)

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(usage)
			return
		}
		log.Fatalf("gornnode: %v", err)
	}

	// The appliance stops the bridge by stopping the process, so a signal is the
	// ordinary way this ends: it releases the pty and removes the published path
	// on the way out, leaving nothing for the next start to find.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, opts); err != nil {
		log.Fatalf("gornnode: %v", err)
	}
}

// run brings the bridge up and serves the appliance until either end is done.
func run(ctx context.Context, opts options) error {
	bridge, err := rnode.Open()
	if err != nil {
		return err
	}
	defer func() { _ = bridge.Close() }()

	// The socket is bound before the path is published, and the order is
	// load-bearing: the appliance waits for the published path and then dials
	// this socket, so a path that appeared first would be an appliance dialing a
	// socket that does not exist yet and giving the radio up as unreachable.
	//
	// The path is removed on every way out: one left behind names a pty that is
	// gone, and the next start would configure the transport with it.
	ln, err := rnode.Listen(opts.socket)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()

	if err := rnode.PublishPath(opts.tty, bridge.SlavePath()); err != nil {
		return err
	}
	defer func() {
		if err := rnode.UnpublishPath(opts.tty); err != nil {
			log.Printf("gornnode: %v", err)
		}
	}()

	log.Printf("gornnode: the radio's serial path is %v, on socket %v", bridge.SlavePath(), opts.socket)

	srv := &rnode.Server{
		Listener: ln,
		Bridge:   bridge,
		OurUID:   opts.uid,
	}
	if err := srv.Serve(ctx); err != nil {
		return err
	}
	log.Printf("gornnode: the appliance let go of the radio")
	return nil
}

// options is what the appliance asked the bridge to do.
type options struct {
	socket string
	tty    string
	uid    int
}

// parseArgs reads the documented command line, and is a pure function of it so
// that every rule it keeps is a test rather than a comment.
func parseArgs(args []string) (options, error) {
	var opts options

	fs := flag.NewFlagSet("gornnode", flag.ContinueOnError)
	// A command line that does not parse is reported by the caller, which is the
	// only place that knows whether anything is listening to its output.
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.socket, "socket", "", "the abstract-socket name to bind")
	fs.StringVar(&opts.tty, "tty", "", "the file to write the slave path to")
	fs.IntVar(&opts.uid, "uid", -1, "the uid a peer must have")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if rest := fs.Args(); len(rest) > 0 {
		return options{}, fmt.Errorf("gornnode takes no arguments of its own, and %q is not part of the documented form", rest[0])
	}
	if opts.socket == "" {
		return options{}, errors.New("gornnode needs --socket: the name the appliance dials")
	}
	if opts.tty == "" {
		return options{}, errors.New("gornnode needs --tty: the file to publish the radio's serial path to")
	}
	if opts.uid < 0 {
		opts.uid = os.Getuid()
	}
	return opts, nil
}
