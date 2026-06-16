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

package tui

import (
	"strings"
	"testing"
)

func TestGenerateQRASCII(t *testing.T) {
	t.Parallel()

	got, err := GenerateQRASCII("Hello, World!")
	if err != nil {
		t.Fatalf("GenerateQRASCII error: %v", err)
	}
	if len(got) == 0 {
		t.Error("GenerateQRASCII returned empty string")
	}
	if strings.TrimSpace(got) == "" {
		t.Error("GenerateQRASCII returned only whitespace")
	}
}

func TestGenerateQRASCIIEmptyInput(t *testing.T) {
	t.Parallel()

	_, err := GenerateQRASCII("")
	if err == nil {
		t.Error("GenerateQRASCII('') should return error for empty input")
	}
}

func TestGenerateQRASCIILXMFAddress(t *testing.T) {
	t.Parallel()

	addr := "aabb1122aabb1122aabb1122aabb1122"
	got, err := GenerateQRASCII(addr)
	if err != nil {
		t.Fatalf("GenerateQRASCII error: %v", err)
	}
	if len(got) == 0 {
		t.Error("GenerateQRASCII returned empty string for LXMF address")
	}
}
