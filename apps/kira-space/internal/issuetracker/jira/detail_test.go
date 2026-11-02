package jira

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

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
	d, ok := parseDetail(readTestdata(t, "issue-detail.json"), "acme.atlassian.net")
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
		`\<script\>alert(1)\</script\>`, "- Columns\n  - date", "```sh\necho `date`", "| a\\|b | text |", "@Dana Dev", `\[attachment: mockup.png\]`} {
		if !strings.Contains(d.Description, want) {
			t.Errorf("description lacks %q:\n%s", want, d.Description)
		}
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

	p, ok := parseDetail(readTestdata(t, "issue-detail-plain.json"), "")
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
	l, ok := parseCommentList(arr)
	if !ok || len(l.comments) != 2 || l.comments[0].ID != "10008" || l.total != 0 {
		t.Fatalf("array %+v", l)
	}
	for name, wrap := range map[string]string{"comments": `{"total":9,"comments":%s}`, "values": `{"values":%s}`} {
		l, ok := parseCommentList([]byte(strings.Replace(wrap, "%s", string(arr), 1)))
		if !ok || len(l.comments) != 2 || l.comments[0].ID != "10008" {
			t.Errorf("%s form %+v", name, l)
		}
	}
	if _, ok := parseCommentList([]byte(`"x"`)); ok {
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
		case a[len(a)-1] == DetailFields:
			return r.full()
		case a[len(a)-1] == DetailFieldsNoComments:
			return r.noComments()
		}
		return fixtureView(t, "issue-no-dev.json")(a)
	}
	return r
}

var detailRef = issuetracker.Ref{Provider: ProviderID, Key: "PROJ-12"}

func TestDetailTooLargeFallsBackToCommentList(t *testing.T) {
	r := newDetailRunner(t)
	r.full = func() ([]byte, error) { return nil, toolexec.ErrOutputTooLarge }
	p, _ := newTestProvider(r)
	d, err := p.IssueDetail(context.Background(), detailRef)
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
			d, err := p.IssueDetail(context.Background(), detailRef)
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
	if _, err := p.IssueDetail(context.Background(), detailRef); !errors.Is(err, toolexec.ErrOutputTooLarge) {
		t.Fatalf("err %v", err)
	}
}

func TestDetailIssueFailureKeepsDetail(t *testing.T) {
	r := newDetailRunner(t)
	base := r.view
	r.view = func(a []string) ([]byte, error) {
		if a[len(a)-1] != DetailFields {
			return nil, errors.New("boom")
		}
		return base(a)
	}
	p, _ := newTestProvider(r)
	d, err := p.IssueDetail(context.Background(), detailRef)
	if err != nil || d.Title == "" || d.Summary.Known {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestDetailReadyErrorsPassThrough(t *testing.T) {
	r := newDetailRunner(t)
	p, _ := newTestProvider(r)
	other := detailRef
	other.Site = "other.atlassian.net"
	if _, err := p.IssueDetail(context.Background(), other); !errors.Is(err, issuetracker.ErrOtherSite) {
		t.Fatalf("err %v", err)
	}
	r.view = func([]string) ([]byte, error) {
		return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "issue does not exist"}
	}
	if _, err := p.IssueDetail(context.Background(), detailRef); !errors.Is(err, issuetracker.ErrNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestDetailDropsCachedIssue(t *testing.T) {
	r := newDetailRunner(t)
	p, _ := newTestProvider(r)
	if _, err := p.Issue(context.Background(), issuetracker.Ref{Provider: ProviderID, Key: "PROJ-12"}); err != nil {
		t.Fatal(err)
	}
	before := r.views.Load()
	if _, err := p.IssueDetail(context.Background(), detailRef); err != nil {
		t.Fatal(err)
	}
	if r.views.Load()-before != 2 {
		t.Fatalf("detail ran %d views, want 2 (fresh issue + detail)", r.views.Load()-before)
	}
}
