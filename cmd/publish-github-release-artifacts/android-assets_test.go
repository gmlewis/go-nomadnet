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
			// The appliance's whole reason for existing is that nobody has to assemble a
			// terminal environment by hand, so the notes have to say what it does instead:
			// it carries the client and the console host inside the APK, and what is left
			// is two buttons.
			"nothing else to install",
			"carries both the `linux/arm64` client",
			"gives it a pseudo-terminal",
		} {
			if !strings.Contains(notes, want) {
				t.Errorf("the release notes do not mention %q:\n%v", want, notes)
			}
		}
		// The appliance stopped needing another app to run the client, and a reader told to
		// install one has been given work that does not exist and a reason to think the
		// appliance is something other than one APK.
		if strings.Contains(notes, "Termux") {
			t.Errorf("the notes still send the reader to another app:\n%v", notes)
		}
		// The cross-repo pin is the whole reason for the version sentence: the APK embeds
		// daemons that are not built from this repository at all. The check is on the phrase
		// alone, without the "v" the sentence puts in front of it, because a version the
		// reader cannot use renders as "van unrecorded version" — which is how this went
		// unnoticed: the guard matched a string the notes never contained.
		if strings.Contains(notes, "unrecorded version") {
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

// TestReadGoReticulumVersion asserts that the version the notes quote is the pin the release
// actually ships, so the cross-repo pin cannot silently drift.
//
// It compares against go.mod, and it does not skip when a sibling checkout is absent. CI
// checks out this repository and nothing else, so a sibling is exactly the thing the test
// could not have: the version it read was the developer's workspace's, the notes said
// "van unrecorded version" on every published release, and this test passed by skipping on
// the one machine where the mistake was invisible.
func TestReadGoReticulumVersion(t *testing.T) {
	t.Parallel()

	pinned := pinnedGoReticulumVersion(t)
	if got := readGoReticulumVersion(); got != pinned {
		t.Errorf("readGoReticulumVersion = %q, but go.mod pins %q", got, pinned)
	}
	// The sentence in the notes supplies no "v" of its own, so what is read has to carry it.
	if !strings.HasPrefix(pinned, "v") {
		t.Errorf("go.mod pins go-reticulum as %q, which is not the form a module version takes", pinned)
	}
}

// pinnedGoReticulumVersion is the version go.mod requires, read here independently of the
// reader under test.
func pinnedGoReticulumVersion(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(repoFile(goModFile))
	if err != nil {
		t.Fatalf("reading %v: %v", goModFile, err)
	}
	for raw := range strings.SplitSeq(string(data), "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(raw), goReticulumModule+" "); ok {
			if fields := strings.Fields(after); len(fields) > 0 {
				return fields[0]
			}
		}
	}
	t.Fatalf("%v does not require %v", goModFile, goReticulumModule)
	return ""
}

// TestGoReticulumVersionInReadsOnlyThePin asserts the reader against a go.mod written here,
// so that the shape it is given is not whatever this repository's happens to be.
func TestGoReticulumVersionInReadsOnlyThePin(t *testing.T) {
	t.Parallel()

	const written = `module github.com/gmlewis/go-nomadnet

go 1.26.4

require (
	github.com/creack/pty/v2 v2.0.1
	github.com/gmlewis/go-reticulum v0.139.0
)

replace github.com/gmlewis/go-reticulum => ../go-reticulum
`
	if got, want := goReticulumVersionIn(written), "v0.139.0"; got != want {
		t.Errorf("goReticulumVersionIn = %q, want %q", got, want)
	}
	// A module that is not required at all is not a version. Reading the replace target, or
	// the module's own path line, would be a version this release did not ship.
	if got := goReticulumVersionIn("module github.com/gmlewis/go-reticulum\n"); got != unrecordedVersion {
		t.Errorf("goReticulumVersionIn = %q, want the placeholder", got)
	}
	if got := goReticulumVersionIn(""); got != unrecordedVersion {
		t.Errorf("goReticulumVersionIn = %q, want the placeholder", got)
	}
}
