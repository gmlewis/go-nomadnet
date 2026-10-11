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
	"bytes"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConsoleSocketNameIsUnpredictable(t *testing.T) {
	t.Parallel()

	// Two sessions on the same device must never agree on a name: an abstract
	// socket is reachable by any process that knows its name, and this one is a
	// keystroke channel into a Reticulum node.
	first := mustEntropy(t)
	second := mustEntropy(t)
	if bytes.Equal(first, second) {
		t.Fatal("two calls to the entropy source returned the same bytes")
	}
	if got, want := ConsoleSocketName(4242, first), ConsoleSocketName(4242, second); got == want {
		t.Errorf("two names for one pid collided: %v", got)
	}

	// A different pid must not collide either, even with the same entropy.
	if a, b := ConsoleSocketName(1, first), ConsoleSocketName(2, first); a == b {
		t.Errorf("two pids produced the same name: %v", a)
	}
}

func TestNewSocketEntropyCarriesSixteenBytes(t *testing.T) {
	t.Parallel()
	entropy, err := NewSocketEntropy()
	if err != nil {
		t.Fatalf("NewSocketEntropy returned an error: %v", err)
	}
	if len(entropy) != SocketEntropyBytes {
		t.Errorf("NewSocketEntropy returned %v bytes, want %v", len(entropy), SocketEntropyBytes)
	}

	again, err := NewSocketEntropy()
	if err != nil {
		t.Fatalf("the second NewSocketEntropy returned an error: %v", err)
	}
	if bytes.Equal(entropy, again) {
		t.Error("two calls to NewSocketEntropy returned the same bytes")
	}
}

func TestConsoleSocketNameIsASingleAbstractName(t *testing.T) {
	t.Parallel()

	entropy := make([]byte, SocketEntropyBytes)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	name := ConsoleSocketName(os.Getpid(), entropy)

	if strings.ContainsAny(name, "\x00/ \t\n") {
		t.Errorf("the name %q carries a NUL, a slash or whitespace", name)
	}
	if !strings.HasPrefix(name, socketNamePrefix) {
		t.Errorf("the name %q does not start with %q", name, socketNamePrefix)
	}
	if withPID := ConsoleSocketName(4242, entropy); !strings.Contains(withPID, "4242") {
		t.Errorf("the name %q does not carry the pid", withPID)
	}

	// The suffix is the entropy, in hex, and nothing else.
	suffix := name[strings.LastIndex(name, "-")+1:]
	decoded, err := hex.DecodeString(suffix)
	if err != nil {
		t.Fatalf("the name's suffix %q is not hex: %v", suffix, err)
	}
	if !bytes.Equal(decoded, entropy) {
		t.Errorf("the suffix decodes to % x, want % x", decoded, entropy)
	}

	// An abstract socket name lives in sun_path, which is 108 bytes including
	// its leading NUL: a longer name is silently truncated by the kernel, and
	// two sessions could then collide on the truncated form.
	if len(name) >= 108 {
		t.Errorf("the name is %v bytes, too long for sun_path", len(name))
	}

	// And it really is usable: the kernel accepts the name as a socket address.
	// The name carries fresh entropy here, because that is what a session binds;
	// a name it has already used is one the app itself reserved.
	//
	// What a leading "@" means is the platform's business. On Linux it makes the
	// name abstract, and binding it creates nothing on disk. Darwin has no
	// abstract sockets and binds the same name as a path, which would be a socket
	// file in the package's own directory under go test; the socket is therefore
	// bound inside a temporary directory wherever the platform puts it in the
	// file system, so a test run leaves nothing behind in the repository.
	bound := "@" + ConsoleSocketName(os.Getpid(), mustEntropy(t))
	if runtime.GOOS != "linux" {
		bound = filepath.Join(tempDir(t), strings.TrimPrefix(bound, "@"))
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: bound, Net: "unix"})
	if err != nil {
		t.Fatalf("the kernel refused the socket name %q: %v", bound, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("closing the test listener: %v", err)
	}
}

func TestPeerIsAccepted(t *testing.T) {
	t.Parallel()
	// The console host runs as the app's own uid, so its socket connection
	// carries that uid and is the only one allowed.
	for _, uid := range []int{0, 10123, 4294967294} {
		if err := CheckPeerUID(uid, uid); err != nil {
			t.Errorf("CheckPeerUID(%v, %v) = %v, want nil", uid, uid, err)
		}
	}
}

func TestPeerIsRejected(t *testing.T) {
	t.Parallel()
	err := CheckPeerUID(10123, 10124)
	if err == nil {
		t.Fatal("a peer from another uid was accepted")
	}
	if !errors.Is(err, ErrPeerUID) {
		t.Errorf("the refusal is %v, want %v", err, ErrPeerUID)
	}
	// The refusal has to be diagnosable from the log alone: it names the uid
	// that was refused and the one that was expected.
	for _, want := range []string{"10124", "10123"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %v", err, want)
		}
	}
}

// mustEntropy returns entropy, failing the test when it cannot be had.
func mustEntropy(t *testing.T) []byte {
	t.Helper()
	entropy, err := NewSocketEntropy()
	if err != nil {
		t.Fatalf("NewSocketEntropy returned an error: %v", err)
	}
	return entropy
}

// tempDir returns a directory that is removed when the test ends.
//
// On Darwin it lives under /tmp, where the default temporary directory is short
// enough for the sun_path this test is about.
func tempDir(t *testing.T) string {
	t.Helper()
	pattern := ""
	if runtime.GOOS == "darwin" {
		pattern = "/tmp"
	}
	dir, err := os.MkdirTemp(pattern, "gonomadnet-console-")
	if err != nil {
		t.Fatalf("creating a temporary directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
