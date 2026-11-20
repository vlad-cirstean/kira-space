package repoflow_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// TestRepoServicesFile: the form writes the `services` key, a repeated script is a problem at the
// later entry, and a shared file may carry the key.
//
// Contract repo-services: tests/ui/repo-services.spec.ts reads these.
func TestRepoServicesFile(t *testing.T) {
	app := flowharness.New(t)
	r := app.NewRepo("proj")
	r.Commit("init", map[string]string{"a.txt": "a\n"})
	rec := importVia(t, app, r.Dir)
	key := rec.RepoID
	svc := app.W.RepoConfigSvc
	mask := flowharness.Mask("hash", "baseHash", "filePath")

	saved, err := svc.SaveFile(repocfg.SaveArgs{Path: key, Text: "path: " + key + "\n"})
	if err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v", saved, err)
	}
	view, err := svc.File(repocfg.FileArgs{Path: key})
	if err != nil {
		t.Fatal(err)
	}
	call := func(name string, services []any) configfs.FormResult {
		t.Helper()
		args := repocfg.FormArgs{File: view.File, Text: view.Text, Edits: []configfs.Edit{{Path: []string{"services"}, Value: services}}}
		app.Contract(t, "repo-services", "args:RepoConfigService.Form#"+name, args, mask)
		res, err := svc.Form(args)
		if err != nil {
			t.Fatalf("Form %s: %v", name, err)
		}
		app.Contract(t, "repo-services", "RepoConfigService.Form#"+name, res, mask)
		return res
	}

	added := call("added", []any{
		map[string]any{"script": "Dev DB", "params": map[string]any{"port": "5432"}},
		map[string]any{"script": "Dev server"},
	})
	f, ok := added.Value.(repocfg.File)
	if added.Problem != nil || !ok || len(f.Services) != 2 || f.Services[0].Script != "Dev DB" || !strings.Contains(added.Text, "services:") {
		t.Fatalf("Form added = %+v, want two services in the text and model", added)
	}

	dup := call("duplicate", []any{map[string]any{"script": "Dev DB"}, map[string]any{"script": "Dev DB"}})
	if dup.Problem == nil || dup.Problem.Path != "/services/1/script" {
		t.Fatalf("Form duplicate problem = %+v, want one at /services/1/script", dup.Problem)
	}

	if msg := app.W.RepoConfig.CheckShared([]byte("origin: https://example.com/acme/app.git\nservices:\n  - script: Dev DB\n")); msg != "" {
		t.Fatalf("shared file with services: %s", msg)
	}
}
