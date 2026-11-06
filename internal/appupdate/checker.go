// Package appupdate is the whole update-availability check plus the detached self-update
// installer (install.go), used by Kira Space — a repo-root package,
// so it imports nothing under either app's own internal/ (Go's internal/ visibility rule enforces
// this at compile time). The check itself is one plain net/http GET against GitHub's public
// releases/latest endpoint, self-contained and dependency-free (no githost/github, no httpclient,
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
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	repoOwner = "vlad-cirstean"
	repoName  = "kira-studio"

	latestReleaseURL = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"

	requestTimeout = 10 * time.Second // githost/github's own apiTimeout, same reasoning
	maxBodyBytes   = 1 << 20          // a releases/latest body is tens of KiB; this is headroom

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

// release is the subset of GitHub's release object this package reads.
type release struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// Result is the whole answer Status returns.
type Result struct {
	UpdateAvailable bool
	CurrentVersion  string
	LatestVersion   string
}

// Checker owns one cached answer per app process. Safe for concurrent use.
type Checker struct {
	running   string
	userAgent string
	fetch     func(ctx context.Context, userAgent string) (release, error) // seam; the real one is httpFetch
	now       func() time.Time

	group singleflight.Group

	mu        sync.Mutex
	checkedAt time.Time
	lastOK    bool
	result    Result
}

// NewChecker builds a Checker over the real network fetch, parameterized only by appName (the
// calling app's own display name — App.Name, app.go) and runningVersion (that app's own
// buildinfo.Version, passed in rather than imported: a repo-root package cannot import either
// app's own internal/buildinfo, Go's internal/ rule).
func NewChecker(appName, runningVersion string) *Checker {
	return &Checker{
		running:   runningVersion,
		userAgent: appName + "/" + runningVersion,
		fetch:     httpFetch,
		now:       time.Now,
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

// FailureCause maps a failed check to a short, URL-free reason for the user.
func FailureCause(err error) string {
	var netErr net.Error
	switch {
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
	rel, err := c.fetch(ctx, c.userAgent)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkedAt = c.now()
	if err != nil {
		c.lastOK = false
		slog.Debug("appupdate: check failed", "scope", "appupdate", "err", err)
		return c.result, err
	}
	c.lastOK = true
	c.result = buildResult(c.running, rel)
	return c.result, nil
}

// buildResult turns a decoded release into this package's Result, skipping a draft or prerelease
// even though releases/latest already excludes both — one `if` against a server-side guarantee
// this app does not control.
func buildResult(running string, rel release) Result {
	result := Result{CurrentVersion: running, LatestVersion: rel.TagName}
	if rel.Draft || rel.Prerelease {
		return result
	}
	if !updateAvailable(running, rel.TagName) {
		return result
	}
	result.UpdateAvailable = true
	return result
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

// httpFetch is the real network call behind Checker.fetch. Any status other than 200 is an error
// — 404 (no published release yet) and 403 (rate-limited) included, which is exactly what makes
// them mean silence rather than a surfaced failure.
func httpFetch(ctx context.Context, userAgent string) (release, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersionHeader)
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("appupdate: unexpected status %d", resp.StatusCode)
	}

	var rel release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&rel); err != nil {
		return release{}, err
	}
	return rel, nil
}
