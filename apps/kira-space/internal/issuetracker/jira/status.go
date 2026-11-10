package jira

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

const (
	versionTimeout = 5 * time.Second
	authTimeout    = 10 * time.Second
	okTTL          = 5 * time.Minute
	notOKTTL       = 30 * time.Second
	// okStaleFor bounds how long an OK entry past okTTL is still served while a background probe runs.
	okStaleFor = time.Hour
)

type discovery struct {
	locator Locator
	runner  Runner
	clock   Clock

	mu      sync.Mutex
	cached  *statusEntry
	probing bool // a background probe runs
}

type statusEntry struct {
	status issuetracker.Status
	at     time.Time
}

func newDiscovery(l Locator, r Runner, c Clock) *discovery {
	return &discovery{locator: l, runner: r, clock: c}
}

// status caches one entry: 5 minutes when ok, 30 seconds otherwise, so a login in another terminal is
// noticed soon. An ok entry up to an hour old is served at once while one background probe replaces
// it; refresh and every other entry probe inline. A caller-cancelled probe is never cached.
func (d *discovery) status(ctx context.Context, refresh bool) issuetracker.Status {
	d.mu.Lock()
	if refresh {
		d.cached = nil
	}
	if e := d.cached; e != nil {
		age := d.clock.Now().Sub(e.at)
		s := e.status
		switch {
		case s.OK() && age < okTTL, !s.OK() && age < notOKTTL:
			d.mu.Unlock()
			return s
		case s.OK() && age < okStaleFor:
			start := !d.probing
			d.probing = true
			d.mu.Unlock()
			if start {
				go d.reprobe()
			}
			return s
		}
	}
	d.mu.Unlock()

	s := d.probe(ctx)
	if errors.Is(ctx.Err(), context.Canceled) {
		return s
	}
	d.mu.Lock()
	d.cached = &statusEntry{status: s, at: d.clock.Now()}
	d.mu.Unlock()
	return s
}

// reprobe replaces the entry with a fresh probe, ok or not. versionTimeout and authTimeout bound it.
func (d *discovery) reprobe() {
	s := d.probe(context.Background())
	d.mu.Lock()
	d.cached = &statusEntry{status: s, at: d.clock.Now()}
	d.probing = false
	d.mu.Unlock()
}

// forget drops the entry, so the next status call probes inline.
func (d *discovery) forget() {
	d.mu.Lock()
	d.cached = nil
	d.mu.Unlock()
}

func (d *discovery) probe(ctx context.Context) issuetracker.Status {
	path, probed, found := d.locator.Locate()
	if !found {
		return issuetracker.Status{Kind: issuetracker.KindNotFound, Probed: probed, Reason: "acli is not installed"}
	}
	vctx, cancel := context.WithTimeout(ctx, versionTimeout)
	out, err := d.runner.Run(vctx, path, []string{"--version"})
	cancel()
	if ctx.Err() != nil {
		return issuetracker.Status{Kind: issuetracker.KindNotFound, Path: path, Reason: "the request was cancelled"}
	}
	if err != nil {
		return issuetracker.Status{Kind: issuetracker.KindNotFound, Path: path, Reason: "acli could not be started: " + describe(err)}
	}
	version := versionRe.FindString(string(out))
	if version == "" {
		return issuetracker.Status{Kind: issuetracker.KindNotFound, Path: path, Reason: "acli did not report a version"}
	}

	actx, cancel := context.WithTimeout(ctx, authTimeout)
	out, err = d.runner.Run(actx, path, []string{"jira", "auth", "status"})
	cancel()
	if ctx.Err() != nil {
		return issuetracker.Status{Kind: issuetracker.KindNotFound, Path: path, Reason: "the request was cancelled"}
	}
	var exitErr *toolexec.ExecError
	switch {
	case errors.As(err, &exitErr):
		return issuetracker.Status{
			Kind: issuetracker.KindUnauthenticated, Path: path, Version: version,
			Reason: "not logged in to Jira: run `acli jira auth login --web`",
		}
	case err != nil:
		return issuetracker.Status{Kind: issuetracker.KindNotFound, Path: path, Reason: "acli could not be started: " + describe(err)}
	}
	site, account := parseAuthStatus(string(out))
	return issuetracker.Status{Kind: issuetracker.KindOK, Path: path, Version: version, Site: site, Account: account}
}

func describe(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "no answer in time"
	}
	return err.Error()
}

var versionRe = regexp.MustCompile(`\d+\.\d+(\.\d+)*`)

// parseAuthStatus reads "Site:" and "Email:" style lines, best-effort: acli documents neither the
// layout nor an exit code. Unrecognised output leaves both empty.
func parseAuthStatus(out string) (site, account string) {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimFunc(k, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') }))
		val := strings.TrimSpace(v)
		switch key {
		case "site":
			site = normalizeHost(val)
		case "email", "account", "user":
			if account == "" {
				account = val
			}
		}
	}
	return site, account
}

func normalizeHost(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	return strings.ToLower(strings.TrimRight(s, "/"))
}
