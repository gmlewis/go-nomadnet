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

package conversation

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/msgpack"
)

// StripAttachmentsFromFile rewrites the LXMF message container at filePath so
// that its file/image/audio attachment fields are removed, mirroring the
// Python NomadNet ConversationMessage.strip_attachments_from_file.
//
// The operation only proceeds when an extracted attachment directory exists
// for the message hash under attachmentPath; otherwise it is a no-op. The
// rewrite is performed as a surgical edit of the on-disk msgpack bytes so the
// result is byte-for-byte identical to Python's msgpack.packb output: every
// byte outside the attachment entries, the fields-map count header, and the
// lxmf_bytes bin value is preserved verbatim, avoiding any dependence on Go
// map iteration order.
func StripAttachmentsFromFile(filePath, attachmentPath string) error {
	containerBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	valStart, valEnd, _, _, lxmfBytes, ok := findLxmfBytesValue(containerBytes)
	if !ok {
		// Python returns when lxmf_bytes is absent.
		return nil
	}

	headerLen := 2*lxmf.DestinationLength + lxmf.SignatureLength
	if len(lxmfBytes) < headerLen {
		return nil
	}
	header := lxmfBytes[:headerLen]
	payloadBytes := lxmfBytes[headerLen:]

	elemStarts, elemEnds, _, arrOK := parseArraySpans(payloadBytes, 0)
	if !arrOK || len(elemStarts) < 4 {
		// Python returns when the payload is not a 4+ element array.
		return nil
	}

	fieldsStart := elemStarts[3]
	fieldsEnd := elemEnds[3]

	entries, mapOK := parseMapEntries(payloadBytes, fieldsStart)
	if !mapOK {
		// Python returns when payload[3] is not a dict.
		return nil
	}

	attachmentKeys := []int{lxmf.FieldFileAttachments, lxmf.FieldImage, lxmf.FieldAudio}
	var removeIdx []int
	for i, e := range entries {
		if isAttachmentKey(payloadBytes, e.keyStart, e.keyEnd, attachmentKeys) {
			removeIdx = append(removeIdx, i)
		}
	}
	if len(removeIdx) == 0 {
		// Python returns when no attachment field is present.
		return nil
	}

	// Compute the message hash exactly as Python does, so the attachment
	// directory lookup matches the directory extract_attachments_from_lxm
	// created (keyed by the same hash).
	var hashPayload []byte
	if len(elemStarts) > 4 {
		// hash_payload = msgpack.packb(payload[:4]); the first four elements
		// round-trip stably, so re-emitting a minimal 4-element array header
		// followed by their verbatim bytes reproduces Python's packb output.
		hashPayload = append(minimalArrayHeader(4), payloadBytes[elemStarts[0]:elemEnds[3]]...)
	} else {
		hashPayload = payloadBytes
	}
	hashInput := append(append([]byte{}, lxmfBytes[:2*lxmf.DestinationLength]...), hashPayload...)
	msgHash := rns.FullHash(hashInput)
	attDir := filepath.Join(attachmentPath, hex.EncodeToString(msgHash))
	if !isDir(attDir) {
		// Python returns when the attachment directory is absent.
		return nil
	}

	// Rebuild the fields map: minimal count header for the surviving entries
	// followed by their verbatim key+value bytes, in original order.
	remaining := make([]mapEntry, 0, len(entries)-len(removeIdx))
	rm := 0
	for i, e := range entries {
		if rm < len(removeIdx) && i == removeIdx[rm] {
			rm++
			continue
		}
		remaining = append(remaining, e)
	}
	newFields := make([]byte, 0, fieldsEnd-fieldsStart)
	newFields = append(newFields, minimalMapHeader(len(remaining))...)
	for _, e := range remaining {
		newFields = append(newFields, payloadBytes[e.keyStart:e.valEnd]...)
	}

	newPayload := make([]byte, 0, len(payloadBytes)-(fieldsEnd-fieldsStart)+len(newFields))
	newPayload = append(newPayload, payloadBytes[:fieldsStart]...)
	newPayload = append(newPayload, newFields...)
	newPayload = append(newPayload, payloadBytes[fieldsEnd:]...)

	newLxmfBytes := make([]byte, 0, len(header)+len(newPayload))
	newLxmfBytes = append(newLxmfBytes, header...)
	newLxmfBytes = append(newLxmfBytes, newPayload...)

	// Re-encode the lxmf_bytes bin value with a minimal length header, matching
	// Python's packb of a bytes object.
	newBinValue, err := msgpack.Pack(newLxmfBytes)
	if err != nil {
		return fmt.Errorf("encode stripped lxmf_bytes: %w", err)
	}

	newContainer := make([]byte, 0, len(containerBytes)-(valEnd-valStart)+len(newBinValue))
	newContainer = append(newContainer, containerBytes[:valStart]...)
	newContainer = append(newContainer, newBinValue...)
	newContainer = append(newContainer, containerBytes[valEnd:]...)

	if err := os.WriteFile(filePath, newContainer, 0o644); err != nil {
		return err
	}
	return nil
}

// mapEntry records the byte spans of a single msgpack map entry within a
// containing byte slice.
type mapEntry struct {
	keyStart, keyEnd int
	valStart, valEnd int
}

// findLxmfBytesValue locates the "lxmf_bytes" entry in the top-level container
// msgpack map and returns the span of its bin value (valStart..valEnd, the
// bytes replaced on rewrite), the span of the bin content
// (contentStart..contentEnd, the raw lxmf_bytes without the msgpack bin
// header), and that content. It reports ok=false when the container is not a
// map, the key is absent, or its value is not a bin.
func findLxmfBytesValue(data []byte) (valStart, valEnd, contentStart, contentEnd int, lxmfBytes []byte, ok bool) {
	entries, found := parseMapEntries(data, 0)
	if !found {
		return 0, 0, 0, 0, nil, false
	}
	for _, e := range entries {
		if isLxmfBytesKey(data, e.keyStart, e.keyEnd) {
			cs, ce, err := binContentSpan(data, e.valStart)
			if err != nil {
				return 0, 0, 0, 0, nil, false
			}
			return e.valStart, e.valEnd, cs, ce, data[cs:ce], true
		}
	}
	return 0, 0, 0, 0, nil, false
}

// binContentSpan returns the byte span of the content of the msgpack bin value
// starting at pos (i.e. the bytes following the bin length header).
func binContentSpan(data []byte, pos int) (contentStart, contentEnd int, err error) {
	if pos >= len(data) {
		return 0, 0, errors.New("msgpack: bin value missing")
	}
	switch data[pos] {
	case 0xc4: // bin8
		if pos+2 > len(data) {
			return 0, 0, errors.New("msgpack: truncated bin8")
		}
		n := int(data[pos+1])
		cs := pos + 2
		return cs, cs + n, nil
	case 0xc5: // bin16
		if pos+3 > len(data) {
			return 0, 0, errors.New("msgpack: truncated bin16")
		}
		n := int(data[pos+1])<<8 | int(data[pos+2])
		cs := pos + 3
		return cs, cs + n, nil
	case 0xc6: // bin32
		if pos+5 > len(data) {
			return 0, 0, errors.New("msgpack: truncated bin32")
		}
		n := int(data[pos+1])<<24 | int(data[pos+2])<<16 | int(data[pos+3])<<8 | int(data[pos+4])
		cs := pos + 5
		return cs, cs + n, nil
	default:
		return 0, 0, fmt.Errorf("msgpack: expected bin value, got 0x%02x", data[pos])
	}
}

// isLxmfBytesKey reports whether the msgpack value spanning [start,end) in data
// decodes to the string "lxmf_bytes" (str or bin form).
func isLxmfBytesKey(data []byte, start, end int) bool {
	k, err := msgpack.Unpack(data[start:end])
	if err != nil {
		return false
	}
	switch v := k.(type) {
	case string:
		return v == "lxmf_bytes"
	case []byte:
		return string(v) == "lxmf_bytes"
	}
	return false
}

// isAttachmentKey reports whether the msgpack value spanning [start,end) in
// data decodes to one of the integer attachment field keys.
func isAttachmentKey(data []byte, start, end int, keys []int) bool {
	k, err := msgpack.Unpack(data[start:end])
	if err != nil {
		return false
	}
	var iv int
	switch v := k.(type) {
	case int64:
		iv = int(v)
	case int:
		iv = v
	default:
		return false
	}
	return slices.Contains(keys, iv)
}

// parseMapEntries parses the msgpack map starting at pos and returns its
// entries' byte spans. The returned spans index into the same data slice.
func parseMapEntries(data []byte, pos int) ([]mapEntry, bool) {
	if pos >= len(data) {
		return nil, false
	}
	b := data[pos]
	var count int
	var body int
	switch {
	case b >= 0x80 && b <= 0x8f:
		count = int(b & 0x0f)
		body = pos + 1
	case b == 0xde:
		if pos+3 > len(data) {
			return nil, false
		}
		count = int(data[pos+1])<<8 | int(data[pos+2])
		body = pos + 3
	case b == 0xdf:
		if pos+5 > len(data) {
			return nil, false
		}
		count = int(data[pos+1])<<24 | int(data[pos+2])<<16 | int(data[pos+3])<<8 | int(data[pos+4])
		body = pos + 5
	default:
		return nil, false
	}

	entries := make([]mapEntry, 0, count)
	cur := body
	for range count {
		keyStart := cur
		keyEnd, err := skipValue(data, keyStart)
		if err != nil {
			return nil, false
		}
		valStart := keyEnd
		valEnd, err := skipValue(data, valStart)
		if err != nil {
			return nil, false
		}
		entries = append(entries, mapEntry{keyStart: keyStart, keyEnd: keyEnd, valStart: valStart, valEnd: valEnd})
		cur = valEnd
	}
	return entries, true
}

// parseArraySpans parses the msgpack array starting at pos and returns the
// byte spans of its elements plus the array's total end offset.
func parseArraySpans(data []byte, pos int) (starts, ends []int, arrEnd int, ok bool) {
	if pos >= len(data) {
		return nil, nil, pos, false
	}
	b := data[pos]
	var count int
	var body int
	switch {
	case b >= 0x90 && b <= 0x9f:
		count = int(b & 0x0f)
		body = pos + 1
	case b == 0xdc:
		if pos+3 > len(data) {
			return nil, nil, pos, false
		}
		count = int(data[pos+1])<<8 | int(data[pos+2])
		body = pos + 3
	case b == 0xdd:
		if pos+5 > len(data) {
			return nil, nil, pos, false
		}
		count = int(data[pos+1])<<24 | int(data[pos+2])<<16 | int(data[pos+3])<<8 | int(data[pos+4])
		body = pos + 5
	default:
		return nil, nil, pos, false
	}

	starts = make([]int, 0, count)
	ends = make([]int, 0, count)
	cur := body
	for range count {
		s := cur
		e, err := skipValue(data, s)
		if err != nil {
			return nil, nil, pos, false
		}
		starts = append(starts, s)
		ends = append(ends, e)
		cur = e
	}
	return starts, ends, cur, true
}

// skipValue returns the end offset of the msgpack value starting at pos within
// data. It walks nested structures without decoding their contents.
func skipValue(data []byte, pos int) (int, error) {
	if pos >= len(data) {
		return 0, errors.New("msgpack: unexpected end of value")
	}
	b := data[pos]
	switch {
	case b <= 0x7f: // positive fixint
		return pos + 1, nil
	case b >= 0xe0: // negative fixint
		return pos + 1, nil
	case b >= 0x80 && b <= 0x8f: // fixmap
		return skipMap(data, pos+1, int(b&0x0f))
	case b >= 0x90 && b <= 0x9f: // fixarray
		return skipArray(data, pos+1, int(b&0x0f))
	case b >= 0xa0 && b <= 0xbf: // fixstr
		return pos + 1 + int(b&0x1f), nil
	case b == 0xc0, b == 0xc2, b == 0xc3: // nil, false, true
		return pos + 1, nil
	case b == 0xc4: // bin8
		return skipBin(data, pos, 1)
	case b == 0xc5: // bin16
		return skipBin(data, pos, 2)
	case b == 0xc6: // bin32
		return skipBin(data, pos, 4)
	case b == 0xc7: // ext8
		return skipExt(data, pos, 1)
	case b == 0xc8: // ext16
		return skipExt(data, pos, 2)
	case b == 0xc9: // ext32
		return skipExt(data, pos, 4)
	case b == 0xca: // float32
		return pos + 5, nil
	case b == 0xcb: // float64
		return pos + 9, nil
	case b == 0xcc: // uint8
		return pos + 2, nil
	case b == 0xcd: // uint16
		return pos + 3, nil
	case b == 0xce: // uint32
		return pos + 5, nil
	case b == 0xcf: // uint64
		return pos + 9, nil
	case b == 0xd0: // int8
		return pos + 2, nil
	case b == 0xd1: // int16
		return pos + 3, nil
	case b == 0xd2: // int32
		return pos + 5, nil
	case b == 0xd3: // int64
		return pos + 9, nil
	case b == 0xd4: // fixext1
		return pos + 3, nil
	case b == 0xd5: // fixext2
		return pos + 4, nil
	case b == 0xd6: // fixext4
		return pos + 6, nil
	case b == 0xd7: // fixext8
		return pos + 10, nil
	case b == 0xd8: // fixext16
		return pos + 18, nil
	case b == 0xd9: // str8
		if pos+2 > len(data) {
			return 0, errors.New("msgpack: truncated str8")
		}
		return pos + 2 + int(data[pos+1]), nil
	case b == 0xda: // str16
		if pos+3 > len(data) {
			return 0, errors.New("msgpack: truncated str16")
		}
		n := int(data[pos+1])<<8 | int(data[pos+2])
		return pos + 3 + n, nil
	case b == 0xdb: // str32
		if pos+5 > len(data) {
			return 0, errors.New("msgpack: truncated str32")
		}
		n := int(data[pos+1])<<24 | int(data[pos+2])<<16 | int(data[pos+3])<<8 | int(data[pos+4])
		return pos + 5 + n, nil
	case b == 0xdc: // array16
		if pos+3 > len(data) {
			return 0, errors.New("msgpack: truncated array16")
		}
		return skipArray(data, pos+3, int(data[pos+1])<<8|int(data[pos+2]))
	case b == 0xdd: // array32
		if pos+5 > len(data) {
			return 0, errors.New("msgpack: truncated array32")
		}
		return skipArray(data, pos+5, int(data[pos+1])<<24|int(data[pos+2])<<16|int(data[pos+3])<<8|int(data[pos+4]))
	case b == 0xde: // map16
		if pos+3 > len(data) {
			return 0, errors.New("msgpack: truncated map16")
		}
		return skipMap(data, pos+3, int(data[pos+1])<<8|int(data[pos+2]))
	case b == 0xdf: // map32
		if pos+5 > len(data) {
			return 0, errors.New("msgpack: truncated map32")
		}
		return skipMap(data, pos+5, int(data[pos+1])<<24|int(data[pos+2])<<16|int(data[pos+3])<<8|int(data[pos+4]))
	default:
		return 0, fmt.Errorf("msgpack: unknown type 0x%02x", b)
	}
}

func skipBin(data []byte, pos, lenSize int) (int, error) {
	headerEnd := pos + 1 + lenSize
	if headerEnd > len(data) {
		return 0, errors.New("msgpack: truncated bin header")
	}
	var n int
	switch lenSize {
	case 1:
		n = int(data[pos+1])
	case 2:
		n = int(data[pos+1])<<8 | int(data[pos+2])
	case 4:
		n = int(data[pos+1])<<24 | int(data[pos+2])<<16 | int(data[pos+3])<<8 | int(data[pos+4])
	}
	end := headerEnd + n
	if end > len(data) {
		return 0, errors.New("msgpack: truncated bin body")
	}
	return end, nil
}

func skipExt(data []byte, pos, lenSize int) (int, error) {
	headerEnd := pos + 1 + lenSize
	if headerEnd > len(data) {
		return 0, errors.New("msgpack: truncated ext header")
	}
	var n int
	switch lenSize {
	case 1:
		n = int(data[pos+1])
	case 2:
		n = int(data[pos+1])<<8 | int(data[pos+2])
	case 4:
		n = int(data[pos+1])<<24 | int(data[pos+2])<<16 | int(data[pos+3])<<8 | int(data[pos+4])
	}
	// ext body = 1 type byte + n data bytes
	end := headerEnd + 1 + n
	if end > len(data) {
		return 0, errors.New("msgpack: truncated ext body")
	}
	return end, nil
}

func skipArray(data []byte, body int, count int) (int, error) {
	cur := body
	for range count {
		end, err := skipValue(data, cur)
		if err != nil {
			return 0, err
		}
		cur = end
	}
	return cur, nil
}

func skipMap(data []byte, body int, count int) (int, error) {
	cur := body
	for range count {
		keyEnd, err := skipValue(data, cur)
		if err != nil {
			return 0, err
		}
		valEnd, err := skipValue(data, keyEnd)
		if err != nil {
			return 0, err
		}
		cur = valEnd
	}
	return cur, nil
}

// minimalMapHeader returns the minimal msgpack encoding of a map with count
// entries, matching Python msgpack.packb's format selection.
func minimalMapHeader(count int) []byte {
	if count < 16 {
		return []byte{0x80 | byte(count)}
	}
	if count < 1<<16 {
		return []byte{0xde, byte(count >> 8), byte(count)}
	}
	return []byte{0xdf, byte(count >> 24), byte(count >> 16), byte(count >> 8), byte(count)}
}

// minimalArrayHeader returns the minimal msgpack encoding of an array with
// count elements, matching Python msgpack.packb's format selection.
func minimalArrayHeader(count int) []byte {
	if count < 16 {
		return []byte{0x90 | byte(count)}
	}
	if count < 1<<16 {
		return []byte{0xdc, byte(count >> 8), byte(count)}
	}
	return []byte{0xdd, byte(count >> 24), byte(count >> 16), byte(count >> 8), byte(count)}
}
