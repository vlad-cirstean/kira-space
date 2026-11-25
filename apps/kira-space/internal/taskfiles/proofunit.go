package taskfiles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// ProofConfig wires the proof unit: the cache of proof folders, the blob store holding the bytes of
// every accepted or proposed file of a unit, and the id source of new proofs.
type ProofConfig struct {
	Cache repos.ProofCache
	Blobs *store.Blobs
	NewID func() string
}

// ProofEvents is the optional extension of Events for proofs.
type ProofEvents interface {
	// ProofReview raises (pending true) or withdraws the review of a proof's pending edit.
	ProofReview(ctx context.Context, taskID, slug string, pending bool) error
	// ProofChanged reports a change of a proof's accepted or pending content, or its delete, that raised
	// or withdrew no review.
	ProofChanged(ctx context.Context, sessionID string) error
}

// ProofResult is the structured answer for one proof unit.
type ProofResult struct {
	Slug, Name string
	// Status is accepted, pending or invalid.
	Status string
	New    bool
	Steps  []ProofStepInfo
	Counts ProofCounts
	// Changed lists what a pending or accepted edit changes; nil for a new proof.
	Changed *ProofChanged
	// Problems are the full lines of an invalid unit.
	Problems []string
}

// ProofStepInfo is a step id and its view.
type ProofStepInfo struct{ ID, View string }

// ProofCounts tallies a unit.
type ProofCounts struct{ Steps, Images, DataFiles int }

// ProofChanged is the step-level difference of an edit.
type ProofChanged struct {
	Added, Changed, Removed []string
	// Fields names changed proof-level fields: name, summary, order.
	Fields []string
}

// proofAuthor is who wrote the proof, from the calling session's view. A proof made before its link
// carried the session has none, so it counts as another agent's.
func proofAuthor(u *repos.ProofUnit, caller string) Author {
	return AuthorOf(u.LinkSource == "agent", u.LinkSession, caller)
}

func proofFolder(slug string) string { return "proofs/" + slug + "/" }

func isProofKind(k Kind) bool { return k == KindProof || k == KindProofData || k == KindProofImage }

// handles reports whether a kind has a handler, the proof kinds counting through the unit.
func (s *Service) handles(k Kind) bool {
	return s.cfg.Handlers[k] != nil || (isProofKind(k) && s.cfg.Proof != nil)
}

// unitSlugs lists the proof slugs a call touches, in order.
func unitSlugs(paths []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range paths {
		if c := Classify(p); c.Slug != "" && !seen[c.Slug] {
			seen[c.Slug] = true
			out = append(out, c.Slug)
		}
	}
	return out
}

type unitFile struct {
	rel  string
	name string
	raw  []byte
	info store.Info
	hash string
}

// unit decides one proof folder all or nothing.
func (s *Service) unit(ctx context.Context, st store.Store, req Request, slug string, present map[string]store.Entry, byPath map[string]model.TaskFile, over map[string]string) (FileResult, *PendingItem) {
	folder := proofFolder(slug)
	md := folder + ProofFile
	var rels []string
	for rel, e := range present {
		if !e.Dir && Classify(rel).Slug == slug {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	var rows []model.TaskFile
	for rel, r := range byPath {
		if strings.HasPrefix(rel, folder) && Classify(rel).Slug == slug {
			rows = append(rows, r)
		}
	}
	if len(rels) == 0 {
		return s.unitGone(ctx, req, slug, rows)
	}
	if s.unitUnchanged(rels, present, byPath, rows) {
		return FileResult{Path: md, Status: StatusUnchanged}, nil
	}
	var probs []problem
	files := proofFiles{}
	var read []unitFile
	for _, rel := range rels {
		e := present[rel]
		name := strings.TrimPrefix(rel, folder)
		class := Classify(rel)
		if e.Refused != "" {
			probs = append(probs, problem{name, 0, fmt.Sprintf("refused, it is a %s. Use a plain file.", e.Refused)})
			continue
		}
		if msg, big := over[rel]; big {
			probs = append(probs, problem{name, 0, msg})
			continue
		}
		raw, info, err := st.Read(ctx, rel, MaxBytes(class.Kind, rel))
		switch {
		case errors.Is(err, store.ErrTooLarge):
			probs = append(probs, problem{name, 0, fmt.Sprintf("too large, the limit is %d KiB.", MaxBytes(class.Kind, rel)>>10)})
			continue
		case err != nil:
			probs = append(probs, problem{name, 0, fmt.Sprintf("cannot read it: %v", err)})
			continue
		}
		read = append(read, unitFile{rel: rel, name: name, raw: raw, info: info, hash: sum(raw)})
		files[name] = raw
	}
	var doc proofDoc
	used := map[string]bool{}
	mdRaw, haveMD := files[ProofFile]
	switch {
	case !haveMD && !hasProblemFor(probs, ProofFile):
		probs = append(probs, problem{ProofFile, 0, "the folder has no proof.md. Write proofs/" + slug + "/proof.md, or remove the folder."})
	case haveMD:
		var ps []problem
		doc, ps = parseProofMD(mdRaw, files, used)
		probs = append(probs, ps...)
	}
	if haveMD && len(probs) == 0 || haveMD && !hasFileProblems(probs) {
		for _, f := range read {
			if f.name != ProofFile && !used[f.name] {
				probs = append(probs, problem{f.name, 0, "not used by proof.md. Name it in a step's data, or remove it."})
			}
		}
	}
	if len(probs) > 0 {
		return s.unitInvalid(req, slug, rels, present, byPath, probs)
	}
	return s.unitApply(ctx, req, slug, doc, read, rows, byPath)
}

func hasProblemFor(probs []problem, name string) bool {
	for _, p := range probs {
		if p.File == name {
			return true
		}
	}
	return false
}

func hasFileProblems(probs []problem) bool {
	for _, p := range probs {
		if p.File != ProofFile {
			return true
		}
	}
	return false
}

// unitUnchanged is true when every file on disk matches an accepted row and no row lacks its file.
func (s *Service) unitUnchanged(rels []string, present map[string]store.Entry, byPath map[string]model.TaskFile, rows []model.TaskFile) bool {
	if len(rels) != len(rows) {
		return false
	}
	for _, rel := range rels {
		r, ok := byPath[rel]
		e := present[rel]
		if !ok || r.Review != model.TaskFileAccepted || r.Size != e.Size || r.Version != string(e.Version) {
			return false
		}
	}
	return true
}

func (s *Service) unitInvalid(req Request, slug string, rels []string, present map[string]store.Entry, byPath map[string]model.TaskFile, probs []problem) (FileResult, *PendingItem) {
	folder := proofFolder(slug)
	lines := make([]string, len(probs))
	for i, p := range probs {
		lines[i] = p.text(folder)
	}
	md := folder + ProofFile
	path := md
	if _, ok := present[md]; !ok {
		path = folder
	}
	msg := strings.Join(lines, "\n")
	err := s.cfg.Index.InTx(func(tx Tx) error {
		for _, rel := range rels {
			row, ok := byPath[rel]
			if !ok {
				row = model.TaskFile{TaskID: req.TaskID, RelPath: rel, Kind: string(Classify(rel).Kind)}
			}
			e := present[rel]
			row.Size, row.Version, row.Review, row.Problem = e.Size, string(e.Version), model.TaskFileInvalid, msg
			if err := tx.PutFile(row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("taskfiles: record invalid proof", "scope", "ade", "task", req.TaskID, "slug", slug, "err", err)
	}
	return FileResult{Path: path, Status: StatusInvalid, Message: msg, Line: probs[0].Line,
		Proof: &ProofResult{Slug: slug, Status: string(StatusInvalid), Problems: lines}}, nil
}

// stepRows turns validated steps into cache rows.
func stepRows(doc proofDoc) []repos.ProofStepRow {
	out := make([]repos.ProofStepRow, len(doc.Steps))
	for i, st := range doc.Steps {
		out[i] = repos.ProofStepRow{StepKey: st.ID, Description: st.Description, Query: st.Query, Language: st.Language,
			Source: st.Source, View: st.View, Payload: st.Payload}
	}
	return out
}

func proofInfo(doc proofDoc, read []unitFile) ([]ProofStepInfo, ProofCounts) {
	steps := make([]ProofStepInfo, 0, len(doc.Steps))
	c := ProofCounts{Steps: len(doc.Steps)}
	for _, st := range doc.Steps {
		steps = append(steps, ProofStepInfo{ID: st.ID, View: st.View})
		switch {
		case st.View == proof.ViewImage:
			c.Images++
		case st.File != "":
			c.DataFiles++
		}
	}
	return steps, c
}

func changedOf(changes []Change) *ProofChanged {
	out := &ProofChanged{}
	for _, c := range changes {
		switch {
		case c.Field != "":
			out.Fields = append(out.Fields, c.Field)
		case c.Op == OpCreate:
			out.Added = append(out.Added, c.Key)
		case c.Op == OpEdit:
			out.Changed = append(out.Changed, c.Key)
		case c.Op == OpDelete:
			out.Removed = append(out.Removed, c.Key)
		}
	}
	return out
}

// cacheHead is the accepted proof's head from the cache.
func (s *Service) cacheHead(tx Tx, u *repos.ProofUnit) (ProofHead, error) {
	rev := 0
	if u.Session.CurrentRev != nil {
		rev = *u.Session.CurrentRev
	}
	steps, err := s.cfg.Proof.Cache.Steps(tx.Querier(), u.Session.ID, rev)
	if err != nil {
		return ProofHead{}, err
	}
	h := ProofHead{Name: u.Session.Name, Summary: u.Session.Summary}
	for _, r := range steps {
		ps := proofStep{Description: r.Description, Query: r.Query, Language: r.Language, Source: r.Source, View: r.View, Payload: r.Payload}
		h.Steps = append(h.Steps, Step{ID: r.StepKey, Hash: ps.hash()})
	}
	return h, nil
}

// unitApply decides a valid unit: new, unchanged, accepted edit, pending edit or invalid for want of a reason.
func (s *Service) unitApply(ctx context.Context, req Request, slug string, doc proofDoc, read []unitFile, rows []model.TaskFile, byPath map[string]model.TaskFile) (FileResult, *PendingItem) {
	folder := proofFolder(slug)
	md := folder + ProofFile
	for _, f := range read { // blobs first: a crash before the commit leaves the files unaccepted
		if _, err := s.cfg.Proof.Blobs.Put(f.raw); err != nil {
			return FileResult{Path: md, Status: StatusInvalid, Message: fmt.Sprintf("%s: could not store the file: %v", f.rel, err)}, nil
		}
	}
	a := &unitTx{s: s, req: req, slug: slug, doc: doc, read: read, rows: rows, md: md,
		by: writerName(req.Session.Role, req.Session.Key), res: FileResult{Path: md}}
	a.steps, a.counts = proofInfo(doc, read)
	if err := s.cfg.Index.InTx(a.run); err != nil {
		return FileResult{Path: md, Status: StatusInvalid, Message: fmt.Sprintf("%s: could not apply: %v", md, err)}, nil
	}
	if a.review != nil {
		if pe, ok := s.cfg.Events.(ProofEvents); ok {
			if err := pe.ProofReview(ctx, req.TaskID, slug, *a.review); err != nil {
				slog.Warn("taskfiles: proof review", "scope", "ade", "task", req.TaskID, "slug", slug, "err", err)
			}
		}
	} else if a.changed != "" {
		s.proofChanged(ctx, req.TaskID, a.changed)
	}
	return a.res, nil
}

// proofChanged tells the proof windows that a proof's content changed.
func (s *Service) proofChanged(ctx context.Context, taskID, sessionID string) {
	pe, ok := s.cfg.Events.(ProofEvents)
	if !ok {
		return
	}
	if err := pe.ProofChanged(ctx, sessionID); err != nil {
		slog.Warn("taskfiles: proof changed", "scope", "ade", "task", taskID, "session", sessionID, "err", err)
	}
}

// unitTx is the write of one valid proof unit inside the index transaction.
type unitTx struct {
	s      *Service
	tx     Tx
	req    Request
	slug   string
	doc    proofDoc
	read   []unitFile
	rows   []model.TaskFile
	md, by string
	steps  []ProofStepInfo
	counts ProofCounts
	res    FileResult
	// review is set when the pending-review flag of the proof changed: true raised, false cleared.
	review *bool
	// changed is the session id whose content changed with no review flag change.
	changed string
}

func (a *unitTx) setReview(v bool) { a.review = &v }

func (a *unitTx) result(st Status, pr *ProofResult) {
	a.res = FileResult{Path: a.md, Status: st, Proof: pr}
}

func (a *unitTx) invalid(msg string) {
	a.res = FileResult{Path: a.md, Status: StatusInvalid, Message: msg, Proof: &ProofResult{Slug: a.slug, Name: a.doc.Name, Status: string(StatusInvalid), Problems: []string{msg}}}
}

func (a *unitTx) putRows(accept bool) error {
	return a.s.putUnitRows(a.tx, a.req, a.read, a.rows, accept, a.by)
}

func (a *unitTx) run(tx Tx) error {
	a.tx = tx
	pc := a.s.cfg.Proof
	q := tx.Querier()
	u, err := pc.Cache.ByTaskSlug(q, a.req.TaskID, a.slug)
	if err != nil {
		return err
	}
	if taken, err := a.nameTaken(u); err != nil || taken {
		return err
	}
	if u == nil {
		return a.create()
	}
	before, err := a.s.cacheHead(tx, u)
	if err != nil {
		return err
	}
	changes := DiffProof(before, a.doc.head())
	if len(changes) == 0 {
		return a.unchanged(u)
	}
	return a.edit(u, before, changes)
}

// nameTaken reports that another proof of the task holds the name, and records the invalid result.
func (a *unitTx) nameTaken(u *repos.ProofUnit) (bool, error) {
	pc := a.s.cfg.Proof
	q := a.tx.Querier()
	other, err := pc.Cache.NameTakenBy(q, a.req.TaskID, a.doc.Name)
	if err != nil || other == "" || (u != nil && other == u.Session.ID) {
		return false, err
	}
	otherSlug, err := pc.Cache.SlugOf(q, other)
	if err != nil {
		return false, err
	}
	a.invalid(problem{ProofFile, 0, fmt.Sprintf("name %q is used by %s. Pick another name or edit that proof.", a.doc.Name, proofFolder(otherSlug))}.text(proofFolder(a.slug)))
	return true, nil
}

func (a *unitTx) create() error {
	pc := a.s.cfg.Proof
	by, source := a.req.Session.Key, "agent"
	if a.req.Session.Role == RoleUser {
		by, source = "", "user"
	}
	id := pc.NewID()
	if err := pc.Cache.Create(a.tx.Querier(), repos.ProofNew{ID: id, TaskID: a.req.TaskID, Slug: a.slug, Name: a.doc.Name, Summary: a.doc.Summary, Steps: stepRows(a.doc), Now: a.s.nowMs(), By: by, Source: source}); err != nil {
		return err
	}
	if err := a.putRows(true); err != nil {
		return err
	}
	a.result(StatusAccepted, &ProofResult{Slug: a.slug, Name: a.doc.Name, Status: string(StatusAccepted), New: true, Steps: a.steps, Counts: a.counts})
	a.changed = id
	return nil
}

// unchanged covers a folder whose content equals the accepted proof; a waiting edit it undoes is dropped.
func (a *unitTx) unchanged(u *repos.ProofUnit) error {
	if u.Session.PendingRev != nil {
		if err := a.s.cfg.Proof.Cache.DropPending(a.tx.Querier(), u.Session.ID, a.s.nowMs()); err != nil {
			return err
		}
		a.setReview(false)
	}
	if err := a.putRows(true); err != nil {
		return err
	}
	a.result(StatusUnchanged, &ProofResult{Slug: a.slug, Name: a.doc.Name, Status: string(StatusAccepted), Steps: a.steps, Counts: a.counts})
	return nil
}

func (a *unitTx) propose(u *repos.ProofUnit, before ProofHead) error {
	return a.s.cfg.Proof.Cache.Propose(a.tx.Querier(), repos.ProofEdit{SessionID: u.Session.ID, Name: a.doc.Name, Summary: a.doc.Summary,
		Steps: diffRows(a.doc, before), Origin: originsOf(a.doc, before), Now: a.s.nowMs()})
}

func (a *unitTx) edit(u *repos.ProofUnit, before ProofHead, changes []Change) error {
	latest, err := a.s.cfg.Proof.Cache.LatestSlug(a.tx.Querier(), a.req.TaskID)
	if err != nil {
		return err
	}
	d := Decide(a.md, Input{Role: a.req.Session.Role, Kind: KindProof, Op: OpEdit, ProofAccepted: true, Latest: latest == a.slug, Author: proofAuthor(u, a.req.Session.Key)})
	pr := &ProofResult{Slug: a.slug, Name: a.doc.Name, Steps: a.steps, Counts: a.counts, Changed: changedOf(changes)}
	if d.Outcome == Pending {
		if err := a.propose(u, before); err != nil {
			return err
		}
		if err := a.putRows(false); err != nil {
			return err
		}
		a.setReview(true)
		pr.Status = string(StatusPending)
		a.result(StatusPending, pr)
		a.res.Message = pendingMessage(a.md, d.Cause, nil)
		return nil
	}
	// A user's edit: the revision is proposed and accepted at once, so view state of unchanged steps carries over.
	if err := a.propose(u, before); err != nil {
		return err
	}
	if err := a.s.cfg.Proof.Cache.Accept(a.tx.Querier(), u.Session.ID, a.s.cfg.Now()); err != nil {
		return err
	}
	if err := a.putRows(true); err != nil {
		return err
	}
	if u.Session.PendingRev != nil {
		a.setReview(false)
	}
	a.changed = u.Session.ID
	pr.Status = string(StatusAccepted)
	a.result(StatusAccepted, pr)
	return nil
}

// diffRows is stepRows with each step's difference to the accepted revision: unchanged, changed or added.
func diffRows(doc proofDoc, before ProofHead) []repos.ProofStepRow {
	hash := make(map[string]string, len(before.Steps))
	for _, st := range before.Steps {
		hash[st.ID] = st.Hash
	}
	rows := stepRows(doc)
	for i, st := range doc.Steps {
		switch h, ok := hash[st.ID]; {
		case !ok:
			rows[i].Diff = "added"
		case h == st.hash():
			rows[i].Diff = "unchanged"
		default:
			rows[i].Diff = "changed"
		}
	}
	return rows
}

// originsOf maps the index of each unchanged or changed step to the ordinal it descends from.
func originsOf(doc proofDoc, before ProofHead) map[int]int {
	pos := make(map[string]int, len(before.Steps))
	hash := make(map[string]string, len(before.Steps))
	for i, st := range before.Steps {
		pos[st.ID] = i + 1
		hash[st.ID] = st.Hash
	}
	out := map[int]int{}
	for i, st := range doc.Steps {
		if o, ok := pos[st.ID]; ok {
			out[i] = o
		}
	}
	return out
}

// putUnitRows records every file of the unit. accept moves the accepted copy to the new bytes; otherwise
// the accepted hash stays and the row waits. Rows of vanished files go on accept and wait otherwise.
func (s *Service) putUnitRows(tx Tx, req Request, read []unitFile, old []model.TaskFile, accept bool, by string) error {
	now := s.nowMs()
	have := map[string]bool{}
	oldBy := make(map[string]model.TaskFile, len(old))
	for _, r := range old {
		oldBy[r.RelPath] = r
	}
	for _, f := range read {
		have[f.rel] = true
		row, ok := oldBy[f.rel]
		if !ok {
			row = model.TaskFile{TaskID: req.TaskID, RelPath: f.rel}
		}
		row.Kind, row.Size, row.Version, row.SHA256 = string(Classify(f.rel).Kind), f.info.Size, string(f.info.Version), f.hash
		if accept {
			row.AcceptedSHA256, row.AcceptedBy, row.AcceptedAt, row.Review, row.Problem = f.hash, by, now, model.TaskFileAccepted, ""
		} else {
			row.Review, row.Problem = model.TaskFilePending, ""
		}
		if err := tx.PutFile(row); err != nil {
			return err
		}
	}
	for _, r := range old {
		if have[r.RelPath] {
			continue
		}
		if accept || r.AcceptedSHA256 == "" {
			if err := tx.DeleteFile(req.TaskID, r.RelPath); err != nil {
				return err
			}
			continue
		}
		r.SHA256, r.Version, r.Size, r.Review, r.Problem = "", "", 0, model.TaskFilePending, ""
		if err := tx.PutFile(r); err != nil {
			return err
		}
	}
	return nil
}

// unitGone handles a folder with rows and no files: the proof was deleted.
func (s *Service) unitGone(ctx context.Context, req Request, slug string, rows []model.TaskFile) (FileResult, *PendingItem) {
	pc := s.cfg.Proof
	folder := proofFolder(slug)
	md := folder + ProofFile
	if len(rows) == 0 {
		return FileResult{}, nil
	}
	var res FileResult
	var item *PendingItem
	var gone string
	err := s.cfg.Index.InTx(func(tx Tx) error {
		q := tx.Querier()
		u, err := pc.Cache.ByTaskSlug(q, req.TaskID, slug)
		if err != nil {
			return err
		}
		if u == nil { // rows without a proof: nothing to protect
			for _, r := range rows {
				if err := tx.DeleteFile(req.TaskID, r.RelPath); err != nil {
					return err
				}
			}
			res = FileResult{Path: md, Status: StatusAccepted}
			return nil
		}
		d := Decide(md, Input{Role: req.Session.Role, Kind: KindProof, Op: OpDelete, ProofAccepted: true, Author: proofAuthor(u, req.Session.Key)})
		pr := &ProofResult{Slug: slug, Name: u.Session.Name}
		if d.Outcome == Pending {
			for _, r := range rows {
				r.Review, r.Problem = model.TaskFilePending, ""
				if err := tx.PutFile(r); err != nil {
					return err
				}
			}
			pr.Status = string(StatusPending)
			pr.Changed = &ProofChanged{Fields: []string{"delete"}}
			msg := fmt.Sprintf("%s: the proof is deleted; waiting for the user (%s).", folder, causeText[d.Cause])
			res = FileResult{Path: md, Status: StatusPending, Message: msg, Proof: pr}
			item = &PendingItem{Path: md, Kind: KindProof, Cause: CauseProofDelete, Changes: []Change{{Op: OpDelete, Text: "delete proof " + u.Session.Name}}}
			return nil
		}
		if err := s.deleteProofTx(tx, req.TaskID, slug, u); err != nil {
			return err
		}
		gone = u.Session.ID
		pr.Status = string(StatusAccepted)
		res = FileResult{Path: md, Status: StatusAccepted, Proof: pr}
		return nil
	})
	if err != nil {
		return FileResult{Path: md, Status: StatusInvalid, Message: fmt.Sprintf("%s: could not apply: %v", md, err)}, nil
	}
	if gone != "" {
		s.proofChanged(ctx, req.TaskID, gone)
	}
	return res, item
}

// deleteProofTx drops the cache and index rows of a proof folder.
func (s *Service) deleteProofTx(tx Tx, taskID, slug string, u *repos.ProofUnit) error {
	if u != nil {
		if err := s.cfg.Proof.Cache.Delete(tx.Querier(), u.Session.ID); err != nil {
			return err
		}
	}
	rows, err := tx.FilesUnder(taskID, proofFolder(slug))
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := tx.DeleteFile(taskID, r.RelPath); err != nil {
			return err
		}
	}
	return nil
}

// DeleteProof finishes an approved delete: the cache and rows of the folder go, and any file the agent
// wrote back since.
func (s *Service) DeleteProof(ctx context.Context, taskID, slug string) error {
	if s.cfg.Proof == nil {
		return fmt.Errorf("%w: proof", ErrNoHandler)
	}
	st, err := s.cfg.Open(taskID)
	if err != nil {
		return err
	}
	if err := st.Remove(ctx, strings.TrimSuffix(proofFolder(slug), "/")); err != nil && !errors.Is(err, store.ErrNotExist) {
		return err
	}
	return s.cfg.Index.InTx(func(tx Tx) error {
		u, err := s.cfg.Proof.Cache.ByTaskSlug(tx.Querier(), taskID, slug)
		if err != nil {
			return err
		}
		return s.deleteProofTx(tx, taskID, slug, u)
	})
}

// RestoreProof puts a proof folder back to its accepted state from the blob store: the pending revision
// is dropped, each accepted file is rewritten byte for byte and each file that was never accepted is removed.
func (s *Service) RestoreProof(ctx context.Context, taskID, slug string) error {
	if s.cfg.Proof == nil {
		return fmt.Errorf("%w: proof", ErrNoHandler)
	}
	var rows []model.TaskFile
	var dropped string
	err := s.cfg.Index.InTx(func(tx Tx) error {
		var err error
		if rows, err = tx.FilesUnder(taskID, proofFolder(slug)); err != nil {
			return err
		}
		if u, err := s.cfg.Proof.Cache.ByTaskSlug(tx.Querier(), taskID, slug); err != nil {
			return err
		} else if u != nil {
			if u.Session.PendingRev != nil {
				dropped = u.Session.ID
			}
			return s.cfg.Proof.Cache.DropPending(tx.Querier(), u.Session.ID, s.nowMs())
		}
		return nil
	})
	if err != nil {
		return err
	}
	if dropped != "" {
		s.proofChanged(ctx, taskID, dropped)
	}
	for _, r := range rows {
		if r.AcceptedSHA256 == "" {
			if err := s.Delete(ctx, taskID, r.RelPath, string(RoleKira), nil); err != nil {
				return err
			}
			continue
		}
		data, ok, err := s.cfg.Proof.Blobs.Get(r.AcceptedSHA256)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("taskfiles: accepted copy of %s is missing", r.RelPath)
		}
		if err := s.Write(ctx, taskID, r.RelPath, data, string(RoleKira), nil); err != nil {
			return err
		}
	}
	return nil
}

// AcceptProofFiles marks the files of a proof folder accepted at the bytes of their pending version, once
// the proof's pending revision became current. Files the edit removed lose their rows.
func (s *Service) AcceptProofFiles(taskID, slug, by string) error {
	return s.cfg.Index.InTx(func(tx Tx) error {
		rows, err := tx.FilesUnder(taskID, proofFolder(slug))
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.SHA256 == "" {
				if err := tx.DeleteFile(taskID, r.RelPath); err != nil {
					return err
				}
				continue
			}
			r.AcceptedSHA256, r.AcceptedBy, r.AcceptedAt, r.Review, r.Problem = r.SHA256, by, s.nowMs(), model.TaskFileAccepted, ""
			if err := tx.PutFile(r); err != nil {
				return err
			}
		}
		return nil
	})
}

// adoptProof makes the files on disk the accepted proof: the user's consent to the folder as it is.
func (s *Service) adoptProof(ctx context.Context, taskID, slug, by string) error {
	st, err := s.cfg.Open(taskID)
	if err != nil {
		return err
	}
	rows, err := s.cfg.Index.Files(taskID)
	if err != nil {
		return err
	}
	byPath := make(map[string]model.TaskFile, len(rows))
	for _, r := range rows {
		byPath[r.RelPath] = r
	}
	entries, err := st.List(ctx, store.ListOptions{MaxDepth: MaxDepth, MaxFiles: MaxFilesPerTask})
	if err != nil && !errors.Is(err, store.ErrTooManyFiles) {
		return err
	}
	present := make(map[string]store.Entry, len(entries))
	for _, e := range entries {
		present[e.Rel] = e
	}
	req := Request{TaskID: taskID, Session: Session{Key: by, Role: RoleUser}}
	fr, _ := s.unit(ctx, st, req, slug, present, byPath, s.sizeProblems(entries))
	if fr.Status == StatusInvalid {
		return errors.New(fr.Message)
	}
	return nil
}
