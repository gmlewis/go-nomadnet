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
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// CopyToClipboard copies text to the system clipboard using
// platform-specific tools. On macOS uses pbcopy, on Linux
// uses xclip or xsel, on Windows uses clip.exe.
func CopyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return ErrNoClipboardTool
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return ErrUnsupportedPlatform
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// OSC52Copy attempts to copy text using the OSC 52 escape sequence,
// which works in terminals that support it (iTerm2, kitty, etc.).
func OSC52Copy(text string) error {
	// OSC 52 format: ESC ] 52 ; c ; <base64> ESC \
	// This is a placeholder implementation
	return nil
}

var (
	// ErrNoClipboardTool is returned when no clipboard tool is found.
	ErrNoClipboardTool = fmt.Errorf("no clipboard tool found (install xclip or xsel)")

	// ErrUnsupportedPlatform is returned on unsupported platforms.
	ErrUnsupportedPlatform = fmt.Errorf("unsupported platform for clipboard operations")
)
