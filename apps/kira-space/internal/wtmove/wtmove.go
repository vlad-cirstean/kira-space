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
	if !Needed(o.DB, o.Home) {
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
	for _, root := range roots {
		moveRepo(o, old, root, byRepo[root], st)
	}
	removeEmptyDirs(old)
	if st.moved > 0 || st.rewritten > 0 || st.transcripts > 0 || len(st.left) > 0 {
		slog.Info("worktree move", "scope", "startup", "moved", st.moved, "rewritten", st.rewritten, "transcripts", st.transcripts, "left", fmt.Sprint(st.left))
	}
}

// removeEmptyDirs removes empty repo folders under old, then old itself when empty. Files stay.
func removeEmptyDirs(old string) {
	if entries, err := os.ReadDir(old); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				_ = os.Remove(filepath.Join(old, e.Name()))
			}
		}
	}
	_ = os.Remove(old)
}

// Needed reports whether the old folder exists or a session still points into it. It is cheap, so a
// caller can skip git discovery when nothing is left to move.
func Needed(db *sql.DB, home string) bool {
	old := filepath.Join(home, oldBase)
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
	// byDir holds the records at <old>/<any>/<dir>, keyed by dir: the repo folder is the name at creation,
	// which a rename or re-import since may have changed.
	byDir := map[string]porcelain.WorktreeRecord{}
	canonOld := canon(old)
	for _, r := range recs {
		c := canon(r.Path)
		byPath[c] = r
		if filepath.Dir(filepath.Dir(c)) == canonOld {
			byDir[filepath.Base(c)] = r
		}
	}
	for _, r := range rows {
		seg := o.Segment(r.repoName)
		oldPath := filepath.Join(old, seg, r.dir)
		newPath := filepath.Join(o.NewBase, seg, r.dir)
		oldRec, inOld := byPath[canon(oldPath)]
		if !inOld {
			if rec, ok := byDir[r.dir]; ok {
				oldRec, inOld, oldPath = rec, true, rec.Path
			}
		}
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
		// Transcripts first: a failure keeps the stored old paths, so the next boot retries both.
		moved, err := claude.MoveProjectDir(o.Home, oldPath, newPath)
		if err != nil {
			slog.Warn("worktree move: rename Claude transcripts", "scope", "startup", "dir", r.dir, "err", err)
			st.leave("transcripts failed")
			continue
		}
		if moved {
			st.transcripts++
		}
		n, err := rewrite(o.DB, oldPath, newPath)
		if err != nil {
			slog.Warn("worktree move: rewrite stored paths", "scope", "startup", "dir", r.dir, "err", err)
			st.leave("rewrite failed")
			continue
		}
		st.rewritten += n
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
	// A rebase run's spec holds each stack worktree as an absolute path.
	stack := fmt.Sprintf("UPDATE %[1]s SET %[2]s = json_set(%[2]s, '$.stack', json((SELECT json_group_array(json(CASE WHEN json_extract(value, '$.worktree') = ? OR substr(json_extract(value, '$.worktree'), 1, ?) = ? THEN json_set(value, '$.worktree', ? || substr(json_extract(value, '$.worktree'), ?)) ELSE value END)) FROM json_each(%[2]s, '$.stack')))) WHERE %[2]s <> '' AND json_type(%[2]s, '$.stack') = 'array' AND EXISTS (SELECT 1 FROM json_each(%[2]s, '$.stack') WHERE json_extract(value, '$.worktree') = ? OR substr(json_extract(value, '$.worktree'), 1, ?) = ?)",
		kiraschema.AdeRuns.Table, kiraschema.AdeRuns.SpecJSON)
	if _, err := tx.Exec(stack, oldPath, n+1, oldPath+"/", newPath, n+1, oldPath, n+1, oldPath+"/"); err != nil {
		return 0, err
	}
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
