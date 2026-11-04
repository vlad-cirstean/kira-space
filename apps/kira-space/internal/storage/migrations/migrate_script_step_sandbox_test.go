package migrations

import (
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"
)

// P510: migration 0068 rebuilds each started task's step array, so order and the other steps survive
// while script steps take the off sandbox.
func TestMigration67ScriptStepSandbox(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/kira.sqlite")
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
	for v := 1; v <= 67; v++ {
		apply(v)
	}
	const write = `{"access":"write","read":[],"allow":[],"network":true}`
	mixed := `{"id":"w","steps":[` +
		`{"id":"a","kind":"agent","sandbox":` + write + `},` +
		`{"id":"s","kind":"script","command":"make","sandbox":` + write + `},` +
		`{"id":"m","kind":"manual"},` +
		`{"id":"s2","kind":"script","command":"go test"},` +
		`{"id":"r","kind":"review","sandbox":` + write + `}]}`
	insert := func(id string, js any) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO ade_tasks (id, kind, color, created_at, workflow_json) VALUES (?, 'task', 0, 1, ?)`, id, js); err != nil {
			t.Fatal(err)
		}
	}
	insert("mixed", mixed)
	insert("null", nil)
	insert("empty", "")
	insert("bad", "{not json")
	apply(68)

	type step struct {
		ID      string          `json:"id"`
		Kind    string          `json:"kind"`
		Command string          `json:"command"`
		Sandbox json.RawMessage `json:"sandbox"`
	}
	var raw sql.NullString
	if err := db.QueryRow(`SELECT workflow_json FROM ade_tasks WHERE id = 'mixed'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Steps []step `json:"steps"`
	}
	if err := json.Unmarshal([]byte(raw.String), &wf); err != nil {
		t.Fatal(err)
	}
	const off = `{"access":"off","read":[],"allow":[],"network":true}`
	wantOrder := []string{"a", "s", "m", "s2", "r"}
	wantSandbox := map[string]string{"a": write, "s": off, "m": "", "s2": off, "r": write}
	if len(wf.Steps) != len(wantOrder) {
		t.Fatalf("steps = %d, want %d", len(wf.Steps), len(wantOrder))
	}
	for i, st := range wf.Steps {
		if st.ID != wantOrder[i] {
			t.Errorf("step %d = %s, want %s", i, st.ID, wantOrder[i])
		}
		if got := string(st.Sandbox); got != wantSandbox[st.ID] {
			t.Errorf("step %s sandbox = %q, want %q", st.ID, got, wantSandbox[st.ID])
		}
	}
	if wf.Steps[1].Command != "make" {
		t.Errorf("script command = %q", wf.Steps[1].Command)
	}
	for id, want := range map[string]sql.NullString{"null": {}, "empty": {Valid: true}, "bad": {String: "{not json", Valid: true}} {
		var got sql.NullString
		if err := db.QueryRow(`SELECT workflow_json FROM ade_tasks WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s snapshot = %+v, want %+v", id, got, want)
		}
	}
}
