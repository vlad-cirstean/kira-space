package claude

// P643 Part 5 step 0 (plan 14.2): stub `state` tool for the schema accuracy test. Two shapes of the
// same tool: arm N (nested {action, args}) and arm FLAT (fields next to action). The raw
// (*mcp.Server).AddTool path keeps the SDK from rejecting or coercing input, so every call reaches
// decode and a bad call becomes an `invalid` item. No Kira backend: a small world per run.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	accArmN    = "N"
	accArmFlat = "FLAT"
)

// accField is one field of an action: name, requiredness and JSON kind (s string, b bool, l string list).
type accField struct {
	name string
	req  bool
	kind byte
}

type accAction struct {
	name    string
	fields  []accField
	note    string
	example string // fields of a valid example call, JSON members without braces
}

var accActions = []accAction{
	{"repo.declare", []accField{{"repos", true, 'l'}}, "repos the work touches.", `"repos": ["api", "app"]`},
	{"branch.request", []accField{{"repo", true, 's'}, {"branch", true, 's'}, {"base", false, 's'}, {"existing", false, 'b'}},
		"new branch (user approves) or existing one; waits until its worktree is ready.", `"repo": "app", "branch": "feature/x"`},
	{"branch.rename", []accField{{"repo", true, 's'}, {"from", true, 's'}, {"to", true, 's'}},
		"rename your branch (user approves).", `"repo": "app", "from": "feature/x", "to": "feature/y"`},
	{"branch.change", []accField{{"repo", true, 's'}, {"from", true, 's'}, {"to", true, 's'}},
		"point task branch from at existing local branch to (user approves).", `"repo": "app", "from": "feature/x", "to": "feature/x-alt"`},
	{"lease.acquire", []accField{{"queue", true, 's'}}, "wait for the queue's lease.", `"queue": "deploy-staging"`},
	{"lease.release", []accField{{"queue", false, 's'}}, "leave one queue, or every queue.", `"queue": "deploy-staging"`},
	{"operation.cancel", []accField{{"opId", true, 's'}}, "stop a running operation (state.yaml operations).", `"opId": "op-7"`},
	{"remote.fetch", []accField{{"remote", false, 's'}}, "fetch from a remote.", `"remote": "origin"`},
	{"branch.push", []accField{{"force", false, 'b'}}, "push the task branch.", `"force": false`},
	{"review.reply", []accField{{"id", true, 's'}, {"reply", true, 's'}, {"resolved", false, 'b'}},
		"answer a review comment, optionally resolve it.", `"id": "c-17", "reply": "Done.", "resolved": true`},
	{"step.finish", []accField{{"status", true, 's'}, {"reason", false, 's'}, {"conflictedFiles", false, 'l'}, {"lastGitError", false, 's'}, {"tried", false, 's'}},
		"end the step; status is done, failed or needs_input, reason is required unless done.", `"status": "done"`},
}

func accActionByName(n string) *accAction {
	for i := range accActions {
		if accActions[i].name == n {
			return &accActions[i]
		}
	}
	return nil
}

func accActionNames() []string {
	n := make([]string, len(accActions))
	for i, a := range accActions {
		n[i] = a.name
	}
	return n
}

// accExample is a valid call of action in the arm's own shape.
func accExample(arm, action string) string {
	a := accActionByName(action)
	if a == nil {
		a = accActionByName("branch.request")
	}
	if arm == accArmN {
		return `{"action": "` + a.name + `", "args": {` + a.example + `}}`
	}
	return `{"action": "` + a.name + `", ` + a.example + `}`
}

func accShapeSentence(arm string) string {
	if arm == accArmN {
		return "put the action's fields in args"
	}
	return "put the action's fields next to action"
}

func accDescription(arm string) string {
	var b strings.Builder
	b.WriteString("Every task state change goes through this tool. Pick action; " + accShapeSentence(arm) + ". A call that waits returns within 10 minutes. " +
		"Reply: a summary line, then one item per change with status done, pending, waiting, denied, failed or invalid; invalid has fix.\n")
	for _, a := range accActions {
		names := make([]string, len(a.fields))
		for i, f := range a.fields {
			names[i] = f.name
			if !f.req {
				names[i] += "?"
			}
		}
		list := strings.Join(names, ", ")
		if arm == accArmN {
			list = "args {" + list + "}"
		}
		b.WriteString("- " + a.name + ": " + list + ": " + a.note + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func accTool(arm string) *mcp.Tool {
	enum := make([]any, len(accActions))
	for i, a := range accActions {
		enum[i] = a.name
	}
	action := &jsonschema.Schema{Type: "string", Enum: enum}
	props := map[string]*jsonschema.Schema{"action": action}
	if arm == accArmN {
		props["args"] = &jsonschema.Schema{Type: "object"}
	} else {
		str, boo := func() *jsonschema.Schema { return &jsonschema.Schema{Type: "string"} }, func() *jsonschema.Schema { return &jsonschema.Schema{Type: "boolean"} }
		list := func() *jsonschema.Schema { return &jsonschema.Schema{Type: "array", Items: str()} }
		for _, f := range []string{"repos", "conflictedFiles"} {
			props[f] = list()
		}
		for _, f := range []string{"existing", "force", "resolved"} {
			props[f] = boo()
		}
		for _, f := range []string{"repo", "branch", "base", "from", "to", "queue", "opId", "remote", "id", "reply", "status", "reason", "lastGitError", "tried"} {
			props[f] = str()
		}
	}
	return &mcp.Tool{Name: "state", Description: accDescription(arm), InputSchema: &jsonschema.Schema{Type: "object", Properties: props, Required: []string{"action"}}}
}

// accSchemaBytes is the JSON of the arm's tools/list as a client sees it.
func accSchemaBytes(t *testing.T, arm string) int {
	t.Helper()
	st := newAccStub(arm, t.TempDir(), time.Now())
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return st.server() }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer ts.Close()
	tools, err := listTools(t, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	return len(raw)
}

// ---- decode and validation ----

// accInvalid is a rejected call: class is action, shape, args-string, missing, stray or value.
type accInvalid struct{ class, msg, fix string }

type accDecoded struct {
	action string
	fields map[string]json.RawMessage
}

func (d accDecoded) str(k string) string {
	var s string
	_ = json.Unmarshal(d.fields[k], &s)
	return s
}

func (d accDecoded) boolean(k string) bool {
	var b bool
	_ = json.Unmarshal(d.fields[k], &b)
	return b
}

func (d accDecoded) list(k string) []string {
	var l []string
	_ = json.Unmarshal(d.fields[k], &l)
	return l
}

func accFix(arm, action, why string) string {
	return why + " Call state again with " + accExample(arm, action) + "."
}

// accDecode runs the shape step and the field step. The value step needs the world (accStub.check).
func accDecode(arm string, raw json.RawMessage) (accDecoded, *accInvalid) {
	var d accDecoded
	top := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &top); err != nil {
			return d, &accInvalid{"shape", "arguments are not a JSON object.", accFix(arm, "branch.request", "Send an object with action.")}
		}
	}
	_ = json.Unmarshal(top["action"], &d.action)
	example := d.action
	if accActionByName(example) == nil {
		example = "branch.request"
	}
	fields := map[string]json.RawMessage{}
	if arm == accArmN {
		var stray []string
		for k := range top {
			if k != "action" && k != "args" {
				stray = append(stray, k)
			}
		}
		slices.Sort(stray)
		if len(stray) > 0 {
			return d, &accInvalid{"shape", "fields " + strings.Join(stray, ", ") + " are at the top level; they belong under args.", accFix(arm, example, "Move them under args.")}
		}
		if a, ok := top["args"]; ok && string(a) != "null" {
			if err := json.Unmarshal(a, &fields); err != nil {
				var s string
				if json.Unmarshal(a, &s) == nil && json.Unmarshal([]byte(s), &map[string]json.RawMessage{}) == nil {
					return d, &accInvalid{"args-string", "args is a JSON string; it must be an object.", accFix(arm, example, "Pass args as an object, not as a string.")}
				}
				return d, &accInvalid{"shape", "args must be an object.", accFix(arm, example, "Pass args as an object.")}
			}
		}
	} else {
		if _, ok := top["args"]; ok {
			return d, &accInvalid{"shape", "args does not exist; the action's fields go next to action.", accFix(arm, example, "Put the fields next to action.")}
		}
		for k, v := range top {
			if k != "action" {
				fields[k] = v
			}
		}
	}
	a := accActionByName(d.action)
	if a == nil {
		msg := "action is missing."
		if d.action != "" {
			msg = "unknown action " + d.action + "."
		}
		return d, &accInvalid{"action", msg + " Actions: " + strings.Join(accActionNames(), ", ") + ".", accFix(arm, example, "Pick one action.")}
	}
	d.fields = fields
	known := map[string]accField{}
	for _, f := range a.fields {
		known[f.name] = f
	}
	var stray []string
	for k := range fields {
		if _, ok := known[k]; !ok {
			stray = append(stray, k)
		}
	}
	slices.Sort(stray)
	if len(stray) > 0 {
		return d, &accInvalid{"stray", a.name + " has no field " + strings.Join(stray, ", ") + ".", accFix(arm, a.name, "Use only the fields of "+a.name+".")}
	}
	for _, f := range a.fields {
		if _, ok := fields[f.name]; !ok {
			if f.req {
				return d, &accInvalid{"missing", a.name + " needs " + f.name + ".", accFix(arm, a.name, a.name+" needs "+f.name+".")}
			}
			continue
		}
		if !accKindOK(f.kind, fields[f.name]) {
			return d, &accInvalid{"value", f.name + " has the wrong type.", accFix(arm, a.name, f.name+" has the wrong type.")}
		}
	}
	return d, nil
}

func accKindOK(kind byte, raw json.RawMessage) bool {
	switch kind {
	case 's':
		var s string
		return json.Unmarshal(raw, &s) == nil
	case 'b':
		var b bool
		return json.Unmarshal(raw, &b) == nil
	default:
		var l []string
		return json.Unmarshal(raw, &l) == nil
	}
}

// ---- reply ----

type accItem struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

type accReplyBody struct {
	Summary string    `json:"summary"`
	Items   []accItem `json:"items"`
	Next    string    `json:"next"`
	Notices []string  `json:"notices"`
}

func accReply(it accItem, next string) *mcp.CallToolResult {
	body := accReplyBody{Summary: "1 " + it.Status, Items: []accItem{it}, Next: next, Notices: []string{}}
	line := it.ID + " " + it.Status + ": " + it.Message
	if it.Fix != "" {
		line += " Fix: " + it.Fix
	}
	text := body.Summary + "\n" + line + "\nnext: " + next
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, StructuredContent: body}
}

// ---- world ----

// accCall is one logged state call.
type accCall struct {
	Seq      int             `json:"seq"`
	Raw      json.RawMessage `json:"raw"`
	Action   string          `json:"action"`
	Valid    bool            `json:"valid"`
	Class    string          `json:"class,omitempty"`
	Subject  string          `json:"subject,omitempty"`
	Statuses []string        `json:"statuses"`
	At       time.Time       `json:"at"`
}

type accChange struct{ repo, from, to string }

type accEvent struct {
	what string
	seq  int
}

type accFinish struct {
	status, reason string
	seq            int
}

// accStub is one run's world. Waits are fields so a test can zero them.
type accStub struct {
	arm, root string
	ready     time.Time
	approve   time.Duration // approval of a new branch, rename or change
	setup     time.Duration // setup of a new branch worktree
	lease     time.Duration // grant of the contended lease

	mu       sync.Mutex
	seq      int
	log      []accCall
	local    map[string][]string          // repo -> local branches
	task     map[string]map[string]string // repo -> task branch -> base
	building map[string]bool              // repo/branch with a setup still running (until ready)
	comments map[string]bool
	replies  map[string]string
	resolved map[string]bool
	replySeq map[string]int
	changes  []accChange
	changeAt []int
	leaseEv  []accEvent
	held     bool
	finishes []accFinish
}

func newAccStub(arm, root string, ready time.Time) *accStub {
	return &accStub{
		arm: arm, root: root, ready: ready,
		approve: 2 * time.Second, setup: 3 * time.Second, lease: 5 * time.Second,
		local:    map[string][]string{"api": {"main"}, "app": {"main", "feature/x", "feature/x-alt"}, "web": {"main", "release/2.4"}},
		task:     map[string]map[string]string{"api": {}, "app": {}},
		building: map[string]bool{}, comments: map[string]bool{}, replies: map[string]string{}, resolved: map[string]bool{}, replySeq: map[string]int{},
	}
}

func (s *accStub) server() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "kira-space", Version: "acc"}, nil)
	srv.AddTool(accTool(s.arm), s.handle)
	return srv
}

func (s *accStub) worktree(repo, branch string) string {
	return filepath.Join(s.root, "wt", repo+"-"+strings.ReplaceAll(branch, "/", "-"))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	tm := time.NewTimer(d)
	defer tm.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-tm.C:
		return nil
	}
}

func (s *accStub) handle(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw := req.Params.Arguments
	d, inv := accDecode(s.arm, raw)
	var bad *accInvalid
	var run func(context.Context) (accItem, error)
	s.mu.Lock()
	if inv != nil {
		bad = inv
	} else {
		bad, run = s.checkLocked(d)
	}
	subject := ""
	if bad == nil {
		subject = s.subject(d)
	}
	call := accCall{Seq: len(s.log) + 1, Raw: slices.Clone(raw), Action: d.action, Valid: bad == nil, Subject: subject, At: time.Now()}
	if bad != nil {
		call.Class = bad.class
		call.Statuses = []string{"invalid"}
	}
	idx := len(s.log)
	s.log = append(s.log, call)
	s.mu.Unlock()
	if bad != nil {
		return accReply(accItem{ID: "call", Status: "invalid", Message: bad.msg, Fix: bad.fix}, "Fix the call and call state again."), nil
	}
	it, err := run(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.log[idx].Statuses = []string{it.Status}
	s.mu.Unlock()
	return accReply(it, "Nothing pending."), nil
}

func (s *accStub) subject(d accDecoded) string {
	switch d.action {
	case "repo.declare":
		return "repos"
	case "branch.request":
		return d.str("repo") + "/" + d.str("branch")
	case "branch.rename", "branch.change":
		return d.str("repo") + "/" + d.str("to")
	case "lease.acquire", "lease.release":
		return d.str("queue")
	case "operation.cancel":
		return d.str("opId")
	case "review.reply":
		return d.str("id")
	case "step.finish":
		return "step"
	case "remote.fetch":
		return "remote"
	}
	return "push"
}

func (s *accStub) wrongValue(a *accAction, msg string) *accInvalid {
	return &accInvalid{"value", msg, accFix(s.arm, a.name, msg)}
}

// checkLocked is the value step against the world; the returned func applies the call (it may wait).
func (s *accStub) checkLocked(d accDecoded) (*accInvalid, func(context.Context) (accItem, error)) {
	a := accActionByName(d.action)
	repoOK := func(r string) bool { _, ok := s.local[r]; return ok }
	switch d.action {
	case "repo.declare":
		for _, r := range d.list("repos") {
			if !repoOK(r) {
				return s.wrongValue(a, "unknown repo "+r+"."), nil
			}
		}
		return nil, s.immediate("repos", "declared")
	case "branch.request":
		repo, branch := d.str("repo"), d.str("branch")
		if !repoOK(repo) {
			return s.wrongValue(a, "unknown repo "+repo+"."), nil
		}
		if base := d.str("base"); base != "" && !slices.Contains(s.local[repo], base) {
			return s.wrongValue(a, "base "+base+" is not a local branch of "+repo+"."), nil
		}
		_, onTask := s.task[repo][branch]
		isLocal := slices.Contains(s.local[repo], branch)
		if d.boolean("existing") && !onTask && !isLocal {
			return s.wrongValue(a, "branch "+branch+" does not exist in "+repo+"."), nil
		}
		id := repo + "/" + branch
		base := d.str("base")
		if base == "" {
			base = "main"
		}
		switch {
		case onTask && s.building[id]:
			return nil, func(ctx context.Context) (accItem, error) {
				if err := sleepCtx(ctx, time.Until(s.ready)); err != nil {
					return accItem{}, err
				}
				s.mu.Lock()
				delete(s.building, id)
				s.mu.Unlock()
				return accItem{ID: id, Status: "done", Message: "worktree " + s.worktree(repo, branch) + " ready"}, nil
			}
		case onTask || isLocal:
			return nil, func(context.Context) (accItem, error) {
				s.mu.Lock()
				if !onTask {
					s.task[repo] = ensure(s.task[repo])
					s.task[repo][branch] = ""
				}
				s.mu.Unlock()
				return accItem{ID: id, Status: "done", Message: "worktree " + s.worktree(repo, branch) + " ready"}, nil
			}
		}
		return nil, func(ctx context.Context) (accItem, error) {
			if err := sleepCtx(ctx, s.approve+s.setup); err != nil {
				return accItem{}, err
			}
			s.mu.Lock()
			s.task[repo] = ensure(s.task[repo])
			s.task[repo][branch] = base
			s.local[repo] = append(s.local[repo], branch)
			s.mu.Unlock()
			return accItem{ID: id, Status: "done", Message: "worktree " + s.worktree(repo, branch) + " ready"}, nil
		}
	case "branch.rename", "branch.change":
		repo, from, to := d.str("repo"), d.str("from"), d.str("to")
		if !repoOK(repo) {
			return s.wrongValue(a, "unknown repo "+repo+"."), nil
		}
		base, onTask := s.task[repo][from]
		if !onTask {
			return s.wrongValue(a, "branch "+from+" is not on the task in "+repo+"."), nil
		}
		if d.action == "branch.change" && !slices.Contains(s.local[repo], to) {
			return s.wrongValue(a, "to "+to+" is not a local branch of "+repo+"."), nil
		}
		id := repo + "/" + to
		return nil, func(ctx context.Context) (accItem, error) {
			if err := sleepCtx(ctx, s.approve); err != nil {
				return accItem{}, err
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.task[repo], from)
			if d.action == "branch.rename" {
				s.task[repo][to] = base
				s.local[repo] = append(s.local[repo], to)
				return accItem{ID: id, Status: "done", Message: "renamed " + from + " to " + to}, nil
			}
			s.task[repo][to] = ""
			s.seq++
			s.changes = append(s.changes, accChange{repo, from, to})
			s.changeAt = append(s.changeAt, s.seq)
			return accItem{ID: id, Status: "done", Message: "task branch " + from + " now points at " + to}, nil
		}
	case "lease.acquire":
		if q := d.str("queue"); q != "deploy-staging" {
			return s.wrongValue(a, "unknown queue "+q+"."), nil
		}
		return nil, func(ctx context.Context) (accItem, error) {
			if err := sleepCtx(ctx, s.lease); err != nil {
				return accItem{}, err
			}
			s.mu.Lock()
			s.held = true
			s.seq++
			s.leaseEv = append(s.leaseEv, accEvent{"acquired", s.seq})
			s.mu.Unlock()
			return accItem{ID: "deploy-staging", Status: "done", Message: "lease held"}, nil
		}
	case "lease.release":
		if q := d.str("queue"); q != "" && q != "deploy-staging" {
			return s.wrongValue(a, "unknown queue "+q+"."), nil
		}
		return nil, func(context.Context) (accItem, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.seq++
			s.leaseEv = append(s.leaseEv, accEvent{"released", s.seq})
			s.held = false
			return accItem{ID: "deploy-staging", Status: "done", Message: "lease released"}, nil
		}
	case "operation.cancel":
		if id := d.str("opId"); id != "op-7" {
			return s.wrongValue(a, "unknown operation "+id+"."), nil
		}
		return nil, s.immediate("op-7", "cancelled")
	case "remote.fetch":
		return nil, s.immediate("remote", "fetched")
	case "branch.push":
		return nil, s.immediate("push", "pushed")
	case "review.reply":
		id := d.str("id")
		if !s.comments[id] {
			return s.wrongValue(a, "unknown comment "+id+"."), nil
		}
		return nil, func(context.Context) (accItem, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.seq++
			s.replies[id], s.resolved[id], s.replySeq[id] = d.str("reply"), d.boolean("resolved"), s.seq
			return accItem{ID: id, Status: "done", Message: "reply recorded"}, nil
		}
	default: // step.finish
		status := d.str("status")
		if !slices.Contains([]string{"done", "failed", "needs_input"}, status) {
			return s.wrongValue(a, "status "+status+" is not done, failed or needs_input."), nil
		}
		if status != "done" && strings.TrimSpace(d.str("reason")) == "" {
			return s.wrongValue(a, "reason is required when status is not done."), nil
		}
		return nil, func(context.Context) (accItem, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.seq++
			s.finishes = append(s.finishes, accFinish{status, d.str("reason"), s.seq})
			return accItem{ID: "step", Status: "done", Message: "step finished: " + status}, nil
		}
	}
}

func ensure(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func (s *accStub) immediate(id, msg string) func(context.Context) (accItem, error) {
	return func(context.Context) (accItem, error) { return accItem{ID: id, Status: "done", Message: msg}, nil }
}

// accStateYAML is the task folder's state.yaml; A1 flips the setup of app/feature/x at ready.
func accStateYAML(buildingApp bool) string {
	setup := "done"
	if buildingApp {
		setup = "waiting"
	}
	return "# Read-only. Kira rewrites this file. State changes: call state. status is done, pending (the user must answer), waiting (Kira or a queue is on it), denied or failed (see note).\n" +
		"rev: 1\ntask: {id: t-1, kind: task, status: running}\nrepos:\n  - {repo: api, setup: {status: done}}\n  - {repo: app, branch: feature/x, setup: {status: " + setup + "}}\nregistered: [web]\n"
}

const accReadme = "Read-only. Kira rewrites state.yaml. State changes: call state.\nKira tool: state. Load it first with ToolSearch select:mcp__kira-space__state\n"

// accSetup builds the arm's benchSetup hook: the stub, its server, mcp.json, task folder files.
func accSetup(arm string) func(*testing.T, *spikeRun, spikeFlow) (string, []string, func()) {
	return func(t *testing.T, r *spikeRun, fl spikeFlow) (string, []string, func()) {
		if fl.id == "A1" {
			r.ready = time.Now().Add(20 * time.Second)
		}
		st := newAccStub(arm, r.root, r.ready)
		switch fl.id {
		case "A1":
			st.task["app"]["feature/x"] = "main"
			st.building["app/feature/x"] = true
		case "A3":
			st.task["app"]["feature/x"] = "main"
			st.comments["c-17"] = true
		}
		accCur = st
		writeFile(t, filepath.Join(r.taskDir, "README.md"), accReadme)
		writeFile(t, filepath.Join(r.taskDir, "state.yaml"), accStateYAML(fl.id == "A1"))
		var timer *time.Timer
		if fl.id == "A1" {
			timer = time.AfterFunc(time.Until(r.ready), func() {
				_ = os.WriteFile(filepath.Join(r.taskDir, "state.yaml"), []byte(accStateYAML(false)), 0o644)
			})
		}
		ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return st.server() }, &mcp.StreamableHTTPOptions{Stateless: true}))
		cfg := filepath.Join(r.root, "mcp.json")
		raw, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"kira-space": map[string]any{"type": "http", "url": ts.URL}}})
		if err := os.WriteFile(cfg, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return cfg, nil, func() {
			if timer != nil {
				timer.Stop()
			}
			ts.Close()
		}
	}
}

// accCur is the stub of the run in flight; the oracles and the scorer read it.
var accCur *accStub

func (s *accStub) callLog() []accCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.log)
}
