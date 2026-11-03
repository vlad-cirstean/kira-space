// Package scriptstest edits run files for flow tests: the app has no Update call, a test edits the
// file like a user does.
package scriptstest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// Bound is the part of an app's CustomScriptsService the helpers use.
type Bound interface {
	List() (scripts.Snapshot, error)
	File(scripts.FileArgs) (scripts.FileView, error)
	SaveFile(scripts.SaveFileArgs) (scripts.FileView, error)
	SetSecrets(scripts.SetSecretsArgs) (scripts.CustomScript, error)
}

// Update replaces the config of run id with fields: it renders the file and saves it. A text that
// does not validate fails the test.
func Update(t testing.TB, b Bound, id string, fields scripts.CustomScriptFields) scripts.CustomScript {
	t.Helper()
	file := scripts.FileName(id)
	cur, err := b.File(scripts.FileArgs{File: file})
	if err != nil {
		t.Fatalf("read run file %s: %v", file, err)
	}
	text, err := scripts.FileOf(fields).Render()
	if err != nil {
		t.Fatalf("render run file %s: %v", file, err)
	}
	v, err := b.SaveFile(scripts.SaveFileArgs{File: file, Text: string(text), BaseHash: cur.Hash})
	if err != nil {
		t.Fatalf("save run file %s: %v", file, err)
	}
	if v.Problem != nil {
		t.Fatalf("run file %s is invalid: line %d: %s", file, v.Problem.Line, v.Problem.Message)
	}
	snap, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Scripts {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("run %s is not listed after the save", id)
	return scripts.CustomScript{}
}

// SetSecrets saves (string) or clears (nil) secret param values of run id.
func SetSecrets(t testing.TB, b Bound, id string, values map[string]*string) scripts.CustomScript {
	t.Helper()
	rec, err := b.SetSecrets(scripts.SetSecretsArgs{ID: id, Values: values})
	if err != nil {
		t.Fatalf("set secrets of %s: %v", id, err)
	}
	return rec
}

// WriteFile writes raw text as run id's file in dir, the way an external editor does.
func WriteFile(t testing.TB, dir, id, text string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := renameio.WriteFile(filepath.Join(dir, scripts.FileName(id)), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
