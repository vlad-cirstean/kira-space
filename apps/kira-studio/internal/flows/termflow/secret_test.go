package termflow

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// secretValue holds a quote so its JSON-escaped form differs from the plain one.
const secretValue = `s3"cr3t-9f2`

func secretParam() []scripts.Param {
	return []scripts.Param{{Name: "key", Type: scripts.ParamText, Required: true, Secret: true}}
}

func withSecret(v string) map[string]*string { return map[string]*string{"key": &v} }

// dbText is every cell of every table, as text.
func dbText(t *testing.T, db *sql.DB) string {
	t.Helper()
	tables, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var n string
		if err := tables.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	_ = tables.Close()
	var sb strings.Builder
	for _, n := range names {
		rows, err := db.Query(`SELECT * FROM "` + n + `"`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			cells := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cells {
				ptrs[i] = &cells[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for _, c := range cells {
				if b, ok := c.([]byte); ok {
					c = string(b)
				}
				sb.WriteString(fmt.Sprint(c))
				sb.WriteByte('\n')
			}
		}
		_ = rows.Close()
	}
	return sb.String()
}

// secretEnv is the KIRA_PARAM env the fake claude saw on its n-th call.
func secretEnv(t *testing.T, app *flowharness.App, n int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(app.FakeDir, fmt.Sprintf("automations-%d.env", n)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func secretRows(t *testing.T, app *flowharness.App) int {
	t.Helper()
	var n int
	if err := app.DB().QueryRow(`SELECT COUNT(*) FROM secret_values`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func noSecret(t *testing.T, what, text string) {
	t.Helper()
	if strings.Contains(text, secretValue) || strings.Contains(text, strings.ReplaceAll(secretValue, `"`, `\"`)) {
		t.Errorf("%s holds the secret", what)
	}
}

func createSecretScript(t *testing.T, app *flowharness.App, f scripts.CustomScriptFields, values map[string]*string) scripts.CustomScript {
	t.Helper()
	if f.Name == "" {
		f.Name = "keyed"
	}
	f.Color = "blue"
	f.Params = secretParam()
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: f, SecretValues: values})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return rec
}

func TestSecretParamSmartRun(t *testing.T) {
	app := flowharness.New(t)
	emit := filepath.Join(t.TempDir(), "emit")
	escaped, _ := json.Marshal(map[string]any{"command": "echo " + secretValue})
	lines := []string{
		"plain " + secretValue,
		fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":%s}]}}`, escaped),
	}
	if err := os.WriteFile(emit, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.Scenario(scenario(fakeagent.Action{Name: "fail", Emit: emit, Stderr: "stderr " + secretValue}))

	rec := createSecretScript(t, app, scripts.CustomScriptFields{Kind: scripts.KindSmart, Command: "Use the key"}, withSecret(secretValue))
	if len(rec.SavedSecrets) != 1 || rec.SavedSecrets[0] != "key" {
		t.Fatalf("savedSecrets = %v, want [key]", rec.SavedSecrets)
	}
	var sealed string
	if err := app.DB().QueryRow(`SELECT value FROM secret_values WHERE scope = 'script-param' AND owner = ? AND name = 'key'`, rec.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "kira:v3:") {
		t.Fatalf("stored value = %q, want a kira:v3: envelope", sealed)
	}
	snap, err := app.W.CustomScripts.List()
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(snap)
	noSecret(t, "snapshot", string(blob))

	mark := app.Events.Mark()
	args := scriptruns.RunArgs{ScriptID: rec.ID}
	pv, err := app.W.ScriptRuns.Preview(args)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Missing) != 0 || len(pv.Env) != 1 || !pv.Env[0].Saved || pv.Env[0].Value != "" || pv.SecretIssue != "" {
		t.Fatalf("preview = missing %v env %+v issue %q, want a saved secret with no value", pv.Missing, pv.Env, pv.SecretIssue)
	}
	pvBlob, _ := json.Marshal(pv)
	noSecret(t, "preview", string(pvBlob))

	id, _ := startSmart(t, app, args)
	run := waitSmart(t, app, id, "failed")
	if got := secretEnv(t, app, 1); got != strconv.Quote("KIRA_PARAM_KEY="+secretValue) {
		t.Errorf("child env = %s, want the saved value", got)
	}
	runBlob, _ := json.Marshal(run)
	noSecret(t, "run", string(runBlob))
	logs := logText(t, app, id)
	noSecret(t, "run log", logs)
	if !strings.Contains(logs, scripts.SecretMask) {
		t.Errorf("run log = %q, want the mask", logs)
	}
	if run.Outcome == nil || !strings.Contains(run.Outcome.LastError, scripts.SecretMask) {
		t.Errorf("outcome = %+v, want the masked last error", run.Outcome)
	}
	events, _ := json.Marshal(app.Events.Since(mark, ""))
	noSecret(t, "pushed events", string(events))
	noSecret(t, "database", dbText(t, app.DB()))
}

func TestSecretParamSchedule(t *testing.T) {
	app, clock := clockApp(t)
	rec := createSecretScript(t, app, scripts.CustomScriptFields{
		Command:  `echo "got=$KIRA_PARAM_KEY"`,
		Schedule: &scripts.Schedule{Cron: "* * * * *", Timezone: "UTC", Enabled: true},
	}, withSecret(secretValue))
	advance(t, clock, 30*time.Second)
	run := waitSched(t, app, rec.ID, 0, "done")
	logs := logText(t, app, run.ID)
	noSecret(t, "run log", logs)
	if !strings.Contains(logs, "got="+scripts.SecretMask) {
		t.Errorf("run log = %q, want the masked echo", logs)
	}

	f := scripts.CustomScriptFields{
		Name: rec.Name, Kind: rec.Kind, Command: rec.Command, Params: rec.Params, Schedule: rec.Schedule, DirMode: rec.DirMode, Color: rec.Color,
	}
	_, err := app.W.CustomScripts.Update(bridge.CustomScriptsUpdateArgs{ID: rec.ID, Fields: f, SecretValues: map[string]*string{"key": nil}})
	if err == nil || !strings.Contains(err.Error(), "needs a saved value for secret param key") {
		t.Fatalf("Update after Clear = %v, want the saved-value refusal", err)
	}
	if n := secretRows(t, app); n != 1 {
		t.Errorf("secret rows after a refused Clear = %d, want 1", n)
	}
}

func TestSecretParamTerminalRun(t *testing.T) {
	app := flowharness.New(t)
	rec := createSecretScript(t, app, scripts.CustomScriptFields{Command: `echo "tok=$KIRA_PARAM_KEY"; exit 3`}, withSecret(secretValue))
	startTerminalRun(t, app, "t-secret", scriptruns.RunArgs{ScriptID: rec.ID})
	run := waitRun(t, app, "t-secret", "failed")
	tail := run.Outcome.OutputTail
	if !strings.Contains(tail, "tok="+scripts.SecretMask) {
		t.Errorf("output tail = %q, want the masked echo", tail)
	}
	runBlob, _ := json.Marshal(run)
	noSecret(t, "run", string(runBlob))
	noSecret(t, "database", dbText(t, app.DB()))
}

func TestSecretParamResume(t *testing.T) {
	app := flowharness.New(t)
	hold := filepath.Join(t.TempDir(), "hold")
	app.Scenario(scenario(fakeagent.Action{WaitFile: hold}))
	rec := createSecretScript(t, app, scripts.CustomScriptFields{Kind: scripts.KindSmart, Command: "Say hi"}, withSecret(secretValue))
	id, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: rec.ID})
	waitSmart(t, app, id, "running")
	testx.WaitUntil(t, wait, func() bool {
		_, err := os.Stat(filepath.Join(app.FakeDir, "automations-1.args"))
		return err == nil
	})
	app.Restart(t)
	if interrupted := waitSmart(t, app, id, "failed"); !interrupted.Resumable {
		t.Fatalf("run = %+v, want resumable", interrupted)
	}
	app.Scenario(scenario(fakeagent.Action{Name: "done"}))
	started, err := app.W.ScriptRuns.Resume(scriptruns.IDArgs{ID: id})
	if err != nil {
		t.Fatalf("Resume with a saved value: %v", err)
	}
	waitSmart(t, app, started.RunID, "done")
	if got := secretEnv(t, app, 2); got != strconv.Quote("KIRA_PARAM_KEY="+secretValue) {
		t.Errorf("resumed env = %s, want the saved value", got)
	}
}

func TestSecretParamCleanup(t *testing.T) {
	app := flowharness.New(t)
	rec := createSecretScript(t, app, scripts.CustomScriptFields{Command: "true"}, withSecret(secretValue))
	if secretRows(t, app) != 1 {
		t.Fatal("no secret row after Create")
	}

	renamed := scripts.CustomScriptFields{
		Name: rec.Name, Kind: rec.Kind, Command: rec.Command, DirMode: rec.DirMode, Color: rec.Color,
		Params: []scripts.Param{{Name: "other", Type: scripts.ParamText, Secret: true}},
	}
	out, err := app.W.CustomScripts.Update(bridge.CustomScriptsUpdateArgs{ID: rec.ID, Fields: renamed})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.SavedSecrets) != 0 || secretRows(t, app) != 0 {
		t.Errorf("after rename: saved %v, rows %d, want none", out.SavedSecrets, secretRows(t, app))
	}

	if _, err := app.W.CustomScripts.Update(bridge.CustomScriptsUpdateArgs{ID: rec.ID, Fields: renamed, SecretValues: withNamed("ghost", secretValue)}); err == nil {
		t.Error("a value for a param that does not exist was accepted")
	}

	if _, err := app.W.CustomScripts.Update(bridge.CustomScriptsUpdateArgs{ID: rec.ID, Fields: renamed, SecretValues: withNamed("other", secretValue)}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.CustomScripts.Delete(bridge.CustomScriptsDeleteArgs{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}
	if secretRows(t, app) != 0 {
		t.Error("Delete left a secret row")
	}

	col, err := app.W.CustomScripts.CreateCollection(bridge.CustomScriptsCreateCollectionArgs{Name: "keys"})
	if err != nil {
		t.Fatal(err)
	}
	createSecretScript(t, app, scripts.CustomScriptFields{Name: "inner", Command: "true", CollectionID: &col.ID}, withSecret(secretValue))
	if secretRows(t, app) != 1 {
		t.Fatal("no secret row in the collection")
	}
	if err := app.W.CustomScripts.DeleteCollection(bridge.CustomScriptsDeleteCollectionArgs{ID: col.ID}); err != nil {
		t.Fatal(err)
	}
	if secretRows(t, app) != 0 {
		t.Error("DeleteCollection left a secret row")
	}
}

func withNamed(name, v string) map[string]*string { return map[string]*string{name: &v} }

func TestSecretParamStorageOff(t *testing.T) {
	app := flowharness.New(t, flowharness.WithoutSecretStore())
	snap, err := app.W.CustomScripts.List()
	if err != nil {
		t.Fatal(err)
	}
	if snap.SecretStorage.Available {
		t.Skip("this platform has a real keychain")
	}
	_, err = app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{
		Fields:       scripts.CustomScriptFields{Name: "keyed", Kind: scripts.KindScript, Command: "true", Color: "blue", Params: secretParam()},
		SecretValues: withSecret(secretValue),
	})
	if errCode(err) != "E_SECRET_STORE" {
		t.Fatalf("Create with a value = %v, want E_SECRET_STORE", err)
	}
	app.Scenario(scenario(fakeagent.Action{Name: "done"}))
	rec := createSecretScript(t, app, scripts.CustomScriptFields{Kind: scripts.KindSmart, Command: "Use the key"}, nil)
	id, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: rec.ID, Params: map[string][]string{"key": {secretValue}}})
	waitSmart(t, app, id, "done")
}
