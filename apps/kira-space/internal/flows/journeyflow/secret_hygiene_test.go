package journeyflow_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// J-SEC-001 / J-X-004: five secrets are used by their features, then no file under the app root may
// hold a marker byte-for-byte, except where the fake agent legitimately receives a declared value.
func TestSecretsNeverReachDisk(t *testing.T) {
	const (
		connPw   = "MARK-conn-pw-5d1f"
		apiTok   = "MARK-api-tok-8e2c"
		cfgRef   = "MARK-cfg-ref-3a7b"
		agentEnv = "MARK-agent-env-9c4d"
		scriptPw = "MARK-script-pw-6b0e"
	)
	markers := []string{connPw, apiTok, cfgRef, agentEnv, scriptPw}
	app := flowharness.New(t)
	claude(app, fakeagent.Action{Name: "done"})
	srv := flowharness.HTTP(t)

	// 1. Connection password: create, then connect to a closed port so the error path runs.
	in := connInput(connPw)
	conn, err := app.W.Connections.Create(in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, _ = app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: conn.ID})
	_ = app.W.Connections.Test(bridge.ConnectionsIDArgs{ID: conn.ID})

	// 2. API secret variable, sent on the wire.
	col, err := app.W.Collections.CreateCollection(bridge.CollectionsCreateCollectionArgs{Name: "api"})
	if err != nil {
		t.Fatal(err)
	}
	env := app.SeedEnvironment(t, "dev", flowharness.APIVar{Name: "token", Value: apiTok, Secret: true})
	if err := app.W.Variables.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: env.ID}); err != nil {
		t.Fatal(err)
	}
	args := bridge.HttpSendArgs{}
	args.OpID, args.TabID = ids.NewID(), "tab-1"
	args.Method, args.URL = "GET", srv.URL+"/status?code=200&t={{token}}"
	args.CollectionID, args.EnvironmentID = col.ID, env.ID
	args.Headers = []httpclient.Header{{Name: "Authorization", Value: "Bearer {{token}}"}}
	if _, err := app.W.Http.Send(ctx, args); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if reqs := srv.Requests(); len(reqs) != 1 || !strings.Contains(reqs[0].Query, apiTok) {
		t.Fatalf("the wire did not carry the token: %+v", reqs)
	}

	// 3 + 4. Agent settings: a ${secret:} reference and a stored value.
	svc := app.W.AgentSettingsSvc
	if _, err := svc.SetSecret(agentcfg.SetSecretArgs{Name: "cfg-ref", Value: cfgRef}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSecret(agentcfg.SetSecretArgs{Name: "agent-env", Value: agentEnv}); err != nil {
		t.Fatal(err)
	}
	cur, err := svc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(agentcfg.SaveArgs{BaseHash: cur.Hash, Text: "env:\n  HYGIENE_ENV: ${secret:agent-env}\n  HYGIENE_REF: ${secret:cfg-ref}\n"}); err != nil {
		t.Fatalf("Save settings: %v", err)
	}

	// 5. Smart script with a secret param, run on the fake agent.
	pw := scriptPw
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{
		Fields: scripts.CustomScriptFields{
			Name: "hygiene", Kind: scripts.KindSmart, Command: "check the thing", Color: "blue",
			Params: []scripts.Param{{Name: "token", Type: scripts.ParamText, Secret: true}},
		},
		SecretValues: map[string]*string{"token": &pw},
	})
	if err != nil {
		t.Fatal(err)
	}
	ra := scriptruns.RunArgs{ScriptID: rec.ID}
	pv, err := app.W.ScriptRuns.Preview(ra)
	if err != nil {
		t.Fatal(err)
	}
	st, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: ra, Hash: pv.Hash})
	if err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		r, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: st.RunID})
		return err == nil && r.State != "running" && r.State != "starting"
	})
	run, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: st.RunID})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range run.Params {
		for _, m := range markers {
			if strings.Contains(p.Value, m) {
				t.Errorf("run param %s holds %s", p.Name, m)
			}
		}
	}

	// The values are live: the store hands them back, so the scan below is not vacuous.
	if r := app.W.Connections.Reveal(bridge.ConnectionsRevealArgs{ID: conn.ID, Confirmed: true}); r.Password == nil || *r.Password != connPw {
		t.Fatalf("Reveal = %+v, want the saved password", r)
	}
	hits := map[string][]string{}
	scanned := 0
	_ = filepath.WalkDir(app.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		scanned++
		for _, m := range markers {
			if strings.Contains(string(b), m) {
				hits[p] = append(hits[p], m)
			}
		}
		return nil
	})
	if scanned < 5 {
		t.Fatalf("scanned %d files under %s", scanned, app.Root)
	}
	for p, ms := range hits {
		if strings.HasPrefix(p, app.FakeDir) {
			continue // the fake agent records what it was given
		}
		t.Errorf("%s holds %v", p, ms)
	}
}

func connInput(password string) connections.Input {
	h, d, u, port := "127.0.0.1", "app", "kira", 1
	return connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "pg", Kind: "postgres", Color: "none", Mode: "fields", Host: &h, Port: &port,
		Database: &d, Username: &u, Options: map[string]any{},
		McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}, Password: &password}
}
