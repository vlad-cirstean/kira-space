package jira

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseDetail(t *testing.T) {
	d, ok := parseDetail(readTestdata(t, "issue-detail.json"), "acme.atlassian.net", "")
	if !ok {
		t.Fatal("unreadable")
	}
	d.Normalize()
	if d.Title != "Export report as CSV" || d.Status != "In Progress" || d.StatusCategory != "indeterminate" ||
		d.Type != "Story" || d.Priority != "High" || d.Resolution != "" || d.DueDate != "2024-03-01" {
		t.Errorf("scalars %+v", d)
	}
	if d.Assignee != nil || d.Reporter == nil || d.Reporter.Name != "Rita Reporter" || d.Reporter.ID != "acc-rep" {
		t.Errorf("people %+v %+v", d.Assignee, d.Reporter)
	}
	if strings.Join(d.Labels, ",") != "backend,export" || strings.Join(d.Components, ",") != "Reports,API" {
		t.Errorf("labels %v components %v", d.Labels, d.Components)
	}
	if d.Parent == nil || d.Parent.Key != "PROJ-1" || d.Parent.Title != "Reporting epic" {
		t.Errorf("parent %+v", d.Parent)
	}
	if d.CreatedAt != 1704164645000 || d.UpdatedAt != 1706933106000 {
		t.Errorf("times %d %d", d.CreatedAt, d.UpdatedAt)
	}
	for _, want := range []string{"## Goal", "**monthly report**", "[the spec](https://example.com/spec)", "`kira export`",
		`\<script\>alert(1)\</script\>`, "- Columns\n  - date", "```sh\necho `date`", "| a\\|b | text |", "[@Dana Dev](#kira-mention)", "[IN REVIEW](#kira-status-blue)", "> [!NOTE]\n> Export runs nightly.", `![mockup.png](https://acme.atlassian.net/rest/api/3/attachment/content/10100)`} {
		if !strings.Contains(d.Description, want) {
			t.Errorf("description lacks %q:\n%s", want, d.Description)
		}
	}
	if len(d.Attachments) != 2 || d.Attachments[0].Filename != "spec.pdf" || d.Attachments[1].Size != 48213 ||
		d.Attachments[1].Author == nil || d.Attachments[1].Author.Name != "Rita Reporter" {
		t.Errorf("attachments %+v", d.Attachments)
	}
	if len(d.Comments) != 3 || d.CommentsTotal != 5 || !d.CommentsTruncated {
		t.Fatalf("comments %d total %d truncated %v", len(d.Comments), d.CommentsTotal, d.CommentsTruncated)
	}
	if d.Comments[1].Body != "Plain text comment\\\nwith a second line" || d.Comments[1].UpdatedAt <= d.Comments[1].CreatedAt {
		t.Errorf("string comment %+v", d.Comments[1])
	}
	if d.Comments[0].URL != "https://acme.atlassian.net/browse/PROJ-12?focusedCommentId=10001" {
		t.Errorf("comment url %q", d.Comments[0].URL)
	}

	p, ok := parseDetail(readTestdata(t, "issue-detail-plain.json"), "", "")
	if !ok || p.Assignee == nil || p.Assignee.Name != "Ann Assignee" || len(p.Comments) != 0 || p.CommentsTotal != 0 ||
		!strings.HasPrefix(p.Description, "h1. Title\\\nline two") || p.CreatedAt == 0 {
		t.Errorf("plain %+v", p)
	}
}

func TestParseJiraTime(t *testing.T) {
	if got := parseJiraTime("2024-01-02T03:04:05.000+0000"); got != 1704164645000 {
		t.Errorf("jira layout %d", got)
	}
	if got := parseJiraTime("2024-01-02T03:04:05.5+01:00"); got != 1704161045500 {
		t.Errorf("rfc3339 %d", got)
	}
	if parseJiraTime("nope") != 0 {
		t.Error("unparsable")
	}
}

func TestParseCommentList(t *testing.T) {
	arr := readTestdata(t, "comment-list.json")
	l, ok := parseCommentList(arr, nil)
	if !ok || len(l.comments) != 2 || l.comments[0].ID != "10008" || l.total != 0 {
		t.Fatalf("array %+v", l)
	}
	for name, wrap := range map[string]string{"comments": `{"total":9,"comments":%s}`, "values": `{"values":%s}`} {
		l, ok := parseCommentList([]byte(strings.Replace(wrap, "%s", string(arr), 1)), nil)
		if !ok || len(l.comments) != 2 || l.comments[0].ID != "10008" {
			t.Errorf("%s form %+v", name, l)
		}
	}
	if _, ok := parseCommentList([]byte(`"x"`), nil); ok {
		t.Error("string accepted")
	}
}

// detailRunner scripts the three views detail issues: full, without comments, and the comment list.
type detailRunner struct {
	scriptRunner
	full, noComments func() ([]byte, error)
	list             func() ([]byte, error)
	lists            atomic.Int32
}

func newDetailRunner(t *testing.T) *detailRunner {
	d := readTestdata(t, "issue-detail.json")
	r := &detailRunner{
		full:       func() ([]byte, error) { return d, nil },
		noComments: func() ([]byte, error) { return d, nil },
		list:       func() ([]byte, error) { return readTestdata(t, "comment-list.json"), nil },
	}
	r.view = func(a []string) ([]byte, error) {
		switch {
		case a[2] == "comment":
			r.lists.Add(1)
			return r.list()
		case isDetailFields(a[len(a)-1]) && strings.Contains(a[len(a)-1], ",comment"):
			return r.full()
		case isDetailFields(a[len(a)-1]):
			return r.noComments()
		}
		return fixtureView(t, "issue-no-dev.json")(a)
	}
	return r
}

func isDetailFields(f string) bool { return strings.HasPrefix(f, DetailFieldsNoComments) }

var detailRef = issuetracker.Ref{Provider: ProviderID, Key: "PROJ-12"}

func TestDetailTooLargeFallsBackToCommentList(t *testing.T) {
	r := newDetailRunner(t)
	r.full = func() ([]byte, error) { return nil, toolexec.ErrOutputTooLarge }
	p, _ := newTestProvider(r)
	d, err := p.IssueDetail(context.Background(), detailRef, false)
	if err != nil || len(d.Comments) != 2 || d.Comments[1].ID != "10009" || d.CommentsOmitted || r.lists.Load() != 1 {
		t.Fatalf("%+v %v", d.Comments, err)
	}
	if d.Comments[0].URL != "https://acme.atlassian.net/browse/PROJ-12?focusedCommentId=10008" || d.CommentsTotal != 2 || d.CommentsTruncated {
		t.Errorf("total %d truncated %v url %q", d.CommentsTotal, d.CommentsTruncated, d.Comments[0].URL)
	}
	if d.Ref.URL != "https://acme.atlassian.net/browse/PROJ-12" || d.Ref.Site != "acme.atlassian.net" || d.Title == "" {
		t.Errorf("ref %+v", d.Ref)
	}
}

func TestDetailCommentListFailureOmitsComments(t *testing.T) {
	for name, fail := range map[string]error{"too large": toolexec.ErrOutputTooLarge, "error": errors.New("boom")} {
		t.Run(name, func(t *testing.T) {
			r := newDetailRunner(t)
			r.full = func() ([]byte, error) { return nil, toolexec.ErrOutputTooLarge }
			r.list = func() ([]byte, error) { return nil, fail }
			p, _ := newTestProvider(r)
			d, err := p.IssueDetail(context.Background(), detailRef, false)
			if err != nil || !d.CommentsOmitted || d.Title == "" {
				t.Fatalf("%+v %v", d, err)
			}
		})
	}
}

func TestDetailSecondTooLargeFails(t *testing.T) {
	r := newDetailRunner(t)
	r.full = func() ([]byte, error) { return nil, toolexec.ErrOutputTooLarge }
	r.noComments = r.full
	p, _ := newTestProvider(r)
	if _, err := p.IssueDetail(context.Background(), detailRef, false); !errors.Is(err, toolexec.ErrOutputTooLarge) {
		t.Fatalf("err %v", err)
	}
}

func TestDetailIssueFailureKeepsDetail(t *testing.T) {
	r := newDetailRunner(t)
	base := r.view
	r.view = func(a []string) ([]byte, error) {
		if !isDetailFields(a[len(a)-1]) {
			return nil, errors.New("boom")
		}
		return base(a)
	}
	p, _ := newTestProvider(r)
	d, err := p.IssueDetail(context.Background(), detailRef, false)
	if err != nil || d.Title == "" || d.Summary.Known {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestDetailReadyErrorsPassThrough(t *testing.T) {
	r := newDetailRunner(t)
	p, _ := newTestProvider(r)
	other := detailRef
	other.Site = "other.atlassian.net"
	if _, err := p.IssueDetail(context.Background(), other, false); !errors.Is(err, issuetracker.ErrOtherSite) {
		t.Fatalf("err %v", err)
	}
	r.view = func([]string) ([]byte, error) {
		return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "issue does not exist"}
	}
	if _, err := p.IssueDetail(context.Background(), detailRef, false); !errors.Is(err, issuetracker.ErrNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestDetailFeedsIssueCache(t *testing.T) {
	r := newDetailRunner(t)
	p, _ := newTestProvider(r)
	if _, err := p.IssueDetail(context.Background(), detailRef, false); err != nil {
		t.Fatal(err)
	}
	before := r.views.Load()
	iss, err := p.Issue(context.Background(), issuetracker.Ref{Provider: ProviderID, Key: "PROJ-12", Site: "acme.atlassian.net"})
	if err != nil || iss.Title != "Export report as CSV" || r.views.Load() != before {
		t.Fatalf("%+v %v views %d->%d", iss, err, before, r.views.Load())
	}
}

func TestDetailCacheRules(t *testing.T) {
	r := newDetailRunner(t)
	var reads atomic.Int32
	r.full = func() ([]byte, error) { reads.Add(1); return readTestdata(t, "issue-detail.json"), nil }
	p, clk := newTestProvider(r)
	ctx := context.Background()
	get := func(fresh bool) issuetracker.IssueDetail {
		t.Helper()
		d, err := p.IssueDetail(ctx, detailRef, fresh)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d := get(false); d.Stale || d.FetchedAt == 0 || reads.Load() != 1 {
		t.Fatalf("first read %+v reads %d", d, reads.Load())
	}
	clk.advance(detailFreshFor - time.Second)
	if d := get(false); d.Stale || reads.Load() != 1 {
		t.Fatalf("within fresh window: stale %v reads %d", d.Stale, reads.Load())
	}
	clk.advance(2 * time.Second)
	if d := get(false); !d.Stale || reads.Load() != 1 {
		t.Fatalf("past fresh window: stale %v reads %d", d.Stale, reads.Load())
	}
	if d := get(true); d.Stale || reads.Load() != 2 {
		t.Fatalf("fresh=true: stale %v reads %d", d.Stale, reads.Load())
	}
	if err := p.PostIssueComment(ctx, detailRef, "hi"); err != nil {
		t.Fatal(err)
	}
	get(false)
	if reads.Load() != 3 {
		t.Fatalf("after post reads %d, want 3", reads.Load())
	}
	clk.advance(detailKeepFor + time.Second)
	get(false)
	if reads.Load() != 4 {
		t.Fatalf("past keep window reads %d, want 4", reads.Load())
	}
	r.full = func() ([]byte, error) { reads.Add(1); return nil, errors.New("boom") }
	clk.advance(detailKeepFor + time.Second)
	if _, err := p.IssueDetail(ctx, detailRef, false); err == nil {
		t.Fatal("error swallowed")
	}
	r.full = func() ([]byte, error) { reads.Add(1); return readTestdata(t, "issue-detail.json"), nil }
	get(false)
	if reads.Load() != 6 {
		t.Fatalf("error was cached: reads %d, want 6", reads.Load())
	}
}

func TestDetailOverflowParallel(t *testing.T) {
	r := newDetailRunner(t)
	r.full = func() ([]byte, error) { return nil, toolexec.ErrOutputTooLarge }
	started := make(chan struct{})
	var once sync.Once
	r.list = func() ([]byte, error) {
		once.Do(func() { close(started) })
		return readTestdata(t, "comment-list.json"), nil
	}
	d := readTestdata(t, "issue-detail.json")
	r.noComments = func() ([]byte, error) {
		select {
		case <-started:
			return d, nil
		case <-time.After(5 * time.Second):
			return nil, errors.New("comment list never started beside the view")
		}
	}
	p, _ := newTestProvider(r)
	got, err := p.IssueDetail(context.Background(), detailRef, true)
	if err != nil || len(got.Comments) != 2 {
		t.Fatalf("%+v %v", got.Comments, err)
	}
}

func TestDetailInFlightReadDoesNotSaveAcrossPost(t *testing.T) {
	r := newDetailRunner(t)
	var reads atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	d := readTestdata(t, "issue-detail.json")
	r.full = func() ([]byte, error) {
		if reads.Add(1) == 1 {
			close(started)
			<-release
		}
		return d, nil
	}
	p, _ := newTestProvider(r)
	ctx := context.Background()
	done := make(chan error, 1)
	go func() { _, err := p.IssueDetail(ctx, detailRef, true); done <- err }()
	<-started
	if err := p.PostIssueComment(ctx, detailRef, "hi"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, err := p.IssueDetail(ctx, detailRef, false); err != nil || got.Stale || reads.Load() != 2 {
		t.Fatalf("pre-post copy was saved: reads %d err %v", reads.Load(), err)
	}
}
