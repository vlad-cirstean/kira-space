package gitops

// ResetArgs builds `reset --<mode> <target>` for the three user-selectable modes (§7.7).
func ResetArgs(mode, target string) []string {
	return []string{"reset", "--" + mode, target}
}

// ResetKeepArgs builds `reset --keep <target>` and exists ONLY for an undo replay:
// --keep preserves unrelated local modifications and REFUSES (exit 128, changing nothing) rather
// than lose work, which is exactly the guarantee an undo needs and no user-selectable mode
// provides. The spec names three modes; a fourth would be a spec change, not an implementation
// choice, so this is deliberately NOT reachable through ResetArgs' own mode string.
func ResetKeepArgs(target string) []string {
	return []string{"reset", "--keep", target}
}

// UndoResetArgs builds the replay of a reset back to target. A hard reset replays as --keep: the
// user may have edited since, and --keep refuses rather than destroy those edits.
func UndoResetArgs(mode, target string) []string {
	if mode == "hard" {
		return ResetKeepArgs(target)
	}
	return ResetArgs(mode, target)
}

// WriteTreeArgs builds `write-tree`: the tree object of the current index.
func WriteTreeArgs() []string { return []string{"write-tree"} }

// UndoMixedResetArgs replays a mixed reset: HEAD back to target without touching the index, then
// the index back to the tree it held before the reset. The working tree is never touched.
func UndoMixedResetArgs(target, indexTree string) [][]string {
	return [][]string{ResetArgs("soft", target), {"read-tree", indexTree}}
}
