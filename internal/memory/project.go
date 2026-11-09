package memory

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/kirathecat/kira-studio/internal/gitremote"
)

const gitTimeout = 5 * time.Second

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ErrNotGitRepo means the directory is not inside a git repository.
var ErrNotGitRepo = errors.New("memory: not in a git repository")

// localKey is "local:" plus path with whitespace percent-escaped, so a path such as
// "/Users/me/My Projects/app" stays a valid, stable key.
func localKey(path string) string {
	var b strings.Builder
	b.WriteString("local:")
	for _, r := range path {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
			continue
		}
		for _, c := range []byte(string(r)) {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// DetectProject returns dir's project key: the normalised origin remote, else "local:" plus the
// main worktree path with whitespace percent-escaped. The error wraps ErrNotGitRepo outside a git
// repository; any other error means the key is unusable.
func DetectProject(ctx context.Context, dir string) (string, error) {
	key := ""
	if remote, err := gitOutput(ctx, dir, "remote", "get-url", "origin"); err == nil {
		key, _ = gitremote.Key(remote)
	}
	if key == "" {
		common, err := gitOutput(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil || common == "" {
			return "", fmt.Errorf("%w: %s", ErrNotGitRepo, dir)
		}
		key = localKey(filepath.Dir(filepath.Clean(common)))
	}
	if err := checkProject(key); err != nil {
		return "", err
	}
	return key, nil
}

const maxProjectLen = 300

// checkProject enforces the project key bounds: 1 to 300 characters, no whitespace.
func checkProject(key string) error {
	if n := len([]rune(key)); n < 1 || n > maxProjectLen {
		return invalid("project must be 1 to %d characters", maxProjectLen)
	}
	if strings.IndexFunc(key, unicode.IsSpace) >= 0 {
		return invalid("project must not contain whitespace")
	}
	return nil
}
