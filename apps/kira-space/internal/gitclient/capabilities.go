package gitclient

import (
	"context"
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// Capabilities are per-repo facts a later phase's operations branch on — none of P1's own scope
// reads these yet (§0.2: no operation exists here to gate), but they are cheap filesystem/config
// facts, not a porcelain parse, so they belong beside Identify() rather than invented per-phase.
type Capabilities struct {
	// CommitGraph reports whether git's own commit-graph file exists — P2's paged `git log` can
	// use `--no-walk` tricks aside, a present commit-graph is what makes a large repo's history
	// walk fast rather than merely correct.
	CommitGraph bool `json:"commitGraph"`
	// SparseWorktree reports core.sparseCheckout — a later phase's file-tree/diff surfaces need to
	// know the worktree does not contain every path the commit does.
	SparseWorktree bool `json:"sparseWorktree"`
	// LinkedWorktree mirrors RepoSummary.IsLinkedWorktree — carried here too so a caller that only
	// has Capabilities (not the whole RepoSummary) still knows it.
	LinkedWorktree bool `json:"linkedWorktree"`
}

// commitGraphExists checks both the single-file and the chunked/split commit-graph layouts git
// has used since it introduced the feature — either is enough to answer "present".
func commitGraphExists(commonDir string) bool {
	if _, err := fileservice.Host().Stat(filepath.Join(commonDir, "objects", "info", "commit-graph")); err == nil {
		return true
	}
	entries, err := fileservice.Host().ReadDir(filepath.Join(commonDir, "objects", "info", "commit-graphs"))
	return err == nil && len(entries) > 0
}

// ProbeCapabilities reads Capabilities for an already-identified repo. sparseCheckout is read via
// `git config`, a single boolean read with a fixed, trivial output shape — the same "not a
// porcelain parser" exemption §0.2 gives `git --version`/`rev-parse`.
func ProbeCapabilities(ctx context.Context, runner Runner, gitPath string, summary RepoSummary) (Capabilities, error) {
	sparse, err := sparseCheckoutEnabled(ctx, runner, gitPath, summary)
	if err != nil {
		return Capabilities{}, err
	}
	return Capabilities{
		CommitGraph:    commitGraphExists(summary.CommonDir),
		SparseWorktree: sparse,
		LinkedWorktree: summary.IsLinkedWorktree,
	}, nil
}

func sparseCheckoutEnabled(ctx context.Context, runner Runner, gitPath string, summary RepoSummary) (bool, error) {
	dir := summary.Root
	if dir == "" {
		dir = summary.CommonDir // a bare repo has no worktree to run in; config is still readable.
	}
	args := []string{"config", "--type=bool", "--default=false", "core.sparseCheckout"}
	res, err := Run(ctx, runner, gitPath, Spec{Dir: dir, Args: args, ReadOnly: true})
	if cerr := Classify(ctx, args, res, err); cerr != nil {
		return false, cerr
	}
	return string(trimTrailingNewline(res.Stdout)) == "true", nil
}

func trimTrailingNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
