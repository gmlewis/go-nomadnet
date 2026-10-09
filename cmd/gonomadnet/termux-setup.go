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
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// termuxVersionEnv is the environment variable Termux's login shell exports.
	termuxVersionEnv = "TERMUX_VERSION"

	// termuxPrefixEnv is the environment variable naming Termux's own filesystem
	// root, which is a path inside Termux's private data directory.
	termuxPrefixEnv = "PREFIX"

	// termuxPrefixMarker is the part of that path that identifies Termux. A
	// desktop has a PREFIX too — Homebrew sets it — so the marker is what tells
	// the two apart.
	termuxPrefixMarker = "com.termux"

	// setupOptOutEnv, when set to anything, stops anything being installed.
	setupOptOutEnv = "GONOMADNET_NO_TERMUX_SETUP"

	// reloadSettingsProgram reloads Termux's settings in the running terminal, so
	// a font installed under it is adopted without the user restarting the app.
	reloadSettingsProgram = "termux-reload-settings"
)

// readDirFunc lists a directory, as os.ReadDir does.
type readDirFunc func(string) ([]os.DirEntry, error)

// installFunc copies a published file to where Termux reads it, reporting
// whether it did. A false return with a nil error means it declined.
type installFunc func(source, target string) (bool, error)

// publishedItem is one file the Android appliance publishes into shared storage
// and the client installs into Termux's own configuration.
type publishedItem struct {
	// what names the file in the line that reports an installation.
	what string

	// name is the name the appliance publishes it under.
	name string

	// marker identifies the file when it is published under some other name.
	// Empty means the name has to match exactly.
	marker string

	// relDest is where the file goes, below the home directory.
	relDest string

	// reload is set when Termux has to re-read its settings to adopt the file.
	reload bool
}

// publishedItems is every file the client installs this way, in the order they
// are tried.
//
// The font's published name is the font's own with the ".ttf" Android's type
// table appends to a file published as OpenType; the marker is what adopts it
// when a device stores it under some other name.
var publishedItems = []publishedItem{
	{
		what:    "terminal font",
		name:    "AtkynsonMonoNerdFontMono-Regular.otf.ttf",
		marker:  "NerdFont",
		relDest: filepath.Join(".termux", "font.ttf"),
		reload:  true,
	},
	{
		// The published name carries the ".txt" Android appends to a file
		// published as plain text, so the marker is what finds it there.
		what:    "tmux configuration",
		name:    "tmux.conf",
		marker:  "tmux.conf",
		relDest: ".tmux.conf",
	},
	{
		what:    "terminal colors",
		name:    "colors.properties",
		marker:  "colors.properties",
		relDest: filepath.Join(".termux", "colors.properties"),
		reload:  true,
	},
}

// adoptTermuxSetup installs into Termux's own configuration the files the
// Android appliance published, so that this program draws into a terminal that
// can render its glyphs, is colored as the desktop's terminal is, and — when
// tmux is used — is configured as on a desktop.
//
// The appliance carries those files but cannot install them. Termux reads its
// terminal font and its colors from `~/.termux/` and tmux reads its settings from
// `~/.tmux.conf`, all inside Termux's own private data directory, and Android
// gives one application no way at all to write into another's — so the appliance
// publishes them into shared storage and this moves them the last step. This is
// the only process of the pair that runs inside Termux, and the only one that can.
//
// It is conservative by design. A file that is already installed is never
// replaced, so a person who chose their own terminal font, colors or tmux
// configuration keeps them, and a file that is not there to adopt is not a
// complaint: the appliance may not have published it yet, or shared storage may
// not be set up. The only lines it writes describe files it has just installed.
func adoptTermuxSetup() {
	if !inTermux(os.Getenv) || os.Getenv(setupOptOutEnv) != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	for _, item := range publishedItems {
		source, installed, err := adopt(item, home, os.ReadDir, installFile)
		if err != nil {
			log.Printf("gonomadnet: could not install the %v: %v", item.what, err)
			continue
		}
		if !installed {
			continue
		}
		log.Printf("gonomadnet: installed the %v %v as %v", item.what, source, filepath.Join(home, item.relDest))
		if item.reload {
			reloadTermuxSettings(os.Getenv, execProgram)
		}
	}
}

// adopt installs one published item, reporting where it came from and whether it
// was installed. Nothing to adopt is not an error: it answers with an empty
// source and no installation.
func adopt(item publishedItem, home string, readDir readDirFunc, install installFunc) (string, bool, error) {
	source := findPublished(publishedDirs(home), item, readDir)
	if source == "" {
		return "", false, nil
	}
	installed, err := install(source, filepath.Join(home, item.relDest))
	return source, installed, err
}

// inTermux reports whether this process is running inside Termux, which is the
// only place Termux's configuration is this program's to install.
//
// Termux's login shell exports TERMUX_VERSION and points PREFIX inside its own
// package directory. Either one is enough: a program started from a shortcut has
// both, and a program started by the Termux:Widget runner has at least the
// prefix. A desktop sets neither, which is what keeps this from reaching for
// files a desktop never published.
func inTermux(env func(string) string) bool {
	return env(termuxVersionEnv) != "" ||
		strings.Contains(env(termuxPrefixEnv), termuxPrefixMarker)
}

// publishedDirs names the places the appliance's published files can be, in the
// order they are tried.
//
// The appliance writes them into the shared Downloads collection, which a Termux
// install reaches either through the symlink `termux-setup-storage` creates or
// by its own path. All three spellings are listed, because which one works
// depends on whether that setup step was ever run and on the Android version.
func publishedDirs(home string) []string {
	return []string{
		filepath.Join(home, "storage", "downloads"),
		"/sdcard/Download",
		"/storage/emulated/0/Download",
	}
}

// findPublished returns the first of dirs that holds the published item, or the
// empty string when none does.
//
// The name the appliance asked MediaStore to store the file under is preferred,
// and for the font any other Nerd Font is the fallback, so a font published by a
// newer appliance — or by hand, for a device whose Android renamed it
// differently — is still adopted. A directory that cannot be read is skipped:
// the downloads symlink is absent until `termux-setup-storage` runs, and that is
// an expected state and not a failure to report.
func findPublished(dirs []string, item publishedItem, readDir readDirFunc) string {
	for _, dir := range dirs {
		entries, err := readDir(dir)
		if err != nil {
			continue
		}
		if name, ok := pickName(dirEntryNames(entries), item); ok {
			return filepath.Join(dir, name)
		}
	}
	return ""
}

// dirEntryNames returns the names in entries, dropping the directories: a
// published file is a file and a directory of that name is not one.
func dirEntryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// pickName chooses the file to adopt out of the names in a directory. The exact
// name the appliance publishes wins; otherwise the first name carrying the
// item's marker, in ReadDir's lexical order, which is stable across runs. An
// item with no marker matches on its name alone.
func pickName(names []string, item publishedItem) (string, bool) {
	var fallback string
	for _, name := range names {
		if name == item.name {
			return name, true
		}
		if fallback == "" && item.marker != "" && strings.Contains(name, item.marker) {
			fallback = name
		}
	}
	return fallback, fallback != ""
}

// installFile copies source to target and reports whether it did.
//
// It declines when target already holds a file. That is the whole of the safety
// rule: this exists to give a terminal that has no Nerd Font one, a terminal
// still wearing its stock colors the desktop's theme, and a tmux that has no
// configuration one, and a file that is already there has been put there by
// somebody who meant it — including, in tmux's case, the person using the
// tablet, who may well have edited it.
//
// The copy lands through a neighbouring temporary file that is renamed into
// place, so an interrupted install cannot leave a half-written font where Termux
// will read it or a half-written configuration where tmux will.
func installFile(source, target string) (bool, error) {
	if _, err := os.Stat(target); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, err
	}
	in, err := os.Open(source)
	if err != nil {
		return false, err
	}
	defer func() { _ = in.Close() }()

	staging := target + ".new"
	// The staging file is cleared on every path that does not end in the rename
	// below, so a failure part way through leaves nothing where Termux or tmux
	// would read a half-written file. Once the rename succeeds there is nothing
	// left to remove and the call is a no-op.
	defer func() { _ = os.Remove(staging) }()

	out, err := os.Create(staging)
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return false, err
	}
	if err := out.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(staging, target); err != nil {
		return false, err
	}
	return true, nil
}

// reloadTermuxSettings asks Termux to re-read its settings, which is what makes
// the terminal adopt a newly installed font or color scheme without the user
// restarting the application.
//
// The program is named by its absolute path because Android's seccomp policy
// kills a process on the system call Go's exec.LookPath issues while resolving an
// unqualified name. A failure here is reported and never fatal: the font is
// installed either way, and the next start of Termux picks it up regardless.
func reloadTermuxSettings(env func(string) string, run func(string, ...string) error) {
	program := filepath.Join(env(termuxPrefixEnv), "bin", reloadSettingsProgram)
	if err := run(program); err != nil {
		log.Printf("gonomadnet: installed the terminal settings, but Termux did not reload them: %v", err)
	}
}

// execProgram runs a program to completion, discarding its output. The reload is
// a side effect with nothing to read back.
func execProgram(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %w", name, err)
	}
	return nil
}
