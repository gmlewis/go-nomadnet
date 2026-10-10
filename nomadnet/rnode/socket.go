// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package rnode

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/console"
)

// SocketNamePrefix begins every radio bridge socket name.
//
// The appliance builds the same name in Kotlin from the same two inputs, and
// the two are asserted against each other on both sides, because a name the app
// and the bridge disagree about is an app dialing a socket nobody is listening
// on and a radio that shows as connected and carries nothing.
const SocketNamePrefix = "gonomadnet-rnode-"

// NewSocketEntropy returns the randomness a socket name carries.
//
// An abstract Unix socket has no filesystem entry and no permissions: any
// process on the device that knows the name can connect to it, and what can be
// injected through this one is written straight into a radio's serial stream.
// The console bridge answers the same question in the same way — see
// console.NewSocketEntropy — and this is that convention rather than a second
// one.
func NewSocketEntropy() ([]byte, error) { return console.NewSocketEntropy() }

// SocketName returns the socket name for the appliance's pid and its entropy.
//
// It is a pure function of the two, so the appliance and the bridge can each
// derive the same name without exchanging it beyond the bridge's command line.
// See console.ConsoleSocketName, whose shape this follows exactly.
func SocketName(pid int, entropy []byte) string {
	return SocketNamePrefix + strconv.Itoa(pid) + "-" + hex.EncodeToString(entropy)
}

// Listen binds the socket the appliance dials.
//
// A name beginning with a slash is bound as a path, which is how the two halves
// are exercised on a development machine, and a bare name is bound abstract,
// which is what the appliance uses: an abstract socket has no filesystem entry,
// so there is no directory an application would need permission to put a socket
// in — and on Android there is none it could use.
func Listen(name string) (net.Listener, error) {
	if name == "" {
		return nil, fmt.Errorf("rnode: a bridge socket needs a name")
	}
	address := name
	if !strings.HasPrefix(name, "/") {
		address = "@" + name
	}
	ln, err := net.Listen("unix", address)
	if err != nil {
		return nil, fmt.Errorf("rnode: listening on %v: %w", name, err)
	}
	return ln, nil
}
