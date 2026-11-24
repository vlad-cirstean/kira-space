// Package fakeaz is an in-process stand-in for the Azure CLI in flow tests. It is both the
// azure.Locator and the azure.Runner, so no process spawns and the host's real az is never reached.
// It answers every argv the real provider builds, projections included, and fails any
// organization-scoped call that lacks `--detect false`, so a flow guards the provider's argv helper.
package fakeaz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// Reviewer is one reviewer's vote: 10 approved, 5 approved with suggestions, 0 none, -5 waiting for
// the author, -10 rejected.
type Reviewer struct {
	Vote     int
	Required bool
}

// Policy is one policy evaluation. Type is the policy type's display name ("Build", "Status",
// "Minimum number of reviewers"); Status is queued, running, approved, rejected, notApplicable or broken.
type Policy struct {
	Type, Status string
}

// PR is one pull request. Src and Dst are branch names without refs/heads/. ForkRepo and
// ForkProject, when set, mark a pull request from a fork.
type PR struct {
	ID          int
	Title       string
	Status      string // active | completed | abandoned
	Draft       bool
	Src, Dst    string
	Head        string
	MergeSha    string
	Created     time.Time
	Author      string
	ForkRepo    string
	ForkProject string
	Reviewers   []Reviewer
	Policies    []Policy
	// MergeStatus (succeeded | conflicts) and AutoComplete (who set it) print in every read.
	MergeStatus  string
	AutoComplete string
}

// Key names one repository: "<org>/<project>/<repo>".
func Key(org, project, repo string) string { return org + "/" + project + "/" + repo }

type repoState struct{ prs []PR }

// Az answers az by argv from a scripted state. Every call is recorded.
type Az struct {
	mu         sync.Mutex
	installed  bool
	extension  bool
	loggedIn   bool
	account    string
	defOrg     string
	defProject string
	failure    string
	repos      map[string]*repoState
	votes      map[int][]string
	threads    map[int][]string
	contexts   map[int][]ThreadContext
	calls      []string
	violations []string
}

// New returns an installed, signed-in az with the azure-devops extension and no defaults.
func New() *Az {
	return &Az{
		installed: true, extension: true, loggedIn: true, account: "me@acme.test",
		repos: map[string]*repoState{}, votes: map[int][]string{}, threads: map[int][]string{}, contexts: map[int][]ThreadContext{},
	}
}

// SetInstalled makes the binary present or absent.
func (a *Az) SetInstalled(v bool) { a.set(func() { a.installed = v }) }

// SetExtension installs or removes the azure-devops extension.
func (a *Az) SetExtension(v bool) { a.set(func() { a.extension = v }) }

// SetLoggedIn signs az in or out.
func (a *Az) SetLoggedIn(v bool) { a.set(func() { a.loggedIn = v }) }

// SetDefaults sets `az devops configure --defaults`; either may be empty.
func (a *Az) SetDefaults(orgURL, project string) {
	a.set(func() { a.defOrg, a.defProject = orgURL, project })
}

// SetFailure makes every organization-scoped call after sign-in fail with stderr; "" clears it.
func (a *Az) SetFailure(stderr string) { a.set(func() { a.failure = stderr }) }

func (a *Az) set(f func()) { a.mu.Lock(); f(); a.mu.Unlock() }

// AddPR adds pr to repo (see Key).
func (a *Az) AddPR(repo string, pr PR) {
	a.set(func() {
		st := a.repos[repo]
		if st == nil {
			st = &repoState{}
			a.repos[repo] = st
		}
		st.prs = append(st.prs, pr)
	})
}

// SetPR changes pull request id in repo.
func (a *Az) SetPR(repo string, id int, change func(*PR)) {
	a.set(func() {
		if st := a.repos[repo]; st != nil {
			for i := range st.prs {
				if st.prs[i].ID == id {
					change(&st.prs[i])
				}
			}
		}
	})
}

// SetPolicies replaces the policy evaluations of pull request id in repo.
func (a *Az) SetPolicies(repo string, id int, ps ...Policy) {
	a.SetPR(repo, id, func(p *PR) { p.Policies = ps })
}

// RemovePR deletes pull request id from repo.
func (a *Az) RemovePR(repo string, id int) {
	a.set(func() {
		if st := a.repos[repo]; st != nil {
			st.prs = slices.DeleteFunc(st.prs, func(p PR) bool { return p.ID == id })
		}
	})
}

// Votes returns the --vote values cast on pull request id, in order.
func (a *Az) Votes(id int) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.votes[id])
}

// Threads returns the comment bodies posted as new threads on pull request id, in order.
func (a *Az) Threads(id int) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.threads[id])
}

// ThreadContext is the file anchor of one new thread: FilePath as sent, the right-side start and
// end lines, and the end offset. Empty for a thread posted without a threadContext.
type ThreadContext struct {
	FilePath       string
	StartLine, End int
	EndOffset      int
}

// ThreadContexts returns the threadContext of each new thread on pull request id, in order.
func (a *Az) ThreadContexts(id int) []ThreadContext {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.contexts[id])
}

// Calls returns every recorded call, in order: "version", "defaults", "project-list", "account",
// "pr-list:<repo key>:<status>", "pr-probe:<repo key>", "pr-show:<id>", "policy:<id>", "vote:<id>:<vote>",
// "invoke:<resource>:<repo key>".
func (a *Az) Calls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.calls)
}

// ResetCalls forgets every recorded call.
func (a *Az) ResetCalls() { a.set(func() { a.calls = nil }) }

// Count returns how many recorded calls start with prefix.
func (a *Az) Count(prefix string) int {
	n := 0
	for _, c := range a.Calls() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// Violations lists the calls that broke the argv contract (a missing `--detect false` or `--org`) or that the
// fake does not support (e.g. `account get-access-token`).
func (a *Az) Violations() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.violations)
}

// Locate implements azure.Locator.
func (a *Az) Locate() (string, []string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.installed {
		return "", []string{"az (on PATH)", "/opt/homebrew/bin/az", "/usr/local/bin/az"}, false
	}
	return "/fake/bin/az", []string{"/fake/bin/az"}, true
}

func fail(stderr string) error { return &toolexec.ExecError{ExitCode: 1, Stderr: stderr} }

const (
	loggedOut = "ERROR: Before you can run Azure DevOps commands, you need to run the login command(az login if using AAD/MSA identity else az devops login if using PAT token) to setup credentials."
	prMissing = "ERROR: TF401180: The requested pull request was not found."
)

// Run implements azure.Runner.
func (a *Az) Run(ctx context.Context, _ string, args []string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(args) == 0 {
		return nil, a.unsupported(args)
	}
	switch {
	case args[0] == "version":
		a.calls = append(a.calls, "version")
		ext := "{}"
		if a.extension {
			ext = `{"azure-devops":"1.0.10"}`
		}
		return []byte(`{"azure-cli":"2.91.0","azure-cli-core":"2.91.0","extensions":` + ext + `}`), nil
	case len(args) > 1 && args[0] == "account" && args[1] == "show":
		a.calls = append(a.calls, "account")
		if !a.loggedIn {
			return nil, fail("ERROR: Please run 'az login' to setup account.")
		}
		return []byte(a.account + "\n"), nil
	case slices.Equal(args[:min(len(args), 3)], []string{"devops", "configure", "--list"}):
		a.calls = append(a.calls, "defaults")
		return []byte(a.configureList()), nil
	}
	if !a.guard(args) {
		return nil, fail("ERROR: fakeaz: call lacks --org or --detect false")
	}
	if !a.loggedIn {
		return nil, fail(loggedOut)
	}
	if a.failure != "" {
		return nil, fail(a.failure)
	}
	switch {
	case has(args, "devops", "project", "list"):
		a.calls = append(a.calls, "project-list")
		return []byte(`[]`), nil
	case has(args, "repos", "pr", "list"):
		return a.prList(args)
	case has(args, "repos", "pr", "show"):
		return a.prShow(args)
	case has(args, "repos", "pr", "policy", "list"):
		return a.policyList(args)
	case has(args, "repos", "pr", "set-vote"):
		return a.setVote(args)
	case has(args, "devops", "invoke"):
		return a.invoke(args)
	}
	return nil, a.unsupported(args)
}

func (a *Az) unsupported(args []string) error {
	a.violations = append(a.violations, "unsupported: "+strings.Join(args, " "))
	return &toolexec.ExecError{ExitCode: 2, Stderr: "fakeaz: unsupported: " + strings.Join(args, " ")}
}

// guard records and rejects an organization-scoped call without `--org <url>` and `--detect false`.
func (a *Az) guard(args []string) bool {
	if flagValue(args, "--org") != "" && flagValue(args, "--detect") == "false" {
		return true
	}
	a.violations = append(a.violations, strings.Join(args, " "))
	return false
}

func (a *Az) configureList() string {
	if a.defOrg == "" && a.defProject == "" {
		return "Use git alias = No\n"
	}
	var b strings.Builder
	b.WriteString("[defaults]\n")
	if a.defOrg != "" {
		b.WriteString("organization = " + a.defOrg + "\n")
	}
	if a.defProject != "" {
		b.WriteString("project = " + a.defProject + "\n")
	}
	b.WriteString("\nUse git alias = No\n")
	return b.String()
}

func has(args []string, words ...string) bool {
	return len(args) >= len(words) && slices.Equal(args[:len(words)], words)
}

func flagValue(args []string, name string) string {
	for i, s := range args {
		if s == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func intFlag(args []string, name string, def int) int {
	if n, err := strconv.Atoi(flagValue(args, name)); err == nil {
		return n
	}
	return def
}

func orgOf(args []string) string {
	u, err := url.Parse(flagValue(args, "--org"))
	if err != nil {
		return ""
	}
	name, _ := url.PathUnescape(strings.Trim(u.EscapedPath(), "/"))
	return name
}

// projection mirrors the azure package's prFields projection.
type projection struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Draft       bool       `json:"draft"`
	Src         string     `json:"src"`
	Dst         string     `json:"dst"`
	Head        string     `json:"head"`
	Created     string     `json:"created"`
	Closed      *string    `json:"closed"`
	Author      string     `json:"author"`
	Repo        string     `json:"repo"`
	ForkRepo    *string    `json:"forkRepo"`
	ForkProject *string    `json:"forkProject"`
	Reviewers   []reviewer `json:"reviewers"`
	MergeStatus string     `json:"mergeStatus"`
	// AutoComplete is null while auto-complete is off.
	AutoComplete *string `json:"autoComplete"`
}

type reviewer struct {
	Vote     int  `json:"vote"`
	Required bool `json:"required"`
}

func project(p PR, repoName string) projection {
	out := projection{
		ID: p.ID, Title: p.Title, Status: p.Status, Draft: p.Draft, Src: "refs/heads/" + p.Src, Dst: "refs/heads/" + p.Dst,
		Head: p.Head, Created: p.Created.UTC().Format(time.RFC3339), Author: p.Author, Repo: repoName, Reviewers: []reviewer{},
		MergeStatus: p.MergeStatus,
	}
	if p.AutoComplete != "" {
		out.AutoComplete = &p.AutoComplete
	}
	if p.Status != "active" {
		closed := p.Created.Add(time.Hour).UTC().Format(time.RFC3339)
		out.Closed = &closed
	}
	if p.ForkRepo != "" {
		out.ForkRepo, out.ForkProject = &p.ForkRepo, &p.ForkProject
	}
	for _, r := range p.Reviewers {
		out.Reviewers = append(out.Reviewers, reviewer{Vote: r.Vote, Required: r.Required})
	}
	return out
}

func marshal(v any) ([]byte, error) { return json.Marshal(v) }

func (a *Az) prList(args []string) ([]byte, error) {
	key := Key(orgOf(args), flagValue(args, "--project"), flagValue(args, "--repository"))
	query := flagValue(args, "--query")
	if query == "[0].pullRequestId" {
		a.calls = append(a.calls, "pr-probe:"+key)
	} else {
		a.calls = append(a.calls, "pr-list:"+key+":"+flagValue(args, "--status"))
	}
	var rows []PR
	repoName := flagValue(args, "--repository")
	if repoName == "" { // project-wide search: every repo of the project
		prefix := orgOf(args) + "/" + flagValue(args, "--project") + "/"
		for k, st := range a.repos {
			if strings.HasPrefix(k, prefix) {
				for _, p := range st.prs {
					rows = append(rows, p)
				}
			}
		}
	} else if st := a.repos[key]; st != nil {
		rows = slices.Clone(st.prs)
	}
	status := flagValue(args, "--status")
	src := strings.TrimPrefix(flagValue(args, "--source-branch"), "refs/heads/")
	rows = slices.DeleteFunc(rows, func(p PR) bool {
		return (status != "" && status != "all" && p.Status != status) || (src != "" && p.Src != src)
	})
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })
	skip := intFlag(args, "--skip", 0)
	rows = rows[min(skip, len(rows)):]
	if top := intFlag(args, "--top", 100); len(rows) > top {
		rows = rows[:top]
	}
	if query == "[0].pullRequestId" {
		if len(rows) == 0 {
			return []byte("null"), nil
		}
		return []byte(strconv.Itoa(rows[0].ID)), nil
	}
	out := make([]projection, 0, len(rows))
	for _, p := range rows {
		out = append(out, project(p, a.repoNameOf(p.ID, repoName)))
	}
	return marshal(out)
}

func (a *Az) repoNameOf(id int, fallback string) string {
	if fallback != "" {
		return fallback
	}
	for k, st := range a.repos {
		for _, p := range st.prs {
			if p.ID == id {
				return k[strings.LastIndex(k, "/")+1:]
			}
		}
	}
	return ""
}

func (a *Az) find(id int) (PR, string, bool) {
	for k, st := range a.repos {
		for _, p := range st.prs {
			if p.ID == id {
				return p, k, true
			}
		}
	}
	return PR{}, "", false
}

func (a *Az) prShow(args []string) ([]byte, error) {
	id := intFlag(args, "--id", 0)
	a.calls = append(a.calls, "pr-show:"+strconv.Itoa(id))
	p, key, ok := a.find(id)
	if !ok {
		return nil, fail(prMissing)
	}
	return marshal(project(p, key[strings.LastIndex(key, "/")+1:]))
}

func (a *Az) policyList(args []string) ([]byte, error) {
	id := intFlag(args, "--id", 0)
	a.calls = append(a.calls, "policy:"+strconv.Itoa(id))
	p, _, ok := a.find(id)
	if !ok {
		return nil, fail(prMissing)
	}
	type eval struct {
		Status string `json:"status"`
		Type   string `json:"type"`
	}
	out := make([]eval, 0, len(p.Policies))
	for _, pol := range p.Policies {
		out = append(out, eval{Status: pol.Status, Type: pol.Type})
	}
	return marshal(out)
}

var votes = []string{"approve", "approve-with-suggestions", "reject", "reset", "wait-for-author"}

func (a *Az) setVote(args []string) ([]byte, error) {
	id, vote := intFlag(args, "--id", 0), flagValue(args, "--vote")
	a.calls = append(a.calls, "vote:"+strconv.Itoa(id)+":"+vote)
	if _, _, ok := a.find(id); !ok {
		return nil, fail(prMissing)
	}
	if !slices.Contains(votes, vote) {
		return nil, fail("ERROR: argument --vote: invalid choice: '" + vote + "'")
	}
	a.votes[id] = append(a.votes[id], vote)
	return []byte(`{}`), nil
}

// route reads `--route-parameters k=v k=v ...` up to the next flag.
func route(args []string) map[string]string {
	out := map[string]string{}
	for i, s := range args {
		if s != "--route-parameters" {
			continue
		}
		for _, kv := range args[i+1:] {
			if strings.HasPrefix(kv, "--") {
				break
			}
			if k, v, ok := strings.Cut(kv, "="); ok {
				out[k] = v
			}
		}
	}
	return out
}

func (a *Az) invoke(args []string) ([]byte, error) {
	rp := route(args)
	resource := flagValue(args, "--resource")
	key := Key(orgOf(args), rp["project"], rp["repositoryId"])
	if flagValue(args, "--area") != "git" || flagValue(args, "--api-version") == "" {
		return nil, a.unsupported(args)
	}
	switch flagValue(args, "--http-method") {
	case "POST":
	default:
		return nil, a.unsupported(args)
	}
	body, err := os.ReadFile(flagValue(args, "--in-file"))
	if err != nil {
		return nil, fail("ERROR: " + err.Error())
	}
	switch strings.ToLower(resource) {
	case "pullrequestquery":
		a.calls = append(a.calls, "invoke:pullRequestQuery:"+key)
		return a.pullRequestQuery(key, body)
	case "pullrequestthreads":
		id, _ := strconv.Atoi(rp["pullRequestId"])
		a.calls = append(a.calls, "invoke:pullRequestThreads:"+key)
		var thread struct {
			Comments []struct {
				Content string `json:"content"`
			} `json:"comments"`
			ThreadContext *struct {
				FilePath       string `json:"filePath"`
				RightFileStart struct {
					Line int `json:"line"`
				} `json:"rightFileStart"`
				RightFileEnd struct {
					Line   int `json:"line"`
					Offset int `json:"offset"`
				} `json:"rightFileEnd"`
			} `json:"threadContext"`
		}
		if json.Unmarshal(body, &thread) != nil || len(thread.Comments) == 0 {
			return nil, fail("ERROR: fakeaz: bad thread body")
		}
		if _, _, ok := a.find(id); !ok {
			return nil, fail(prMissing)
		}
		a.threads[id] = append(a.threads[id], thread.Comments[0].Content)
		tc := ThreadContext{}
		if x := thread.ThreadContext; x != nil {
			tc = ThreadContext{FilePath: x.FilePath, StartLine: x.RightFileStart.Line, End: x.RightFileEnd.Line, EndOffset: x.RightFileEnd.Offset}
		}
		a.contexts[id] = append(a.contexts[id], tc)
		return []byte(`{"id":1,"status":"active","continuation_token":null}`), nil
	}
	return nil, a.unsupported(args)
}

func (a *Az) pullRequestQuery(key string, body []byte) ([]byte, error) {
	var req struct {
		Queries []struct {
			Type  string   `json:"type"`
			Items []string `json:"items"`
		} `json:"queries"`
	}
	if json.Unmarshal(body, &req) != nil {
		return nil, fail("ERROR: fakeaz: bad query body")
	}
	results := []map[string][]map[string]any{}
	for _, q := range req.Queries {
		m := map[string][]map[string]any{}
		for _, sha := range q.Items {
			m[sha] = []map[string]any{}
			if st := a.repos[key]; st != nil {
				for _, p := range st.prs {
					if (q.Type == "commit" && p.Head == sha) || (q.Type == "lastMergeCommit" && p.MergeSha == sha) {
						m[sha] = append(m[sha], restPR(p))
					}
				}
			}
		}
		results = append(results, m)
	}
	return marshal(map[string]any{"continuation_token": nil, "results": results})
}

// restPR is a pull request as the REST API prints it.
func restPR(p PR) map[string]any {
	rv := make([]map[string]any, 0, len(p.Reviewers))
	for _, r := range p.Reviewers {
		rv = append(rv, map[string]any{"vote": r.Vote, "isRequired": r.Required})
	}
	out := map[string]any{
		"pullRequestId": p.ID, "title": p.Title, "status": p.Status, "isDraft": p.Draft,
		"sourceRefName": "refs/heads/" + p.Src, "targetRefName": "refs/heads/" + p.Dst,
		"creationDate":          p.Created.UTC().Format(time.RFC3339),
		"lastMergeSourceCommit": map[string]any{"commitId": p.Head},
		"createdBy":             map[string]any{"uniqueName": p.Author},
		"reviewers":             rv,
		"mergeStatus":           p.MergeStatus,
		"url":                   fmt.Sprintf("https://dev.azure.com/fake/_apis/git/pullRequests/%d", p.ID),
	}
	if p.AutoComplete != "" {
		out["autoCompleteSetBy"] = map[string]any{"uniqueName": p.AutoComplete}
	}
	if p.ForkRepo != "" {
		out["forkSource"] = map[string]any{"repository": map[string]any{"name": p.ForkRepo, "project": map[string]any{"name": p.ForkProject}}}
	}
	return out
}
