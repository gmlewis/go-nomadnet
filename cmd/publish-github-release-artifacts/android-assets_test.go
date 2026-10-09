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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// tempDir returns a short-lived directory under /tmp. t.TempDir() is unusable on
// macOS in this repository: its path is too long for the Unix domain sockets
// Reticulum binds.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "publish-android-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// progressTo captures what the publisher says to its progress stream.
func progressTo(t *testing.T, dir string) (*os.File, func() string) {
	t.Helper()
	path := filepath.Join(dir, "progress.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f, func() string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		return string(data)
	}
}

// stageAPK writes a file that stands in for a signed appliance.
func stageAPK(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("not really an APK"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestCollectAndroidArtifacts pins how the appliance reaches a release: it is consumed from a
// staging directory where scripts/build-android-apk.sh left it, and never built here.
func TestCollectAndroidArtifacts(t *testing.T) {
	t.Parallel()

	const version = "0.164.0"

	t.Run("a staged appliance is attached", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		staging := filepath.Join(dir, "dist", "android")
		want := stageAPK(t, staging, "gonomadnet-"+version+"-android-arm64-v8a.apk")
		progress, log := progressTo(t, dir)

		got := collectAndroidArtifacts(staging, version, progress)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("collectAndroidArtifacts = %v, want [%v]", got, want)
		}
		if !strings.Contains(log(), "Attaching the Android appliance") {
			t.Errorf("the progress log does not say what it attached:\n%v", log())
		}
	})

	t.Run("nothing staged is a warning and not a failure", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		missing := filepath.Join(dir, "dist", "android")
		progress, log := progressTo(t, dir)

		got := collectAndroidArtifacts(missing, version, progress)
		if len(got) != 0 {
			t.Fatalf("collectAndroidArtifacts = %v, want nothing", got)
		}
		// The point of the wording: a release with no appliance is a normal release, and the
		// person reading the log should be told how to make one rather than left wondering.
		if !strings.Contains(log(), "will not carry one") {
			t.Errorf("a missing staging directory was not explained:\n%v", log())
		}
		if !strings.Contains(log(), "build-android-apk.sh") {
			t.Errorf("the log does not say how to stage one:\n%v", log())
		}
	})

	t.Run("an empty staging directory is also a warning", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		staging := filepath.Join(dir, "dist", "android")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		progress, log := progressTo(t, dir)

		if got := collectAndroidArtifacts(staging, version, progress); len(got) != 0 {
			t.Fatalf("collectAndroidArtifacts = %v, want nothing", got)
		}
		if !strings.Contains(log(), "will not carry one") {
			t.Errorf("an empty staging directory was not explained:\n%v", log())
		}
	})

	t.Run("an appliance from another version is refused", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		staging := filepath.Join(dir, "dist", "android")
		stageAPK(t, staging, "gonomadnet-0.163.0-android-arm64-v8a.apk")
		progress, log := progressTo(t, dir)

		got := collectAndroidArtifacts(staging, version, progress)
		if len(got) != 0 {
			t.Fatalf("collectAndroidArtifacts attached a stale appliance: %v", got)
		}
		// A release that shipped an appliance built from other sources would be worse than
		// one that shipped none, because nothing about the download would say so.
		if !strings.Contains(log(), "does not carry version") {
			t.Errorf("a stale appliance was not explained:\n%v", log())
		}
	})

	t.Run("files that are not the appliance are ignored", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		staging := filepath.Join(dir, "dist", "android")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		for _, name := range []string{"README.md", "notes.txt", "app-debug.apk", "gonomadnet-" + version + "-android-arm64-v8a.apk.sha256"} {
			if err := os.WriteFile(filepath.Join(staging, name), []byte("x"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
		}
		progress, _ := progressTo(t, dir)

		// "app-debug.apk" is a debug build someone left behind: it is an APK but it is not
		// one of ours, and attaching it would publish an unsigned appliance under a released
		// version's name.
		if got := collectAndroidArtifacts(staging, version, progress); len(got) != 0 {
			t.Fatalf("collectAndroidArtifacts = %v, want nothing", got)
		}
	})
}

// TestThePruneNeverDeletesAnAppliance is the retention guard. The appliance is an install
// artifact, not a build output: the binaries in the platform pattern are rebuilt for every
// release, so retiring one costs a download, while an APK is a signed thing somebody installed
// once and must be able to fetch again to upgrade in place.
func TestThePruneNeverDeletesAnAppliance(t *testing.T) {
	t.Parallel()

	rel := publishedRelease{
		tag: "v0.151.0",
		assets: []asset{
			{name: "gonomadnet-0.151.0-linux-amd64", id: 1},
			{name: "gonomadnet-0.151.0-android-arm64-v8a.apk", id: 2},
			{name: "gonomadnet-0.151.0-android-arm64-v8a.APK", id: 3},
		},
	}
	deletable, skipped := partitionAssets(rel, assetNamePattern("0.151.0"))

	if len(deletable) != 1 || deletable[0].name != "gonomadnet-0.151.0-linux-amd64" {
		t.Fatalf("deletable = %v, want only the platform binary", deletable)
	}
	for _, name := range []string{"gonomadnet-0.151.0-android-arm64-v8a.apk", "gonomadnet-0.151.0-android-arm64-v8a.APK"} {
		found := false
		for _, s := range skipped {
			if s == name {
				found = true
			}
		}
		if !found {
			t.Errorf("%v was not reported as skipped, so the prune believes it may delete it", name)
		}
	}
}

// TestAnApplianceWouldSurviveAPlatformPatternThatMatchedIt asserts the exemption is explicit
// rather than incidental. The platform pattern does not match an APK's name today, so the
// appliance is already safe — and that is exactly why this is written down: the next person to
// add "android" to that pattern, which is a natural thing to do, must not have to notice this
// guard to avoid destroying every appliance ever released.
func TestAnApplianceWouldSurviveAPlatformPatternThatMatchedIt(t *testing.T) {
	t.Parallel()

	rel := publishedRelease{
		tag: "v0.151.0",
		assets: []asset{
			{name: "gonomadnet-0.151.0-linux-amd64", id: 1},
			{name: "gonomadnet-0.151.0-android-arm64-v8a.apk", id: 2},
		},
	}
	// A pattern greedy enough to match the appliance's name, which is what a future
	// "and android too" edit would produce.
	greedy := regexp.MustCompile(`^gonomadnet-0\.151\.0-.*$`)

	deletable, skipped := partitionAssets(rel, greedy)
	if len(deletable) != 1 || deletable[0].name != "gonomadnet-0.151.0-linux-amd64" {
		t.Fatalf("deletable = %v, want only the platform binary even under a greedy pattern", deletable)
	}
	if len(skipped) != 1 || skipped[0] != "gonomadnet-0.151.0-android-arm64-v8a.apk" {
		t.Fatalf("skipped = %v, want the appliance", skipped)
	}
}

// TestReleaseNotesDescribeTheAndroidAppliance asserts the notes say what the APK is, which
// go-reticulum it carries, and where to read before installing it. An artifact that arrives
// with no explanation is an artifact nobody can trust.
func TestReleaseNotesDescribeTheAndroidAppliance(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	binary := filepath.Join(dir, "gonomadnet-0.164.0-linux-amd64")
	if err := os.WriteFile(binary, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	apk := stageAPK(t, filepath.Join(dir, "dist", "android"), "gonomadnet-0.164.0-android-arm64-v8a.apk")

	t.Run("with an appliance", func(t *testing.T) {
		t.Parallel()

		notes := buildReleaseNotes("0.164.0", "gmlewis/go-nomadnet", []string{binary, apk})
		for _, want := range []string{
			"Android appliance",
			"gonomadnet-0.164.0-android-arm64-v8a.apk",
			"go-reticulum v",
			"adb install -r",
			"docs/Android-APK.md",
			// The appliance's whole reason for existing is that nobody hand-assembles a
			// Termux install, so the notes have to say so: it installs Termux itself,
			// carries the client inside the APK, and publishes the launchers.
			"installs the correct Termux build",
			"carries its own `linux/arm64` client inside the APK",
			"publishes the client and the launchers into Downloads",
		} {
			if !strings.Contains(notes, want) {
				t.Errorf("the release notes do not mention %q:\n%v", want, notes)
			}
		}
		// A reader who is told to move a downloaded client into Termux by hand has been
		// given work the app already does, and a reason to think the app does not do it.
		if strings.Contains(notes, "moving the `linux-arm64` client") {
			t.Errorf("the notes still describe a hand-assembled Termux install:\n%v", notes)
		}
		// The cross-repo pin is the whole reason for the version sentence: the APK embeds
		// daemons that are not built from this repository at all.
		if strings.Contains(notes, "go-reticulum v an unrecorded version") {
			t.Errorf("the go-reticulum version was not read from the sibling module:\n%v", notes)
		}
	})

	t.Run("without one", func(t *testing.T) {
		t.Parallel()

		notes := buildReleaseNotes("0.164.0", "gmlewis/go-nomadnet", []string{binary})
		if strings.Contains(notes, "Android appliance") {
			t.Errorf("the notes describe an appliance that is not attached:\n%v", notes)
		}
	})
}

// TestReadGoReticulumVersion asserts the version that the notes quote is the sibling module's
// own, so the cross-repo pin cannot silently drift.
func TestReadGoReticulumVersion(t *testing.T) {
	t.Parallel()

	got := readGoReticulumVersion()
	if got == "an unrecorded version" {
		t.Skipf("%v is not present beside this repository, so there is nothing to compare", goReticulumVersionFile)
	}
	if !strings.HasPrefix(got, "0.") {
		t.Fatalf("readGoReticulumVersion = %q, want a version like 0.137.0", got)
	}
}
