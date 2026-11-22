package llmprompts

// The one file with prompt text. Every LLM prompt and LLM-facing instruction either app sends is an
// entry here; callers read the effective text through the handles below. Text is a Go
// text/template: an entry without variables and fragments is literal and never parsed.

var (
	spaceApp  = []App{Space}
	studioApp = []App{Studio}
	bothApps  = []App{Studio, Space}
)

// Variables. Fields tagged prompt:"required" must appear in an edited template.

// CountVars fills a text with a count.
type CountVars struct {
	Count int `json:"count"`
}

// MaxVars fills an argument description with an upper bound.
type MaxVars struct {
	Max int `json:"max"`
}

// LimitVars fills an argument description with a default and an upper bound.
type LimitVars struct {
	Default int `json:"default"`
	Max     int `json:"max"`
}

// SandboxNoteVars fills the sandbox note appended to a sandboxed run's prompt. Tools is true when
// the run has the Kira git tools in place of the shell git commands the sandbox blocks.
type SandboxNoteVars struct {
	Access   string   `json:"access"`
	Worktree string   `json:"worktree"`
	Read     []string `json:"read"`
	Network  bool     `json:"network"`
	Enforced bool     `json:"enforced"`
	Tools    bool     `json:"tools"`
}

// ResultVar is one declared result of a step.
type ResultVar struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	OK          bool   `json:"ok"`
}

// FinishVars fills the finish instruction. Implicit is true when the step declares no results
// beyond the implicit done and failed pair.
type FinishVars struct {
	Implicit bool `json:"implicit"`
	// Plain names the finish_step tool: a run without a Space grant, such as a review round, has no
	// state step.finish.
	Plain   bool        `json:"plain"`
	Results []ResultVar `json:"results"`
}

// StatusVars fills the finish_step status argument description.
type StatusVars struct {
	Results []ResultVar `json:"results" prompt:"required"`
}

// RebaseStackVar is one branch of a rebase run, root first.
type RebaseStackVar struct {
	Name         string `json:"name"`
	Worktree     string `json:"worktree"`
	ParentRef    string `json:"parentRef"`
	ParentName   string `json:"parentName"`
	ParentBefore string `json:"parentBefore"`
}

// RebaseVars fills the rebase prompt. The caller sends no prompt for an empty stack.
type RebaseVars struct {
	Repo      string           `json:"repo"`
	Onto      string           `json:"onto" prompt:"required"`
	Remote    string           `json:"remote"`
	Review    string           `json:"review"`
	Autostash bool             `json:"autostash"`
	Push      bool             `json:"push"`
	Stack     []RebaseStackVar `json:"stack" prompt:"required"`
}

// RepoLineVars is one repository of a launch message. ReadOnlyRoot, when set, makes it read-only.
type RepoLineVars struct {
	Repo         string `json:"repo"`
	Branch       string `json:"branch"`
	Worktree     string `json:"worktree"`
	Base         string `json:"base"`
	ReadOnlyRoot string `json:"readOnlyRoot"`
	// ChangeRequestURL is the branch's pull or merge request link, in review messages only.
	ChangeRequestURL string `json:"changeRequestURL"`
}

// HandoverBlockVars fills the handover block fragment.
type HandoverBlockVars struct {
	Handover     string `json:"handover"`
	HandoverFrom string `json:"handoverFrom"`
}

// PassHandoverVars fills the handover that starts an orchestrator pass: the user's optional message and the
// previous pass's final handover.
type PassHandoverVars struct {
	Message      string `json:"message"`
	Previous     string `json:"previous"`
	PreviousFrom string `json:"previousFrom"`
}

// JiraLineVars fills the Jira line.
type JiraLineVars struct {
	JiraKey string `json:"jiraKey"`
	JiraURL string `json:"jiraURL"`
}

// StepVars fills the step prompt. Message and StepPrompt arrive with placeholders substituted and
// trailing newlines trimmed. A non-empty Message replaces the composed header.
type StepVars struct {
	Title    string `json:"title"`
	JiraKey  string `json:"jiraKey"`
	JiraURL  string `json:"jiraURL"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
	// Outside says the step runs outside the repos, in the task folder (Worktree).
	Outside    bool       `json:"outside"`
	StepNumber int        `json:"stepNumber"`
	StepCount  int        `json:"stepCount"`
	StepName   string     `json:"stepName"`
	StepPrompt string     `json:"stepPrompt"`
	Message    string     `json:"message"`
	Extra      string     `json:"extra"`
	Context    []string   `json:"context"`
	Finish     FinishVars `json:"finish"`
	// Notes are the task notes; Handover is what the previous stage left, HandoverFrom its stage name.
	// All arrive capped and outside Message, so a Run dialog edit keeps them.
	Notes        string    `json:"notes"`
	Handover     string    `json:"handover"`
	HandoverFrom string    `json:"handoverFrom"`
	Files        FilesVars `json:"files"`
}

// FilesVars fills the task-files fragment: the task folder, the step's draft file and the provider's tool-loading sentence.
// A zero value drops the fragment (a board with no files root).
type FilesVars struct {
	Folder string `json:"folder"`
	// Draft is the step's handover draft path, set only while the step's route continues.
	Draft string `json:"draft"`
	Load  string `json:"load"`
	// Proofs are the task's proofs, one index line each ("slug "Name": id (view), ..."), at most 5.
	Proofs []string `json:"proofs"`
}

// ManualVars fills the manual step message. StepPrompt arrives substituted; Notes arrive capped.
type ManualVars struct {
	StepName string         `json:"stepName"`
	Title    string         `json:"title"`
	JiraKey  string         `json:"jiraKey"`
	JiraURL  string         `json:"jiraURL"`
	Notes    string         `json:"notes"`
	Repos    []RepoLineVars `json:"repos"`
	// Folder is the task folder, shown while the task has no repo.
	Folder string `json:"folder"`
	// Outside says the step runs outside the repos, in Folder.
	Outside    bool   `json:"outside"`
	StepPrompt string `json:"stepPrompt"`
	// Handover is what the previous stage left, HandoverFrom its stage name; both capped.
	Handover     string    `json:"handover"`
	HandoverFrom string    `json:"handoverFrom"`
	Files        FilesVars `json:"files"`
}

// ReviewVars fills the review message. Notes arrive capped.
type ReviewVars struct {
	Title   string         `json:"title"`
	JiraKey string         `json:"jiraKey"`
	JiraURL string         `json:"jiraURL"`
	Notes   string         `json:"notes"`
	Repos   []RepoLineVars `json:"repos"`
	// Handover is what the previous stage left, HandoverFrom its stage name; both capped.
	Handover     string    `json:"handover"`
	HandoverFrom string    `json:"handoverFrom"`
	Files        FilesVars `json:"files"`
}

// StartVars fills the Start message; StepName is empty when the workflow has no open step.
type StartVars struct {
	Title        string   `json:"title"`
	JiraKey      string   `json:"jiraKey"`
	JiraURL      string   `json:"jiraURL"`
	Repo         string   `json:"repo"`
	Branch       string   `json:"branch"`
	Worktree     string   `json:"worktree"`
	ReadOnlyRoot string   `json:"readOnlyRoot"`
	StepName     string   `json:"stepName"`
	Context      []string `json:"context"`
	// Handover is what the previous stage left, HandoverFrom its stage name; both capped.
	Handover     string    `json:"handover"`
	HandoverFrom string    `json:"handoverFrom"`
	Files        FilesVars `json:"files"`
}

// RebaseContextVars fills the line about a branch whose last rebase did not finish.
type RebaseContextVars struct {
	Branch  string `json:"branch"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
	RunID   string `json:"runID" prompt:"required"`
}

// ReviewCommentVar is one review comment sent to an agent.
type ReviewCommentVar struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Lines string `json:"lines"`
	Body  string `json:"body"`
	// Uncommitted marks a comment written against worktree content no commit holds yet.
	Uncommitted bool `json:"uncommitted"`
}

// ReviewRoundVars fills the review round messages and the comment list fragment.
type ReviewRoundVars struct {
	Title    string             `json:"title"`
	JiraKey  string             `json:"jiraKey"`
	JiraURL  string             `json:"jiraURL"`
	Repo     string             `json:"repo"`
	Branch   string             `json:"branch"`
	Base     string             `json:"base"`
	Worktree string             `json:"worktree"`
	Round    int                `json:"round"`
	Comments []ReviewCommentVar `json:"comments"`
	// ChangeRequestURL is the branch's pull or merge request link, in the round message only.
	ChangeRequestURL string `json:"changeRequestURL"`
}

// PanelForwardItemVar is one panel change: a label and its diff text.
type PanelForwardItemVar struct {
	Label string `json:"label"`
	Diff  string `json:"diff"`
}

// PanelForwardVars fills the forwarded panel changes message.
type PanelForwardVars struct {
	Items []PanelForwardItemVar `json:"items"`
}

// ResumeVars fills the resume-after-restart message.
type ResumeVars struct {
	App string `json:"app" prompt:"required"`
	// Leases adds the line about resource leases, which a restart drops.
	Leases bool `json:"leases"`
}

// SendBackVars fills the send-back message.
type SendBackVars struct {
	From   string `json:"from"`
	Branch string `json:"branch"`
	Reason string `json:"reason" prompt:"required"`
}

func arg[V any](parent ID, field string, apps []App, applies Applies, text string, example V) Prompt[V] {
	return define(Entry{
		ID: ID(string(parent) + ".arg." + field), Kind: KindArgument, Apps: apps, Parent: parent,
		Field: field, Name: field, Applies: applies, Default: text,
	}, example)
}

// Headless agent runs.

// HeadlessFinishStepSuffix ends every agent step prompt.
var HeadlessFinishStepSuffix = define(Entry{
	ID: "headless.finishStep.suffix", Kind: KindSuffix, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessFinishStepSuffix.title",
	PurposeKey: "llmPrompts.entry.headlessFinishStepSuffix.description",
	UsedByKey:  "llmPrompts.entry.headlessFinishStepSuffix.usedBy.body",
	Default:    `{{if .Implicit}}When this step is done, call {{if .Plain}}finish_step with status "done"{{else}}state with action step.finish and args status "done"{{end}}. If it failed, use status "failed", what failed and the reason. If you need a decision from me, use status "needs_input" and your question.{{else}}When this step ends, call {{if .Plain}}finish_step once with status one of: {{else}}state with action step.finish once, args status one of: {{end}}{{range $i, $r := .Results}}{{if $i}}, {{end}}{{$r.ID}} ({{if $r.OK}}ok{{else}}not ok{{end}}{{if $r.Description}}: {{$r.Description}}{{end}}){{end}}. For a not-ok status add the reason. If you need a decision from me, use status "needs_input" with your question.{{end}}{{if not .Plain}} Write a proof for any checkable result before finishing.{{end}}`,
}, FinishVars{Implicit: true, Results: []ResultVar{{ID: "done", OK: true}, {ID: "failed"}}})

// HeadlessSpaceSuffix goes before the finish instruction when the Kira Space tools are on.
var HeadlessSpaceSuffix = define(Entry{
	ID: "headless.space.suffix", Kind: KindSuffix, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessSpaceSuffix.title",
	PurposeKey: "llmPrompts.entry.headlessSpaceSuffix.description",
	UsedByKey:  "llmPrompts.entry.headlessSpaceSuffix.usedBy.body",
	Default:    `Kira tool: state. Your working directory may be a repo root: read it, change nothing there. Create branches and worktrees only with state branch.request and remove them only with state worktree.remove; you name each. Never git branch, git worktree or git checkout -b. Work in the returned worktrees.`,
}, NoVars{})

// HeadlessSpaceFiles names the task folder, the step's draft file when its route continues and how to load the tools.
var HeadlessSpaceFiles = define(Entry{
	ID: "headless.space.files", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessSpaceFiles.title",
	PurposeKey: "llmPrompts.entry.headlessSpaceFiles.description",
	UsedByKey:  "llmPrompts.entry.headlessSpaceFiles.usedBy.body",
	Default: `This task's Details files live in {{.Folder}}; see README.md there. Kira applies your edits on save; read state.yaml fileErrors after each edit.{{if .Draft}} Write your handover to {{.Draft}} before you finish.{{end}} When the step is done, call state with step.finish.{{range $i, $p := .Proofs}}
{{if eq $i 0}}Proofs: {{else}}  {{end}}{{$p}}{{end}}{{if .Load}} {{.Load}}{{end}}`,
}, FilesVars{Folder: "/home/me/.kira-space/tasks/t1", Draft: "handovers/draft-implement.md", Load: `Load the tools first: ToolSearch with query "select:mcp__kira-space__state".`})

// HeadlessSandboxNote tells a sandboxed agent what it may write and what to do on a denial.
var HeadlessSandboxNote = define(Entry{
	ID: "headless.sandbox.note", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessSandboxNote.title",
	PurposeKey: "llmPrompts.entry.headlessSandboxNote.description",
	UsedByKey:  "llmPrompts.entry.headlessSandboxNote.usedBy.body",
	Default: `Sandbox{{if not .Enforced}} (declared, not enforced){{end}}: {{if eq .Access "write"}}write only in {{.Worktree}} and $TMPDIR{{else}}read only; write only in $TMPDIR{{end}}.{{range .Read}}
Read also: {{.}}{{end}}{{if not .Network}}
Network: Claude API only.{{end}}{{if .Tools}}
No git branch, worktree, tag, stash, fetch or push. Use state: branch.request, remote.fetch, branch.push.{{end}}
"Operation not permitted" means the sandbox blocked it. Do not retry. Finish with status failed (state step.finish, or finish_step); name the path in reason.`,
}, SandboxNoteVars{
	Access: "write", Worktree: "/work/kira-space/feat-login", Read: []string{"/work/specs"}, Network: false, Enforced: true, Tools: true,
})

// HeadlessRebaseReportSuffix ends every rebase run's prompt.
var HeadlessRebaseReportSuffix = define(Entry{
	ID: "headless.rebase.reportSuffix", Kind: KindSuffix, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessRebaseReportSuffix.title",
	PurposeKey: "llmPrompts.entry.headlessRebaseReportSuffix.description",
	UsedByKey:  "llmPrompts.entry.headlessRebaseReportSuffix.usedBy.body",
	Default:    `Run git only with the git tool: pass the worktree path from the list above and the words after git as args. No shell. When finished, call finish_step. status "done" only when every branch above is rebased and no rebase is in progress. status "failed" with reason, conflictedFiles, lastGitError and tried when you could not finish; leave a rebase in progress only when you could not resolve it, and say so. status "needs_input" with your question when you need a decision. Kira Space checks the result in git.`,
}, NoVars{})

// HeadlessScriptReportSuffix ends every smart script prompt.
var HeadlessScriptReportSuffix = define(Entry{
	ID: "headless.script.reportSuffix", Kind: KindSuffix, Apps: bothApps, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessScriptReportSuffix.title",
	PurposeKey: "llmPrompts.entry.headlessScriptReportSuffix.description",
	UsedByKey:  "llmPrompts.entry.headlessScriptReportSuffix.usedBy.body",
	Default:    `When finished, call finish_step (or state with action step.finish, when finish_step is not listed) with status "done" and a one-line reason. If you could not finish, use status "failed" and the reason. If you need a decision, use status "needs_input" and your question.`,
}, NoVars{})

// HeadlessSpaceInstructions is what the Kira Space tools server tells a client when it connects.
var HeadlessSpaceInstructions = define(Entry{
	ID: "headless.space.instructions", Kind: KindInstructions, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessSpaceInstructions.title",
	PurposeKey: "llmPrompts.entry.headlessSpaceInstructions.description",
	UsedByKey:  "llmPrompts.entry.headlessSpaceInstructions.usedBy.body",
	Default:    `Use the state tool for every task state change. Task state is in state.yaml in the task folder. Create, change or remove branches and worktrees only with state; never git branch or git worktree. A call that waits returns within 10 minutes; if it is not done, check the state.yaml fields it names. Kira applies task file edits on save; read state.yaml fileErrors after each edit; end the step with state step.finish.`,
}, NoVars{})

// HeadlessToolFinishStep describes the finish_step tool.
var HeadlessToolFinishStep = define(Entry{
	ID: "headless.tool.finish_step", Kind: KindTool, Apps: bothApps, Applies: NextCall, Name: "finish_step",
	Default: `Report how this pipeline step ended. Call it exactly once, as the last action.`,
}, NoVars{})

var (
	_ = arg(HeadlessToolFinishStep.id, "status", bothApps, NextCall,
		`How the step ended. {{range .Results}}{{.ID}} ({{if .OK}}ok{{else}}not ok{{end}}{{if .Description}}: {{.Description}}{{end}}). {{end}}needs_input when the user must decide.`,
		StatusVars{Results: []ResultVar{{ID: "done", OK: true}, {ID: "failed"}}})
	_ = arg(HeadlessToolFinishStep.id, "reason", bothApps, NextCall, `Why it failed, or what you need. Required when status is not ok or needs_input.`, NoVars{})
	_ = arg(HeadlessToolFinishStep.id, "conflictedFiles", bothApps, NextCall, `Files that still conflict, relative to the repo.`, NoVars{})
	_ = arg(HeadlessToolFinishStep.id, "lastGitError", bothApps, NextCall, `Last git error output you saw.`, NoVars{})
	_ = arg(HeadlessToolFinishStep.id, "tried", bothApps, NextCall, `What you tried before giving up.`, NoVars{})
)

// HeadlessFinishStepAck is the reply to a finish_step call.
var HeadlessFinishStepAck = define(Entry{
	ID: "headless.finishStep.ack", Kind: KindToolResult, Apps: bothApps, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessFinishStepAck.title",
	PurposeKey: "llmPrompts.entry.headlessFinishStepAck.description",
	UsedByKey:  "llmPrompts.entry.headlessFinishStepAck.usedBy.body",
	Default:    `Recorded. Stop now.`,
}, NoVars{})

// HeadlessToolState describes the state tool: the header of its description. Each granted action adds
// its own line (HeadlessStateAction*).
var HeadlessToolState = define(Entry{
	ID: "headless.tool.state", Kind: KindTool, Apps: spaceApp, Applies: NextCall, Name: "state",
	Default: `Every task state change goes through this tool. Pick action; put the action's fields in args. A call that waits returns within 10 minutes. Reply: a summary line, then one item per change with status done, pending, waiting, denied, failed or invalid; invalid has fix.`,
}, NoVars{})

var (
	_ = arg(HeadlessToolState.id, "action", spaceApp, NextCall, `What to do; see the tool description.`, NoVars{})
	_ = arg(HeadlessToolState.id, "args", spaceApp, NextCall, `The action's fields, as an object.`, NoVars{})
)

func defineStateAction(name, text string) Prompt[NoVars] {
	return define(Entry{
		ID: ID("headless.tool.state.action." + name), Kind: KindTool, Apps: spaceApp, Applies: NextCall,
		Name: name, Default: text,
	}, NoVars{})
}

// The state action lines: one entry per action, named in the state description of a grant that holds it.
var (
	HeadlessStateRepoDeclare     = defineStateAction("repo.declare", `repo.declare: args {repos}: repos the work touches.`)
	HeadlessStateBranchRequest   = defineStateAction("branch.request", `branch.request: args {repo, branch, base?, existing?}: create a branch and its worktree, or check out an existing one (user approves only when another worktree holds it); waits until its worktree is ready.`)
	HeadlessStateBranchRename    = defineStateAction("branch.rename", `branch.rename: args {repo, from, to, renameRemote?}: rename your branch, and with renameRemote its remote branch too (user approves).`)
	HeadlessStateBranchChange    = defineStateAction("branch.change", `branch.change: args {repo, from, to}: point task branch from at existing local branch to (user approves).`)
	HeadlessStateWorktreeRemove  = defineStateAction("worktree.remove", `worktree.remove: args {repo, branch}: remove your branch's worktree; the branch stays in git. Refused while it has uncommitted changes or anything works in it.`)
	HeadlessStateLeaseAcquire    = defineStateAction("lease.acquire", `lease.acquire: args {queue}: wait for the queue's lease.`)
	HeadlessStateLeaseRelease    = defineStateAction("lease.release", `lease.release: args {queue?}: leave one queue, or every queue.`)
	HeadlessStateOperationCancel = defineStateAction("operation.cancel", `operation.cancel: args {opId}: stop a running operation (state.yaml operations).`)
	HeadlessStateRemoteFetch     = defineStateAction("remote.fetch", `remote.fetch: args {remote?}: fetch a remote of your repo.`)
	HeadlessStateBranchPush      = defineStateAction("branch.push", `branch.push: args {force?}: push your worktree's branch.`)
	HeadlessStateReviewReply     = defineStateAction("review.reply", `review.reply: args {id, reply, resolved?}: answer one review comment you were sent.`)
	HeadlessStateStepFinish      = defineStateAction("step.finish", `step.finish: args {status, reason?, conflictedFiles?, lastGitError?, tried?}: end the step; applies your latest task file edits first. Status values: see the step prompt.`)
)

// HeadlessArtifactsHeader opens the artifact list a finishing call returns.
var HeadlessArtifactsHeader = define(Entry{
	ID: "headless.artifacts.header", Kind: KindToolResult, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessArtifactsHeader.title",
	PurposeKey: "llmPrompts.entry.headlessArtifactsHeader.description",
	UsedByKey:  "llmPrompts.entry.headlessArtifactsHeader.usedBy.body",
	Default:    `Every artifact of this task. Name them in your final message to the user.`,
}, NoVars{})

// HeadlessArtifactsMore ends a list cut short.
var HeadlessArtifactsMore = define(Entry{
	ID: "headless.artifacts.more", Kind: KindToolResult, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessArtifactsMore.title",
	PurposeKey: "llmPrompts.entry.headlessArtifactsMore.description",
	UsedByKey:  "llmPrompts.entry.headlessArtifactsMore.usedBy.body",
	Default:    `{{.Count}} more artifacts not shown.`,
}, CountVars{Count: 1})

// HeadlessArtifactsUnavailable replaces the artifact list when it could not be read.
var HeadlessArtifactsUnavailable = define(Entry{
	ID: "headless.artifacts.unavailable", Kind: KindToolResult, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.headlessArtifactsUnavailable.title",
	PurposeKey: "llmPrompts.entry.headlessArtifactsUnavailable.description",
	UsedByKey:  "llmPrompts.entry.headlessArtifactsUnavailable.usedBy.body",
	Default:    `Artifacts could not be listed.`,
}, NoVars{})

// HeadlessToolGit describes the git tool.
var HeadlessToolGit = define(Entry{
	ID: "headless.tool.git", Kind: KindTool, Apps: spaceApp, Applies: NextCall, Name: "git",
	Default: `Run git in a worktree named in the prompt. Allowed: status, diff, log, show, rev-parse, merge-base, ls-files, fetch <remote>, rebase (not interactive), add, rm, checkout --ours|--theirs -- <path>, and push --force-with-lease when the prompt asks for it. Options outside those shapes are refused. Put paths after a "--" entry.`,
}, NoVars{})

var (
	_ = arg(HeadlessToolGit.id, "worktree", spaceApp, NextCall, `A worktree path named in the prompt.`, NoVars{})
	_ = arg(HeadlessToolGit.id, "args", spaceApp, NextCall, `Words after git, one per entry, such as ["rebase","--continue"]. Paths go after a "--" entry.`, NoVars{})
)

// HeadlessToolCreateTask describes the create_task tool.
var HeadlessToolCreateTask = define(Entry{
	ID: "headless.tool.create_task", Kind: KindTool, Apps: spaceApp, Applies: NextCall, Name: "create_task",
	Default: `Ask the user to approve a new child task; waits for the answer. The brief is the child's first handover. Child tasks cannot create tasks. Every call waits for the user, so ask for what you need in one call. Outcome: created, denied, timeout or failed. Read children in state.yaml (chain) and their folders; you are not woken when a child finishes.`,
}, NoVars{})

var (
	_ = arg(HeadlessToolCreateTask.id, "title", spaceApp, NextCall, `Child task title, 1 to 200 characters.`, NoVars{})
	_ = arg(HeadlessToolCreateTask.id, "workflow", spaceApp, NextCall, `Workflow id or name. It must exist and have a step.`, NoVars{})
	_ = arg(HeadlessToolCreateTask.id, "repos", spaceApp, NextCall, `Repos of the child, 0 to {{.Max}}. Each gets a branch slot the child names later.`, MaxVars{Max: 10})
	_ = arg(HeadlessToolCreateTask.id, "repos.repo", spaceApp, NextCall, `Repo: nickname, name or code repo id, from state.yaml.`, NoVars{})
	_ = arg(HeadlessToolCreateTask.id, "repos.base", spaceApp, NextCall, `Ref the child's branch starts from, such as main. Default: the repo's default branch.`, NoVars{})
	_ = arg(HeadlessToolCreateTask.id, "brief", spaceApp, NextCall, `What the child must do, what it needs to know and what to hand back. Markdown, at most {{.Max}} bytes.`, MaxVars{Max: 16384})
)

// ADE.

// AdeRebasePrompt is the rebase prompt the preview shows and the run sends.
var AdeRebasePrompt = define(Entry{
	ID: "ade.rebase.prompt", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeRebasePrompt.title",
	PurposeKey: "llmPrompts.entry.adeRebasePrompt.description",
	UsedByKey:  "llmPrompts.entry.adeRebasePrompt.usedBy.body",
	Default: `{{if .Stack}}Rebase {{(index .Stack 0).Name}} (repo {{.Repo}}) onto {{.Onto}}{{if gt (len .Stack) 1}}, then restack branches built on it{{end}}:
{{range $i, $s := .Stack}}{{inc $i}}. In {{$s.Worktree}}: {{if $i}}git rebase {{if $.Autostash}}--autostash {{end}}--onto {{$s.ParentName}} {{$s.ParentBefore}}
{{else}}{{if $.Remote}}git fetch {{$.Remote}}, then {{end}}git rebase {{if $.Autostash}}--autostash {{end}}{{$s.ParentRef}}
{{end}}{{end}}{{if .Review}}Do not modify {{.Review}}.
{{end}}Resolve conflicts by editing files, then git add and git rebase --continue.
{{if .Push}}Then push each rebased branch with git push --force-with-lease.{{else}}Do not push.{{end}}{{end}}`,
}, RebaseVars{
	Repo: "kira-space", Onto: "origin/main", Remote: "origin", Autostash: true,
	Stack: []RebaseStackVar{
		{Name: "feat/a", Worktree: "/work/kira-space/feat-a", ParentRef: "origin/main"},
		{Name: "feat/b", Worktree: "/work/kira-space/feat-b", ParentRef: "feat/a", ParentName: "feat/a", ParentBefore: "1a2b3c4"},
	},
})

// AdeStepPrompt is the default prompt of an agent step, ending with the finish instruction.
var AdeStepPrompt = define(Entry{
	ID: "ade.step.prompt", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:          "llmPrompts.entry.adeStepPrompt.title",
	PurposeKey:        "llmPrompts.entry.adeStepPrompt.description",
	UsedByKey:         "llmPrompts.entry.adeStepPrompt.usedBy.body",
	RequiredFragments: []ID{"headless.finishStep.suffix"},
	Default: `{{if .Message}}{{.Message}}{{else}}Task: {{.Title}}
{{if .JiraKey}}{{template "ade.jiraLine" .}}
{{end}}{{if .Repo}}- Repo: {{.Repo}} · Branch: {{.Branch}} · Worktree: {{.Worktree}}{{else if .Outside}}- Outside the repos. Folder: {{.Worktree}}.{{else}}- No repo yet. Folder: {{.Worktree}}. Pick repos and branches with the Kira Space tools.{{end}}
Step {{.StepNumber}}/{{.StepCount}}: {{.StepName}}{{if .StepPrompt}}
{{.StepPrompt}}{{end}}{{end}}{{if .Extra}}

{{.Extra}}{{end}}{{if .Context}}

{{join .Context "\n"}}{{end}}{{if .Notes}}

- Notes: {{.Notes}}{{end}}{{if .Handover}}

{{template "ade.handoverBlock" .}}{{end}}

{{template "headless.space.suffix"}}{{if .Files.Folder}}
{{template "headless.space.files" .Files}}{{end}}

{{template "headless.finishStep.suffix" .Finish}}
`,
}, StepVars{
	Title: "Add login form", JiraKey: "KIRA-42", JiraURL: "https://jira.example.com/browse/KIRA-42",
	Repo: "kira-space", Branch: "feat/login", Worktree: "/work/kira-space/feat-login",
	StepNumber: 2, StepCount: 5, StepName: "Implement", StepPrompt: "Implement the task in the worktree.",
	Context: []string{}, Finish: FinishVars{Implicit: true, Results: []ResultVar{{ID: "done", OK: true}, {ID: "failed"}}},
	Notes: "Check the error states.", Handover: "Form wired; validation still missing.", HandoverFrom: "Design",
})

// AdeManualMessage is the first message of a manual step session.
var AdeManualMessage = define(Entry{
	ID: "ade.manual.message", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeManualMessage.title",
	PurposeKey: "llmPrompts.entry.adeManualMessage.description",
	UsedByKey:  "llmPrompts.entry.adeManualMessage.usedBy.body",
	Default: `{{.StepName}}: {{.Title}}{{if .JiraKey}}
{{template "ade.jiraLine" .}}{{end}}{{range .Repos}}
{{template "ade.repoLine" .}}{{end}}{{if .Outside}}
- Outside the repos. Folder: {{.Folder}}.{{else if not .Repos}}
- No repo yet. Folder: {{.Folder}}. Pick repos and branches with the Kira Space tools.{{end}}{{if .Notes}}
- Notes: {{.Notes}}{{end}}{{if .StepPrompt}}
{{.StepPrompt}}{{end}}{{if .Handover}}
{{template "ade.handoverBlock" .}}{{end}}
{{template "headless.space.suffix"}}{{if .Files.Folder}}
{{template "headless.space.files" .Files}}{{end}}{{if not .Files.Draft}}
{{template "ade.handoverLine"}}{{end}}`,
}, ManualVars{
	StepName: "Review", Title: "Add login form", JiraKey: "KIRA-42", JiraURL: "https://jira.example.com/browse/KIRA-42",
	Notes: "Check the error states.", StepPrompt: "Check kira-space on feat/login.",
	Handover: "Form wired; validation still missing.", HandoverFrom: "Implement",
	Repos: []RepoLineVars{
		{Repo: "kira-space", Branch: "feat/login", Worktree: "/work/kira-space/feat-login", Base: "main"},
		{Repo: "docs", ReadOnlyRoot: "/repos/docs"},
	},
})

// AdeManualSandboxNote tells the agent of a sandboxed manual step session what it may write.
var AdeManualSandboxNote = define(Entry{
	ID: "ade.manual.sandboxNote", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeManualSandboxNote.title",
	PurposeKey: "llmPrompts.entry.adeManualSandboxNote.description",
	UsedByKey:  "llmPrompts.entry.adeManualSandboxNote.usedBy.body",
	Default: `Sandbox{{if not .Enforced}} (declared, not enforced){{end}}: {{if eq .Access "write"}}write only in {{.Worktree}} and $TMPDIR{{else}}read only; write only in $TMPDIR{{end}}.{{range .Read}}
Read also: {{.}}{{end}}{{if not .Network}}
Network: Claude API only.{{end}}
No git push or fetch here; the user pushes from Kira Space.
"Operation not permitted" means the sandbox blocked it. Do not retry. Tell the user the path.`,
}, SandboxNoteVars{
	Access: "write", Worktree: "/work/kira-space/feat-login", Read: []string{"/work/specs"}, Network: false, Enforced: true,
})

// AdeReviewMessage is the first message of a task's review agent.
var AdeReviewMessage = define(Entry{
	ID: "ade.review.message", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeReviewMessage.title",
	PurposeKey: "llmPrompts.entry.adeReviewMessage.description",
	UsedByKey:  "llmPrompts.entry.adeReviewMessage.usedBy.body",
	Default: `Task: {{.Title}}{{if .JiraKey}}
{{template "ade.jiraLine" .}}{{end}}{{if .Notes}}
- Notes: {{.Notes}}{{end}}{{range .Repos}}
{{if .ReadOnlyRoot}}- {{.Repo}}: read only, in {{.ReadOnlyRoot}}{{else}}- {{.Repo}}: {{.Branch}} (base {{.Base}}) worktree {{.Worktree}}{{end}}{{if .ChangeRequestURL}}, PR {{.ChangeRequestURL}}{{end}}{{end}}
I review these branches in Kira Space and will ask questions about them. Answer from the code; change files only when I ask.{{if .Handover}}
{{template "ade.handoverBlock" .}}{{end}}
{{template "headless.space.suffix"}}{{if .Files.Folder}}
{{template "headless.space.files" .Files}}{{end}}{{if not .Files.Draft}}
{{template "ade.handoverLine"}}{{end}}`,
}, ReviewVars{
	Title: "Add login form", JiraKey: "KIRA-42", JiraURL: "https://jira.example.com/browse/KIRA-42",
	Notes: "Check the error states.", Handover: "Form wired; validation still missing.", HandoverFrom: "Implement",
	Repos: []RepoLineVars{
		{Repo: "kira-space", Branch: "feat/login", Worktree: "/work/kira-space/feat-login", Base: "main", ChangeRequestURL: "https://github.com/acme/kira-space/pull/42"},
		{Repo: "docs", ReadOnlyRoot: "/repos/docs"},
	},
})

var reviewRoundExample = ReviewRoundVars{
	Title: "Add login form", JiraKey: "KIRA-42", JiraURL: "https://jira.example.com/browse/KIRA-42",
	Repo: "kira-space", Branch: "feat/login", Base: "main", Worktree: "/work/kira-space/feat-login", Round: 1,
	ChangeRequestURL: "https://github.com/acme/kira-space/pull/42",
	Comments: []ReviewCommentVar{
		{ID: "3", Kind: "change", Path: "src/login.ts", Lines: "12-14", Body: "Rename this to isValid."},
		{ID: "4", Kind: "question", Path: "src/login.ts", Lines: "30", Body: "Why retry twice?"},
		{ID: "5", Kind: "change", Path: "src/login.ts", Lines: "41", Body: "Drop this log line.", Uncommitted: true},
	},
}

// AdeReviewComments lists the comments sent to a review round and says how to answer each.
var AdeReviewComments = define(Entry{
	ID: "ade.review.comments", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeReviewComments.title",
	PurposeKey: "llmPrompts.entry.adeReviewComments.description",
	UsedByKey:  "llmPrompts.entry.adeReviewComments.usedBy.body",
	Default: `{{range .Comments}}id {{.ID}} {{.Kind}} {{.Path}}:{{.Lines}}{{if .Uncommitted}} (uncommitted){{end}}
{{.Body}}
{{end}}Per comment, call state review.reply once.
change: make the change, commit it, reply what changed, resolved true.
question: answer in the reply, change no file for it, resolved true.
Cannot do it: reply why, resolved false.`,
}, reviewRoundExample)

// AdeReviewRoundMessage is the first message of a dedicated review step agent.
var AdeReviewRoundMessage = define(Entry{
	ID: "ade.review.round.message", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:          "llmPrompts.entry.adeReviewRoundMessage.title",
	PurposeKey:        "llmPrompts.entry.adeReviewRoundMessage.description",
	UsedByKey:         "llmPrompts.entry.adeReviewRoundMessage.usedBy.body",
	RequiredFragments: []ID{"ade.review.comments"},
	Default: `Review of task {{.Title}}{{if .JiraKey}}
{{template "ade.jiraLine" .}}{{end}}
Repo {{.Repo}}, branch {{.Branch}} (base {{.Base}}), worktree {{.Worktree}}.{{if .ChangeRequestURL}}
PR {{.ChangeRequestURL}}{{end}}
Read the branch diff against the base first. The reviewer left these comments:
{{template "ade.review.comments" .}}`,
}, reviewRoundExample)

// AdeReviewFollowupMessage goes to a resumed session that takes review comments.
var AdeReviewFollowupMessage = define(Entry{
	ID: "ade.review.followup.message", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:          "llmPrompts.entry.adeReviewFollowupMessage.title",
	PurposeKey:        "llmPrompts.entry.adeReviewFollowupMessage.description",
	UsedByKey:         "llmPrompts.entry.adeReviewFollowupMessage.usedBy.body",
	RequiredFragments: []ID{"ade.review.comments"},
	Default: `Review round {{.Round}} on {{.Branch}}. The reviewer left these comments on your work:
{{template "ade.review.comments" .}}`,
}, reviewRoundExample)

// AdeReviewForwardMessage carries the user's open review comments into the interactive review agent.
var AdeReviewForwardMessage = define(Entry{
	ID: "ade.review.forward.message", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeReviewForwardMessage.title",
	PurposeKey: "llmPrompts.entry.adeReviewForwardMessage.description",
	UsedByKey:  "llmPrompts.entry.adeReviewForwardMessage.usedBy.body",
	Default: `My review comments on {{.Repo}} {{.Branch}} (base {{.Base}}):
{{range .Comments}}id {{.ID}} {{.Kind}} {{.Path}}:{{.Lines}}{{if .Uncommitted}} (uncommitted){{end}}
{{.Body}}
{{end}}Answer each question. Make the change a change comment asks for.`,
}, reviewRoundExample)

// AdePanelForwardMessage carries the user's task panel edits, as diffs, into the agents running in the step.
var AdePanelForwardMessage = define(Entry{
	ID: "ade.panel.forward.message", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adePanelForwardMessage.title",
	PurposeKey: "llmPrompts.entry.adePanelForwardMessage.description",
	UsedByKey:  "llmPrompts.entry.adePanelForwardMessage.usedBy.body",
	Default: `The user changed the task panel. Only the changes follow, as a diff; nothing else changed.
{{range .Items}}--- {{.Label}}
{{.Diff}}
{{end}}Take these changes into account in your current work.`,
}, PanelForwardVars{Items: []PanelForwardItemVar{
	{Label: "note (Implement, 14:05)", Diff: "@@ -2,3 +2,3 @@\n Use the new endpoint.\n-Keep the old flag.\n+Drop the old flag.\n Add a test."},
	{Label: "task title", Diff: "- Add login\n+ Add login form"},
}})

// AdeStartMessage is the first message of a branch-level Start session.
var AdeStartMessage = define(Entry{
	ID: "ade.start.message", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeStartMessage.title",
	PurposeKey: "llmPrompts.entry.adeStartMessage.description",
	UsedByKey:  "llmPrompts.entry.adeStartMessage.usedBy.body",
	Default: `Task: {{.Title}}{{if .JiraKey}}
{{template "ade.jiraLine" .}}{{end}}
{{template "ade.repoLine" .}}{{if .StepName}}
Step: {{.StepName}}{{end}}{{range .Context}}
{{.}}{{end}}{{if .Handover}}
{{template "ade.handoverBlock" .}}{{end}}
{{template "headless.space.suffix"}}{{if .Files.Folder}}
{{template "headless.space.files" .Files}}{{end}}{{if not .Files.Draft}}
{{template "ade.handoverLine"}}{{end}}`,
}, StartVars{
	Title: "Add login form", JiraKey: "KIRA-42", JiraURL: "https://jira.example.com/browse/KIRA-42",
	Repo: "kira-space", Branch: "feat/login", Worktree: "/work/kira-space/feat-login", StepName: "Implement",
	Context: []string{}, Handover: "Form wired; validation still missing.", HandoverFrom: "Design",
})

// AdeHandoverBlock is what the previous stage left for the next one.
var AdeHandoverBlock = define(Entry{
	ID: "ade.handoverBlock", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeHandoverBlock.title",
	PurposeKey: "llmPrompts.entry.adeHandoverBlock.description",
	UsedByKey:  "llmPrompts.entry.adeHandoverBlock.usedBy.body",
	Default: `Handover from {{.HandoverFrom}}:
{{.Handover}}`,
}, HandoverBlockVars{HandoverFrom: "Design", Handover: "Form wired; validation still missing."})

// AdePassHandover is the handover an orchestrator carries into a new pass.
var AdePassHandover = define(Entry{
	ID: "ade.pass.handover", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adePassHandover.title",
	PurposeKey: "llmPrompts.entry.adePassHandover.description",
	UsedByKey:  "llmPrompts.entry.adePassHandover.usedBy.body",
	Default: `{{if .Message}}New pass. Message from the user:
{{.Message}}{{end}}{{if and .Message .Previous}}

{{end}}{{if .Previous}}Previous pass{{if .PreviousFrom}}, ended at {{.PreviousFrom}}{{end}}:
{{.Previous}}{{end}}`,
}, PassHandoverVars{Message: "Start with the API tasks.", Previous: "Spawned 3 tasks; 2 finished.", PreviousFrom: "Review"})

// AdeHandoverLine tells an interactive session to leave notes for the next stage.
var AdeHandoverLine = define(Entry{
	ID: "ade.handoverLine", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeHandoverLine.title",
	PurposeKey: "llmPrompts.entry.adeHandoverLine.description",
	UsedByKey:  "llmPrompts.entry.adeHandoverLine.usedBy.body",
	Default:    `When this stage is done, write your handover to its draft file in the task folder. Then call state with action step.finish and args status "done".`,
}, NoVars{})

// AdeRepoLine is one repository line of a launch message.
var AdeRepoLine = define(Entry{
	ID: "ade.repoLine", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeRepoLine.title",
	PurposeKey: "llmPrompts.entry.adeRepoLine.description",
	UsedByKey:  "llmPrompts.entry.adeRepoLine.usedBy.body",
	Default:    `{{if .ReadOnlyRoot}}- Repo: {{.Repo}} (read only, in {{.ReadOnlyRoot}}){{else}}- Repo: {{.Repo}} · Branch: {{.Branch}} · Worktree: {{.Worktree}}{{end}}`,
}, RepoLineVars{Repo: "kira-space", Branch: "feat/login", Worktree: "/work/kira-space/feat-login"})

// AdeJiraLine is the Jira line of a launch message.
var AdeJiraLine = define(Entry{
	ID: "ade.jiraLine", Kind: KindFragment, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeJiraLine.title",
	PurposeKey: "llmPrompts.entry.adeJiraLine.description",
	UsedByKey:  "llmPrompts.entry.adeJiraLine.usedBy.body",
	Default:    `- Jira: {{.JiraKey}} {{.JiraURL}}`,
}, JiraLineVars{JiraKey: "KIRA-42", JiraURL: "https://jira.example.com/browse/KIRA-42"})

// AdeRebaseContextLine is the line about a branch whose last rebase did not finish.
var AdeRebaseContextLine = define(Entry{
	ID: "ade.rebase.contextLine", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeRebaseContextLine.title",
	PurposeKey: "llmPrompts.entry.adeRebaseContextLine.description",
	UsedByKey:  "llmPrompts.entry.adeRebaseContextLine.usedBy.body",
	Default:    `- Last rebase of {{.Branch}} {{.Outcome}}: {{.Reason}} (run {{.RunID}}; see runs in state.yaml)`,
}, RebaseContextVars{Branch: "feat/login", Outcome: "failed", Reason: "conflicts in a.go", RunID: "run-7"})

// AdeSendBackLine tells a step's agent that a later step failed on its work.
var AdeSendBackLine = define(Entry{
	ID: "ade.sendBack.line", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.adeSendBackLine.title",
	PurposeKey: "llmPrompts.entry.adeSendBackLine.description",
	UsedByKey:  "llmPrompts.entry.adeSendBackLine.usedBy.body",
	Default:    `{{.From}} failed on {{.Branch}}: {{.Reason}}. Fix the implementation.`,
}, SendBackVars{From: "Verify", Branch: "feat/login", Reason: "tests red"})

// RunResumeLine tells a resumed Claude session that Kira quit mid-turn.
var RunResumeLine = define(Entry{
	ID: "run.resume.line", Kind: KindMessage, Apps: bothApps, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.runResumeLine.title",
	PurposeKey: "llmPrompts.entry.runResumeLine.description",
	UsedByKey:  "llmPrompts.entry.runResumeLine.usedBy.body",
	Default:    `Kira {{.App}} closed while you worked; your last turn was cut off. A tool call marked interrupted has an unknown outcome: it may have run in part. Check that action's current state (files, git, running processes) before you redo or skip it.{{if .Leases}} Leases are lost on restart: re-acquire any you held.{{end}} Then continue the task.`,
}, ResumeVars{App: "Space", Leases: true})

// RestoreContinueLine is sent at a resumed session's input when the user picked Continue after a restart.
var RestoreContinueLine = define(Entry{
	ID: "restore.continue.line", Kind: KindMessage, Apps: spaceApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.restoreContinueLine.title",
	PurposeKey: "llmPrompts.entry.restoreContinueLine.description",
	UsedByKey:  "llmPrompts.entry.restoreContinueLine.usedBy.body",
	Default:    `Kira restarted: continue where you left off.`,
}, NoVars{})

// Database MCP (Kira Space).

// DbmcpInstructions is what the database MCP server tells a client when it connects.
var DbmcpInstructions = define(Entry{
	ID: "dbmcp.instructions", Kind: KindInstructions, Apps: studioApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.dbmcpInstructions.title",
	PurposeKey: "llmPrompts.entry.dbmcpInstructions.description",
	UsedByKey:  "llmPrompts.entry.dbmcpInstructions.usedBy.body",
	Default:    "Kira Studio: databases, Kubernetes clusters, HTTP environments and scripts the user exposed. Start with `list`. A call may wait up to 10 minutes for approval. The user may hold a result for review; a withheld result is an error that says what already ran: never repeat that call. Temporary variables carry values between calls without passing through you: a later call uses {{plain.name}} or {{secret.name}} in `api_request`, or `params` in `run_query`. Results list names, never values.",
}, NoVars{})

// DbmcpInstructionsScripts follows the instructions while agent scripts are on.
var DbmcpInstructionsScripts = define(Entry{
	ID: "dbmcp.instructions.scripts", Kind: KindSuffix, Apps: studioApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.dbmcpInstructionsScripts.title",
	PurposeKey: "llmPrompts.entry.dbmcpInstructionsScripts.description",
	UsedByKey:  "llmPrompts.entry.dbmcpInstructionsScripts.usedBy.body",
	Default:    "A script writes `plain.name = value` (the user can preview it) or `secret.name = value` (never readable, never shown to you); `delete plain.name` removes one; a later script reads `plain.name`. Values are set once the result is final; it lists `tempSet` and `tempDeleted` names. Scripts: a JavaScript function body ending in `return`; sandbox with no network, files, timers or `console`; UTC; 2 s, 256 MiB; the user sees it. Names are literal; never declare your own `plain` or `secret`; at most 16 writes per run, 1 MiB each. A script error does not undo a write: never rerun it.",
}, NoVars{})

// DbmcpToolList describes the list tool.
var DbmcpToolList = define(Entry{
	ID: "dbmcp.tool.list", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "list",
	Default: "Start here. Lists what the user exposed and what each allows: connections (databases), clusters (Kubernetes), environments (HTTP variables), scripts, temp variables.",
}, NoVars{})

var _ = arg(DbmcpToolList.id, "what", studioApp, NextCall, `Sections to return. Omit for all.`, NoVars{})

// DbmcpToolDescribe describes the describe tool.
var DbmcpToolDescribe = define(Entry{
	ID: "dbmcp.tool.describe", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "describe",
	Default: "Structure of a connection. No path: top level. A container path: its children, or with columns every table's columns in one call. A table path: columns, keys, indexes, row estimate.",
}, NoVars{})

var (
	_ = arg(DbmcpToolDescribe.id, "connectionId", studioApp, NextCall, `From list.`, NoVars{})
	_ = arg(DbmcpToolDescribe.id, "path", studioApp, NextCall, `A path from an earlier describe. Omit for the top level.`, NoVars{})
	_ = arg(DbmcpToolDescribe.id, "columns", studioApp, NextCall, `On a database or schema path: every table's columns instead of its children.`, NoVars{})
	_ = arg(DbmcpToolDescribe.id, "refresh", studioApp, NextCall, `Re-read instead of the cache.`, NoVars{})
)

// DbmcpToolBrowseTable describes the browse_table tool.
var DbmcpToolBrowseTable = define(Entry{
	ID: "dbmcp.tool.browse_table", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "browse_table",
	Default: "Read one table or view without SQL: rows, a row count, or pages. Use it for every plain read.",
}, NoVars{})

var (
	_ = arg(DbmcpToolBrowseTable.id, "connectionId", studioApp, NextCall, `From list.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "path", studioApp, NextCall, `Table path from describe.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "columns", studioApp, NextCall, `Omit for all.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters", studioApp, NextCall, `All must hold. At most 20.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters.column", studioApp, NextCall, `Column name.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters.op", studioApp, NextCall, `like: % and _ are wildcards. in: values, up to 50.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters.value", studioApp, NextCall, `Not for in, isNull, notNull.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters.values", studioApp, NextCall, `For in.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "order", studioApp, NextCall, `At most 5. The key is appended so pages do not overlap.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "order.column", studioApp, NextCall, `Column name.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "order.direction", studioApp, NextCall, `asc or desc.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "limit", studioApp, NextCall, `Page size, 1-{{.Max}}, default {{.Default}}.`, LimitVars{Default: 50, Max: 100})
	_ = arg(DbmcpToolBrowseTable.id, "offset", studioApp, NextCall, `The previous nextOffset.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "count", studioApp, NextCall, `Count the matching rows instead of returning them.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "exemption", studioApp, NextCall, `Only after a limit refusal; the refusal says how.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "exemption.id", studioApp, NextCall, `From the refusal.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "exemption.reason", studioApp, NextCall, `One sentence for the user, at most 300 characters.`, NoVars{})
)

// DbmcpToolRunQuery describes the run_query tool.
var DbmcpToolRunQuery = define(Entry{
	ID: "dbmcp.tool.run_query", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "run_query",
	Default: "One statement for joins, grouped aggregates or expressions; plain reads go to browse_table. Declare the least mode that fits; a stronger statement is refused with the mode to use. Rows stop at maxRows; truncated says so. explain: true plans a SELECT without running it.",
}, NoVars{})

var (
	_ = arg(DbmcpToolRunQuery.id, "connectionId", studioApp, NextCall, `From list.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "sql", studioApp, NextCall, `One statement. Kibana: the connection's queryFormat in list.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "path", studioApp, NextCall, `Database path from describe; "" where the engine has none.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "mode", studioApp, NextCall, `read: SELECT-like. write: data changes. ddl: schema changes.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "maxRows", studioApp, NextCall, `1-{{.Max}}, default {{.Default}}.`, LimitVars{Default: 100, Max: 2000})
	_ = arg(DbmcpToolRunQuery.id, "explain", studioApp, NextCall, `Plan only, mode read.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "includeRaw", studioApp, NextCall, `With explain: the raw plan text too.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "params", studioApp, NextCall, `Temporary variable names in placeholder order, never values. Syntax: the connection's paramSyntax in list.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "script", studioApp, NextCall, `Runs on the result before you get it; it can set values for params: plain.name = value or secret.name = value. Global result {columns, rows of text cells or null, rowCount, truncated}; you get {output, input: {rowCount, truncated}}. SQL numbers are text: use Number() or BigInt().`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "exemption", studioApp, NextCall, `Only after a limit refusal; the refusal says how.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "exemption.id", studioApp, NextCall, `From the refusal.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "exemption.reason", studioApp, NextCall, `One sentence for the user, at most 300 characters.`, NoVars{})
)

// DbmcpToolOperations describes the operations tool.
var DbmcpToolOperations = define(Entry{
	ID: "dbmcp.tool.operations", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "operations",
	Default: "List running work: queries, reads, HTTP, Kubernetes and script runs. cancel stops one; a cancelled write rolls back.",
}, NoVars{})

var _ = arg(DbmcpToolOperations.id, "cancel", studioApp, NextCall, `Operation id. Omit to list.`, NoVars{})

// DbmcpToolRunScript describes the run_script tool.
var DbmcpToolRunScript = define(Entry{
	ID: "dbmcp.tool.run_script", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "run_script",
	Default: "Start a user script from list with scriptId, or read a run with runId: state, exit code, output tail. A short script waits up to waitSeconds; a long one returns at once and keeps running. Stop one with operations.",
}, NoVars{})

var (
	_ = arg(DbmcpToolRunScript.id, "scriptId", studioApp, NextCall, `From list.`, NoVars{})
	_ = arg(DbmcpToolRunScript.id, "runId", studioApp, NextCall, `From an earlier run_script.`, NoVars{})
	_ = arg(DbmcpToolRunScript.id, "params", studioApp, NextCall, `By parameter name: a string, or a list for multiselect.`, NoVars{})
	_ = arg(DbmcpToolRunScript.id, "waitSeconds", studioApp, NextCall, `0-{{.Max}}, default {{.Default}}; 0 for a long script.`, LimitVars{Default: 25, Max: 50})
	_ = arg(DbmcpToolRunScript.id, "lines", studioApp, NextCall, `Output lines, 1-{{.Max}}, default {{.Default}}.`, LimitVars{Default: 50, Max: 500})
	_ = arg(DbmcpToolRunScript.id, "contains", studioApp, NextCall, `Keep lines containing this text.`, NoVars{})
	_ = arg(DbmcpToolRunScript.id, "from", studioApp, NextCall, `tail or head, default tail.`, NoVars{})
)

// DbmcpToolAPIRequest describes the api_request tool.
var DbmcpToolAPIRequest = define(Entry{
	ID: "dbmcp.tool.api_request", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "api_request",
	Default: "Send one HTTP request. Use {{name}} (environment), {{plain.name}} or {{secret.name}} (temp) in url, headers, query, form or body; unquoted in a JSON body. A secret goes only to its allowed hosts. The response is capped and its secrets masked.",
}, NoVars{})

var (
	_ = arg(DbmcpToolAPIRequest.id, "environmentId", studioApp, NextCall, `From list. Omit for none.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "method", studioApp, NextCall, `HTTP method.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "url", studioApp, NextCall, `Full http or https URL.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "headers", studioApp, NextCall, `Request headers.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "headers.name", studioApp, NextCall, `Header name.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "headers.value", studioApp, NextCall, `Header value.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "query", studioApp, NextCall, `Percent-encoded for you.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "query.name", studioApp, NextCall, `Query name.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "query.value", studioApp, NextCall, `Query value.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "body", studioApp, NextCall, `Up to 1 MiB. Exclusive with form.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "contentType", studioApp, NextCall, `Content-Type of body.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "form", studioApp, NextCall, `Sent urlencoded. Exclusive with body.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "form.name", studioApp, NextCall, `Field name.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "form.value", studioApp, NextCall, `Field value.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "timeoutSeconds", studioApp, NextCall, `1-{{.Max}}, default {{.Default}}.`, LimitVars{Default: 30, Max: 300})
	_ = arg(DbmcpToolAPIRequest.id, "maxResponseBytes", studioApp, NextCall, `Up to {{.Max}}, default {{.Default}}.`, LimitVars{Default: 65536, Max: 524288})
	_ = arg(DbmcpToolAPIRequest.id, "followRedirects", studioApp, NextCall, `Default true.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "inputs", studioApp, NextCall, `Names of values only the user has (token, password, code), at most 4. The user types them in the approval; write each as {{secret.name}}. Never ask for them in chat.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "script", studioApp, NextCall, `Optional. At least one of prepare or transform.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "script.prepare", studioApp, NextCall, `Runs before approval. Globals request (your fields, references unresolved) and input. Returns fields that replace yours; a value it sets is what {{plain.name}} or {{secret.name}} here sends.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "script.input", studioApp, NextCall, `Any JSON for prepare.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "script.transform", studioApp, NextCall, `Runs on the response. Globals response {status, headers [{name, value}], contentType, body (masked text), json, truncated, finalUrl} and request. You get {output}. Integers above 2^53 are BigInt; kira.parseJSON keeps exact numbers.`, NoVars{})
)

// DbmcpToolKubeRead describes the kube_read tool.
var DbmcpToolKubeRead = define(Entry{
	ID: "dbmcp.tool.kube_read", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "kube_read",
	Default: "Read one Kubernetes cluster. op discover: namespaces and kind keys. list: one kind's objects as kubectl get columns. describe: one object with conditions, events and manifest; secret values become byte counts. logs: last lines of a pod container.",
}, NoVars{})

var (
	_ = arg(DbmcpToolKubeRead.id, "clusterId", studioApp, NextCall, `From list.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "op", studioApp, NextCall, `What to read.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "kind", studioApp, NextCall, `Kind key from discover, such as v1/pods.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "namespace", studioApp, NextCall, `Omit for all namespaces or a cluster-scoped kind.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "name", studioApp, NextCall, `Object name; the pod for logs.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "labelSelector", studioApp, NextCall, `list: such as app=web,tier!=db.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "maxRows", studioApp, NextCall, `list: 1-{{.Max}}, default {{.Default}}.`, LimitVars{Default: 200, Max: 2000})
	_ = arg(DbmcpToolKubeRead.id, "container", studioApp, NextCall, `logs: omit when the pod has one.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "previous", studioApp, NextCall, `logs: the previous instance.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "timestamps", studioApp, NextCall, `logs: prefix timestamps.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "tail", studioApp, NextCall, `logs: 1-{{.Max}}, default {{.Default}}.`, LimitVars{Default: 200, Max: 5000})
	_ = arg(DbmcpToolKubeRead.id, "script", studioApp, NextCall, `At least one of prepare or transform.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "script.prepare", studioApp, NextCall, `Runs before approval on global request (your args); returns args that replace yours, never clusterId or op. Reads plain.name, not secret.name.`, NoVars{})
	_ = arg(DbmcpToolKubeRead.id, "script.transform", studioApp, NextCall, `Runs on globals result (what op returns) and request; can set values for a later read: plain.name = value. You get {output, input: {rowCount, truncated}}.`, NoVars{})
)
