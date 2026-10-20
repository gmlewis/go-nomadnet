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

// TestPlanPruneDeletesEveryAsset pins the retention policy: the newest keep
// published releases keep their assets, everything older is planned for
// deletion, drafts are reported and never planned, and a pruned release's plan
// holds EVERY asset it still carries — nothing is selected by name.
//
// The asset names here cover everything a real release carries: the platform
// binaries this publisher generates, the installable appliances it publishes
// for Android and iOS, and a file it never produced. All of them must be
// planned for deletion on a pruned release.
func TestPlanPruneDeletesEveryAsset(t *testing.T) {
	t.Parallel()

	assets := func(names ...string) []asset {
		var out []asset
		for i, name := range names {
			out = append(out, asset{id: int64(i + 1), name: name, size: int64(len(name))})
		}
		return out
	}
	all := []publishedRelease{
		{tag: "v0.154.0", createdAt: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
			assets: assets("gonomadnet-0.154.0-linux-amd64")},
		{tag: "v0.153.0", createdAt: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC),
			assets: assets("gonomadnet-0.153.0-linux-amd64")},
		{tag: "v0.152.0", createdAt: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
			assets: assets(
				"gonomadnet-0.152.0-linux-amd64",
				"gonomadnet-0.152.0-android-arm64-v8a.apk",
				"gonomadnet-0.152.0-ios-arm64.ipa",
				"SHA256SUMS",
			)},
		{tag: "v0.151.0", createdAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), draft: true,
			assets: assets("gonomadnet-0.151.0-linux-amd64")},
	}

	retained, drafts, plans := planPrune(all, 2)

	if len(retained) != 2 || retained[0].tag != "v0.154.0" || retained[1].tag != "v0.153.0" {
		t.Errorf("retained = %v, want the newest 2 published releases (v0.154.0, v0.153.0)", tagList(retained))
	}
	if len(drafts) != 1 || drafts[0].tag != "v0.151.0" {
		t.Errorf("drafts = %v, want just v0.151.0", tagList(drafts))
	}
	if len(plans) != 1 {
		t.Fatalf("got %v plan(s), want 1 (only v0.152.0 falls outside the window)", len(plans))
	}
	if plans[0].rel.tag != "v0.152.0" {
		t.Errorf("plan tag = %v, want v0.152.0", plans[0].rel.tag)
	}
	if got := len(plans[0].assets); got != len(all[2].assets) {
		t.Fatalf("plan holds %v asset(s), want all %v of the release's assets", got, len(all[2].assets))
	}
	want := map[string]bool{
		"gonomadnet-0.152.0-linux-amd64":           true,
		"gonomadnet-0.152.0-android-arm64-v8a.apk": true,
		"gonomadnet-0.152.0-ios-arm64.ipa":         true,
		"SHA256SUMS":                               true,
	}
	for _, a := range plans[0].assets {
		if !want[a.name] {
			t.Errorf("plan holds unexpected asset %q", a.name)
		}
		delete(want, a.name)
	}
	for name := range want {
		t.Errorf("plan omits %q; every asset of a pruned release must be deleted", name)
	}

	// A window wider than a release's remaining assets prunes nothing.
	if _, _, plans := planPrune(all, 99); len(plans) != 0 {
		t.Errorf("keep=99 produced %v plan(s), want none", len(plans))
	}
}

// tagList returns the tags of releases in order, for test failure messages.
func tagList(releases []publishedRelease) []string {
	var tags []string
	for _, rel := range releases {
		tags = append(tags, rel.tag)
	}
	return tags
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
		{30184410941, "28.11 GB"}, // a GB-scale total
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
