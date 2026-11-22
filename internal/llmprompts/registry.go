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
	Default:    "Query the databases, Kubernetes clusters and scripts the user exposed in Kira Studio. Start with `list_connections`: each entry's `access` names the modes `run_query` may use and which need approval, `description` says what it is for, `maskedColumns` lists masked columns. Declare the least powerful `mode` that fits. Walk structure with `list_children`, passing back a returned `path`; levels differ per engine. `describe` a database or schema path for every table's columns before writing a query. Read rows, counts and pages with `browse_table`; use `run_query` only for joins, grouped aggregates or expressions, and `explain` an expensive-looking one first. A masked cell is redacted; a trailing `#TAG` is a correlation token: equal values share a tag, rarely two distinct ones do, so a match is a hint, not proof. A prompt permission waits up to 10 minutes for the user. `operations` lists running work; its `cancel` stops one. `run_script` runs a `list_scripts` script and returns its output tail. `kube_*` tools are read only, and their reads take scripts too; start with `kube_list_clusters`. `api_request` appears only when the user enabled it; you see variable names, never values. Saved variables: a script saves a result with `plain.name = value` or `secret.name = value` (such as a token); you never see a secret. When one result feeds the next call, save it and make that call yourself with {{plain.name}} or {{secret.name}}, `params` or, in a kube read, a script's `plain.name`.",
}, NoVars{})

// DbmcpToolListConnections describes the list_connections tool.
var DbmcpToolListConnections = define(Entry{
	ID: "dbmcp.tool.list_connections", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "list_connections",
	Default: "List database connections exposed to MCP: kind, read-only flag, live status and, once connected, capabilities. `access` lists the modes run_query may use and which need approval; `access.masked` is browse_table's permission. `enforcement` says what backs mode read: session (the database refuses writes), app (Kira checks) or none. `tables` is all, or the only tables MCP may touch; others are refused. `expensiveQuery` gives rowsThreshold and check: plan, off or none. `columnReview.withheld` lists columns not yet reviewed for masking: results drop them and a query using one is refused. `reviewResults` true holds each result until the user sends it.",
}, NoVars{})

// DbmcpToolListChildren describes the list_children tool.
var DbmcpToolListChildren = define(Entry{
	ID: "dbmcp.tool.list_children", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "list_children",
	Default: "List one container's own children: databases, schemas, tables, keys, topics, buckets, whatever this connection's engine has at that level. Omit path for the top level. Tables outside the connection's allow list are hidden.",
}, NoVars{})

var (
	_ = arg(DbmcpToolListChildren.id, "connectionId", studioApp, NextCall, `Connection id from list_connections.`, NoVars{})
	_ = arg(DbmcpToolListChildren.id, "path", studioApp, NextCall, `Encoded container path, exactly as list_children returned it. Omit for the top level.`, NoVars{})
	_ = arg(DbmcpToolListChildren.id, "refresh", studioApp, NextCall, `Bypass the cache and re-read from the server. Default false.`, NoVars{})
)

// DbmcpToolDescribe describes the describe tool.
var DbmcpToolDescribe = define(Entry{
	ID: "dbmcp.tool.describe", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "describe",
	Default: "Describe a table, view or collection: columns, primary key, foreign keys, indexes, row estimate. On a database or schema path: the columns of every table and view in it, in one call. Call it before querying.",
}, NoVars{})

var (
	_ = arg(DbmcpToolDescribe.id, "connectionId", studioApp, NextCall, `Connection id from list_connections.`, NoVars{})
	_ = arg(DbmcpToolDescribe.id, "path", studioApp, NextCall, `Encoded path from list_children: a table, or a database or schema.`, NoVars{})
	_ = arg(DbmcpToolDescribe.id, "refresh", studioApp, NextCall, `Bypass the cache and re-read from the server. Default false.`, NoVars{})
)

// DbmcpToolBrowseTable describes the browse_table tool.
var DbmcpToolBrowseTable = define(Entry{
	ID: "dbmcp.tool.browse_table", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "browse_table",
	Default: "Example: {\"connectionId\":\"c1\",\"path\":\"<from list_children>\",\"filters\":[{\"column\":\"status\",\"op\":\"in\",\"values\":[\"open\",\"held\"]}],\"order\":[{\"column\":\"id\",\"direction\":\"asc\"}],\"limit\":50}. Use it for every plain read of one table or view, no SQL: rows, a row count, or pages. Options: columns; filters, all ANDed: eq, ne, lt, lte, gt, gte, like, isNull, notNull, and in with up to 50 values; order; limit up to 100 rows; offset paging: while hasMore, pass nextOffset back as offset; count: true returns the matching row count instead of rows. The table's key is appended to the order so pages do not overlap; stableOrder false says it could not be. Use run_query only for joins, grouped aggregates or expressions. Masked columns stay masked and can only be filtered with isNull or notNull, never sorted. Columns withheld until masking is reviewed are left out and listed. Needs the connection's masked access (list_connections). A call whose plan estimates at least expensiveQuery.rowsThreshold rows read, or that passes the statement timeout, is refused with an exemption id: filter or order on an indexed column, narrow the scope, or repeat the exact call with `exemption` to ask the user to lift that limit once. The user always decides.",
}, NoVars{})

var (
	_ = arg(DbmcpToolBrowseTable.id, "connectionId", studioApp, NextCall, `Connection id from list_connections.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "path", studioApp, NextCall, `Encoded table path from list_children.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "columns", studioApp, NextCall, `Exact column names to return. Omit for every column.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters", studioApp, NextCall, `Conditions, all must hold. At most 20.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters.column", studioApp, NextCall, `Exact column name.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters.op", studioApp, NextCall, `eq, ne, lt, lte, gt, gte, in, like, isNull or notNull. in matches any of values. like: % and _ are wildcards.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters.value", studioApp, NextCall, `The value for eq, ne, lt, lte, gt, gte and like. Omit for in, isNull and notNull.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "filters.values", studioApp, NextCall, `The values for in, 1 to 50: matches rows equal to any of them.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "order", studioApp, NextCall, `Sort terms in priority order. At most 5. The table's key is appended when it has one, so pages are stable.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "order.column", studioApp, NextCall, `Exact column name.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "order.direction", studioApp, NextCall, `asc or desc.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "limit", studioApp, NextCall, `Rows to return, 1-{{.Max}}. Default {{.Default}}.`, LimitVars{Default: 50, Max: 100})
	_ = arg(DbmcpToolBrowseTable.id, "offset", studioApp, NextCall, `Rows to skip, 0-100000. For the next page pass the previous nextOffset; past 100000 filter on the last order value instead. Default 0.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "count", studioApp, NextCall, `true returns count and exact for the rows the filters match, instead of rows. Takes filters only: no columns, order, limit or offset. Default false.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "exemption", studioApp, NextCall, `Only after a limit refusal: asks the user to lift that limit for this exact call. Same connectionId, path, columns, filters, order, limit, offset and count as the refused call.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "exemption.id", studioApp, NextCall, `Exemption id from the refusal. Works once; expires in 10 minutes.`, NoVars{})
	_ = arg(DbmcpToolBrowseTable.id, "exemption.reason", studioApp, NextCall, `One sentence for the user: why this read needs the limit lifted. Max 300 characters.`, NoVars{})
)

// DbmcpToolRunQuery describes the run_query tool.
var DbmcpToolRunQuery = define(Entry{
	ID: "dbmcp.tool.run_query", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "run_query",
	Default: "Not for plain row reads: use browse_table (rows, counts, in filters, pages). Run one statement per call, not a script, for joins, grouped aggregates or expressions. Declare `mode`: read runs on a separate read-only connection (enforcement session: the database refuses writes); write and ddl run on the normal one. A denied mode is refused; an approval mode waits up to 10 minutes for the user. A statement more powerful than its mode is refused with the mode to use. Reading stops at maxRows: rowCount is rows fetched, a lower bound when truncated is true. explain: true plans a SELECT or WITH without running it: node tree, estimated rows and cost, issues such as full scans; not on mongodb, redis, kafka, s3, sqs or kibana. A call over a limit (statement timeout, expensiveQuery row estimate) is refused with an exemption id: improve the statement, or repeat it with `exemption` to ask the user to lift the limit once. Kept variables hold values you never see: name them in `params`, one per placeholder, and Kira binds them as parameters, never into the text. One statement; not with explain; not on mongodb, redis, kibana. list_kept_variables shows what exists.",
}, NoVars{})

var (
	_ = arg(DbmcpToolRunQuery.id, "connectionId", studioApp, NextCall, `Connection id from list_connections.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "sql", studioApp, NextCall, `One statement. On kibana: a JSON object with keys dataView (id from list_children), query, language (kuery or lucene), timeRange ({"from","to"} ISO-8601 or date math, or null), filters ([{"field","operator","values","disabled"}]; operator is, is_not, is_one_of, is_not_one_of, exists, does_not_exist or between), sort ([{"field","order"}]), size, timeZone (IANA).`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "path", studioApp, NextCall, `Encoded database path from list_children. Required on postgres, mysql, mariadb and mongodb; pass "" on other engines.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "mode", studioApp, NextCall, `read, write or ddl: read for SELECT-like statements, write for data changes, ddl for schema changes. Must be allowed for the connection (see list_connections access).`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "maxRows", studioApp, NextCall, `Rows to return, 1-{{.Max}}. Default {{.Default}}. Reading stops at this many rows; rowCount is then a lower bound.`, LimitVars{Default: 200, Max: 2000})
	_ = arg(DbmcpToolRunQuery.id, "explain", studioApp, NextCall, `true plans the statement without running it. Mode read only. Default false.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "includeRaw", studioApp, NextCall, `With explain: also return the raw plan text. Default false.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "params", studioApp, NextCall, `Kept variable names, 1-64, never values. postgres: $1, $2 in this order; a list binds as an array (id = ANY($1)). mysql, mariadb: one ? per entry, in order; a list binds as JSON text (id IN (SELECT j.v FROM JSON_TABLE(?, '$[*]' COLUMNS (v BIGINT PATH '$')) j); text items: JSON_CONTAINS(?, JSON_QUOTE(col))). sqlite: ? in order; a list binds as JSON text (id IN (SELECT value FROM json_each(?))). clickhouse: {name:Type} with the kept name, once each (id IN {ids:Array(UInt64)}). A value is text, number, boolean or a flat list of one type.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "script", studioApp, NextCall, `Function body, up to 64 KiB, that turns the result into what you receive. Global: result. See the description.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "exemption", studioApp, NextCall, `Only after a limit refusal: user lifts it for this exact call. Same connectionId, sql, path, mode, params, script.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "exemption.id", studioApp, NextCall, `Exemption id from the refusal. Works once; expires in 10 minutes.`, NoVars{})
	_ = arg(DbmcpToolRunQuery.id, "exemption.reason", studioApp, NextCall, `One sentence for the user: why this statement needs the limit lifted. Max 300 characters.`, NoVars{})
)

// DbmcpToolOperations describes the operations tool.
var DbmcpToolOperations = define(Entry{
	ID: "dbmcp.tool.operations", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "operations",
	Default: "List running operations: queries, metadata reads, HTTP, gRPC and Kubernetes calls on exposed connections, and script runs. With cancel: stop one by id. A stopped query returns a cancelled error to its caller; a write rolls back.",
}, NoVars{})

var _ = arg(DbmcpToolOperations.id, "cancel", studioApp, NextCall, `Operation id from the list. Omit to list.`, NoVars{})

// DbmcpToolListScripts describes the list_scripts tool.
var DbmcpToolListScripts = define(Entry{
	ID: "dbmcp.tool.list_scripts", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "list_scripts",
	Default: "List scripts exposed to MCP with description, approval mode, mode (short or long) and parameters. You can't set secret parameters.",
}, NoVars{})

// DbmcpToolRunScript describes the run_script tool.
var DbmcpToolRunScript = define(Entry{
	ID: "dbmcp.tool.run_script", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "run_script",
	Default: "Run a script exposed to MCP. Returns run id, state, exit code and, once the run ends, the last 50 output lines. The user may be asked to approve. A short script waits up to waitSeconds, then keeps running; call get_script_run. A long script returns at once and keeps running: read its output with get_script_run, stop it with operations.",
}, NoVars{})

var (
	_ = arg(DbmcpToolRunScript.id, "scriptId", studioApp, NextCall, `Script id from list_scripts.`, NoVars{})
	_ = arg(DbmcpToolRunScript.id, "params", studioApp, NextCall, `Values by parameter name: a string, or a list for multiselect.`, NoVars{})
	_ = arg(DbmcpToolRunScript.id, "waitSeconds", studioApp, NextCall, `Seconds to wait for the run to end, 0-{{.Max}}. Default {{.Default}}, 0 for a long script.`, LimitVars{Default: 25, Max: 50})
)

// DbmcpToolGetScriptRun describes the get_script_run tool.
var DbmcpToolGetScriptRun = define(Entry{
	ID: "dbmcp.tool.get_script_run", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "get_script_run",
	Default: "Read a script run's state, exit code and duration, optionally waiting for it to end. Once ended, while a long script runs, or with from, lines or contains, also returns stored output, at most 500 lines. Secret values are masked.",
}, NoVars{})

var (
	_ = arg(DbmcpToolGetScriptRun.id, "runId", studioApp, NextCall, `Run id from run_script.`, NoVars{})
	_ = arg(DbmcpToolGetScriptRun.id, "waitSeconds", studioApp, NextCall, `Seconds to wait for the run to end, 0-{{.Max}}. Default {{.Default}}, 0 for a long script.`, LimitVars{Default: 25, Max: 50})
	_ = arg(DbmcpToolGetScriptRun.id, "from", studioApp, NextCall, `tail: last lines. head: first lines. Default tail.`, NoVars{})
	_ = arg(DbmcpToolGetScriptRun.id, "lines", studioApp, NextCall, `Output lines to return, 1-{{.Max}}. Default {{.Default}}.`, LimitVars{Default: 50, Max: 500})
	_ = arg(DbmcpToolGetScriptRun.id, "contains", studioApp, NextCall, `Keep lines containing this text (case-sensitive), before the line cap.`, NoVars{})
)

// DbmcpToolAPIListEnvironments describes the api_list_environments tool.
var DbmcpToolAPIListEnvironments = define(Entry{
	ID: "dbmcp.tool.api_list_environments", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "api_list_environments",
	Default: "List environments that share variables with agents: id, name, whether reads skip approval, and variable names. A secret lists its allowed hosts. Values are never shown. Requests without an environment are allowed. reviewResults true: the user holds that class's responses (read: GET, HEAD, OPTIONS without body).",
}, NoVars{})

// DbmcpToolAPIRequest describes the api_request tool.
var DbmcpToolAPIRequest = define(Entry{
	ID: "dbmcp.tool.api_request", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "api_request",
	Default: "Send one HTTP request. Reference shared variables as {{name}}, in the url, headers, query or body. A secret goes only to its allowed hosts, otherwise the call fails. The user may be asked to approve; hosts the user allowed are sent without asking. User access rules may refuse a host or method, and redirects to hosts they do not allow; the error names the allowed ones. TLS is always verified. Secret values are masked in the response, which is capped. Kept variables: Kira holds named values you never see. Use {{kept.name}} in a later url path, query, header, form field or body. In a JSON body write it unquoted; Kira inserts JSON. A list fits only a JSON body. Values expire (default 1 hour) and the user can clear them. list_kept_variables shows what exists. Inputs: for a value only the user has (token, password, one-time code), declare it in inputs as {name, label} and write {{kept.name}} where it goes. Kira asks the user in the approval and keeps each typed value for the user's retention time (default 60 min) for later requests to the same host. A missing value always waits for the user; a kept one is reused without asking. You never see it: an echo shows {{kept.name}}. Never ask the user to paste it into the chat.",
}, NoVars{})

// DbmcpToolRunQueryScripts is added to run_query's description while scripts are on.
var DbmcpToolRunQueryScripts = define(Entry{
	ID: "dbmcp.tool.run_query.scripts", Kind: KindSuffix, Apps: studioApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.dbmcpToolRunQueryScripts.title",
	PurposeKey: "llmPrompts.entry.dbmcpToolRunQueryScripts.description",
	UsedByKey:  "llmPrompts.entry.dbmcpToolRunQueryScripts.usedBy.body",
	Default:    "Optional script, JavaScript run by Kira in a sandbox: no network, files, timers or console; Date is fixed for the run and UTC; 2 s and 256 MiB. It is a function body that ends with return. The user sees it beside the statement. It runs after the statement, on the masked result in the global result (the JSON run_query returns: kind, columns [{name, dataType, typeClass}], rows of text cells or null, rowCount, truncated, truncatedCells; documents, entries or messages on other kinds). You receive only {kind: \"script\", output: <its return value>, input: {kind, rowCount, truncated}}, never the rows unless it returns them. SQL numbers are text cells: use Number(...) or BigInt(...); big decimals stay text. Result input up to 8 MiB, output up to 4 MiB. Saved variables: `plain` holds the plain values your script names. `plain.name = value` saves one the user can preview; `secret.name = value` saves a secret the user reveals only behind their fingerprint (1 MiB, 16 per run); `delete plain.name` removes one. Names are literal (`plain.name`, `plain[\"name\"]`); never declare your own `plain` or `secret`. Saved once the result is final and sent; you get saved: [{name, type, items, secret, expiresInSeconds}] and deleted: [names], never a value. Secrets cannot be read. The statement alone decides the approval class and what the connection allows; the script cannot widen it. Not with explain. A script error does not undo a write: its changes stay, do not rerun it.",
}, NoVars{})

// DbmcpToolListKept describes the list_kept_variables tool.
var DbmcpToolListKept = define(Entry{
	ID: "dbmcp.tool.list_kept_variables", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "list_kept_variables",
	Default: "List the kept variables you can use as {{kept.name}}: name, type, item count, secret flag, source (kind, tool) and seconds until expiry. Values are never shown. Entries with source user were typed by the user; use them only with the host they were given for. A secret made by a script goes only to the host that made it. One-time values are not listed.",
}, NoVars{})

var (
	_ = arg(DbmcpToolAPIRequest.id, "environmentId", studioApp, NextCall, `Environment id from api_list_environments. Omit to use no variables.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "method", studioApp, NextCall, `GET, HEAD, OPTIONS, POST, PUT, PATCH or DELETE.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "url", studioApp, NextCall, `Full http or https URL. May hold {{name}} references.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "headers", studioApp, NextCall, `Request headers.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "headers.name", studioApp, NextCall, `Header name.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "headers.value", studioApp, NextCall, `Header value. May hold {{name}} references.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "query", studioApp, NextCall, `Query entries added to the URL, percent-encoded for you.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "query.name", studioApp, NextCall, `Query name.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "query.value", studioApp, NextCall, `Query value. May hold {{name}} references.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "body", studioApp, NextCall, `Raw request body, up to 1 MiB. Exclusive with form.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "contentType", studioApp, NextCall, `Content-Type of body, such as application/json.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "form", studioApp, NextCall, `Form fields sent as application/x-www-form-urlencoded. Exclusive with body.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "form.name", studioApp, NextCall, `Field name.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "form.value", studioApp, NextCall, `Field value. May hold {{name}} references.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "timeoutSeconds", studioApp, NextCall, `Request timeout, 1-{{.Max}}. Default {{.Default}}.`, LimitVars{Default: 30, Max: 300})
	_ = arg(DbmcpToolAPIRequest.id, "maxResponseBytes", studioApp, NextCall, `Response body cap in bytes, up to {{.Max}}. Default {{.Default}}.`, LimitVars{Default: 65536, Max: 524288})
	_ = arg(DbmcpToolAPIRequest.id, "followRedirects", studioApp, NextCall, `Follow redirects. Default true. A redirect that would carry a credential to a host not allowed fails.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "inputs", studioApp, NextCall, `Values the user types in the approval, never shown to you. Each has name (starts with a letter; letters, digits, underscore; at most 40) and label (what the user is asked for, one line, at most 80 characters). Use each as {{kept.name}} in this request. At most 4.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "inputs.name", studioApp, NextCall, `Kept variable name for the value.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "inputs.label", studioApp, NextCall, `What the user is asked for, such as Personal access token.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "script", studioApp, NextCall, `Optional scripts Kira runs for this request. At least one of prepare or transform.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "script.prepare", studioApp, NextCall, `Function body, up to 64 KiB. Globals: request (your method, url, headers, query, body, contentType, form, with {{name}} references unresolved) and input. Return an object with any of method, url, headers, query, body (text, or an object or array sent as JSON), contentType, form.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "script.input", studioApp, NextCall, `Any JSON, up to 1 MiB, given to prepare as input.`, NoVars{})
	_ = arg(DbmcpToolAPIRequest.id, "script.transform", studioApp, NextCall, `Function body, up to 64 KiB. Globals: response (status, statusText, headers [{name, value}], contentType, body text with secrets masked up to 2 MiB, json: the parsed body or null, bodyBytes, truncated, binary, finalUrl, redirects) and request (method, url). Integers above 2^53 arrive as BigInt; use kira.parseJSON for exact numbers in nested JSON text. The result is capped at maxResponseBytes.`, NoVars{})
)

// DbmcpToolAPIRequestScripts is added to api_request's description while scripts are on.
var DbmcpToolAPIRequestScripts = define(Entry{
	ID: "dbmcp.tool.api_request.scripts", Kind: KindSuffix, Apps: studioApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.dbmcpToolApiRequestScripts.title",
	PurposeKey: "llmPrompts.entry.dbmcpToolApiRequestScripts.description",
	UsedByKey:  "llmPrompts.entry.dbmcpToolApiRequestScripts.usedBy.body",
	Default:    "Optional script, JavaScript run by Kira in a sandbox: no network, files, timers or console; Date is fixed for the run and UTC; 2 s and 256 MiB. Each is a function body that ends with return. The user sees both scripts. script.prepare runs before approval and returns request fields that replace yours. script.transform runs on the response; you receive only {result: <its return value>}, never the raw response unless it returns it. Saved variables: `plain` holds the plain values your script names. `plain.name = value` saves one the user can preview; `secret.name = value` saves a secret the user reveals only behind their fingerprint (1 MiB, 16 per run); `delete plain.name` removes one. Names are literal (`plain.name`, `plain[\"name\"]`); never declare your own `plain` or `secret`. A secret from this API goes only to this host. A value prepare saves is what {{plain.name}} or {{secret.name}} in this request sends. Saved once the response is final and sent; you get saved: [{name, type, items, secret, expiresInSeconds}] and deleted: [names], never a value. Secrets cannot be read. prepare cannot return a {{...}} reference not written in your script or request.",
}, NoVars{})

// DbmcpToolAPIRequestReview is always added to api_request's description.
var DbmcpToolAPIRequestReview = define(Entry{
	ID: "dbmcp.tool.api_request.reviewSuffix", Kind: KindSuffix, Apps: studioApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.dbmcpToolApiRequestReview.title",
	PurposeKey: "llmPrompts.entry.dbmcpToolApiRequestReview.description",
	UsedByKey:  "llmPrompts.entry.dbmcpToolApiRequestReview.usedBy.body",
	Default:    "The user may hold the response for review before you get it (reviewResults in api_list_environments). A withheld response is an error that says the request was sent; do not resend it.",
}, NoVars{})

// DbmcpToolKubeListClusters describes the kube_list_clusters tool.
var DbmcpToolKubeListClusters = define(Entry{
	ID: "dbmcp.tool.kube_list_clusters", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "kube_list_clusters",
	Default: "List the Kubernetes clusters (kubeconfig contexts) exposed to MCP, with each one's description and approval mode (prompt or allow). reviewResults true: the user holds list, describe and logs results until they send them.",
}, NoVars{})

// DbmcpToolKubeReview is added to the description of every kube tool that returns cluster data.
var DbmcpToolKubeReview = define(Entry{
	ID: "dbmcp.tool.kube.reviewSuffix", Kind: KindSuffix, Apps: studioApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.dbmcpToolKubeReview.title",
	PurposeKey: "llmPrompts.entry.dbmcpToolKubeReview.description",
	UsedByKey:  "llmPrompts.entry.dbmcpToolKubeReview.usedBy.body",
	Default:    "The user may hold the result for review before you get it (reviewResults in kube_list_clusters). A withheld result is an error with no data; the read ran, so do not rerun it to get the data.",
}, NoVars{})

// DbmcpToolKubeScripts is added to the description of kube_list_resources, kube_describe and kube_pod_logs while scripts are on.
var DbmcpToolKubeScripts = define(Entry{
	ID: "dbmcp.tool.kube.scripts", Kind: KindSuffix, Apps: studioApp, Applies: NextCall,
	TitleKey:   "llmPrompts.entry.dbmcpToolKubeScripts.title",
	PurposeKey: "llmPrompts.entry.dbmcpToolKubeScripts.description",
	UsedByKey:  "llmPrompts.entry.dbmcpToolKubeScripts.usedBy.body",
	Default:    "Optional script, JavaScript run by Kira in a sandbox: no network, files, timers or console; Date is fixed for the run and UTC; 2 s and 256 MiB. Each part is a function body that ends with return. The user sees both. script.prepare runs before approval on the global request (your args without clusterId) and returns args that replace yours; it cannot change clusterId or the tool. Omit args prepare sets. script.transform runs after the read on the global result (what the tool returns) and request; you receive only {kind: \"script\", output: <its return value>, input: {operation, rowCount, truncated}}, never the data unless it returns it. Result input up to 8 MiB, output up to 4 MiB. Saved variables: `plain` holds the plain values your script names. `plain.name = value` saves one the user can preview; `secret.name = value` saves a secret the user reveals only behind their fingerprint (1 MiB, 16 per run); `delete plain.name` removes one; for a later kube read, api_request or run_query. Names are literal (`plain.name`, `plain[\"name\"]`); never declare your own `plain` or `secret`. Saved once the result is sent, prepare's too; you get saved: [{name, type, items, secret, expiresInSeconds}] and deleted: [names], never a value. Secrets cannot be read here.",
}, NoVars{})

// DbmcpToolKubeDiscover describes the kube_discover tool.
var DbmcpToolKubeDiscover = define(Entry{
	ID: "dbmcp.tool.kube_discover", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "kube_discover",
	Default: "List one cluster's namespaces and resource kinds. restricted: the cluster denied the namespace list; fallback is then the context's own namespace. partial: some API groups failed discovery. Pass a kind's key to kube_list_resources and kube_describe.",
}, NoVars{})

var _ = arg(DbmcpToolKubeDiscover.id, "clusterId", studioApp, NextCall, `Cluster id from kube_list_clusters.`, NoVars{})

// DbmcpToolKubeListResources describes the kube_list_resources tool.
var DbmcpToolKubeListResources = define(Entry{
	ID: "dbmcp.tool.kube_list_resources", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "kube_list_resources",
	Default: "List one kind's objects as kubectl get columns. Namespace empty lists all namespaces. Capped at maxRows; truncated says so.",
}, NoVars{})

var (
	_ = arg(DbmcpToolKubeListResources.id, "clusterId", studioApp, NextCall, `Cluster id from kube_list_clusters.`, NoVars{})
	_ = arg(DbmcpToolKubeListResources.id, "kind", studioApp, NextCall, `Kind key from kube_discover, such as v1/pods or apps/v1/deployments.`, NoVars{})
	_ = arg(DbmcpToolKubeListResources.id, "namespace", studioApp, NextCall, `Namespace to list. Omit for all namespaces.`, NoVars{})
	_ = arg(DbmcpToolKubeListResources.id, "labelSelector", studioApp, NextCall, `Label selector, such as app=web,tier!=db. Omit for none.`, NoVars{})
	_ = arg(DbmcpToolKubeListResources.id, "maxRows", studioApp, NextCall, `Rows to return, 1-{{.Max}}. Default {{.Default}}.`, LimitVars{Default: 200, Max: 2000})
	_ = arg(DbmcpToolKubeListResources.id, "script", studioApp, NextCall, `Optional scripts Kira runs for this read. At least one of prepare or transform.`, NoVars{})
	_ = arg(DbmcpToolKubeListResources.id, "script.prepare", studioApp, NextCall, `Function body, up to 64 KiB. Global: request (kind, namespace, labelSelector, maxRows). Return an object with any of these args to replace yours.`, NoVars{})
	_ = arg(DbmcpToolKubeListResources.id, "script.transform", studioApp, NextCall, `Function body, up to 64 KiB. Globals: result (columns, rows [{name, namespace, cells}], rowCount, truncated) and request (the final args).`, NoVars{})
)

// DbmcpToolKubeDescribe describes the kube_describe tool.
var DbmcpToolKubeDescribe = define(Entry{
	ID: "dbmcp.tool.kube_describe", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "kube_describe",
	Default: "Describe one object: metadata, owners, conditions, events and YAML manifest. Secret values are replaced by their byte counts.",
}, NoVars{})

var (
	_ = arg(DbmcpToolKubeDescribe.id, "clusterId", studioApp, NextCall, `Cluster id from kube_list_clusters.`, NoVars{})
	_ = arg(DbmcpToolKubeDescribe.id, "kind", studioApp, NextCall, `Kind key from kube_discover, such as v1/pods.`, NoVars{})
	_ = arg(DbmcpToolKubeDescribe.id, "name", studioApp, NextCall, `Object name.`, NoVars{})
	_ = arg(DbmcpToolKubeDescribe.id, "namespace", studioApp, NextCall, `Object namespace. Omit for a cluster-scoped kind.`, NoVars{})
	_ = arg(DbmcpToolKubeDescribe.id, "script", studioApp, NextCall, `Optional scripts Kira runs for this read. At least one of prepare or transform.`, NoVars{})
	_ = arg(DbmcpToolKubeDescribe.id, "script.prepare", studioApp, NextCall, `Function body, up to 64 KiB. Global: request (kind, namespace, name). Return an object with any of these args to replace yours.`, NoVars{})
	_ = arg(DbmcpToolKubeDescribe.id, "script.transform", studioApp, NextCall, `Function body, up to 64 KiB. Globals: result (kind, apiVersion, name, namespace, uid, created, labels, annotations, owners, conditions, containers, events, manifest, manifestTruncated) and request (the final args).`, NoVars{})
)

// DbmcpToolKubePodLogs describes the kube_pod_logs tool.
var DbmcpToolKubePodLogs = define(Entry{
	ID: "dbmcp.tool.kube_pod_logs", Kind: KindTool, Apps: studioApp, Applies: NextCall, Name: "kube_pod_logs",
	Default: "Read the last lines of one pod container's log, up to 256 KiB. No follow.",
}, NoVars{})

var (
	_ = arg(DbmcpToolKubePodLogs.id, "clusterId", studioApp, NextCall, `Cluster id from kube_list_clusters.`, NoVars{})
	_ = arg(DbmcpToolKubePodLogs.id, "namespace", studioApp, NextCall, `Pod namespace.`, NoVars{})
	_ = arg(DbmcpToolKubePodLogs.id, "pod", studioApp, NextCall, `Pod name, from kube_list_resources on v1/pods.`, NoVars{})
	_ = arg(DbmcpToolKubePodLogs.id, "container", studioApp, NextCall, `Container name. Omit when the pod has one.`, NoVars{})
	_ = arg(DbmcpToolKubePodLogs.id, "previous", studioApp, NextCall, `Read the previous container instance's log. Default false.`, NoVars{})
	_ = arg(DbmcpToolKubePodLogs.id, "timestamps", studioApp, NextCall, `Prefix each line with its timestamp. Default false.`, NoVars{})
	_ = arg(DbmcpToolKubePodLogs.id, "tail", studioApp, NextCall, `Lines from the end, 1-{{.Max}}. Default {{.Default}}.`, LimitVars{Default: 200, Max: 5000})
	_ = arg(DbmcpToolKubePodLogs.id, "script", studioApp, NextCall, `Optional scripts Kira runs for this read. At least one of prepare or transform.`, NoVars{})
	_ = arg(DbmcpToolKubePodLogs.id, "script.prepare", studioApp, NextCall, `Function body, up to 64 KiB. Global: request (namespace, pod, container, previous, timestamps, tail). Return an object with any of these args to replace yours.`, NoVars{})
	_ = arg(DbmcpToolKubePodLogs.id, "script.transform", studioApp, NextCall, `Function body, up to 64 KiB. Globals: result (lines [{ts, text}], truncated) and request (the final args).`, NoVars{})
)
