// Package apifilestest seeds the api config files for tests: the api_* tables are a projection of
// the files, so a test that needs a collection or environment writes the file and loads it.
package apifilestest

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apifiles"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// Var is one variable of a seeded collection or environment. A Secret variable keeps its Value in
// the secret store under a name derived from the owner and the variable, never in the file.
type Var struct {
	Name, Value string
	Secret      bool
}

// Fixture is a store over a temp folder plus its secret namespace.
type Fixture struct {
	Store   *apifiles.Store
	Secrets *apifiles.Secrets
}

// New builds a store over a fresh temp folder. cipher seals the secrets; it is never started, so a
// test loads files with Sync.
func New(t testing.TB, db *sql.DB, cipher *secrets.Cipher) *Fixture {
	t.Helper()
	sec := &apifiles.Secrets{Store: secrets.NewStore(db, cipher), DB: db}
	st, err := apifiles.New(apifiles.Options{DB: db, Dir: filepath.Join(t.TempDir(), "api"), Secrets: sec})
	if err != nil {
		t.Fatal(err)
	}
	return &Fixture{Store: st, Secrets: sec}
}

// Seeder writes files into a store.
type Seeder struct{ Store *apifiles.Store }

// WriteFile writes a file into the api folder and loads it, as an external edit would.
func (s Seeder) WriteFile(t testing.TB, file string, text []byte) {
	t.Helper()
	if err := configfs.WriteAtomic(filepath.Join(s.Store.Dir(), file), text, 0o644); err != nil {
		t.Fatal(err)
	}
	s.Store.Sync()
}

func (s Seeder) vars(t testing.TB, stem string, vars []Var) []apifiles.Variable {
	t.Helper()
	out := make([]apifiles.Variable, 0, len(vars))
	for _, v := range vars {
		if !v.Secret {
			out = append(out, apifiles.Variable{Name: v.Name, Value: v.Value})
			continue
		}
		ref := apifiles.SecretName(stem, v.Name)
		if err := s.Store.PutSecret(ref, v.Value); err != nil {
			t.Fatal(err)
		}
		out = append(out, apifiles.Variable{Name: v.Name, Secret: "${secret:" + ref + "}"})
	}
	return out
}

// Collection writes an empty collection file named after name, with vars, and returns the file name.
func (s Seeder) Collection(t testing.TB, name string, vars ...Var) string {
	t.Helper()
	stem := apifiles.Slug(name, "collection")
	text, err := configfs.RenderYAML(apifiles.CollectionFile{Name: name, Variables: s.vars(t, stem, vars), Items: []apifiles.Item{}}, "")
	if err != nil {
		t.Fatal(err)
	}
	file := stem + apifiles.CollectionExt
	s.WriteFile(t, file, text)
	return file
}

// Environment writes an environment file named after name, with vars, and returns the file name.
func (s Seeder) Environment(t testing.TB, name string, vars ...Var) string {
	t.Helper()
	stem := apifiles.Slug(name, "environment")
	text, err := configfs.RenderYAML(apifiles.EnvironmentFile{Name: name, Variables: s.vars(t, stem, vars)}, "")
	if err != nil {
		t.Fatal(err)
	}
	file := stem + apifiles.EnvironmentExt
	s.WriteFile(t, file, text)
	return file
}
