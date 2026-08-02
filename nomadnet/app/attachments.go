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

package app

import (
	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
)

// SaveConversationAttachments copies the selected received attachments of the
// conversation identified by sourceHash to the download directory, mirroring
// Python's do_save (Conversations.py:2368-2391). The save directory is the
// configured attachment_save_path, falling back to downloads_path (Python
// save_dir = app.attachment_save_path if set else app.downloads_path). The
// conversation is rescanned so newly-ingested messages are present. Returns
// the saved destination paths and a count of failures.
func (a *App) SaveConversationAttachments(sourceHash string, selections []conversation.SaveAttachmentSelection) (saved []string, failed int) {
	if a.ConversationPath == "" {
		return nil, 0
	}
	conv := a.ConversationCache.Get(sourceHash)
	if conv == nil {
		return nil, 0
	}
	conv.SetTransport(a.Transport)
	_ = conv.ScanStorage()

	saveDir := a.AttachmentSavePath
	if saveDir == "" {
		saveDir = a.DownloadsPath
	}
	return conv.SaveAttachmentsToDir(selections, saveDir)
}