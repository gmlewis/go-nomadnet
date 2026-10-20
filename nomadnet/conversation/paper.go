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
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/gmlewis/go-reticulum/lxmf"
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

// ingestPaperMessage ingests the outbound paper message and appends the INGESTED
// message to the conversation, mirroring Python's paper_output, which in every
// ingesting mode runs
//
//	message_path = Conversation.ingest(lxm, self.app, originator=True)
//	self.messages.append(ConversationMessage(message_path))
//
// (Conversation.py:358-360, 368-370, 377-379). The appended conversation message is
// therefore the file inside the conversation directory — not the PNG or URI written
// to downloads_path, and not the temporary print file, which is removed after
// printing.
func (c *Conversation) ingestPaperMessage(lxm *lxmf.Message) bool {
	messagePath, err := c.sendDeps.Ingest(lxm, true)
	if err != nil {
		return false
	}
	c.Messages = append(c.Messages, NewMessage(messagePath))
	return true
}

// writePNG encodes img as a PNG file at path. The bytes are buffered before the
// write so a failed encode cannot leave a truncated QR on disk.
func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

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
		if !c.ingestPaperMessage(lxm) {
			return "", false
		}
		return path, true

	case PaperSaveQR:
		img, err := lxm.AsQR()
		if err != nil {
			return "", false
		}
		path := filepath.Join(deps.DownloadsPath(), "LXM_"+hex.EncodeToString(lxm.Hash)+".png")
		if err := writePNG(path, img); err != nil {
			return "", false
		}
		if !c.ingestPaperMessage(lxm) {
			return "", false
		}
		return path, true

	case PaperPrintQR:
		img, err := lxm.AsQR()
		if err != nil {
			return "", false
		}
		tmpPath := filepath.Join(deps.TmpFilesPath(), hex.EncodeToString(lxm.Hash))
		if err := writePNG(tmpPath, img); err != nil {
			return "", false
		}
		printOK := deps.PrintFile(tmpPath)
		_ = os.Remove(tmpPath)
		if !printOK {
			return "", false
		}
		if !c.ingestPaperMessage(lxm) {
			return "", false
		}
		return "", true

	default:
		return "", false
	}
}
