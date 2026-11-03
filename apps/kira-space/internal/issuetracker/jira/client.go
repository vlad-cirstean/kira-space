package jira

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

const (
	viewTimeout  = 10 * time.Second
	fetchBound   = 20 * time.Second
	resultOKTTL  = 60 * time.Second
	resultErrTTL = 15 * time.Second
	noDevTTL     = 10 * time.Minute

	// firstFields reads everything but the heavy fields once per site, to find the Development field.
	firstFields    = "*all,-comment,-attachment,-worklog,-description"
	fallbackFields = "summary,status,*navigable"
)

type resultEntry struct {
	issue issuetracker.Issue
	err   error
	until time.Time
}

// devInfo is what a site's first read taught: the Development field's id, or that it has none.
type devInfo struct {
	id      string
	noDevTo time.Time // no Development field until then
}

type detailEntry struct {
	d  issuetracker.IssueDetail
	at time.Time
}

type client struct {
	disc   *discovery
	runner Runner
	clock  Clock
	bound  time.Duration

	mu    sync.Mutex
	cache map[string]resultEntry
	dev   map[string]devInfo
	// details holds the last issue view per key, see detail.
	details map[string]detailEntry
	// detailGen counts forgetDetail calls per key; a read that began before one must not save.
	detailGen map[string]uint64
	sf        singleflight.Group
}

func newClient(d *discovery, r Runner, c Clock) *client {
	return &client{disc: d, runner: r, clock: c, bound: fetchBound, cache: map[string]resultEntry{}, dev: map[string]devInfo{}, details: map[string]detailEntry{}, detailGen: map[string]uint64{}}
}

type fetched struct {
	issue issuetracker.Issue
	err   error
}

// issue answers from the 60 s result cache, else one coalesced fetch per key. The fetch runs on a
// context detached from the caller, so a caller that gives up (a short artifact budget) leaves it
// running and the next call finds its result in the cache.
func (c *client) issue(ctx context.Context, ref issuetracker.Ref) (issuetracker.Issue, error) {
	key := cacheKey(ref)
	if e, ok := c.cached(key); ok {
		return e.issue, e.err
	}
	ch := c.sf.DoChan(key, func() (any, error) {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.bound)
		defer cancel()
		iss, err := c.fetch(fctx, ref)
		c.store(key, iss, err)
		return fetched{iss, err}, nil
	})
	select {
	case r := <-ch:
		f := r.Val.(fetched)
		return f.issue, f.err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return issuetracker.Issue{}, fmt.Errorf("%w: %v", issuetracker.ErrTimeout, ctx.Err())
		}
		return issuetracker.Issue{}, ctx.Err()
	}
}

func (c *client) cached(key string) (resultEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || !c.clock.Now().Before(e.until) {
		delete(c.cache, key)
		return resultEntry{}, false
	}
	return e, true
}

// store keeps a result. Missing CLI or login is cheap to re-probe through the status cache, so those
// are not stored; the other errors live the short TTL.
func (c *client) store(key string, iss issuetracker.Issue, err error) {
	if errors.Is(err, issuetracker.ErrUnavailable) || errors.Is(err, issuetracker.ErrUnauthenticated) ||
		errors.Is(err, issuetracker.ErrOtherSite) {
		return
	}
	ttl := resultOKTTL
	if err != nil {
		ttl = resultErrTTL
	}
	c.mu.Lock()
	c.cache[key] = resultEntry{issue: iss, err: err, until: c.clock.Now().Add(ttl)}
	c.mu.Unlock()
}

// ready gates a read on the CLI being installed, signed in, and on the site ref names.
func (c *client) ready(ctx context.Context, ref issuetracker.Ref) (issuetracker.Status, error) {
	st := c.disc.status(ctx, false)
	switch st.Kind {
	case issuetracker.KindOK:
	case issuetracker.KindUnauthenticated:
		return st, issuetracker.ErrUnauthenticated
	default:
		return st, issuetracker.ErrUnavailable
	}
	if ref.Site != "" && st.Site != "" && !strings.EqualFold(ref.Site, st.Site) {
		return st, fmt.Errorf("%w: %s is not %s", issuetracker.ErrOtherSite, ref.Site, st.Site)
	}
	return st, nil
}

func cacheKey(ref issuetracker.Ref) string { return strings.ToLower(ref.Site) + "|" + ref.Key }

// forget drops the cached result of key.
func (c *client) forget(key string) {
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()
}

func (c *client) fetch(ctx context.Context, ref issuetracker.Ref) (issuetracker.Issue, error) {
	st, err := c.ready(ctx, ref)
	if err != nil {
		return issuetracker.Issue{}, err
	}

	info := c.devInfo(st.Site)
	first := info.id == "" && !c.clock.Now().Before(info.noDevTo)
	fields := "summary,status"
	switch {
	case first:
		fields = firstFields
	case info.id != "":
		fields = "summary,status," + info.id
	}
	out, err := c.view(ctx, st.Path, ref.Key, fields)
	if first && errors.Is(err, toolexec.ErrOutputTooLarge) {
		c.setDev(st.Site, devInfo{noDevTo: c.clock.Now().Add(noDevTTL)})
		out, err = c.view(ctx, st.Path, ref.Key, fallbackFields)
		first = false
	}
	if err != nil {
		return issuetracker.Issue{}, err
	}
	p, ok := parseIssue(out)
	if !ok {
		return issuetracker.Issue{}, errors.New("jira: acli returned an unreadable work item")
	}

	iss := issuetracker.Issue{
		Ref:   ref,
		Title: p.title, Status: p.status, StatusCategory: p.category,
	}
	iss.Ref.Key = p.key
	if iss.Ref.URL == "" && st.Site != "" {
		iss.Ref.URL = "https://" + st.Site + "/browse/" + p.key
	}
	if iss.Ref.Site == "" {
		iss.Ref.Site = st.Site
	}
	switch {
	case first:
		id, sum, found := findDevField(p.fields)
		if found {
			c.setDev(st.Site, devInfo{id: id})
			iss.Summary = sum
		} else {
			c.setDev(st.Site, devInfo{noDevTo: c.clock.Now().Add(noDevTTL)})
		}
	case info.id != "":
		if sum, found := parseDevSummary(p.fields[info.id]); found {
			iss.Summary = sum
		}
	}

	return iss, nil
}

func (c *client) devInfo(site string) devInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dev[site]
}

func (c *client) setDev(site string, d devInfo) {
	c.mu.Lock()
	c.dev[site] = d
	c.mu.Unlock()
}

// view runs `acli jira workitem view` and classifies a failure.
func (c *client) view(ctx context.Context, path, key, fields string) ([]byte, error) {
	vctx, cancel := context.WithTimeout(ctx, viewTimeout)
	defer cancel()
	out, err := c.runner.Run(vctx, path, []string{"jira", "workitem", "view", key, "--json", "--fields", fields})
	if err == nil {
		return out, nil
	}
	return nil, classify(err, vctx, "workitem view")
}

// classify maps a failed acli run to a typed error; err itself when it matches none.
func classify(err error, vctx context.Context, what string) error {
	if vctx.Err() != nil {
		return fmt.Errorf("%w: acli %s", issuetracker.ErrTimeout, what)
	}
	var exitErr *toolexec.ExecError
	if errors.As(err, &exitErr) {
		msg := strings.ToLower(exitErr.Stderr)
		switch {
		case containsAny(msg, "does not exist", "not exist", "not found", "404", "permission", "no issue"):
			return fmt.Errorf("%w: %s", issuetracker.ErrNotFound, exitErr.Stderr)
		case containsAny(msg, "unauthorized", "401", "not logged in", "authenticate", "log in"):
			return fmt.Errorf("%w: %s", issuetracker.ErrUnauthenticated, exitErr.Stderr)
		}
	}
	return err
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
