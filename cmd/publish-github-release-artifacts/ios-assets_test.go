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
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stageIPA writes a packaged iOS appliance that is signed, which is what a release can carry.
//
// An .ipa is a zip, and what makes it signed is the provisioning profile inside it, so the
// stand-in is a zip holding exactly that: a file with the right name and nothing else would
// be the unsigned appliance this publisher must refuse.
func stageIPA(t *testing.T, dir, name string) string {
	t.Helper()
	return stageIPAWith(t, dir, name, true)
}

// stageUnsignedIPA writes a packaged appliance that carries no signature.
func stageUnsignedIPA(t *testing.T, dir, name string) string {
	t.Helper()
	return stageIPAWith(t, dir, name, false)
}

func stageIPAWith(t *testing.T, dir, name string, signed bool) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, name)

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = file.Close() }()

	archive := zip.NewWriter(file)
	entries := []string{"Payload/Gonomadnet.app/Gonomadnet", "Payload/Gonomadnet.app/Info.plist"}
	if signed {
		entries = append(entries, "Payload/Gonomadnet.app/embedded.mobileprovision")
	}
	for _, entry := range entries {
		w, err := archive.Create(entry)
		if err != nil {
			t.Fatalf("creating %v: %v", entry, err)
		}
		if _, err := w.Write([]byte("stage")); err != nil {
			t.Fatalf("writing %v: %v", entry, err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("closing the archive: %v", err)
	}
	return path
}

// TestCollectIOSArtifacts pins how the iOS appliance reaches a release: the publisher BUILDS
// it for the version being published and then consumes what the build staged in dist/ios.
func TestCollectIOSArtifacts(t *testing.T) {
	t.Parallel()

	const version = "0.164.0"

	t.Run("a staged appliance is attached", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		staging := filepath.Join(dir, "dist", "ios")
		want := stageIPA(t, staging, "gonomadnet-"+version+"-ios-arm64.ipa")
		progress, log := progressTo(t, dir)

		got := collectIOSArtifacts(staging, version, progress)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("collectIOSArtifacts = %v, want [%v]", got, want)
		}
		if !strings.Contains(log(), "Attaching the iOS appliance") {
			t.Errorf("the progress log does not say what it attached:\n%v", log())
		}
	})

	t.Run("nothing staged is a warning and not a failure", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		missing := filepath.Join(dir, "dist", "ios")
		progress, log := progressTo(t, dir)

		got := collectIOSArtifacts(missing, version, progress)
		if len(got) != 0 {
			t.Fatalf("collectIOSArtifacts = %v, want nothing", got)
		}
		if !strings.Contains(log(), "will not carry one") {
			t.Errorf("a missing staging directory was not explained:\n%v", log())
		}
		if !strings.Contains(log(), "build-ios-app.sh") {
			t.Errorf("the log does not say how to stage one:\n%v", log())
		}
	})

	t.Run("another version's appliance is not this release's", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		staging := filepath.Join(dir, "dist", "ios")
		stageIPA(t, staging, "gonomadnet-0.163.0-ios-arm64.ipa")
		progress, log := progressTo(t, dir)

		if got := collectIOSArtifacts(staging, version, progress); len(got) != 0 {
			t.Fatalf("collectIOSArtifacts = %v, want nothing: that IPA is another version's", got)
		}
		if strings.Contains(log(), "Attaching") {
			t.Errorf("a leftover from another release was announced as attached:\n%v", log())
		}
	})

	t.Run("an unsigned appliance is not this release's to publish", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		staging := filepath.Join(dir, "dist", "ios")
		stageUnsignedIPA(t, staging, "gonomadnet-"+version+"-ios-arm64.ipa")
		progress, log := progressTo(t, dir)

		if got := collectIOSArtifacts(staging, version, progress); len(got) != 0 {
			t.Fatalf("collectIOSArtifacts = %v, want nothing: an .ipa with no signature installs on nothing", got)
		}
		if !strings.Contains(log(), "is not signed") {
			t.Errorf("the log does not say why it was passed over:\n%v", log())
		}
		if !strings.Contains(log(), "build-ios-app.sh") {
			t.Errorf("the log does not say how to sign one:\n%v", log())
		}
	})

	t.Run("this release's appliance is picked out of a directory full of leftovers", func(t *testing.T) {
		t.Parallel()

		dir := tempDir(t)
		staging := filepath.Join(dir, "dist", "ios")
		stageIPA(t, staging, "gonomadnet-0.163.0-ios-arm64.ipa")
		want := stageIPA(t, staging, "gonomadnet-"+version+"-ios-arm64.ipa")
		progress, _ := progressTo(t, dir)

		got := collectIOSArtifacts(staging, version, progress)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("collectIOSArtifacts = %v, want [%v]", got, want)
		}
	})
}

// TestReleaseNotesDescribeTheIOSAppliance asserts that a release carrying an IPA says what it
// is and what a reader can do with it.
//
// The wording matters more here than anywhere else in the notes: an app that is signed for
// someone else's team installs on none of a reader's devices, and a reader who downloads it
// expecting an app and finds "Unable to Install" has been told the appliance exists and given
// no way to have it.
func TestReleaseNotesDescribeTheIOSAppliance(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	binary := filepath.Join(dir, "gonomadnet-0.164.0-linux-amd64")
	if err := os.WriteFile(binary, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ipa := stageIPA(t, filepath.Join(dir, "dist", "ios"), "gonomadnet-0.164.0-ios-arm64.ipa")
	apk := stageAPK(t, filepath.Join(dir, "dist", "android"), "gonomadnet-0.164.0-android-arm64-v8a.apk")

	t.Run("with an appliance", func(t *testing.T) {
		t.Parallel()

		notes := buildReleaseNotes("0.164.0", "gmlewis/go-nomadnet", []string{binary, ipa})
		for _, want := range []string{
			"iOS appliance",
			"gonomadnet-0.164.0-ios-arm64.ipa",
			"docs/iOS-Build.md",
			"docs/iOS.md",
			// The three facts a reader needs and cannot guess: it is one process, Bluetooth is
			// how a radio is reached, and the IPA is signed for a team whose devices are not
			// theirs, so running it means building it.
			"no way to start a program",
			"Bluetooth LoRa radio",
			"carries the maintainer's Apple Development signature",
			"signing it with your own " +
				"developer account",
		} {
			if !strings.Contains(notes, want) {
				t.Errorf("the release notes do not mention %q:\n%v", want, notes)
			}
		}
	})

	t.Run("without one", func(t *testing.T) {
		t.Parallel()

		notes := buildReleaseNotes("0.164.0", "gmlewis/go-nomadnet", []string{binary})
		if strings.Contains(notes, "iOS appliance") {
			t.Errorf("the notes describe an appliance that is not attached:\n%v", notes)
		}
	})

	// Both appliances can be described at once, and each keeps its own section: a release that
	// carried both must not describe one in terms of the other.
	t.Run("with both", func(t *testing.T) {
		t.Parallel()

		notes := buildReleaseNotes("0.164.0", "gmlewis/go-nomadnet", []string{binary, ipa, apk})
		for _, want := range []string{"Android appliance", "iOS appliance", "adb install -r"} {
			if !strings.Contains(notes, want) {
				t.Errorf("the release notes do not mention %q:\n%v", want, notes)
			}
		}
	})
}

// TestBuildIOSApplianceRunsTheBuildScript asserts the publisher builds the appliance rather
// than looking for a staged file.
func TestBuildIOSApplianceRunsTheBuildScript(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	marker := filepath.Join(dir, "built")
	script := filepath.Join(dir, "build-ios-app.sh")
	body := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	progress, _ := progressTo(t, dir)

	if err := buildIOSAppliance(script, progress); err != nil {
		t.Fatalf("buildIOSAppliance: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the build script was not run: %v", err)
	}

	if err := buildIOSAppliance(filepath.Join(dir, "nope.sh"), progress); err == nil {
		t.Error("buildIOSAppliance accepted a script that does not exist")
	}
}
