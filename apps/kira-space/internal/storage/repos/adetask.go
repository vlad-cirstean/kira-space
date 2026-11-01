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

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
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
	ErrRepoOnTask    = errors.New("repos: task already has a branch in that repo")
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

const adeTaskColumns = `id, kind, title, owner, jira_key, jira_url, host_url, workflow_id, entry_step_id, since_json, workflow_json, workflow_hash, est, notes, color, created_at, archived_at, status_id, status_by, status_step`
const adeTaskBranchInsertColumns = `id, task_id, code_repo_id, name, kind, base, base_branch_id, base_pending_from, queued_after, position, had_commits, added_at, merged_at, archived_at, origin`
const adeTaskBranchColumns = adeTaskBranchInsertColumns + `, cr_number, cr_title, cr_state, cr_url, review_comments`
const adeRunColumns = `id, task_id, step_id, branch_id, attempt, state, loops, note, summary, session_id, exit_code, started_at, finished_at, outcome_json, purpose, spec_json, launch_note, launch_resume_id, launch_prompt, launch_extra`

// AdeTaskRepo reads and writes the v2 task store: tasks, branches, plan, runs, worktree setup and
// last-valid workflows.
type AdeTaskRepo struct {
	DB *sql.DB
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
		&t.EntryStepID, &since, &workflow, &t.WorkflowHash, &t.Est, &t.Notes, &t.Color, &t.CreatedAt, &archived, &t.StatusID, &t.StatusBy, &t.StatusStep); err != nil {
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
		&had, &b.AddedAt, &merged, &archived, &b.Origin, &crNumber, &crTitle, &crState, &crURL, &comments); err != nil {
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

func (r *AdeTaskRepo) queryTasks(query string, args ...any) ([]model.AdeTask, error) {
	rows, err := r.DB.Query(query, args...)
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
	return r.queryTasks(`SELECT t.id, t.kind, t.title, t.owner, t.jira_key, t.jira_url, t.host_url, t.workflow_id,
		t.entry_step_id, t.since_json, t.workflow_json, t.workflow_hash, t.est, t.notes, t.color, t.created_at, t.archived_at, t.status_id, t.status_by, t.status_step
		FROM ade_tasks t LEFT JOIN ade_task_plan p ON p.task_id = t.id
		WHERE t.archived_at IS NULL ORDER BY p.position IS NULL, p.position, t.created_at, t.id`)
}

// ListArchived returns archived tasks, newest archive first.
func (r *AdeTaskRepo) ListArchived() ([]model.AdeTask, error) {
	return r.queryTasks(`SELECT ` + adeTaskColumns + ` FROM ade_tasks WHERE archived_at IS NOT NULL ORDER BY archived_at DESC, id`)
}

// GetTask returns one task, live or archived; ErrTaskNotFound if absent.
func (r *AdeTaskRepo) GetTask(id string) (model.AdeTask, error) {
	t, err := scanAdeTask(r.DB.QueryRow(`SELECT `+adeTaskColumns+` FROM ade_tasks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdeTask{}, ErrTaskNotFound
	}
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: get ade task %s: %w", id, err)
	}
	return t, nil
}

// insertTask writes the task, its branches and a plan row on `day` (Later when nil), in tx: at the end of
// the order, or at the top when top is set. It assigns the task's color: the first slot no live task uses,
// else the least-used slot overall.
func insertTask(tx *sql.Tx, t model.AdeTask, branches []model.AdeTaskBranch, day *string, top bool) (model.AdeTask, error) {
	if err := t.Validate(); err != nil {
		return model.AdeTask{}, err
	}
	rows, err := tx.Query(`SELECT color, archived_at IS NULL FROM ade_tasks`)
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
		id, err := statusForNew(tx, t.Kind == model.AdeTaskKindReview)
		if err != nil {
			return model.AdeTask{}, err
		}
		t.StatusID = id
	}
	if t.StatusBy == "" {
		t.StatusBy = model.AdeStatusByAuto
	}
	if _, err := tx.Exec(`INSERT INTO ade_tasks (id, kind, title, owner, jira_key, jira_url, host_url, workflow_id, entry_step_id, since_json, est, notes, color, created_at, archived_at, status_id, status_by, status_step) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)`,
		t.ID, t.Kind, t.Title, t.Owner, t.JiraKey, t.JiraURL, t.HostURL, t.WorkflowID, t.EntryStepID, since, t.Est,
		t.Notes, t.Color, t.CreatedAt, t.StatusID, t.StatusBy, t.StatusStep); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: insert ade task %s: %w", t.ID, err)
	}
	for _, b := range branches {
		if err := insertTaskBranch(tx, b); err != nil {
			return model.AdeTask{}, err
		}
	}
	position := `COALESCE(MAX(position) + 1, 0)`
	if top {
		position = `COALESCE(MIN(position) - 1, 0)`
	}
	if _, err := tx.Exec(`INSERT INTO ade_task_plan (task_id, day, position)
		SELECT ?, ?, `+position+` FROM ade_task_plan`, t.ID, day); err != nil {
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

func insertTaskBranch(tx *sql.Tx, b model.AdeTaskBranch) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO ade_task_branches (`+adeTaskBranchInsertColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)`,
		b.ID, b.TaskID, b.CodeRepoID, b.Name, b.Kind, b.Base, b.BaseBranchID, b.BasePendingFrom, b.QueuedAfter, b.Position, boolInt(b.HadCommits), b.AddedAt, b.Origin); err != nil {
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
// one transaction and returns the stored task (color assigned).
func (r *AdeTaskRepo) CreateTask(t model.AdeTask, branches []model.AdeTaskBranch, day *string) (model.AdeTask, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: begin create ade task: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	out, err := insertTask(tx, t, branches, day, false)
	if err != nil {
		return model.AdeTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: commit create ade task: %w", err)
	}
	return out, nil
}

// CaptureTask inserts a branchless task at the top of Later (no day) and returns the stored task.
func (r *AdeTaskRepo) CaptureTask(t model.AdeTask) (model.AdeTask, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: begin capture ade task: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	out, err := insertTask(tx, t, nil, nil, true)
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
	cur, err := scanAdeTask(tx.QueryRow(`SELECT `+adeTaskColumns+` FROM ade_tasks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdeTask{}, ErrTaskNotFound
	}
	if err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: select ade task %s: %w", id, err)
	}
	prevKind := cur.Kind
	if err := checkStatusExists(tx, p.StatusID); err != nil {
		return model.AdeTask{}, err
	}
	if p.Kind != nil && *p.Kind != cur.Kind {
		if err := moveTaskKind(&cur, *p.Kind); err != nil {
			return model.AdeTask{}, err
		}
	}
	if prevKind == model.AdeTaskKindReview && cur.Kind == model.AdeTaskKindTask {
		id, err := statusForNew(tx, false)
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
		if err := moveBranchKinds(tx, id, prevKind, cur.Kind); err != nil {
			return model.AdeTask{}, err
		}
	}
	if _, err := tx.Exec(`UPDATE ade_tasks SET kind = ?, owner = ?, title = ?, jira_key = ?, jira_url = ?, host_url = ?, est = ?, notes = ?, color = ?, status_id = ?, status_by = ?, status_step = ? WHERE id = ?`,
		cur.Kind, cur.Owner, cur.Title, cur.JiraKey, cur.JiraURL, cur.HostURL, cur.Est, cur.Notes, cur.Color, cur.StatusID, cur.StatusBy, cur.StatusStep, id); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: update ade task %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return model.AdeTask{}, fmt.Errorf("repos: commit update ade task %s: %w", id, err)
	}
	return cur, nil
}

// checkStatusExists refuses a status id outside the set; nil passes.
func checkStatusExists(tx *sql.Tx, statusID *string) error {
	if statusID == nil {
		return nil
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ade_task_statuses WHERE id = ?`, *statusID).Scan(&n); err != nil {
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
	if p.Notes != nil {
		cur.Notes = *p.Notes
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
func moveBranchKinds(tx *sql.Tx, id string, prev, to string) error {
	from, toKind := model.AdeBranchKindMine, model.AdeBranchKindParked
	if to == model.AdeTaskKindTask {
		from, toKind = model.AdeBranchKindParked, model.AdeBranchKindMine
		if prev == model.AdeTaskKindReview {
			from = model.AdeBranchKindReview
		}
	}
	if _, err := tx.Exec(`UPDATE ade_task_branches SET kind = ? WHERE task_id = ? AND kind = ?`, toKind, id, from); err != nil {
		return fmt.Errorf("repos: move ade task branches to %s: %w", toKind, err)
	}
	return nil
}

// AddBranch appends b to its task after the last branch. A task holds at most one live branch per
// repo (ErrRepoOnTask); a created branch name is unique among live branches of the repo
// (ErrBranchOnTask). b.Position is overwritten.
func (r *AdeTaskRepo) AddBranch(b model.AdeTaskBranch) (model.AdeTaskBranch, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: begin add ade task branch: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ade_tasks WHERE id = ? AND archived_at IS NULL`, b.TaskID).Scan(&exists); err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: check ade task %s: %w", b.TaskID, err)
	}
	if exists == 0 {
		return model.AdeTaskBranch{}, ErrTaskNotFound
	}
	var same int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ade_task_branches WHERE task_id = ? AND code_repo_id = ? AND archived_at IS NULL`,
		b.TaskID, b.CodeRepoID).Scan(&same); err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: check ade task repo: %w", err)
	}
	if same > 0 {
		return model.AdeTaskBranch{}, ErrRepoOnTask
	}
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position) + 1, 0) FROM ade_task_branches WHERE task_id = ?`, b.TaskID).Scan(&b.Position); err != nil {
		return model.AdeTaskBranch{}, fmt.Errorf("repos: next ade branch position: %w", err)
	}
	if err := insertTaskBranch(tx, b); err != nil {
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
	old, err := scanAdeTaskBranch(tx.QueryRow(`SELECT `+adeTaskBranchColumns+` FROM ade_task_branches
		WHERE id = ? AND archived_at IS NULL AND task_id IN (SELECT id FROM ade_tasks WHERE archived_at IS NULL)`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBranchMissing
	}
	if err != nil {
		return fmt.Errorf("repos: load ade branch %s: %w", id, err)
	}
	var runs int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ade_runs WHERE branch_id = ?`, id).Scan(&runs); err != nil {
		return fmt.Errorf("repos: count ade runs of branch %s: %w", id, err)
	}
	if old.Name == "" && runs == 0 {
		_, err = tx.Exec(`DELETE FROM ade_task_branches WHERE id = ?`, id)
	} else {
		_, err = tx.Exec(`UPDATE ade_task_branches SET archived_at = ? WHERE id = ?`, now, id)
	}
	if err != nil {
		return fmt.Errorf("repos: detach ade branch %s: %w", id, err)
	}
	fill := replacement != nil && old.Name == "" && replacement.CodeRepoID == old.CodeRepoID
	if replacement != nil {
		replacement.Position = old.Position
		if err := insertTaskBranch(tx, *replacement); err != nil {
			return err
		}
	}
	if fill {
		// The replacement fills the same not-created row, so what depended on the row keeps depending on it.
		if _, err := tx.Exec(`UPDATE ade_task_branches SET base_branch_id = ? WHERE base_branch_id = ? AND archived_at IS NULL`, replacement.ID, id); err != nil {
			return fmt.Errorf("repos: repoint ade branches stacked on %s: %w", id, err)
		}
		if _, err := tx.Exec(`UPDATE ade_task_branches SET queued_after = ? WHERE queued_after = ?`, replacement.ID, id); err != nil {
			return fmt.Errorf("repos: repoint ade branches queued after %s: %w", id, err)
		}
	} else {
		newBase := cmp.Or(old.Name, old.Base)
		if _, err := tx.Exec(`UPDATE ade_task_branches SET base_branch_id = '', base = ?, base_pending_from = ''
			WHERE base_branch_id = ? AND archived_at IS NULL`, newBase, id); err != nil {
			return fmt.Errorf("repos: free ade branches stacked on %s: %w", id, err)
		}
		if _, err := tx.Exec(`UPDATE ade_task_branches SET queued_after = '' WHERE queued_after = ?`, id); err != nil {
			return fmt.Errorf("repos: free ade branches queued after %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit detach ade branch %s: %w", id, err)
	}
	return nil
}

func (r *AdeTaskRepo) queryBranches(where string, args ...any) ([]model.AdeTaskBranch, error) {
	rows, err := r.DB.Query(`SELECT b.id, b.task_id, b.code_repo_id, b.name, b.kind, b.base, b.base_branch_id, b.base_pending_from, b.queued_after, b.position,
		b.had_commits, b.added_at, b.merged_at, b.archived_at, b.origin, b.cr_number, b.cr_title, b.cr_state, b.cr_url, b.review_comments
		FROM ade_task_branches b JOIN ade_tasks t ON t.id = b.task_id WHERE `+where+` ORDER BY b.task_id, b.position, b.id`, args...)
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
	return r.queryBranches(`t.archived_at IS NULL AND b.archived_at IS NULL`)
}

// BranchesArchived returns the branches of archived tasks (history).
func (r *AdeTaskRepo) BranchesArchived() ([]model.AdeTaskBranch, error) {
	return r.queryBranches(`t.archived_at IS NOT NULL`)
}

// BranchesOfTask returns every branch row of one task, archived or not, in position order.
func (r *AdeTaskRepo) BranchesOfTask(taskID string) ([]model.AdeTaskBranch, error) {
	return r.queryBranches(`b.task_id = ?`, taskID)
}

// GetBranch returns one branch by id; ErrBranchMissing if absent.
func (r *AdeTaskRepo) GetBranch(id string) (model.AdeTaskBranch, error) {
	b, err := scanAdeTaskBranch(r.DB.QueryRow(`SELECT `+adeTaskBranchColumns+` FROM ade_task_branches WHERE id = ?`, id))
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
	rows, err := r.DB.Query(`SELECT task_id, day, position, backlog FROM ade_task_plan ORDER BY position, task_id`)
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
	rows, err := tx.Query(`SELECT task_id FROM ade_task_plan ORDER BY position, task_id`)
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
		res, err := tx.Exec(`UPDATE ade_task_plan SET position = ? WHERE task_id = ?`, i, id)
		if err != nil {
			return fmt.Errorf("repos: set ade task plan position %s: %w", id, err)
		}
		if err := sqlitex.RequireOneRow(res, "ade task plan row "+id); err != nil {
			return err
		}
	}
	for id, day := range days {
		res, err := tx.Exec(`UPDATE ade_task_plan SET day = ?, backlog = 0 WHERE task_id = ?`, day, id)
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
	err = tx.QueryRow(`SELECT archived_at IS NULL FROM ade_tasks WHERE id = ?`, taskID).Scan(&live)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
		return ErrTaskNotFound
	}
	if err != nil {
		return fmt.Errorf("repos: load ade task %s: %w", taskID, err)
	}
	if _, err := tx.Exec(`INSERT INTO ade_task_plan (task_id, day, position, backlog)
		SELECT ?, NULL, COALESCE(MAX(position) + 1, 0), ? FROM ade_task_plan WHERE true
		ON CONFLICT (task_id) DO UPDATE SET day = NULL, backlog = excluded.backlog`, taskID, boolInt(backlog)); err != nil {
		return fmt.Errorf("repos: upsert ade task plan %s: %w", taskID, err)
	}
	rows, err := tx.Query(`SELECT p.task_id, p.day IS NULL AND p.backlog = ? AND (? = 1 OR t.kind <> 'review')
		FROM ade_task_plan p JOIN ade_tasks t ON t.id = p.task_id ORDER BY p.position, p.task_id`, boolInt(backlog), boolInt(backlog))
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
		if _, err := tx.Exec(`UPDATE ade_task_plan SET position = ? WHERE task_id = ?`, i, id); err != nil {
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
	res, err := r.DB.Exec(`UPDATE ade_task_branches SET queued_after = ? WHERE id = ?`, after, branchID)
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
	res, err := r.DB.Exec(`UPDATE ade_task_branches SET base = ?, base_branch_id = ?, base_pending_from = ? WHERE id = ?`,
		base, baseBranchID, pendingFrom, id)
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
	for _, id := range ids {
		if _, err := r.DB.Exec(`UPDATE ade_task_branches SET base_pending_from = '' WHERE id = ?`, id); err != nil {
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
	for _, id := range hadCommits {
		if _, err := tx.Exec(`UPDATE ade_task_branches SET had_commits = 1 WHERE id = ? AND had_commits = 0`, id); err != nil {
			return fmt.Errorf("repos: mark ade branch had commits %s: %w", id, err)
		}
	}
	for id, at := range merged {
		if _, err := tx.Exec(`UPDATE ade_task_branches SET merged_at = ? WHERE id = ? AND merged_at IS NULL`, at, id); err != nil {
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
	return r.queryRunsByTask(` WHERE task_id IN (SELECT id FROM ade_tasks WHERE archived_at IS NULL)`)
}

// RunsOfTask returns one task's runs, oldest first.
func (r *AdeTaskRepo) RunsOfTask(taskID string) ([]model.AdeRun, error) {
	by, err := r.queryRunsByTask(` WHERE task_id = ?`, taskID)
	return by[taskID], err
}

func (r *AdeTaskRepo) queryRunsByTask(where string, args ...any) (map[string][]model.AdeRun, error) {
	rows, err := r.DB.Query(`SELECT `+adeRunColumns+` FROM ade_runs`+where+` ORDER BY COALESCE(started_at, 0), id`, args...)
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
	rows, err := r.DB.Query(`SELECT branch_id, state, started_at, finished_at, exit_code, note FROM ade_worktree_setup`)
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

// LastValid returns the last valid workflow JSON recorded per file name.
func (r *AdeTaskRepo) LastValid() (map[string]string, error) {
	rows, err := r.DB.Query(`SELECT file_name, workflow_json FROM ade_workflow_last_valid`)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade workflow last valid: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var file, js string
		if err := rows.Scan(&file, &js); err != nil {
			return nil, fmt.Errorf("repos: scan ade workflow last valid: %w", err)
		}
		out[file] = js
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade workflow last valid rows: %w", err)
	}
	return out, nil
}

// RecordLastValid upserts the last valid workflow JSON for a file.
func (r *AdeTaskRepo) RecordLastValid(file, workflowJSON string, now int64) error {
	if _, err := r.DB.Exec(`INSERT INTO ade_workflow_last_valid (file_name, workflow_json, recorded_at) VALUES (?, ?, ?)
		ON CONFLICT(file_name) DO UPDATE SET workflow_json = excluded.workflow_json, recorded_at = excluded.recorded_at`,
		file, workflowJSON, now); err != nil {
		return fmt.Errorf("repos: record ade workflow last valid %s: %w", file, err)
	}
	return nil
}

// ForgetLastValid drops the last valid workflow row of a file that no longer exists.
func (r *AdeTaskRepo) ForgetLastValid(file string) error {
	if _, err := r.DB.Exec(`DELETE FROM ade_workflow_last_valid WHERE file_name = ?`, file); err != nil {
		return fmt.Errorf("repos: forget ade workflow last valid %s: %w", file, err)
	}
	return nil
}

// SetBranchName records the created branch name; ErrBranchNameTaken when a live branch of the repo
// already holds it, ErrBranchMissing when the branch is gone.
func (r *AdeTaskRepo) SetBranchName(id, name string) error {
	res, err := r.DB.Exec(`UPDATE ade_task_branches SET name = ? WHERE id = ?`, name, id)
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

// SetBranchOrigin marks who made the branch row ("" user, "agent").
func (r *AdeTaskRepo) SetBranchOrigin(id, origin string) error {
	res, err := r.DB.Exec(`UPDATE ade_task_branches SET origin = ? WHERE id = ?`, origin, id)
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
	if since == nil {
		res, err = tx.Exec(`UPDATE ade_tasks SET entry_step_id = ? WHERE id = ?`, stepID, taskID)
	} else {
		var js []byte
		if js, err = json.Marshal(since); err != nil {
			return fmt.Errorf("repos: encode ade task since %s: %w", taskID, err)
		}
		res, err = tx.Exec(`UPDATE ade_tasks SET entry_step_id = ?, since_json = ? WHERE id = ?`, stepID, string(js), taskID)
	}
	if err != nil {
		return fmt.Errorf("repos: set ade task entry step %s: %w", taskID, err)
	}
	if err := sqlitex.RequireOneRow(res, "ade task "+taskID); err != nil {
		return err
	}
	if handoff != nil {
		if err := applyHandoff(tx, taskID, *handoff); err != nil {
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
	const runIDs = `SELECT id FROM ade_runs WHERE task_id = ?`
	if _, err := tx.Exec(`DELETE FROM ade_log_chunks WHERE kind = 'run' AND id IN (`+runIDs+`)`, taskID); err != nil {
		return fmt.Errorf("repos: delete ade run log chunks %s: %w", taskID, err)
	}
	if _, err := tx.Exec(`DELETE FROM ade_logs WHERE kind = 'run' AND id IN (`+runIDs+`)`, taskID); err != nil {
		return fmt.Errorf("repos: delete ade run logs %s: %w", taskID, err)
	}
	if _, err := tx.Exec(`DELETE FROM ade_runs WHERE task_id = ?`, taskID); err != nil {
		return fmt.Errorf("repos: delete ade runs %s: %w", taskID, err)
	}
	res, err := tx.Exec(`UPDATE ade_tasks SET workflow_id = ?, entry_step_id = ?, since_json = NULL, workflow_json = NULL, workflow_hash = '' WHERE id = ?`,
		workflowID, entryStepID, taskID)
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
	res, err := r.DB.Exec(`UPDATE ade_tasks SET status_id = ?, status_by = ?, status_step = ? WHERE id = ?`,
		statusID, model.AdeStatusByAuto, step, taskID)
	if err != nil {
		return fmt.Errorf("repos: set ade task status %s: %w", taskID, err)
	}
	return sqlitex.RequireOneRow(res, "ade task "+taskID)
}

// HasRunning reports whether any run of the task is running.
func (r *AdeTaskRepo) HasRunning(taskID string) (bool, error) {
	var n int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM ade_runs WHERE task_id = ? AND state = ?`, taskID, model.AdeRunRunning).Scan(&n); err != nil {
		return false, fmt.Errorf("repos: count running ade runs %s: %w", taskID, err)
	}
	return n > 0, nil
}

// SetWorkflowSnapshot stores the whole workflow a task began with. A task that already has one keeps
// it: the first snapshot wins.
func (r *AdeTaskRepo) SetWorkflowSnapshot(taskID, workflowJSON, hash string) error {
	if _, err := r.DB.Exec(`UPDATE ade_tasks SET workflow_json = ?, workflow_hash = ? WHERE id = ? AND workflow_json IS NULL`,
		workflowJSON, hash, taskID); err != nil {
		return fmt.Errorf("repos: set ade task workflow snapshot %s: %w", taskID, err)
	}
	return nil
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
	if _, err := r.DB.Exec(`INSERT INTO ade_runs (`+adeRunColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.TaskID, run.StepID, run.BranchID, run.Attempt, run.State,
		run.Loops, run.Note, run.Summary, run.SessionID, run.ExitCode, run.StartedAt, run.FinishedAt, outcome, run.Purpose, run.SpecJSON,
		run.Launch.Note, run.Launch.ResumeID, run.Launch.Prompt, run.Launch.Extra); err != nil {
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
	cur, err := scanAdeRun(tx.QueryRow(`SELECT `+adeRunColumns+` FROM ade_runs WHERE id = ?`, id))
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
	if _, err := tx.Exec(`UPDATE ade_runs SET state = ?, note = ?, summary = ?, session_id = ?,
		exit_code = ?, started_at = ?, finished_at = ?, outcome_json = ?, launch_note = ?, launch_resume_id = ?, launch_prompt = ?,
		launch_extra = ?, spec_json = ? WHERE id = ?`,
		cur.State, cur.Note, cur.Summary, cur.SessionID, cur.ExitCode, cur.StartedAt,
		cur.FinishedAt, outcome, cur.Launch.Note, cur.Launch.ResumeID, cur.Launch.Prompt, cur.Launch.Extra, cur.SpecJSON, id); err != nil {
		return model.AdeRun{}, fmt.Errorf("repos: update ade run %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return model.AdeRun{}, fmt.Errorf("repos: commit update ade run %s: %w", id, err)
	}
	return cur, nil
}

// GetRun returns one run; ErrRunNotFound if absent.
func (r *AdeTaskRepo) GetRun(id string) (model.AdeRun, error) {
	run, err := scanAdeRun(r.DB.QueryRow(`SELECT `+adeRunColumns+` FROM ade_runs WHERE id = ?`, id))
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
	rows, err := r.DB.Query(`SELECT `+adeRunColumns+` FROM ade_runs a WHERE task_id = ? AND attempt = (
		SELECT MAX(attempt) FROM ade_runs b WHERE b.task_id = a.task_id
		AND b.step_id = a.step_id AND b.branch_id = a.branch_id AND b.purpose = a.purpose) ORDER BY COALESCE(started_at, 0), id`, taskID)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

// UpsertSetup writes a branch's worktree-setup row.
func (r *AdeTaskRepo) UpsertSetup(s model.AdeWorktreeSetup) error {
	if _, err := r.DB.Exec(`INSERT INTO ade_worktree_setup (branch_id, state, started_at, finished_at, exit_code, note)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(branch_id) DO UPDATE SET state = excluded.state,
		started_at = excluded.started_at, finished_at = excluded.finished_at, exit_code = excluded.exit_code,
		note = excluded.note`,
		s.BranchID, s.State, s.StartedAt, s.FinishedAt, s.ExitCode, s.Note); err != nil {
		return fmt.Errorf("repos: upsert ade worktree setup %s: %w", s.BranchID, err)
	}
	return nil
}

// GetSetup returns a branch's setup row; (nil, nil) when it has none.
func (r *AdeTaskRepo) GetSetup(branchID string) (*model.AdeWorktreeSetup, error) {
	var s model.AdeWorktreeSetup
	var finished, exit sql.NullInt64
	err := r.DB.QueryRow(`SELECT branch_id, state, started_at, finished_at, exit_code, note FROM ade_worktree_setup WHERE branch_id = ?`,
		branchID).Scan(&s.BranchID, &s.State, &s.StartedAt, &finished, &exit, &s.Note)
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
	if _, err := r.DB.Exec(`UPDATE ade_runs SET note = ? WHERE branch_id = ? AND state = ?`, note, branchID, model.AdeRunPending); err != nil {
		return nil, fmt.Errorf("repos: set pending ade run note %s: %w", branchID, err)
	}
	rows, err := r.DB.Query(`SELECT `+adeRunColumns+` FROM ade_runs WHERE branch_id = ? AND state = ? ORDER BY id`, branchID, model.AdeRunPending)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

// GetReview returns the review run's send state; (nil, nil) when it has none.
func (r *AdeTaskRepo) GetReview(runID string) (*model.AdeReview, error) {
	var v model.AdeReview
	err := r.DB.QueryRow(`SELECT run_id, route, target_run_id, dedicated_claude_id, rounds FROM ade_reviews WHERE run_id = ?`, runID).
		Scan(&v.RunID, &v.Route, &v.TargetRunID, &v.DedicatedClaudeID, &v.Rounds)
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
	if _, err := r.DB.Exec(`INSERT INTO ade_reviews (run_id, route, target_run_id) VALUES (?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET route = excluded.route, target_run_id = excluded.target_run_id`,
		v.RunID, v.Route, v.TargetRunID); err != nil {
		return fmt.Errorf("repos: upsert ade review %s: %w", v.RunID, err)
	}
	return nil
}

// BumpReviewRound counts one more round and returns the new count.
func (r *AdeTaskRepo) BumpReviewRound(runID string) (int, error) {
	var n int
	if err := r.DB.QueryRow(`UPDATE ade_reviews SET rounds = rounds + 1 WHERE run_id = ? RETURNING rounds`, runID).Scan(&n); err != nil {
		return 0, fmt.Errorf("repos: bump ade review round %s: %w", runID, err)
	}
	return n, nil
}

// SetReviewDedicated stores the dedicated route's Claude session id ("" forgets a lost one).
func (r *AdeTaskRepo) SetReviewDedicated(runID, claudeID string) error {
	if _, err := r.DB.Exec(`UPDATE ade_reviews SET dedicated_claude_id = ? WHERE run_id = ?`, claudeID, runID); err != nil {
		return fmt.Errorf("repos: set ade review session %s: %w", runID, err)
	}
	return nil
}

// Reviews returns the send state of every review run, by run id.
func (r *AdeTaskRepo) Reviews() (map[string]model.AdeReview, error) {
	rows, err := r.DB.Query(`SELECT run_id, route, target_run_id, dedicated_claude_id, rounds FROM ade_reviews
		WHERE run_id IN (SELECT id FROM ade_runs WHERE task_id IN (SELECT id FROM ade_tasks WHERE archived_at IS NULL))`)
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
	rows, err := r.DB.Query(`SELECT `+adeRunColumns+` FROM ade_runs WHERE task_id = ? AND purpose = ? ORDER BY attempt, id`,
		taskID, model.AdeRunPurposeReview)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

// PendingOnBranch lists the pending runs of a branch with the given purpose.
func (r *AdeTaskRepo) PendingOnBranch(branchID, purpose string) ([]model.AdeRun, error) {
	rows, err := r.DB.Query(`SELECT `+adeRunColumns+` FROM ade_runs WHERE branch_id = ? AND purpose = ? AND state = ? ORDER BY id`,
		branchID, purpose, model.AdeRunPending)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

// CountRunning counts runs in the running state.
func (r *AdeTaskRepo) CountRunning() (int, error) {
	var n int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM ade_runs WHERE state = 'running'`).Scan(&n); err != nil {
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
	rows, err := r.DB.Query(`UPDATE ade_runs SET state = ?, note = ?, finished_at = ?, outcome_json = ?,
		launch_note = '', launch_prompt = '', launch_extra = '',
		launch_resume_id = CASE WHEN purpose IN ('', 'review') AND session_id <> '' THEN
			COALESCE((SELECT agent_session_id FROM ade_sessions s WHERE s.id = ade_runs.session_id), '') ELSE '' END
		WHERE state = 'running' RETURNING `+adeRunColumns,
		model.AdeRunStuck, out.Reason, now, outcome)
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
	rows, err := tx.Query(`SELECT b.id, b.task_id FROM ade_worktree_setup s JOIN ade_task_branches b ON b.id = s.branch_id
		WHERE s.state = 'running' ORDER BY b.id`)
	out, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeTaskBranch, bool, error) {
		var b model.AdeTaskBranch
		err := rows.Scan(&b.ID, &b.TaskID)
		return b, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("repos: select running ade setups: %w", err)
	}
	if _, err := tx.Exec(`UPDATE ade_worktree_setup SET state = ?, finished_at = ?, note = ? WHERE state = 'running'`,
		model.AdeSetupFailed, now, note); err != nil {
		return nil, fmt.Errorf("repos: recover ade setups: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("repos: commit recover ade setups: %w", err)
	}
	return out, nil
}

// CountRuns counts a step's runs on one branch in the given state, across attempts.
func (r *AdeTaskRepo) CountRuns(taskID, stepID, branchID, state string) (int, error) {
	var n int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM ade_runs WHERE task_id = ? AND step_id = ? AND branch_id = ? AND state = ?`,
		taskID, stepID, branchID, state).Scan(&n); err != nil {
		return 0, fmt.Errorf("repos: count ade runs %s: %w", taskID, err)
	}
	return n, nil
}

// HasRunningOn reports whether a run on the branch is running.
func (r *AdeTaskRepo) HasRunningOn(branchID string) (bool, error) {
	var n int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM ade_runs WHERE branch_id = ? AND state = ?`, branchID, model.AdeRunRunning).Scan(&n); err != nil {
		return false, fmt.Errorf("repos: count running ade runs on %s: %w", branchID, err)
	}
	return n > 0, nil
}

// HasActiveStepRunOn reports whether a step run (not a rebase) on the branch is running or pending.
func (r *AdeTaskRepo) HasActiveStepRunOn(branchID string) (bool, error) {
	var n int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM ade_runs WHERE branch_id = ? AND purpose = '' AND state IN (?, ?)`,
		branchID, model.AdeRunRunning, model.AdeRunPending).Scan(&n); err != nil {
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
	rows, err := r.DB.Query(`SELECT `+adeRunColumns+` FROM ade_runs WHERE purpose = ? AND state = ? ORDER BY id`,
		model.AdeRunPurposeRebase, model.AdeRunRunning)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeRun, bool, error) {
		run, err := scanAdeRun(rows)
		return run, true, err
	})
}

// LatestRebaseRun returns the newest rebase run rooted at the branch, nil when none.
func (r *AdeTaskRepo) LatestRebaseRun(branchID string) (*model.AdeRun, error) {
	run, err := scanAdeRun(r.DB.QueryRow(`SELECT `+adeRunColumns+` FROM ade_runs WHERE branch_id = ? AND purpose = ?
		ORDER BY attempt DESC, id DESC LIMIT 1`, branchID, model.AdeRunPurposeRebase))
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
	var n int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM ade_runs WHERE branch_id = ? AND purpose = ?`, branchID, model.AdeRunPurposeRebase).Scan(&n); err != nil {
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
	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE ade_task_branches SET cr_number = NULL, cr_title = NULL, cr_state = NULL, cr_url = NULL WHERE id = ?`, id); err != nil {
			return fmt.Errorf("repos: clear ade branch %s change request: %w", id, err)
		}
	}
	for id, cr := range crs {
		if _, err := tx.Exec(`UPDATE ade_task_branches SET cr_number = ?, cr_title = ?, cr_state = ?, cr_url = ? WHERE id = ?`,
			cr.Number, cr.Title, cr.State, cr.URL, id); err != nil {
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
	if _, err := r.DB.Exec(`UPDATE ade_task_branches SET review_comments = ? WHERE id = ?`, v, id); err != nil {
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
	t, err := scanAdeTask(tx.QueryRow(`SELECT `+adeTaskColumns+` FROM ade_tasks WHERE id = ?`, taskID))
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
	err = tx.QueryRow(`SELECT mine.name, COALESCE(NULLIF(other_t.title, ''), other_t.jira_key, '')
		FROM ade_task_branches mine
		JOIN ade_task_branches other ON other.code_repo_id = mine.code_repo_id AND other.name = mine.name AND other.archived_at IS NULL
		JOIN ade_tasks other_t ON other_t.id = other.task_id
		WHERE mine.task_id = ? AND mine.archived_at = ? AND mine.name <> '' LIMIT 1`, taskID, *t.ArchivedAt).Scan(&held, &holder)
	if err == nil {
		return &BranchHeldError{Branch: held, TaskTitle: holder}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("repos: check ade task %s branch names: %w", taskID, err)
	}
	if _, err := tx.Exec(`UPDATE ade_task_branches SET archived_at = NULL WHERE task_id = ? AND archived_at = ?`, taskID, *t.ArchivedAt); err != nil {
		return fmt.Errorf("repos: restore ade task branches %s: %w", taskID, err)
	}
	if _, err := tx.Exec(`UPDATE ade_tasks SET archived_at = NULL WHERE id = ?`, taskID); err != nil {
		return fmt.Errorf("repos: restore ade task %s: %w", taskID, err)
	}
	if _, err := tx.Exec(`INSERT INTO ade_task_plan (task_id, day, position)
		SELECT ?, NULL, COALESCE(MAX(position) + 1, 0) FROM ade_task_plan`, taskID); err != nil {
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
	res, err := tx.Exec(`UPDATE ade_tasks SET archived_at = ? WHERE id = ? AND archived_at IS NULL`, now, taskID)
	if err != nil {
		return fmt.Errorf("repos: archive ade task %s: %w", taskID, err)
	}
	if err := sqlitex.RequireOneRow(res, "ade task "+taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE ade_task_branches SET archived_at = ? WHERE task_id = ? AND archived_at IS NULL`, now, taskID); err != nil {
		return fmt.Errorf("repos: archive ade task branches %s: %w", taskID, err)
	}
	if _, err := tx.Exec(`DELETE FROM ade_task_plan WHERE task_id = ?`, taskID); err != nil {
		return fmt.Errorf("repos: drop ade task plan row %s: %w", taskID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit archive ade task %s: %w", taskID, err)
	}
	return nil
}
