package repos_test

import (
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
)

func newLayoutRepo(t *testing.T) *repos.LayoutRepo {
	return &repos.LayoutRepo{DB: newRepos(t).DB}
}

func floatPtr(f float64) *float64 { return &f }

// TestLayoutRepoSetSurvivesConcurrentDisjointPatches is the rule: LayoutRepo.Set rewrites
// every leaf on each call (unlike SettingsRepo.Set), so its read-modify-write must be atomic —
// two concurrent Set calls patching entirely different leaves must both survive, not have one
// silently overwrite the other from a stale pre-write snapshot (measured at 109/200 rounds
// lost against the pre-fix code).
func TestLayoutRepoSetSurvivesConcurrentDisjointPatches(t *testing.T) {
	const rounds = 50
	lost := 0

	for round := 0; round < rounds; round++ {
		r := newLayoutRepo(t)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = r.Set(model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{
				"studio": {Project: &model.ProjectSizePatch{Width: floatPtr(333)}},
			}})
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Set(model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{
				"api": {Operations: &model.OperationsSizePatch{Height: floatPtr(321)}},
			}})
		}()
		wg.Wait()

		got, err := r.GetAll()
		if err != nil {
			t.Fatalf("round %d: GetAll: %v", round, err)
		}
		if got.Modes["studio"].Project.Width != 333 || got.Modes["api"].Operations.Height != 321 {
			lost++
		}
	}

	if lost != 0 {
		t.Fatalf("%d/%d rounds lost one of two disjoint concurrent patches, want 0", lost, rounds)
	}
}

// TestLayoutRepoWidthUserSetRoundTrips guards the flag staying a stored leaf: an absent patch field
// leaves it alone, and an explicit false clears it.
func TestLayoutRepoWidthUserSetRoundTrips(t *testing.T) {
	r := newLayoutRepo(t)
	set := func(v *bool, w *float64) {
		t.Helper()
		if _, err := r.Set(model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{
			"studio": {Project: &model.ProjectSizePatch{Width: w, WidthUserSet: v}},
		}}); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	flag := func() bool {
		t.Helper()
		got, err := r.GetAll()
		if err != nil {
			t.Fatalf("GetAll: %v", err)
		}
		return got.Modes["studio"].Project.WidthUserSet
	}
	yes, no := true, false
	set(&yes, floatPtr(300))
	if !flag() {
		t.Fatal("widthUserSet lost after Set(true)")
	}
	set(nil, floatPtr(310))
	if !flag() {
		t.Fatal("widthUserSet cleared by a patch that omits it")
	}
	set(&no, nil)
	if flag() {
		t.Fatal("widthUserSet still true after Set(false)")
	}
}
