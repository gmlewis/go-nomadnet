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
	"bytes"
	"errors"
	"strings"

	"github.com/mdp/qrterminal/v3"
)

// GenerateQRASCII generates a QR code from the given data string and
// returns it as an ASCII art string suitable for terminal display.
// Matches Python's show_qr_dialog() QR generation at Conversations.py:641.
// Uses Low error correction (matching Python's ERROR_CORRECT_L) and
// half-block rendering for compact display.
func GenerateQRASCII(data string) (string, error) {
	if data == "" {
		return "", errors.New("cannot generate QR code from empty data")
	}

	var buf bytes.Buffer
	config := qrterminal.Config{
		Level:          qrterminal.L,
		Writer:         &buf,
		HalfBlocks:     true,
		BlackChar:      qrterminal.BLACK,
		WhiteChar:      qrterminal.WHITE,
		BlackWhiteChar: qrterminal.BLACK,
		WhiteBlackChar: qrterminal.WHITE,
		QuietZone:      1,
	}
	qrterminal.GenerateWithConfig(data, config)

	result := strings.TrimRight(buf.String(), "\n")
	if result == "" {
		return "", errors.New("QR code generation produced empty output")
	}
	return result, nil
}
