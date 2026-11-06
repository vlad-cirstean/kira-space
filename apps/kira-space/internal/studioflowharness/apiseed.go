package flowharness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles/apifilestest"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// APIVar is one variable of a seeded collection or environment. A Secret variable keeps its Value in
// the secret store, never in the file.
type APIVar = apifilestest.Var

func (a *App) seeder() apifilestest.Seeder { return apifilestest.Seeder{Store: a.W.ApiFiles} }

// WriteAPIFile writes a file into the api config folder and loads it, as an external edit would.
func (a *App) WriteAPIFile(t testing.TB, file string, text []byte) {
	t.Helper()
	a.seeder().WriteFile(t, file, text)
}

// RemoveAPIFile deletes a file from the api config folder and reloads.
func (a *App) RemoveAPIFile(t testing.TB, file string) {
	t.Helper()
	if err := os.Remove(filepath.Join(a.W.ApiFiles.Dir(), file)); err != nil {
		t.Fatal(err)
	}
	a.W.ApiFiles.Sync()
}

// SeedCollection writes a collection file named after name, with vars, and returns its row.
func (a *App) SeedCollection(t testing.TB, name string, vars ...APIVar) model.Collection {
	t.Helper()
	file := a.seeder().Collection(t, name, vars...)
	tree, err := a.W.Collections.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range tree.Collections {
		if c.File == file {
			return c
		}
	}
	t.Fatalf("collection file %s did not load", file)
	return model.Collection{}
}

// SeedEnvironment writes an environment file named after name, with vars, and returns its row.
func (a *App) SeedEnvironment(t testing.TB, name string, vars ...APIVar) model.Environment {
	t.Helper()
	file := a.seeder().Environment(t, name, vars...)
	envs, err := a.W.Variables.ListEnvironments()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range envs {
		if e.File == file {
			return e
		}
	}
	t.Fatalf("environment file %s did not load", file)
	return model.Environment{}
}
