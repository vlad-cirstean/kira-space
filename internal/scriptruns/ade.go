package scriptruns

import (
	"context"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// ADE is Kira Space's task board as script runs see it. Studio leaves Service.ADE nil.
type ADE interface {
	// Tasks lists the live tasks in board order.
	Tasks() ([]TaskChoice, error)
	// Context resolves a task's variables and branch choices; a non-empty branchID picks that branch.
	Context(taskID, branchID string) (ADEContext, error)
	// ClaimWorktree creates a missing worktree of the branch and takes the board's claim on it, so
	// the pipeline waits while the smart run works there. release drops the claim.
	ClaimWorktree(ctx context.Context, branchID, script string) (release func(), err error)
	// Busy says why a smart run cannot claim the branch now, "" when it can.
	Busy(branchID string) string
	// Tools are the Kira Space tools a task-bound smart run may call.
	Tools() agentmcp.SpaceTools
	// SandboxGit serves state remote.fetch and branch.push to a sandboxed smart run.
	SandboxGit() agentmcp.SandboxGit
	// SandboxTarget points those tools at branchID's worktree for one run; release drops it.
	SandboxTarget(runID, branchID, worktree string) (release func(), err error)
}

// TaskChoice is a live task the run dialog can pick.
type TaskChoice struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// BranchChoice is a branch of the chosen task; a disabled one says why.
type BranchChoice struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled"`
	Why      string `json:"why"`
}

// ADEContext is what the board knows about a task, and about one of its branches when chosen.
type ADEContext struct {
	TaskID, TaskTitle string
	// Vars holds task and jira; with a branch also repo, branch, base and worktree.
	Vars     map[string]string
	Branches []BranchChoice
	// Branch is the chosen branch: the given one, or the only enabled one. Nil when none.
	Branch *BranchChoice
	// Worktree is the existing path, else the predicted one; Pending says it is created on Run.
	Worktree string
	Pending  bool
	// Preparing is the setup gate text while the worktree is not ready, "" when ready.
	Preparing string
}

// Needs lists what the run dialog must ask the user to pick.
type Needs struct {
	Tasks    []TaskChoice   `json:"tasks"`
	Branches []BranchChoice `json:"branches"`
}

// RunADE is the task and branch a run resolved.
type RunADE struct {
	TaskID      string `json:"taskId"`
	TaskTitle   string `json:"taskTitle"`
	BranchID    string `json:"branchId"`
	BranchLabel string `json:"branchLabel"`
}

// branchVars are the built-in variables that need a branch.
var branchVars = map[string]bool{"repo": true, "branch": true, "base": true, "worktree": true}

// planADE resolves the ADE variables, the folder and the pickers of a run. text is the command or
// prompt body the run uses. Without ADE (Studio) a task is refused and the variables stay literal.
func (s *Service) planADE(args RunArgs, p *planned, pv *Preview, text string, rendered map[string]string) error {
	if s.ADE == nil {
		if args.TaskID != "" {
			return ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.tasksSpaceOnly.body", nil)
		}
		return nil
	}
	used := scripts.VarsUsed(text, nil)
	if args.TaskID == "" {
		return s.askTask(pv, used, args.ListTasks)
	}
	c, err := s.ADE.Context(args.TaskID, args.BranchID)
	if err != nil {
		return ipcerr.BadRequest(err.Error())
	}
	if args.BranchID != "" && c.Branch == nil {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.branchCannotRun.body", nil)
	}
	askBranch(p, pv, c, used)
	applyVars(p, pv, c, rendered)
	if c.Branch == nil {
		return nil
	}
	pv.ADE.BranchID, pv.ADE.BranchLabel = c.Branch.ID, c.Branch.Label
	if p.script.UseAdeDir {
		s.useWorktree(p, pv, c)
	}
	return nil
}

// askTask asks for a task when the text uses an ADE variable; list only fills the choices.
func (s *Service) askTask(pv *Preview, used []string, list bool) error {
	if len(used) == 0 && !list {
		return nil
	}
	tasks, err := s.ADE.Tasks()
	if err != nil {
		return ipcerr.InternalErr(err)
	}
	if tasks != nil {
		pv.Needs.Tasks = tasks
	}
	if len(used) > 0 {
		pv.Missing = append(pv.Missing, "task")
	}
	return nil
}

// askBranch asks for a branch when a branch variable is used or the worktree folder applies.
func askBranch(p *planned, pv *Preview, c ADEContext, used []string) {
	if c.Branch != nil {
		return
	}
	enabled := 0
	for _, b := range c.Branches {
		if !b.Disabled {
			enabled++
		}
	}
	needs := p.script.UseAdeDir && enabled > 0
	for _, v := range used {
		needs = needs || branchVars[v]
	}
	switch {
	case !needs:
	case enabled == 0:
		setBlocker(pv, i18n.T("backend.core.scriptRun.taskNoBranch.body", nil))
	default:
		pv.Needs.Branches = c.Branches
		pv.Missing = append(pv.Missing, "branch")
	}
}

// applyVars adds the built-in variables to the prompt values and the environment, ahead of the params.
func applyVars(p *planned, pv *Preview, c ADEContext, rendered map[string]string) {
	builtin, builtinEnv := []EnvVar{}, []string{}
	for _, name := range scripts.BuiltinVars {
		v, ok := c.Vars[name]
		if !ok {
			continue
		}
		rendered[name] = v
		builtin = append(builtin, EnvVar{Name: scripts.BuiltinEnv(name), Value: v, FromVar: name})
		builtinEnv = append(builtinEnv, scripts.BuiltinEnv(name)+"="+v)
	}
	pv.Env, p.envList = append(builtin, pv.Env...), append(builtinEnv, p.envList...)
	pv.ADE = &RunADE{TaskID: c.TaskID, TaskTitle: c.TaskTitle}
}

// useWorktree runs the script in the chosen branch's worktree, which a smart or headless run also needs free.
func (s *Service) useWorktree(p *planned, pv *Preview, c ADEContext) {
	if pv.Blocker == p.dir.Blocker {
		pv.Blocker = ""
	}
	p.dir = scripts.Dir{Mode: scripts.DirModeWorktree, Path: c.Worktree, Branch: c.Branch.Label, Pending: c.Pending}
	pv.Dir = p.dir
	if c.Preparing != "" {
		setBlocker(pv, c.Preparing)
	}
	if (p.script.Kind == scripts.KindSmart || p.opts.headless) && !p.opts.claimed {
		if why := s.ADE.Busy(c.Branch.ID); why != "" {
			setBlocker(pv, why)
		}
	}
}

func setBlocker(pv *Preview, why string) {
	if pv.Blocker == "" {
		pv.Blocker = why
	}
}
