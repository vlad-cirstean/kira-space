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
// present target folder gets the source's entries merged in; an entry the target already has stays in
// the source, never overwritten. Transcript content is never read.
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
	dinfo, err := os.Lstat(dst)
	if err != nil {
		if err := os.Rename(src, dst); err != nil {
			return false, err
		}
		return true, nil
	}
	if !dinfo.IsDir() {
		return false, fmt.Errorf("claude project path %s is not a folder", filepath.Base(dst))
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		to := filepath.Join(dst, e.Name())
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		if err := os.Rename(filepath.Join(src, e.Name()), to); err != nil {
			return moved, err
		}
		moved = true
	}
	_ = os.Remove(src) // empty only
	return moved, nil
}
