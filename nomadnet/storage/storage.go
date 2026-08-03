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

// Package storage manages NomadNet's on-disk storage layout.
//
// NomadNet organizes its data under a config directory with subdirectories
// for conversations, attachments, pages, files, cache, resources, and
// temporary files.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths holds all storage directory paths for a NomadNet instance.
type Paths struct {
	// Root is the base config directory (e.g., ~/.nomadnetwork).
	Root string

	// Storage is the main storage directory.
	Storage string

	// Identity is the path to the primary identity file.
	Identity string

	// Cache is the cache directory.
	Cache string

	// Resources is the resources directory.
	Resources string

	// Conversations is the conversations directory.
	Conversations string

	// Directory is the peer directory storage.
	Directory string

	// PeerSettings is the path to the peer settings file (msgpack).
	PeerSettings string

	// TmpFiles is the temporary files directory.
	TmpFiles string

	// Attachments is the attachments directory.
	Attachments string

	// Pages is the node pages directory.
	Pages string

	// Files is the node files directory.
	Files string

	// LogFile is the path to the log file.
	LogFile string

	// ErrorFile is the path to the error log file.
	ErrorFile string

	// Examples is the path to the examples directory.
	Examples string
}

// New creates a Paths struct rooted at the given config directory.
func New(root string) *Paths {
	return &Paths{
		Root:          root,
		Storage:       filepath.Join(root, "storage"),
		Identity:      filepath.Join(root, "storage", "identity"),
		Cache:         filepath.Join(root, "storage", "cache"),
		Resources:     filepath.Join(root, "storage", "resources"),
		Conversations: filepath.Join(root, "storage", "conversations"),
		Directory:     filepath.Join(root, "storage", "directory"),
		PeerSettings:  filepath.Join(root, "storage", "peersettings"),
		TmpFiles:      filepath.Join(root, "storage", "tmp"),
		Attachments:   filepath.Join(root, "storage", "attachments"),
		Pages:         filepath.Join(root, "storage", "pages"),
		Files:         filepath.Join(root, "storage", "files"),
		LogFile:       filepath.Join(root, "logfile"),
		ErrorFile:     filepath.Join(root, "errors"),
		Examples:      filepath.Join(root, "examples"),
	}
}

// EnsureDirs creates all required storage directories if they don't exist.
func (p *Paths) EnsureDirs() error {
	dirs := []string{
		p.Storage,
		p.Cache,
		p.Resources,
		p.Conversations,
		p.Pages,
		p.Files,
		p.TmpFiles,
		p.Attachments,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %v: %w", dir, err)
		}
	}

	return nil
}

// ConversationDir returns the path for a specific conversation's storage.
func (p *Paths) ConversationDir(sourceHash string) string {
	return filepath.Join(p.Conversations, sourceHash)
}

// UnreadFlag returns the path to the unread flag file for a conversation.
func (p *Paths) UnreadFlag(sourceHash string) string {
	return filepath.Join(p.ConversationDir(sourceHash), "unread")
}

// FailedFlag returns the path to the failed flag file for a conversation.
func (p *Paths) FailedFlag(sourceHash string) string {
	return filepath.Join(p.ConversationDir(sourceHash), "failed")
}

// MessageDir returns the path for a specific message within a conversation.
func (p *Paths) MessageDir(sourceHash, messageID string) string {
	return filepath.Join(p.ConversationDir(sourceHash), messageID)
}
