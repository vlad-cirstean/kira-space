// Package wtmove is the one-time boot move of task worktrees from <home>/wt/<repo>/<wt_id> to
// <home>/kira-space-wts/<repo>/<wt_id> (P563). Each worktree moves with `git worktree move`, then the
// absolute paths Kira stored outside git are rewritten and Claude's transcript folder is renamed so
// `--resume` keeps working. Every pass is idempotent; a failure logs and retries next boot. The next
// chapter deletes this package and its call in appwire.
package wtmove

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
)

// oldBase is the pre-P563 folder under the user home.
const oldBase = "wt"

const gitTimeout = 30 * time.Second

// Options configures Run. NewBase and Segment come from the ADE package so the folder layout lives in
// one place.
type Options struct {
	DB *sql.DB
	// Home is the user home.
	Home string
	// GitPath is the resolved git executable.
	GitPath string
	// NewBase is the new worktrees folder, e.g. <home>/kira-space-wts.
	NewBase string
	// Segment turns a repo name into one path segment.
	Segment func(string) string
}

type row struct {
	repoName, root, dir string
}

type stats struct {
	moved, rewritten, transcripts int
	left                          map[string]int
}

func (s *stats) leave(reason string) { s.left[reason]++ }

// Run moves every worktree row's folder and rewrites the stored paths. It never overwrites or prunes.
func Run(o Options) {
	old := filepath.Join(o.Home, oldBase)
	if !needed(o.DB, old) {
		return
	}
	rows, err := loadRows(o.DB)
	if err != nil {
		slog.Warn("worktree move: read branches", "scope", "startup", "err", err)
		return
	}
	st := &stats{left: map[string]int{}}
	byRepo := map[string][]row{}
	var roots []string
	for _, r := range rows {
		if _, ok := byRepo[r.root]; !ok {
			roots = append(roots, r.root)
		}
		byRepo[r.root] = append(byRepo[r.root], r)
	}
	sort.Strings(roots)
	segs := map[string]bool{}
	for _, root := range roots {
		for _, r := range byRepo[root] {
			segs[o.Segment(r.repoName)] = true
		}
		moveRepo(o, old, root, byRepo[root], st)
	}
	for seg := range segs {
		_ = os.Remove(filepath.Join(old, seg)) // empty only
	}
	_ = os.Remove(old)
	if st.moved > 0 || st.rewritten > 0 || st.transcripts > 0 || len(st.left) > 0 {
		slog.Info("worktree move", "scope", "startup", "moved", st.moved, "rewritten", st.rewritten, "transcripts", st.transcripts, "left", fmt.Sprint(st.left))
	}
}

// needed reports whether the old folder exists or a session still points into it.
func needed(db *sql.DB, old string) bool {
	if _, err := os.Lstat(old); err == nil {
		return true
	}
	q := fmt.Sprintf("SELECT 1 FROM %s WHERE substr(%s, 1, ?) = ? LIMIT 1", kiraschema.AdeSessions.Table, kiraschema.AdeSessions.Cwd)
	var one int
	err := db.QueryRow(q, utf8.RuneCountInString(old)+1, old+"/").Scan(&one)
	return err == nil
}

func loadRows(db *sql.DB) ([]row, error) {
	b, r := kiraschema.AdeTaskBranches, kiraschema.CodeRepos
	q := fmt.Sprintf("SELECT DISTINCT r.%s, r.%s, b.%s FROM %s b JOIN %s r ON r.%s = b.%s WHERE b.%s <> '' ORDER BY r.%s, b.%s",
		r.Name, r.Root, b.WorktreeDir, b.Table, r.Table, r.ID, b.CodeRepoID, b.WorktreeDir, r.ID, b.WorktreeDir)
	rs, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []row
	for rs.Next() {
		var x row
		if err := rs.Scan(&x.repoName, &x.root, &x.dir); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rs.Err()
}

func runGit(gitPath, root string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	res, err := gitclient.Run(ctx, gitclient.NewExecRunner(), gitPath, gitclient.Spec{Dir: root, Args: args})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", errors.New(strings.TrimSpace(string(res.Stderr)))
	}
	return string(res.Stdout), nil
}

func moveRepo(o Options, old, root string, rows []row, st *stats) {
	git := func(args ...string) (string, error) { return runGit(o.GitPath, root, args...) }
	out, err := git(porcelain.WorktreeListArgs()...)
	if err != nil {
		slog.Warn("worktree move: list worktrees", "scope", "startup", "repo", filepath.Base(root), "err", err)
		st.leave("list failed")
		return
	}
	recs, err := porcelain.ParseWorktreeList([]byte(out))
	if err != nil {
		slog.Warn("worktree move: parse worktrees", "scope", "startup", "repo", filepath.Base(root), "err", err)
		st.leave("list failed")
		return
	}
	byPath := map[string]porcelain.WorktreeRecord{}
	for _, r := range recs {
		byPath[canon(r.Path)] = r
	}
	for _, r := range rows {
		seg := o.Segment(r.repoName)
		oldPath := filepath.Join(old, seg, r.dir)
		newPath := filepath.Join(o.NewBase, seg, r.dir)
		oldRec, inOld := byPath[canon(oldPath)]
		_, inNew := byPath[canon(newPath)]
		switch {
		case inNew:
		case inOld:
			if !moveOne(git, oldPath, newPath, oldRec, st) {
				continue
			}
		default:
			if _, err := os.Lstat(oldPath); err == nil {
				slog.Warn("worktree move: not a worktree of the repo", "scope", "startup", "repo", filepath.Base(root), "dir", r.dir)
				st.leave("not a worktree")
			}
			continue
		}
		n, err := rewrite(o.DB, oldPath, newPath)
		if err != nil {
			slog.Warn("worktree move: rewrite stored paths", "scope", "startup", "dir", r.dir, "err", err)
			st.leave("rewrite failed")
			continue
		}
		st.rewritten += n
		moved, err := claude.MoveProjectDir(o.Home, oldPath, newPath)
		if err != nil {
			slog.Warn("worktree move: rename Claude transcripts", "scope", "startup", "dir", r.dir, "err", err)
		} else if moved {
			st.transcripts++
		}
	}
}

// moveOne moves one listed worktree and reports whether the move is done.
func moveOne(git func(...string) (string, error), oldPath, newPath string, rec porcelain.WorktreeRecord, st *stats) bool {
	name := filepath.Base(oldPath)
	if _, err := os.Lstat(oldPath); err != nil {
		slog.Warn("worktree move: folder missing; run git worktree prune or restore the folder", "scope", "startup", "dir", name)
		st.leave("missing")
		return false
	}
	if _, err := os.Lstat(newPath); err == nil {
		slog.Warn("worktree move: target exists", "scope", "startup", "dir", name)
		st.leave("target exists")
		return false
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		slog.Warn("worktree move: create target folder", "scope", "startup", "dir", name, "err", err)
		st.leave("move failed")
		return false
	}
	if rec.Locked {
		if _, err := git("worktree", "unlock", oldPath); err != nil {
			slog.Warn("worktree move: unlock", "scope", "startup", "dir", name, "err", err)
			st.leave("move failed")
			return false
		}
	}
	if _, err := git("worktree", "move", oldPath, newPath); err != nil {
		slog.Warn("worktree move: git refused", "scope", "startup", "dir", name, "err", err)
		if rec.Locked {
			relock(git, oldPath, rec)
		}
		st.leave("move failed")
		return false
	}
	st.moved++
	if rec.Locked {
		relock(git, newPath, rec)
	}
	return true
}

func relock(git func(...string) (string, error), path string, rec porcelain.WorktreeRecord) {
	args := []string{"worktree", "lock"}
	if rec.LockedReason != "" {
		args = append(args, "--reason", rec.LockedReason)
	}
	if _, err := git(append(args, path)...); err != nil {
		slog.Warn("worktree move: relock failed", "scope", "startup", "dir", filepath.Base(path), "err", err)
	}
}

// canon resolves symlinks on the longest existing prefix, so /var and /private/var compare equal.
func canon(p string) string {
	p = filepath.Clean(p)
	var rest []string
	for cur := p; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				r = filepath.Join(r, rest[i])
			}
			return r
		} else if !errors.Is(err, fs.ErrNotExist) {
			return p
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = append(rest, filepath.Base(cur))
		cur = parent
	}
}

// rewrite points every stored absolute path under oldPath at newPath, in one transaction.
func rewrite(db *sql.DB, oldPath, newPath string) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	total := 0
	t, s, sr := kiraschema.Tabs, kiraschema.AdeSessions, kiraschema.ScriptRuns
	jsonCwd := fmt.Sprintf("json_extract(%s, '$.cwd')", t.StateJSON)
	steps := []struct{ set, expr, extra string }{
		{fmt.Sprintf("UPDATE %s SET %s = ? || substr(%s, ?)", s.Table, s.Cwd, s.Cwd), s.Cwd, ""},
		{fmt.Sprintf("UPDATE %s SET %s = json_set(%s, '$.cwd', ? || substr(%s, ?))", t.Table, t.StateJSON, t.StateJSON, jsonCwd), jsonCwd, fmt.Sprintf(" AND %s = 'terminal'", t.Kind)},
		{fmt.Sprintf("UPDATE %s SET %s = ? || substr(%s, ?)", t.Table, t.Path, t.Path), t.Path, fmt.Sprintf(" AND %s = 'local-file'", t.Kind)},
		{fmt.Sprintf("UPDATE %s SET %s = ? || substr(%s, ?)", sr.Table, sr.Cwd, sr.Cwd), sr.Cwd, ""},
	}
	n := utf8.RuneCountInString(oldPath)
	for _, st := range steps {
		// substr, never LIKE: "_" in a path is a LIKE wildcard.
		q := fmt.Sprintf("%s WHERE (%s = ? OR substr(%s, 1, ?) = ?)%s", st.set, st.expr, st.expr, st.extra)
		res, err := tx.Exec(q, newPath, n+1, oldPath, n+1, oldPath+"/")
		if err != nil {
			return 0, err
		}
		c, _ := res.RowsAffected()
		total += int(c)
	}
	return total, tx.Commit()
}
