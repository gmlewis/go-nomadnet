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

// Command aui-probe exercises the AutoInterface LAN peer-discovery path
// directly, so "nodes do not discover each other" can be split into software
// faults versus network faults (WiFi access points very commonly filter
// IPv6 multicast between wireless stations).
//
// It reproduces the exact wire protocol of go-reticulum's AutoInterface
// (rns/interfaces/auto.go): the discovery multicast group is derived from the
// shared group ID as
//
//	ff<type><scope>:0000:<w1>:<w2>:<w3>:<w4>:<w5>:<w6>
//
// where type=1 (temporary) and scope=2 (link) by default give the ff12::
// prefix, and w1..w6 are the big-endian 16-bit words of
// sha256(groupID)[2:14]. A discovery datagram is exactly
// sha256(groupID + source-IP-string), and receivers accept it only when that
// checksum matches their own recomputation.
//
// Usage:
//
//	aui-probe group [-group ID]                       print the derived group address
//	aui-probe send  [-iface NAME] [-count N] ...      inject discovery tokens
//	aui-probe listen [-iface NAME] [-timeout D] ...   receive and verify tokens
//
// Typical pairwise reachability test on two boxes A and B sharing one LAN:
//
//	B$ aui-probe listen -iface wlan0 -timeout 30s
//	A$ aui-probe send   -iface wlan0
//
// If B prints verified tokens from A's link-local address, IPv6 multicast
// crosses the network and discovery failures are a software problem. If B
// stays silent while A reports successful sends, the network is dropping
// IPv6 multicast (check AP snooping settings) — no amount of code changes on
// the nodes will fix that.
//
// Flags common to send/listen:
//
//	-group ID    AutoInterface group ID (default "reticulum")
//	-iface NAME  local interface to scope multicast by (default en0/wlan-ish)
//	-port N      discovery UDP port (default 29716)
//
// Additional flags:
//
//	send:   -count N   datagrams to send (default 3)
//	        -addr IP   send unicast to IP instead of the multicast group;
//	                   overrides -iface scoping when combined with -zone
//	        -zone IDX  scope index for -addr (usually the ifindex of NAME)
//	        -src IP    source identity claimed by the token (default: this
//	                   interface's first IPv6 link-local address). Receivers
//	                   verify sha256(group+src), so it must equal what the
//	                   receiver observes as the datagram's source address.
//	listen: -timeout D how long to listen (default 15s, e.g. 30s)
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const (
	defaultGroupID       = "reticulum"
	defaultDiscoveryPort = 29716
	multicastType        = "1" // temporary
	multicastScope       = "2" // link-local
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
	}
	mode := os.Args[1]

	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	group := fs.String("group", defaultGroupID, "AutoInterface group ID")
	ifaceName := fs.String("iface", "", "interface to scope/link by")
	port := fs.Int("port", defaultDiscoveryPort, "discovery UDP port")

	switch mode {
	case "group":
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s (group %q)\n", deriveMulticastAddress(*group), *group)

	case "send":
		count := fs.Int("count", 3, "datagrams to send")
		src := fs.String("src", "", "source IP string claimed by the token (default: iface LL)")
		addr := fs.String("addr", "", "optional unicast destination instead of multicast")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		sendTokens(*group, *ifaceName, *port, *count, *src, *addr)

	case "listen":
		timeout := fs.Duration("timeout", 15*time.Second, "how long to listen")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		if err := listenTokens(*group, *ifaceName, *port, *timeout); err != nil {
			log.Fatalf("aui-probe: %v", err)
		}

	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: aui-probe <group|send|listen> [flags]")
	fmt.Fprintln(os.Stderr, "run 'aui-probe' with no mode for full help")
	flag.PrintDefaults()
	os.Exit(2)
}

// deriveMulticastAddress mirrors rns/interfaces/auto.go
// autoMulticastDiscoveryIP for the default temporary/link multicast type and
// scope: ff12:0:w1:w2:w3:w4:w5:w6 from big-endian 16-bit words of
// sha256(groupID) bytes 2..13.
func deriveMulticastAddress(groupID string) string {
	h := sha256.Sum256([]byte(groupID))
	segs := []string{"0"}
	for i := 2; i+1 <= 13; i += 2 {
		segs = append(segs, fmt.Sprintf("%02x", int(h[i])<<8|int(h[i+1])))
	}
	return "ff" + multicastType + multicastScope + ":" + strings.Join(segs, ":")
}

// discoveryToken is the payload AutoInterface peers exchange: the SHA-256 of
// the group ID concatenated with the sender's observed source IP string.
func discoveryToken(groupID, srcIP string) []byte {
	sum := sha256.Sum256(append([]byte(groupID), []byte(srcIP)...))
	return sum[:]
}

func firstLinkLocal(iface *net.Interface) string {
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip := ipnet.IP; ip.To16() != nil && ip.To4() == nil && ip.IsLinkLocalUnicast() {
			return ip.String()
		}
	}
	return ""
}

func sendTokens(groupID, ifaceName string, port, count int, src, addr string) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		log.Fatalf("aui-probe: -iface %v: %v", ifaceName, err)
	}
	if src == "" {
		if src = firstLinkLocal(iface); src == "" {
			log.Fatalf("aui-probe: interface %v has no IPv6 link-local address; pass -src", ifaceName)
		}
	}
	token := discoveryToken(groupID, src)

	var dst *net.UDPAddr
	if addr != "" {
		dst = &net.UDPAddr{IP: net.ParseIP(addr), Port: port, Zone: iface.Name}
	} else {
		dst = &net.UDPAddr{IP: net.ParseIP(deriveMulticastAddress(groupID)), Port: port, Zone: iface.Name}
	}

	conn, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6unspecified})
	if err != nil {
		log.Fatalf("aui-probe: %v", err)
	}
	defer func() { _ = conn.Close() }()

	for i := range count {
		if _, err := conn.WriteToUDP(token, dst); err != nil {
			log.Fatalf("aui-probe: send %d/%d to %v: %v", i+1, count, dst, err)
		}
	}
	fmt.Printf("sent %d token(s) claiming src %v to %v:%d on %v\n", count, src, dst.IP, port, iface.Name)
}

func listenTokens(groupID, ifaceName string, port int, timeout time.Duration) error {
	var iface *net.Interface
	if ifaceName != "" {
		var err error
		if iface, err = net.InterfaceByName(ifaceName); err != nil {
			return fmt.Errorf("-iface %v: %w", ifaceName, err)
		}
	} else {
		var err error
		if iface, err = firstMulticastInterface(); err != nil {
			return err
		}
	}
	groupAddr := &net.UDPAddr{IP: net.ParseIP(deriveMulticastAddress(groupID)), Port: port}
	conn, err := net.ListenMulticastUDP("udp6", iface, groupAddr)
	if err != nil {
		return fmt.Errorf("join %v on %v: %w", groupAddr.IP, iface.Name, err)
	}
	defer func() { _ = conn.Close() }()
	fmt.Printf("listening on %v (%v) for %v; expect tokens claiming sha256(%q + src)\n",
		groupAddr.IP, iface.Name, timeout, groupID)

	buf := make([]byte, 2048)
	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)
	for time.Now().Before(deadline) {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return fmt.Errorf("read: %w", err)
		}
		if n < sha256.Size {
			fmt.Printf("<- %d byte(s) from %v (short)\n", n, from)
			continue
		}
		want := hex.EncodeToString(discoveryToken(groupID, from.IP.String()))
		got := hex.EncodeToString(buf[:sha256.Size])
		verdict := "MISMATCH"
		if want == got {
			verdict = "verified"
		}
		fmt.Printf("<- token from %-42v %s\n", from, verdict)
	}
	return nil
}

// firstMulticastInterface picks a default interface when -iface is omitted:
// the first non-loopback, multicast-capable interface whose name is not one
// of the virtual/pseudo kinds that cannot join IPv6 multicast groups
// (macOS anpi*/bridge*/awdl*/llw*/utun*, container bridges, and so on).
func firstMulticastInterface() (*net.Interface, error) {
	skip := []string{"anpi", "bridge", "awdl", "llw", "utun", "docker", "virbr", "veth"}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagMulticast == 0 ||
			ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		virtual := false
		for _, p := range skip {
			if strings.HasPrefix(ifc.Name, p) {
				virtual = true
				break
			}
		}
		if !virtual {
			return &ifc, nil
		}
	}
	return nil, fmt.Errorf("no multicast-capable interface found; pass -iface")
}
