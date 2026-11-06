package windowflow_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	spacemodel "github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	studiomodel "github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

var (
	tsKindList = regexp.MustCompile(`(?s)\b\w*RENDERABLE_TAB_KINDS\b[^=]*=\s*\[(.*?)\]`)
	tsKindName = regexp.MustCompile(`'([a-z-]+)'`)
)

// frontendTabKinds is every kind a tabDomain.ts under the frontend registers as renderable.
func frontendTabKinds(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "frontend", "src")
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "tabDomain.ts" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, list := range tsKindList.FindAllSubmatch(src, -1) {
			for _, k := range tsKindName.FindAllSubmatch(list[1], -1) {
				seen[string(k[1])] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// The kinds the frontend registers, the kinds the two Go models accept, and the database
// TabsService routes each to agree; a kind on one side only is dropped on restore.
func TestTabKindsAgreeWithFrontend(t *testing.T) {
	ts := frontendTabKinds(t)
	var goKinds []string
	for k := range studiomodel.RenderableTabKinds {
		goKinds = append(goKinds, k)
	}
	for k := range spacemodel.RenderableTabKinds {
		goKinds = append(goKinds, k)
	}
	slices.Sort(goKinds)
	goKinds = slices.Compact(goKinds)
	if len(ts) < 10 || !slices.Equal(ts, goKinds) {
		t.Fatalf("frontend kinds %v, Go kinds %v: the lists must match", ts, goKinds)
	}

	app := flowharness.New(t)
	ensure(t, app, "w")
	count := func(conn *sql.DB, id string) int {
		var n int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM tabs WHERE id = ?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, kind := range ts {
		ws := "ws"
		rec := spacemodel.TabRecord{ID: "id-" + kind, Path: "p/" + kind, Kind: kind, State: json.RawMessage(`{}`), Active: true, WorkspaceID: &ws}
		if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "w", Tabs: []spacemodel.TabRecord{rec}}); err != nil {
			t.Fatalf("Save %s: %v", kind, err)
		}
		inSpace, inStudio := count(app.DB(), rec.ID), count(app.StudioDB(), rec.ID)
		if want := studiomodel.IsStudioTabKind(kind); (inStudio == 1) != want || (inSpace == 1) == want {
			t.Errorf("%s: space rows %d, studio rows %d, studio kind %v", kind, inSpace, inStudio, want)
		}
	}
}
