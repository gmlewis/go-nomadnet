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
	"strings"
	"testing"
	"time"
)

// TestParseReleaseAssets verifies the gh TSV stream is grouped into releases
// with their assets, ordered newest first regardless of the order the API
// returned them in, that each release's assets are sorted by name, and that the
// draft flag is carried through (a draft keeps its assets; see pruneReleaseAssets).
func TestParseReleaseAssets(t *testing.T) {
	t.Parallel()

	// Deliberately unsorted: the older release is listed first, the newer
	// release's assets arrive in reverse name order, and a draft is mixed in.
	tsv := strings.Join([]string{
		"v0.9.0\t2026-09-01T10:00:00Z\tfalse\t11\t100\tgonomadnet-0.9.0-linux-amd64",
		"v0.9.0\t2026-09-01T10:00:00Z\tfalse\t12\t200\tgonomadnet-0.9.0-darwin-amd64",
		"v0.10.0\t2026-09-02T10:00:00Z\tfalse\t21\t300\tgonomadnet-0.10.0-linux-amd64",
		"v0.10.0\t2026-09-02T10:00:00Z\tfalse\t22\t400\tgonomadnet-0.10.0-darwin-amd64",
		"v0.11.0\t2026-09-03T10:00:00Z\ttrue\t31\t500\tgonomadnet-0.11.0-linux-amd64",
		"",
	}, "\n")

	releases, err := parseReleaseAssets(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("parseReleaseAssets: %v", err)
	}
	if len(releases) != 3 {
		t.Fatalf("got %v release(s), want 3", len(releases))
	}
	if releases[0].tag != "v0.11.0" || releases[1].tag != "v0.10.0" || releases[2].tag != "v0.9.0" {
		t.Errorf("tags = %v, %v, %v; want v0.11.0 (newest) first",
			releases[0].tag, releases[1].tag, releases[2].tag)
	}
	if !releases[0].draft {
		t.Error("v0.11.0 draft = false, want true (the draft flag must be carried through)")
	}
	if releases[1].draft || releases[2].draft {
		t.Error("a published release was marked draft = true")
	}
	if got := releases[1].assets[0].name; got != "gonomadnet-0.10.0-darwin-amd64" {
		t.Errorf("first asset of v0.10.0 = %q, want the name-sorted darwin one", got)
	}
	if got := releases[1].totalBytes(); got != 700 {
		t.Errorf("v0.10.0 totalBytes() = %v, want 700", got)
	}
	if got := releases[2].totalBytes(); got != 300 {
		t.Errorf("v0.9.0 totalBytes() = %v, want 300", got)
	}
}

// TestParseReleaseAssetsErrors pins the failure modes: a malformed line, a
// non-RFC3339 timestamp, a bad draft flag, and a non-numeric id or size must
// all be reported rather than silently mis-parsed into a delete list.
func TestParseReleaseAssetsErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		desc string
		tsv  string
	}{
		{"wrong field count", "v0.9.0\t2026-09-01T10:00:00Z\tfalse\t11\t100"},
		{"bad timestamp", "v0.9.0\tyesterday\tfalse\t11\t100\tgonomadnet-0.9.0-linux-amd64"},
		{"bad draft flag", "v0.9.0\t2026-09-01T10:00:00Z\tmaybe\t11\t100\tgonomadnet-0.9.0-linux-amd64"},
		{"bad asset id", "v0.9.0\t2026-09-01T10:00:00Z\tfalse\tx\t100\tgonomadnet-0.9.0-linux-amd64"},
		{"bad size", "v0.9.0\t2026-09-01T10:00:00Z\tfalse\t11\tsmall\tgonomadnet-0.9.0-linux-amd64"},
	}
	for _, c := range cases {
		if _, err := parseReleaseAssets(strings.NewReader(c.tsv)); err == nil {
			t.Errorf("%v: parseReleaseAssets() = nil error, want an error", c.desc)
		}
	}
}

// TestSelectPrunable pins the retention window: the newest keep releases are
// retained, everything older is pruned, and a window larger than the number of
// releases prunes nothing.
func TestSelectPrunable(t *testing.T) {
	t.Parallel()

	releases := []publishedRelease{
		{tag: "v0.12.0"}, {tag: "v0.11.0"}, {tag: "v0.10.0"},
		{tag: "v0.9.0"}, {tag: "v0.8.0"},
	}
	cases := []struct {
		keep          int
		wantRetained  int
		wantPruneTags []string
	}{
		{keep: 3, wantRetained: 3, wantPruneTags: []string{"v0.9.0", "v0.8.0"}},
		{keep: 5, wantRetained: 5, wantPruneTags: nil},
		{keep: 10, wantRetained: 5, wantPruneTags: nil}, // window wider than history
		{keep: 1, wantRetained: 1, wantPruneTags: []string{"v0.11.0", "v0.10.0", "v0.9.0", "v0.8.0"}},
	}
	for _, c := range cases {
		retained, prune := selectPrunable(releases, c.keep)
		if len(retained) != c.wantRetained {
			t.Errorf("keep=%v: retained %v release(s), want %v", c.keep, len(retained), c.wantRetained)
		}
		var tags []string
		for _, rel := range prune {
			tags = append(tags, rel.tag)
		}
		if strings.Join(tags, ",") != strings.Join(c.wantPruneTags, ",") {
			t.Errorf("keep=%v: prune tags = %v, want %v", c.keep, tags, c.wantPruneTags)
		}
	}
}

// TestAssetNamePattern verifies the guard that keeps a hand-attached file safe.
// The generated list is every asset name the v0.151.0 release actually carries,
// so the test fails if the publisher's naming ever drifts out of the pattern;
// anything else — a checksum list, a notes file, an archive, or a binary built
// for a different version — must not match.
func TestAssetNamePattern(t *testing.T) {
	t.Parallel()

	re := assetNamePattern("0.151.0")
	generated := []string{
		"gonomadnet-0.151.0-darwin-amd64",
		"gonomadnet-0.151.0-darwin-arm64",
		"gonomadnet-0.151.0-freebsd-amd64",
		"gonomadnet-0.151.0-freebsd-arm64",
		"gonomadnet-0.151.0-linux-amd64",
		"gonomadnet-0.151.0-linux-arm64",
		"gonomadnet-0.151.0-windows-amd64.exe",
		"gonomadnet-0.151.0-windows-arm64.exe",
		"gonomadnet-0.151.0-pocket_terminal-linux-arm",
		"gonomadnet-0.151.0-pocket_terminal-linux-arm64",
		"gonomadnet-0.151.0-pocket_terminal-linux-riscv64",
		"gonomadnet-0.151.0-pocket_communicator-linux-arm",
		"gonomadnet-0.151.0-pocket_communicator-linux-arm64",
		"gonomadnet-0.151.0-pocket_communicator-linux-riscv64",
	}
	for _, name := range generated {
		if !re.MatchString(name) {
			t.Errorf("assetNamePattern(0.151.0) does not match generated name %q", name)
		}
	}
	notGenerated := []string{
		"SHA256SUMS",
		"release-notes.md",
		"gonomadnet-0.150.0-linux-amd64",        // a different version's binary
		"gonomadnet-0.151.0-linux-amd64.tar.gz", // an archive
		"gonomadnet-0.151.0-linux-amd64.sha256", // a checksum sidecar
		"gonomadnet-0.151.0-linux-amd64.sig",    // a detached signature
		"gonomadnet-0.151.0-linux-amd64.pdb",    // debug symbols
		"gonomadnet-0.151.0-plan9-amd64",        // a platform this tool never builds
		"gonomadnet-0.151.0-linux-sparc64",      // an architecture it never builds
	}
	for _, name := range notGenerated {
		if re.MatchString(name) {
			t.Errorf("assetNamePattern(0.151.0) matches non-generated name %q; it must be skipped", name)
		}
	}

	// The guard is a shape match, so a hand-uploaded file that deliberately
	// mimics "<program>-<version>-<platform>" is indistinguishable from a real
	// artifact and would be pruned. This is a known limit, pinned here so the
	// behavior is a decision rather than an accident.
	if !re.MatchString("hotfix-gonomadnet-0.151.0-linux-amd64") {
		t.Error("assetNamePattern(0.151.0) no longer matches a name mimicking the generated scheme; " +
			"if the pattern was tightened, update this expectation and the comment in prune.go")
	}
}

// TestHumanBytes checks the size formatting used in the prune report.
func TestHumanBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		n    int64
		want string
	}{
		{0, "0.0 MB"},
		{1024 * 1024, "1.0 MB"},
		{21 * 1024 * 1024, "21.0 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
		{323 * 1024 * 1024, "323.0 MB"},
		{30184410941, "28.11 GB"}, // every asset this repo stores today
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%v) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestPacer verifies the pacing that keeps a bulk delete under GitHub's
// secondary rate limit: it spaces requests by the interval implied by
// perMinute, and the first request never waits.
func TestPacer(t *testing.T) {
	t.Parallel()

	// 6000/minute => 10ms between requests, fast enough to test.
	p := &pacer{perMinute: 6000}
	if got := p.interval(); got != 10*time.Millisecond {
		t.Fatalf("interval() = %v, want 10ms", got)
	}
	start := time.Now()
	p.wait() // no sleep: first request
	if elapsed := time.Since(start); elapsed > 5*time.Millisecond {
		t.Errorf("first wait() slept %v, want no delay", elapsed)
	}
	p.wait()
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Errorf("second wait() returned after %v, want at least the 10ms interval", elapsed)
	}
	// A pacer with no rate set never sleeps, so a zero value is harmless.
	q := &pacer{}
	if got := q.interval(); got != 0 {
		t.Errorf("zero pacer interval() = %v, want 0", got)
	}
}

// TestRemainingTimeEstimate verifies the ETA reported during a long prune:
// with a known rate it extrapolates from what has already been done, and with
// nothing done yet it falls back to the configured pace.
func TestRemainingTimeEstimate(t *testing.T) {
	t.Parallel()

	// 600 assets in 60s of elapsed work => 10/s for the remaining 600 => 60s.
	start := time.Now().Add(-60 * time.Second)
	if got := remainingTime(start, 600, 600); got < 55*time.Second || got > 65*time.Second {
		t.Errorf("remainingTime(600 done, 600 to go) = %v, want about 1m", got)
	}
	// Nothing done yet: estimate from the configured pace, which is one minute
	// of work per deletesPerMinute assets.
	if got := remainingTime(time.Now(), 0, deletesPerMinute); got != time.Minute {
		t.Errorf("remainingTime(0 done, %v to go) = %v, want 1m", deletesPerMinute, got)
	}
}
