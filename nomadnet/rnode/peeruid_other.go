// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !linux

package rnode

import (
	"fmt"
	"net"
	"runtime"
)

// platformPeerUID refuses to identify a peer on a platform that cannot say who
// it is.
//
// The radio's socket is a write path into a transmitter, so a peer that cannot
// be shown to be this application is not served: a development machine is not
// the appliance, and answering "allow" where the appliance answers with the
// kernel's record of who connected is the one behaviour that must not differ
// between the two. The tests supply their own reader.
func platformPeerUID(net.Conn) (int, error) {
	return 0, fmt.Errorf("%v cannot report a Unix connection's peer uid", runtime.GOOS)
}
