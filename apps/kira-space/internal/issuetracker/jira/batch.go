package jira

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

const (
	batchMax            = 50
	searchTimeout       = 20 * time.Second
	batchFallbackFanOut = 2
	searchFields        = "summary,status,priority,assignee,updated"
)

// issues reads refs with `workitem search`, batchMax keys per call, and refreshes the Issue cache. A
// chunk acli cannot answer is read per key; a refused search is skipped for noDevTTL.
func (c *client) issues(ctx context.Context, refs []issuetracker.Ref) (map[string]issuetracker.BatchIssue, error) {
	out := map[string]issuetracker.BatchIssue{}
	if len(refs) == 0 {
		return out, nil
	}
	st, err := c.ready(ctx, issuetracker.Ref{})
	if err != nil {
		return nil, err
	}
	refs = sameSite(refs, st.Site)
	if len(refs) == 0 {
		return out, nil
	}
	if info := c.devInfo(st.Site); info.id == "" && !c.clock.Now().Before(info.noDevTo) {
		// One read finds the Development field id; a failure here only drops the summary.
		if _, err := c.issue(ctx, refs[0], true); errors.Is(err, issuetracker.ErrUnauthenticated) || errors.Is(err, issuetracker.ErrUnavailable) {
			return nil, err
		}
	}
	for start := 0; start < len(refs); start += batchMax {
		chunk := refs[start:min(start+batchMax, len(refs))]
		if c.searchSkipped() {
			c.readEach(ctx, chunk, out)
			continue
		}
		res, err := c.searchChunk(ctx, st, chunk)
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch {
		case err == nil:
			for k, v := range res {
				out[k] = v
			}
		case errors.Is(err, issuetracker.ErrUnauthenticated), errors.Is(err, issuetracker.ErrUnavailable):
			return nil, err
		case errors.Is(err, issuetracker.ErrNotFound), errors.Is(err, toolexec.ErrOutputTooLarge), errors.Is(err, issuetracker.ErrTimeout):
			// Jira rejects a whole `key in` list when one key is gone: read the chunk per key.
			c.readEach(ctx, chunk, out)
		default:
			c.mu.Lock()
			c.noSearchTo = c.clock.Now().Add(noDevTTL)
			c.mu.Unlock()
			slog.Warn("jira: workitem search failed, reading per issue", "err", err)
			c.readEach(ctx, chunk, out)
		}
	}
	return out, nil
}

// sameSite drops duplicate keys and refs pasted from another site than the signed-in one.
func sameSite(refs []issuetracker.Ref, site string) []issuetracker.Ref {
	seen := map[string]bool{}
	kept := make([]issuetracker.Ref, 0, len(refs))
	for _, r := range refs {
		if (r.Site != "" && site != "" && !strings.EqualFold(r.Site, site)) || seen[strings.ToUpper(r.Key)] {
			continue
		}
		seen[strings.ToUpper(r.Key)] = true
		kept = append(kept, r)
	}
	return kept
}

func (c *client) searchSkipped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clock.Now().Before(c.noSearchTo)
}

// readEach reads refs one `workitem view` each, two at a time. A key that fails stays absent.
func (c *client) readEach(ctx context.Context, refs []issuetracker.Ref, out map[string]issuetracker.BatchIssue) {
	var (
		g  errgroup.Group
		mu sync.Mutex
	)
	g.SetLimit(batchFallbackFanOut)
	for _, ref := range refs {
		g.Go(func() error {
			iss, err := c.issue(ctx, ref, true)
			if err == nil {
				mu.Lock()
				out[ref.Key] = issuetracker.BatchIssue{Issue: iss}
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
}

// searchChunk runs one `workitem search` for chunk and caches every issue it returns.
func (c *client) searchChunk(ctx context.Context, st issuetracker.Status, chunk []issuetracker.Ref) (map[string]issuetracker.BatchIssue, error) {
	keys := make([]string, len(chunk))
	byKey := make(map[string]issuetracker.Ref, len(chunk))
	for i, r := range chunk {
		keys[i] = r.Key
		byKey[strings.ToUpper(r.Key)] = r
	}
	fields := searchFields
	devID := c.devInfo(st.Site).id
	if devID != "" {
		fields += "," + devID
	}
	sctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	// Keys come from ParseRef, which accepts only the Jira key pattern: the JQL holds no user text.
	raw, err := c.runner.Run(sctx, st.Path, []string{
		"jira", "workitem", "search", "--jql", "key in (" + strings.Join(keys, ", ") + ")",
		"--fields", fields, "--json", "--limit", strconv.Itoa(len(chunk)),
	})
	if err != nil {
		return nil, c.noteAuth(classify(err, sctx, "workitem search"))
	}
	items, ok := splitSearch(raw)
	if !ok {
		return nil, errors.New("jira: acli returned an unreadable search result")
	}
	out := make(map[string]issuetracker.BatchIssue, len(items))
	for _, item := range items {
		ref, bi, ok := c.batchIssue(item, st.Site, devID, byKey)
		if !ok {
			continue
		}
		c.store(cacheKey(ref), bi.Issue, nil)
		out[ref.Key] = bi
	}
	return out, nil
}

// splitSearch accepts a top-level array or an object holding issues.
func splitSearch(raw []byte) ([]json.RawMessage, bool) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) == nil {
		return items, true
	}
	var obj struct {
		Issues []json.RawMessage `json:"issues"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return nil, false
	}
	return obj.Issues, true
}

// batchIssue converts one search element. ok is false for a key nobody asked for or an unreadable item.
func (c *client) batchIssue(item json.RawMessage, site, devID string, byKey map[string]issuetracker.Ref) (issuetracker.Ref, issuetracker.BatchIssue, bool) {
	p, ok := parseIssue(item)
	if !ok {
		return issuetracker.Ref{}, issuetracker.BatchIssue{}, false
	}
	ref, ok := byKey[strings.ToUpper(p.key)]
	if !ok {
		return issuetracker.Ref{}, issuetracker.BatchIssue{}, false
	}
	iss := issuetracker.Issue{
		Ref:   fillRef(ref, site, p.key),
		Title: p.title, Status: p.status, StatusCategory: p.category,
	}
	if sum, found := parseDevSummary(p.fields[devID]); devID != "" && found {
		iss.Summary = sum
	} else {
		iss.Summary = c.cachedSummary(cacheKey(ref))
	}
	var prio struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(p.fields["priority"], &prio)
	var assignee personJSON
	_ = json.Unmarshal(p.fields["assignee"], &assignee)
	var updated string
	_ = json.Unmarshal(p.fields["updated"], &updated)
	return ref, issuetracker.BatchIssue{Issue: iss, Priority: prio.Name, Assignee: assignee.AccountID, UpdatedAt: parseJiraTime(updated)}, true
}

// cachedSummary is the Development summary of the kept copy of key, zero when there is none.
func (c *client) cachedSummary(key string) issuetracker.ChangeRequestSummary {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cache[key].issue.Summary
}
