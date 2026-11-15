package agentmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// state.go: the state tool, one tool for every task state change. The agent sends {action, args};
// Kira checks the call itself, so a bad call is an invalid item with a fix, never a protocol error.

// Bounds on state call fields.
const (
	maxStateText = 8 << 10
	// maxStateRepos bounds repo.declare's list.
	maxStateRepos = maxDeclareRepos
)

// Field kinds of an action.
const (
	kindString = 's'
	kindBool   = 'b'
	kindList   = 'l'
)

type stateField struct {
	name string
	req  bool
	kind byte
}

type stateAction struct {
	name   string
	fields []stateField
	// example is the members of a valid args object, for a fix.
	example string
	line    llmprompts.Prompt[llmprompts.NoVars]
	enabled func(toolSet) bool
}

func spaceOnly(ts toolSet) bool { return ts.space }

var stateActions = []stateAction{
	{"repo.declare", []stateField{{"repos", true, kindList}}, `"repos": ["api", "app"]`, llmprompts.HeadlessStateRepoDeclare, spaceOnly},
	{"branch.request", []stateField{{"repo", true, kindString}, {"branch", true, kindString}, {"base", false, kindString}, {"existing", false, kindBool}},
		`"repo": "app", "branch": "feature/x"`, llmprompts.HeadlessStateBranchRequest, spaceOnly},
	{"branch.rename", []stateField{{"repo", true, kindString}, {"from", true, kindString}, {"to", true, kindString}, {"renameRemote", false, kindBool}},
		`"repo": "app", "from": "feature/x", "to": "feature/y"`, llmprompts.HeadlessStateBranchRename, spaceOnly},
	{"branch.change", []stateField{{"repo", true, kindString}, {"from", true, kindString}, {"to", true, kindString}},
		`"repo": "app", "from": "feature/x", "to": "feature/x-alt"`, llmprompts.HeadlessStateBranchChange, spaceOnly},
	{"lease.acquire", []stateField{{"queue", true, kindString}}, `"queue": "deploy-staging"`, llmprompts.HeadlessStateLeaseAcquire, spaceOnly},
	{"lease.release", []stateField{{"queue", false, kindString}}, `"queue": "deploy-staging"`, llmprompts.HeadlessStateLeaseRelease, spaceOnly},
	{"operation.cancel", []stateField{{"opId", true, kindString}}, `"opId": "op-7"`, llmprompts.HeadlessStateOperationCancel, spaceOnly},
	{"remote.fetch", []stateField{{"remote", false, kindString}}, `"remote": "origin"`, llmprompts.HeadlessStateRemoteFetch, func(ts toolSet) bool { return ts.sandbox }},
	{"branch.push", []stateField{{"force", false, kindBool}}, `"force": false`, llmprompts.HeadlessStateBranchPush, func(ts toolSet) bool { return ts.sandbox && ts.push }},
	{"review.reply", []stateField{{"id", true, kindString}, {"reply", true, kindString}, {"resolved", false, kindBool}},
		`"id": "c-17", "reply": "Done.", "resolved": true`, llmprompts.HeadlessStateReviewReply, func(ts toolSet) bool { return ts.review }},
	{"step.finish", []stateField{{"status", true, kindString}, {"reason", false, kindString}, {"conflictedFiles", false, kindList}, {"lastGitError", false, kindString}, {"tried", false, kindString}},
		`"status": "done"`, llmprompts.HeadlessStateStepFinish, spaceOnly},
}

func stateActionsOf(ts toolSet) []stateAction {
	var out []stateAction
	for _, a := range stateActions {
		if a.enabled(ts) {
			out = append(out, a)
		}
	}
	return out
}

func stateActionByName(actions []stateAction, name string) *stateAction {
	for i := range actions {
		if actions[i].name == name {
			return &actions[i]
		}
	}
	return nil
}

func (a *stateAction) fieldNamed(name string) (stateField, bool) {
	for _, f := range a.fields {
		if f.name == name {
			return f, true
		}
	}
	return stateField{}, false
}

// exampleCall is a valid call of the action, for a fix.
func exampleCall(action string) string {
	for _, a := range stateActions {
		if a.name == action {
			return `{"action": "` + a.name + `", "args": {` + a.example + `}}`
		}
	}
	return exampleCall("branch.request")
}

func actionNames(actions []stateAction) []string {
	n := make([]string, len(actions))
	for i, a := range actions {
		n[i] = a.name
	}
	return n
}

// stateTool is the state tool of a tool set: the action enum and the description of its actions.
func (s *Server) stateTool(ts toolSet) *mcp.Tool {
	actions := stateActionsOf(ts)
	enum := make([]any, len(actions))
	var desc strings.Builder
	for i, a := range actions {
		enum[i] = a.name
		desc.WriteString("\n- " + a.line.Text(s.prompts, llmprompts.NoVars{}))
	}
	sch := llmprompts.InputSchema[stateArgs](s.prompts, llmprompts.HeadlessToolState.ID(), nil)
	sch.Properties["action"].Enum = enum
	return &mcp.Tool{Name: "state", Description: llmprompts.HeadlessToolState.Text(s.prompts, llmprompts.NoVars{}) + desc.String(), InputSchema: sch}
}

// stateArgs is the shape of a state call, for the schema; decodeState reads the raw call.
type stateArgs struct {
	Action string         `json:"action"`
	Args   map[string]any `json:"args,omitempty"`
}

// stateInvalid is a rejected call: class is action, shape, args-string, missing, stray or value.
type stateInvalid struct{ class, msg, fix string }

func fixOf(action, why string) string {
	return strings.TrimSpace(why+" Call state again with "+exampleCall(action)) + "."
}

type stateCallArgs struct {
	action string
	fields map[string]json.RawMessage
}

func (d stateCallArgs) str(k string) string {
	var v string
	_ = json.Unmarshal(d.fields[k], &v)
	return strings.TrimSpace(v)
}

func (d stateCallArgs) raw(k string) string {
	var v string
	_ = json.Unmarshal(d.fields[k], &v)
	return v
}

func (d stateCallArgs) boolean(k string) bool {
	var v bool
	_ = json.Unmarshal(d.fields[k], &v)
	return v
}

func (d stateCallArgs) list(k string) []string {
	var v []string
	_ = json.Unmarshal(d.fields[k], &v)
	return v
}

// decodeState runs the shape step then the field step: the top level is {action, args}, args holds
// exactly the action's fields with the right types. Values are the action's own check.
func decodeState(raw json.RawMessage, actions []stateAction) (stateCallArgs, *stateInvalid) {
	var d stateCallArgs
	top := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &top); err != nil {
			return d, &stateInvalid{"shape", "arguments are not a JSON object.", fixOf("branch.request", "Send an object with action.")}
		}
	}
	_ = json.Unmarshal(top["action"], &d.action)
	example := d.action
	if stateActionByName(actions, example) == nil {
		example = "branch.request"
		if len(actions) > 0 {
			example = actions[0].name
		}
	}
	var stray []string
	for k := range top {
		if k != "action" && k != "args" {
			stray = append(stray, k)
		}
	}
	slices.Sort(stray)
	if len(stray) > 0 {
		return d, &stateInvalid{"shape", "fields " + strings.Join(stray, ", ") + " are at the top level; they belong under args.", fixOf(example, "Move them under args.")}
	}
	fields := map[string]json.RawMessage{}
	if a, ok := top["args"]; ok && string(a) != "null" {
		if err := json.Unmarshal(a, &fields); err != nil {
			var str string
			if json.Unmarshal(a, &str) == nil && json.Unmarshal([]byte(str), &map[string]json.RawMessage{}) == nil {
				return d, &stateInvalid{"args-string", "args is a JSON string; it must be an object.", fixOf(example, "Pass args as an object, not as a string.")}
			}
			return d, &stateInvalid{"shape", "args must be an object.", fixOf(example, "Pass args as an object.")}
		}
	}
	act := stateActionByName(actions, d.action)
	if act == nil {
		msg := "action is missing."
		if d.action != "" {
			msg = "unknown action " + d.action + "."
		}
		return d, &stateInvalid{"action", msg + " Actions: " + strings.Join(actionNames(actions), ", ") + ".", fixOf(example, "Pick one action.")}
	}
	d.fields = fields
	stray = stray[:0]
	for k := range fields {
		if _, ok := act.fieldNamed(k); !ok {
			stray = append(stray, k)
		}
	}
	slices.Sort(stray)
	if len(stray) > 0 {
		return d, &stateInvalid{"stray", act.name + " has no field " + strings.Join(stray, ", ") + ".", fixOf(act.name, "Use only the fields of "+act.name+".")}
	}
	for _, f := range act.fields {
		v, ok := fields[f.name]
		if !ok {
			if f.req {
				return d, &stateInvalid{"missing", act.name + " needs " + f.name + ".", fixOf(act.name, act.name+" needs "+f.name+".")}
			}
			continue
		}
		if !kindOK(f.kind, v) {
			return d, &stateInvalid{"value", f.name + " has the wrong type.", fixOf(act.name, f.name+" has the wrong type.")}
		}
	}
	return d, nil
}

func kindOK(kind byte, raw json.RawMessage) bool {
	switch kind {
	case kindString:
		var v string
		return json.Unmarshal(raw, &v) == nil
	case kindBool:
		var v bool
		return json.Unmarshal(raw, &v) == nil
	default:
		var v []string
		return json.Unmarshal(raw, &v) == nil
	}
}

// stateOutcome is what one action returns: the reply and extra content blocks (the artifacts of a finish).
type stateOutcome struct {
	reply Reply
	extra []mcp.Content
}

func invalidItem(action, msg string) Item {
	return Item{ID: "call", Status: StatusInvalid, Message: msg, Fix: fixOf(action, "")}
}

// stateFail turns an action error into an item: a ToolError is the agent's to fix, anything else generic.
func stateFail(action, id string, err error) Item {
	var te ToolError
	if errors.As(err, &te) {
		return Item{ID: id, Status: StatusInvalid, Message: te.Error(), Fix: exampleCall(action)}
	}
	slog.Warn("agentmcp: state action", "scope", "ade", "action", action, "err", err)
	return Item{ID: id, Status: StatusFailed, Message: "Kira could not complete that. Try again, or ask the user."}
}

func (s *Server) stateCall(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var g Grant
	var ok bool
	if req.Extra != nil {
		g, ok = s.grant(req.Extra.TokenInfo)
	}
	if !ok || g.TaskID == "" {
		return toolError("Kira tools are not available for this session."), nil
	}
	ts := s.toolSetOf(g)
	actions := stateActionsOf(ts)
	d, bad := decodeState(req.Params.Arguments, actions)
	if bad != nil {
		return oneItem(Item{ID: "call", Status: StatusInvalid, Message: bad.msg, Fix: bad.fix}, "Fix the call and call state again.", nil).result(), nil
	}
	ctx, done := s.callCtx(ctx, req)
	defer done()
	ctx = withProgress(ctx, req)
	out := s.runState(ctx, req, g, d)
	if ctx.Err() != nil {
		return toolError("The call ended: its run or session is gone."), nil // a Kira fault, not an item
	}
	return out.reply.result(out.extra...), nil
}

func (s *Server) runState(ctx context.Context, req *mcp.CallToolRequest, g Grant, d stateCallArgs) stateOutcome {
	call := SpaceCall{TaskID: g.TaskID, RunID: g.RunID, StepID: g.StepID, SessionID: g.SessionID, Orchestrator: g.Orchestrator}
	one := func(it Item, next string, notices ...string) stateOutcome {
		return stateOutcome{reply: oneItem(it, next, notices)}
	}
	switch d.action {
	case "repo.declare":
		repos := d.list("repos")
		if n := len(repos); n == 0 || n > maxStateRepos {
			return one(invalidItem(d.action, "repos needs 1 to 20 entries."), "")
		}
		info, err := s.space.DeclareRepos(ctx, call.TaskID, repos)
		if err != nil {
			return one(stateFail(d.action, "repos", err), "")
		}
		return one(Item{ID: "repos", Status: StatusDone, Message: declaredText(info)}, "Create each branch with branch.request.")
	case "branch.request":
		specs, msg := cleanBranches([]BranchSpec{{Repo: d.str("repo"), Branch: d.str("branch"), Base: d.str("base"), Existing: d.boolean("existing")}})
		if msg != "" {
			return one(invalidItem(d.action, "Every branch.request needs a repo and a branch."), "")
		}
		res, err := s.space.RequestBranches(ctx, call, specs)
		if err != nil {
			return one(stateFail(d.action, specs[0].Repo+"/"+specs[0].Branch, err), "")
		}
		items := make([]Item, 0, len(res.Branches))
		for _, rb := range res.Branches {
			items = append(items, branchItem(call, rb, res.Waited))
		}
		return stateOutcome{reply: newReply(items, nextOf(items), nil)}
	case "branch.rename":
		args := RenameBranchArgs{Repo: d.str("repo"), From: d.str("from"), To: d.str("to"), RenameRemote: d.boolean("renameRemote")}
		if args.From == "" || args.To == "" || args.From == args.To {
			return one(invalidItem(d.action, "from and to must be two different branch names."), "")
		}
		res, err := s.space.RenameBranch(ctx, call, args)
		if err != nil {
			return one(stateFail(d.action, args.Repo+"/"+args.To, err), "")
		}
		return one(outcomeItem(call, args.Repo+"/"+args.To, res.Outcome, BranchRenamed, res.Next, res.Waited, "answers[]"), nextOfOutcome(res.Outcome))
	case "branch.change":
		args := ChangeBranchArgs{Repo: d.str("repo"), From: d.str("from"), To: d.str("to")}
		if args.From == "" || args.To == "" || args.From == args.To {
			return one(invalidItem(d.action, "from and to must be two different branch names."), "")
		}
		res, err := s.space.ChangeBranch(ctx, call, args)
		if err != nil {
			return one(stateFail(d.action, args.Repo+"/"+args.To, err), "")
		}
		return one(outcomeItem(call, args.Repo+"/"+args.To, res.Outcome, BranchChanged, res.Next, res.Waited, "answers[]"), nextOfOutcome(res.Outcome))
	case "lease.acquire":
		queue := d.str("queue")
		if queue == "" {
			return one(invalidItem(d.action, "queue is required."), "")
		}
		res, err := s.space.AcquireLease(ctx, call, AcquireLeaseArgs{Queue: queue})
		if err != nil {
			return one(stateFail(d.action, queue, err), "")
		}
		it := Item{ID: queue, Status: StatusDone, Message: res.Next}
		next := ""
		if res.State == LeaseQueued {
			it.Status = StatusWaiting
			it.Message = notFinished(call, res.Waited, "leases", leaseWaitDetail(res))
			next = "Do other work, or check leases in state.yaml."
		}
		return one(it, next, res.Notices...)
	case "lease.release":
		res, err := s.space.ReleaseLease(ctx, call, ReleaseLeaseArgs{Queue: d.str("queue")})
		if err != nil {
			return one(stateFail(d.action, cmpNonEmpty(d.str("queue"), "leases"), err), "")
		}
		msg := "No lease was held."
		if len(res.Released) > 0 {
			msg = "Released: " + strings.Join(res.Released, ", ") + "."
		}
		return one(Item{ID: cmpNonEmpty(d.str("queue"), "leases"), Status: StatusDone, Message: msg}, "", res.Notices...)
	case "operation.cancel":
		opID := d.str("opId")
		res, err := s.space.CancelOperation(ctx, call.TaskID, opID)
		if err != nil {
			return one(stateFail(d.action, opID, err), "")
		}
		return one(Item{ID: opID, Status: StatusDone, Message: fmt.Sprintf("Cancelled the %s operation.", res.Kind)}, "")
	case "remote.fetch":
		return s.sandboxState(ctx, req, d, func(ctx context.Context, g Grant) (RemoteResult, error) {
			remote := remoteName(d.str("remote"))
			if len(remote) > 255 || strings.HasPrefix(remote, "-") {
				return RemoteResult{}, ToolError("remote must be a remote name of your repo.")
			}
			return s.sandbox.FetchRemote(ctx, g.RunID, remote)
		})
	case "branch.push":
		return s.sandboxState(ctx, req, d, func(ctx context.Context, g Grant) (RemoteResult, error) {
			if !g.Sandbox.Push {
				return RemoteResult{}, ToolError("push is not allowed in a read-only step.")
			}
			return s.sandbox.PushBranch(ctx, g.RunID, d.boolean("force"))
		})
	case "review.reply":
		return one(s.reviewState(g, d), "")
	default: // step.finish
		return s.finishState(ctx, req, g, d)
	}
}

func cmpNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func declaredText(info TaskInfo) string {
	parts := make([]string, 0, len(info.Repos))
	for _, r := range info.Repos {
		if r.Branch != nil {
			parts = append(parts, r.Repo+" ("+*r.Branch+")")
		} else {
			parts = append(parts, r.Repo+" (no branch yet)")
		}
	}
	return "Task repos: " + strings.Join(parts, ", ") + "."
}

// waitedText is the wait of a call in words: "10 minutes", "5 seconds".
func waitedText(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%d minutes", int(d/time.Minute))
	}
	return fmt.Sprintf("%d seconds", int(d/time.Second))
}

// notFinished is the message of a call that ended its wait with the work still open. fields names the
// state.yaml fields that change when it finishes; detail says what is open.
func notFinished(call SpaceCall, waited time.Duration, fields, detail string) string {
	m := fmt.Sprintf("Not finished after %s", waitedText(waited))
	if detail != "" {
		m += ": " + detail
	} else {
		m += "."
	}
	m = strings.TrimSuffix(m, ".") + ". Check state.yaml in the task folder: " + fields + ". Do not act on it until it shows done."
	if call.RunID == "" {
		m += " Kira also tells you when it changes."
	}
	return m
}

func leaseWaitDetail(res AcquireLeaseResult) string {
	d := fmt.Sprintf("queued at position %d", res.Position)
	if res.HolderTask != "" {
		d += " behind task " + res.HolderTask
	}
	return d
}

// branchItem is the item of one branch.request result.
func branchItem(call SpaceCall, rb RequestedBranch, waited time.Duration) Item {
	id := rb.Repo + "/" + rb.Branch
	switch rb.Outcome {
	case BranchCreated, BranchExisting, BranchCheckedOut:
		if rb.Setup.State == "failed" {
			return Item{ID: id, Status: StatusFailed, Message: rb.Next}
		}
		return Item{ID: id, Status: StatusDone, Message: rb.Next}
	case BranchPending:
		return Item{ID: id, Status: StatusPending, Message: notFinished(call, waited, "pending[] and answers[]", "the user has not answered")}
	case BranchWaiting:
		detail := "the worktree is still preparing"
		if rb.Setup.Note != "" {
			detail += " (" + rb.Setup.Note + ")"
		}
		return Item{ID: id, Status: StatusWaiting, Message: notFinished(call, waited, "repos[].setup of "+id, detail)}
	case BranchDenied:
		return Item{ID: id, Status: StatusDenied, Message: rb.Next}
	}
	return Item{ID: id, Status: StatusFailed, Message: rb.Next}
}

// outcomeItem is the item of a rename or change: ok is the success outcome.
func outcomeItem(call SpaceCall, id, outcome, ok, next string, waited time.Duration, fields string) Item {
	switch outcome {
	case ok:
		return Item{ID: id, Status: StatusDone, Message: next}
	case BranchPending:
		return Item{ID: id, Status: StatusPending, Message: notFinished(call, waited, fields, "the user has not answered")}
	case BranchDenied:
		return Item{ID: id, Status: StatusDenied, Message: next}
	}
	return Item{ID: id, Status: StatusFailed, Message: next}
}

func nextOfOutcome(outcome string) string {
	if outcome == BranchPending {
		return "Do other work; read state.yaml later."
	}
	return ""
}

// nextOf is the reply's next line for a set of branch items.
func nextOf(items []Item) string {
	for _, it := range items {
		if it.Status == StatusPending || it.Status == StatusWaiting {
			return "Do other work; read state.yaml later."
		}
	}
	return ""
}

func (s *Server) sandboxState(ctx context.Context, req *mcp.CallToolRequest, d stateCallArgs, run func(context.Context, Grant) (RemoteResult, error)) stateOutcome {
	g, ok := s.runGrant(req)
	if !ok || s.sandbox == nil {
		return stateOutcome{reply: oneItem(Item{ID: d.action, Status: StatusInvalid, Message: d.action + " is not available for this session."}, "", nil)}
	}
	res, err := run(ctx, g)
	if err != nil {
		return stateOutcome{reply: oneItem(stateFail(d.action, d.action, err), "", nil)}
	}
	msg := fmt.Sprintf("%s %s.", map[string]string{"remote.fetch": "Fetched", "branch.push": "Pushed"}[d.action], res.Remote)
	if res.Branch != "" && d.action == "branch.push" {
		msg = fmt.Sprintf("Pushed %s to %s.", res.Branch, res.Remote)
	}
	if out := trimRemoteOutput(res.Output); out != "" {
		msg += "\n" + out
	}
	return stateOutcome{reply: oneItem(Item{ID: d.action, Status: StatusDone, Message: msg}, "", nil)}
}

func (s *Server) reviewState(g Grant, d stateCallArgs) Item {
	id := strings.TrimPrefix(d.str("id"), "#")
	if g.RunID == "" || g.Review == nil || s.review == nil {
		return Item{ID: id, Status: StatusInvalid, Message: "review.reply is not available for this session."}
	}
	if !slices.Contains(g.Review.CommentIDs, id) {
		return Item{ID: id, Status: StatusInvalid, Message: fmt.Sprintf("comment %s was not sent to you.", id), Fix: exampleCall("review.reply")}
	}
	reply := strings.TrimSpace(d.raw("reply"))
	if reply == "" {
		return Item{ID: id, Status: StatusInvalid, Message: "reply must not be empty.", Fix: exampleCall("review.reply")}
	}
	if len(reply) > MaxReviewReplyBytes {
		return Item{ID: id, Status: StatusInvalid, Message: fmt.Sprintf("reply is too long: at most %d bytes.", MaxReviewReplyBytes), Fix: exampleCall("review.reply")}
	}
	if err := s.review.Reply(g.RunID, id, reply, d.boolean("resolved")); err != nil {
		return Item{ID: id, Status: StatusInvalid, Message: err.Error(), Fix: exampleCall("review.reply")}
	}
	return Item{ID: id, Status: StatusDone, Message: "Saved."}
}

// finishState is step.finish: the finish of a run (its outcome goes to Kira) or of a session stage
// (the draft handover is marked finished and the user is asked to move the task on).
func (s *Server) finishState(ctx context.Context, req *mcp.CallToolRequest, g Grant, d stateCallArgs) stateOutcome {
	args := finishArgs{Status: d.str("status"), Reason: d.raw("reason"), ConflictedFiles: d.list("conflictedFiles"), LastGitError: d.raw("lastGitError"), Tried: d.raw("tried")}
	fail := func(msg string) stateOutcome {
		return stateOutcome{reply: oneItem(Item{ID: "step", Status: StatusInvalid, Message: msg, Fix: exampleCall("step.finish")}, "", nil)}
	}
	if g.RunID == "" {
		if args.Status != finishStatusDone {
			return fail(`In a session only status "done" ends the step. To ask the user something, say it in your reply.`)
		}
		if s.space == nil {
			return fail("step.finish is not available for this session.")
		}
		note, err := s.space.FinishSession(ctx, SpaceCall{TaskID: g.TaskID, StepID: g.StepID, SessionID: g.SessionID, Orchestrator: g.Orchestrator})
		if err != nil {
			return stateOutcome{reply: oneItem(stateFail("step.finish", "step", err), "", nil)}
		}
		return stateOutcome{reply: oneItem(Item{ID: "step", Status: StatusDone, Message: note}, "", nil)}
	}
	ack, refuse, err := s.finishRun(ctx, req, g, args)
	switch {
	case err != nil:
		return stateOutcome{reply: oneItem(stateFail("step.finish", "step", err), "", nil)}
	case refuse != "":
		return fail(refuse)
	}
	return stateOutcome{reply: oneItem(Item{ID: "step", Status: StatusDone, Message: ack}, "", nil), extra: s.artifactBlocks(ctx, req, g)}
}

func (s *Server) addStateTool(srv *mcp.Server, ts toolSet) {
	srv.AddTool(s.stateTool(ts), s.stateCall)
}
