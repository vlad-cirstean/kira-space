package repos

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// Sentinels AdeTaskRepo returns; bridge maps them to E_BAD_REQUEST/E_NOT_FOUND.
var (
	ErrTaskNotFound = errors.New("repos: task not found")
	// ErrTaskNotArchived marks an archived-only call on a live task.
	ErrTaskNotArchived = errors.New("repos: task is not archived")
	// ErrStatusUnknown marks a status id missing from the status set.
	ErrStatusUnknown = errors.New("repos: unknown task status")
	// ErrShelfTarget marks a Shelve beforeID that is not on the target shelf.
	ErrShelfTarget   = errors.New("repos: before task is not on that shelf")
	ErrBranchOnTask  = errors.New("repos: branch is already on a task")
	ErrDraftInRepo   = errors.New("repos: task already has a branch slot waiting for a name in that repo")
	ErrReviewKind    = errors.New("repos: a task only moves between task and parked, and a review task only becomes a task")
	ErrTaskUnnamed   = errors.New("a task needs a title or a Jira key")
	ErrBranchMissing = errors.New("repos: branch not found")
	ErrRunNotFound   = errors.New("repos: run not found")
	// ErrBranchNameTaken marks a created-branch name already used by a live branch of the repo.
	ErrBranchNameTaken = errors.New("repos: a live branch in that repo already has that name")
)

// BranchHeldError is RestoreTask's refusal: a named branch of the task is live on another task. It
// wraps ErrBranchOnTask.
type BranchHeldError struct {
	Branch    string
	TaskTitle string
}

func (e *BranchHeldError) Error() string {
	return fmt.Sprintf("repos: branch %s is live on task %q", e.Branch, e.TaskTitle)
}

func (e *BranchHeldError) Unwrap() error { return ErrBranchOnTask }

// maxColorSlots is the task colour slot count (0-19).
const maxColorSlots = 20

// MaxEstHours and MaxEstDays bound an estimate: a longer one makes the Plan span thousands of days.
const (
	MaxEstHours = 480
	MaxEstDays  = 60
)

// EstOverCap reports an estimate above the bound.
func EstOverCap(est string) bool {
	if est == "" {
		return false
	}
	n, err := strconv.ParseFloat(est[:len(est)-1], 64)
	if err != nil {
		return false
	}
	if est[len(est)-1] == 'h' {
		return n > MaxEstHours
	}
	return n > MaxEstDays
}

// Column lists in scan order.
var (
	adeTaskColumns = []string{
		kiraschema.AdeTasks.ID, kiraschema.AdeTasks.Kind, kiraschema.AdeTasks.Title, kiraschema.AdeTasks.Owner,
		kiraschema.AdeTasks.JiraKey, kiraschema.AdeTasks.JiraURL, kiraschema.AdeTasks.HostURL, kiraschema.AdeTasks.WorkflowID,
		kiraschema.AdeTasks.EntryStepID, kiraschema.AdeTasks.SinceJSON, kiraschema.AdeTasks.WorkflowJSON,
		kiraschema.AdeTasks.WorkflowHash, kiraschema.AdeTasks.Est, kiraschema.AdeTasks.Color, kiraschema.AdeTasks.CreatedAt,
		kiraschema.AdeTasks.ArchivedAt, kiraschema.AdeTasks.StatusID, kiraschema.AdeTasks.StatusBy, kiraschema.AdeTasks.StatusStep,
	}
	adeTaskBranchInsertColumns = []string{
		kiraschema.AdeTaskBranches.ID, kiraschema.AdeTaskBranches.TaskID, kiraschema.AdeTaskBranches.CodeRepoID,
		kiraschema.AdeTaskBranches.Name, kiraschema.AdeTaskBranches.Kind, kiraschema.AdeTaskBranches.Base,
		kiraschema.AdeTaskBranches.BaseBranchID, kiraschema.AdeTaskBranches.BasePendingFrom,
		kiraschema.AdeTaskBranches.QueuedAfter, kiraschema.AdeTaskBranches.Position, kiraschema.AdeTaskBranches.HadCommits,
		kiraschema.AdeTaskBranches.AddedAt, kiraschema.AdeTaskBranches.MergedAt, kiraschema.AdeTaskBranches.ArchivedAt,
		kiraschema.AdeTaskBranches.Origin,
	}
	adeTaskBranchColumns = append(slices.Clone(adeTaskBranchInsertColumns),
		kiraschema.AdeTaskBranches.CrNumber, kiraschema.AdeTaskBranches.CrTitle, kiraschema.AdeTaskBranches.CrState,
		kiraschema.AdeTaskBranches.CrURL, kiraschema.AdeTaskBranches.ReviewComments, kiraschema.AdeTaskBranches.WorktreeDir)
	adeRunColumns = []string{
		kiraschema.AdeRuns.ID, kiraschema.AdeRuns.TaskID, kiraschema.AdeRuns.StepID, kiraschema.AdeRuns.BranchID,
		kiraschema.AdeRuns.Attempt, kiraschema.AdeRuns.State, kiraschema.AdeRuns.Loops, kiraschema.AdeRuns.Note,
		kiraschema.AdeRuns.Summary, kiraschema.AdeRuns.SessionID, kiraschema.AdeRuns.ExitCode, kiraschema.AdeRuns.StartedAt,
		kiraschema.AdeRuns.FinishedAt, kiraschema.AdeRuns.OutcomeJSON, kiraschema.AdeRuns.Purpose, kiraschema.AdeRuns.SpecJSON,
		kiraschema.AdeRuns.LaunchNote, kiraschema.AdeRuns.LaunchResumeID, kiraschema.AdeRuns.LaunchPrompt, kiraschema.AdeRuns.LaunchExtra,
	}
)

func (r *AdeTaskRepo) selectTask() *sqlbuilder.SelectBuilder {
	return r.Dialect.Select(adeTaskColumns...).From(kiraschema.AdeTasks.Table)
}

func (r *AdeTaskRepo) selectRuns() *sqlbuilder.SelectBuilder {
	return r.Dialect.Select(adeRunColumns...).From(kiraschema.AdeRuns.Table)
}

// AdeTaskRepo reads and writes the v2 task store: tasks, branches, plan, runs, worktree setup and
// last-valid workflows.
type AdeTaskRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

func nullInt64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func nullIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func scanAdeTask(row rowScanner) (model.AdeTask, error) {
	var t model.AdeTask
	var since, workflow sql.NullString
	var archived sql.NullInt64
	if err := row.Scan(&t.ID, &t.Kind, &t.Title, &t.Owner, &t.JiraKey, &t.JiraURL, &t.HostURL, &t.WorkflowID,
		&t.EntryStepID, &since, &workflow, &t.WorkflowHash, &t.Est, &t.Color, &t.CreatedAt, &archived, &t.StatusID, &t.StatusBy, &t.StatusStep); err != nil {
		return model.AdeTask{}, err
	}
	t.SinceJSON = since.String
	t.WorkflowJSON = workflow.String
	t.ArchivedAt = nullInt64Ptr(archived)
	return t, nil
}

func scanAdeTaskBranch(row rowScanner) (model.AdeTaskBranch, error) {
	var b model.AdeTaskBranch
	var had int
	var merged, archived, crNumber sql.NullInt64
	var crTitle, crState, crURL, comments sql.NullString
	if err := row.Scan(&b.ID, &b.TaskID, &b.CodeRepoID, &b.Name, &b.Kind, &b.Base, &b.BaseBranchID, &b.BasePendingFrom, &b.QueuedAfter, &b.Position,
		&had, &b.AddedAt, &merged, &archived, &b.Origin, &crNumber, &crTitle, &crState, &crURL, &comments, &b.WorktreeDir); err != nil {
		return model.AdeTaskBranch{}, err
	}
	if crNumber.Valid {
		b.ChangeRequest = &model.AdeChangeRequest{Number: int(crNumber.Int64), Title: crTitle.String, State: crState.String, URL: crURL.String}
	}
	b.ReviewCommentsJSON = comments.String
	b.HadCommits = had != 0
	b.MergedAt = nullInt64Ptr(merged)
	b.ArchivedAt = nullInt64Ptr(archived)
	return b, nil
}

func (r *AdeTaskRepo) queryTasks(b sqlb.Builder) ([]model.AdeTask, error) {
	rows, err := sqlb.Query(r.DB, b)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade tasks: %w", err)
	}
	defer rows.Close()
	out := make([]model.AdeTask, 0)
	for rows.Next() {
		t, err := scanAdeTask(rows)
		if err != nil {
			return nil, fmt.Errorf("repos: scan ade task: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade tasks rows: %w", err)
	}
	return out, nil
}

// ListLive returns non-archived tasks in plan order (a task with no plan row sorts last).
func (r *AdeTaskRepo) ListLive() ([]model.AdeTask, error) {
	t, p := kiraschema.AdeTasks, kiraschema.AdeTaskPlan
	sb := r.Dialect.Select(adeQual("t", adeTaskColumns)...).From(t.Table + " t")
	sb.JoinWithOption(sqlbuilder.LeftJoin, p.Table+" p", "p."+p.TaskID+" = t."+t.ID)
	sb.Where(sb.IsNull("t."+t.ArchivedAt)).
		OrderBy("p."+p.Position+" IS NULL", "p."+p.Position, "t."+t.CreatedAt, "t."+t.ID)
	return r.queryTasks(sb)
}

// ListArchived returns archived tasks, newest archive first.
func (r *AdeTaskRepo) ListArchived() ([]model.AdeTask, error) {
	c := kiraschema.AdeTasks
	sb := r.selectTask()
	sb.Where(sb.IsNotNull(c.ArchivedAt)).OrderBy(c.ArchivedAt+" DESC", c.ID)
	return r.queryTasks(sb)
}

// GetTask returns one task, live or archived; ErrTaskNotFound if absent.
func (r *AdeTaskRepo) GetTask(id string) (model.AdeTask, error) {
	sb := r.selectTask()
	sb.Where(sb.Equal(kiraschema.AdeTasks.ID, id))
	t, err := scanAdeTask(sqlb.QueryRow(r.DB, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdeTask{}, ErrTaskNotFound
	}
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: get ade task %s: %w", id, err)
	}
	return t, nil
}

// insertTask writes the task, its branches and a plan row on `day` (Later when nil), in tx: at the end of
// the order, or at the top when top is set, plus the first user note when given. It assigns the task's color: the first slot no live task uses,
// else the least-used slot overall.
func insertTask(tx *sql.Tx, d sqlb.Dialect, t model.AdeTask, branches []model.AdeTaskBranch, day *string, top bool, note *model.AdeHandover) (model.AdeTask, error) {
	if err := t.Validate(); err != nil {
		return model.AdeTask{}, err
	}
	rows, err := sqlb.Query(tx, d.Select(kiraschema.AdeTasks.Color, kiraschema.AdeTasks.ArchivedAt+" IS NULL").From(kiraschema.AdeTasks.Table))
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: query ade task colors: %w", err)
	}
	visible := make(map[int]bool)
	count := make(map[int]int)
	for rows.Next() {
		var slot int
		var live bool
		if err := rows.Scan(&slot, &live); err != nil {
			rows.Close()
			return model.AdeTask{}, fmt.Errorf("repos: scan ade task color: %w", err)
		}
		count[slot]++
		if live {
			visible[slot] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return model.AdeTask{}, fmt.Errorf("repos: ade task colors rows: %w", err)
	}
	rows.Close()
	t.Color = pickColorSlot(visible, count)

	var since any
	if t.SinceJSON != "" {
		since = t.SinceJSON
	}
	if t.StatusID == "" {
		id, err := statusForNew(tx, d, t.Kind == model.AdeTaskKindReview)
		if err != nil {
			return model.AdeTask{}, err
		}
		t.StatusID = id
	}
	if t.StatusBy == "" {
		t.StatusBy = model.AdeStatusByAuto
	}
	c := kiraschema.AdeTasks
	ib := d.InsertInto(c.Table).Cols(c.ID, c.Kind, c.Title, c.Owner, c.JiraKey, c.JiraURL, c.HostURL, c.WorkflowID, c.EntryStepID,
		c.SinceJSON, c.Est, c.Color, c.CreatedAt, c.StatusID, c.StatusBy, c.StatusStep).
		Values(t.ID, t.Kind, t.Title, t.Owner, t.JiraKey, t.JiraURL, t.HostURL, t.WorkflowID, t.EntryStepID, since, t.Est,
			t.Color, t.CreatedAt, t.StatusID, t.StatusBy, t.StatusStep)
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: insert ade task %s: %w", t.ID, err)
	}
	for _, b := range branches {
		if err := insertTaskBranch(tx, d, b); err != nil {
			return model.AdeTask{}, err
		}
	}
	if note != nil {
		if err := insertNote(tx, d, *note); err != nil {
			return model.AdeTask{}, err
		}
	}
	pl := kiraschema.AdeTaskPlan
	position := "COALESCE(MAX(" + pl.Position + ") + 1, 0)"
	if top {
		position = "COALESCE(MIN(" + pl.Position + ") - 1, 0)"
	}
	sel := d.Select()
	sel.Select(sel.Var(t.ID), sel.Var(day), position).From(pl.Table)
	if _, err := sqlb.Exec(tx, sqlb.AdeInsertSelect(d, pl.Table, []string{pl.TaskID, pl.Day, pl.Position}, sel, "")); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: insert ade task plan %s: %w", t.ID, err)
	}
	return t, nil
}

func pickColorSlot(visible map[int]bool, count map[int]int) int {
	for slot := 0; slot < maxColorSlots; slot++ {
		if !visible[slot] {
			return slot
		}
	}
	best, bestCount := 0, -1
	for slot := 0; slot < maxColorSlots; slot++ {
		if bestCount == -1 || count[slot] < bestCount {
			best, bestCount = slot, count[slot]
		}
	}
	return best
}

func insertTaskBranch(tx *sql.Tx, d sqlb.Dialect, b model.AdeTaskBranch) error {
	if err := b.Validate(); err != nil {
		return err
	}
	c := kiraschema.AdeTaskBranches
	ib := d.InsertInto(c.Table).Cols(c.ID, c.TaskID, c.CodeRepoID, c.Name, c.Kind, c.Base, c.BaseBranchID, c.BasePendingFrom,
		c.QueuedAfter, c.Position, c.HadCommits, c.AddedAt, c.Origin).
		Values(b.ID, b.TaskID, b.CodeRepoID, b.Name, b.Kind, b.Base, b.BaseBranchID, b.BasePendingFrom, b.QueuedAfter,
			b.Position, boolInt(b.HadCommits), b.AddedAt, b.Origin)
	if _, err := sqlb.Exec(tx, ib); err != nil {
		if isUniqueViolation(err) {
			return ErrBranchOnTask
		}
		return fmt.Errorf("repos: insert ade task branch %s: %w", b.ID, err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// CreateTask inserts a task, its branches and a plan row at the end of `day` (Later when nil) in
// one transaction, with the first user note when note is not nil, and returns the stored task (color assigned).
func (r *AdeTaskRepo) CreateTask(t model.AdeTask, branches []model.AdeTaskBranch, day *string, note *model.AdeHandover) (model.AdeTask, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: begin create ade task: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	out, err := insertTask(tx, r.Dialect, t, branches, day, false, note)
	if err != nil {
		return model.AdeTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: commit create ade task: %w", err)
	}
	return out, nil
}

// CaptureTask inserts a branchless task at the top of Later (nil day) or of the day, and returns
// the stored task.
func (r *AdeTaskRepo) CaptureTask(t model.AdeTask, day *string) (model.AdeTask, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: begin capture ade task: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	out, err := insertTask(tx, r.Dialect, t, nil, day, true, nil)
	if err != nil {
		return model.AdeTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: commit capture ade task: %w", err)
	}
	return out, nil
}

// UpdateTask writes the patched leaves. Kind moves only between task and parked; a review task
// only becomes a task, clearing its owner and turning its review branches into mine.
func (r *AdeTaskRepo) UpdateTask(id string, p model.AdeTaskPatch) (model.AdeTask, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: begin update ade task: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	curQ := r.selectTask()
	curQ.Where(curQ.Equal(kiraschema.AdeTasks.ID, id))
	cur, err := scanAdeTask(sqlb.QueryRow(tx, curQ))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdeTask{}, ErrTaskNotFound
	}
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: select ade task %s: %w", id, err)
	}
	prevKind := cur.Kind
	if err := checkStatusExists(tx, r.Dialect, p.StatusID); err != nil {
		return model.AdeTask{}, err
	}
	if p.Kind != nil && *p.Kind != cur.Kind {
		if err := moveTaskKind(&cur, *p.Kind); err != nil {
			return model.AdeTask{}, err
		}
	}
	if prevKind == model.AdeTaskKindReview && cur.Kind == model.AdeTaskKindTask {
		id, err := statusForNew(tx, r.Dialect, false)
		if err != nil {
			return model.AdeTask{}, err
		}
		cur.StatusID, cur.StatusBy, cur.StatusStep = id, model.AdeStatusByAuto, ""
	}
	if p.StatusID != nil {
		cur.StatusID, cur.StatusBy = *p.StatusID, model.AdeStatusByUser
	}
	if err := patchLeaves(&cur, p); err != nil {
		return model.AdeTask{}, err
	}
	// A branch-derived task may start with neither; a patch must not strip the last of them.
	if (p.Title != nil || p.JiraKey != nil) && cur.Title == "" && cur.JiraKey == "" {
		return model.AdeTask{}, ErrTaskUnnamed
	}
	if cur.Kind != prevKind {
		if err := moveBranchKinds(tx, r.Dialect, id, prevKind, cur.Kind); err != nil {
			return model.AdeTask{}, err
		}
	}
	c := kiraschema.AdeTasks
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Kind, cur.Kind), ub.Assign(c.Owner, cur.Owner), ub.Assign(c.Title, cur.Title), ub.Assign(c.JiraKey, cur.JiraKey),
		ub.Assign(c.JiraURL, cur.JiraURL), ub.Assign(c.HostURL, cur.HostURL), ub.Assign(c.Est, cur.Est), ub.Assign(c.Color, cur.Color),
		ub.Assign(c.StatusID, cur.StatusID), ub.Assign(c.StatusBy, cur.StatusBy), ub.Assign(c.StatusStep, cur.StatusStep)).
		Where(ub.Equal(c.ID, id))
	if _, err := sqlb.Exec(tx, ub); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: update ade task %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: commit update ade task %s: %w", id, err)
	}
	return cur, nil
}

// checkStatusExists refuses a status id outside the set; nil passes.
func checkStatusExists(tx *sql.Tx, d sqlb.Dialect, statusID *string) error {
	if statusID == nil {
		return nil
	}
	s := kiraschema.AdeTaskStatuses
	n, err := adeCount(tx, d, s.Table, func(sb *sqlbuilder.SelectBuilder) []string { return []string{sb.Equal(s.ID, *statusID)} })
	if err != nil {
		return fmt.Errorf("repos: check ade task status %s: %w", *statusID, err)
	}
	if n == 0 {
		return ErrStatusUnknown
	}
	return nil
}

// patchLeaves applies the plain text leaves of p to cur.
func patchLeaves(cur *model.AdeTask, p model.AdeTaskPatch) error {
	if p.Est != nil {
		cur.Est = *p.Est
	}
	if p.Title != nil {
		cur.Title = *p.Title
	}
	if p.JiraKey != nil {
		cur.JiraKey = *p.JiraKey
	}
	if p.JiraURL != nil {
		cur.JiraURL = *p.JiraURL
	}
	if p.HostURL != nil {
		cur.HostURL = *p.HostURL
	}
	if p.Color != nil {
		if *p.Color < 0 || *p.Color >= maxColorSlots {
			return fmt.Errorf("repos: color %d out of range", *p.Color)
		}
		cur.Color = *p.Color
	}
	return nil
}

// moveTaskKind sets cur's kind: task <-> parked, or review -> task (which clears the owner).
func moveTaskKind(cur *model.AdeTask, to string) error {
	toTask := to == model.AdeTaskKindTask
	if (cur.Kind == model.AdeTaskKindReview && !toTask) || (!toTask && to != model.AdeTaskKindParked) {
		return ErrReviewKind
	}
	if cur.Kind == model.AdeTaskKindReview {
		cur.Owner = ""
	}
	cur.Kind = to
	return nil
}

// moveBranchKinds flips a task's branches to follow its kind move.
func moveBranchKinds(tx *sql.Tx, d sqlb.Dialect, id string, prev, to string) error {
	from, toKind := model.AdeBranchKindMine, model.AdeBranchKindParked
	if to == model.AdeTaskKindTask {
		from, toKind = model.AdeBranchKindParked, model.AdeBranchKindMine
		if prev == model.AdeTaskKindReview {
			from = model.AdeBranchKindReview
		}
	}
	c := kiraschema.AdeTaskBranches
	ub := d.Update(c.Table)
	ub.Set(ub.Assign(c.Kind, toKind)).Where(ub.Equal(c.TaskID, id), ub.Equal(c.Kind, from))
	if _, err := sqlb.Exec(tx, ub); err != nil {
		return fmt.Errorf("repos: move ade task branches to %s: %w", toKind, err)
	}
	return nil
}

// AddBranch appends b to its task after the last branch. A task holds at most one not-created
// (draft) live branch per repo (ErrDraftInRepo); a created branch name is unique among live branches of the repo
// (ErrBranchOnTask). b.Position is overwritten.
func (r *AdeTaskRepo) AddBranch(b model.AdeTaskBranch) (model.AdeTaskBranch, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: begin add ade task branch: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	tk, bc := kiraschema.AdeTasks, kiraschema.AdeTaskBranches
	exists, err := adeCount(tx, r.Dialect, tk.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(tk.ID, b.TaskID), sb.IsNull(tk.ArchivedAt)}
	})
	if err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: check ade task %s: %w", b.TaskID, err)
	}
	if exists == 0 {
		return model.AdeTaskBranch{}, ErrTaskNotFound
	}
	if b.Name == "" {
		drafts, err := adeCount(tx, r.Dialect, bc.Table, func(sb *sqlbuilder.SelectBuilder) []string {
			return []string{sb.Equal(bc.TaskID, b.TaskID), sb.Equal(bc.CodeRepoID, b.CodeRepoID), sb.Equal(bc.Name, ""), sb.IsNull(bc.ArchivedAt)}
		})
		if err != nil {
			return model.AdeTaskBranch{}, fmt.Errorf("repos: check ade task repo drafts: %w", err)
		}
		if drafts > 0 {
			return model.AdeTaskBranch{}, ErrDraftInRepo
		}
	}
	next := r.Dialect.Select("COALESCE(MAX(" + bc.Position + ") + 1, 0)").From(bc.Table)
	next.Where(next.Equal(bc.TaskID, b.TaskID))
	if err := sqlb.QueryRow(tx, next).Scan(&b.Position); err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: next ade branch position: %w", err)
	}
	if err := insertTaskBranch(tx, r.Dialect, b); err != nil {
		return model.AdeTaskBranch{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: commit add ade task branch: %w", err)
	}
	return b, nil
}

// DetachBranch takes a live branch off its live task in one transaction. A branch never created and
// without runs is deleted; any other is archived, so its runs, reviews and marks stay as history.
// Without a replacement, live branches stacked on it fall back to its name (its ref stays in git) or,
// while it is not created, to its own base, and branches queued after it are freed. A non-nil
// replacement takes the old position (ErrBranchOnTask on a taken name); one filling a not-created row
// of the same repo also takes the dependents.
func (r *AdeTaskRepo) DetachBranch(id string, replacement *model.AdeTaskBranch, now int64) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin detach ade branch %s: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck
	bc := kiraschema.AdeTaskBranches
	oldQ := r.Dialect.Select(adeTaskBranchColumns...).From(bc.Table)
	oldQ.Where(oldQ.Equal(bc.ID, id), oldQ.IsNull(bc.ArchivedAt), oldQ.In(bc.TaskID, adeLiveTaskIDs(r.Dialect)))
	old, err := scanAdeTaskBranch(sqlb.QueryRow(tx, oldQ))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBranchMissing
	}
	if err != nil {
		return fmt.Errorf("repos: load ade branch %s: %w", id, err)
	}
	runs, err := adeCount(tx, r.Dialect, kiraschema.AdeRuns.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(kiraschema.AdeRuns.BranchID, id)}
	})
	if err != nil {
		return fmt.Errorf("repos: count ade runs of branch %s: %w", id, err)
	}
	if old.Name == "" && runs == 0 {
		del := r.Dialect.DeleteFrom(bc.Table)
		del.Where(del.Equal(bc.ID, id))
		_, err = sqlb.Exec(tx, del)
	} else {
		ub := r.Dialect.Update(bc.Table)
		ub.Set(ub.Assign(bc.ArchivedAt, now)).Where(ub.Equal(bc.ID, id))
		_, err = sqlb.Exec(tx, ub)
	}
	if err != nil {
		return fmt.Errorf("repos: detach ade branch %s: %w", id, err)
	}
	fill := replacement != nil && old.Name == "" && replacement.CodeRepoID == old.CodeRepoID
	if replacement != nil {
		replacement.Position = old.Position
		if err := insertTaskBranch(tx, r.Dialect, *replacement); err != nil {
			return err
		}
	}
	if fill {
		// The replacement fills the same not-created row, so what depended on the row keeps depending on it.
		stackedUp := r.Dialect.Update(bc.Table)
		stackedUp.Set(stackedUp.Assign(bc.BaseBranchID, replacement.ID)).Where(stackedUp.Equal(bc.BaseBranchID, id), stackedUp.IsNull(bc.ArchivedAt))
		if _, err := sqlb.Exec(tx, stackedUp); err != nil {
			return fmt.Errorf("repos: repoint ade branches stacked on %s: %w", id, err)
		}
		queuedUp := r.Dialect.Update(bc.Table)
		queuedUp.Set(queuedUp.Assign(bc.QueuedAfter, replacement.ID)).Where(queuedUp.Equal(bc.QueuedAfter, id))
		if _, err := sqlb.Exec(tx, queuedUp); err != nil {
			return fmt.Errorf("repos: repoint ade branches queued after %s: %w", id, err)
		}
	} else {
		newBase := cmp.Or(old.Name, old.Base)
		stackedUp := r.Dialect.Update(bc.Table)
		stackedUp.Set(stackedUp.Assign(bc.BaseBranchID, ""), stackedUp.Assign(bc.Base, newBase), stackedUp.Assign(bc.BasePendingFrom, "")).
			Where(stackedUp.Equal(bc.BaseBranchID, id), stackedUp.IsNull(bc.ArchivedAt))
		if _, err := sqlb.Exec(tx, stackedUp); err != nil {
			return fmt.Errorf("repos: free ade branches stacked on %s: %w", id, err)
		}
		queuedUp := r.Dialect.Update(bc.Table)
		queuedUp.Set(queuedUp.Assign(bc.QueuedAfter, "")).Where(queuedUp.Equal(bc.QueuedAfter, id))
		if _, err := sqlb.Exec(tx, queuedUp); err != nil {
			return fmt.Errorf("repos: free ade branches queued after %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit detach ade branch %s: %w", id, err)
	}
	return nil
}

func (r *AdeTaskRepo) queryBranches(where func(sb *sqlbuilder.SelectBuilder) []string) ([]model.AdeTaskBranch, error) {
	b, t := kiraschema.AdeTaskBranches, kiraschema.AdeTasks
	sb := r.Dialect.Select(adeQual("b", adeTaskBranchColumns)...).From(b.Table + " b")
	sb.Join(t.Table+" t", "t."+t.ID+" = b."+b.TaskID)
	sb.Where(where(sb)...).OrderBy("b."+b.TaskID, "b."+b.Position, "b."+b.ID)
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade task branches: %w", err)
	}
	defer rows.Close()
	out := make([]model.AdeTaskBranch, 0)
	for rows.Next() {
		b, err := scanAdeTaskBranch(rows)
		if err != nil {
			return nil, fmt.Errorf("repos: scan ade task branch: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade task branches rows: %w", err)
	}
	return out, nil
}

// BranchesLive returns the non-archived branches of live tasks.
func (r *AdeTaskRepo) BranchesLive() ([]model.AdeTaskBranch, error) {
	return r.queryBranches(func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.IsNull("t." + kiraschema.AdeTasks.ArchivedAt), sb.IsNull("b." + kiraschema.AdeTaskBranches.ArchivedAt)}
	})
}

// BranchesArchived returns the branches of archived tasks (history).
func (r *AdeTaskRepo) BranchesArchived() ([]model.AdeTaskBranch, error) {
	return r.queryBranches(func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.IsNotNull("t." + kiraschema.AdeTasks.ArchivedAt)}
	})
}

// BranchesOfTask returns every branch row of one task, archived or not, in position order.
func (r *AdeTaskRepo) BranchesOfTask(taskID string) ([]model.AdeTaskBranch, error) {
	return r.queryBranches(func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal("b."+kiraschema.AdeTaskBranches.TaskID, taskID)}
	})
}

// GetBranch returns one branch by id; ErrBranchMissing if absent.
func (r *AdeTaskRepo) GetBranch(id string) (model.AdeTaskBranch, error) {
	sb := r.Dialect.Select(adeTaskBranchColumns...).From(kiraschema.AdeTaskBranches.Table)
	sb.Where(sb.Equal(kiraschema.AdeTaskBranches.ID, id))
	b, err := scanAdeTaskBranch(sqlb.QueryRow(r.DB, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdeTaskBranch{}, ErrBranchMissing
	}
	if err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: get ade task branch %s: %w", id, err)
	}
	return b, nil
}

// PlanRows returns every plan row in position order.
func (r *AdeTaskRepo) PlanRows() ([]model.AdeTaskPlanRow, error) {
	pl := kiraschema.AdeTaskPlan
	sb := r.Dialect.Select(pl.TaskID, pl.Day, pl.Position, pl.Backlog).From(pl.Table).OrderBy(pl.Position, pl.TaskID)
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade task plan: %w", err)
	}
	defer rows.Close()
	out := make([]model.AdeTaskPlanRow, 0)
	for rows.Next() {
		var p model.AdeTaskPlanRow
		var day sql.NullString
		if err := rows.Scan(&p.TaskID, &day, &p.Position, &p.Backlog); err != nil {
			return nil, fmt.Errorf("repos: scan ade task plan: %w", err)
		}
		if day.Valid {
			v := day.String
			p.Day = &v
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade task plan rows: %w", err)
	}
	return out, nil
}

// SetPlan rewrites plan positions densely in order (plan rows not in order keep their relative
// position after it) and sets the day of each key in days (nil = Later).
func (r *AdeTaskRepo) SetPlan(order []string, days map[string]*string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin set ade task plan: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	pl := kiraschema.AdeTaskPlan
	rows, err := sqlb.Query(tx, r.Dialect.Select(pl.TaskID).From(pl.Table).OrderBy(pl.Position, pl.TaskID))
	if err != nil {
		return fmt.Errorf("repos: query ade task plan: %w", err)
	}
	inOrder := make(map[string]bool, len(order))
	for _, id := range order {
		inOrder[id] = true
	}
	final := append(make([]string, 0, len(order)), order...)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("repos: scan ade task plan: %w", err)
		}
		if !inOrder[id] {
			final = append(final, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("repos: ade task plan rows: %w", err)
	}
	rows.Close()
	for i, id := range final {
		ub := r.Dialect.Update(pl.Table)
		ub.Set(ub.Assign(pl.Position, i)).Where(ub.Equal(pl.TaskID, id))
		res, err := sqlb.Exec(tx, ub)
		if err != nil {
			return fmt.Errorf("repos: set ade task plan position %s: %w", id, err)
		}
		if err := sqlitex.RequireOneRow(res, "ade task plan row "+id); err != nil {
			return err
		}
	}
	for id, day := range days {
		ub := r.Dialect.Update(pl.Table)
		ub.Set(ub.Assign(pl.Day, day), ub.Assign(pl.Backlog, 0)).Where(ub.Equal(pl.TaskID, id))
		res, err := sqlb.Exec(tx, ub)
		if err != nil {
			return fmt.Errorf("repos: set ade task plan day %s: %w", id, err)
		}
		if err := sqlitex.RequireOneRow(res, "ade task plan row "+id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit set ade task plan: %w", err)
	}
	return nil
}

// Shelve moves the live task onto the Later (backlog false) or Backlog shelf, dropping its day, and
// places it right before beforeID, or after the last task on that shelf when beforeID is empty (end of
// the order for an empty shelf). beforeID must be on the same shelf, else ErrShelfTarget. Later holds
// undated, non-Backlog tasks other than review items. A task with no plan row gets one.
func (r *AdeTaskRepo) Shelve(taskID string, backlog bool, beforeID string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin shelve ade task %s: %w", taskID, err)
	}
	defer tx.Rollback() //nolint:errcheck
	var live bool
	tk, pl := kiraschema.AdeTasks, kiraschema.AdeTaskPlan
	liveQ := r.Dialect.Select(tk.ArchivedAt + " IS NULL").From(tk.Table)
	liveQ.Where(liveQ.Equal(tk.ID, taskID))
	err = sqlb.QueryRow(tx, liveQ).Scan(&live)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
		return ErrTaskNotFound
	}
	if err != nil {
		return fmt.Errorf("repos: load ade task %s: %w", taskID, err)
	}
	sel := r.Dialect.Select()
	sel.Select(sel.Var(taskID), "NULL", "COALESCE(MAX("+pl.Position+") + 1, 0)", sel.Var(boolInt(backlog))).From(pl.Table).Where("true")
	shelve := sqlb.AdeInsertSelect(r.Dialect, pl.Table, []string{pl.TaskID, pl.Day, pl.Position, pl.Backlog}, sel,
		"ON CONFLICT ("+pl.TaskID+") DO UPDATE SET "+pl.Day+" = NULL, "+pl.Backlog+" = excluded."+pl.Backlog)
	if _, err := sqlb.Exec(tx, shelve); err != nil {
		return fmt.Errorf("repos: upsert ade task plan %s: %w", taskID, err)
	}
	members := r.Dialect.Select()
	members.Select("p."+pl.TaskID, "p."+pl.Day+" IS NULL AND p."+pl.Backlog+" = "+members.Var(boolInt(backlog))+
		" AND ("+members.Var(boolInt(backlog))+" = 1 OR t."+tk.Kind+" <> "+members.Var(model.AdeTaskKindReview)+")").
		From(pl.Table+" p").Join(tk.Table+" t", "t."+tk.ID+" = p."+pl.TaskID).OrderBy("p."+pl.Position, "p."+pl.TaskID)
	rows, err := sqlb.Query(tx, members)
	if err != nil {
		return fmt.Errorf("repos: query ade task plan: %w", err)
	}
	var ids []string
	onShelf := make(map[string]bool)
	for rows.Next() {
		var id string
		var member bool
		if err := rows.Scan(&id, &member); err != nil {
			rows.Close()
			return fmt.Errorf("repos: scan ade task plan: %w", err)
		}
		if id == taskID {
			continue
		}
		ids = append(ids, id)
		onShelf[id] = member
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("repos: ade task plan rows: %w", err)
	}
	rows.Close()
	at := len(ids)
	if beforeID != "" {
		if !onShelf[beforeID] {
			return ErrShelfTarget
		}
		at = slices.Index(ids, beforeID)
	} else {
		for i, id := range ids {
			if onShelf[id] {
				at = i + 1
			}
		}
	}
	ids = slices.Insert(ids, at, taskID)
	for i, id := range ids {
		ub := r.Dialect.Update(pl.Table)
		ub.Set(ub.Assign(pl.Position, i)).Where(ub.Equal(pl.TaskID, id))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return fmt.Errorf("repos: set ade task plan position %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit shelve ade task %s: %w", taskID, err)
	}
	return nil
}

// SetQueuedAfter sets one branch's queued-after branch id ("" clears it).
func (r *AdeTaskRepo) SetQueuedAfter(branchID, after string) error {
	bc := kiraschema.AdeTaskBranches
	ub := r.Dialect.Update(bc.Table)
	ub.Set(ub.Assign(bc.QueuedAfter, after)).Where(ub.Equal(bc.ID, branchID))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: set ade queued after %s: %w", branchID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repos: set ade queued after %s: %w", branchID, err)
	}
	if n == 0 {
		return ErrBranchMissing
	}
	return nil
}

// SetBranchBase stores a branch's base: the ref name, the planner branch it stacks on ("" = none) and
// the previous base's display name while a rebase onto the new one is pending ("" = none).
func (r *AdeTaskRepo) SetBranchBase(id, base, baseBranchID, pendingFrom string) error {
	bc := kiraschema.AdeTaskBranches
	ub := r.Dialect.Update(bc.Table)
	ub.Set(ub.Assign(bc.Base, base), ub.Assign(bc.BaseBranchID, baseBranchID), ub.Assign(bc.BasePendingFrom, pendingFrom)).
		Where(ub.Equal(bc.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: set ade branch base %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repos: set ade branch base %s: %w", id, err)
	}
	if n == 0 {
		return ErrBranchMissing
	}
	return nil
}

// ClearBasePending drops the pending-rebase mark of the given branches.
func (r *AdeTaskRepo) ClearBasePending(ids []string) error {
	bc := kiraschema.AdeTaskBranches
	for _, id := range ids {
		ub := r.Dialect.Update(bc.Table)
		ub.Set(ub.Assign(bc.BasePendingFrom, "")).Where(ub.Equal(bc.ID, id))
		if _, err := sqlb.Exec(r.DB, ub); err != nil {
			return fmt.Errorf("repos: clear ade base pending %s: %w", id, err)
		}
	}
	return nil
}

// MarkBranchFacts idempotently sets had_commits and sets merged_at once (never overwritten).
func (r *AdeTaskRepo) MarkBranchFacts(hadCommits []string, merged map[string]int64) error {
	if len(hadCommits) == 0 && len(merged) == 0 {
		return nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin mark ade branch facts: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	bc := kiraschema.AdeTaskBranches
	for _, id := range hadCommits {
		ub := r.Dialect.Update(bc.Table)
		ub.Set(ub.Assign(bc.HadCommits, 1)).Where(ub.Equal(bc.ID, id), ub.Equal(bc.HadCommits, 0))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return fmt.Errorf("repos: mark ade branch had commits %s: %w", id, err)
		}
	}
	for id, at := range merged {
		ub := r.Dialect.Update(bc.Table)
		ub.Set(ub.Assign(bc.MergedAt, at)).Where(ub.Equal(bc.ID, id), ub.IsNull(bc.MergedAt))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return fmt.Errorf("repos: mark ade branch merged %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit mark ade branch facts: %w", err)
	}
	return nil
}

// RunsByLiveTask returns the runs of live (unarchived) tasks grouped by task id, oldest first.
func (r *AdeTaskRepo) RunsByLiveTask() (map[string][]model.AdeRun, error) {
	return r.queryRunsByTask(func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.In(kiraschema.AdeRuns.TaskID, adeLiveTaskIDs(r.Dialect))}
	})
}

// RunsOfTask returns one task's runs, oldest first.
func (r *AdeTaskRepo) RunsOfTask(taskID string) ([]model.AdeRun, error) {
	by, err := r.queryRunsByTask(func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(kiraschema.AdeRuns.TaskID, taskID)}
	})
	return by[taskID], err
}

func (r *AdeTaskRepo) queryRunsByTask(where func(sb *sqlbuilder.SelectBuilder) []string) (map[string][]model.AdeRun, error) {
	c := kiraschema.AdeRuns
	sb := r.selectRuns()
	sb.Where(where(sb)...).OrderBy("COALESCE("+c.StartedAt+", 0)", c.ID)
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade runs: %w", err)
	}
	defer rows.Close()
	out := make(map[string][]model.AdeRun)
	for rows.Next() {
		run, err := scanAdeRun(rows)
		if err != nil {
			return nil, fmt.Errorf("repos: scan ade run: %w", err)
		}
		out[run.TaskID] = append(out[run.TaskID], run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade runs rows: %w", err)
	}
	return out, nil
}

// SetupByBranch returns every worktree-setup row keyed by branch id.
func (r *AdeTaskRepo) SetupByBranch() (map[string]model.AdeWorktreeSetup, error) {
	rows, err := sqlb.Query(r.DB, r.selectSetup())
	if err != nil {
		return nil, fmt.Errorf("repos: query ade worktree setup: %w", err)
	}
	defer rows.Close()
	out := make(map[string]model.AdeWorktreeSetup)
	for rows.Next() {
		var s model.AdeWorktreeSetup
		var finished, exit sql.NullInt64
		if err := rows.Scan(&s.BranchID, &s.State, &s.StartedAt, &finished, &exit, &s.Note); err != nil {
			return nil, fmt.Errorf("repos: scan ade worktree setup: %w", err)
		}
		s.FinishedAt, s.ExitCode = nullInt64Ptr(finished), nullIntPtr(exit)
		out[s.BranchID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade worktree setup rows: %w", err)
	}
	return out, nil
}

// SetBranchName records the created branch name; ErrBranchNameTaken when a live branch of the repo
// already holds it, ErrBranchMissing when the branch is gone.
func (r *AdeTaskRepo) SetBranchName(id, name string) error {
	bc := kiraschema.AdeTaskBranches
	ub := r.Dialect.Update(bc.Table)
	ub.Set(ub.Assign(bc.Name, name)).Where(ub.Equal(bc.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrBranchNameTaken
		}
		return fmt.Errorf("repos: set ade branch name %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repos: set ade branch name %s: %w", id, err)
	}
	if n == 0 {
		return ErrBranchMissing
	}
	return nil
}

// RenameBranch renames a created branch row and repoints the rows of its repository that stack on it
// (by planner link or by base name), base and pending-from alike, in one transaction. Returns those rows' ids.
func (r *AdeTaskRepo) RenameBranch(id, from, to string) ([]string, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("repos: begin rename ade branch %s: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck
	var repoID string
	bc := kiraschema.AdeTaskBranches
	repoQ := r.Dialect.Select(bc.CodeRepoID).From(bc.Table)
	repoQ.Where(repoQ.Equal(bc.ID, id), repoQ.Equal(bc.Name, from))
	if err := sqlb.QueryRow(tx, repoQ).Scan(&repoID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBranchMissing
		}
		return nil, fmt.Errorf("repos: rename ade branch %s: %w", id, err)
	}
	nameUp := r.Dialect.Update(bc.Table)
	nameUp.Set(nameUp.Assign(bc.Name, to)).Where(nameUp.Equal(bc.ID, id))
	if _, err := sqlb.Exec(tx, nameUp); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrBranchNameTaken
		}
		return nil, fmt.Errorf("repos: rename ade branch %s: %w", id, err)
	}
	stackedCond := func(c *sqlbuilder.Cond) []string {
		return []string{c.NotEqual(bc.ID, id), c.Equal(bc.CodeRepoID, repoID),
			c.Or(c.Equal(bc.BaseBranchID, id), c.And(c.Equal(bc.BaseBranchID, ""), c.Equal(bc.Base, from)))}
	}
	stackedQ := r.Dialect.Select(bc.ID).From(bc.Table)
	stackedQ.Where(stackedCond(&stackedQ.Cond)...)
	rows, err := sqlb.Query(tx, stackedQ)
	if err != nil {
		return nil, fmt.Errorf("repos: rename ade branch %s stacked: %w", id, err)
	}
	var stacked []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			rows.Close() //nolint:errcheck
			return nil, fmt.Errorf("repos: rename ade branch %s stacked: %w", id, err)
		}
		stacked = append(stacked, sid)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("repos: rename ade branch %s stacked: %w", id, err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: rename ade branch %s stacked: %w", id, err)
	}
	baseUp := r.Dialect.Update(bc.Table)
	baseUp.Set(baseUp.Assign(bc.Base, to)).Where(stackedCond(&baseUp.Cond)...)
	if _, err := sqlb.Exec(tx, baseUp); err != nil {
		return nil, fmt.Errorf("repos: rename ade branch %s bases: %w", id, err)
	}
	pendUp := r.Dialect.Update(bc.Table)
	pendUp.Set(pendUp.Assign(bc.BasePendingFrom, to)).Where(pendUp.Equal(bc.CodeRepoID, repoID), pendUp.Equal(bc.BasePendingFrom, from))
	if _, err := sqlb.Exec(tx, pendUp); err != nil {
		return nil, fmt.Errorf("repos: rename ade branch %s pending: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("repos: commit rename ade branch %s: %w", id, err)
	}
	return stacked, nil
}

// SetBranchOrigin marks who made the branch row ("" user, "agent").
func (r *AdeTaskRepo) SetBranchOrigin(id, origin string) error {
	bc := kiraschema.AdeTaskBranches
	ub := r.Dialect.Update(bc.Table)
	ub.Set(ub.Assign(bc.Origin, origin)).Where(ub.Equal(bc.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: set ade branch origin %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repos: set ade branch origin %s: %w", id, err)
	}
	if n == 0 {
		return ErrBranchMissing
	}
	return nil
}

// ReserveWorktreeDir stores dir as the row's worktree folder when it has none and returns the stored
// value, so the first writer wins.
func (r *AdeTaskRepo) ReserveWorktreeDir(branchID, dir string) (string, error) {
	bc := kiraschema.AdeTaskBranches
	ub := r.Dialect.Update(bc.Table)
	ub.Set(ub.Assign(bc.WorktreeDir, dir)).Where(ub.Equal(bc.ID, branchID), ub.Equal(bc.WorktreeDir, ""))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return "", fmt.Errorf("repos: reserve ade worktree dir %s: %w", branchID, err)
	}
	var stored string
	sb := r.Dialect.Select(bc.WorktreeDir).From(bc.Table)
	sb.Where(sb.Equal(bc.ID, branchID))
	err := sqlb.QueryRow(r.DB, sb).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrBranchMissing
	}
	if err != nil {
		return "", fmt.Errorf("repos: read ade worktree dir %s: %w", branchID, err)
	}
	return stored, nil
}

// ReplaceWorktreeDir swaps the row's worktree folder from old to next; a no-op when it changed meanwhile.
func (r *AdeTaskRepo) ReplaceWorktreeDir(branchID, old, next string) error {
	bc := kiraschema.AdeTaskBranches
	ub := r.Dialect.Update(bc.Table)
	ub.Set(ub.Assign(bc.WorktreeDir, next)).Where(ub.Equal(bc.ID, branchID), ub.Equal(bc.WorktreeDir, old))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: replace ade worktree dir %s: %w", branchID, err)
	}
	return nil
}

// SetEntryStep sets the step the task last entered ("done" when finished). since, when not nil, replaces the
// pass watermark ({"<stepId>/<branchId>": attempt}); nil keeps it. handoff, when not nil, is applied in the
// same transaction, so a move and its handover never land apart.
func (r *AdeTaskRepo) SetEntryStep(taskID, stepID string, since map[string]int, handoff *model.AdeHandoff) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin set ade task entry step %s: %w", taskID, err)
	}
	defer tx.Rollback() //nolint:errcheck
	var res sql.Result
	tk := kiraschema.AdeTasks
	ub := r.Dialect.Update(tk.Table)
	ub.Set(ub.Assign(tk.EntryStepID, stepID)).Where(ub.Equal(tk.ID, taskID))
	if since != nil {
		var js []byte
		if js, err = json.Marshal(since); err != nil {
			return fmt.Errorf("repos: encode ade task since %s: %w", taskID, err)
		}
		ub.SetMore(ub.Assign(tk.SinceJSON, string(js)))
	}
	res, err = sqlb.Exec(tx, ub)
	if err != nil {
		return fmt.Errorf("repos: set ade task entry step %s: %w", taskID, err)
	}
	if err := sqlitex.RequireOneRow(res, "ade task "+taskID); err != nil {
		return err
	}
	if handoff != nil {
		if err := applyHandoff(tx, r.Dialect, taskID, *handoff); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResetWorkflow switches the task's workflow and restarts it: the task's runs and run logs are
// deleted and the entry step is set in one transaction.
func (r *AdeTaskRepo) ResetWorkflow(taskID, workflowID, entryStepID string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin reset ade task workflow %s: %w", taskID, err)
	}
	defer tx.Rollback() //nolint:errcheck
	runs, chunks, logs, tk := kiraschema.AdeRuns, kiraschema.AdeLogChunks, kiraschema.AdeLogs, kiraschema.AdeTasks
	runIDs := func() *sqlbuilder.SelectBuilder {
		sb := r.Dialect.Select(runs.ID).From(runs.Table)
		sb.Where(sb.Equal(runs.TaskID, taskID))
		return sb
	}
	delChunks := r.Dialect.DeleteFrom(chunks.Table)
	delChunks.Where(delChunks.Equal(chunks.Kind, "run"), delChunks.In(chunks.ID, runIDs()))
	if _, err := sqlb.Exec(tx, delChunks); err != nil {
		return fmt.Errorf("repos: delete ade run log chunks %s: %w", taskID, err)
	}
	delLogs := r.Dialect.DeleteFrom(logs.Table)
	delLogs.Where(delLogs.Equal(logs.Kind, "run"), delLogs.In(logs.ID, runIDs()))
	if _, err := sqlb.Exec(tx, delLogs); err != nil {
		return fmt.Errorf("repos: delete ade run logs %s: %w", taskID, err)
	}
	delRuns := r.Dialect.DeleteFrom(runs.Table)
	delRuns.Where(delRuns.Equal(runs.TaskID, taskID))
	if _, err := sqlb.Exec(tx, delRuns); err != nil {
		return fmt.Errorf("repos: delete ade runs %s: %w", taskID, err)
	}
	reset := r.Dialect.Update(tk.Table)
	reset.Set(reset.Assign(tk.WorkflowID, workflowID), reset.Assign(tk.EntryStepID, entryStepID), reset.Assign(tk.SinceJSON, nil),
		reset.Assign(tk.WorkflowJSON, nil), reset.Assign(tk.WorkflowHash, "")).Where(reset.Equal(tk.ID, taskID))
	res, err := sqlb.Exec(tx, reset)
	if err != nil {
		return fmt.Errorf("repos: reset ade task workflow %s: %w", taskID, err)
	}
	if err := sqlitex.RequireOneRow(res, "ade task "+taskID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit reset ade task workflow %s: %w", taskID, err)
	}
	return nil
}

// SetAutoStatus stores the status the engine applied for step key `step` (status_by = auto).
func (r *AdeTaskRepo) SetAutoStatus(taskID, statusID, step string) error {
	tk := kiraschema.AdeTasks
	ub := r.Dialect.Update(tk.Table)
	ub.Set(ub.Assign(tk.StatusID, statusID), ub.Assign(tk.StatusBy, model.AdeStatusByAuto), ub.Assign(tk.StatusStep, step)).
		Where(ub.Equal(tk.ID, taskID))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: set ade task status %s: %w", taskID, err)
	}
	return sqlitex.RequireOneRow(res, "ade task "+taskID)
}

// HasRunning reports whether any run of the task is running.
func (r *AdeTaskRepo) HasRunning(taskID string) (bool, error) {
	c := kiraschema.AdeRuns
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.TaskID, taskID), sb.Equal(c.State, model.AdeRunRunning)}
	})
	if err != nil {
		return false, fmt.Errorf("repos: count running ade runs %s: %w", taskID, err)
	}
	return n > 0, nil
}

// SetWorkflowSnapshot stores the whole workflow a task began with. A task that already has one keeps
// it: the first snapshot wins.
func (r *AdeTaskRepo) SetWorkflowSnapshot(taskID, workflowJSON, hash string) error {
	tk := kiraschema.AdeTasks
	ub := r.Dialect.Update(tk.Table)
	ub.Set(ub.Assign(tk.WorkflowJSON, workflowJSON), ub.Assign(tk.WorkflowHash, hash)).Where(ub.Equal(tk.ID, taskID), ub.IsNull(tk.WorkflowJSON))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: set ade task workflow snapshot %s: %w", taskID, err)
	}
	return nil
}

func (r *AdeTaskRepo) selectSetup() *sqlbuilder.SelectBuilder {
	c := kiraschema.AdeWorktreeSetup
	return r.Dialect.Select(c.BranchID, c.State, c.StartedAt, c.FinishedAt, c.ExitCode, c.Note).From(c.Table)
}

func (r *AdeTaskRepo) selectReviews() *sqlbuilder.SelectBuilder {
	c := kiraschema.AdeReviews
	return r.Dialect.Select(c.RunID, c.Route, c.TargetRunID, c.DedicatedClaudeID, c.Rounds).From(c.Table)
}

func scanAdeRun(row rowScanner) (model.AdeRun, error) {
	var run model.AdeRun
	var exit, started, finished sql.NullInt64
	var outcome string
	if err := row.Scan(&run.ID, &run.TaskID, &run.StepID, &run.BranchID, &run.Attempt, &run.State,
		&run.Loops, &run.Note, &run.Summary, &run.SessionID, &exit, &started, &finished, &outcome, &run.Purpose, &run.SpecJSON,
		&run.Launch.Note, &run.Launch.ResumeID, &run.Launch.Prompt, &run.Launch.Extra); err != nil {
		return model.AdeRun{}, err
	}
	if outcome != "" {
		var o model.AdeRunOutcome
		if err := json.Unmarshal([]byte(outcome), &o); err != nil {
			return model.AdeRun{}, fmt.Errorf("repos: decode ade run %s outcome: %w", run.ID, err)
		}
		run.Outcome = &o
	}
	run.ExitCode = nullIntPtr(exit)
	run.StartedAt, run.FinishedAt = nullInt64Ptr(started), nullInt64Ptr(finished)
	return run, nil
}

func encodeOutcome(o *model.AdeRunOutcome) (string, error) {
	if o == nil {
		return "", nil
	}
	b, err := json.Marshal(o)
	if err != nil {
		return "", fmt.Errorf("repos: encode run outcome: %w", err)
	}
	return string(b), nil
}

// InsertRun writes a new run attempt.
func (r *AdeTaskRepo) InsertRun(run model.AdeRun) error {
	if run.ID == "" || run.TaskID == "" || run.Attempt < 1 {
		return fmt.Errorf("repos: insert ade run: id, taskId and attempt >= 1 are required")
	}
	outcome, err := encodeOutcome(run.Outcome)
	if err != nil {
		return err
	}
	ib := r.Dialect.InsertInto(kiraschema.AdeRuns.Table).Cols(adeRunColumns...).
		Values(run.ID, run.TaskID, run.StepID, run.BranchID, run.Attempt, run.State,
			run.Loops, run.Note, run.Summary, run.SessionID, run.ExitCode, run.StartedAt, run.FinishedAt, outcome, run.Purpose, run.SpecJSON,
			run.Launch.Note, run.Launch.ResumeID, run.Launch.Prompt, run.Launch.Extra)
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: insert ade run %s: %w", run.ID, err)
	}
	return nil
}

// UpdateRun applies the non-nil leaves of p and returns the stored run.
func (r *AdeTaskRepo) UpdateRun(id string, p model.AdeRunPatch) (model.AdeRun, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeRun{}, fmt.Errorf("repos: begin update ade run: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	curQ := r.selectRuns()
	curQ.Where(curQ.Equal(kiraschema.AdeRuns.ID, id))
	cur, err := scanAdeRun(sqlb.QueryRow(tx, curQ))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdeRun{}, ErrRunNotFound
	}
	if err != nil {
		return model.AdeRun{}, fmt.Errorf("repos: select ade run %s: %w", id, err)
	}
	if p.State != nil {
		cur.State = *p.State
	}
	if p.Note != nil {
		cur.Note = *p.Note
	}
	if p.Summary != nil {
		cur.Summary = *p.Summary
	}
	if p.SessionID != nil {
		cur.SessionID = *p.SessionID
	}
	if p.ExitCode != nil {
		cur.ExitCode = p.ExitCode
	}
	if p.StartedAt != nil {
		cur.StartedAt = p.StartedAt
	}
	if p.FinishedAt != nil {
		cur.FinishedAt = p.FinishedAt
	}
	if p.Outcome != nil {
		cur.Outcome = p.Outcome
	}
	if p.Launch != nil {
		cur.Launch = *p.Launch
	}
	if p.SpecJSON != nil {
		cur.SpecJSON = *p.SpecJSON
	}
	outcome, err := encodeOutcome(cur.Outcome)
	if err != nil {
		return model.AdeRun{}, err
	}
	c := kiraschema.AdeRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.State, cur.State), ub.Assign(c.Note, cur.Note), ub.Assign(c.Summary, cur.Summary),
		ub.Assign(c.SessionID, cur.SessionID), ub.Assign(c.ExitCode, cur.ExitCode), ub.Assign(c.StartedAt, cur.StartedAt),
		ub.Assign(c.FinishedAt, cur.FinishedAt), ub.Assign(c.OutcomeJSON, outcome), ub.Assign(c.LaunchNote, cur.Launch.Note),
		ub.Assign(c.LaunchResumeID, cur.Launch.ResumeID), ub.Assign(c.LaunchPrompt, cur.Launch.Prompt),
		ub.Assign(c.LaunchExtra, cur.Launch.Extra), ub.Assign(c.SpecJSON, cur.SpecJSON)).Where(ub.Equal(c.ID, id))
	if _, err := sqlb.Exec(tx, ub); err != nil {
		return model.AdeRun{}, fmt.Errorf("repos: update ade run %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return model.AdeRun{}, fmt.Errorf("repos: commit update ade run %s: %w", id, err)
	}
	return cur, nil
}

// GetRun returns one run; ErrRunNotFound if absent.
func (r *AdeTaskRepo) GetRun(id string) (model.AdeRun, error) {
	sb := r.selectRuns()
	sb.Where(sb.Equal(kiraschema.AdeRuns.ID, id))
	run, err := scanAdeRun(sqlb.QueryRow(r.DB, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdeRun{}, ErrRunNotFound
	}
	if err != nil {
		return model.AdeRun{}, fmt.Errorf("repos: get ade run %s: %w", id, err)
	}
	return run, nil
}

// LatestRuns returns the latest attempt of every (step, branch, purpose) of one task.
func (r *AdeTaskRepo) LatestRuns(taskID string) ([]model.AdeRun, error) {
	c := kiraschema.AdeRuns
	newest := r.Dialect.Select("MAX(" + c.Attempt + ")").From(c.Table + " b")
	newest.Where("b."+c.TaskID+" = a."+c.TaskID, "b."+c.StepID+" = a."+c.StepID, "b."+c.BranchID+" = a."+c.BranchID,
		"b."+c.Purpose+" = a."+c.Purpose)
	sb := r.Dialect.Select(adeRunColumns...).From(c.Table + " a")
	sb.Where(sb.Equal(c.TaskID, taskID), c.Attempt+" = ("+sb.Var(newest)+")").OrderBy("COALESCE("+c.StartedAt+", 0)", c.ID)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

// UpsertSetup writes a branch's worktree-setup row.
func (r *AdeTaskRepo) UpsertSetup(s model.AdeWorktreeSetup) error {
	c := kiraschema.AdeWorktreeSetup
	ib := r.Dialect.InsertInto(c.Table).Cols(c.BranchID, c.State, c.StartedAt, c.FinishedAt, c.ExitCode, c.Note).
		Values(s.BranchID, s.State, s.StartedAt, s.FinishedAt, s.ExitCode, s.Note)
	sqlb.Upsert(ib, []string{c.BranchID}, []string{c.State, c.StartedAt, c.FinishedAt, c.ExitCode, c.Note}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: upsert ade worktree setup %s: %w", s.BranchID, err)
	}
	return nil
}

// GetSetup returns a branch's setup row; (nil, nil) when it has none.
func (r *AdeTaskRepo) GetSetup(branchID string) (*model.AdeWorktreeSetup, error) {
	var s model.AdeWorktreeSetup
	var finished, exit sql.NullInt64
	sb := r.selectSetup()
	sb.Where(sb.Equal(kiraschema.AdeWorktreeSetup.BranchID, branchID))
	err := sqlb.QueryRow(r.DB, sb).Scan(&s.BranchID, &s.State, &s.StartedAt, &finished, &exit, &s.Note)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repos: get ade worktree setup %s: %w", branchID, err)
	}
	s.FinishedAt, s.ExitCode = nullInt64Ptr(finished), nullIntPtr(exit)
	return &s, nil
}

// SetPendingNote rewrites the note of a branch's pending runs and returns them.
func (r *AdeTaskRepo) SetPendingNote(branchID, note string) ([]model.AdeRun, error) {
	c := kiraschema.AdeRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Note, note)).Where(ub.Equal(c.BranchID, branchID), ub.Equal(c.State, model.AdeRunPending))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return nil, fmt.Errorf("repos: set pending ade run note %s: %w", branchID, err)
	}
	sb := r.selectRuns()
	sb.Where(sb.Equal(c.BranchID, branchID), sb.Equal(c.State, model.AdeRunPending)).OrderBy(c.ID)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

// GetReview returns the review run's send state; (nil, nil) when it has none.
func (r *AdeTaskRepo) GetReview(runID string) (*model.AdeReview, error) {
	var v model.AdeReview
	sb := r.selectReviews()
	sb.Where(sb.Equal(kiraschema.AdeReviews.RunID, runID))
	err := sqlb.QueryRow(r.DB, sb).Scan(&v.RunID, &v.Route, &v.TargetRunID, &v.DedicatedClaudeID, &v.Rounds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repos: get ade review %s: %w", runID, err)
	}
	return &v, nil
}

// UpsertReview writes the route and target of a review run; the round count and dedicated session stay.
func (r *AdeTaskRepo) UpsertReview(v model.AdeReview) error {
	c := kiraschema.AdeReviews
	ib := r.Dialect.InsertInto(c.Table).Cols(c.RunID, c.Route, c.TargetRunID).Values(v.RunID, v.Route, v.TargetRunID)
	sqlb.Upsert(ib, []string{c.RunID}, []string{c.Route, c.TargetRunID}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: upsert ade review %s: %w", v.RunID, err)
	}
	return nil
}

// BumpReviewRound counts one more round and returns the new count.
func (r *AdeTaskRepo) BumpReviewRound(runID string) (int, error) {
	var n int
	c := kiraschema.AdeReviews
	ub := r.Dialect.Update(c.Table)
	ub.Set(c.Rounds + " = " + c.Rounds + " + 1").Where(ub.Equal(c.RunID, runID)).Returning(c.Rounds)
	if err := sqlb.QueryRow(r.DB, ub).Scan(&n); err != nil {
		return 0, fmt.Errorf("repos: bump ade review round %s: %w", runID, err)
	}
	return n, nil
}

// SetReviewDedicated stores the dedicated route's Claude session id ("" forgets a lost one).
func (r *AdeTaskRepo) SetReviewDedicated(runID, claudeID string) error {
	c := kiraschema.AdeReviews
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.DedicatedClaudeID, claudeID)).Where(ub.Equal(c.RunID, runID))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: set ade review session %s: %w", runID, err)
	}
	return nil
}

// Reviews returns the send state of every review run, by run id.
func (r *AdeTaskRepo) Reviews() (map[string]model.AdeReview, error) {
	runs := kiraschema.AdeRuns
	liveRuns := r.Dialect.Select(runs.ID).From(runs.Table)
	liveRuns.Where(liveRuns.In(runs.TaskID, adeLiveTaskIDs(r.Dialect)))
	sb := r.selectReviews()
	sb.Where(sb.In(kiraschema.AdeReviews.RunID, liveRuns))
	rows, err := sqlb.Query(r.DB, sb)
	list, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeReview, bool, error) {
		var v model.AdeReview
		err := rows.Scan(&v.RunID, &v.Route, &v.TargetRunID, &v.DedicatedClaudeID, &v.Rounds)
		return v, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("repos: ade reviews: %w", err)
	}
	out := make(map[string]model.AdeReview, len(list))
	for _, v := range list {
		out[v.RunID] = v
	}
	return out, nil
}

// ReviewRounds returns the task's review rounds, oldest first.
func (r *AdeTaskRepo) ReviewRounds(taskID string) ([]model.AdeRun, error) {
	rows, err := sqlb.Query(r.DB, r.reviewRoundsQuery(taskID))
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

func (r *AdeTaskRepo) reviewRoundsQuery(taskID string) *sqlbuilder.SelectBuilder {
	c := kiraschema.AdeRuns
	sb := r.selectRuns()
	return sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.Purpose, model.AdeRunPurposeReview)).OrderBy(c.Attempt, c.ID)
}

// PendingOnBranch lists the pending runs of a branch with the given purpose.
func (r *AdeTaskRepo) PendingOnBranch(branchID, purpose string) ([]model.AdeRun, error) {
	rows, err := sqlb.Query(r.DB, r.pendingOnBranchQuery(branchID, purpose))
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

func (r *AdeTaskRepo) pendingOnBranchQuery(branchID, purpose string) *sqlbuilder.SelectBuilder {
	c := kiraschema.AdeRuns
	sb := r.selectRuns()
	return sb.Where(sb.Equal(c.BranchID, branchID), sb.Equal(c.Purpose, purpose), sb.Equal(c.State, model.AdeRunPending)).OrderBy(c.ID)
}

// CountRunning counts runs in the running state.
func (r *AdeTaskRepo) CountRunning() (int, error) {
	c := kiraschema.AdeRuns
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.State, model.AdeRunRunning)}
	})
	if err != nil {
		return 0, fmt.Errorf("repos: count running ade runs: %w", err)
	}
	return n, nil
}

// RecoverRunning turns every running run stuck, with out as its outcome and its reason as the note,
// and returns them (boot recovery). A step run with a Claude session keeps that session as its
// launch spec's ResumeID, so a Resume can continue it; every other launch column is cleared.
func (r *AdeTaskRepo) RecoverRunning(now int64, out model.AdeRunOutcome) ([]model.AdeRun, error) {
	outcome, err := encodeOutcome(&out)
	if err != nil {
		return nil, err
	}
	c, s := kiraschema.AdeRuns, kiraschema.AdeSessions
	ub := r.Dialect.Update(c.Table)
	resume := c.LaunchResumeID + " = CASE WHEN " + c.Purpose + " IN (" + ub.Var("") + ", " + ub.Var(model.AdeRunPurposeReview) +
		") AND " + c.SessionID + " <> " + ub.Var("") + " THEN COALESCE((SELECT " + s.AgentSessionID + " FROM " + s.Table +
		" s WHERE s." + s.ID + " = " + c.Table + "." + c.SessionID + "), " + ub.Var("") + ") ELSE " + ub.Var("") + " END"
	ub.Set(ub.Assign(c.State, model.AdeRunStuck), ub.Assign(c.Note, out.Reason), ub.Assign(c.FinishedAt, now),
		ub.Assign(c.OutcomeJSON, outcome), ub.Assign(c.LaunchNote, ""), ub.Assign(c.LaunchPrompt, ""), ub.Assign(c.LaunchExtra, ""), resume).
		Where(ub.Equal(c.State, model.AdeRunRunning)).Returning(adeRunColumns...)
	rows, err := sqlb.Query(r.DB, ub)
	runs, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("repos: recover ade runs: %w", err)
	}
	slices.SortFunc(runs, func(a, b model.AdeRun) int { return strings.Compare(a.ID, b.ID) })
	return runs, nil
}

// FailRunningSetups marks every running worktree setup failed and returns their branch and task ids
// (boot recovery).
func (r *AdeTaskRepo) FailRunningSetups(now int64, note string) ([]model.AdeTaskBranch, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("repos: begin recover ade setups: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	su, bc := kiraschema.AdeWorktreeSetup, kiraschema.AdeTaskBranches
	runQ := r.Dialect.Select("b."+bc.ID, "b."+bc.TaskID).From(su.Table + " s")
	runQ.Join(bc.Table+" b", "b."+bc.ID+" = s."+su.BranchID).
		Where(runQ.Equal("s."+su.State, model.AdeSetupRunning)).OrderBy("b." + bc.ID)
	rows, err := sqlb.Query(tx, runQ)
	out, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeTaskBranch, bool, error) {
		var b model.AdeTaskBranch
		err := rows.Scan(&b.ID, &b.TaskID)
		return b, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("repos: select running ade setups: %w", err)
	}
	failUp := r.Dialect.Update(su.Table)
	failUp.Set(failUp.Assign(su.State, model.AdeSetupFailed), failUp.Assign(su.FinishedAt, now), failUp.Assign(su.Note, note)).
		Where(failUp.Equal(su.State, model.AdeSetupRunning))
	if _, err := sqlb.Exec(tx, failUp); err != nil {
		return nil, fmt.Errorf("repos: recover ade setups: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("repos: commit recover ade setups: %w", err)
	}
	return out, nil
}

// CountRuns counts a step's runs on one branch in the given state, across attempts.
func (r *AdeTaskRepo) CountRuns(taskID, stepID, branchID, state string) (int, error) {
	c := kiraschema.AdeRuns
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.TaskID, taskID), sb.Equal(c.StepID, stepID), sb.Equal(c.BranchID, branchID), sb.Equal(c.State, state)}
	})
	if err != nil {
		return 0, fmt.Errorf("repos: count ade runs %s: %w", taskID, err)
	}
	return n, nil
}

// HasRunningOn reports whether a run on the branch is running.
func (r *AdeTaskRepo) HasRunningOn(branchID string) (bool, error) {
	c := kiraschema.AdeRuns
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.BranchID, branchID), sb.Equal(c.State, model.AdeRunRunning)}
	})
	if err != nil {
		return false, fmt.Errorf("repos: count running ade runs on %s: %w", branchID, err)
	}
	return n > 0, nil
}

// HasActiveStepRunOn reports whether a step run (not a rebase) on the branch is running or pending.
func (r *AdeTaskRepo) HasActiveStepRunOn(branchID string) (bool, error) {
	c := kiraschema.AdeRuns
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.BranchID, branchID), sb.Equal(c.Purpose, ""), sb.In(c.State, model.AdeRunRunning, model.AdeRunPending)}
	})
	if err != nil {
		return false, fmt.Errorf("repos: count active ade step runs on %s: %w", branchID, err)
	}
	return n > 0, nil
}

// RunningRebaseOn returns the running rebase run whose stack holds the branch, nil when none.
func (r *AdeTaskRepo) RunningRebaseOn(branchID string) (*model.AdeRun, error) {
	runs, err := r.RunningRebases()
	if err != nil {
		return nil, err
	}
	for i := range runs {
		if runs[i].BranchID == branchID {
			return &runs[i], nil
		}
		var spec model.AdeRebaseSpec
		if json.Unmarshal([]byte(runs[i].SpecJSON), &spec) != nil {
			continue
		}
		for _, st := range spec.Stack {
			if st.BranchID == branchID {
				return &runs[i], nil
			}
		}
	}
	return nil, nil
}

// RunningRebases lists the rebase runs in state running.
func (r *AdeTaskRepo) RunningRebases() ([]model.AdeRun, error) {
	c := kiraschema.AdeRuns
	sb := r.selectRuns()
	sb.Where(sb.Equal(c.Purpose, model.AdeRunPurposeRebase), sb.Equal(c.State, model.AdeRunRunning)).OrderBy(c.ID)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

// LatestRebaseRun returns the newest rebase run rooted at the branch, nil when none.
func (r *AdeTaskRepo) LatestRebaseRun(branchID string) (*model.AdeRun, error) {
	c := kiraschema.AdeRuns
	sb := r.selectRuns()
	sb.Where(sb.Equal(c.BranchID, branchID), sb.Equal(c.Purpose, model.AdeRunPurposeRebase)).
		OrderBy(c.Attempt+" DESC", c.ID+" DESC").Limit(1)
	run, err := scanAdeRun(sqlb.QueryRow(r.DB, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repos: latest ade rebase run %s: %w", branchID, err)
	}
	return &run, nil
}

// CountRebaseRuns counts the rebase runs rooted at the branch.
func (r *AdeTaskRepo) CountRebaseRuns(branchID string) (int, error) {
	c := kiraschema.AdeRuns
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.BranchID, branchID), sb.Equal(c.Purpose, model.AdeRunPurposeRebase)}
	})
	if err != nil {
		return 0, fmt.Errorf("repos: count ade rebase runs %s: %w", branchID, err)
	}
	return n, nil
}

// SetBranchChangeRequests replaces the stored change request of each listed branch with crs (key =
// branch id) in one transaction; a listed branch missing from crs ends with none.
func (r *AdeTaskRepo) SetBranchChangeRequests(ids []string, crs map[string]model.AdeChangeRequest) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin set ade branch change requests: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	bc := kiraschema.AdeTaskBranches
	for _, id := range ids {
		ub := r.Dialect.Update(bc.Table)
		ub.Set(ub.Assign(bc.CrNumber, nil), ub.Assign(bc.CrTitle, nil), ub.Assign(bc.CrState, nil), ub.Assign(bc.CrURL, nil)).
			Where(ub.Equal(bc.ID, id))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return fmt.Errorf("repos: clear ade branch %s change request: %w", id, err)
		}
	}
	for id, cr := range crs {
		ub := r.Dialect.Update(bc.Table)
		ub.Set(ub.Assign(bc.CrNumber, cr.Number), ub.Assign(bc.CrTitle, cr.Title), ub.Assign(bc.CrState, cr.State), ub.Assign(bc.CrURL, cr.URL)).
			Where(ub.Equal(bc.ID, id))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return fmt.Errorf("repos: set ade branch %s change request: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit set ade branch change requests: %w", err)
	}
	return nil
}

// SetBranchReviewComments stores (json) or clears ("") the review comment snapshot of one branch.
func (r *AdeTaskRepo) SetBranchReviewComments(id, json string) error {
	var v any
	if json != "" {
		v = json
	}
	bc := kiraschema.AdeTaskBranches
	ub := r.Dialect.Update(bc.Table)
	ub.Set(ub.Assign(bc.ReviewComments, v)).Where(ub.Equal(bc.ID, id))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: set ade branch %s review comments: %w", id, err)
	}
	return nil
}

// RestoreTask unarchives the task in one transaction: the task and the branch rows archived with
// it (same archived_at) go live and the task takes a plan row at the end of Later. Rows detached
// earlier stay detached. A named branch live on another task refuses with *BranchHeldError.
func (r *AdeTaskRepo) RestoreTask(taskID string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin restore ade task %s: %w", taskID, err)
	}
	defer tx.Rollback() //nolint:errcheck
	tq := r.selectTask()
	tq.Where(tq.Equal(kiraschema.AdeTasks.ID, taskID))
	t, err := scanAdeTask(sqlb.QueryRow(tx, tq))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskNotFound
	}
	if err != nil {
		return fmt.Errorf("repos: load ade task %s: %w", taskID, err)
	}
	if t.ArchivedAt == nil {
		return ErrTaskNotArchived
	}
	var held, holder string
	bc, tk := kiraschema.AdeTaskBranches, kiraschema.AdeTasks
	heldQ := r.Dialect.Select()
	heldQ.Select("mine."+bc.Name, "COALESCE(NULLIF(other_t."+tk.Title+", ''), other_t."+tk.JiraKey+", '')").
		From(bc.Table+" mine").
		Join(bc.Table+" other", "other."+bc.CodeRepoID+" = mine."+bc.CodeRepoID, "other."+bc.Name+" = mine."+bc.Name,
			heldQ.IsNull("other."+bc.ArchivedAt)).
		Join(tk.Table+" other_t", "other_t."+tk.ID+" = other."+bc.TaskID).
		Where(heldQ.Equal("mine."+bc.TaskID, taskID), heldQ.Equal("mine."+bc.ArchivedAt, *t.ArchivedAt),
			heldQ.NotEqual("mine."+bc.Name, "")).Limit(1)
	err = sqlb.QueryRow(tx, heldQ).Scan(&held, &holder)
	if err == nil {
		return &BranchHeldError{Branch: held, TaskTitle: holder}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("repos: check ade task %s branch names: %w", taskID, err)
	}
	brUp := r.Dialect.Update(bc.Table)
	brUp.Set(brUp.Assign(bc.ArchivedAt, nil)).Where(brUp.Equal(bc.TaskID, taskID), brUp.Equal(bc.ArchivedAt, *t.ArchivedAt))
	if _, err := sqlb.Exec(tx, brUp); err != nil {
		return fmt.Errorf("repos: restore ade task branches %s: %w", taskID, err)
	}
	taskUp := r.Dialect.Update(tk.Table)
	taskUp.Set(taskUp.Assign(tk.ArchivedAt, nil)).Where(taskUp.Equal(tk.ID, taskID))
	if _, err := sqlb.Exec(tx, taskUp); err != nil {
		return fmt.Errorf("repos: restore ade task %s: %w", taskID, err)
	}
	pl := kiraschema.AdeTaskPlan
	planSel := r.Dialect.Select()
	planSel.Select(planSel.Var(taskID), "NULL", "COALESCE(MAX("+pl.Position+") + 1, 0)").From(pl.Table)
	if _, err := sqlb.Exec(tx, sqlb.AdeInsertSelect(r.Dialect, pl.Table, []string{pl.TaskID, pl.Day, pl.Position}, planSel, "")); err != nil {
		return fmt.Errorf("repos: insert ade task plan row %s: %w", taskID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit restore ade task %s: %w", taskID, err)
	}
	return nil
}

// ArchiveTask archives the task and its branches and drops its plan row in one transaction.
func (r *AdeTaskRepo) ArchiveTask(taskID string, now int64) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin archive ade task %s: %w", taskID, err)
	}
	defer tx.Rollback() //nolint:errcheck
	tk, bc, pl := kiraschema.AdeTasks, kiraschema.AdeTaskBranches, kiraschema.AdeTaskPlan
	taskUp := r.Dialect.Update(tk.Table)
	taskUp.Set(taskUp.Assign(tk.ArchivedAt, now)).Where(taskUp.Equal(tk.ID, taskID), taskUp.IsNull(tk.ArchivedAt))
	res, err := sqlb.Exec(tx, taskUp)
	if err != nil {
		return fmt.Errorf("repos: archive ade task %s: %w", taskID, err)
	}
	if err := sqlitex.RequireOneRow(res, "ade task "+taskID); err != nil {
		return err
	}
	brUp := r.Dialect.Update(bc.Table)
	brUp.Set(brUp.Assign(bc.ArchivedAt, now)).Where(brUp.Equal(bc.TaskID, taskID), brUp.IsNull(bc.ArchivedAt))
	if _, err := sqlb.Exec(tx, brUp); err != nil {
		return fmt.Errorf("repos: archive ade task branches %s: %w", taskID, err)
	}
	planDel := r.Dialect.DeleteFrom(pl.Table)
	planDel.Where(planDel.Equal(pl.TaskID, taskID))
	if _, err := sqlb.Exec(tx, planDel); err != nil {
		return fmt.Errorf("repos: drop ade task plan row %s: %w", taskID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit archive ade task %s: %w", taskID, err)
	}
	return nil
}
