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

package console

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

// socketNamePrefix begins every console socket name.
const socketNamePrefix = "gonomadnet-console-"

// SocketEntropyBytes is how much randomness a console socket name carries.
//
// An abstract Unix socket has no filesystem entry and no permissions: any
// process on the device that knows the name can connect to it, and this socket
// carries the keystrokes of a terminal attached to a Reticulum node. Sixteen
// bytes is far more than enough to make the name unguessable, and the uid check
// in CheckPeerUID is the second lock on the same door.
const SocketEntropyBytes = 16

// ErrPeerUID reports a connection from a process that is not this app.
var ErrPeerUID = errors.New("console: refusing a peer from another uid")

// NewSocketEntropy returns SocketEntropyBytes of cryptographic randomness for a
// console socket name.
func NewSocketEntropy() ([]byte, error) {
	entropy := make([]byte, SocketEntropyBytes)
	if _, err := rand.Read(entropy); err != nil {
		return nil, fmt.Errorf("console: reading entropy for a socket name: %w", err)
	}
	return entropy, nil
}

// ConsoleSocketName returns the abstract-socket name of a console session.
//
// It is a pure function of the app's pid and the session's entropy, so the
// appliance and the host can each derive the same name without exchanging it
// beyond the host's command line. The name carries no NUL and no slash, so it
// is usable as the "@name" form of an abstract socket address.
func ConsoleSocketName(pid int, entropy []byte) string {
	return socketNamePrefix + strconv.Itoa(pid) + "-" + hex.EncodeToString(entropy)
}

// CheckPeerUID reports whether a connecting peer may use the console.
//
// The host runs as the app's own uid, and nothing else on the device has any
// business writing to the appliance's terminal. Any other uid is refused, and
// the refusal names both uids so the log alone is enough to diagnose it.
func CheckPeerUID(ourUID, peerUID int) error {
	if peerUID != ourUID {
		return fmt.Errorf("%w: peer uid %v, ours %v", ErrPeerUID, peerUID, ourUID)
	}
	return nil
}
