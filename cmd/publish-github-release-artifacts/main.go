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

// Command publish-github-release-artifacts builds standalone executables for
// the major supported platforms/targets and publishes them as assets of a
// GitHub Release (via the gh CLI) tagged with the current version string read
// from nomadnet/version/version.go.
//
// Tagging happens FIRST: the version's git tag is created and pushed before the
// (slow) build/upload, so downstream modules can `go mod tidy` against the tag
// immediately. An existing tag is never modified.
//
// If a release for that version already exists, the command fails unless
// --force is supplied. --force deletes the existing GitHub Release (the
// release page and its uploaded asset binaries) and recreates it with freshly
// built artifacts — but it does NOT touch the git tag. Keeping the tag
// immutable means the Go module proxy (proxy.golang.org / pkg.go.dev) checksum
// for the tagged source never changes, so `go mod tidy` / `go get` in downstream
// modules never fails with a "verifying ... checksum mismatch" ("hacker
// modifying a known tagged release") error, while the published binaries can
// still be refreshed. The release notes embed a sha256 checksum table for
// every uploaded artifact.
//
// Pruning happens LAST, after a successful publish: the assets of every release
// older than the newest --keep-releases (default 10) releases that still have
// assets are deleted. Releases, release notes, and git tags are never touched —
// only the uploaded binaries — so module checksums and changelog links are
// unaffected while release-asset storage stops growing without bound. Use
// --prune-only to run just that step, e.g. --prune-only --dry-run to preview
// exactly what an unattended publish would delete.
//
// Usage:
//
//	publish-github-release-artifacts [--force] [-n|--dry-run] [--prune-only] [--keep-releases N]
//
// This program is normally driven by scripts/publish-github-release-artifacts.sh.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// versionFile is parsed (not imported) so the publisher always reads whatever
// version string currently sits in the working tree, with no build cache that
// could serve a stale value.
const versionFile = "nomadnet/version/version.go"

// target is a single GOOS/GOARCH build target.
type target struct {
	goos, goarch string
}

// majorTargets lists the major platforms we ship release artifacts for, each
// built for both amd64 and arm64. Windows gets the .exe suffix; everything
// else is a bare executable.
var majorTargets = []target{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
	{"freebsd", "amd64"},
	{"freebsd", "arm64"},
}

// hwTarget describes a build target tailored for a specific hardware form factor,
// including its target architecture, Go build tags, and descriptive identifier.
type hwTarget struct {
	goos, goarch string
	formFactor   string // "pocket_terminal", "pocket_communicator"
	buildTags    string // "pocket_terminal", "pocket_communicator"
	armVersion   string // "7" for GOARM=7
}

// hardwareTargets lists pre-built firmware/binaries for DIY handheld targets.
var hardwareTargets = []hwTarget{
	// Form Factor A (Pocket Linux Terminal): RPi Zero 2W / SBC with SPI LCD & CardKB
	{"linux", "arm64", "pocket_terminal", "pocket_terminal", ""},
	{"linux", "arm", "pocket_terminal", "pocket_terminal", "7"},
	{"linux", "riscv64", "pocket_terminal", "pocket_terminal", ""},

	// Form Factor B (Pocket Communicator): Standalone Communicator / Daemon
	{"linux", "arm64", "pocket_communicator", "pocket_communicator", ""},
	{"linux", "arm", "pocket_communicator", "pocket_communicator", "7"},
	{"linux", "riscv64", "pocket_communicator", "pocket_communicator", ""},
}

// binaryName is the published artifact's base name.
const binaryName = "gonomadnet"

// scratchDirPrefix names the build scratch dir os.MkdirTemp creates under /tmp.
// It deliberately begins with no prefix scripts/clean-test-tmp.sh sweeps: a
// "gonomadnet-release-" prefix sat inside that script's swept "gonomadnet-"
// family, so a concurrent test run's sweep deleted the artifacts out from under
// a publish mid-flight, and because go build quietly recreates a missing output
// directory nothing failed until gh reported a missing file. scratch_test.go
// enforces the guarantee against the script's live prefix lists, and
// not_swept_prefixes there names this prefix so -c keeps it that way.
const scratchDirPrefix = "publish-release-gonomadnet-*"

// The Android appliance, which reaches a release as a staged file rather than as a build.
const (
	// androidStagingDir is where scripts/build-android-apk.sh leaves a signed APK.
	//
	// This publisher never builds one. Assembling an APK needs a signing key, an Android SDK
	// and four cross-compiled Go daemons, and a release path that failed without them would
	// break the releases of a repository whose whole purpose is a Go program. The appliance is
	// therefore built and signed beforehand, and consumed from here.
	androidStagingDir = "dist/android"
	// androidAssetPrefix and androidAssetSuffix name what the publisher will attach:
	// gonomadnet-<version>-android-<abi>.apk.
	androidAssetPrefix = "gonomadnet-"
	androidAssetSuffix = ".apk"
	// goReticulumVersionFile is the sibling module's version, which the APK's daemons are
	// compiled from. A release note that names it is the only record of the cross-repo pin.
	// It is relative to the repository root, like every other path here; repoFile is what
	// makes that true from anywhere.
	goReticulumVersionFile = "../go-reticulum/rns/version.go"
)

// repoFile resolves a path named relative to the repository root.
//
// The publisher is run from the repository root, so its paths are written relative to it.
// The unit tests are not run from there: `go test` gives every test the package's own
// directory as its working directory, so a relative path that is right for the publisher is
// wrong for the test that checks what the publisher would write — and a test that cannot
// read what it is asserting on is a test that passes for the wrong reason. The root is
// found by walking up to the directory holding go.mod, which is the same directory in both
// cases; a path that already resolves is left alone, and one that never resolves is
// returned unchanged so its caller reports the path it actually tried.
func repoFile(rel string) string {
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	dir, err := os.Getwd()
	if err != nil {
		return rel
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return rel
		}
		dir = parent
	}
}

// inUseMarker is the file markScratchInUse drops inside the scratch dir. It
// names this process's PID, and scripts/clean-test-tmp.sh leaves a directory
// carrying it alone while that PID lives — the second line of defence behind the
// prefix above, and the only one that helps a scratch dir whose name a future
// sweeper prefix happens to match.
const inUseMarker = ".do-not-sweep"

// options gathers the command's knobs so run's signature stays readable.
type options struct {
	force        bool // replace an existing release for this version
	dryRun       bool // make no remote changes: no tag, no publish, no deletes
	pruneOnly    bool // prune old release assets and do nothing else
	keepReleases int  // release-asset retention window (see prune.go)
}

func main() {
	force := flag.Bool("force", false,
		"replace an existing release for this version (deletes previous assets)")
	dryRun := flag.Bool("n", false,
		"print the full Markdown release description that would be written to "+
			"stdout and exit, without publishing or deleting anything (artifacts "+
			"are still built so the sha256 checksums are real)")
	flag.BoolVar(dryRun, "dry-run", false, "alias for -n")
	pruneOnly := flag.Bool("prune-only", false,
		"prune old release assets only: skip the build, the tag, and the publish")
	keepReleases := flag.Int("keep-releases", defaultKeepReleases,
		"keep the assets of this many of the newest releases; every older release's "+
			"assets are pruned (releases and tags are never deleted)")
	flag.Usage = func() {
		log.Printf("Usage: %v [--force] [-n|--dry-run] [--prune-only] [--keep-releases N]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	opts := options{
		force:        *force,
		dryRun:       *dryRun,
		pruneOnly:    *pruneOnly,
		keepReleases: *keepReleases,
	}
	if err := run(opts); err != nil {
		log.Printf("publish-github-release-artifacts: %v\n", err)
		os.Exit(1)
	}
}

// run builds the release artifacts and either publishes them as a new GitHub
// release (when opts.dryRun is false) or prints the Markdown release description
// that would be written to stdout and returns (when opts.dryRun is true). In
// dry-run mode all progress output is routed to stderr so stdout contains only
// the Markdown description. Either way, the last step is pruning the assets of
// releases that have fallen out of the retention window (see prune.go).
func run(opts options) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found in PATH: %w", err)
	}

	// progress writes build/publish chatter; in dry-run mode it goes to stderr
	// so stdout stays a clean Markdown document.
	progress := os.Stdout
	if opts.dryRun {
		progress = os.Stderr
	}

	// --prune-only touches neither the working tree nor the artifacts, so it
	// skips the toolchain check, the build, the tag, and the publish entirely.
	if opts.pruneOnly {
		return pruneReleaseAssets(opts.keepReleases, opts.dryRun, progress)
	}

	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	version, err := readVersion()
	if err != nil {
		return err
	}
	tag := "v" + version
	mustFprintf(progress, "Publishing release for version %v (tag %v)\n", version, tag)

	repo, err := ghRepoSlug()
	if err != nil {
		return err
	}
	mustFprintf(progress, "Repository: %v\n", repo)

	exists := false
	hasTag := false
	if !opts.dryRun {
		exists, err = releaseExists(tag)
		if err != nil {
			return err
		}
		if exists && !opts.force {
			return fmt.Errorf(
				"release %v already exists; run scripts/bump-minor-version.sh to "+
					"bump the minor version in %v, then retry "+
					"(or re-run with --force to replace the existing release)",
				tag, versionFile)
		}
		hasTag, err = tagExists(tag)
		if err != nil {
			return err
		}
	}

	// Tag FIRST, before the (slow) build/upload, so downstream modules can
	// `go mod tidy` against the tag immediately. The tag is created only when
	// absent; an existing tag is NEVER modified — that immutability is what
	// keeps the proxy.golang.org / pkg.go.dev module checksum stable so
	// consumers never hit a "verifying ... checksum mismatch" error. Dry-run
	// skips all remote mutation.
	if !opts.dryRun {
		if hasTag {
			mustFprintf(progress,
				"Tag %v already exists; not modifying it (--force never touches tags). "+
					"Artifacts will be rebuilt against the existing tag.\n", tag)
		} else {
			clean, cerr := workingTreeClean()
			if cerr != nil {
				return cerr
			}
			if !clean {
				return fmt.Errorf(
					"working tree has uncommitted changes to tracked files; commit them "+
						"(including the version bump in %v) and push before publishing so "+
						"the tag points at the exact source the artifacts are built from",
					versionFile)
			}
			mustFprintf(progress,
				"Creating and pushing tag %v first (consumers can `go mod tidy` immediately).\n", tag)
			if err := createAndPushTag(tag); err != nil {
				return err
			}
		}
	}

	// Build into a scratch dir under the system temp location so we never
	// pollute the working tree and can clean up wholesale.
	outDir, err := os.MkdirTemp("/tmp", scratchDirPrefix)
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()
	if err := markScratchInUse(outDir); err != nil {
		return err
	}

	assets, err := buildAll(outDir, version, progress)
	if err != nil {
		return err
	}

	// The Android appliance is consumed, never built: see androidStagingDir.
	assets = append(assets, collectAndroidArtifacts(androidStagingDir, version, progress)...)

	notes := buildReleaseNotes(version, repo, assets)

	if opts.dryRun {
		fmt.Print(notes)
		// Preview the retention step too, so a dry run shows everything a real
		// run would do. The plan goes to progress (stderr), keeping stdout a
		// clean Markdown document.
		return pruneReleaseAssets(opts.keepReleases, true, progress)
	}

	// Everything is built and hashed; make sure it is still on disk before gh is
	// asked to upload it.
	if err := verifyArtifacts(assets); err != nil {
		return err
	}

	// Recreate the GitHub Release (release page + uploaded binaries) WITHOUT
	// touching the git tag. With --force the existing release is deleted first
	// (no --cleanup-tag, so the tag stays); `gh release create` then reuses the
	// existing (immutable) tag.
	if opts.force && exists {
		mustFprintf(progress,
			"--force: deleting existing release %v and its assets (tag is left untouched)\n", tag)
		if err := gh("release", "delete", tag, "--yes"); err != nil {
			return fmt.Errorf("delete existing release: %w", err)
		}
	}

	args := []string{"release", "create", tag, "--title", tag, "--notes", notes}
	args = append(args, assets...)
	if err := gh(args...); err != nil {
		return fmt.Errorf("create release: %w", err)
	}

	mustFprintf(progress, "\nPublished release %v with %v asset(s):\n", tag, len(assets))
	for _, a := range assets {
		mustFprintf(progress, "  %v\n", filepath.Base(a))
	}

	// LAST: retire the assets of the release that just fell out of the
	// retention window. This only runs after the publish has succeeded, so a
	// failed or aborted publish never deletes anything. The failure message
	// spells out that the release itself is already published, because the
	// prune is resumable and can be finished on its own.
	if err := pruneReleaseAssets(opts.keepReleases, false, progress); err != nil {
		return fmt.Errorf(
			"release %v was published successfully, but pruning old release assets "+
				"failed (re-run ./scripts/publish-github-release-artifacts.sh "+
				"--prune-only to finish the cleanup): %w", tag, err)
	}
	return nil
}

// readVersion parses the VERSION constant out of nomadnet/version/version.go.
func readVersion() (string, error) {
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return "", fmt.Errorf("read %v: %w", versionFile, err)
	}
	re := regexp.MustCompile(`VERSION\s*=\s*"([^"]+)"`)
	m := re.FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("no VERSION string constant found in %v", versionFile)
	}
	v := string(m[1])
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(v) {
		return "", fmt.Errorf("version %q in %v is not a clean MAJOR.MINOR.PATCH semver", v, versionFile)
	}
	return v, nil
}

// releaseExists reports whether a GitHub release with the given tag exists.
func releaseExists(tag string) (bool, error) {
	cmd := exec.Command("gh", "release", "view", tag, "--json", "tagName")
	if err := cmd.Run(); err != nil {
		// gh returns a non-zero exit (and a message like "release not found")
		// when the tag does not exist; treat that as "does not exist".
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			if strings.Contains(string(ee.Stderr), "not found") {
				return false, nil
			}
		}
		// Fall back to parsing combined output for the not-found signal.
		out, outErr := exec.Command("gh", "release", "view", tag).CombinedOutput()
		if outErr != nil && strings.Contains(string(out), "not found") {
			return false, nil
		}
		if outErr == nil {
			return true, nil
		}
		return false, fmt.Errorf("check existing release %v: %w", tag, err)
	}
	return true, nil
}

// tagExists reports whether a git tag named tag exists on the remote.
func tagExists(tag string) (bool, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("gh", "api", "--method", "GET",
		"repos/:owner/:repo/git/refs/tags/"+tag)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// gh exits non-zero with a "Not Found" / 404 message when the ref
		// does not exist; treat that as "no such tag". The check is
		// case-insensitive because gh outputs "Not Found" (capitalized).
		s := strings.ToLower(stderr.String())
		if strings.Contains(s, "not found") || strings.Contains(s, "404") {
			return false, nil
		}
		return false, fmt.Errorf("check existing tag %v: %w (stderr: %v)", tag, err, stderr.String())
	}
	return true, nil
}

// workingTreeClean reports whether the working tree has no uncommitted changes
// to tracked files (untracked files are ignored). The publisher tags HEAD, so a
// clean tree ensures the tag points at the exact source the artifacts are built
// from — keeping the published module source and the built binaries in sync.
func workingTreeClean() (bool, error) {
	err := exec.Command("git", "diff", "--quiet", "HEAD").Run()
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("check working tree clean: %w", err)
}

// createAndPushTag creates a lightweight git tag at HEAD and pushes it to
// origin. The local tag is refreshed with -f (local-only, no remote effect);
// the push uses no --force, so an existing remote tag is rejected rather than
// moved. The caller only reaches here when tagExists reported the remote tag as
// absent, so the push creates a new remote tag without ever modifying one.
func createAndPushTag(tag string) error {
	if err := exec.Command("git", "tag", "-f", tag).Run(); err != nil {
		return fmt.Errorf("git tag %v: %w", tag, err)
	}
	if err := exec.Command("git", "push", "origin", tag).Run(); err != nil {
		return fmt.Errorf("git push origin %v (ensure HEAD is pushed and the tag is new to the remote): %w", tag, err)
	}
	return nil
}

// ghRepoSlug returns the "owner/repo" slug gh is authenticated against.
func ghRepoSlug() (string, error) {
	out, err := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner").Output()
	if err != nil {
		return "", fmt.Errorf("determine repo slug: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gh runs a gh command, streaming stdio to the terminal.
func gh(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// markScratchInUse records this process's PID in dir, so
// scripts/clean-test-tmp.sh skips the directory: the sweeper reads the marker
// and leaves the dir alone for as long as that PID is alive. Without it, only
// the dir's name stands between a long build/upload and a concurrent test run's
// /tmp sweep.
func markScratchInUse(dir string) error {
	marker := filepath.Join(dir, inUseMarker)
	if err := os.WriteFile(marker, fmt.Appendf(nil, "%v\n", os.Getpid()), 0o644); err != nil {
		return fmt.Errorf("mark scratch dir %v in use: %w", dir, err)
	}
	return nil
}

// verifyArtifacts fails when a built artifact is no longer on disk. gh treats
// every asset argument as a glob and reports a missing file as the opaque
// "no matches found for <path>", so checking here instead names the file and
// says what happened: the scratch dir is removed when this process exits, so a
// file that has already vanished means something deleted it meanwhile.
func verifyArtifacts(assets []string) error {
	for _, path := range assets {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf(
				"artifact %v vanished after it was built (%w); the build scratch dir "+
					"is only deleted when this process exits, so a concurrent cleaner "+
					"(scripts/clean-test-tmp.sh, which once swept this dir mid-publish) "+
					"removed it. Re-run the script to rebuild and publish", path, err)
		}
	}
	return nil
}

// wagoSupportedTarget reports whether the target platform links the wago
// in-process wasm runtime (-tags wago): Linux, Darwin, or Windows on amd64
// or arm64; every other target keeps the zero-overhead stub.
func wagoSupportedTarget(goos, goarch string) bool {
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return false
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return false
	}
	return true
}

// buildTagsWithWago appends the wago tag to a build-tag list unless it is
// already present.
func buildTagsWithWago(tags string) string {
	for tag := range strings.SplitSeq(tags, ",") {
		if tag == "wago" {
			return tags
		}
	}
	if tags == "" {
		return "wago"
	}
	return tags + ",wago"
}

// buildAll builds one executable per target into outDir and returns the
// absolute paths of the produced artifacts. progress receives build chatter.
func buildAll(outDir, version string, progress *os.File) ([]string, error) {
	var assets []string
	for _, t := range majorTargets {
		name := fmt.Sprintf("%v-%v-%v-%v", binaryName, version, t.goos, t.goarch)
		if t.goos == "windows" {
			name += ".exe"
		}
		outPath := filepath.Join(outDir, name)

		mustFprintf(progress, "Building %v/%v -> %v\n", t.goos, t.goarch, name)
		buildArgs := []string{"build", "-trimpath"}
		if wagoSupportedTarget(t.goos, t.goarch) {
			// Release binaries on supported platforms link the wasm page
			// sandbox; other platforms keep the static-serving stub.
			buildArgs = append(buildArgs, "-tags=wago")
		}
		buildArgs = append(buildArgs, "-o", outPath, "./cmd/gonomadnet")
		cmd := exec.Command("go", buildArgs...)
		cmd.Env = append(os.Environ(),
			"GOOS="+t.goos,
			"GOARCH="+t.goarch,
			"CGO_ENABLED=0",
		)
		cmd.Stdout = progress
		cmd.Stderr = progress
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("build %v/%v: %w", t.goos, t.goarch, err)
		}
		assets = append(assets, outPath)
	}

	for _, hw := range hardwareTargets {
		name := fmt.Sprintf("%v-%v-%v-%v-%v", binaryName, version, hw.formFactor, hw.goos, hw.goarch)
		outPath := filepath.Join(outDir, name)

		mustFprintf(progress, "Building hardware target [%v] %v/%v -> %v\n", hw.formFactor, hw.goos, hw.goarch, name)
		buildTags := hw.buildTags
		if wagoSupportedTarget(hw.goos, hw.goarch) {
			buildTags = buildTagsWithWago(buildTags)
		}
		args := []string{"build", "-trimpath", "-tags=" + buildTags, "-o", outPath, "./cmd/gonomadnet"}
		cmd := exec.Command("go", args...)
		env := append(os.Environ(),
			"GOOS="+hw.goos,
			"GOARCH="+hw.goarch,
			"CGO_ENABLED=0",
		)
		if hw.armVersion != "" {
			env = append(env, "GOARM="+hw.armVersion)
		}
		cmd.Env = env
		cmd.Stdout = progress
		cmd.Stderr = progress
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("build hardware target %v (%v/%v): %w", hw.formFactor, hw.goos, hw.goarch, err)
		}
		assets = append(assets, outPath)
	}

	return assets, nil
}

// collectAndroidArtifacts finds the signed Android appliance staged for this release, and
// explains itself when there is none.
//
// It never fails. A release that carried no APK is a normal, working release; a release that
// refused to happen because an APK was missing would be a regression in the only path the Go
// project has for shipping anything at all.
func collectAndroidArtifacts(stagingDir, version string, progress *os.File) []string {
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		mustFprintf(progress,
			"No Android appliance staged in %v, so this release will not carry one.\n"+
				"  Build one with ./scripts/build-android-apk.sh, which leaves a signed APK there.\n",
			stagingDir)
		return nil
	}

	var assets []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, androidAssetPrefix) || !strings.HasSuffix(lower, androidAssetSuffix) {
			continue
		}
		if !strings.Contains(name, version) {
			// A stale appliance is worse than none: it would be published as this release's
			// build while containing daemons from another one.
			mustFprintf(progress,
				"Skipping %v: its name does not carry version %v, and a release must not "+
					"ship an appliance built from other sources.\n", name, version)
			continue
		}
		mustFprintf(progress, "Attaching the Android appliance %v\n", name)
		assets = append(assets, filepath.Join(stagingDir, name))
	}
	if len(assets) == 0 {
		mustFprintf(progress,
			"No Android appliance named version %v is staged in %v, so this release will not "+
				"carry one.\n", version, stagingDir)
	}
	return assets
}

// readGoReticulumVersion returns the version of the sibling go-reticulum module the release
// binaries and the APK's daemons were compiled from, or an explanatory placeholder.
//
// It is a file read rather than a build query because the answer has to appear in the release
// notes, which are rendered before anything is uploaded and on a machine where a `go list`
// against the workspace may not be meaningful.
func readGoReticulumVersion() string {
	data, err := os.ReadFile(repoFile(goReticulumVersionFile))
	if err != nil {
		return "an unrecorded version"
	}
	for raw := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if after, ok := strings.CutPrefix(line, "const VERSION = "); ok {
			return strings.Trim(after, "\"")
		}
	}
	return "an unrecorded version"
}

// hasAndroidArtifact reports whether any staged asset is the Android appliance.
func hasAndroidArtifact(assets []string) bool {
	for _, a := range assets {
		if strings.HasSuffix(strings.ToLower(filepath.Base(a)), androidAssetSuffix) {
			return true
		}
	}
	return false
}

// buildReleaseNotes assembles the Markdown body for the release, including a
// sha256 checksum table for every artifact and DIY hardware targets.
func buildReleaseNotes(version, repo string, assets []string) string {
	var b strings.Builder
	mustFprintf(&b, "# Go NomadNet v%v\n\n", version)
	mustFprintf(&b, "Standalone executables built from [github.com/%v](https://github.com/%v) at tag v%v.\n\n", repo, repo, version)
	mustFprintf(&b, "Built with Go on %v/%v with `CGO_ENABLED=0`.\n\n", runtime.GOOS, runtime.GOARCH)
	mustFprintf(&b, "## Artifacts\n\n")
	mustFprintf(&b, "| File | sha256 |\n")
	mustFprintf(&b, "| --- | --- |\n")
	// Sort the artifact rows by filename so the published table is stable and
	// easy to scan regardless of the build order above.
	sorted := make([]string, len(assets))
	copy(sorted, assets)
	sort.Slice(sorted, func(i, j int) bool {
		return filepath.Base(sorted[i]) < filepath.Base(sorted[j])
	})
	for _, a := range sorted {
		sum, err := sha256sum(a)
		if err != nil {
			// Keep going; record the error in the table rather than aborting.
			mustFprintf(&b, "| %v | <error: %v> |\n", filepath.Base(a), err)
			continue
		}
		mustFprintf(&b, "| %v | `%v` |\n", filepath.Base(a), sum)
	}
	mustFprintf(&b, "\nVerify a download with `shasum -a 256 <file>`.\n\n")

	if hasAndroidArtifact(assets) {
		// What a reader of the release page needs about the APK: what it is, which go-reticulum
		// it carries, how to install it, and what the app does about the terminal the client
		// needs. The cross-repo pin is the reason for the version sentence: the appliance embeds
		// daemons compiled from the sibling module, so a note that named only this version would
		// leave its whole transport unaccounted for.
		mustFprintf(&b, "## Android appliance\n\n")
		mustFprintf(&b, "This release also carries a signed Android APK, for a tablet or phone. It is "+
			"not a build of the program above: it is the companion app, and what it adds is the "+
			"device's own GNSS receiver and compass, the Reticulum node daemons and a sensor "+
			"service that run in the background, and a `linux/arm64` build of the terminal client. "+
			"It embeds daemons compiled from **go-reticulum v%v**.\n\n", readGoReticulumVersion())
		mustFprintf(&b, "To install it, open the downloaded APK on the device and allow Android to "+
			"install from unknown sources when it asks. From a computer, `adb install -r <apk>` "+
			"does the same over USB. The APK is signed, so a later release upgrades it in place.\n\n")
		mustFprintf(&b, "The client is a terminal program, and Android has no terminal, so the app "+
			"brings one: it carries both the `linux/arm64` client and the console host that "+
			"gives it a pseudo-terminal, and draws the result on the screen. There is nothing "+
			"else to install and nothing to configure by hand — start the node, then open the "+
			"client, and it attaches to the transport the app owns.\n\n")
		mustFprintf(&b, "Follow [the Android appliance guide]"+
			"(https://github.com/%v/blob/master/docs/Android-APK.md): it is the short version "+
			"of the above, and it needs no computer. "+
			"[Running gonomadnet on Android](https://github.com/%v/blob/master/docs/Android.md) "+
			"covers running the client on a device without the appliance, and what Android does "+
			"to a terminal program.\n\n", repo, repo)
	}

	mustFprintf(&b, "## Hardware Projects & Pre-built Artifacts\n\n")
	mustFprintf(&b, "These binaries are pre-compiled for standalone DIY hardware targets:\n\n")
	mustFprintf(&b, "- **Form Factor A (Pocket Linux Terminal)**: Full interactive TUI on Raspberry Pi Zero 2W / SBC with 2.8\" SPI LCD and CardKB I2C keyboard.\n")
	mustFprintf(&b, "  - `gonomadnet-%v-pocket_terminal-linux-arm64` (Raspberry Pi Zero 2W, Pi 3/4/5 64-bit)\n", version)
	mustFprintf(&b, "  - `gonomadnet-%v-pocket_terminal-linux-arm` (Raspberry Pi Zero / Pi 1/2 32-bit)\n", version)
	mustFprintf(&b, "  - `gonomadnet-%v-pocket_terminal-linux-riscv64` (Milk-V Duo S / RISC-V SBCs)\n", version)
	mustFprintf(&b, "- **Form Factor B (Pocket Communicator)**: Embedded daemon/client mode with zero terminal dependencies for handheld communicators.\n")
	mustFprintf(&b, "  - `gonomadnet-%v-pocket_communicator-linux-arm64`\n", version)
	mustFprintf(&b, "  - `gonomadnet-%v-pocket_communicator-linux-arm`\n", version)
	mustFprintf(&b, "  - `gonomadnet-%v-pocket_communicator-linux-riscv64`\n", version)
	mustFprintf(&b, "\nSee [`Hardware-Projects-Guide.md`](https://github.com/gmlewis/asic-reticulum/blob/master/Hardware-Projects-Guide.md) for the complete bill of materials, assembly, and flashing instructions (including zero-install web flashing via [ESPConnect](https://thelastoutpostworkshop.github.io/ESPConnect/) and [Espressif Web Flasher](https://espressif.github.io/esptool-js/)).\n")

	mustFprintf(&b, "\n## Post-download setup\n\n")
	mustFprintf(&b, "Make the downloaded executable runnable:\n\n")
	mustFprintf(&b, "```\nchmod a+x gonomadnet-<version>-<os>-<arch>\n```\n\n")
	mustFprintf(&b, "On macOS, executables downloaded from the internet carry a\n")
	mustFprintf(&b, "quarantine attribute that blocks them from running until you approve\n")
	mustFprintf(&b, "them. Clear it with:\n\n")
	mustFprintf(&b, "```\nxattr -d com.apple.quarantine gonomadnet-<version>-<os>-<arch>\n```\n")
	return b.String()
}

// sha256sum returns the SHA-256 hex digest of the file at path.
func sha256sum(path string) (string, error) {
	out, err := exec.Command("shasum", "-a", "256", path).Output()
	if err != nil {
		return "", err
	}
	// shasum output: "<hash>  <path>"
	fields := strings.Fields(string(out))
	if len(fields) < 1 {
		return "", fmt.Errorf("unexpected shasum output: %q", string(out))
	}
	return fields[0], nil
}

func mustFprintf(w io.Writer, fmtStr string, args ...any) {
	if _, err := fmt.Fprintf(w, fmtStr, args...); err != nil {
		log.Fatalf("Fprintf failed: %v", err)
	}
}
