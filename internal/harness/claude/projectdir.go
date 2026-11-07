package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectDirName is the folder name Claude Code keeps a cwd's transcripts under: each non-alphanumeric
// character becomes "-". It is Claude Code's own scheme as of today; a mismatch only makes resume fall
// to the resume-lost path, never loses data.
func ProjectDirName(cwd string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, cwd)
}

// MoveProjectDir renames <home>/.claude/projects/<name(oldCwd)> to the folder for newCwd so `--resume`
// finds the transcripts after a worktree move. moved is false when there is nothing to rename. A
// present target is an error: nothing is merged or overwritten. Transcript content is never read.
func MoveProjectDir(home, oldCwd, newCwd string) (moved bool, err error) {
	projects := filepath.Join(home, ".claude", "projects")
	src := filepath.Join(projects, ProjectDirName(oldCwd))
	dst := filepath.Join(projects, ProjectDirName(newCwd))
	if src == dst {
		return false, nil
	}
	info, err := os.Lstat(src)
	if err != nil || !info.IsDir() {
		return false, nil
	}
	if _, err := os.Lstat(dst); err == nil {
		return false, fmt.Errorf("claude project folder %s exists", filepath.Base(dst))
	}
	if err := os.Rename(src, dst); err != nil {
		return false, err
	}
	return true, nil
}
