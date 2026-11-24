package taskfiles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// Status is the per-file answer of a submit.
type Status string

// Statuses of FileResult.
const (
	StatusAccepted  Status = "accepted"
	StatusPending   Status = "pending"
	StatusRejected  Status = "rejected"
	StatusInvalid   Status = "invalid"
	StatusRestored  Status = "restored"
	StatusUnchanged Status = "unchanged"
	StatusIgnored   Status = "ignored"
)

// FileResult is the answer for one path.
type FileResult struct {
	Path   string
	Status Status
	// Message says why for every status but accepted and unchanged; for invalid it ends in what to change.
	Message string
	// Line is the 1-based line of an invalid file's problem, 0 when unknown.
	Line   int
	Notify Notify
	// Proof is set once per proof unit, on its proof.md entry.
	Proof *ProofResult
}

// Result is the answer of one submit, files in path order.
type Result struct {
	Files []FileResult
}

// Counts is the number of files per status the reply summary line uses.
type Counts struct {
	Accepted, Pending, Invalid int
}

// Counts tallies the files that count in the summary: ignored, unchanged and restored do not.
func (r Result) Counts() Counts {
	var c Counts
	for _, f := range r.Files {
		switch f.Status {
		case StatusAccepted:
			c.Accepted++
		case StatusPending:
			c.Pending++
		case StatusInvalid, StatusRejected:
			c.Invalid++
		}
	}
	return c
}

// Touched reports whether any file changed state: anything but unchanged and ignored.
func (r Result) Touched() bool {
	for _, f := range r.Files {
		if f.Status != StatusUnchanged && f.Status != StatusIgnored {
			return true
		}
	}
	return false
}

// Session is the writer of one call.
type Session struct {
	// Key identifies the session; it is recorded as the accepting writer of each file.
	Key  string
	Role Role
	// LastSync is the session's last launch or accepted submit. Zero means unknown: collisions fall
	// back to plain classification.
	LastSync time.Time
	// Step is the task's current step id, which the draft file must carry.
	Step string
}

// Request is one submit.
type Request struct {
	TaskID string
	// Paths limits the call to these paths; empty scans the whole task folder.
	Paths   []string
	Session Session
}

// Config wires a Service.
type Config struct {
	Index Index
	// Open returns the store of a task's folder.
	Open     func(taskID string) (store.Store, error)
	Handlers map[Kind]Handler
	// Proof wires the proof unit; nil leaves proof paths unhandled.
	Proof  *ProofConfig
	Events Events
	Now    func() time.Time
}

// Service is the submit core. Callers serialize calls per task.
type Service struct {
	cfg Config
}

// New returns the service of cfg.
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg}
}

const maxIgnoredReported = 10

// Sum is the content hash the index stores in TaskFile.SHA256.
func Sum(b []byte) string { return sum(b) }

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Service) nowMs() int64 { return s.cfg.Now().UnixMilli() }

func writerName(r Role, key string) string {
	if key != "" {
		return key
	}
	return string(r)
}

// Submit walks the task folder (or the named paths), validates and diffs each changed file against
// its cache, decides, applies what is accepted, restores what is refused and raises one approval for
// what waits on the user.
func (s *Service) Submit(ctx context.Context, req Request) (Result, error) {
	st, err := s.cfg.Open(req.TaskID)
	if err != nil {
		return Result{}, err
	}
	rows, err := s.cfg.Index.Files(req.TaskID)
	if err != nil {
		return Result{}, err
	}
	intents, err := s.cfg.Index.Intents()
	if err != nil {
		return Result{}, err
	}
	run := &submitRun{s: s, ctx: ctx, st: st, req: req, byPath: make(map[string]model.TaskFile, len(rows)), intents: map[string]string{},
		pend: PendingSubmit{TaskID: req.TaskID, Session: req.Session}}
	for _, in := range intents {
		if in.TaskID == req.TaskID {
			run.intents[in.RelPath] = in.SHA256
		}
	}
	for _, r := range rows {
		run.byPath[r.RelPath] = r
	}
	entries, err := run.list()
	if err != nil {
		return Result{}, err
	}
	paths, bad := s.targets(ctx, st, req, run.present, run.byPath, run.truncated)
	run.out = append(run.out, bad...)
	run.over = s.sizeProblems(entries)
	units := run.units(paths)
	run.files(paths, units)
	if err := run.finish(); err != nil {
		return Result{}, err
	}
	return Result{Files: run.out}, nil
}

// submitRun is the state of one Submit call.
type submitRun struct {
	s       *Service
	ctx     context.Context
	st      store.Store
	req     Request
	byPath  map[string]model.TaskFile
	present map[string]store.Entry
	over    map[string]string
	// intents maps the path of an open Kira write to the hash it is writing, "" for a delete.
	intents map[string]string
	// truncated: the listing stopped at the file cap, so a path missing from present is unknown, not gone.
	truncated bool
	out       []FileResult
	pend      PendingSubmit
}

// list reads the folder; a folder over the file cap is one invalid result, not an error.
func (r *submitRun) list() ([]store.Entry, error) {
	entries, err := r.st.List(r.ctx, store.ListOptions{MaxDepth: MaxDepth, MaxFiles: MaxFilesPerTask})
	if errors.Is(err, store.ErrTooManyFiles) {
		r.truncated = true
		r.out = append(r.out, FileResult{Path: ".", Status: StatusInvalid,
			Message: fmt.Sprintf("the task folder holds more than %d files; remove some.", MaxFilesPerTask)})
	} else if err != nil {
		return nil, err
	}
	r.present = make(map[string]store.Entry, len(entries))
	for _, e := range entries {
		r.present[e.Rel] = e
	}
	return entries, nil
}

// add keeps a result and the approval item it raised, if any; a result with no path says nothing.
func (r *submitRun) add(fr FileResult, item *PendingItem) {
	if fr.Path == "" {
		return
	}
	r.out = append(r.out, fr)
	if item != nil {
		r.pend.Items = append(r.pend.Items, *item)
	}
}

// units runs every proof folder as one unit and returns the slugs it handled.
func (r *submitRun) units(paths []string) map[string]bool {
	done := map[string]bool{}
	if r.s.cfg.Proof == nil {
		return done
	}
	for _, slug := range unitSlugs(paths) {
		done[slug] = true
		if r.truncated && r.unitUnlisted(slug) {
			continue
		}
		r.add(r.s.unit(r.ctx, r.st, r.req, slug, r.present, r.byPath, r.over))
	}
	return done
}

// unitUnlisted is whether an indexed file of the proof folder is outside a truncated listing, so the
// unit cannot tell a deleted file from an unlisted one.
func (r *submitRun) unitUnlisted(slug string) bool {
	for rel := range r.byPath {
		if Classify(rel).Slug == slug {
			if _, ok := r.present[rel]; !ok {
				return true
			}
		}
	}
	return false
}

// files runs every other path on its own, skipping the files of a proof unit.
func (r *submitRun) files(paths []string, units map[string]bool) {
	ignored := 0
	hinted := map[string]bool{}
	for _, rel := range paths {
		if c := Classify(rel); c.Slug != "" && units[c.Slug] {
			continue
		}
		e, ok := r.present[rel]
		fr, item := r.s.one(r.ctx, r.st, r.req, rel, e, ok, r.byPath[rel], r.over, r.intents)
		if fr.Status == StatusInvalid && hinted[fr.Path] {
			continue
		}
		if fr.Status == StatusInvalid && ProofFolderHint(rel) != "" {
			hinted[fr.Path] = true
		}
		if fr.Status == StatusIgnored {
			if ignored++; ignored > maxIgnoredReported {
				continue
			}
		}
		r.add(fr, item)
	}
	if ignored > maxIgnoredReported {
		r.out = append(r.out, FileResult{Path: ".", Status: StatusIgnored, Message: fmt.Sprintf("and %d more files that are not task files.", ignored-maxIgnoredReported)})
	}
}

// finish raises the approval for what waits on the user and notifies what was accepted.
func (r *submitRun) finish() error {
	ev := r.s.cfg.Events
	if ev == nil {
		return nil
	}
	if len(r.pend.Items) > 0 {
		if err := ev.Pending(r.ctx, r.pend); err != nil {
			return fmt.Errorf("taskfiles: raise approval: %w", err)
		}
	}
	for _, f := range r.out {
		if f.Status == StatusAccepted && f.Notify != NotifyNone {
			if err := ev.Accepted(r.ctx, r.req.TaskID, f.Path, f.Notify); err != nil {
				slog.Warn("taskfiles: notify accepted file", "scope", "ade", "task", r.req.TaskID, "path", f.Path, "err", err)
			}
		}
	}
	return nil
}

// targets are the paths a call looks at, in path order, and an invalid result per unusable named path.
// With a truncated listing indexed paths outside it are left alone.
func (s *Service) targets(ctx context.Context, st store.Store, req Request, present map[string]store.Entry, rows map[string]model.TaskFile, truncated bool) ([]string, []FileResult) {
	var out []string
	var bad []FileResult
	if len(req.Paths) > 0 {
		for _, p := range req.Paths {
			if msg := namedProblem(ctx, st, p, present, rows, truncated); msg != "" {
				bad = append(bad, FileResult{Path: p, Status: StatusInvalid, Message: msg})
			} else if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	} else {
		for p, e := range present {
			if !e.Dir {
				out = append(out, p)
			}
		}
		for p := range rows {
			if _, ok := present[p]; !ok && !truncated {
				out = append(out, p)
			}
		}
		for p, e := range present { // a folder at the depth limit is not Details
			if e.Dir && Classify(p+"/x").Kind == KindIgnored && !strings.HasPrefix(p, ".") {
				out = append(out, p+"/")
			}
		}
	}
	sort.Strings(out)
	return out, bad
}

// namedProblem is why a named path cannot be processed, "" when it can. A path a truncated listing
// lacks is checked with Stat: on disk it is unread, off disk it is a deletion to process.
func namedProblem(ctx context.Context, st store.Store, p string, present map[string]store.Entry, rows map[string]model.TaskFile, truncated bool) string {
	if err := store.ValidRel(p); err != nil {
		return p + ": not a path inside the task folder; use a relative path like notes/a.md."
	}
	if truncated && !hasEntry(p, present, nil) {
		if _, err := st.Stat(ctx, strings.TrimSuffix(p, "/")); err == nil {
			return fmt.Sprintf("%s: not read, the task folder holds more than %d files; remove some.", p, MaxFilesPerTask)
		}
	}
	if !hasEntry(p, present, rows) {
		return p + ": no such file in the task folder."
	}
	return ""
}

// hasEntry reports whether rel names a file or folder in the task folder, or an indexed file (or proof
// folder) that was deleted since: submit still has to process the deletion.
func hasEntry(rel string, present map[string]store.Entry, rows map[string]model.TaskFile) bool {
	trimmed := strings.TrimSuffix(rel, "/")
	if _, ok := present[trimmed]; ok {
		return true
	}
	for p := range rows {
		if p == trimmed || strings.HasPrefix(p, trimmed+"/") {
			return true
		}
	}
	return false
}

// sizeProblems maps a path to its cap message when the task or its proof folder is over its total.
func (s *Service) sizeProblems(entries []store.Entry) map[string]string {
	var total int64
	proofs := map[string]int64{}
	for _, e := range entries {
		if e.Dir {
			continue
		}
		total += e.Size
		if c := Classify(e.Rel); c.Slug != "" {
			proofs[c.Slug] += e.Size
		}
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.Dir {
			continue
		}
		switch c := Classify(e.Rel); {
		case total > MaxBytesPerTask:
			out[e.Rel] = fmt.Sprintf("the task folder is %d MiB; the limit is %d MiB. Remove files.", total>>20, MaxBytesPerTask>>20)
		case c.Slug != "" && proofs[c.Slug] > MaxBytesPerProof:
			out[e.Rel] = fmt.Sprintf("proofs/%s is %d MiB; the limit is %d MiB per proof.", c.Slug, proofs[c.Slug]>>20, MaxBytesPerProof>>20)
		}
	}
	return out
}

// one processes a single path; the pending item it returns is nil unless the file waits on the user.
func (s *Service) one(ctx context.Context, st store.Store, req Request, rel string, e store.Entry, exists bool, row model.TaskFile, over map[string]string, intents map[string]string) (FileResult, *PendingItem) {
	hasRow := row.TaskID != ""
	class := Classify(rel)
	h := s.cfg.Handlers[class.Kind]
	if res, done := skipPath(rel, e, exists, class, h); done {
		return res, nil
	}
	fail := func(msg string, line int) (FileResult, *PendingItem) {
		return s.markInvalid(req, rel, class, row, hasRow, e, msg, line), nil
	}
	if res, done := settledFile(rel, e, exists, row); done {
		return res, nil
	}
	if msg, tooBig := over[rel]; tooBig && exists {
		return fail(rel+": "+msg, 0)
	}
	if !exists {
		if hash, open := intents[rel]; open && hash == "" {
			return FileResult{}, nil // Kira is deleting it
		}
		return s.gone(ctx, st, req, rel, class, h, row, hasRow)
	}
	raw, info, err := st.Read(ctx, rel, MaxBytes(class.Kind, rel))
	switch {
	case errors.Is(err, store.ErrTooLarge):
		return fail(fmt.Sprintf("%s: too large, the limit is %d KiB.", rel, MaxBytes(class.Kind, rel)>>10), 0)
	case err != nil:
		return fail(fmt.Sprintf("%s: cannot read it: %v", rel, err), 0)
	}
	hash := sum(raw)
	if want, open := intents[rel]; open && want == hash {
		return FileResult{}, nil // Kira is mid-write: its own transaction records the file
	}
	if hasRow && hash == row.AcceptedSHA256 {
		row.Size, row.Version, row.SHA256 = info.Size, string(info.Version), hash
		row.Review, row.Problem = model.TaskFileAccepted, ""
		if err := s.cfg.Index.InTx(func(tx Tx) error { return tx.PutFile(row) }); err != nil {
			return fail(rel+": "+err.Error(), 0)
		}
		return FileResult{Path: rel, Status: StatusUnchanged}, nil
	}
	if hasRow && row.Review == model.TaskFilePending && hash == row.SHA256 {
		// Still the content that waits on the user: a later pass must not decide it again.
		return FileResult{Path: rel, Status: StatusPending, Message: row.Problem}, nil
	}
	if class.Kind.KiraOwned() {
		return s.restoreResult(ctx, req.TaskID, rel)
	}
	parsed, err := h.Parse(rel, class, raw)
	if err != nil {
		var pe *ParseError
		if errors.As(err, &pe) {
			return fail(fmt.Sprintf("%s: %s", rel, pe.Error()), pe.Line)
		}
		return fail(fmt.Sprintf("%s: %v", rel, err), 0)
	}
	return s.decide(ctx, st, req, rel, class, h, parsed, info, hash, row, hasRow)
}

// skipPath short-circuits a path that is no task file: a proof-folder hint, a folder, an ignored kind or none.
func skipPath(rel string, e store.Entry, exists bool, class Class, h Handler) (FileResult, bool) {
	if hint := ProofFolderHint(rel); hint != "" {
		if !exists && !strings.HasSuffix(rel, "/") {
			return FileResult{}, true
		}
		return FileResult{Path: hintPath(rel), Status: StatusInvalid, Message: hint}, true
	}
	if strings.HasSuffix(rel, "/") {
		return FileResult{Path: rel, Status: StatusIgnored, Message: "not a task file."}, true
	}
	if class.Kind == KindIgnored || h == nil {
		if !exists || strings.HasPrefix(e.Rel, ".") || strings.Contains(rel, "/.") {
			return FileResult{}, true
		}
		return FileResult{Path: rel, Status: StatusIgnored, Message: "not a task file."}, true
	}
	return FileResult{}, false
}

// settledFile answers a present file that needs no read: refused, or unchanged since accepted.
func settledFile(rel string, e store.Entry, exists bool, row model.TaskFile) (FileResult, bool) {
	if !exists {
		return FileResult{}, false
	}
	if e.Refused != "" {
		return FileResult{Path: rel, Status: StatusInvalid, Message: fmt.Sprintf("%s: refused, it is a %s. Use a plain file.", rel, e.Refused)}, true
	}
	if row.TaskID != "" && row.Review == model.TaskFileAccepted && row.Size == e.Size && string(e.Version) == row.Version {
		return FileResult{Path: rel, Status: StatusUnchanged}, true
	}
	return FileResult{}, false
}

// markInvalid records the problem on the file's row (keeping its accepted copy) and returns the result.
func (s *Service) markInvalid(req Request, rel string, class Class, row model.TaskFile, hasRow bool, e store.Entry, msg string, line int) FileResult {
	if !hasRow {
		row = model.TaskFile{TaskID: req.TaskID, RelPath: rel, Kind: string(class.Kind)}
	}
	row.Size, row.Version, row.Review, row.Problem = e.Size, string(e.Version), model.TaskFileInvalid, msg
	if err := s.cfg.Index.InTx(func(tx Tx) error { return tx.PutFile(row) }); err != nil {
		slog.Warn("taskfiles: record invalid file", "scope", "ade", "task", req.TaskID, "path", rel, "err", err)
	}
	return FileResult{Path: rel, Status: StatusInvalid, Message: msg, Line: line}
}

// gone handles a path with an index row and no file.
func (s *Service) gone(ctx context.Context, st store.Store, req Request, rel string, class Class, h Handler, row model.TaskFile, hasRow bool) (FileResult, *PendingItem) {
	if !hasRow {
		return FileResult{}, nil
	}
	if class.Kind.KiraOwned() {
		return s.restoreResult(ctx, req.TaskID, rel)
	}
	return s.decide(ctx, st, req, rel, class, h, nil, store.Info{}, "", row, hasRow)
}

// decide diffs, decides per change and applies in one transaction.
func (s *Service) decide(ctx context.Context, st store.Store, req Request, rel string, class Class, h Handler, parsed Parsed, info store.Info, hash string, row model.TaskFile, hasRow bool) (FileResult, *PendingItem) {
	var res FileResult
	var item *PendingItem
	err := s.cfg.Index.InTx(func(tx Tx) error {
		changes, err := h.Diff(ctx, tx, req.TaskID, rel, class, parsed)
		if err != nil {
			return err
		}
		coll := s.collisionOf(req, row, hasRow)
		v := s.judge(req, rel, class, changes, coll, s.proofAccepted(req.TaskID, class, tx))
		pending, reject, notify, cause := v.pending, v.reject, v.notify, v.cause
		switch {
		case len(reject) > 0:
			res = FileResult{Path: rel, Status: StatusRejected, Message: reject[0].Message}
			return nil
		case len(changes) == 0:
			res = FileResult{Path: rel, Status: StatusUnchanged}
			return s.putRow(tx, req, rel, class, row, hasRow, info, hash, model.TaskFileAccepted, "", true)
		}
		if err := applyAccepted(ctx, tx, h, req.TaskID, rel, class, parsed, v); err != nil {
			return err
		}
		if len(pending) > 0 {
			msg := pendingMessage(rel, cause, coll)
			res = FileResult{Path: rel, Status: StatusPending, Message: msg}
			item = &PendingItem{Path: rel, Kind: class.Kind, Cause: cause, Changes: pending, Collision: coll}
			return s.putRow(tx, req, rel, class, row, hasRow, info, hash, model.TaskFilePending, msg, false)
		}
		res = FileResult{Path: rel, Status: StatusAccepted, Notify: notify}
		if parsed == nil {
			return tx.DeleteFile(req.TaskID, rel)
		}
		return s.putRow(tx, req, rel, class, row, hasRow, info, hash, model.TaskFileAccepted, "", true)
	})
	if err != nil {
		return FileResult{Path: rel, Status: StatusInvalid, Message: fmt.Sprintf("%s: could not apply: %v", rel, err)}, nil
	}
	if res.Status == StatusRejected && Classify(rel).Kind == KindDraft {
		if err := st.Remove(ctx, rel); err != nil {
			slog.Warn("taskfiles: remove rejected draft", "scope", "ade", "task", req.TaskID, "path", rel, "err", err)
		}
	}
	return res, item
}

// applyAccepted moves the accepted changes of a file into the cache. A delete applies only when
// nothing else of the file waits on the user.
func applyAccepted(ctx context.Context, tx Tx, h Handler, taskID, rel string, class Class, parsed Parsed, v verdict) error {
	switch {
	case len(v.accepted) == 0:
		return nil
	case parsed != nil:
		return h.Apply(ctx, tx, taskID, rel, class, parsed, v.accepted)
	case len(v.pending) == 0:
		return h.Remove(ctx, tx, taskID, rel, class)
	}
	return nil
}

func pendingMessage(rel string, cause Cause, coll *Collision) string {
	if coll != nil {
		return fmt.Sprintf("%s: %s changed it at %s after your last read; waiting for the user.", rel, coll.By, coll.At.UTC().Format("15:04:05"))
	}
	return fmt.Sprintf("%s: waiting for the user (%s).", rel, causeText[cause])
}

// collisionOf is set when another agent accepted the file after the session's last sync; the user's
// and Kira's own acceptances are no collision.
func (s *Service) collisionOf(req Request, row model.TaskFile, hasRow bool) *Collision {
	if hasRow && !req.Session.LastSync.IsZero() && row.AcceptedBy != "" && row.AcceptedBy != string(RoleKira) && row.AcceptedBy != string(RoleUser) && row.AcceptedBy != writerName(req.Session.Role, req.Session.Key) &&
		row.AcceptedAt > req.Session.LastSync.UnixMilli() {
		return &Collision{By: row.AcceptedBy, At: time.UnixMilli(row.AcceptedAt)}
	}
	return nil
}

// verdict sorts the changes of one file by their decision.
type verdict struct {
	accepted, pending []Change
	reject            []Decision
	notify            Notify
	cause             Cause
}

func (s *Service) judge(req Request, rel string, class Class, changes []Change, coll *Collision, proofAccepted bool) verdict {
	var v verdict
	for _, c := range changes {
		d := Decide(rel, Input{
			Role: req.Session.Role, Kind: class.Kind, Op: c.Op, Author: c.Author, Slot: c.Slot,
			Step: class.Step, CurrentStep: req.Session.Step, ProofAccepted: proofAccepted, Latest: c.Latest,
			Collision: coll,
		})
		switch d.Outcome {
		case Accept:
			v.accepted = append(v.accepted, c)
			if d.Notify != NotifyNone {
				v.notify = d.Notify
			}
		case Pending:
			v.pending = append(v.pending, c)
			v.cause = d.Cause
		case Reject:
			v.reject = append(v.reject, d)
		}
	}
	return v
}

// putRow writes the file's index row. accept moves the accepted copy to this file's hash.
func (s *Service) putRow(tx Tx, req Request, rel string, class Class, row model.TaskFile, hasRow bool, info store.Info, hash, review, problem string, accept bool) error {
	if !hasRow {
		row = model.TaskFile{TaskID: req.TaskID, RelPath: rel}
	}
	row.Kind, row.Size, row.Version, row.SHA256 = string(class.Kind), info.Size, string(info.Version), hash
	row.Review, row.Problem = review, problem
	if accept {
		row.AcceptedSHA256, row.AcceptedBy, row.AcceptedAt = hash, writerName(req.Session.Role, req.Session.Key), s.nowMs()
	}
	return tx.PutFile(row)
}

// proofAccepted is whether the proof folder of class already has an accepted proof.md.
func (s *Service) proofAccepted(taskID string, class Class, tx Tx) bool {
	if class.Slug == "" {
		return false
	}
	r, err := tx.File(taskID, "proofs/"+class.Slug+"/"+ProofFile)
	return err == nil && r != nil && r.AcceptedSHA256 != ""
}

// Changed lists the paths whose size or version differ from the index, new and missing files
// included, without reading a file. It is the stat pass of task open, Details focus and Refresh.
// Kira-written files and pending or invalid rows count: they are changes nobody settled.
func (s *Service) Changed(ctx context.Context, taskID string) ([]string, error) {
	st, err := s.cfg.Open(taskID)
	if err != nil {
		return nil, err
	}
	rows, err := s.cfg.Index.Files(taskID)
	if err != nil {
		return nil, err
	}
	entries, err := st.List(ctx, store.ListOptions{MaxDepth: MaxDepth, MaxFiles: MaxFilesPerTask})
	if err != nil && !errors.Is(err, store.ErrTooManyFiles) {
		return nil, err
	}
	byPath := make(map[string]model.TaskFile, len(rows))
	for _, r := range rows {
		byPath[r.RelPath] = r
	}
	var out []string
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		seen[e.Rel] = true
		if e.Dir || !s.handles(Classify(e.Rel).Kind) {
			continue
		}
		r, ok := byPath[e.Rel]
		if !ok || r.Review != model.TaskFileAccepted || r.Size != e.Size || r.Version != string(e.Version) {
			out = append(out, e.Rel)
		}
	}
	for p, r := range byPath {
		if !seen[p] && r.Review == model.TaskFileAccepted {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}
