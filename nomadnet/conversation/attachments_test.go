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
	"bytes"
	"testing"
)

func TestSafeAttachmentName(t *testing.T) {
	t.Parallel()
	pad := func(n int, b byte) []byte {
		out := make([]byte, n)
		for i := range out {
			out[i] = b
		}
		return out
	}
	tests := []struct {
		name     string
		input    any
		fallback string
		want     string
	}{
		{"simple", "photo.png", "attachment", "photo.png"},
		{"bytes", []byte("photo.png"), "attachment", "photo.png"},
		{"path_slash", "/etc/passwd", "attachment", "passwd"},
		{"path_backslash", `C:\Users\x\file.txt`, "attachment", "file.txt"},
		{"colon", "scheme:rest:tail.png", "attachment", "tail.png"},
		{"dots", "....hidden", "attachment", "hidden"},
		{"dotdot", "..", "attachment", "attachment"},
		{"dot_only", ".", "attachment", "attachment"},
		{"empty", "", "attachment", "attachment"},
		{"none", nil, "attachment", "attachment"},
		{"int", 123, "attachment", "123"},
		{"control", "a\x00b\x07c", "attachment", "abc"},
		{"longname", string(pad(250, 'a')) + ".txt", "attachment", string(pad(196, 'a')) + ".txt"},
		{"custom_fallback", "", "myfile", "myfile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SafeAttachmentName(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("SafeAttachmentName(%v, %q) = %q, want %q", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestDetectImageExt(t *testing.T) {
	t.Parallel()
	zeros := func(n int) []byte { return bytes.Repeat([]byte{0}, n) }
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"png", append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, zeros(20)...), ".png"},
		{"jpg", append([]byte{0xff, 0xd8, 0xff}, zeros(20)...), ".jpg"},
		{"gif", append([]byte("GIF8"), zeros(20)...), ".gif"},
		{"webp", append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, append([]byte("WEBP"), zeros(20)...)...)...), ".webp"},
		{"heic1c", append([]byte{0, 0, 0, 0x1c}, zeros(20)...), ".heic"},
		{"heic18", append([]byte{0, 0, 0, 0x18}, zeros(20)...), ".heic"},
		{"short", zeros(5), ".bin"},
		{"none", zeros(20), ".bin"},
		{"nil", nil, ".bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DetectImageExt(tt.data)
			if got != tt.want {
				t.Errorf("DetectImageExt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectAudioExt(t *testing.T) {
	t.Parallel()
	zeros := func(n int) []byte { return bytes.Repeat([]byte{0}, n) }
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"ogg", append([]byte("OggS"), zeros(20)...), ".ogg"},
		{"mp3_fffb", append([]byte{0xff, 0xfb}, zeros(20)...), ".mp3"},
		{"mp3_id3", append([]byte("ID3"), zeros(20)...), ".mp3"},
		{"wav", append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, append([]byte("WAVE"), zeros(20)...)...)...), ".wav"},
		{"flac", append([]byte("fLaC"), zeros(20)...), ".flac"},
		{"short", zeros(5), ".bin"},
		{"none", zeros(20), ".bin"},
		{"nil", nil, ".bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DetectAudioExt(tt.data)
			if got != tt.want {
				t.Errorf("DetectAudioExt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtFromMediaFormat(t *testing.T) {
	t.Parallel()
	zeros := func(n int) []byte { return bytes.Repeat([]byte{0}, n) }
	pngData := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, zeros(20)...)
	oggData := append([]byte("OggS"), zeros(20)...)
	flacData := append([]byte("fLaC"), zeros(20)...)
	tests := []struct {
		name    string
		fmtVal  any
		data    []byte
		isAudio bool
		want    string
	}{
		{"str_webp", "webp", zeros(20), false, ".webp"},
		{"str_weird", "W E B P!", zeros(20), false, ".webp"},
		{"str_long", "abcdefghijk", zeros(20), false, ".abcdefgh"},
		{"str_empty", "", pngData, false, ".png"},
		{"none_str_image", nil, pngData, false, ".png"},
		{"int_audio_ogg", 20, oggData, true, ".ogg"},
		{"int_audio_c2", 5, zeros(20), true, ".c2"},
		{"int_audio_other", 100, oggData, true, ".ogg"},
		{"int_image", 20, pngData, false, ".png"},
		{"audio_fallback_detect", nil, flacData, true, ".flac"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtFromMediaFormat(tt.fmtVal, tt.data, tt.isAudio)
			if got != tt.want {
				t.Errorf("ExtFromMediaFormat = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnpackMediaField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		field   any
		wantFmt any
		wantDat []byte
		wantOK  bool
	}{
		{"bytes", []byte("rawdata"), nil, []byte("rawdata"), true},
		{"list_str_bytes", []any{"webp", []byte("imgdata")}, "webp", []byte("imgdata"), true},
		{"list_int_bytes", []any{5, []byte("audiodata")}, 5, []byte("audiodata"), true},
		{"list_short", []any{[]byte("onlyone")}, nil, nil, false},
		{"list_notbytes", []any{"a", "notbytes"}, nil, nil, false},
		{"none", nil, nil, nil, false},
		{"str", "hello", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fmtVal, data := UnpackMediaField(tt.field)
			if tt.wantOK {
				if !bytes.Equal(data, tt.wantDat) {
					t.Errorf("data = %v, want %v", data, tt.wantDat)
				}
			} else {
				if data != nil {
					t.Errorf("data = %v, want nil", data)
				}
			}
			if tt.wantFmt == nil {
				if fmtVal != nil {
					t.Errorf("fmt = %v, want nil", fmtVal)
				}
			} else {
				if fmtVal != tt.wantFmt {
					t.Errorf("fmt = %v, want %v", fmtVal, tt.wantFmt)
				}
			}
		})
	}
}
