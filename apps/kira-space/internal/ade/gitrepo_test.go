package ade

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/testx"
)

// Shared real-git fixtures: a bare origin plus a clone, run with the "mine" or "other" identity.

var skipWithoutGitQueue = testx.SkipWithoutGit

const queueMineName, queueMineEmail = "Mine Author", "mine@example.com"
const queueOtherName, queueOtherEmail = "Other Author", "other@example.com"

// runGitQueue runs a git command with the "mine" identity as both author and committer — every
// queue_test.go call not explicitly authored otherwise.
func runGitQueue(t testing.TB, dir string, args ...string) string {
	t.Helper()
	return runGitQueueAs(t, dir, queueMineName, queueMineEmail, args...)
}

func runGitQueueAs(t testing.TB, dir, name, email string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+name, "GIT_AUTHOR_EMAIL="+email,
		"GIT_COMMITTER_NAME="+name, "GIT_COMMITTER_EMAIL="+email,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

func writeQueueFile(t testing.TB, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// initQueueRepo builds a bare "origin" plus a clone checked out on "main" with one commit, the
// "mine" identity configured (the board's user.email read).
func initQueueRepo(t testing.TB) (origin, dir string) {
	t.Helper()
	origin = t.TempDir()
	runGitQueue(t, origin, "init", "-q", "--bare", "-b", "main")
	dir = t.TempDir()
	runGitQueue(t, dir, "clone", "-q", origin, ".")
	runGitQueue(t, dir, "config", "user.name", queueMineName)
	runGitQueue(t, dir, "config", "user.email", queueMineEmail)
	runGitQueue(t, dir, "checkout", "-q", "-b", "main")
	writeQueueFile(t, dir, "base.txt", "base\n")
	runGitQueue(t, dir, "add", "base.txt")
	runGitQueue(t, dir, "commit", "-q", "-m", "base")
	runGitQueue(t, dir, "push", "-q", "-u", "origin", "main")
	return origin, dir
}

// addQueueWorktree creates branch off base in a fresh linked worktree, returning its path.
func addQueueWorktree(t *testing.T, mainDir, branch, base string) string {
	t.Helper()
	wt := filepath.Join(t.TempDir(), branch)
	runGitQueue(t, mainDir, "worktree", "add", "-q", "-b", branch, wt, base)
	return wt
}
