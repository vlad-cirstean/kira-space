// Package appupdate is the whole update-availability check plus the detached self-update
// installer (install.go), used by Kira Space — a repo-root package,
// so it imports nothing under either app's own internal/ (Go's internal/ visibility rule enforces
// this at compile time). The check itself is a paginated plain net/http listing of GitHub's public
// releases endpoint, stepped one minor at a time by internal/releaseline, self-contained and dependency-free (no githost/github, no httpclient,
// no Wails import — drivable from a plain httptest server, matching the same self-contained shape
// internal/httpclient states as its own contract).
//
// internal/githost/github is deliberately not reused: every call of
// apps/kira-space/internal/githost/github is gated on a working,
// authenticated `gh` CLI (githost/github/discovery.go), and an update check that only runs for
// users who have installed and logged into the GitHub CLI is not an update check.
// internal/httpclient is not reused either — it is the user-facing request builder (redirect
// recording, wire capture, truncation reporting) for a question this package is not asking.
package appupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/kirathecat/kira-studio/internal/releaseline"
)

const (
	repoOwner = "vlad-cirstean"
	repoName  = "kira-space"

	// RepoURL is the one source of the GitHub repository URL for the app.
	RepoURL = "https://github.com/" + repoOwner + "/" + repoName

	releasesURL = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases"

	requestTimeout = 10 * time.Second // per page; githost/github's own apiTimeout, same reasoning
	perPage        = 100
	// maxPages bounds a listing at 1,000 releases; more is an error (fail closed), never a
	// truncated list that could drop an old install's own minor and cause a jump.
	maxPages = 10
	// maxPageBytes: release bodies carry notes, so a full page is far larger than a latest body.
	maxPageBytes = 8 << 20

	// apiVersionHeader restates apps/kira-space/internal/githost/github/api.go's own unexported constant of the same name — GitHub's
	// REST API version pin, so a future GitHub-side default-version bump can never silently reshape
	// the decoded fields here.
	apiVersionHeader = "2022-11-28"
)

// okInterval/failureInterval mirror githost/github/discovery.go's own okTTL/notOKTTL split: a good
// answer is worth holding for a long time, a failure is worth retrying sooner.
const (
	okInterval      = 6 * time.Hour
	failureInterval = 30 * time.Minute
)

// Result is the whole answer Status returns. Versions carry no v prefix. TargetVersion is the
// offered step, NewestVersion the highest published release.
type Result struct {
	UpdateAvailable bool
	CurrentVersion  string
	TargetVersion   string
	NewestVersion   string
}

// Checker owns one cached answer per app process. Safe for concurrent use.
type Checker struct {
	running   string
	userAgent string
	asset     string
	fetch     func(ctx context.Context, userAgent string) ([]releaseline.Release, error) // seam; the real one lists over HTTP
	now       func() time.Time

	group singleflight.Group

	mu        sync.Mutex
	checkedAt time.Time
	lastOK    bool
	result    Result
}

// CheckerOptions overrides the production wiring; a zero field takes the production value.
// Tests point ReleasesURL at an httptest server.
type CheckerOptions struct {
	AppName, Version string
	ReleasesURL      string
	Asset            string
	PerPage          int
}

// NewChecker builds a Checker over the real network fetch, parameterized only by appName (the
// calling app's own display name — App.Name, app.go) and runningVersion (that app's own
// buildinfo.Version, passed in rather than imported: a repo-root package cannot import either
// app's own internal/buildinfo, Go's internal/ rule).
func NewChecker(appName, runningVersion string) *Checker {
	return NewCheckerFrom(CheckerOptions{AppName: appName, Version: runningVersion})
}

// NewCheckerFrom is NewChecker with overridable endpoint, asset name and page size.
func NewCheckerFrom(o CheckerOptions) *Checker {
	if o.ReleasesURL == "" {
		o.ReleasesURL = releasesURL
	}
	if o.Asset == "" {
		o.Asset = platformAsset()
	}
	if o.PerPage <= 0 {
		o.PerPage = perPage
	}
	return &Checker{
		running:   o.Version,
		userAgent: o.AppName + "/" + o.Version,
		asset:     o.Asset,
		fetch: func(ctx context.Context, userAgent string) ([]releaseline.Release, error) {
			return listReleases(ctx, o.ReleasesURL, o.PerPage, userAgent)
		},
		now: time.Now,
	}
}

// Status returns the current cached answer, refreshing it first if it is due. It never returns an
// error: a failed check is silence, logged at debug level and nothing more (§3.3).
func (c *Checker) Status(ctx context.Context) Result {
	if !isReleaseBuild(c.running) {
		return Result{CurrentVersion: c.running}
	}

	if !c.dueForCheck() {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.result
	}

	res, _ := c.fetchLatest(ctx)
	return res
}

// ErrNotReleaseBuild: a dev build never checks.
var ErrNotReleaseBuild = errors.New("appupdate: not a release build")

// Check fetches now, whatever the cache holds, and returns the fetch error Status hides. It shares
// Status's single-flight, so a poll already in flight is joined, not doubled.
func (c *Checker) Check(ctx context.Context) (Result, error) {
	if !isReleaseBuild(c.running) {
		return Result{CurrentVersion: c.running}, ErrNotReleaseBuild
	}
	return c.fetchLatest(ctx)
}

var statusErrRe = regexp.MustCompile(`unexpected status (\d+)$`)

// errRateLimited: GitHub's unauthenticated limit (60 requests/hour/IP) is spent.
var errRateLimited = errors.New("appupdate: rate limited")

// FailureCause maps a failed check to a short, URL-free reason for the user.
func FailureCause(err error) string {
	var netErr net.Error
	switch {
	case errors.Is(err, errRateLimited):
		return "rate limited"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return "timed out"
	case errors.As(err, new(*net.OpError)) || errors.As(err, new(*net.DNSError)):
		return "offline"
	}
	if m := statusErrRe.FindStringSubmatch(err.Error()); m != nil {
		return "unexpected status " + m[1]
	}
	return "unknown error"
}

type fetched struct {
	result Result
	err    error
}

func (c *Checker) fetchLatest(ctx context.Context) (Result, error) {
	v, _, _ := c.group.Do("latest", func() (any, error) {
		res, err := c.refresh(ctx)
		return fetched{res, err}, nil
	})
	f := v.(fetched)
	return f.result, f.err
}

// dueForCheck reports whether the cached answer is stale enough to warrant a real network call —
// okInterval after a successful check, failureInterval after a failed one.
func (c *Checker) dueForCheck() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.checkedAt.IsZero() {
		return true
	}
	interval := failureInterval
	if c.lastOK {
		interval = okInterval
	}
	return c.now().Sub(c.checkedAt) >= interval
}

// refresh performs the real fetch and updates the cache. On error the previous result is left in
// place — only checkedAt and lastOK move — so a transient failure never blanks out a result a
// prior successful check already produced.
func (c *Checker) refresh(ctx context.Context) (Result, error) {
	rels, err := c.fetch(ctx, c.userAgent)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkedAt = c.now()
	if err != nil {
		c.lastOK = false
		slog.Debug("appupdate: check failed", "scope", "appupdate", "err", err)
		return c.result, err
	}
	c.lastOK = true
	c.result = buildResult(c.running, c.asset, rels)
	return c.result, nil
}

// buildResult turns the full release listing into this package's Result via the stepping rule.
func buildResult(running, asset string, rels []releaseline.Release) Result {
	step := releaseline.NextStep(running, rels, asset)
	return Result{
		UpdateAvailable: step.Target != "",
		CurrentVersion:  strings.TrimPrefix(running, "v"),
		TargetVersion:   step.Target,
		NewestVersion:   step.Newest,
	}
}

// httpClient is a package-level *http.Client over one *http.Transport, mirroring
// internal/httpclient's own sharedClient precedent (connection reuse, and a proxy-aware transport
// so a user behind a corporate proxy is not silently broken). TLS config left nil, so verification
// is on with no opt-out.
var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	},
}

// listReleases pages through the releases endpoint until a short page. Any failing page fails the
// whole listing: a partial list must never drive a decision. Only the decoded subset of
// releaseline.Release is read, never a download URL.
func listReleases(ctx context.Context, baseURL string, size int, userAgent string) ([]releaseline.Release, error) {
	var all []releaseline.Release
	for page := 1; page <= maxPages; page++ {
		rels, err := fetchPage(ctx, baseURL, size, page, userAgent)
		if err != nil {
			return nil, err
		}
		all = append(all, rels...)
		if len(rels) < size {
			return all, nil
		}
	}
	return nil, fmt.Errorf("appupdate: more than %d releases", maxPages*size)
}

// fetchPage is one releases GET. Any status other than 200 is an error; 429, or 403 with an empty
// rate-limit budget, is errRateLimited.
func fetchPage(ctx context.Context, baseURL string, size, page int, userAgent string) ([]releaseline.Release, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	url := baseURL + "?per_page=" + strconv.Itoa(size) + "&page=" + strconv.Itoa(page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersionHeader)
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
		return nil, errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("appupdate: unexpected status %d", resp.StatusCode)
	}

	var rels []releaseline.Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPageBytes)).Decode(&rels); err != nil {
		return nil, err
	}
	return rels, nil
}
