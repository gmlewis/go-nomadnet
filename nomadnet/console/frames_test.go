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
	"io"
	"math/rand"
	"testing"
)

// goldenFrames is the contract between this package and the appliance's Kotlin
// half, ConsoleFrames.kt. Both test files carry this same table. If the two
// codecs disagree, the appliance attaches to a console that renders nothing and
// logs nothing, so the table is checked byte for byte in both languages.
var goldenFrames = []struct {
	name string
	hex  string
	want Frame
}{
	{"data hello", "010000000568656c6c6f", DataFrame([]byte("hello"))},
	{"data empty", "0100000000", DataFrame([]byte{})},
	{"resize 80x24", "020000000400500018", ResizeFrame(80, 24)},
	{"resize 200x60", "020000000400c8003c", ResizeFrame(200, 60)},
	{"exit 0", "030000000100", ExitFrame(0)},
	{"exit 130", "030000000182", ExitFrame(130)},
}

// framesEqual compares two frames, treating a nil payload and an empty payload
// as the same frame: both mean "a DATA frame carrying nothing".
func framesEqual(a, b Frame) bool {
	return a.Type == b.Type && a.Cols == b.Cols && a.Rows == b.Rows &&
		a.Code == b.Code && bytes.Equal(a.Data, b.Data)
}

func TestFrameGoldenTable(t *testing.T) {
	t.Parallel()
	for _, tc := range goldenFrames {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatalf("the golden table entry is not hex: %v", err)
			}

			if got := AppendFrame(nil, tc.want); !bytes.Equal(got, want) {
				t.Errorf("AppendFrame(%+v) = % x, want % x", tc.want, got, want)
			}

			var buf bytes.Buffer
			if err := WriteFrame(&buf, tc.want); err != nil {
				t.Fatalf("WriteFrame(%+v) returned error: %v", tc.want, err)
			}
			if !bytes.Equal(buf.Bytes(), want) {
				t.Errorf("WriteFrame(%+v) wrote % x, want % x", tc.want, buf.Bytes(), want)
			}

			got, err := ReadFrame(bytes.NewReader(want))
			if err != nil {
				t.Fatalf("ReadFrame(% x) returned error: %v", want, err)
			}
			if !framesEqual(got, tc.want) {
				t.Errorf("ReadFrame(% x) = %+v, want %+v", want, got, tc.want)
			}
		})
	}
}

func TestFrameRejectsOversizeLength(t *testing.T) {
	t.Parallel()
	// A peer that claims 0xFFFFFFFF bytes must not be able to make the reader
	// reserve four gigabytes, so the length is refused before any allocation.
	for _, tc := range []struct {
		name string
		hex  string
	}{
		{"one byte over the maximum", "0100100001"},
		{"the largest possible length", "01ffffffff"},
		{"a resize frame over the maximum", "0200100001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wire, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatalf("the test vector is not hex: %v", err)
			}
			_, err = ReadFrame(bytes.NewReader(wire))
			if !errors.Is(err, ErrOversizeFrame) {
				t.Errorf("ReadFrame(% x) error = %v, want %v", wire, err, ErrOversizeFrame)
			}
		})
	}
}

func TestFrameRejectsUnknownType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		hex  string
	}{
		{"the reserved zero type", "0000000000"},
		{"a type above the highest defined", "0400000000"},
		{"the highest possible type", "ff00000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wire, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatalf("the test vector is not hex: %v", err)
			}
			_, err = ReadFrame(bytes.NewReader(wire))
			if !errors.Is(err, ErrUnknownFrameType) {
				t.Errorf("ReadFrame(% x) error = %v, want %v", wire, err, ErrUnknownFrameType)
			}
		})
	}
}

func TestFrameRejectsTruncatedPayload(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		hex  string
	}{
		{"a payload cut short of its length", "01000000056865"},
		{"a payload that never arrives", "0100000005"},
		{"a header cut short", "010000"},
		{"a single byte", "01"},
		{"a resize frame cut short", "02000000040050"},
		{"an exit frame with no code", "0300000001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wire, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatalf("the test vector is not hex: %v", err)
			}
			_, err = ReadFrame(bytes.NewReader(wire))
			if !errors.Is(err, ErrTruncatedFrame) {
				t.Errorf("ReadFrame(% x) error = %v, want %v", wire, err, ErrTruncatedFrame)
			}
		})
	}
}

func TestFrameEndsCleanlyAtAFrameBoundary(t *testing.T) {
	t.Parallel()
	// Every session finishes this way, so a stream that ends between frames has
	// to be told apart from one that ends inside a frame: the first is a clean
	// shutdown, the second means the bytes seen so far cannot be trusted.
	if _, err := ReadFrame(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Errorf("ReadFrame of an empty stream = %v, want %v", err, io.EOF)
	}

	r := bytes.NewReader(AppendFrame(nil, DataFrame([]byte("hello"))))
	if _, err := ReadFrame(r); err != nil {
		t.Fatalf("reading the only frame: %v", err)
	}
	if _, err := ReadFrame(r); !errors.Is(err, io.EOF) {
		t.Errorf("ReadFrame past the last frame = %v, want %v", err, io.EOF)
	}
}

func TestFrameRejectsMalformedPayloadLength(t *testing.T) {
	t.Parallel()
	// A resize frame carries exactly four bytes and an exit frame exactly one.
	// A frame that carries any other number is a protocol error, not a short
	// read to be acted on.
	for _, tc := range []struct {
		name string
		hex  string
	}{
		{"a resize frame with two bytes", "02000000020050"},
		{"a resize frame with five bytes", "02000000050050001800"},
		{"an exit frame with two bytes", "03000000020000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wire, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatalf("the test vector is not hex: %v", err)
			}
			_, err = ReadFrame(bytes.NewReader(wire))
			if !errors.Is(err, ErrMalformedFrame) {
				t.Errorf("ReadFrame(% x) error = %v, want %v", wire, err, ErrMalformedFrame)
			}
		})
	}
}

func TestFrameRoundTripsRandomPayloads(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test data
	sizes := []int{0, 1, 2, 3, 255, 256, 1024, 4096, MaxPayload}
	for _, size := range sizes {
		payload := make([]byte, size)
		if _, err := rng.Read(payload); err != nil {
			t.Fatalf("filling a %v-byte payload: %v", size, err)
		}

		wire := AppendFrame(nil, DataFrame(payload))
		got, err := ReadFrame(bytes.NewReader(wire))
		if err != nil {
			t.Fatalf("ReadFrame of a %v-byte payload: %v", size, err)
		}
		if got.Type != FrameData {
			t.Errorf("ReadFrame of a %v-byte payload returned type %v, want %v", size, got.Type, FrameData)
		}
		if !bytes.Equal(got.Data, payload) {
			t.Errorf("a %v-byte payload did not survive the round trip", size)
		}
	}

	// Resize and exit frames round trip too, at both ends of their range.
	for _, f := range []Frame{
		ResizeFrame(0, 0),
		ResizeFrame(1, 1),
		ResizeFrame(80, 24),
		ResizeFrame(65535, 65535),
		ExitFrame(0),
		ExitFrame(1),
		ExitFrame(255),
	} {
		got, err := ReadFrame(bytes.NewReader(AppendFrame(nil, f)))
		if err != nil {
			t.Fatalf("ReadFrame(%+v): %v", f, err)
		}
		if !framesEqual(got, f) {
			t.Errorf("ReadFrame(AppendFrame(%+v)) = %+v", f, got)
		}
	}
}

func TestFrameReadsConsecutiveFramesFromOneStream(t *testing.T) {
	t.Parallel()
	// The host's pump writes frame after frame down one socket, so the reader
	// must consume exactly one frame's bytes and leave the next one intact.
	var wire []byte
	want := []Frame{
		DataFrame([]byte("hello")),
		ResizeFrame(80, 24),
		DataFrame([]byte("world")),
		ExitFrame(130),
	}
	for _, f := range want {
		wire = AppendFrame(wire, f)
	}

	r := bytes.NewReader(wire)
	for i, w := range want {
		got, err := ReadFrame(r)
		if err != nil {
			t.Fatalf("frame %v: %v", i, err)
		}
		if !framesEqual(got, w) {
			t.Errorf("frame %v = %+v, want %+v", i, got, w)
		}
	}
	if r.Len() != 0 {
		t.Errorf("%v bytes were left unconsumed", r.Len())
	}
}
