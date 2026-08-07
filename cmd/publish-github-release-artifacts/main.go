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
// new GitHub Release (via the gh CLI) tagged with the current version string
// read from nomadnet/version/version.go.
//
// If a release for that version already exists, the command fails unless
// --force is supplied, in which case the previous release (and its assets) is
// deleted and replaced with freshly built ones. The release notes embed a
// sha256 checksum table for every uploaded artifact.
//
// Usage:
//
//	publish-github-release-artifacts [--force]
//
// This program is normally driven by scripts/publish-github-release-artifacts.sh.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [--force]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(*force); err != nil {
		fmt.Fprintf(os.Stderr, "publish-github-release-artifacts: %v\n", err)
		os.Exit(1)
	}
}

func run(force bool) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	version, err := readVersion()
	if err != nil {
		return err
	}
	tag := "v" + version
	fmt.Printf("Publishing release for version %s (tag %s)\n", version, tag)

	exists, err := releaseExists(tag)
	if err != nil {
		return err
	}
	if exists && !force {
		return fmt.Errorf(
			"release %s already exists; run scripts/bump-minor-version.sh to "+
				"bump the minor version in %s, then retry "+
				"(or re-run with --force to replace the existing release)",
			tag, versionFile)
	}

	repo, err := ghRepoSlug()
	if err != nil {
		return err
	}
	fmt.Printf("Repository: %s\n", repo)

	// Build into a scratch dir under the system temp location so we never
	// pollute the working tree and can clean up wholesale.
	outDir, err := os.MkdirTemp("/tmp", "gonomadnet-release-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	assets, err := buildAll(outDir, version)
	if err != nil {
		return err
	}

	notes := buildReleaseNotes(version, repo, assets)

	if exists && force {
		fmt.Printf("--force: deleting existing release %s and its assets\n", tag)
		if err := gh("release", "delete", tag, "--yes"); err != nil {
			return fmt.Errorf("delete existing release: %w", err)
		}
	}

	args := []string{"release", "create", tag, "--title", tag, "--notes", notes}
	args = append(args, assets...)
	if err := gh(args...); err != nil {
		return fmt.Errorf("create release: %w", err)
	}

	fmt.Printf("\nPublished release %s with %d asset(s):\n", tag, len(assets))
	for _, a := range assets {
		fmt.Printf("  %s\n", filepath.Base(a))
	}
	return nil
}

// readVersion parses the Version constant out of nomadnet/version/version.go.
func readVersion() (string, error) {
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", versionFile, err)
	}
	re := regexp.MustCompile(`Version\s*=\s*"([^"]+)"`)
	m := re.FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("no Version string constant found in %s", versionFile)
	}
	v := string(m[1])
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(v) {
		return "", fmt.Errorf("version %q in %s is not a clean MAJOR.MINOR.PATCH semver", v, versionFile)
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
		return false, fmt.Errorf("check existing release %s: %w", tag, err)
	}
	return true, nil
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
// absolute paths of the produced artifacts.
func buildAll(outDir, version string) ([]string, error) {
	var assets []string
	for _, t := range majorTargets {
		name := fmt.Sprintf("%s-%s-%s-%s", binaryName, version, t.goos, t.goarch)
		if t.goos == "windows" {
			name += ".exe"
		}
		outPath := filepath.Join(outDir, name)

		fmt.Printf("Building %s/%s -> %s\n", t.goos, t.goarch, name)
		cmd := exec.Command("go", "build", "-trimpath", "-o", outPath, "./cmd/gonomadnet")
		cmd.Env = append(os.Environ(),
			"GOOS="+t.goos,
			"GOARCH="+t.goarch,
			"CGO_ENABLED=0",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("build %s/%s: %w", t.goos, t.goarch, err)
		}
		assets = append(assets, outPath)
	}
	return assets, nil
}

// buildReleaseNotes assembles the Markdown body for the release, including a
// sha256 checksum table for every artifact.
func buildReleaseNotes(version, repo string, assets []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Go NomadNet v%s\n\n", version)
	fmt.Fprintf(&b, "Standalone executables built from [github.com/%s](https://github.com/%s) at tag v%s.\n\n", repo, repo, version)
	fmt.Fprintf(&b, "Built with Go on %s/%s with `CGO_ENABLED=0`.\n\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "## Artifacts\n\n")
	fmt.Fprintf(&b, "| File | sha256 |\n")
	fmt.Fprintf(&b, "| --- | --- |\n")
	for _, a := range assets {
		sum, err := sha256sum(a)
		if err != nil {
			// Keep going; record the error in the table rather than aborting.
			fmt.Fprintf(&b, "| %s | <error: %v> |\n", filepath.Base(a), err)
			continue
		}
		fmt.Fprintf(&b, "| %s | `%s` |\n", filepath.Base(a), sum)
	}
	fmt.Fprintf(&b, "\nVerify a download with `shasum -a 256 <file>`.\n")
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
