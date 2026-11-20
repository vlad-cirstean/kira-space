package gitops

import (
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/internal/fileservice"
)

func readTrimmed(path string) *string {
	b, _, err := fileservice.Host().Read(path, 0)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func pathExists(path string) bool {
	_, err := fileservice.Host().Stat(path)
	return err == nil
}

// ReadInProgressStateFiles reads gitDir's per-worktree state files — NEVER commonDir: every
// one of these lives per-worktree, and in a linked worktree that is <commonDir>/worktrees/<name>/,
// exactly what gitDir already is. A missing file is the zero value, never an error —
// os.ReadFile's error is swallowed by design, since "not there" is the answer 99% of the time and
// ClassifyInProgress has no use for the distinction. AUTO_MERGE is deliberately not read:
// upstream's probe found it left behind after an aborted cherry-pick, a stale artefact rather
// than a state signal.
//
// The reads run sequentially, not concurrently: stat/read calls on files almost certainly
// in the page cache, and a goroutine fan-out here would be a JavaScript Promise.all idiom for an
// I/O model Go does not have.
func ReadInProgressStateFiles(gitDir string) gitpreflight.InProgressStateFiles {
	// head-name/onto live under rebase-merge/ for a merge-backend rebase, but under
	// rebase-apply/ for an apply-backend one (or a plain `git am`, which shares the same
	// directory) — only one of the two directories is ever present at a time, so trying the
	// rebase-merge/ path first and falling back to rebase-apply/ is exactly equivalent to reading
	// whichever one exists.
	headName := readTrimmed(filepath.Join(gitDir, "rebase-merge", "head-name"))
	onto := readTrimmed(filepath.Join(gitDir, "rebase-merge", "onto"))
	if headName == nil {
		headName = readTrimmed(filepath.Join(gitDir, "rebase-apply", "head-name"))
	}
	if onto == nil {
		onto = readTrimmed(filepath.Join(gitDir, "rebase-apply", "onto"))
	}

	return gitpreflight.InProgressStateFiles{
		MergeHead:           readTrimmed(filepath.Join(gitDir, "MERGE_HEAD")),
		CherryPickHead:      readTrimmed(filepath.Join(gitDir, "CHERRY_PICK_HEAD")),
		RevertHead:          readTrimmed(filepath.Join(gitDir, "REVERT_HEAD")),
		BisectLog:           pathExists(filepath.Join(gitDir, "BISECT_LOG")),
		RebaseMergeDir:      pathExists(filepath.Join(gitDir, "rebase-merge")),
		RebaseApplyDir:      pathExists(filepath.Join(gitDir, "rebase-apply")),
		RebaseApplyApplying: pathExists(filepath.Join(gitDir, "rebase-apply", "applying")),
		RebaseHeadName:      headName,
		RebaseOnto:          onto,
		SequencerDir:        pathExists(filepath.Join(gitDir, "sequencer")),
		SequencerTodoKind:   readSequencerTodoKind(gitDir),
	}
}

// readSequencerTodoKind is the read: sequencer/todo's first non-comment, non-blank line names
// the pending command for a multi-commit revert or cherry-pick — "revert"/"pick" (git never
// abbreviates either token in this file, unlike interactive rebase's own todo). Mirrors git's own
// wt-status.c, which reads exactly this to label a sequencer state once every *_HEAD file is
// already gone. "" for a missing/empty file, or a first command this isn't (there is no other
// sequencer op — an interactive rebase's own todo lives in rebase-merge/, checked well before this
// is ever reached).
func readSequencerTodoKind(gitDir string) gitpreflight.InProgressKind {
	b, _, err := fileservice.Host().Read(filepath.Join(gitDir, "sequencer", "todo"), 0)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		verb, _, _ := strings.Cut(line, " ")
		switch verb {
		case "revert":
			return gitpreflight.InProgressRevert
		case "pick":
			return gitpreflight.InProgressCherryPick
		default:
			return ""
		}
	}
	return ""
}

// sequencerVerb maps an InProgressKind to its git subcommand token — the value ContinueArgs/
// AbortArgs/SkipArgs each switched on before appending their own action suffix (I2-46).
var sequencerVerb = map[gitpreflight.InProgressKind]string{
	gitpreflight.InProgressMerge:      "merge",
	gitpreflight.InProgressCherryPick: "cherry-pick",
	gitpreflight.InProgressRevert:     "revert",
	gitpreflight.InProgressRebase:     "rebase",
}

// sequencerArgs is ContinueArgs/AbortArgs/SkipArgs's shared shape: {verb, action} for a kind in
// allowed, else (nil, false) — matching InProgressOperation's own Can{Continue,Abort,Skip} gate,
// which the caller (gitsession.RunOp) treats false as "refuse before spawning".
func sequencerArgs(kind gitpreflight.InProgressKind, action string, allowed ...gitpreflight.InProgressKind) ([]string, bool) {
	for _, k := range allowed {
		if k == kind {
			return []string{sequencerVerb[kind], action}, true
		}
	}
	return nil, false
}

// ContinueArgs is the Ordering table's own --continue column: (nil, false) for the two kinds v1
// never offers it for — bisect (nothing to continue) and unmergedOnly (no state file to advance).
// Rebase gained Continue at G26 D12, retiring G5's "§9 report-only posture": that posture was
// correct only while nothing in the app could START a rebase, and G26's restack executor does.
// GIT_EDITOR=true is already in hygieneEnv (runner.go) and probe M5/P11 both confirm
// `rebase --continue` never opens an editor anyway (no -i in play here).
func ContinueArgs(kind gitpreflight.InProgressKind) ([]string, bool) {
	return sequencerArgs(kind, "--continue",
		gitpreflight.InProgressMerge, gitpreflight.InProgressCherryPick,
		gitpreflight.InProgressRevert, gitpreflight.InProgressRebase)
}

// AbortArgs is the Ordering table's own --abort column: every kind except unmergedOnly (there is
// no state file for git to abort — InProgressOperation.CanAbort is false for exactly that kind).
// bisect's "abort" is `git bisect reset` (bisect has no --abort flag), handled here directly
// rather than through sequencerVerb/sequencerArgs.
func AbortArgs(kind gitpreflight.InProgressKind) ([]string, bool) {
	if kind == gitpreflight.InProgressBisect {
		return []string{"bisect", "reset"}, true
	}
	return sequencerArgs(kind, "--abort",
		gitpreflight.InProgressMerge, gitpreflight.InProgressCherryPick,
		gitpreflight.InProgressRevert, gitpreflight.InProgressRebase)
}

// SkipArgs is the sequencer remedy for an empty pick or revert, joined by rebase — the hint line
// names it verbatim ("You can instead skip this commit: run "git rebase --skip""), and an
// already-applied commit inside a stack (a branch reordered under it, say) is exactly the case
// the automatic same-patch drop does not cover. (nil, false) for every other kind, since
// InProgressOperation.CanSkip is the UI's own gate and this is only the second line of defence,
// not the first.
func SkipArgs(kind gitpreflight.InProgressKind) ([]string, bool) {
	return sequencerArgs(kind, "--skip",
		gitpreflight.InProgressCherryPick, gitpreflight.InProgressRevert, gitpreflight.InProgressRebase)
}
