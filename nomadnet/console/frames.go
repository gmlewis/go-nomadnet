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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxPayload is the largest frame payload either half of the console protocol
// accepts. Frames are keystrokes and terminal output, so a megabyte is far
// beyond anything legitimate; the limit exists so that a peer which announces
// 0xFFFFFFFF bytes cannot make the other end reserve four gigabytes.
const MaxPayload = 1 << 20

// The errors a frame codec returns. Every one of them means the connection is
// unusable: the byte stream has lost its framing, and resynchronising would
// mean guessing where the next frame begins.
var (
	// ErrOversizeFrame reports a frame whose declared length exceeds MaxPayload.
	ErrOversizeFrame = errors.New("console: frame payload larger than 1 MiB")

	// ErrUnknownFrameType reports a frame whose type byte names no frame this
	// protocol defines.
	ErrUnknownFrameType = errors.New("console: unknown frame type")

	// ErrTruncatedFrame reports a frame that ended before its declared length,
	// including a header that ended mid-field.
	ErrTruncatedFrame = errors.New("console: truncated frame")

	// ErrMalformedFrame reports a frame whose payload length is impossible for
	// its type, such as a resize frame that is not four bytes.
	ErrMalformedFrame = errors.New("console: malformed frame payload")
)

// FrameType identifies a frame on the console socket.
type FrameType byte

// The frame types. DATA flows both ways, RESIZE both ways, and EXIT only from
// the host to the app.
const (
	// FrameData carries raw terminal bytes.
	FrameData FrameType = 0x01

	// FrameResize carries a new window size: columns then rows, big-endian.
	FrameResize FrameType = 0x02

	// FrameExit carries the client's exit code, which is the whole payload.
	FrameExit FrameType = 0x03
)

// Frame is one decoded console frame.
//
// Only the fields its Type names are meaningful: Data for FrameData, Cols and
// Rows for FrameResize, and Code for FrameExit.
type Frame struct {
	// Type says which frame this is.
	Type FrameType

	// Data is the payload of a FrameData frame.
	Data []byte

	// Cols and Rows are the window size of a FrameResize frame.
	Cols uint16
	Rows uint16

	// Code is the exit status of a FrameExit frame.
	Code byte
}

// DataFrame returns a DATA frame carrying p.
func DataFrame(p []byte) Frame { return Frame{Type: FrameData, Data: p} }

// ResizeFrame returns a RESIZE frame for a cols-by-rows terminal.
func ResizeFrame(cols, rows uint16) Frame {
	return Frame{Type: FrameResize, Cols: cols, Rows: rows}
}

// ExitFrame returns an EXIT frame carrying the client's exit code.
func ExitFrame(code byte) Frame { return Frame{Type: FrameExit, Code: code} }

// payload returns the bytes this frame carries, in wire order.
func (f Frame) payload() []byte {
	switch f.Type {
	case FrameData:
		return f.Data
	case FrameResize:
		return []byte{byte(f.Cols >> 8), byte(f.Cols), byte(f.Rows >> 8), byte(f.Rows)}
	case FrameExit:
		return []byte{f.Code}
	}
	return nil
}

// String describes the frame for logs and test failures.
func (f Frame) String() string {
	switch f.Type {
	case FrameData:
		return fmt.Sprintf("Data(%q)", f.Data)
	case FrameResize:
		return fmt.Sprintf("Resize(%vx%v)", f.Cols, f.Rows)
	case FrameExit:
		return fmt.Sprintf("Exit(%v)", f.Code)
	}
	return fmt.Sprintf("Frame(type=0x%02x)", byte(f.Type))
}

// AppendFrame appends the wire form of f to dst and returns the extended slice.
//
// Every frame is type(1) | length(4, big-endian) | payload.
func AppendFrame(dst []byte, f Frame) []byte {
	payload := f.payload()
	var header [5]byte
	header[0] = byte(f.Type)
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	dst = append(dst, header[:]...)
	return append(dst, payload...)
}

// WriteFrame writes f to w as a single Write, so that a frame can never be
// interleaved with another one on a shared connection.
func WriteFrame(w io.Writer, f Frame) error {
	if _, err := w.Write(AppendFrame(nil, f)); err != nil {
		return fmt.Errorf("console: writing %v: %w", f, err)
	}
	return nil
}

// ReadFrame reads exactly one frame from r.
//
// A declared length over MaxPayload and a type this protocol does not define
// are refused before anything is read or allocated, and a stream that ends
// inside a frame is an error rather than a short read to be acted on.
//
// A stream that ends exactly at a frame boundary returns io.EOF, which is a
// clean end of the session and not a lost frame; the two have to be told apart,
// because one is how every session finishes and the other means the bytes seen
// since the last frame cannot be trusted.
func ReadFrame(r io.Reader) (Frame, error) {
	var header [5]byte
	n, err := io.ReadFull(r, header[:])
	if err != nil {
		if n == 0 && errors.Is(err, io.EOF) {
			return Frame{}, io.EOF
		}
		return Frame{}, fmt.Errorf("console: reading frame header: %w", truncation(err))
	}

	ft := FrameType(header[0])
	length := binary.BigEndian.Uint32(header[1:])

	// The order matters: neither check may allocate, and both must happen
	// before the payload is read.
	if length > MaxPayload {
		return Frame{}, fmt.Errorf("%w: %v bytes", ErrOversizeFrame, length)
	}
	if !ft.known() {
		return Frame{}, fmt.Errorf("%w: 0x%02x", ErrUnknownFrameType, byte(ft))
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Frame{}, fmt.Errorf("console: reading frame payload: %w", truncation(err))
	}

	f := Frame{Type: ft}
	switch ft {
	case FrameData:
		f.Data = payload
	case FrameResize:
		if len(payload) != 4 {
			return Frame{}, fmt.Errorf("%w: a resize frame is %v bytes, want 4", ErrMalformedFrame, len(payload))
		}
		f.Cols = binary.BigEndian.Uint16(payload[0:2])
		f.Rows = binary.BigEndian.Uint16(payload[2:4])
	case FrameExit:
		if len(payload) != 1 {
			return Frame{}, fmt.Errorf("%w: an exit frame is %v bytes, want 1", ErrMalformedFrame, len(payload))
		}
		f.Code = payload[0]
	}
	return f, nil
}

// known reports whether ft names a frame this protocol defines.
func (ft FrameType) known() bool {
	switch ft {
	case FrameData, FrameResize, FrameExit:
		return true
	}
	return false
}

// truncation maps the two ways a read can end early onto ErrTruncatedFrame,
// leaving any other read error as it stands.
func truncation(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrTruncatedFrame
	}
	return err
}
