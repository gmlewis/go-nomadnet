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
// Usage:
//
//	publish-github-release-artifacts [--force] [-n]
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

// binaryName is the published artifact's base name.
const binaryName = "gonomadnet"

func main() {
	force := flag.Bool("force", false,
		"replace an existing release for this version (deletes previous assets)")
	dryRun := flag.Bool("n", false,
		"print the full Markdown release description that would be written to "+
			"stdout and exit, without publishing (artifacts are still built so "+
			"the sha256 checksums are real)")
	flag.Usage = func() {
		log.Printf("Usage: %v [--force] [-n]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(*force, *dryRun); err != nil {
		log.Printf("publish-github-release-artifacts: %v\n", err)
		os.Exit(1)
	}
}

// run builds the release artifacts and either publishes them as a new GitHub
// release (when dryRun is false) or prints the Markdown release description
// that would be written to stdout and returns (when dryRun is true). In dry-run
// mode all progress output is routed to stderr so stdout contains only the
// Markdown description.
func run(force, dryRun bool) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	// progress writes build/publish chatter; in dry-run mode it goes to stderr
	// so stdout stays a clean Markdown document.
	progress := os.Stdout
	if dryRun {
		progress = os.Stderr
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
	if !dryRun {
		exists, err = releaseExists(tag)
		if err != nil {
			return err
		}
		if exists && !force {
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
	if !dryRun {
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
	outDir, err := os.MkdirTemp("/tmp", "gonomadnet-release-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	assets, err := buildAll(outDir, version, progress)
	if err != nil {
		return err
	}

	notes := buildReleaseNotes(version, repo, assets)

	if dryRun {
		fmt.Print(notes)
		return nil
	}

	// Recreate the GitHub Release (release page + uploaded binaries) WITHOUT
	// touching the git tag. With --force the existing release is deleted first
	// (no --cleanup-tag, so the tag stays); `gh release create` then reuses the
	// existing (immutable) tag.
	if force && exists {
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
		return false, fmt.Errorf("check existing tag %v: %w (stderr: %s)", tag, err, stderr.String())
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
		cmd := exec.Command("go", "build", "-trimpath", "-o", outPath, "./cmd/gonomadnet")
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
	return assets, nil
}

// buildReleaseNotes assembles the Markdown body for the release, including a
// sha256 checksum table for every artifact.
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
	mustFprintf(&b, "\nVerify a download with `shasum -a 256 <file>`.\n")
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
