package jira

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// DetailFields is what the issue view reads in one `workitem view`.
const DetailFields = "summary,status,issuetype,priority,resolution,assignee,reporter,labels,components,created,updated,duedate,parent,description,comment"

// DetailFieldsNoComments is DetailFields for an issue whose comments overflow acli's output cap.
const DetailFieldsNoComments = "summary,status,issuetype,priority,resolution,assignee,reporter,labels,components,created,updated,duedate,parent,description"

const (
	detailBound      = 25 * time.Second
	commentListLimit = "50"
)

var _ issuetracker.IssueDetails = (*Provider)(nil)

func (p *Provider) IssueDetail(ctx context.Context, ref issuetracker.Ref) (issuetracker.IssueDetail, error) {
	return p.client.detail(ctx, ref)
}

type detailResult struct {
	d   issuetracker.IssueDetail
	err error
}

// detail reads ref fresh: it drops the cached Issue first. Concurrent
// callers share one read; the caller's context bounds it.
func (c *client) detail(ctx context.Context, ref issuetracker.Ref) (issuetracker.IssueDetail, error) {
	st, err := c.ready(ctx, ref)
	if err != nil {
		return issuetracker.IssueDetail{}, err
	}
	key := cacheKey(ref)
	v, _, _ := c.sf.Do("detail|"+key, func() (any, error) {
		dctx, cancel := context.WithTimeout(ctx, detailBound)
		defer cancel()
		c.forget(key)
		d, err := c.readDetail(dctx, st, ref)
		return detailResult{d, err}, nil
	})
	r := v.(detailResult)
	return r.d, r.err
}

func (c *client) readDetail(ctx context.Context, st issuetracker.Status, ref issuetracker.Ref) (issuetracker.IssueDetail, error) {
	var (
		wg      sync.WaitGroup
		iss     issuetracker.Issue
		issErr  error
		raw     detailRaw
		viewErr error
	)
	wg.Add(2)
	go func() { defer wg.Done(); iss, issErr = c.issue(ctx, ref) }()
	go func() { defer wg.Done(); raw, viewErr = c.detailView(ctx, st.Path, ref.Key) }()
	wg.Wait()
	if viewErr != nil {
		return issuetracker.IssueDetail{}, viewErr
	}
	d, ok := parseDetail(raw.out, st.Site)
	if !ok {
		return issuetracker.IssueDetail{}, errors.New("jira: acli returned an unreadable work item")
	}
	if raw.list != nil {
		d.Comments, d.CommentsTotal = raw.list.comments, max(raw.list.total, len(raw.list.comments))
		d.CommentsTruncated = raw.list.total == 0 && len(raw.list.comments) >= commentListMax
		for i := range d.Comments {
			d.Comments[i].URL = commentURL(st.Site, d.Ref.Key, d.Comments[i].ID)
		}
	}
	d.CommentsOmitted = raw.omitted

	d.Ref = ref
	d.Ref.Key = firstNonEmpty(d.Ref.Key, ref.Key)
	if st.Site != "" {
		d.Ref.URL = "https://" + st.Site + "/browse/" + d.Ref.Key
	}
	if d.Ref.Site == "" {
		d.Ref.Site = st.Site
	}
	if issErr == nil {
		d.Summary = iss.Summary
	}
	d.Normalize()
	return d, nil
}

const commentListMax = 50

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type commentList struct {
	comments []issuetracker.Comment
	total    int // 0 when the list did not say
}

type detailRaw struct {
	out     []byte
	list    *commentList // set when comments came from `comment list`
	omitted bool         // comments could not be read at all
}

// detailView reads the work item. When the output overflows the cap it re-reads without comments and
// takes them from `comment list`, which pages and so stays small.
func (c *client) detailView(ctx context.Context, path, key string) (detailRaw, error) {
	out, err := c.view(ctx, path, key, DetailFields)
	if err == nil {
		return detailRaw{out: out}, nil
	}
	if !errors.Is(err, toolexec.ErrOutputTooLarge) {
		return detailRaw{}, err
	}
	out, err = c.view(ctx, path, key, DetailFieldsNoComments)
	if err != nil {
		return detailRaw{}, err
	}
	list, err := c.commentList(ctx, path, key)
	if err != nil {
		return detailRaw{out: out, omitted: true}, nil
	}
	return detailRaw{out: out, list: list}, nil
}

func (c *client) commentList(ctx context.Context, path, key string) (*commentList, error) {
	vctx, cancel := context.WithTimeout(ctx, viewTimeout)
	defer cancel()
	out, err := c.runner.Run(vctx, path, []string{"jira", "workitem", "comment", "list", "--key", key, "--json", "--limit", commentListLimit, "--order", "-created"})
	if err != nil {
		return nil, classify(err, vctx, "workitem comment list")
	}
	l, ok := parseCommentList(out)
	if !ok {
		return nil, errors.New("jira: acli returned an unreadable comment list")
	}
	return &l, nil
}

// parseCommentList accepts a top-level array or an object holding comments or values, newest first,
// and returns the comments oldest first.
func parseCommentList(out []byte) (commentList, bool) {
	var items []json.RawMessage
	var total int
	if json.Unmarshal(out, &items) != nil {
		var obj struct {
			Comments []json.RawMessage `json:"comments"`
			Values   []json.RawMessage `json:"values"`
			Total    int               `json:"total"`
		}
		if json.Unmarshal(out, &obj) != nil {
			return commentList{}, false
		}
		items, total = obj.Comments, obj.Total
		if items == nil {
			items = obj.Values
		}
	}
	l := commentList{total: total, comments: make([]issuetracker.Comment, 0, len(items))}
	for i := len(items) - 1; i >= 0; i-- {
		if c, ok := parseComment(items[i]); ok {
			l.comments = append(l.comments, c)
		}
	}
	return l, true
}

type personJSON struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

func (p *personJSON) person() *issuetracker.Person {
	if p == nil || (p.AccountID == "" && p.DisplayName == "") {
		return nil
	}
	return &issuetracker.Person{ID: p.AccountID, Name: p.DisplayName}
}

func parseComment(raw json.RawMessage) (issuetracker.Comment, bool) {
	var c struct {
		ID      string          `json:"id"`
		Author  *personJSON     `json:"author"`
		Body    json.RawMessage `json:"body"`
		Created string          `json:"created"`
		Updated string          `json:"updated"`
	}
	if json.Unmarshal(raw, &c) != nil || c.ID == "" {
		return issuetracker.Comment{}, false
	}
	out := issuetracker.Comment{ID: c.ID, Body: richText(c.Body), CreatedAt: parseJiraTime(c.Created), UpdatedAt: parseJiraTime(c.Updated)}
	if a := c.Author.person(); a != nil {
		out.Author = *a
	}
	return out, true
}

// richText converts an ADF object or a plain string body to Markdown.
func richText(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return plainMarkdown(s)
	}
	return adfMarkdown(raw)
}

func commentURL(site, key, id string) string {
	if site == "" || key == "" || id == "" {
		return ""
	}
	return "https://" + site + "/browse/" + key + "?focusedCommentId=" + id
}

// parseJiraTime reads Jira's 2024-01-02T03:04:05.000+0000 and RFC 3339 into unix ms; 0 when unreadable.
func parseJiraTime(s string) int64 {
	for _, layout := range []string{"2006-01-02T15:04:05.000-0700", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func names(raw json.RawMessage) []string {
	var items []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &items)
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Name != "" {
			out = append(out, it.Name)
		}
	}
	return out
}

func nameOf(raw json.RawMessage) string {
	var v struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &v)
	return v.Name
}

// parseDetail reads the `workitem view --json` payload of DetailFields. Comments come from the payload's
// comment field; site fills their links.
func parseDetail(out []byte, site string) (issuetracker.IssueDetail, bool) {
	p, ok := parseIssue(out)
	if !ok {
		return issuetracker.IssueDetail{}, false
	}
	f := p.fields
	d := issuetracker.IssueDetail{
		Ref:   issuetracker.Ref{Provider: ProviderID, Key: p.key, Site: site},
		Title: p.title, Status: p.status, StatusCategory: p.category,
		Type: nameOf(f["issuetype"]), Priority: nameOf(f["priority"]), Resolution: nameOf(f["resolution"]),
		Labels: []string{}, Components: names(f["components"]),
		CreatedAt: timeField(f["created"]), UpdatedAt: timeField(f["updated"]),
		Description: richText(f["description"]),
	}
	_ = json.Unmarshal(f["labels"], &d.Labels)
	var as, rp *personJSON
	_ = json.Unmarshal(f["assignee"], &as)
	_ = json.Unmarshal(f["reporter"], &rp)
	d.Assignee, d.Reporter = as.person(), rp.person()
	_ = json.Unmarshal(f["duedate"], &d.DueDate)
	var par struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
		} `json:"fields"`
	}
	if json.Unmarshal(f["parent"], &par) == nil && par.Key != "" {
		d.Parent = &issuetracker.IssueLink{Key: par.Key, Title: par.Fields.Summary}
	}
	var cm struct {
		Comments []json.RawMessage `json:"comments"`
		Total    int               `json:"total"`
	}
	if json.Unmarshal(f["comment"], &cm) == nil {
		for _, raw := range cm.Comments {
			if c, ok := parseComment(raw); ok {
				c.URL = commentURL(site, p.key, c.ID)
				d.Comments = append(d.Comments, c)
			}
		}
		d.CommentsTotal = max(cm.Total, len(d.Comments))
	}
	return d, true
}

func timeField(raw json.RawMessage) int64 {
	var s string
	_ = json.Unmarshal(raw, &s)
	return parseJiraTime(s)
}
