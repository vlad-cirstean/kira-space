package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// DetailFieldsNoComments is what the issue view reads in one `workitem view`, without comments (used
// when they overflow acli's output cap). The Development field id, once known, is appended.
const DetailFieldsNoComments = "summary,status,issuetype,priority,resolution,assignee,reporter,labels,components,created,updated,duedate,parent,description,attachment"

// DetailFields is DetailFieldsNoComments plus the comments.
const DetailFields = DetailFieldsNoComments + ",comment"

// detailFields builds the field list of one detail read.
func detailFields(devID string, comments bool) string {
	f := DetailFieldsNoComments
	if comments {
		f += ",comment"
	}
	if devID != "" {
		f += "," + devID
	}
	return f
}

const (
	detailBound      = 25 * time.Second
	detailFreshFor   = 30 * time.Second
	detailKeepFor    = 10 * time.Minute
	commentListLimit = "50"
)

var _ issuetracker.IssueDetails = (*Provider)(nil)

func (p *Provider) IssueDetail(ctx context.Context, ref issuetracker.Ref, fresh bool) (issuetracker.IssueDetail, error) {
	return p.client.detail(ctx, ref, fresh)
}

type detailResult struct {
	d   issuetracker.IssueDetail
	err error
}

// detail answers from the saved copy unless fresh: younger than detailFreshFor as is, younger than
// detailKeepFor marked Stale. Else one coalesced read per key. The read runs on a context detached from
// the caller, so a caller that gives up leaves it running for the next one.
func (c *client) detail(ctx context.Context, ref issuetracker.Ref, fresh bool) (issuetracker.IssueDetail, error) {
	st, err := c.ready(ctx, ref)
	if err != nil {
		return issuetracker.IssueDetail{}, err
	}
	if ref.Site == "" {
		ref.Site = st.Site // a bare key and its link share one saved copy
	}
	key := cacheKey(ref)
	if !fresh {
		if d, ok := c.savedDetail(key); ok {
			return d, nil
		}
	}
	ch := c.sf.DoChan("detail|"+key, func() (any, error) {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), detailBound)
		defer cancel()
		d, err := c.readDetail(dctx, st, ref)
		if err == nil {
			c.saveDetail(key, d)
		}
		return detailResult{d, err}, nil
	})
	select {
	case r := <-ch:
		v := r.Val.(detailResult)
		return v.d, v.err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return issuetracker.IssueDetail{}, fmt.Errorf("%w: %v", issuetracker.ErrTimeout, ctx.Err())
		}
		return issuetracker.IssueDetail{}, ctx.Err()
	}
}

func (c *client) savedDetail(key string) (issuetracker.IssueDetail, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.details[key]
	if !ok {
		return issuetracker.IssueDetail{}, false
	}
	age := c.clock.Now().Sub(e.at)
	if age >= detailKeepFor {
		delete(c.details, key)
		return issuetracker.IssueDetail{}, false
	}
	d := e.d
	d.Stale = age >= detailFreshFor
	return d, true
}

// saveDetail keeps d and refreshes the Issue result cache from it, so the next Issue read is free.
func (c *client) saveDetail(key string, d issuetracker.IssueDetail) {
	c.mu.Lock()
	c.details[key] = detailEntry{d: d, at: c.clock.Now()}
	c.mu.Unlock()
	c.store(key, issuetracker.Issue{
		Ref: d.Ref, Title: d.Title, Status: d.Status, StatusCategory: d.StatusCategory, Summary: d.Summary,
	}, nil)
}

func (c *client) forgetDetail(key string) {
	c.mu.Lock()
	delete(c.details, key)
	c.mu.Unlock()
}

// readDetail reads the work item in one acli call. Only a site's first read also runs c.issue, which
// finds the Development field id.
func (c *client) readDetail(ctx context.Context, st issuetracker.Status, ref issuetracker.Ref) (issuetracker.IssueDetail, error) {
	info := c.devInfo(st.Site)
	discover := info.id == "" && !c.clock.Now().Before(info.noDevTo)
	var (
		wg     sync.WaitGroup
		iss    issuetracker.Issue
		issErr error
	)
	if discover {
		wg.Add(1)
		go func() { defer wg.Done(); iss, issErr = c.issue(ctx, ref) }()
	}
	raw, viewErr := c.detailView(ctx, st.Path, ref.Key, info.id)
	wg.Wait()
	if viewErr != nil {
		return issuetracker.IssueDetail{}, viewErr
	}
	d, ok := parseDetail(raw.out, st.Site, info.id)
	if !ok {
		return issuetracker.IssueDetail{}, errors.New("jira: acli returned an unreadable work item")
	}
	if raw.list != nil {
		l := raw.list.convert(mediaResolver(d.Attachments))
		d.Comments, d.CommentsTotal = l.comments, max(l.total, len(l.comments))
		d.CommentsTruncated = l.total == 0 && len(l.comments) >= commentListMax
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
	if discover && issErr == nil {
		d.Summary = iss.Summary
	}
	d.FetchedAt = c.clock.Now().UnixMilli()
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

// commentPage is a `comment list` answer split into comments, newest first, not yet converted: the
// attachment URLs that media nodes resolve to come from the work item read beside it.
type commentPage struct {
	items []json.RawMessage
	total int
}

type detailRaw struct {
	out     []byte
	list    *commentPage // set when comments came from `comment list`
	omitted bool         // comments could not be read at all
}

// detailView reads the work item. When the output overflows the cap it re-reads without comments and
// takes them from `comment list`, which pages and so stays small; the two run at once.
func (c *client) detailView(ctx context.Context, path, key, devID string) (detailRaw, error) {
	out, err := c.view(ctx, path, key, detailFields(devID, true))
	if err == nil {
		return detailRaw{out: out}, nil
	}
	if !errors.Is(err, toolexec.ErrOutputTooLarge) {
		return detailRaw{}, err
	}
	var (
		wg      sync.WaitGroup
		list    *commentPage
		listErr error
	)
	wg.Add(1)
	go func() { defer wg.Done(); list, listErr = c.commentList(ctx, path, key) }()
	out, err = c.view(ctx, path, key, detailFields(devID, false))
	wg.Wait()
	if err != nil {
		return detailRaw{}, err
	}
	if listErr != nil {
		return detailRaw{out: out, omitted: true}, nil
	}
	return detailRaw{out: out, list: list}, nil
}

func (c *client) commentList(ctx context.Context, path, key string) (*commentPage, error) {
	vctx, cancel := context.WithTimeout(ctx, viewTimeout)
	defer cancel()
	out, err := c.runner.Run(vctx, path, []string{"jira", "workitem", "comment", "list", "--key", key, "--json", "--limit", commentListLimit, "--order", "-created"})
	if err != nil {
		return nil, classify(err, vctx, "workitem comment list")
	}
	l, ok := splitCommentList(out)
	if !ok {
		return nil, errors.New("jira: acli returned an unreadable comment list")
	}
	return &l, nil
}

// splitCommentList accepts a top-level array or an object holding comments or values, newest first.
func splitCommentList(out []byte) (commentPage, bool) {
	var items []json.RawMessage
	var total int
	if json.Unmarshal(out, &items) != nil {
		var obj struct {
			Comments []json.RawMessage `json:"comments"`
			Values   []json.RawMessage `json:"values"`
			Total    int               `json:"total"`
		}
		if json.Unmarshal(out, &obj) != nil {
			return commentPage{}, false
		}
		items, total = obj.Comments, obj.Total
		if items == nil {
			items = obj.Values
		}
	}
	return commentPage{items: items, total: total}, true
}

// convert returns the comments oldest first.
func (p commentPage) convert(m mediaURL) commentList {
	l := commentList{total: p.total, comments: make([]issuetracker.Comment, 0, len(p.items))}
	for i := len(p.items) - 1; i >= 0; i-- {
		if c, ok := parseComment(p.items[i], m); ok {
			l.comments = append(l.comments, c)
		}
	}
	return l
}

func parseCommentList(out []byte, m mediaURL) (commentList, bool) {
	p, ok := splitCommentList(out)
	if !ok {
		return commentList{}, false
	}
	return p.convert(m), true
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

func parseComment(raw json.RawMessage, m mediaURL) (issuetracker.Comment, bool) {
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
	out := issuetracker.Comment{ID: c.ID, Body: richText(c.Body, m), CreatedAt: parseJiraTime(c.Created), UpdatedAt: parseJiraTime(c.Updated)}
	if a := c.Author.person(); a != nil {
		out.Author = *a
	}
	return out, true
}

// richText converts an ADF object or a plain string body to Markdown.
func richText(raw json.RawMessage, m mediaURL) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return plainMarkdown(s)
	}
	return adfMarkdown(raw, m)
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
func parseDetail(out []byte, site, devID string) (issuetracker.IssueDetail, bool) {
	p, ok := parseIssue(out)
	if !ok {
		return issuetracker.IssueDetail{}, false
	}
	f := p.fields
	atts := parseAttachments(f["attachment"])
	m := mediaResolver(atts)
	d := issuetracker.IssueDetail{
		Ref:   issuetracker.Ref{Provider: ProviderID, Key: p.key, Site: site},
		Title: p.title, Status: p.status, StatusCategory: p.category,
		Type: nameOf(f["issuetype"]), Priority: nameOf(f["priority"]), Resolution: nameOf(f["resolution"]),
		Labels: []string{}, Components: names(f["components"]),
		CreatedAt: timeField(f["created"]), UpdatedAt: timeField(f["updated"]),
		Description: richText(f["description"], m), Attachments: atts,
	}
	if devID != "" {
		d.Summary, _ = parseDevSummary(f[devID])
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
			if c, ok := parseComment(raw, m); ok {
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

// parseAttachments reads fields.attachment: id, filename, mimeType, size, created, author, content.
func parseAttachments(raw json.RawMessage) []issuetracker.Attachment {
	var items []struct {
		ID       string      `json:"id"`
		Filename string      `json:"filename"`
		MimeType string      `json:"mimeType"`
		Size     int64       `json:"size"`
		Created  string      `json:"created"`
		Author   *personJSON `json:"author"`
		Content  string      `json:"content"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	out := make([]issuetracker.Attachment, 0, len(items))
	for _, it := range items {
		if it.ID == "" {
			continue
		}
		out = append(out, issuetracker.Attachment{
			ID: it.ID, Filename: it.Filename, MimeType: it.MimeType, Size: it.Size,
			CreatedAt: parseJiraTime(it.Created), Author: it.Author.person(), URL: it.Content,
		})
	}
	return out
}

// mediaResolver maps a file name in an ADF media node to its attachment content URL. Only https
// URLs resolve; the newest of several same-named attachments wins.
func mediaResolver(atts []issuetracker.Attachment) mediaURL {
	if len(atts) == 0 {
		return nil
	}
	byName := make(map[string]issuetracker.Attachment, len(atts))
	for _, a := range atts {
		if !strings.HasPrefix(a.URL, "https://") {
			continue
		}
		if old, ok := byName[a.Filename]; !ok || a.CreatedAt > old.CreatedAt {
			byName[a.Filename] = a
		}
	}
	return func(name string) string { return byName[name].URL }
}
