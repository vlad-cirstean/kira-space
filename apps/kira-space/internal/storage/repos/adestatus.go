package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ErrStatusSet marks a status set Save refuses; the wrapped text names the failed rule.
var ErrStatusSet = errors.New("repos: invalid task status set")

// AdeStatusRepo reads and rewrites the task status set (ade_task_statuses).
type AdeStatusRepo struct {
	DB *sql.DB
}

// queryer is satisfied by both *sql.DB and *sql.Tx, so one query/scan loop serves a plain call and
// a transaction.
type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func listStatuses(q queryer) ([]model.AdeTaskStatus, error) {
	rows, err := q.Query(`SELECT id, name, color, position FROM ade_task_statuses ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade task statuses: %w", err)
	}
	defer rows.Close()
	out := make([]model.AdeTaskStatus, 0)
	for rows.Next() {
		var s model.AdeTaskStatus
		if err := rows.Scan(&s.ID, &s.Name, &s.Color, &s.Position); err != nil {
			return nil, fmt.Errorf("repos: scan ade task status: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade task statuses rows: %w", err)
	}
	return out, nil
}

func defaultsOf(q queryer) (model.AdeStatusDefaults, error) {
	rows, err := q.Query(`SELECT slot, status_id FROM ade_task_status_defaults`)
	if err != nil {
		return model.AdeStatusDefaults{}, fmt.Errorf("repos: query ade task status defaults: %w", err)
	}
	defer rows.Close()
	var d model.AdeStatusDefaults
	for rows.Next() {
		var slot, id string
		if err := rows.Scan(&slot, &id); err != nil {
			return model.AdeStatusDefaults{}, fmt.Errorf("repos: scan ade task status default: %w", err)
		}
		switch slot {
		case model.AdeStatusSlotNew:
			d.New = id
		case model.AdeStatusSlotStep:
			d.Step = id
		case model.AdeStatusSlotReview:
			d.Review = id
		case model.AdeStatusSlotFinished:
			d.Finished = id
		}
	}
	if err := rows.Err(); err != nil {
		return model.AdeStatusDefaults{}, fmt.Errorf("repos: ade task status defaults rows: %w", err)
	}
	return d, nil
}

// statusForNew is the status a created task starts in: the review slot for a review task when set,
// else the new slot; ErrStatusUnknown when the new slot is empty.
func statusForNew(q queryer, review bool) (string, error) {
	d, err := defaultsOf(q)
	if err != nil {
		return "", err
	}
	if review && d.Review != "" {
		return d.Review, nil
	}
	if d.New == "" {
		return "", ErrStatusUnknown
	}
	return d.New, nil
}

// List returns the set in position order.
func (r *AdeStatusRepo) List() ([]model.AdeTaskStatus, error) {
	return listStatuses(r.DB)
}

// Defaults returns the status of each automatic change; an empty field keeps the current status.
func (r *AdeStatusRepo) Defaults() (model.AdeStatusDefaults, error) {
	return defaultsOf(r.DB)
}

func setErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrStatusSet, fmt.Sprintf(format, args...))
}

var slugRuns = regexp.MustCompile(`[^a-z0-9]+`)

// statusSlug turns a name into an id not in taken: lowercase, runs of other characters become "-",
// a collision gets -2, -3, ...
func statusSlug(name string, taken map[string]bool) string {
	base := strings.Trim(slugRuns.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(base) > model.AdeStatusIDMax {
		base = strings.Trim(base[:model.AdeStatusIDMax], "-")
	}
	if base == "" {
		base = "status"
	}
	id := base
	for n := 2; taken[id]; n++ {
		suffix := "-" + strconv.Itoa(n)
		cut := base
		if len(cut)+len(suffix) > model.AdeStatusIDMax {
			cut = strings.Trim(cut[:model.AdeStatusIDMax-len(suffix)], "-")
		}
		id = cut + suffix
	}
	return id
}

// Save rewrites the set to `set` in order and the defaults to `defaults`, and returns both. Tasks on a
// removed status move to reassign[removed], which every removed status needs, in the same transaction.
// A default naming a removed status moves to the same target.
// A status with an empty ID is new and gets a slug of its name.
func (r *AdeStatusRepo) Save(set []model.AdeTaskStatus, defaults model.AdeStatusDefaults, reassign map[string]string) ([]model.AdeTaskStatus, model.AdeStatusDefaults, error) {
	if len(set) < model.AdeStatusMin || len(set) > model.AdeStatusMax {
		return nil, model.AdeStatusDefaults{}, setErr("a set holds %d to %d statuses", model.AdeStatusMin, model.AdeStatusMax)
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, model.AdeStatusDefaults{}, fmt.Errorf("repos: begin save ade task statuses: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	old, err := listStatuses(tx)
	if err != nil {
		return nil, model.AdeStatusDefaults{}, err
	}
	oldByID := make(map[string]model.AdeTaskStatus, len(old))
	for _, s := range old {
		oldByID[s.ID] = s
	}

	next, taken, err := normalizeSet(set, oldByID)
	if err != nil {
		return nil, model.AdeStatusDefaults{}, err
	}
	defaults = moveRemovedDefaults(defaults, taken, oldByID, reassign)
	if err := defaults.Validate(next); err != nil {
		return nil, model.AdeStatusDefaults{}, setErr("%v", err)
	}
	if err := reassignRemoved(tx, old, next, taken, reassign); err != nil {
		return nil, model.AdeStatusDefaults{}, err
	}
	for _, s := range next {
		if _, err := tx.Exec(`INSERT INTO ade_task_statuses (id, name, color, position) VALUES (?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET name = excluded.name, color = excluded.color, position = excluded.position`,
			s.ID, s.Name, s.Color, s.Position); err != nil {
			return nil, model.AdeStatusDefaults{}, fmt.Errorf("repos: write ade task status %s: %w", s.ID, err)
		}
	}
	if err := writeDefaults(tx, defaults); err != nil {
		return nil, model.AdeStatusDefaults{}, err
	}
	if err := tx.Commit(); err != nil {
		return nil, model.AdeStatusDefaults{}, fmt.Errorf("repos: commit save ade task statuses: %w", err)
	}
	return next, defaults, nil
}

// moveRemovedDefaults points each default naming a removed status at that status's reassign target.
func moveRemovedDefaults(d model.AdeStatusDefaults, taken map[string]bool, oldByID map[string]model.AdeTaskStatus, reassign map[string]string) model.AdeStatusDefaults {
	move := func(id string) string {
		if _, existed := oldByID[id]; existed && !taken[id] {
			return reassign[id]
		}
		return id
	}
	d.New, d.Step, d.Review, d.Finished = move(d.New), move(d.Step), move(d.Review), move(d.Finished)
	return d
}

func writeDefaults(tx *sql.Tx, d model.AdeStatusDefaults) error {
	if _, err := tx.Exec(`DELETE FROM ade_task_status_defaults`); err != nil {
		return fmt.Errorf("repos: clear ade task status defaults: %w", err)
	}
	for _, e := range [...]struct{ slot, id string }{
		{model.AdeStatusSlotNew, d.New}, {model.AdeStatusSlotStep, d.Step},
		{model.AdeStatusSlotReview, d.Review}, {model.AdeStatusSlotFinished, d.Finished},
	} {
		if e.id == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO ade_task_status_defaults (slot, status_id) VALUES (?, ?)`, e.slot, e.id); err != nil {
			return fmt.Errorf("repos: write ade task status default %s: %w", e.slot, err)
		}
	}
	return nil
}

// normalizeSet validates set against the old rows, assigns slugs to new statuses and positions to all.
// taken holds every id of the result.
func normalizeSet(set []model.AdeTaskStatus, oldByID map[string]model.AdeTaskStatus) (next []model.AdeTaskStatus, taken map[string]bool, err error) {
	next = make([]model.AdeTaskStatus, len(set))
	taken = map[string]bool{}
	names := map[string]bool{}
	for i, s := range set {
		s.Name = strings.TrimSpace(s.Name)
		if err := s.Validate(); err != nil {
			return nil, nil, setErr("%v", err)
		}
		if s.ID != "" {
			if _, ok := oldByID[s.ID]; !ok {
				return nil, nil, setErr("status %q does not exist", s.ID)
			}
			if taken[s.ID] {
				return nil, nil, setErr("status %q appears twice", s.ID)
			}
			taken[s.ID] = true
		}
		key := strings.ToLower(s.Name)
		if names[key] {
			return nil, nil, setErr("two statuses are named %q", s.Name)
		}
		names[key] = true
		next[i] = s
	}
	// A new status never takes a removed id: reassignRemoved would read it as kept.
	reserved := maps.Clone(taken)
	for id := range oldByID {
		reserved[id] = true
	}
	for i := range next {
		if next[i].ID == "" {
			next[i].ID = statusSlug(next[i].Name, reserved)
			reserved[next[i].ID] = true
			taken[next[i].ID] = true
		}
		next[i].Position = i
	}
	return next, taken, nil
}

// reassignRemoved moves the tasks of each old status missing from taken to its target and deletes the row.
func reassignRemoved(tx *sql.Tx, old, next []model.AdeTaskStatus, taken map[string]bool, reassign map[string]string) error {
	for _, o := range old {
		if taken[o.ID] {
			continue
		}
		target := reassign[o.ID]
		if target == "" {
			return setErr("choose where the tasks of %q go", o.ID)
		}
		if !slices.ContainsFunc(next, func(s model.AdeTaskStatus) bool { return s.ID == target }) {
			return setErr("tasks of %q cannot move to %q: it is not in the set", o.ID, target)
		}
		if _, err := tx.Exec(`UPDATE ade_tasks SET status_id = ?, status_by = ? WHERE status_id = ?`, target, model.AdeStatusByUser, o.ID); err != nil {
			return fmt.Errorf("repos: reassign ade tasks of status %s: %w", o.ID, err)
		}
		if _, err := tx.Exec(`DELETE FROM ade_task_statuses WHERE id = ?`, o.ID); err != nil {
			return fmt.Errorf("repos: delete ade task status %s: %w", o.ID, err)
		}
	}
	return nil
}
