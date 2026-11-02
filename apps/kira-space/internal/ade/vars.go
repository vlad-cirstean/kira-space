package ade

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// runVars are the prompt and command variables of one run (SPEC2 section 5.1.1).
type runVars struct {
	Task, Jira, Repo, Branch, Worktree string
}

// substitute fills {task} {jira} {repo} {branch} {worktree} in one pass, so a value that contains
// a placeholder is not expanded again. quote single-quotes each value: script commands run in a
// shell, and task titles and branch names must not reach it unquoted.
func substitute(text string, v runVars, quote bool) string {
	val := func(s string) string {
		if quote {
			return loginshell.Quote(s)
		}
		return s
	}
	return strings.NewReplacer(
		"{task}", val(v.Task), "{jira}", val(v.Jira), "{repo}", val(v.Repo),
		"{branch}", val(v.Branch), "{worktree}", val(v.Worktree),
	).Replace(text)
}

// promptInput is what composePrompt needs about the run.
type promptInput struct {
	Title, JiraKey, JiraURL string
	Vars                    runVars
	Step, Of                int // 1-based position and step count
	Def                     stepDef
	// Message, when non-empty, replaces the composed prompt (the Run dialog's edited text).
	Message string
	// Extra, when non-empty, is added after the body (the failure line of a fresh send-back run).
	Extra string
	// Notes are the task notes; Handover and HandoverFrom what the previous stage left (the stage name).
	// They stay when Message replaces the composed header.
	Notes, Handover, HandoverFrom string
	// Context lines (a rebase that did not finish) go after the body.
	Context []string
}

// stepVars maps a run to the step prompt's template variables. Message and the step prompt arrive
// with placeholders substituted and trailing newlines trimmed.
func stepVars(in promptInput) llmprompts.StepVars {
	v := llmprompts.StepVars{
		Title: in.Vars.Task, JiraKey: in.JiraKey, JiraURL: in.JiraURL,
		Repo: in.Vars.Repo, Branch: in.Vars.Branch, Worktree: in.Vars.Worktree,
		StepNumber: in.Step, StepCount: in.Of, StepName: in.Def.Name,
		StepPrompt: strings.TrimRight(substitute(in.Def.Prompt, in.Vars, false), "\n"),
		Extra:      in.Extra, Context: in.Context,
		Finish: agentmcp.FinishVars(resultSpecs(in.Def.Results)),
		Notes:  capNotes(in.Notes), Handover: capNotes(in.Handover), HandoverFrom: in.HandoverFrom,
	}
	if in.Message != "" {
		v.Message = strings.TrimRight(substitute(in.Message, in.Vars, false), "\n")
	}
	return v
}

// composePrompt builds an agent run's prompt (SPEC2 section 9 "Start step" shape) and always ends
// it with the finish_step instruction.
func composePrompt(src llmprompts.Source, in promptInput) string {
	return llmprompts.AdeStepPrompt.Text(src, stepVars(in))
}

// composeResumePrompt is the message a resumed run gets, after a send-back or a restart: the line, then
// the finish_step instruction, which follows the current step results.
func composeResumePrompt(src llmprompts.Source, line string, results []adewire.StepResult) string {
	return line + "\n\n" + agentmcp.FinishInstruction(src, resultSpecs(results)) + "\n"
}

// repoLine is one repo of a launch message.
type repoLine struct {
	Nick, Branch, Worktree string
	// Base is the branch's base ref, shown in the review message only.
	Base string
	// ReadOnlyRoot, when set, makes the line a read-only repo in that directory.
	ReadOnlyRoot string
	// BranchID is the task branch the line describes.
	BranchID string
	// ChangeRequestURL is the branch's pull or merge request link, filled for the review message only.
	ChangeRequestURL string
}

func repoLineVars(repos []repoLine) []llmprompts.RepoLineVars {
	out := make([]llmprompts.RepoLineVars, len(repos))
	for i, r := range repos {
		out[i] = llmprompts.RepoLineVars{Repo: r.Nick, Branch: r.Branch, Worktree: r.Worktree, Base: r.Base, ReadOnlyRoot: r.ReadOnlyRoot, ChangeRequestURL: r.ChangeRequestURL}
	}
	return out
}

// composeManualMessage is the first message of a manual step's interactive session (SPEC2 section 5).
// The step prompt sees every writable repo, joined with ", ".
func composeManualMessage(src llmprompts.Source, step adewire.WorkflowStep, title, jiraKey, jiraURL, notes string, repos []repoLine, folder string, in handoverText) string {
	v := llmprompts.ManualVars{
		StepName: step.Name, Title: title, JiraKey: jiraKey, JiraURL: jiraURL,
		Notes: capNotes(notes), Repos: repoLineVars(repos), Folder: folder, Handover: capNotes(in.Body), HandoverFrom: in.From,
	}
	if step.Prompt != "" {
		v.StepPrompt = substitute(step.Prompt, repoVars(title, jiraKey, repos), false)
	}
	return llmprompts.AdeManualMessage.Text(src, v)
}

// repoVars is the manual step prompt's variable set: every writable repo, joined with ", ".
func repoVars(title, jiraKey string, repos []repoLine) runVars {
	vars := runVars{Task: title, Jira: jiraKey}
	var nicks, names, trees []string
	for _, r := range repos {
		if r.ReadOnlyRoot == "" {
			nicks, names, trees = append(nicks, r.Nick), append(names, r.Branch), append(trees, r.Worktree)
		}
	}
	vars.Repo, vars.Branch, vars.Worktree = strings.Join(nicks, ", "), strings.Join(names, ", "), strings.Join(trees, ", ")
	return vars
}

const reviewNotesMax = 2000

// capNotes keeps a session's first message well under the terminal's command bound.
func capNotes(notes string) string {
	if r := []rune(notes); len(r) > reviewNotesMax {
		return string(r[:reviewNotesMax])
	}
	return notes
}

// composeReviewMessage is the first message of a task's review agent.
func composeReviewMessage(src llmprompts.Source, title, jiraKey, jiraURL, notes string, repos []repoLine, in handoverText) string {
	return llmprompts.AdeReviewMessage.Text(src, llmprompts.ReviewVars{
		Title: title, JiraKey: jiraKey, JiraURL: jiraURL, Notes: capNotes(notes), Repos: repoLineVars(repos),
		Handover: capNotes(in.Body), HandoverFrom: in.From,
	})
}

// composeStartMessage is the first message of a branch-level Start session (SPEC2 section 9); step
// is the first step not done, "" when the workflow has none.
func composeStartMessage(src llmprompts.Source, title, jiraKey, jiraURL string, repo repoLine, step string, context []string, in handoverText) string {
	return llmprompts.AdeStartMessage.Text(src, llmprompts.StartVars{
		Title: title, JiraKey: jiraKey, JiraURL: jiraURL,
		Repo: repo.Nick, Branch: repo.Branch, Worktree: repo.Worktree, ReadOnlyRoot: repo.ReadOnlyRoot,
		StepName: step, Context: context, Handover: capNotes(in.Body), HandoverFrom: in.From,
	})
}
