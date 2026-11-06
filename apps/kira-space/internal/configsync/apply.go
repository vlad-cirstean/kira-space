package configsync

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/fsmove"
)

// Preview is what a pull would do.
type Preview struct {
	Token  string     `json:"token"`
	Commit string     `json:"commit"`
	Rows   []Row      `json:"rows"`
	Files  []FileDiff `json:"files"`
}

// ApplyResult counts what a pull did.
type ApplyResult struct {
	// Applied is the number of files written or removed.
	Applied int `json:"applied"`
	// Kept is the number of conflicted units left as they were locally.
	Kept int `json:"kept"`
	// Blocked is the number of units the gate refused.
	Blocked int `json:"blocked"`
	// Backup is the folder holding the replaced files, "" when none was replaced.
	Backup string `json:"backup"`
}

// Preview computes the plan for the fetched commit and opens a ten-minute, single-use session.
func (s *Service) Preview(ctx context.Context) (Preview, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	_ = s.load()
	set, fetched := s.set, s.st.Fetched
	s.mu.Unlock()
	if set.Repo == nil {
		return Preview{}, errf(ErrNotFound, "backend.space.configSync.notConfigured.body")
	}
	if fetched == "" {
		return Preview{}, errf(ErrNotFound, "backend.space.configSync.nothingFetched.body")
	}
	tree, se := s.mirror.readTree(ctx, fetched)
	if se != nil {
		return Preview{}, &Error{Kind: ErrInternal, Key: "backend.space.configSync.internal.body", Cause: se}
	}
	p, err := s.buildPlan(tree, set)
	if err != nil {
		return Preview{}, internal(err)
	}
	token, err := randomToken()
	if err != nil {
		return Preview{}, internal(err)
	}
	s.mu.Lock()
	s.sess = &session{token: token, expires: s.now().Add(previewTTL), plan: p}
	s.mu.Unlock()
	rows := p.rows()
	if rows == nil {
		rows = []Row{}
	}
	files := s.diffs(p)
	if files == nil {
		files = []FileDiff{}
	}
	return Preview{Token: token, Commit: p.commit, Rows: rows, Files: files}, nil
}

type journalEntry struct {
	Module  string `json:"module"`
	File    string `json:"file"`
	Existed bool   `json:"existed"`
}

// journal is .sync/apply.json, present only while an apply runs.
type journal struct {
	State     string         `json:"state"`
	Backup    string         `json:"backup"`
	Commit    string         `json:"commit"`
	AppliedAt string         `json:"appliedAt"`
	Files     []journalEntry `json:"files"`
}

const (
	journalApplying  = "applying"
	journalCommitted = "committed"
)

type pending struct {
	fp  *filePlan
	res fileResult
}

// Apply writes the previewed plan. choices maps a conflicted row id to mine or shared (default
// mine). Any change to a target, or a newer fetch, since the preview fails with a conflict and
// writes nothing. A failed write rolls every file back.
func (s *Service) Apply(ctx context.Context, token string, choices map[string]string) (ApplyResult, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	p, err := s.takeSession(token)
	if err != nil {
		return ApplyResult{}, err
	}
	work, skipBase, out := s.collect(p, choices)
	now := s.now()
	jr := journal{State: journalApplying, Commit: p.commit, AppliedAt: now.UTC().Format(time.RFC3339)}
	for _, w := range work {
		jr.Files = append(jr.Files, journalEntry{Module: w.fp.module, File: w.fp.file, Existed: w.fp.oursOK})
		if w.fp.oursOK {
			jr.Backup = filepath.Join(s.o.Home, "backups", "configsync-"+now.UTC().Format("20060102T150405Z"))
		}
	}
	if err := s.commit(p, jr, work, skipBase); err != nil {
		return ApplyResult{}, internal(err)
	}
	s.pruneBackups()
	out.Applied, out.Backup = len(work), jr.Backup
	s.mu.Lock()
	s.st = loadState(s.o.Home)
	s.mu.Unlock()
	s.refreshLocked(ctx)
	s.emit()
	return out, nil
}

// takeSession spends the preview session and checks nothing moved since it was made.
func (s *Service) takeSession(token string) (*plan, error) {
	s.mu.Lock()
	sess := s.sess
	s.sess = nil
	fetched := s.st.Fetched
	s.mu.Unlock()
	if sess == nil || sess.token != token || s.now().After(sess.expires) {
		return nil, errf(ErrNotFound, "backend.space.configSync.previewExpired.body")
	}
	if sess.plan.commit != fetched {
		return nil, errf(ErrConflict, "backend.space.configSync.changed.body")
	}
	for _, fp := range sess.plan.files {
		cur, _, err := readLocal(s.localPath(fp.module, fp.file))
		if err != nil {
			return nil, internal(err)
		}
		if configfs.TextHash(cur) != fp.oursHash {
			return nil, errf(ErrConflict, "backend.space.configSync.changed.body")
		}
	}
	return sess.plan, nil
}

// collect decides every file under choices: the writes, the files whose merge base must not move,
// and the counts.
func (s *Service) collect(p *plan, choices map[string]string) (work []pending, skipBase map[*filePlan]bool, out ApplyResult) {
	skipBase = map[*filePlan]bool{}
	now := s.now()
	for _, fp := range p.files {
		res := s.resultFor(fp, choices, now)
		if res.blocked != "" {
			out.Blocked++
			skipBase[fp] = true
			continue
		}
		for _, r := range fp.rows {
			switch {
			case r.Kind == KindBlocked:
				out.Blocked++
			case r.verdict == verdictConflict && choices[r.ID] != ChoiceShared:
				out.Kept++
			}
		}
		if res.changed {
			work = append(work, pending{fp: fp, res: res})
		}
	}
	return work, skipBase, out
}

// commit runs the journaled write: backup, journal, staged base, files, journal committed, base
// swap. A failure before the committed journal rolls every file back.
func (s *Service) commit(p *plan, jr journal, work []pending, skipBase map[*filePlan]bool) error {
	if err := s.backup(jr, work); err != nil {
		return err
	}
	if err := s.writeJournal(jr); err != nil {
		_ = os.RemoveAll(jr.Backup)
		return err
	}
	if err := s.stageBase(p, skipBase); err != nil {
		s.rollback(jr)
		return err
	}
	for _, w := range work {
		if err := s.writeOne(w); err != nil {
			s.rollback(jr)
			return err
		}
	}
	jr.State = journalCommitted
	if err := s.writeJournal(jr); err != nil {
		s.rollback(jr)
		return err
	}
	return s.finish(jr)
}

func (s *Service) writeOne(w pending) error {
	path := s.localPath(w.fp.module, w.fp.file)
	if !w.res.remove {
		return s.o.Write(path, w.res.data)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Service) journalPath() string { return filepath.Join(syncDir(s.o.Home), journalFile) }

func (s *Service) writeJournal(jr journal) error {
	b, err := json.MarshalIndent(jr, "", "  ")
	if err != nil {
		return err
	}
	return configfs.WriteAtomic(s.journalPath(), b, 0o644)
}

// backup copies every file the apply replaces or removes.
func (s *Service) backup(jr journal, work []pending) error {
	for _, w := range work {
		if !w.fp.oursOK {
			continue
		}
		dst := filepath.Join(jr.Backup, w.fp.module, w.fp.file)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := fsmove.CopyVerified(s.localPath(w.fp.module, w.fp.file), dst); err != nil {
			return err
		}
	}
	return nil
}

// stageBase builds base.next: the current base with every non-blocked unit set to its shared version.
func (s *Service) stageBase(p *plan, skip map[*filePlan]bool) error {
	root := syncDir(s.o.Home)
	next := filepath.Join(root, baseNextDir)
	if err := os.RemoveAll(next); err != nil {
		return err
	}
	if fsmove.Exists(filepath.Join(root, baseDir)) {
		if err := fsmove.CopyTree(filepath.Join(root, baseDir), next); err != nil {
			return err
		}
	} else if err := os.MkdirAll(next, 0o755); err != nil {
		return err
	}
	for _, fp := range p.files {
		if skip[fp] {
			continue
		}
		path := filepath.Join(next, fp.module, baseName(fp.file, fp.entry))
		if !fp.newBaseExist {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			continue
		}
		if err := configfs.WriteAtomic(path, fp.newBaseFile, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// finish swaps base.next in and records the applied commit, then drops the journal. It is safe to repeat.
func (s *Service) finish(jr journal) error {
	root := syncDir(s.o.Home)
	next, cur, prev := filepath.Join(root, baseNextDir), filepath.Join(root, baseDir), filepath.Join(root, baseDir+".prev")
	if fsmove.Exists(next) {
		if err := os.RemoveAll(prev); err != nil {
			return err
		}
		if fsmove.Exists(cur) {
			if err := os.Rename(cur, prev); err != nil {
				return err
			}
		}
		if err := os.Rename(next, cur); err != nil {
			return err
		}
		_ = os.RemoveAll(prev)
	}
	return finishState(s.o.Home, jr)
}

func finishState(home string, jr journal) error {
	st := loadState(home)
	st.Applied, st.AppliedAt = jr.Commit, jr.AppliedAt
	if err := saveState(home, st); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(syncDir(home), journalFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// rollback restores every journaled file from the backup and drops the staged base and the journal.
func (s *Service) rollback(jr journal) {
	if err := rollbackFiles(s.o.Home, jr); err != nil {
		slog.Error("config sync: rollback failed", "scope", "configsync", "err", err)
	}
}

func rollbackFiles(home string, jr journal) error {
	var first error
	for _, f := range jr.Files {
		path := filepath.Join(configfs.ModuleDir(home, f.Module), f.File)
		if f.Existed {
			data, err := os.ReadFile(filepath.Join(jr.Backup, f.Module, f.File))
			if err == nil {
				err = configfs.WriteAtomic(path, data, 0o644)
			}
			if err != nil && first == nil {
				first = err
			}
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	root := syncDir(home)
	if err := os.RemoveAll(filepath.Join(root, baseNextDir)); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(root, journalFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Recover settles an apply a crash cut short: an applying journal rolls back, a committed one
// finishes. Run at boot before the module stores start.
func Recover(home string) error {
	b, err := os.ReadFile(filepath.Join(syncDir(home), journalFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var jr journal
	if err := json.Unmarshal(b, &jr); err != nil {
		return err
	}
	if jr.State == journalApplying {
		return rollbackFiles(home, jr)
	}
	root := syncDir(home)
	next, cur := filepath.Join(root, baseNextDir), filepath.Join(root, baseDir)
	if fsmove.Exists(next) {
		if err := os.RemoveAll(cur); err != nil {
			return err
		}
		if err := os.Rename(next, cur); err != nil {
			return err
		}
	}
	return finishState(home, jr)
}

// pruneBackups keeps the newest keepBackups configsync backups.
func (s *Service) pruneBackups() {
	dir := filepath.Join(s.o.Home, "backups")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() && strings.HasPrefix(e.Name(), "configsync-") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keepBackups {
		if err := os.RemoveAll(filepath.Join(dir, names[0])); err != nil {
			slog.Warn("config sync: prune backup", "scope", "configsync", "err", err)
		}
		names = names[1:]
	}
}
