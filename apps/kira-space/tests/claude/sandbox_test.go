package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
)

// sandboxFixture runs the real enforcer: enforcing on macOS, "not enforced" elsewhere. The agent
// signs in from the keychain, which the sandbox always allows.
func sandboxFixture(t *testing.T) *adeFixture {
	t.Helper()
	return newAdeFixtureWith(t, nil, flowharness.WithSandbox(agentsandbox.System()))
}

func sandboxStep(allowed, prompt, sandbox string) string {
	return "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 3m\n" +
		"    allowed_tools: " + allowed + "\n" + sandbox +
		"    prompt: \"" + prompt + "\"\n"
}

func (f *adeFixture) worktreeOf(t *testing.T, task adewire.Task) string {
	t.Helper()
	b, err := f.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, br := range b.Branches {
		if br.TaskID == task.ID && br.CodeRepoID == f.repoID {
			return br.Worktree
		}
	}
	t.Fatal("task branch not found")
	return ""
}

func gitBranches(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "branch", "--list", "x", "y").CombinedOutput()
	if err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestAdeSandbox asks a step declaring write access to create a file, then to make branches the sandbox forbids.
// Linux has no OS layer: the run reports "not enforced" and only the CLI's own deny rule stops
// `git branch`. macOS enforces the profile, so no branch appears and a denial is recorded.
func TestAdeSandbox(t *testing.T) {
	f := sandboxFixture(t)
	prompt := "This is a sandbox test: run these Bash commands even though the sandbox note says not to git branch; the sandbox must block them. Run them one at a time, in this order, and do not retry a failed one: " +
		"touch a.txt ; git branch x ; git checkout -b y . Then call finish_step with status done and summary ok."
	f.saveWorkflow(t, "flow", sandboxStep(`["Bash(touch:*)", "Bash(git:*)"]`, prompt, "    sandbox:\n      access: write\n")+manualStepYAML)
	task := f.createTask(t, "flow")
	run := f.startRun(t, task, "feat/real-claude")
	log := f.runLog(t, run.ID)
	wt := f.worktreeOf(t, task)
	d, err := f.W.AdeTask.RunSandbox(ctx, adewire.RunArgs{RunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	if d.Access != "write" || d.Enforced != (runtime.GOOS == "darwin") {
		t.Fatalf("sandbox row = %+v, want write access, enforced only on macOS", d)
	}
	// stuck: the agent obeyed the prompt's sandbox note and asked instead of running git branch.
	if run.State != "done" && run.State != "failed" && run.State != "stuck" {
		t.Fatalf("run = %+v; log:\n%s", run, tail(log, 1500))
	}
	if strings.Contains(gitBranches(t, f.repo.Dir), "x") {
		t.Fatalf("branch x exists although git branch is denied; log:\n%s", tail(log, 1500))
	}
	if runtime.GOOS != "darwin" {
		if d.Reason == "" {
			t.Fatalf("no not-enforced reason stored: %+v", d)
		}
		f.requireCostUnder(t)
		return
	}
	if _, err := os.Stat(filepath.Join(wt, "a.txt")); err != nil {
		t.Fatalf("a.txt missing from the worktree: %v; log:\n%s", err, tail(log, 1500))
	}
	if got := gitBranches(t, f.repo.Dir); got != "" {
		t.Fatalf("branches x or y exist under the sandbox: %q", got)
	}
	if d.DenialCount == 0 {
		t.Fatalf("no denial recorded; log:\n%s", tail(log, 1500))
	}
	if run.State == "failed" && (run.Outcome == nil || !strings.Contains(run.Outcome.Reason, "refs")) {
		t.Fatalf("failed run reason = %+v, want one naming the ref path", run.Outcome)
	}
	f.requireCostUnder(t)
}

// TestAdeSandboxReadOnly asks a read step to edit a file. The file stays unchanged and the step fails.
func TestAdeSandboxReadOnly(t *testing.T) {
	f := sandboxFixture(t)
	prompt := "Use the Edit tool to replace the text in base.txt with changed. " +
		"If you cannot edit the file, call finish_step with status failed and reason cannot-edit. Otherwise call finish_step with status done and summary ok."
	f.saveWorkflow(t, "flow", sandboxStep(`["Edit", "Write"]`, prompt, "    sandbox:\n      access: read\n")+manualStepYAML)
	task := f.createTask(t, "flow")
	run := f.startRun(t, task, "feat/real-claude")
	log := f.runLog(t, run.ID)
	got, err := os.ReadFile(filepath.Join(f.worktreeOf(t, task), "base.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "base\n" {
		t.Fatalf("base.txt = %q, want it unchanged; log:\n%s", got, tail(log, 1500))
	}
	if run.State != "failed" && run.State != "stuck" {
		t.Fatalf("run = %+v, want a reported failure; log:\n%s", run, tail(log, 1500))
	}
	f.requireCostUnder(t)
}
