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

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fontItem is the table entry the font tests run against.
func fontItem() publishedItem {
	for _, item := range publishedItems {
		if strings.Contains(item.relDest, "font.ttf") {
			return item
		}
	}
	return publishedItem{}
}

// tmuxItem is the table entry the tmux tests run against.
func tmuxItem() publishedItem {
	for _, item := range publishedItems {
		if item.relDest == ".tmux.conf" {
			return item
		}
	}
	return publishedItem{}
}

// colorsItem is the table entry the terminal-color tests run against.
func colorsItem() publishedItem {
	for _, item := range publishedItems {
		if strings.Contains(item.relDest, "colors.properties") {
			return item
		}
	}
	return publishedItem{}
}

// envFrom answers with a fixed environment, as a Termux login shell would.
func envFrom(pairs map[string]string) func(string) string {
	return func(name string) string { return pairs[name] }
}

// dirOf answers with one fixed directory listing, or with one fixed failure.
func dirOf(entries map[string][]os.DirEntry, failure error) func(string) ([]os.DirEntry, error) {
	return func(dir string) ([]os.DirEntry, error) {
		if failure != nil {
			return nil, failure
		}
		names, ok := entries[dir]
		if !ok {
			return nil, errors.New("no such directory")
		}
		out := make([]os.DirEntry, 0, len(names))
		out = append(out, names...)
		return out, nil
	}
}

// file builds a directory entry of the kind ReadDir returns for a plain file.
func file(name string) os.DirEntry { return entry{name: name} }

// dir builds a directory entry of the kind ReadDir returns for a directory.
func dir(name string) os.DirEntry { return entry{name: name, isDir: true} }

// entry is the smallest thing that satisfies os.DirEntry, so a listing can be
// written in a table without touching the filesystem.
type entry struct {
	name  string
	isDir bool
}

func (e entry) Name() string               { return e.name }
func (e entry) IsDir() bool                { return e.isDir }
func (e entry) Type() os.FileMode          { return 0 }
func (e entry) Info() (os.FileInfo, error) { return nil, errors.New("not used") }

func TestInTermux(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{
			name: "a Termux login shell",
			env:  map[string]string{"TERMUX_VERSION": "0.118.3", "PREFIX": "/data/data/com.termux/files/usr"},
			want: true,
		},
		{
			name: "the version alone, which a shortcut has",
			env:  map[string]string{"TERMUX_VERSION": "0.118.3"},
			want: true,
		},
		{
			name: "the prefix alone, which the widget runner has",
			env:  map[string]string{"PREFIX": "/data/data/com.termux/files/usr"},
			want: true,
		},
		{
			// Homebrew sets PREFIX on macOS, so the prefix is matched on what it
			// is and not on being set at all.
			name: "a desktop with its own prefix",
			env:  map[string]string{"PREFIX": "/opt/homebrew"},
		},
		{
			name: "a desktop with nothing set",
			env:  map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := inTermux(envFrom(tt.env)); got != tt.want {
				t.Errorf("inTermux(%v) = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

// TestPublishedItemsAreWhatTheAppliancePublishes pins the contract between the
// client and the Android appliance: the names the appliance writes into shared
// storage and the files the client installs from them.
func TestPublishedItemsAreWhatTheAppliancePublishes(t *testing.T) {
	t.Parallel()

	// The font's published name is the font's own with the extension Android's
	// type table appends to a file offered as OpenType.
	font := fontItem()
	if font.name == "" {
		t.Fatal("no item installs a terminal font")
	}
	if want := "AtkynsonMonoNerdFontMono-Regular.otf.ttf"; font.name != want {
		t.Errorf("the font is published as %q, want %q", font.name, want)
	}
	if want := filepath.Join(".termux", "font.ttf"); font.relDest != want {
		t.Errorf("the font is installed as %q, want %q", font.relDest, want)
	}
	if !font.reload {
		t.Error("the font does not ask Termux to reload its settings, so the terminal keeps its old font")
	}

	tmux := tmuxItem()
	if tmux.name == "" {
		t.Fatal("no item installs a tmux configuration")
	}
	if want := "tmux.conf"; tmux.name != want {
		t.Errorf("the tmux configuration is published as %q, want %q", tmux.name, want)
	}
	if want := ".tmux.conf"; tmux.relDest != want {
		t.Errorf("the tmux configuration is installed as %q, want %q", tmux.relDest, want)
	}
	// tmux reads its configuration when it starts, so nothing has to be told to
	// re-read anything.
	if tmux.reload {
		t.Error("the tmux configuration asks Termux to reload its settings, which it has no reason to do")
	}

	colors := colorsItem()
	if colors.name == "" {
		t.Fatal("no item installs terminal colors")
	}
	if want := "colors.properties"; colors.name != want {
		t.Errorf("the terminal colors are published as %q, want %q", colors.name, want)
	}
	if want := filepath.Join(".termux", "colors.properties"); colors.relDest != want {
		t.Errorf("the terminal colors are installed as %q, want %q", colors.relDest, want)
	}
	// A palette Termux has already read into the running terminal is not adopted
	// by writing the file, so this is one of the two that has to be reloaded.
	if !colors.reload {
		t.Error("the terminal colors do not ask Termux to reload its settings, so the terminal keeps its old palette")
	}
}

func TestFindPublished(t *testing.T) {
	t.Parallel()

	const sdcard = "/sdcard/Download"
	const symlink = "/home/storage/downloads"

	tests := []struct {
		name    string
		item    publishedItem
		dirs    []string
		entries map[string][]os.DirEntry
		failure error
		want    string
	}{
		{
			name: "the name the appliance publishes the font under",
			item: fontItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("kjv.txt"), file(fontItem().name), file("Reticulum Manual.epub")},
			},
			want: sdcard + "/" + fontItem().name,
		},
		{
			// Android's type table appends .ttf to a font published as OpenType,
			// so the stored name is the published one; a device that stored it
			// under the font's own name is adopted through the marker.
			name: "another Nerd Font, which a newer appliance may publish",
			item: fontItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("SomeOtherNerdFont-Regular.ttf")},
			},
			want: sdcard + "/SomeOtherNerdFont-Regular.ttf",
		},
		{
			name: "the exact name wins over a marker match",
			item: fontItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("ANerdFont-Regular.ttf"), file(fontItem().name)},
			},
			want: sdcard + "/" + fontItem().name,
		},
		{
			name: "the first directory that holds one wins",
			item: fontItem(),
			dirs: []string{symlink, sdcard},
			entries: map[string][]os.DirEntry{
				sdcard:  {file(fontItem().name)},
				symlink: {file("kjv.txt")},
			},
			want: sdcard + "/" + fontItem().name,
		},
		{
			name: "a directory holding no font falls through to the next",
			item: fontItem(),
			dirs: []string{symlink, sdcard},
			entries: map[string][]os.DirEntry{
				symlink: {file("kjv.txt")},
				sdcard:  {file(fontItem().name)},
			},
			want: sdcard + "/" + fontItem().name,
		},
		{
			// A directory named like the file is not the file.
			name: "a directory is not a font",
			item: fontItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {dir(fontItem().name)},
			},
		},
		{
			name: "nothing published",
			item: fontItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("kjv.txt"), file("gonomadnet")},
			},
		},
		{
			name:    "a downloads directory that cannot be read is skipped",
			item:    fontItem(),
			dirs:    []string{symlink, sdcard},
			failure: errors.New("permission denied"),
		},
		{
			name:    "no directories at all",
			item:    fontItem(),
			dirs:    nil,
			entries: map[string][]os.DirEntry{},
		},
		{
			name: "the tmux configuration under the name it was asked for",
			item: tmuxItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("kjv.txt"), file("tmux.conf")},
			},
			want: sdcard + "/tmux.conf",
		},
		{
			// MediaStore appends the extension Android's type table gives a
			// file published as plain text, which is why the item carries a
			// marker at all.
			name: "the tmux configuration under the name Android stored it as",
			item: tmuxItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("kjv.txt"), file("tmux.conf.txt")},
			},
			want: sdcard + "/tmux.conf.txt",
		},
		{
			name: "an unrelated file is not the tmux configuration",
			item: tmuxItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("kjv.txt"), file("gonomadnet")},
			},
		},
		{
			name: "the terminal colors under the name they were asked for",
			item: colorsItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("kjv.txt"), file("colors.properties")},
			},
			want: sdcard + "/colors.properties",
		},
		{
			// The same MediaStore rename the tmux configuration is exposed to, and
			// the reason this item carries a marker too.
			name: "the terminal colors under the name Android stored them as",
			item: colorsItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("kjv.txt"), file("colors.properties.txt")},
			},
			want: sdcard + "/colors.properties.txt",
		},
		{
			// The two published text files share no marker, so a downloads
			// directory holding only the tmux configuration must not answer with
			// it for the colors.
			name: "the tmux configuration is not the terminal colors",
			item: colorsItem(),
			dirs: []string{sdcard},
			entries: map[string][]os.DirEntry{
				sdcard: {file("kjv.txt"), file("tmux.conf"), file("tmux.conf.txt")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := findPublished(tt.dirs, tt.item, dirOf(tt.entries, tt.failure))
			if got != tt.want {
				t.Errorf("findPublished(%v, %v) = %q, want %q", tt.dirs, tt.item.name, got, tt.want)
			}
		})
	}
}

// TestPublishedDirs pins the order the downloads directories are tried in: the
// symlink termux-setup-storage creates first, because it is the only one that
// exists when the user ran that step, and the absolute paths after it.
func TestPublishedDirs(t *testing.T) {
	t.Parallel()

	home := "/data/data/com.termux/files/home"
	want := []string{
		filepath.Join(home, "storage", "downloads"),
		"/sdcard/Download",
		"/storage/emulated/0/Download",
	}

	got := publishedDirs(home)
	if len(got) != len(want) {
		t.Fatalf("publishedDirs(%q) = %v, want %v", home, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("publishedDirs(%q)[%v] = %q, want %q", home, i, got[i], want[i])
		}
	}
}

// TestAdopt covers the step between finding a published file and installing it.
func TestAdopt(t *testing.T) {
	t.Parallel()

	const sdcard = "/sdcard/Download"

	t.Run("installs what was found, and says where it came from", func(t *testing.T) {
		t.Parallel()
		dir := tempDir(t)
		listing := dirOf(map[string][]os.DirEntry{sdcard: {file("tmux.conf")}}, nil)

		var gotSource, gotTarget string
		install := func(source, target string) (bool, error) {
			gotSource, gotTarget = source, target
			return true, nil
		}

		source, installed, err := adopt(tmuxItem(), dir, listing, install)
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}
		if !installed {
			t.Fatal("adopt reported nothing installed")
		}
		if want := sdcard + "/tmux.conf"; source != want {
			t.Errorf("adopt reported the source %q, want %q", source, want)
		}
		if gotSource != source {
			t.Errorf("install was handed %q, want %q", gotSource, source)
		}
		if want := filepath.Join(dir, ".tmux.conf"); gotTarget != want {
			t.Errorf("install was handed the target %q, want %q", gotTarget, want)
		}
	})

	t.Run("nothing to adopt is not an error", func(t *testing.T) {
		t.Parallel()
		dir := tempDir(t)
		listing := dirOf(map[string][]os.DirEntry{sdcard: {file("kjv.txt")}}, nil)
		called := false

		source, installed, err := adopt(tmuxItem(), dir, listing, func(string, string) (bool, error) {
			called = true
			return true, nil
		})
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}
		if installed || source != "" {
			t.Errorf("adopt reported %q, %v, want nothing installed", source, installed)
		}
		if called {
			t.Error("adopt installed a file it never found")
		}
	})

	t.Run("a failure to install is reported", func(t *testing.T) {
		t.Parallel()
		dir := tempDir(t)
		listing := dirOf(map[string][]os.DirEntry{sdcard: {file("tmux.conf")}}, nil)

		_, installed, err := adopt(tmuxItem(), dir, listing, func(string, string) (bool, error) {
			return false, errors.New("no space left")
		})
		if err == nil {
			t.Error("adopt did not report a failed installation")
		}
		if installed {
			t.Error("adopt reported an installation that failed")
		}
	})
}

func TestPickName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		item  publishedItem
		names []string
		want  string
		ok    bool
	}{
		{
			name:  "the exact name",
			item:  tmuxItem(),
			names: []string{"kjv.txt", "tmux.conf"},
			want:  "tmux.conf",
			ok:    true,
		},
		{
			name:  "the name with the extension Android added",
			item:  tmuxItem(),
			names: []string{"tmux.conf.txt"},
			want:  "tmux.conf.txt",
			ok:    true,
		},
		{
			name:  "the exact name wins over the marker",
			item:  tmuxItem(),
			names: []string{"tmux.conf.txt", "tmux.conf"},
			want:  "tmux.conf",
			ok:    true,
		},
		{
			name:  "an item with no marker matches on its name alone",
			item:  publishedItem{name: "tmux.conf", relDest: ".tmux.conf"},
			names: []string{"tmux.conf.txt"},
		},
		{
			name:  "an empty directory",
			item:  tmuxItem(),
			names: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := pickName(tt.names, tt.item)
			if got != tt.want || ok != tt.ok {
				t.Errorf("pickName(%v, %v) = %q, %v, want %q, %v", tt.names, tt.item.name, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestInstallFile(t *testing.T) {
	t.Parallel()

	t.Run("copies a file that is not installed yet", func(t *testing.T) {
		t.Parallel()
		dir := tempDir(t)
		source := filepath.Join(dir, "published.ttf")
		if err := os.WriteFile(source, []byte("font bytes"), 0o644); err != nil {
			t.Fatalf("writing the source: %v", err)
		}
		target := filepath.Join(dir, ".termux", "font.ttf")

		installed, err := installFile(source, target)
		if err != nil {
			t.Fatalf("installFile: %v", err)
		}
		if !installed {
			t.Fatal("installFile reported nothing installed")
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("reading the installed file: %v", err)
		}
		if string(got) != "font bytes" {
			t.Errorf("installed %q, want %q", got, "font bytes")
		}
		// The staging file is renamed into place, so nothing is left beside it.
		if _, err := os.Stat(target + ".new"); !os.IsNotExist(err) {
			t.Errorf("a staging file was left behind: %v", err)
		}
	})

	t.Run("leaves an installed file alone", func(t *testing.T) {
		t.Parallel()
		dir := tempDir(t)
		source := filepath.Join(dir, "published.conf")
		if err := os.WriteFile(source, []byte("the appliance's configuration"), 0o644); err != nil {
			t.Fatalf("writing the source: %v", err)
		}
		target := filepath.Join(dir, ".tmux.conf")
		if err := os.WriteFile(target, []byte("a configuration somebody edited"), 0o644); err != nil {
			t.Fatalf("writing the installed file: %v", err)
		}

		installed, err := installFile(source, target)
		if err != nil {
			t.Fatalf("installFile: %v", err)
		}
		if installed {
			t.Error("installFile replaced a file that was already installed")
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("reading the installed file: %v", err)
		}
		if string(got) != "a configuration somebody edited" {
			t.Errorf("the installed file became %q", got)
		}
	})

	t.Run("reports a source that is not there", func(t *testing.T) {
		t.Parallel()
		dir := tempDir(t)
		if _, err := installFile(filepath.Join(dir, "absent.ttf"), filepath.Join(dir, "font.ttf")); err == nil {
			t.Error("installFile did not report a missing source")
		}
	})
}

func TestReloadTermuxSettings(t *testing.T) {
	t.Parallel()

	const prefix = "/data/data/com.termux/files/usr"

	t.Run("names the program by its absolute path", func(t *testing.T) {
		t.Parallel()
		env := envFrom(map[string]string{termuxPrefixEnv: prefix})

		var ran []string
		reloadTermuxSettings(env, func(name string, args ...string) error {
			ran = append(ran, name)
			return nil
		})

		want := prefix + "/bin/" + reloadSettingsProgram
		if len(ran) != 1 || ran[0] != want {
			t.Errorf("ran %v, want [%v]", ran, want)
		}
	})

	// A reload that fails is reported, not fatal: the font is installed either
	// way and the next start of Termux adopts it.
	t.Run("a failing reload is not fatal", func(t *testing.T) {
		t.Parallel()
		env := envFrom(map[string]string{termuxPrefixEnv: prefix})
		reloadTermuxSettings(env, func(string, ...string) error { return errors.New("no such program") })
	})
}
