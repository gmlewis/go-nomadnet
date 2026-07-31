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
	"os"
	"path/filepath"

	"github.com/gmlewis/go-reticulum/lxmf"
	"rsc.io/qr"
)

// PaperOutputMode selects how a paper (offline) message is emitted, mirroring
// the mode argument of Python Conversation.paper_output at Conversation.py:330.
type PaperOutputMode int

const (
	// PaperPrintQR renders the message as a QR code written to a temporary
	// file, prints that file via SendDeps.PrintFile, then removes it. The bool
	// result reports the print success.
	PaperPrintQR PaperOutputMode = iota
	// PaperSaveQR renders the message as a QR PNG saved under downloads_path
	// and returns the save path.
	PaperSaveQR
	// PaperSaveURI writes the paper-message URI (with a trailing newline) to a
	// .txt file under downloads_path and returns the save path.
	PaperSaveURI
	// PaperReturnURI returns the paper-message URI string without writing a
	// file or ingesting.
	PaperReturnURI
)

// PaperOutput generates an offline paper (QR/URI) message addressed to the
// conversation peer, mirroring Python Conversation.paper_output at
// Conversation.py:330. It builds an LXMF message with the PAPER delivery
// method, then emits it according to mode. The save and return modes return
// the resulting path or URI string with ok=true; print_qr returns ok reporting
// the print success. On any failure (no destination, encode error, write
// error) it returns ("", false). Modes save_qr, save_uri, and a successful
// print_qr ingest the outbound message into the conversation, matching Python.
func (c *Conversation) PaperOutput(content, title string, mode PaperOutputMode) (string, bool) {
	deps := c.sendDeps
	if deps == nil {
		return "", false
	}
	dest := deps.SendDestination()
	if dest == nil {
		return "", false
	}
	source := deps.LXMFSource()
	if source == nil {
		return "", false
	}

	lxm, err := lxmf.NewMessage(dest, source, content, title, nil)
	if err != nil {
		return "", false
	}
	lxm.DesiredMethod = lxmf.MethodPaper

	switch mode {
	case PaperReturnURI:
		uri, err := lxm.AsURI(true)
		if err != nil {
			return "", false
		}
		return uri, true

	case PaperSaveURI:
		uri, err := lxm.AsURI(true)
		if err != nil {
			return "", false
		}
		path := filepath.Join(deps.DownloadsPath(), "LXM_"+hex.EncodeToString(lxm.Hash)+".txt")
		if err := os.WriteFile(path, []byte(uri+"\n"), 0o644); err != nil {
			return "", false
		}
		if _, err := deps.Ingest(lxm, true); err != nil {
			return "", false
		}
		c.Messages = append(c.Messages, NewMessage(path))
		return path, true

	case PaperSaveQR:
		uri, err := lxm.AsURI(false)
		if err != nil {
			return "", false
		}
		code, err := qr.Encode(uri, qr.L)
		if err != nil {
			return "", false
		}
		lxm.DetermineTransportEncryption()
		path := filepath.Join(deps.DownloadsPath(), "LXM_"+hex.EncodeToString(lxm.Hash)+".png")
		if err := os.WriteFile(path, code.PNG(), 0o644); err != nil {
			return "", false
		}
		if _, err := deps.Ingest(lxm, true); err != nil {
			return "", false
		}
		c.Messages = append(c.Messages, NewMessage(path))
		return path, true

	case PaperPrintQR:
		uri, err := lxm.AsURI(false)
		if err != nil {
			return "", false
		}
		code, err := qr.Encode(uri, qr.L)
		if err != nil {
			return "", false
		}
		lxm.DetermineTransportEncryption()
		tmpPath := filepath.Join(deps.TmpFilesPath(), hex.EncodeToString(lxm.Hash))
		if err := os.WriteFile(tmpPath, code.PNG(), 0o644); err != nil {
			return "", false
		}
		printOK := deps.PrintFile(tmpPath)
		_ = os.Remove(tmpPath)
		if !printOK {
			return "", false
		}
		if _, err := deps.Ingest(lxm, true); err != nil {
			return "", false
		}
		c.Messages = append(c.Messages, NewMessage(tmpPath))
		return "", true

	default:
		return "", false
	}
}
