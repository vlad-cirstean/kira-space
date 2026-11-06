package dbflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Contract connection-form, read by tests/ui/connection-form.spec.ts: the form edits a connection
// file as node edits (comments and key order stay), a YAML-side change re-decodes, a bad draft is a
// problem with a line, and a typed password goes through the write-only secret call.
func TestConnectionForm(t *testing.T) {
	app := flowharness.New(t)
	const sc = "connection-form"
	const typed = "typed-pw-9c1f"
	cfg := app.W.ConnectionConfig
	mask := flowharness.Mask("hash", "baseHash", "createdAt", "updatedAt")
	paths := flowharness.Replace(app.KiraHome, "<home>")

	in := sqliteInput("Pg", "")
	in.Kind, in.Database = "postgres", ptr("app")
	in.Host, in.Port, in.Username = ptr("127.0.0.1"), ptr(5432), ptr("kira")
	in.Password = ptr(fileSecret)
	conn := createConn(t, app, in)

	// A head comment, a line comment and color moved to the end of the file.
	seed := "# Staging database for the web team.\n" +
		"name: Pg # shown in the sidebar\n" +
		"kind: postgres\n" +
		"host: 127.0.0.1\n" +
		"port: 5432\n" +
		"database: app\n" +
		"username: kira\n" +
		"password: ${secret:pg.password}\n" +
		"mcp:\n  read: deny\n  write: deny\n  auto_explain: false\n" +
		"color: blue\n"
	d, err := cfg.File(bridge.ConnectionFileArgs{File: conn.File})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.SaveFile(bridge.ConnectionSaveFileArgs{File: conn.File, Text: seed, BaseHash: d.Hash}); err != nil {
		t.Fatal(err)
	}
	view, err := cfg.File(bridge.ConnectionFileArgs{File: conn.File})
	if err != nil || view.Text != seed || view.Problem != nil {
		t.Fatalf("File = %+v, %v, want the seed", view, err)
	}
	app.Contract(t, sc, "file#read", view, mask, paths)

	var recorded []any
	call := func(name string, edits []configfs.Edit, text string) configfs.FormResult {
		t.Helper()
		args := bridge.ConnectionFormArgs{File: conn.File, Text: text, Edits: edits}
		app.Contract(t, sc, "args:ConnectionConfigService.Form#"+name, args, mask, paths)
		res, err := cfg.Form(args)
		if err != nil {
			t.Fatalf("Form %s: %v", name, err)
		}
		app.Contract(t, sc, "form#"+name, res, mask, paths)
		recorded = append(recorded, args, res)
		return res
	}

	read := call("read", []configfs.Edit{}, view.Text)
	f, ok := read.Value.(connfiles.File)
	if read.Problem != nil || read.Text != seed || !ok || f.Name != "Pg" || f.Port != 5432 {
		t.Fatalf("Form without edits = %+v, want the text unchanged and the file decoded", read)
	}

	renamed := call("rename", []configfs.Edit{{Path: []string{"name"}, Value: "Pg staging"}}, view.Text)
	if renamed.Problem != nil || !strings.Contains(renamed.Text, "name: Pg staging # shown in the sidebar") {
		t.Fatalf("Form of one scalar = %+v", renamed)
	}

	// A scalar, a nested key, a list and an unset in one call.
	edited := call("edited", []configfs.Edit{
		{Path: []string{"port"}, Value: 5433},
		{Path: []string{"mcp", "description"}, Value: "Replica for reports"},
		{Path: []string{"masks"}, Value: []any{map[string]any{"table": "users", "column": "email", "kind": "email"}}},
		{Path: []string{"color"}, Unset: true},
	}, view.Text)
	if edited.Problem != nil {
		t.Fatalf("Form = %+v", edited)
	}
	for _, want := range []string{"# Staging database for the web team.", "# shown in the sidebar", "port: 5433", "description: Replica for reports", "column: email"} {
		if !strings.Contains(edited.Text, want) {
			t.Errorf("edited text lacks %q:\n%s", want, edited.Text)
		}
	}
	if strings.Contains(edited.Text, "color:") {
		t.Errorf("edited text keeps the unset color:\n%s", edited.Text)
	}
	if a, b, c := strings.Index(edited.Text, "name:"), strings.Index(edited.Text, "kind:"), strings.Index(edited.Text, "host:"); !(a < b && b < c) {
		t.Errorf("key order changed:\n%s", edited.Text)
	}
	if got, ok := edited.Value.(connfiles.File); !ok || got.Port != 5433 || got.MCP == nil || got.MCP.Description != "Replica for reports" || len(got.Masks) != 1 {
		t.Fatalf("decoded value = %#v", edited.Value)
	}

	// A typed password: the file gets the reference, the value goes through SetSecret.
	pw := call("password", []configfs.Edit{{Path: []string{"password"}, Value: connfiles.RefOf("pg-staging.password")}}, view.Text)
	if pw.Problem != nil || !strings.Contains(pw.Text, "password: ${secret:pg-staging.password}") || strings.Contains(pw.Text, typed) {
		t.Fatalf("Form of the password reference = %+v", pw)
	}
	if _, err := cfg.SetSecret(bridge.ConnectionSecretArgs{Kind: connfiles.KindPassword, Name: "pg-staging.password", Value: typed}); err != nil {
		t.Fatal(err)
	}

	yamlSide := strings.Replace(view.Text, "name: Pg", "name: Pg yaml", 1)
	changed := call("yaml", []configfs.Edit{}, yamlSide)
	if got, ok := changed.Value.(connfiles.File); !ok || got.Name != "Pg yaml" || changed.Text != yamlSide {
		t.Fatalf("Form of a YAML-side change = %+v", changed)
	}

	bad := call("bad", []configfs.Edit{}, strings.Replace(view.Text, "kind: postgres", "kind: nosuchkind", 1))
	if bad.Problem == nil || bad.Problem.Line == 0 || bad.Value != nil {
		t.Fatalf("Form of bad text = %+v, want a problem with a line and no value", bad)
	}

	saveArgs := bridge.ConnectionSaveFileArgs{File: conn.File, Text: pw.Text, BaseHash: view.Hash}
	app.Contract(t, sc, "args:ConnectionConfigService.SaveFile", saveArgs, mask, paths)
	if _, err := cfg.SaveFile(bridge.ConnectionSaveFileArgs{File: conn.File, Text: pw.Text, BaseHash: "stale"}); testx.AsIpcErr(t, err).Code != "E_CONFLICT" {
		t.Errorf("SaveFile with a stale hash = %v, want E_CONFLICT", err)
	}
	out, err := cfg.SaveFile(saveArgs)
	if err != nil || out.Problem != nil || out.Text != pw.Text {
		t.Fatalf("SaveFile = %+v, %v", out, err)
	}
	app.Contract(t, sc, "file#saved", out, mask, paths)
	back, err := cfg.File(bridge.ConnectionFileArgs{File: conn.File})
	if err != nil || back.Text != pw.Text {
		t.Fatalf("File after SaveFile = %+v, %v", back, err)
	}
	if r := app.W.Connections.Reveal(bridge.ConnectionsRevealArgs{ID: conn.ID, Confirmed: true}); r.Password == nil || *r.Password != typed {
		t.Fatalf("Reveal = %+v, want the typed password", r)
	}

	all, err := json.Marshal(recorded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(all), typed) || strings.Contains(back.Text, typed) {
		t.Error("a form call or the file holds the typed password")
	}
	if b, _ := json.Marshal([]any{view, out}); strings.Contains(string(b), typed) {
		t.Error("a file view holds the typed password")
	}
}
