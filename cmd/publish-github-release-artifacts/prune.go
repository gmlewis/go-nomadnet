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

// Release-asset retention for publish-github-release-artifacts.
//
// Every publish uploads the full platform matrix (14 binaries, about 308 MB),
// and nothing in the publish path ever removed an older upload. One release a
// day therefore added ~308 MB per day forever, and the account carried 28 GB of
// old binaries that no consumer downloads (the whole history has had 554
// downloads, against 1,454 stored assets). GitHub documents no cap on total
// release size, but its Acceptable Use Policy reserves the right to throttle
// file hosting for accounts that place undue strain on the infrastructure, so
// unbounded growth is a real risk.
//
// pruneReleaseAssets deletes the ASSETS of every release older than the newest
// --keep-releases releases that still have assets. It never deletes a release,
// and never deletes a git tag: the release page, its notes, and the tagged
// source all stay exactly as published, so proxy.golang.org / pkg.go.dev
// module checksums and changelog links are unaffected. Only the uploaded
// binaries go away.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// defaultKeepReleases is the default retention window: the assets of the
// newest N releases that still have assets are kept and everything older is
// pruned. See pruneReleaseAssets.
const defaultKeepReleases = 10

// deletesPerMinute paces asset-deletion requests. Two GitHub limits bind:
//
//   - the primary limit of 5000 requests/hour for an authenticated token, which
//     is the tighter one here: 5000/60 is about 83 deletions/minute;
//   - the secondary limit of 900 points/minute on REST endpoints, where a
//     mutative request (POST/PATCH/PUT/DELETE) costs 5 points, so about
//     180 deletions/minute.
//
// We pace just under the primary limit (75/minute, 4500/hour) so deletions flow
// steadily rather than bursting and then parking when the hourly budget runs
// out, and so the token keeps enough budget for anything else it does in the
// same hour. The one-time cleanup of ~1,314 assets therefore takes about
// 18 minutes; in steady state a publish retires only about 14 assets, a few
// seconds. waitForRateLimit is the backstop for when other activity has already
// spent part of the hourly budget.
const deletesPerMinute = 75

// rateLimitCheckEvery is how many deletions happen between checks of the
// remaining core API budget. The endpoint does not count against the primary
// limit, and once per few hundred deletions is far below any secondary limit.
const rateLimitCheckEvery = 200

// rateLimitFloor is the remaining core API budget below which the pruner stops
// and waits for the window to reset. A publish happens at the end of the same
// run, so we leave room for it rather than draining the budget to zero.
const rateLimitFloor = 500

// deleteAttempts is how many times a single asset deletion is tried before the
// run gives up. Transient failures (a throttled response, a flaky connection)
// are retried with exponential backoff; a genuine "not found" is not a failure
// at all (see errAssetGone).
const deleteAttempts = 5

// progressEvery is how many deletions happen between periodic progress lines,
// so a long prune shows steady motion without flooding the terminal.
const progressEvery = 500

// assetPlatformPattern matches the platform suffix of an artifact name, in
// both the major-target form (linux-amd64) and the hardware form
// (pocket_terminal-linux-arm64).
const assetPlatformPattern = `(?:(?:linux|darwin|windows|freebsd)-(?:amd64|arm64|arm|riscv64)|` +
	`pocket_(?:terminal|communicator|hub)(?:-[a-z]+)?-linux-(?:amd64|arm64|arm|riscv64))`

// errAssetGone reports that an asset was already deleted (or its release was
// removed). That is a success for our purposes: a prune run that was
// interrupted, or that is racing another run, can simply be re-run, because
// the plan is recomputed from live state on every run.
var errAssetGone = errors.New("asset already deleted")

// asset is one uploaded file attached to a GitHub release.
type asset struct {
	id   int64
	name string
	size int64
}

// publishedRelease is a release and the assets still attached to it. A draft
// release is unpublished work in progress: it is reported but never pruned.
type publishedRelease struct {
	tag       string
	createdAt time.Time
	draft     bool
	assets    []asset
}

// totalBytes returns the combined size of the release's assets.
func (r publishedRelease) totalBytes() int64 {
	var n int64
	for _, a := range r.assets {
		n += a.size
	}
	return n
}

// releasesWithAssetsQuery is the gh --jq filter that flattens the releases API
// into one TSV line per asset: tag, created_at, draft flag, asset id, asset
// size, name.
//
// Drafts are included so the prune can REPORT them (an unpublished draft keeps
// its assets, and silently retaining a gigabyte would look like a bug), but
// they are never pruned. Note that the releases list endpoint returns every
// asset of each release (verified against all 1,454 assets in this repo), so
// a single paginated call is enough and no per-release request is needed.
const releasesWithAssetsQuery = `.[] | .tag_name as $t | .created_at as $c | ` +
	`(.draft | tostring) as $d | .assets[] | ` +
	`[$t, $c, $d, (.id | tostring), (.size | tostring), .name] | @tsv`

// listReleasesWithAssets returns every release that still has assets, newest
// first, drafts included. A release whose assets were fully pruned in an
// earlier run has no assets and therefore does not appear, so it stops
// consuming a slot in the retention window — which is what makes re-running
// converge.
func listReleasesWithAssets() ([]publishedRelease, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("gh", "api", "--paginate",
		"repos/:owner/:repo/releases?per_page=100",
		"--jq", releasesWithAssetsQuery)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list releases: %w (stderr: %v)", err, strings.TrimSpace(stderr.String()))
	}
	return parseReleaseAssets(bytes.NewReader(out))
}

// parseReleaseAssets reads the TSV stream produced by releasesWithAssetsQuery
// and groups it into releases, newest first, each with its assets sorted by
// name. Sorting is explicit rather than trusting API order so that the plan a
// dry run prints is exactly the plan a real run executes.
func parseReleaseAssets(r io.Reader) ([]publishedRelease, error) {
	byTag := make(map[string]*publishedRelease)
	var order []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 6 {
			return nil, fmt.Errorf("unexpected release asset line %q (want 6 tab-separated fields)", line)
		}
		createdAt, err := time.Parse(time.RFC3339, f[1])
		if err != nil {
			return nil, fmt.Errorf("parse created_at %q: %w", f[1], err)
		}
		draft, err := strconv.ParseBool(f[2])
		if err != nil {
			return nil, fmt.Errorf("parse draft flag %q: %w", f[2], err)
		}
		id, err := strconv.ParseInt(f[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse asset id %q: %w", f[3], err)
		}
		size, err := strconv.ParseInt(f[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse asset size %q: %w", f[4], err)
		}
		rel := byTag[f[0]]
		if rel == nil {
			rel = &publishedRelease{tag: f[0], createdAt: createdAt, draft: draft}
			byTag[f[0]] = rel
			order = append(order, f[0])
		}
		rel.assets = append(rel.assets, asset{id: id, name: f[5], size: size})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read release asset list: %w", err)
	}

	releases := make([]publishedRelease, 0, len(order))
	for _, tag := range order {
		rel := byTag[tag]
		sort.Slice(rel.assets, func(i, j int) bool { return rel.assets[i].name < rel.assets[j].name })
		releases = append(releases, *rel)
	}
	sort.SliceStable(releases, func(i, j int) bool {
		if !releases[i].createdAt.Equal(releases[j].createdAt) {
			return releases[i].createdAt.After(releases[j].createdAt)
		}
		return releases[i].tag > releases[j].tag // deterministic tie-break
	})
	return releases, nil
}

// selectPrunable splits releases (newest first) into the ones whose assets are
// retained and the ones whose assets are pruned. keep is the size of the
// retention window; the returned prune slice is newest first, so the run
// retires the most recently retired release first and leaves the oldest — the
// least likely to matter — for last, in case the run is interrupted.
func selectPrunable(releases []publishedRelease, keep int) (retained, prune []publishedRelease) {
	if keep > len(releases) {
		keep = len(releases)
	}
	return releases[:keep], releases[keep:]
}

// assetNamePattern returns the regexp matching the artifact names this
// publisher generates for version: "<program>-<version>-<platform>", with an
// optional .exe suffix.
//
// The prune deletes only names matching this pattern, so anything a human
// attached to an old release — a checksum list, a hotfix binary, a notes file
// — is reported and left alone. Every one of the 1,454 assets in this repo
// matches, so nothing is skipped today; the guard exists so that a future
// hand-uploaded file can never be destroyed by an automatic prune.
func assetNamePattern(version string) *regexp.Regexp {
	return regexp.MustCompile(`^[a-z][a-z0-9-]*-` + regexp.QuoteMeta(version) + `-` +
		assetPlatformPattern + `(?:\.exe)?$`)
}

// humanBytes formats a byte count as MB or GB with one decimal place.
func humanBytes(n int64) string {
	const (
		mb = 1 << 20
		gb = 1 << 30
	)
	if n >= gb {
		return fmt.Sprintf("%.2f GB", float64(n)/gb)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/mb)
}

// pacer spaces destructive requests so the run stays under GitHub's secondary
// rate limit. It allows perMinute requests per rolling minute and never
// blocks for longer than needed for the next slot.
type pacer struct {
	perMinute int
	last      time.Time
}

// wait blocks until the next request slot is available, then records it.
func (p *pacer) wait() {
	if !p.last.IsZero() {
		if d := p.interval() - time.Since(p.last); d > 0 {
			time.Sleep(d)
		}
	}
	p.last = time.Now()
}

// interval is the minimum spacing between two requests.
func (p *pacer) interval() time.Duration {
	if p.perMinute <= 0 {
		return 0
	}
	return time.Minute / time.Duration(p.perMinute)
}

// deleteAsset deletes one release asset by id. A 404 means the asset is
// already gone, which the caller treats as success (errAssetGone).
func deleteAsset(id int64) error {
	var stderr bytes.Buffer
	cmd := exec.Command("gh", "api", "--method", "DELETE",
		fmt.Sprintf("repos/:owner/:repo/releases/assets/%d", id))
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s := strings.ToLower(stderr.String())
		if strings.Contains(s, "not found") || strings.Contains(s, "404") {
			return errAssetGone
		}
		return fmt.Errorf("delete asset %d: %w (stderr: %v)", id, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// deleteAssetWithRetry deletes one asset, retrying transient failures with
// exponential backoff (1s, 2s, 4s, 8s). A throttled response is reported as
// 403/429; the backoff covers it, and the next rate-limit check parks the run
// until the window resets if the budget really is exhausted.
func deleteAssetWithRetry(id int64) error {
	var err error
	for i := range deleteAttempts {
		if i > 0 {
			time.Sleep(time.Duration(1<<uint(i-1)) * time.Second)
		}
		err = deleteAsset(id)
		if err == nil || errors.Is(err, errAssetGone) {
			return err
		}
	}
	return err
}

// coreRateLimitRemaining reports the remaining core API request budget and the
// Unix time at which that window resets. The rate_limit endpoint itself does
// not count against the primary limit.
func coreRateLimitRemaining() (remaining, reset int64, err error) {
	out, err := exec.Command("gh", "api", "rate_limit",
		"--jq", `.resources.core | "\(.remaining) \(.reset)"`).Output()
	if err != nil {
		return 0, 0, fmt.Errorf("read rate limit: %w", err)
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) != 2 {
		return 0, 0, fmt.Errorf("unexpected rate_limit output %q", string(out))
	}
	if remaining, err = strconv.ParseInt(f[0], 10, 64); err != nil {
		return 0, 0, fmt.Errorf("parse rate limit remaining %q: %w", f[0], err)
	}
	if reset, err = strconv.ParseInt(f[1], 10, 64); err != nil {
		return 0, 0, fmt.Errorf("parse rate limit reset %q: %w", f[1], err)
	}
	return remaining, reset, nil
}

// waitForRateLimit parks the run until the core API window resets, but only
// when the remaining budget has fallen to rateLimitFloor.
func waitForRateLimit(progress io.Writer) error {
	remaining, reset, err := coreRateLimitRemaining()
	if err != nil {
		return err
	}
	if remaining >= rateLimitFloor {
		return nil
	}
	until := time.Until(time.Unix(reset, 0).Add(5 * time.Second))
	if until <= 0 {
		return nil
	}
	mustFprintf(progress,
		"API budget down to %v request(s); waiting %v for the window to reset...\n",
		remaining, until.Round(time.Second))
	time.Sleep(until)
	return nil
}

// pruneReleaseAssets deletes the assets of every published release older than
// the newest keep releases that still have assets, leaving every release, its
// notes, and its git tag untouched. Draft releases keep their assets and are
// only reported. With dryRun set it prints the plan and deletes nothing.
//
// The run is resumable: the plan is recomputed from live state each time, so
// an interrupted run can simply be repeated.
func pruneReleaseAssets(keep int, dryRun bool, progress io.Writer) error {
	if keep < 1 {
		return fmt.Errorf("--keep-releases must be at least 1, got %v", keep)
	}
	all, err := listReleasesWithAssets()
	if err != nil {
		return err
	}
	var drafts, releases []publishedRelease
	for _, rel := range all {
		if rel.draft {
			drafts = append(drafts, rel)
			continue
		}
		releases = append(releases, rel)
	}
	retained, prune := selectPrunable(releases, keep)

	// Build the plan once, so the dry run and the real run describe exactly the
	// same set, and so the totals below count only assets this tool will really
	// delete (never a hand-attached file the guard skips).
	type prunePlan struct {
		rel       publishedRelease
		deletable []asset
		skipped   []string
	}
	plans := make([]prunePlan, 0, len(prune))
	var assetsToDelete int
	var bytesToDelete int64
	var skippedNames []string
	for _, rel := range prune {
		deletable, skipped := partitionAssets(rel, assetNamePattern(strings.TrimPrefix(rel.tag, "v")))
		plans = append(plans, prunePlan{rel: rel, deletable: deletable, skipped: skipped})
		assetsToDelete += len(deletable)
		bytesToDelete += deletableBytes(deletable)
		skippedNames = append(skippedNames, skipped...)
	}

	mustFprintf(progress,
		"\nRelease asset retention: keeping assets for the newest %v release(s) that have any.\n", keep)
	for _, rel := range retained {
		mustFprintf(progress, "  keep  %-12v %4v asset(s)  %v\n",
			rel.tag, len(rel.assets), humanBytes(rel.totalBytes()))
	}
	for _, rel := range drafts {
		mustFprintf(progress, "  draft %-12v %4v asset(s)  %v (unpublished: assets kept)\n",
			rel.tag, len(rel.assets), humanBytes(rel.totalBytes()))
	}
	if len(prune) == 0 {
		mustFprintf(progress, "Nothing to prune: only %v release(s) still have assets.\n", len(releases))
		return nil
	}
	mustFprintf(progress, "  Pruning %v older release(s): %v asset(s), %v\n",
		len(prune), assetsToDelete, humanBytes(bytesToDelete))
	if len(skippedNames) > 0 {
		mustFprintf(progress,
			"  %v asset(s) on those releases were not built by this tool and will be kept.\n",
			len(skippedNames))
	}
	if dryRun {
		for _, pl := range plans {
			mustFprintf(progress, "  prune %-12v %4v asset(s)  %v\n",
				pl.rel.tag, len(pl.deletable), humanBytes(deletableBytes(pl.deletable)))
		}
		mustFprintf(progress,
			"Dry run: nothing deleted. %v asset(s) across %v release(s) (%v) would be pruned.\n",
			assetsToDelete, len(prune), humanBytes(bytesToDelete))
		return nil
	}

	start := time.Now()
	p := &pacer{perMinute: deletesPerMinute}
	var deleted int
	var bytesFreed int64
	mustFprintf(progress, "Deleting at up to %v asset(s)/minute; interrupt and re-run to resume.\n",
		deletesPerMinute)
	for _, pl := range plans {
		var relDeleted int
		var relBytes int64
		for _, a := range pl.deletable {
			if deleted > 0 && deleted%rateLimitCheckEvery == 0 {
				if err := waitForRateLimit(progress); err != nil {
					return err
				}
			}
			p.wait()
			err := deleteAssetWithRetry(a.id)
			switch {
			case err == nil:
				deleted++
				relDeleted++
				relBytes += a.size
				bytesFreed += a.size
			case errors.Is(err, errAssetGone):
				// Already deleted by an earlier interrupted run.
				deleted++
				relDeleted++
			default:
				mustFprintf(progress,
					"Stopping after %v asset(s) (%v freed); re-run to resume.\n",
					deleted, humanBytes(bytesFreed))
				return err
			}
			if deleted%progressEvery == 0 {
				mustFprintf(progress, "  ... %v/%v asset(s) deleted (%v freed, %v elapsed, ~%v left)\n",
					deleted, assetsToDelete, humanBytes(bytesFreed),
					time.Since(start).Round(time.Second),
					remainingTime(start, deleted, assetsToDelete-deleted))
			}
		}
		mustFprintf(progress, "  pruned %-12v %4v asset(s)  %v\n",
			pl.rel.tag, relDeleted, humanBytes(relBytes))
	}

	mustFprintf(progress, "Pruned %v asset(s) across %v release(s): %v freed in %v.\n",
		deleted, len(plans), humanBytes(bytesFreed), time.Since(start).Round(time.Second))
	for _, name := range skippedNames {
		mustFprintf(progress, "  kept (not built by this tool): %v\n", name)
	}
	return nil
}

// partitionAssets splits a release's assets into those this tool generated for
// the release's version (which the prune may delete) and everything else
// (returned as names, so the caller can report what it deliberately kept).
func partitionAssets(rel publishedRelease, re *regexp.Regexp) (deletable []asset, skipped []string) {
	for _, a := range rel.assets {
		if re.MatchString(a.name) {
			deletable = append(deletable, a)
			continue
		}
		skipped = append(skipped, a.name)
	}
	return deletable, skipped
}

// deletableBytes returns the combined size of the given assets.
func deletableBytes(assets []asset) int64 {
	var n int64
	for _, a := range assets {
		n += a.size
	}
	return n
}

// remainingTime estimates how long the rest of a paced prune takes, based on
// the average rate observed so far.
func remainingTime(start time.Time, done, todo int) time.Duration {
	elapsed := time.Since(start)
	if done <= 0 || elapsed <= 0 {
		return time.Duration(todo) * time.Minute / deletesPerMinute
	}
	return time.Duration(float64(elapsed) / float64(done) * float64(todo))
}
