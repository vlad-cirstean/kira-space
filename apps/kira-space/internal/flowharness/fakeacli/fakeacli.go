// Package fakeacli is an in-process stand-in for the Atlassian CLI in flow tests. It is both the
// jira.Locator and the jira.Runner, so no process spawns and the host's real acli is never reached.
package fakeacli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// Acli answers acli by argv from a scripted state. Every call is recorded.
type Acli struct {
	mu        sync.Mutex
	installed bool
	loggedIn  bool
	site      string
	issues    map[string]string
	details   map[string]string
	comments  map[string]string
	posted    map[string][]PostedComment
	tooLarge  map[string]bool
	gate      chan struct{}
	calls     []string
}

// New returns an installed, logged-in acli for site with no issues.
func New(site string) *Acli {
	return &Acli{installed: true, loggedIn: true, site: site, issues: map[string]string{}, details: map[string]string{}, comments: map[string]string{}, tooLarge: map[string]bool{}, posted: map[string][]PostedComment{}}
}

// SetInstalled makes the binary present or absent.
func (a *Acli) SetInstalled(v bool) { a.mu.Lock(); a.installed = v; a.mu.Unlock() }

// SetLoggedIn signs the CLI in or out.
func (a *Acli) SetLoggedIn(v bool) { a.mu.Lock(); a.loggedIn = v; a.mu.Unlock() }

// SetIssue serves raw as the JSON of workitem view KEY.
func (a *Acli) SetIssue(key, raw string) { a.mu.Lock(); a.issues[key] = raw; a.mu.Unlock() }

// SetIssueDetail serves raw as the JSON of workitem view KEY when --fields starts with
// jira.DetailFieldsNoComments (comments only when it holds ",comment"). A trailing customfield_ id
// copies that field from the SetIssue JSON into the served detail. The call records "detail:<KEY>".
// Another --fields keeps the SetIssue path.
func (a *Acli) SetIssueDetail(key, raw string) { a.mu.Lock(); a.details[key] = raw; a.mu.Unlock() }

// SetTooLarge makes a detail view of KEY that asks for comments overflow acli's output cap.
func (a *Acli) SetTooLarge(key string, v bool) { a.mu.Lock(); a.tooLarge[key] = v; a.mu.Unlock() }

// SetComments serves raw as the JSON of workitem comment list for KEY. An empty raw removes it.
func (a *Acli) SetComments(key, raw string) {
	a.mu.Lock()
	if raw == "" {
		delete(a.comments, key)
	} else {
		a.comments[key] = raw
	}
	a.mu.Unlock()
}

// Hold blocks every workitem view and comment list until the returned release func is called or the caller gives up.
func (a *Acli) Hold() (release func()) {
	gate := make(chan struct{})
	a.mu.Lock()
	a.gate = gate
	a.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

// Calls returns every recorded call, in order: "version", "auth", or "view:<KEY>", "detail:<KEY>" or "comments:<KEY>".
func (a *Acli) Calls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.calls)
}

// Count returns how many recorded calls start with prefix.
func (a *Acli) Count(prefix string) int {
	n := 0
	for _, c := range a.Calls() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// Locate implements jira.Locator.
func (a *Acli) Locate() (string, []string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.installed {
		return "", []string{"acli (on PATH)", "/opt/homebrew/bin/acli", "/usr/local/bin/acli"}, false
	}
	return "/fake/bin/acli", []string{"/fake/bin/acli"}, true
}

// Run implements jira.Runner.
func (a *Acli) Run(ctx context.Context, _ string, args []string) ([]byte, error) {
	a.mu.Lock()
	loggedIn, site, gate := a.loggedIn, a.site, a.gate
	a.mu.Unlock()
	switch {
	case len(args) == 1 && args[0] == "--version":
		a.record("version")
		return []byte("acli version 1.3.4-stable\n"), nil
	case slices.Equal(args, []string{"jira", "auth", "status"}):
		a.record("auth")
		if !loggedIn {
			return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "no Jira account is logged in"}
		}
		return []byte(fmt.Sprintf("Authenticated\n  Site: %s\n  Email: me@acme.test\n", site)), nil
	case len(args) == 8 && slices.Equal(args[:4], []string{"jira", "workitem", "comment", "create"}) && args[4] == "--key" && args[6] == "--body-file":
		return a.commentCreate(ctx, args[5], args[7], loggedIn, gate)
	case len(args) >= 4 && slices.Equal(args[:3], []string{"jira", "workitem", "view"}):
		return a.view(ctx, args, gate)
	case len(args) == 11 && slices.Equal(args[:4], []string{"jira", "workitem", "comment", "list"}) && args[4] == "--key" &&
		slices.Equal(args[6:], []string{"--json", "--limit", "50", "--order", "-created"}):
		key := args[5]
		a.record("comments:" + key)
		if err := wait(ctx, gate); err != nil {
			return nil, err
		}
		a.mu.Lock()
		raw, ok := a.comments[key]
		a.mu.Unlock()
		if !ok {
			return nil, notFound()
		}
		return []byte(raw), nil
	}
	return nil, &toolexec.ExecError{ExitCode: 2, Stderr: "fakeacli: unsupported: " + strings.Join(args, " ")}
}

// PostedComment is one `comment create`: the body file's content and the path acli was given.
type PostedComment struct{ Body, Path string }

// Posted returns the comments created on KEY, in order.
func (a *Acli) Posted(key string) []PostedComment {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.posted[key])
}

func (a *Acli) commentCreate(ctx context.Context, key, path string, loggedIn bool, gate chan struct{}) ([]byte, error) {
	a.record("comment-create:" + key)
	if err := wait(ctx, gate); err != nil {
		return nil, err
	}
	if !loggedIn {
		return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "no Jira account is logged in"}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "Error: cannot read body file"}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, inIssues := a.issues[key]
	_, inDetails := a.details[key]
	if !inIssues && !inDetails {
		return nil, notFound()
	}
	a.posted[key] = append(a.posted[key], PostedComment{Body: string(body), Path: path})
	return []byte("Comment added\n"), nil
}

func (a *Acli) view(ctx context.Context, args []string, gate chan struct{}) ([]byte, error) {
	key := args[3]
	fields := ""
	for i, v := range args {
		if v == "--fields" && i+1 < len(args) {
			fields = args[i+1]
		}
	}
	isDetail := strings.HasPrefix(fields, jira.DetailFieldsNoComments)
	withComments := strings.Contains(fields, ",comment")
	devField := ""
	if i := strings.LastIndex(fields, ","); i >= 0 && strings.HasPrefix(fields[i+1:], "customfield_") {
		devField = fields[i+1:]
	}
	a.mu.Lock()
	detail, hasDetail := a.details[key]
	large := a.tooLarge[key]
	raw, ok := a.issues[key]
	a.mu.Unlock()
	switch {
	case isDetail && hasDetail:
		a.record("detail:" + key)
	default:
		a.record("view:" + key)
	}
	if err := wait(ctx, gate); err != nil {
		return nil, err
	}
	if isDetail && hasDetail {
		if large && withComments {
			return nil, toolexec.ErrOutputTooLarge
		}
		if devField != "" && ok {
			detail = copyField(detail, raw, devField)
		}
		if !withComments {
			return dropComments(detail), nil
		}
		return []byte(detail), nil
	}
	if !ok {
		return nil, notFound()
	}
	return []byte(raw), nil
}

func wait(ctx context.Context, gate chan struct{}) error {
	if gate == nil {
		return nil
	}
	select {
	case <-gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func notFound() error {
	return &toolexec.ExecError{ExitCode: 1, Stderr: "Error: issue does not exist or you do not have permission to see it"}
}

func (a *Acli) record(call string) {
	a.mu.Lock()
	a.calls = append(a.calls, call)
	a.mu.Unlock()
}

// IssueJSON is the workitem view JSON of an issue with a Development field of prs pull requests, in the
// string form Jira serves it.
func IssueJSON(key, title, status, category string, prs int) string {
	dev := fmt.Sprintf(`{pullrequest={dataType=pullrequest, state=OPEN, stateCount=%d}, json={"cachedValue":{"errors":[],"summary":{"pullrequest":{"overall":{"count":%d,"state":"OPEN","dataType":"pullrequest","open":true},"byInstanceType":{"GitHub":{"count":%d,"name":"GitHub"}}}}},"isStale":false}}`, prs, prs, prs)
	b, err := json.Marshal(map[string]any{"key": key, "fields": map[string]any{
		"summary": title, "status": map[string]any{"name": status, "statusCategory": map[string]any{"key": category}},
		"customfield_10000": dev,
	}})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// copyField sets fields.<name> of dst to its value in src; dst unchanged when src lacks it.
func copyField(dst, src, name string) string {
	var s, d, sf, df map[string]json.RawMessage
	if json.Unmarshal([]byte(src), &s) != nil || json.Unmarshal(s["fields"], &sf) != nil || sf[name] == nil ||
		json.Unmarshal([]byte(dst), &d) != nil || json.Unmarshal(d["fields"], &df) != nil {
		return dst
	}
	df[name] = sf[name]
	d["fields"], _ = json.Marshal(df)
	out, err := json.Marshal(d)
	if err != nil {
		return dst
	}
	return string(out)
}

// dropComments removes fields.comment, as acli does when it is not asked for.
func dropComments(raw string) []byte {
	var all, fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &all) != nil || json.Unmarshal(all["fields"], &fields) != nil {
		return []byte(raw)
	}
	delete(fields, "comment")
	all["fields"], _ = json.Marshal(fields)
	out, err := json.Marshal(all)
	if err != nil {
		return []byte(raw)
	}
	return out
}
