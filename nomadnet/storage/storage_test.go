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

package storage

import (
	"os"
	"testing"
)

func TestNewPaths(t *testing.T) {
	t.Parallel()

	p := New("/home/user/.nomadnetwork")

	if p.Root != "/home/user/.nomadnetwork" {
		t.Errorf("Root = %q", p.Root)
	}
	if p.Storage != "/home/user/.nomadnetwork/storage" {
		t.Errorf("Storage = %q", p.Storage)
	}
	if p.Identity != "/home/user/.nomadnetwork/storage/identity" {
		t.Errorf("Identity = %q", p.Identity)
	}
	if p.Cache != "/home/user/.nomadnetwork/storage/cache" {
		t.Errorf("Cache = %q", p.Cache)
	}
	if p.Resources != "/home/user/.nomadnetwork/storage/resources" {
		t.Errorf("Resources = %q", p.Resources)
	}
	if p.Conversations != "/home/user/.nomadnetwork/storage/conversations" {
		t.Errorf("Conversations = %q", p.Conversations)
	}
	if p.Directory != "/home/user/.nomadnetwork/storage/directory" {
		t.Errorf("Directory = %q", p.Directory)
	}
	if p.PeerSettings != "/home/user/.nomadnetwork/storage/peersettings" {
		t.Errorf("PeerSettings = %q", p.PeerSettings)
	}
	if p.TmpFiles != "/home/user/.nomadnetwork/storage/tmp" {
		t.Errorf("TmpFiles = %q", p.TmpFiles)
	}
	if p.Attachments != "/home/user/.nomadnetwork/storage/attachments" {
		t.Errorf("Attachments = %q", p.Attachments)
	}
	if p.Pages != "/home/user/.nomadnetwork/storage/pages" {
		t.Errorf("Pages = %q", p.Pages)
	}
	if p.Files != "/home/user/.nomadnetwork/storage/files" {
		t.Errorf("Files = %q", p.Files)
	}
	if p.LogFile != "/home/user/.nomadnetwork/logfile" {
		t.Errorf("LogFile = %q", p.LogFile)
	}
	if p.ErrorFile != "/home/user/.nomadnetwork/errors" {
		t.Errorf("ErrorFile = %q", p.ErrorFile)
	}
}

func TestEnsureDirs(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	p := New(dir)

	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	// Verify all directories were created
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

	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("directory %s not created: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}

func TestEnsureDirsIdempotent(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	p := New(dir)

	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("First EnsureDirs: %v", err)
	}
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("Second EnsureDirs: %v", err)
	}
}

func TestConversationDir(t *testing.T) {
	t.Parallel()

	p := New("/home/user/.nomadnetwork")
	got := p.ConversationDir("abc123")
	want := "/home/user/.nomadnetwork/storage/conversations/abc123"
	if got != want {
		t.Errorf("ConversationDir = %q, want %q", got, want)
	}
}

func TestUnreadFlag(t *testing.T) {
	t.Parallel()

	p := New("/home/user/.nomadnetwork")
	got := p.UnreadFlag("abc123")
	want := "/home/user/.nomadnetwork/storage/conversations/abc123/unread"
	if got != want {
		t.Errorf("UnreadFlag = %q, want %q", got, want)
	}
}

func TestFailedFlag(t *testing.T) {
	t.Parallel()

	p := New("/home/user/.nomadnetwork")
	got := p.FailedFlag("abc123")
	want := "/home/user/.nomadnetwork/storage/conversations/abc123/failed"
	if got != want {
		t.Errorf("FailedFlag = %q, want %q", got, want)
	}
}

func TestMessageDir(t *testing.T) {
	t.Parallel()

	p := New("/home/user/.nomadnetwork")
	got := p.MessageDir("abc123", "msg456")
	want := "/home/user/.nomadnetwork/storage/conversations/abc123/msg456"
	if got != want {
		t.Errorf("MessageDir = %q, want %q", got, want)
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nomadnet-storage-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
