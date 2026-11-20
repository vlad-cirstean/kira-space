package gitops

// WorktreeListArgs lives in gitclient/porcelain (the read side) — this file holds only the
// three write-argv builders: worktreeAdd's three creation modes, plus worktreeRemove. Every
// worktreeAdd variant passes an explicit commit-ish and a leading `--` before the path — bare
// DWIM (`git worktree add <path>` with no commit-ish silently creates a branch named after the
// path's basename) is never relied on anywhere in this file.

// WorktreeAddExistingBranchArgs builds `git worktree add -- <path> <branch>` — the
// "existingBranch" mode: branch is an already-existing local branch name, passed as the explicit
// commit-ish so git never has to guess one. `--` (the flag surface: `add [-f]
// [--detach] [--checkout] [--lock [--reason <s>]] [--orphan] [(-b|-B) <branch>] <path>
// [<commit-ish>]`) guards a path that happens to start with `-`.
func WorktreeAddExistingBranchArgs(path, branch string) []string {
	return []string{"worktree", "add", "--", path, branch}
}

// WorktreeAddNewBranchArgs builds `git worktree add -b <branch> -- <path> <startPoint>` — D2's
// "newBranch" mode. startPoint is always explicit (D3): never omitted to let git DWIM one from the
// path's basename.
func WorktreeAddNewBranchArgs(path, branch, startPoint string) []string {
	return []string{"worktree", "add", "-b", branch, "--", path, startPoint}
}

// WorktreeAddDetachArgs builds `git worktree add --detach -- <path> <commitish>` — D2's "detach"
// mode: no branch at all, an explicit commit-ish (a sha, a tag, or any other resolvable target).
func WorktreeAddDetachArgs(path, commitish string) []string {
	return []string{"worktree", "add", "--detach", "--", path, commitish}
}

// WorktreeRemoveArgs builds `git worktree remove [--force] <path>` (`remove [-f]
// <worktree>` is the entire flag surface — no `--` separator exists for this subcommand). force is
// the dirty-route flag, gated host-side by a freshly re-checked confirmation token
// (gitsession's own prepareWorktreeRemove) before this argv is ever built — never offered for the
// three unconditional blockers (mainWorktree/currentWorktree/openInAnotherWindow/locked), which
// have no force route at all.
func WorktreeRemoveArgs(path string, force bool) []string {
	if force {
		return []string{"worktree", "remove", "--force", path}
	}
	return []string{"worktree", "remove", path}
}
