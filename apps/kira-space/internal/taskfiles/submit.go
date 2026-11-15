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
	Reason  string
	Session Session
	// Tool names the tool in error texts: "submit" (default) or "proof".
	Tool string
}

// Config wires a Service.
type Config struct {
	Index Index
	// Open returns the store of a task's folder.
	Open     func(taskID string) (store.Store, error)
	Handlers map[Kind]Handler
	Events   Events
	Now      func() time.Time
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
	if req.Tool == "" {
		req.Tool = "submit"
	}
	reason, err := CleanReason(req.Reason)
	if err != nil {
		return Result{}, err
	}
	req.Reason = reason
	st, err := s.cfg.Open(req.TaskID)
	if err != nil {
		return Result{}, err
	}
	rows, err := s.cfg.Index.Files(req.TaskID)
	if err != nil {
		return Result{}, err
	}
	byPath := make(map[string]model.TaskFile, len(rows))
	for _, r := range rows {
		byPath[r.RelPath] = r
	}
	entries, listErr := st.List(ctx, store.ListOptions{MaxDepth: MaxDepth, MaxFiles: MaxFilesPerTask})
	var out []FileResult
	if errors.Is(listErr, store.ErrTooManyFiles) {
		out = append(out, FileResult{Path: ".", Status: StatusInvalid,
			Message: fmt.Sprintf("the task folder holds more than %d files; remove some.", MaxFilesPerTask)})
	} else if listErr != nil {
		return Result{}, listErr
	}
	present := make(map[string]store.Entry, len(entries))
	for _, e := range entries {
		present[e.Rel] = e
	}
	paths, bad := s.targets(req, present, byPath)
	out = append(out, bad...)
	over := s.sizeProblems(entries)

	pend := PendingSubmit{TaskID: req.TaskID, Session: req.Session, Reason: req.Reason}
	ignored := 0
	for _, rel := range paths {
		e, ok := present[rel]
		fr, item := s.one(ctx, st, req, rel, e, ok, byPath[rel], over)
		if fr.Status == StatusIgnored {
			if ignored++; ignored > maxIgnoredReported {
				continue
			}
		}
		if fr.Path == "" {
			continue
		}
		out = append(out, fr)
		if item != nil {
			pend.Items = append(pend.Items, *item)
		}
	}
	if ignored > maxIgnoredReported {
		out = append(out, FileResult{Path: ".", Status: StatusIgnored, Message: fmt.Sprintf("and %d more files that are not task files.", ignored-maxIgnoredReported)})
	}
	if len(pend.Items) > 0 && s.cfg.Events != nil {
		if err := s.cfg.Events.Pending(ctx, pend); err != nil {
			return Result{}, fmt.Errorf("taskfiles: raise approval: %w", err)
		}
	}
	for _, f := range out {
		if f.Status == StatusAccepted && f.Notify != NotifyNone && s.cfg.Events != nil {
			if err := s.cfg.Events.Accepted(ctx, req.TaskID, f.Path, f.Notify); err != nil {
				slog.Warn("taskfiles: notify accepted file", "scope", "ade", "task", req.TaskID, "path", f.Path, "err", err)
			}
		}
	}
	return Result{Files: out}, nil
}

// targets are the paths a call looks at, in path order, and an invalid result per unusable named path.
func (s *Service) targets(req Request, present map[string]store.Entry, rows map[string]model.TaskFile) ([]string, []FileResult) {
	var out []string
	var bad []FileResult
	if len(req.Paths) > 0 {
		for _, p := range req.Paths {
			if err := store.ValidRel(p); err != nil {
				bad = append(bad, FileResult{Path: p, Status: StatusInvalid, Message: p + ": not a path inside the task folder; use a relative path like notes/a.md."})
				continue
			}
			if !slices.Contains(out, p) {
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
			if _, ok := present[p]; !ok {
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
func (s *Service) one(ctx context.Context, st store.Store, req Request, rel string, e store.Entry, exists bool, row model.TaskFile, over map[string]string) (FileResult, *PendingItem) {
	hasRow := row.TaskID != ""
	if strings.HasSuffix(rel, "/") {
		return FileResult{Path: rel, Status: StatusIgnored, Message: "not a task file."}, nil
	}
	class := Classify(rel)
	h := s.cfg.Handlers[class.Kind]
	if class.Kind == KindIgnored || h == nil {
		if !exists || strings.HasPrefix(e.Rel, ".") || strings.Contains(rel, "/.") {
			return FileResult{}, nil
		}
		return FileResult{Path: rel, Status: StatusIgnored, Message: "not a task file."}, nil
	}
	fail := func(msg string, line int) (FileResult, *PendingItem) {
		return s.markInvalid(req, rel, class, row, hasRow, e, msg, line), nil
	}
	if exists && e.Refused != "" {
		return FileResult{Path: rel, Status: StatusInvalid, Message: fmt.Sprintf("%s: refused, it is a %s. Use a plain file.", rel, e.Refused)}, nil
	}
	if exists && hasRow && row.Review == model.TaskFileAccepted && row.Size == e.Size && string(e.Version) == row.Version {
		return FileResult{Path: rel, Status: StatusUnchanged}, nil
	}
	if msg, tooBig := over[rel]; tooBig && exists {
		return fail(rel+": "+msg, 0)
	}
	if !exists {
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
	if hasRow && hash == row.AcceptedSHA256 {
		row.Size, row.Version, row.SHA256 = info.Size, string(info.Version), hash
		row.Review, row.Problem = model.TaskFileAccepted, ""
		if err := s.cfg.Index.InTx(func(tx Tx) error { return tx.PutFile(row) }); err != nil {
			return fail(rel+": "+err.Error(), 0)
		}
		return FileResult{Path: rel, Status: StatusUnchanged}, nil
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
		var coll *Collision
		if hasRow && !req.Session.LastSync.IsZero() && row.AcceptedBy != "" && row.AcceptedBy != writerName(req.Session.Role, req.Session.Key) &&
			row.AcceptedAt > req.Session.LastSync.UnixMilli() {
			coll = &Collision{By: row.AcceptedBy, At: time.UnixMilli(row.AcceptedAt)}
		}
		proofAccepted := s.proofAccepted(req.TaskID, class, tx)
		var accepted, pending []Change
		var invalid, reject []Decision
		var notify Notify
		var cause Cause
		for _, c := range changes {
			d := Decide(rel, req.Tool, Input{
				Role: req.Session.Role, Kind: class.Kind, Op: c.Op, Own: c.Own, Slot: c.Slot,
				Step: class.Step, CurrentStep: req.Session.Step, ProofAccepted: proofAccepted,
				Collision: coll, Reason: req.Reason,
			})
			switch d.Outcome {
			case Accept:
				accepted = append(accepted, c)
				if d.Notify != NotifyNone {
					notify = d.Notify
				}
			case Pending:
				pending = append(pending, c)
				cause = d.Cause
			case Invalid:
				invalid = append(invalid, d)
			case Reject:
				reject = append(reject, d)
			}
		}
		switch {
		case len(reject) > 0:
			res = FileResult{Path: rel, Status: StatusRejected, Message: reject[0].Message}
			return nil
		case len(changes) == 0:
			res = FileResult{Path: rel, Status: StatusUnchanged}
			return s.putRow(tx, req, rel, class, row, hasRow, info, hash, model.TaskFileAccepted, "", true)
		}
		if len(accepted) > 0 {
			if parsed == nil {
				if len(pending) == 0 && len(invalid) == 0 {
					if err := h.Remove(ctx, tx, req.TaskID, rel, class); err != nil {
						return err
					}
				}
			} else if err := h.Apply(ctx, tx, req.TaskID, rel, class, parsed, accepted); err != nil {
				return err
			}
		}
		switch {
		case len(invalid) > 0:
			msgs := make([]string, len(invalid))
			for i, d := range invalid {
				msgs[i] = d.Message
			}
			res = FileResult{Path: rel, Status: StatusInvalid, Message: strings.Join(dedupe(msgs), " ")}
			return s.putRow(tx, req, rel, class, row, hasRow, info, hash, model.TaskFileInvalid, res.Message, false)
		case len(pending) > 0:
			msg := fmt.Sprintf("%s: waiting for the user (%s).", rel, req.Reason)
			if coll != nil {
				msg = fmt.Sprintf("%s: %s changed it at %s after your last read; waiting for the user (%s).", rel, coll.By, coll.At.UTC().Format("15:04:05"), req.Reason)
			}
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

func dedupe(in []string) []string {
	var out []string
	for _, s := range in {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
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
		if e.Dir || s.cfg.Handlers[Classify(e.Rel).Kind] == nil {
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
