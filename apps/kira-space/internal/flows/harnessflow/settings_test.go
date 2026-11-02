package harnessflow_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentsandbox/sandboxtest"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeprovider"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const settingsYAML = `env:
  P453_MARK: ${secret:gh-token}
mcp_servers:
  gh:
    url: http://127.0.0.1:9/mcp
    headers:
      Authorization: Bearer ${secret:gh-token}
hooks:
  mark:
    event: session_start
    command: echo mark
claude:
  status_line:
    command: echo user-status
  skip_hooks: []
`

// appFiles lists the per-run settings files left under root.
func appFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(filepath.Base(p), "-app.") && strings.HasSuffix(p, ".json") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func saveSettings(t *testing.T, app *flowharness.App, text string) agentcfg.View {
	t.Helper()
	cur, err := app.W.AgentSettingsSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	v, err := app.W.AgentSettingsSvc.Save(agentcfg.SaveArgs{Text: text, BaseHash: cur.Hash})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return v
}

// mcpWith returns the recorded --mcp-config copy that holds want.
func mcpWith(t *testing.T, stem, want string) string {
	t.Helper()
	files, _ := filepath.Glob(stem + ".mcp*")
	for _, f := range files {
		if b, _ := os.ReadFile(f); strings.Contains(string(b), want) {
			return string(b)
		}
	}
	t.Fatalf("no recorded mcp file of %s holds %q (files %v)", stem, want, files)
	return ""
}

func noMCPWith(t *testing.T, stem, unwanted string) {
	t.Helper()
	files, _ := filepath.Glob(stem + ".mcp*")
	for _, f := range files {
		if b, _ := os.ReadFile(f); strings.Contains(string(b), unwanted) {
			t.Fatalf("%s holds %q, want an isolated run without app settings", f, unwanted)
		}
	}
}

// columnsHolding names every column of every table of db whose text contains needle.
func columnsHolding(t *testing.T, db *sql.DB, needle string) []string {
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
	var hits []string
	for _, tbl := range names {
		cols, err := db.Query(fmt.Sprintf(`SELECT name FROM pragma_table_info('%s')`, tbl))
		if err != nil {
			t.Fatal(err)
		}
		var colNames []string
		for cols.Next() {
			var c string
			if err := cols.Scan(&c); err != nil {
				t.Fatal(err)
			}
			colNames = append(colNames, c)
		}
		_ = cols.Close()
		for _, c := range colNames {
			var n int
			q := fmt.Sprintf(`SELECT COUNT(*) FROM "%s" WHERE CAST("%s" AS TEXT) LIKE ?`, tbl, c)
			if err := db.QueryRow(q, "%"+needle+"%").Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n > 0 {
				hits = append(hits, tbl+"."+c)
			}
		}
	}
	return hits
}

// TestAgentSettingsReachEveryLaunch: the settings saved through the editor's bound service reach every
// launch that takes them (interactive, Take over, headless ADE steps, Continue in terminal), never an
// isolated smart script, and no plaintext secret lands anywhere kept. Frontend half:
// ade-agent-settings.spec.ts.
func TestAgentSettingsReachEveryLaunch(t *testing.T) {
	secret := "p453-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Setenv(fakeagent.EnvRecordEnv, "P453_MARK")
	fake := fakeprovider.New(map[string]json.RawMessage{})
	sbx := &sandboxtest.Fake{Dir: t.TempDir()}
	hold := filepath.Join(t.TempDir(), "hold")
	app := flowharness.New(t, flowharness.WithProvider(fake), flowharness.WithTrackerGrace(time.Second), flowharness.WithSandbox(sbx))
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	emit := filepath.Join(app.FakeDir, "emit.txt")
	if err := os.WriteFile(emit, []byte("token="+secret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": {
		{WaitFile: hold, Name: "done"}, {Emit: emit, Name: "done"}, {Name: "done"},
	}}})
	rec := newRecorder(t, app)
	svc := app.W.AgentSettingsSvc

	// Nothing saved yet: no file, storage available, each provider listed without files.
	v, err := svc.Get()
	if err != nil || v.Exists || !v.Storage.Available || len(v.Providers) != 3 {
		t.Fatalf("Get = %+v, %v, want no file, storage and three providers", v, err)
	}

	// Save and set the secret through the bound service.
	saveSettings(t, app, settingsYAML)
	if _, err := svc.SetSecret(agentcfg.SetSecretArgs{Name: "gh-token", Value: secret}); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if _, err := svc.SetSecret(agentcfg.SetSecretArgs{Name: "Bad Name", Value: "x"}); err == nil {
		t.Fatal("SetSecret with a bad name succeeded")
	}
	if _, err := svc.Save(agentcfg.SaveArgs{Text: "x", BaseHash: "stale"}); err == nil {
		t.Fatal("Save on a stale hash succeeded")
	}
	v, err = svc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Secrets) != 1 || !v.Secrets[0].Set || !v.Secrets[0].Referenced || v.Problem != nil {
		t.Fatalf("view = %+v, want gh-token set and referenced", v)
	}
	var claudeView agentcfg.ProviderView
	for _, p := range v.Providers {
		if p.ID == "claude" {
			claudeView = p
		}
	}
	var viewMCP string
	for _, f := range claudeView.Files {
		if f.Name == "mcp.json" {
			viewMCP = f.Text
		}
	}
	if !strings.Contains(viewMCP, "Bearer ${secret:gh-token}") {
		t.Fatalf("view mcp.json = %q, want the reference verbatim", viewMCP)
	}
	app.Contract(t, "agent-settings", "view", v)
	if b, _ := json.Marshal(v); strings.Contains(string(b), secret) {
		t.Fatal("the view holds the secret value")
	}

	// Claude interactive: an ad hoc session on a task branch.
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: window})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	repoRec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	tk := task(t, app, repoRec.ID, "flow", "id: flow\nname: Flow\nsteps:\n"+
		"  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n"+
		"  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n")
	row := branchOf(t, app, tk.ID, repoRec.ID)
	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "feat/api-set"})
	if err != nil {
		t.Fatal(err)
	}
	adhoc, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(adhoc.Command, secret) {
		t.Fatalf("interactive command holds the secret: %q", adhoc.Command)
	}
	openAgent(t, app, adhoc)
	stem := rec.nextStem()
	mcp := mcpWith(t, stem, "Bearer "+secret)
	if !strings.Contains(mcp, `"gh"`) {
		t.Fatalf("app mcp file = %q, want server gh", mcp)
	}
	if env, _ := os.ReadFile(stem + ".appenv"); strings.TrimSpace(string(env)) != "P453_MARK="+secret {
		t.Fatalf(".appenv = %q, want the resolved value", env)
	}
	settings, _ := os.ReadFile(stem + ".settings")
	if i, j := bytes.Index(settings, []byte("echo mark")), bytes.Index(settings, []byte("kira")); i < 0 || j < 0 || j > i {
		t.Fatalf("--settings = %s, want the user hook after Kira's entry", settings)
	}
	if !strings.Contains(string(settings), "statusLine") {
		t.Fatalf("--settings = %s, want the status line", settings)
	}
	if got := appFiles(t, app.SpaceHome); len(got) == 0 {
		t.Fatal("no app file while the session runs")
	}
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, adhoc.SessionID).State == "stopped" })
	testx.WaitUntil(t, waitFor, func() bool { return len(appFiles(t, app.SpaceHome)) == 0 })

	// Claude headless: an ADE step gets the same servers and a headless render.
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: tk.ID}); err != nil {
		t.Fatal(err)
	}
	stem = rec.nextStem()
	mcpWith(t, stem, "Bearer "+secret)
	if b, _ := os.ReadFile(stem + ".settings"); strings.Contains(string(b), "statusLine") || strings.Contains(string(b), "defaultMode") {
		t.Fatalf("headless --settings = %s, want no status line or permission mode", b)
	}
	testx.WaitUntil(t, waitFor, func() bool { return latestRun(t, app, tk.ID, "one").State == "done" })
	testx.WaitUntil(t, waitFor, func() bool { return len(appFiles(t, app.SpaceHome)) == 0 })

	// A smart script is isolated: no app file, no app env.
	smart := smartRun(t, app, "ask", "sonnet")
	stem = rec.nextStem()
	noMCPWith(t, stem, `"gh"`)
	if _, err := os.Stat(stem + ".appenv"); err == nil {
		t.Fatal("an isolated smart script got the app env")
	}

	// Continue in terminal gets the app file and the env through the token.
	resume, err := app.W.ScriptRuns.ResumeCommand(scriptruns.ResumeCommandArgs{RunID: smart.ID})
	if err != nil || !strings.Contains(resume.Command, "--mcp-config") || resume.Token == "" {
		t.Fatalf("ResumeCommand = %+v, %v, want an app file and a token", resume, err)
	}
	if strings.Contains(resume.Command, secret) {
		t.Fatalf("resume command holds the secret: %q", resume.Command)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "t-resume", Cwd: app.Work, Cols: 100, Rows: 30, WindowKey: window,
		Command: resume.Command, LaunchKind: terminal.LaunchKindAgent, AgentLaunchToken: resume.Token,
	}); err != nil {
		t.Fatalf("Terminal.Open: %v", err)
	}
	stem = rec.nextStem()
	mcpWith(t, stem, "Bearer "+secret)
	if env, _ := os.ReadFile(stem + ".appenv"); strings.TrimSpace(string(env)) != "P453_MARK="+secret {
		t.Fatalf("resume .appenv = %q, want the resolved value", env)
	}
	testx.WaitUntil(t, waitFor, func() bool { return len(appFiles(t, app.SpaceHome)) == 0 })

	// The fake provider: interactive and headless runs record the resolved config.
	setProvider(t, app, fakeprovider.ID)
	step, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: tk.ID})
	if err != nil {
		t.Fatal(err)
	}
	openAgent(t, app, step)
	stem = rec.nextStem()
	if cfg, _ := os.ReadFile(stem + ".appconfig"); !strings.Contains(string(cfg), secret) {
		t.Fatalf("fake interactive config = %q, want the resolved value", cfg)
	}
	if env, _ := os.ReadFile(stem + ".appenv"); strings.TrimSpace(string(env)) != "P453_MARK="+secret {
		t.Fatalf("fake interactive .appenv = %q", env)
	}
	setProvider(t, app, "claude")

	// A missing secret fails the launch by name and writes nothing.
	if _, err := svc.DeleteSecret(agentcfg.DeleteSecretArgs{Name: "gh-token"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := svc.Get(); len(v.Secrets) != 1 || v.Secrets[0].Set {
		t.Fatalf("secrets after delete = %+v, want gh-token referenced and not set", v.Secrets)
	}
	before := len(appFiles(t, app.SpaceHome)) // the fake provider's session is still open
	if _, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID}); err == nil || !strings.Contains(err.Error(), "gh-token") {
		t.Fatalf("StartAdHoc with a missing secret = %v, want an error naming gh-token", err)
	}
	if got := appFiles(t, app.SpaceHome); len(got) != before {
		t.Fatalf("a failed launch left %v", got)
	}

	// An invalid edit keeps the last good settings and reports its line.
	broken := strings.Replace(settingsYAML, "event: session_start", "event: nope", 1)
	bv := saveSettings(t, app, broken)
	if bv.Problem == nil || bv.Problem.Line == 0 || !bv.LastGood {
		t.Fatalf("view of an invalid edit = %+v, want a problem line and last good", bv)
	}

	// No plaintext secret in the database, the sealed store, or any file kept under the app homes.
	if hits := columnsHolding(t, app.DB(), secret); len(hits) != 0 {
		t.Fatalf("kira.db holds the secret in %v", hits)
	}
	if hits := columnsHolding(t, app.DB(), "token=••••"); len(hits) == 0 {
		t.Fatal("the headless step's stored log has no masked line")
	}
	var js string
	if err := app.DB().QueryRow(`SELECT value_json FROM config_last_valid WHERE folder = 'agent' AND file = 'settings.yaml'`).Scan(&js); err != nil || !strings.Contains(js, "${secret:gh-token}") {
		t.Fatalf("projection row = %q, %v, want the reference", js, err)
	}
	sealed, err := sqlitex.OpenImmediate(filepath.Join(app.SpaceHome, "agent-secrets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sealed.Close() }()
	_, _ = svc.SetSecret(agentcfg.SetSecretArgs{Name: "gh-token", Value: secret})
	if hits := columnsHolding(t, sealed, secret); len(hits) != 0 {
		t.Fatalf("agent-secrets.db holds the secret in %v", hits)
	}
	_ = filepath.WalkDir(app.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(p, app.FakeDir) || strings.HasPrefix(p, app.Work) || strings.Contains(filepath.Base(p), "-app.") {
			return nil // a live session's own 0600 file; its release is asserted above
		}
		if b, _ := os.ReadFile(p); strings.Contains(string(b), secret) {
			t.Errorf("%s holds the secret", p)
		}
		return nil
	})

	// Crash leftovers are swept at the next start.
	left := filepath.Join(app.SpaceHome, "ade", "runs", "x-app.mcp.json")
	if err := os.MkdirAll(filepath.Dir(left), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(left, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.Restart()
	if _, err := os.Stat(left); err == nil {
		t.Fatal("a leftover app file survived a restart")
	}
}

const openCodeSettingsYAML = `env:
  P453_MARK: ${secret:gh-token}
permissions:
  allow: [Read, "WebFetch(domain:example.com)"]
  deny: ["Bash(rm:*)"]
mcp_servers:
  gh:
    url: http://127.0.0.1:9/mcp
    headers:
      Authorization: Bearer ${secret:gh-token}
hooks:
  mark:
    event: session_start
    command: echo mark
opencode:
  model: anthropic/claude-sonnet-4-5
`

// contentDoc is the OPENCODE_CONFIG_CONTENT document of a recorded launch, without Kira's own
// `instructions`, as a map.
func contentDoc(t *testing.T, content string) map[string]json.RawMessage {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("OPENCODE_CONFIG_CONTENT = %q: %v", content, err)
	}
	delete(doc, "instructions")
	return doc
}

// TestAgentSettingsOpenCode: the settings reach OpenCode through OPENCODE_CONFIG_CONTENT only, on a TUI
// launch and a headless step, with the resolved secret in the environment and no file. Claude and
// the fake provider are covered by TestAgentSettingsReachEveryLaunch. Frontend half:
// ade-agent-settings.spec.ts.
func TestAgentSettingsOpenCode(t *testing.T) {
	secret := "p453-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Setenv(fakeagent.EnvRecordEnv, "P453_MARK")
	app := flowharness.New(t, flowharness.WithOpenCode(), flowharness.WithTrackerGrace(time.Second))
	app.Scenario(fakeagent.Scenario{OpenCode: map[string][]fakeagent.Action{"*": {{Name: "done"}}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	repoRec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: window})
	svc := app.W.AgentSettingsSvc
	saveSettings(t, app, openCodeSettingsYAML)
	if _, err := svc.SetSecret(agentcfg.SetSecretArgs{Name: "gh-token", Value: secret}); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	// The view shows the content document with the reference verbatim and the notes of what is not applied.
	v, err := svc.Get()
	if err != nil || v.Problem != nil {
		t.Fatalf("Get = %+v, %v", v, err)
	}
	var view agentcfg.ProviderView
	for _, p := range v.Providers {
		if p.ID == "opencode" {
			view = p
		}
	}
	if len(view.Files) != 1 || view.Files[0].Name != "opencode config" || !strings.Contains(view.Files[0].Text, "Bearer ${secret:gh-token}") {
		t.Fatalf("opencode view files = %+v, want one config with the reference verbatim", view.Files)
	}
	notes := strings.Join(view.Notes, "\n")
	for _, want := range []string{"WebFetch(domain:example.com)", "hook mark", "OPENCODE_CONFIG_CONTENT"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("opencode notes = %q, want %q", notes, want)
		}
	}
	if b, _ := json.Marshal(v); strings.Contains(string(b), secret) {
		t.Fatal("the view holds the secret value")
	}

	// TUI: the content document has the resolved header, model, deny and allow rules; no file holds it.
	tk := task(t, app, repoRec.ID, "ocset", ocWorkflow("ocset", ""))
	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: branchOf(t, app, tk.ID, repoRec.ID).ID, Name: "feat/api-ocset"})
	if err != nil {
		t.Fatal(err)
	}
	launch, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID, Provider: "opencode"})
	if err != nil {
		t.Fatalf("StartAdHoc opencode: %v", err)
	}
	if strings.Contains(launch.Command, secret) {
		t.Fatalf("command holds the secret: %q", launch.Command)
	}
	openTUI(t, app, launch)
	env := envLines(t, waitFake(t, app, "opencode-1.env"))
	content := env["OPENCODE_CONFIG_CONTENT"]
	if !strings.Contains(content, `"Authorization":"Bearer `+secret+`"`) {
		t.Fatalf("OPENCODE_CONFIG_CONTENT = %s, want the resolved header", content)
	}
	if mcp := waitFake(t, app, "opencode-1.appmcp"); !strings.Contains(mcp, `"gh"`) || !strings.Contains(mcp, secret) {
		t.Fatalf("recorded mcp block = %q, want server gh with the secret", mcp)
	}
	for _, want := range []string{`"model":"anthropic/claude-sonnet-4-5"`, `"rm*":"deny"`, `"read":"allow"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("OPENCODE_CONFIG_CONTENT = %s, want %s", content, want)
		}
	}
	if strings.Contains(content, "WebFetch") || strings.Contains(content, "webfetch") {
		t.Fatalf("OPENCODE_CONFIG_CONTENT = %s, want the untranslatable allow entry left out", content)
	}
	if got := waitFake(t, app, "opencode-1.appenv"); strings.TrimSpace(got) != "P453_MARK="+secret {
		t.Fatalf(".appenv = %q, want the resolved value", got)
	}
	got := contentDoc(t, content)
	want := contentDoc(t, strings.ReplaceAll(view.Files[0].Text, "${secret:gh-token}", secret))
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if string(gb) != string(wb) {
		t.Fatalf("launch document = %s, view with the secret = %s, want equal", gb, wb)
	}
	if left := appFiles(t, app.SpaceHome); len(left) != 0 {
		t.Fatalf("opencode launch wrote app files %v, want none", left)
	}
	_ = filepath.WalkDir(app.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(p, app.FakeDir) || strings.HasPrefix(p, app.Work) {
			return nil
		}
		if b, _ := os.ReadFile(p); strings.Contains(string(b), secret) {
			t.Errorf("%s holds the secret", p)
		}
		return nil
	})

	// Headless: the step gets the same servers and model, the deny rule, and the env.
	flow := task(t, app, repoRec.ID, "ochead", ocAgentFlow("ochead", ""))
	headBr := branchOf(t, app, flow.ID, repoRec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: flow.ID, BranchNames: map[string]string{headBr.ID: "feat/api-ochead"}}); err != nil {
		t.Fatal(err)
	}
	henv := envLines(t, waitFake(t, app, "opencode-2.env"))
	hcontent := henv["OPENCODE_CONFIG_CONTENT"]
	for _, want := range []string{`"Authorization":"Bearer ` + secret + `"`, `"model":"anthropic/claude-sonnet-4-5"`, `"rm*":"deny"`} {
		if !strings.Contains(hcontent, want) {
			t.Fatalf("headless OPENCODE_CONFIG_CONTENT = %s, want %s", hcontent, want)
		}
	}
	if got := waitFake(t, app, "opencode-2.appenv"); strings.TrimSpace(got) != "P453_MARK="+secret {
		t.Fatalf("headless .appenv = %q, want the resolved value", got)
	}
	testx.WaitUntil(t, waitFor, func() bool { return latestRun(t, app, flow.ID, "one").State == "done" })
}
