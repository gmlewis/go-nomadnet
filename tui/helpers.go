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
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

// OSC52Copy copies text using the OSC 52 escape sequence, which works
// in terminals that support it (iTerm2, kitty, etc.).
// Matches Python's osc52_copy() at Helpers.py:7.
func OSC52Copy(text string) error {
	if text == "" {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	escape := "\x1b]52;c;" + encoded + "\x07"
	_, err := fmt.Print(escape)
	return err
}

var (
	// ErrNoClipboardTool is returned when no clipboard tool is found.
	ErrNoClipboardTool = fmt.Errorf("no clipboard tool found (install xclip or xsel)")

	// ErrUnsupportedPlatform is returned on unsupported platforms.
	ErrUnsupportedPlatform = fmt.Errorf("unsupported platform for clipboard operations")
)

// TruncateString returns s truncated to at most maxVisible runes, appending
// "..." when the input is longer. It operates on runes, never splitting a
// multibyte UTF-8 character, so the result is always valid UTF-8 and never
// contains a stray U+FFFD. It is the rune-safe replacement for the byte-wise
// `s[:n]+"..."` pattern. A non-positive maxVisible yields just "..." when the
// input is non-empty.
func TruncateString(s string, maxVisible int) string {
	if maxVisible < 0 {
		maxVisible = 0
	}
	r := []rune(s)
	if len(r) <= maxVisible {
		return s
	}
	if maxVisible <= 0 {
		return "..."
	}
	return string(r[:maxVisible]) + "..."
}

// ClickableIcon is a tview primitive that renders a glyph and fires
// a callback on mouse click. Matches Python's ClickableIcon at
// Helpers.py:20.
type ClickableIcon struct {
	*tview.Box
	glyph  string
	action func()
}

// NewClickableIcon creates a clickable icon with the given glyph and action.
func NewClickableIcon(glyph string, action func()) *ClickableIcon {
	ci := &ClickableIcon{
		Box:    tview.NewBox(),
		glyph:  glyph,
		action: action,
	}
	ci.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action&tview.MouseLeftClick != 0 {
			x, y := event.Position()
			if ci.InRect(x, y) {
				ci.HandleMouseLeftClick()
			}
		}
		return action, event
	})
	return ci
}

// HandleMouseLeftClick fires the on-click callback.
// Matches Python's ClickableIcon.mouse_event() at Helpers.py:27.
func (ci *ClickableIcon) HandleMouseLeftClick() {
	if ci.action != nil {
		ci.action()
	}
}

// Draw implements tview.Primitive.
func (ci *ClickableIcon) Draw(screen tcell.Screen) {
	ci.Box.DrawForSubclass(screen, ci)
	x, y, w, _ := ci.GetInnerRect()
	if w > 0 {
		ci.glyph = truncateStr(ci.glyph, w)
	}
	for i, r := range ci.glyph {
		if x+i >= x+w {
			break
		}
		screen.SetContent(x+i, y, r, nil, tcell.StyleDefault.Foreground(tcell.NewHexColor(0xdddddd)))
	}
}

// BuildTrustBanner creates a trust warning banner with Trust/Block/Do nothing
// buttons for untrusted peers. Matches Python's _build_trust_banner at
// Conversations.py:1957.
func BuildTrustBanner(onTrust, onBlock, onIgnore func()) *tview.Flex {
	warning := tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xffdd33)).
		SetText(" ⚠ This peer isn't trusted yet.")

	trustBtn := tview.NewButton("[Trust]")
	trustBtn.SetBackgroundColor(tcell.NewHexColor(0x444444))
	trustBtn.SetLabelColor(tcell.NewHexColor(0xdddddd))
	if onTrust != nil {
		trustBtn.SetSelectedFunc(func() { onTrust() })
	}

	blockBtn := tview.NewButton("[Block]")
	blockBtn.SetBackgroundColor(tcell.NewHexColor(0x444444))
	blockBtn.SetLabelColor(tcell.NewHexColor(0xdddddd))
	if onBlock != nil {
		blockBtn.SetSelectedFunc(func() { onBlock() })
	}

	ignoreBtn := tview.NewButton("[Do nothing]")
	ignoreBtn.SetBackgroundColor(tcell.NewHexColor(0x444444))
	ignoreBtn.SetLabelColor(tcell.NewHexColor(0xdddddd))
	if onIgnore != nil {
		ignoreBtn.SetSelectedFunc(func() { onIgnore() })
	}

	banner := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(warning, 0, 1, false).
		AddItem(trustBtn, 9, 0, false).
		AddItem(tview.NewTextView().SetText(" "), 1, 0, false).
		AddItem(blockBtn, 8, 0, false).
		AddItem(tview.NewTextView().SetText(" "), 1, 0, false).
		AddItem(ignoreBtn, 13, 0, false)
	banner.SetBackgroundColor(tcell.NewHexColor(0x553300))

	return banner
}
