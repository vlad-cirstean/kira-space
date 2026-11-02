// CollectionsRepo.ImportTree commits the whole collection tree in one transaction, then
// VariablesRepo.ImportVariables runs as a second, separate one. If the second fails — the
// realistic case is a `type: "secret"` variable on a machine where secret storage is
// unavailable — Import used to return an error while leaving the collection and its whole request
// tree already committed and visible in the panel: the reported failure didn't match what was on
// disk. Import now deletes the just-created collection (ON DELETE CASCADE on api_items) before
// returning that error.
package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

func writeTempCollection(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collection.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTempCollection: %v", err)
	}
	return path
}

// failingCipher fails every Encrypt call, simulating secret storage being unavailable
// (repos.Cipher's real implementations return E_SECRET_STORE for the equivalent failure).
type failingCipher struct{}

func (failingCipher) Encrypt(secrets.Scope, string) (string, error) { return "", errFailingCipher }
func (failingCipher) Decrypt(secrets.Scope, string) (string, error) { return "", errFailingCipher }

var errFailingCipher = &cipherUnavailableError{}

type cipherUnavailableError struct{}

func (*cipherUnavailableError) Error() string { return "secret storage is unavailable" }

const importCollectionWithSecretVariable = `{
  "info": {"name": "atomicity-test", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
  "item": [],
  "variable": [
    {"key": "apiKey", "value": "sk_live_x", "type": "secret"}
  ]
}`

func TestImport_RollsBackTheCollectionWhenImportVariablesFails(t *testing.T) {
	t.Setenv("KIRA_HOME", t.TempDir())
	db, err := storage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	collectionsRepo := &repos.CollectionsRepo{DB: db.DB}
	variablesRepo := repos.NewVariables(db.DB, failingCipher{})

	svc := &CollectionsService{Deps: appcore.Deps{
		Repos:  &repos.Repos{Collections: collectionsRepo, Variables: variablesRepo},
		Events: &fakeEmitter{},
	}}

	path := writeTempCollection(t, importCollectionWithSecretVariable)
	res, err := svc.Import(CollectionsImportArgs{Paths: []string{path}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(res.Files) != 1 || res.Files[0].Error == nil || !strings.Contains(*res.Files[0].Error, errFailingCipher.Error()) {
		t.Fatalf("Import files = %+v, want one entry naming the cipher failure", res.Files)
	}

	collections, items, err := collectionsRepo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, c := range collections {
		if c.Name == "atomicity-test" {
			t.Fatalf("collection %q was left committed after ImportVariables failed — Import is not atomic", c.Name)
		}
	}
	if len(items) != 0 {
		t.Fatalf("items = %v, want none left behind after a rolled-back import", items)
	}
}
