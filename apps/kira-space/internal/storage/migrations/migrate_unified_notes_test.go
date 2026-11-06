package migrations

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// P543: migration 0077 copies each task's notes into the handover chain as a user note and drops
// ade_tasks.notes. The table rebuild must keep every handover row and both indexes.
func TestMigration77MovesNotesIntoChain(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/kira.sqlite?_foreign_keys=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	steps, err := All()
	if err != nil {
		t.Fatal(err)
	}
	apply := func(version int) {
		t.Helper()
		for _, m := range steps {
			if m.Version == version {
				if _, err := db.Exec(m.SQL); err != nil {
					t.Fatalf("apply v%d: %v", version, err)
				}
			}
		}
	}
	for v := 1; v <= 76; v++ {
		apply(v)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	task := func(id, notes string, created int64, archived any) {
		exec(`INSERT INTO ade_tasks (id, kind, title, notes, color, created_at, archived_at) VALUES (?, 'task', ?, ?, 0, ?, ?)`,
			id, id, notes, created, archived)
	}
	task("t1", "  keep **this**\nverbatim \n", 100, nil)
	task("t2", "   \n", 200, nil)
	task("t3", "archived note", 300, 999)
	task("t4", "", 400, nil)
	exec(`INSERT INTO ade_task_handovers (id, task_id, from_step_id, from_step_name, to_step_id, to_step_name, body, source, state, finished, dismissed, edited, created_at, updated_at, session_id)
		VALUES ('h1', 't1', 'a', 'A', 'b', 'B', 'handed body', 'run', 'handed', 0, 0, 1, 10, 11, 'sess-1'),
		       ('h2', 't1', 'b', 'B', '', '', 'draft body', 'agent', 'draft', 1, 0, 0, 20, 21, 'sess-2')`)

	apply(77)

	rows, err := db.Query(`SELECT task_id, body, from_step_id, source, created_at, updated_at FROM ade_task_handovers WHERE state = 'note' ORDER BY task_id`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var task, body, step, source string
		var created, updated int64
		if err := rows.Scan(&task, &body, &step, &source, &created, &updated); err != nil {
			t.Fatal(err)
		}
		got = append(got, strings.Join([]string{task, body, step, source}, "|"))
		if created != updated || (task == "t1" && created != 100) || (task == "t3" && created != 300) {
			t.Errorf("note of %s created %d updated %d", task, created, updated)
		}
	}
	rows.Close()
	want := []string{"t1|  keep **this**\nverbatim \n||user", "t3|archived note||user"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("notes = %q, want %q", got, want)
	}

	var h1, h2 string
	if err := db.QueryRow(`SELECT quote(from_step_id)||quote(from_step_name)||quote(to_step_id)||quote(to_step_name)||quote(body)||quote(source)||quote(state)||finished||dismissed||edited||created_at||updated_at||quote(session_id)
		FROM ade_task_handovers WHERE id = 'h1'`).Scan(&h1); err != nil || h1 != "'a''A''b''B''handed body''run''handed'0011011'sess-1'" {
		t.Errorf("h1 = %q, err %v", h1, err)
	}
	if err := db.QueryRow(`SELECT quote(body)||state||finished||quote(session_id) FROM ade_task_handovers WHERE id = 'h2'`).Scan(&h2); err != nil || h2 != "'draft body'draft1'sess-2'" {
		t.Errorf("h2 = %q, err %v", h2, err)
	}

	for _, idx := range []string{"ade_task_handovers_draft", "ade_task_handovers_task"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, idx).Scan(&n); err != nil || n != 1 {
			t.Errorf("index %s count = %d, err %v", idx, n, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO ade_task_handovers (id, task_id, from_step_id, from_step_name, body, source, state, created_at, updated_at)
		VALUES ('h3', 't1', 'b', 'B', 'second', 'agent', 'draft', 30, 30)`); err == nil {
		t.Error("second draft for one stage accepted, want the partial unique index to refuse it")
	}

	if _, err := db.Exec(`SELECT notes FROM ade_tasks`); err == nil {
		t.Error("ade_tasks.notes still exists")
	}
	exec(`DELETE FROM ade_tasks WHERE id = 't1'`)
	var left int
	if err := db.QueryRow(`SELECT count(*) FROM ade_task_handovers WHERE task_id = 't1'`).Scan(&left); err != nil || left != 0 {
		t.Errorf("t1 chain rows after cascade = %d, err %v", left, err)
	}
}
