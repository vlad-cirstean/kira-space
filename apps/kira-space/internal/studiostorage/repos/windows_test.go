package repos_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
)

func newWindowsRepo(t *testing.T) *repos.WindowsRepo {
	return &repos.WindowsRepo{DB: newRepos(t).DB}
}

// P22 D12/F20/F21: a window remembers which app mode it was closed in — SetMode/GetMode round
// trip, and List (the boot-time read every existing window-bounds path already used) carries the
// same value.
func TestWindowsRepoModeRoundTrips(t *testing.T) {
	r := newWindowsRepo(t)
	if err := r.Create(model.WindowRecord{Key: "w1", Order: 0}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A freshly created window (no explicit mode) reads back the migration's own DEFAULT.
	got, err := r.GetMode("w1")
	if err != nil {
		t.Fatalf("GetMode (fresh): %v", err)
	}
	if got != model.DefaultWindowMode {
		t.Errorf("GetMode (fresh) = %q, want %q", got, model.DefaultWindowMode)
	}

	if err := r.SetMode("w1", "api"); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	got, err = r.GetMode("w1")
	if err != nil {
		t.Fatalf("GetMode (after SetMode): %v", err)
	}
	if got != "api" {
		t.Errorf("GetMode (after SetMode) = %q, want %q", got, "api")
	}

	records, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, rec := range records {
		if rec.Key == "w1" {
			found = true
			if rec.Mode != "api" {
				t.Errorf("List: w1.Mode = %q, want %q", rec.Mode, "api")
			}
		}
	}
	if !found {
		t.Fatal("List did not return the w1 record")
	}
}

// An unrecognised mode value (a hand-edited row, or a future removed mode) normalises to the
// default rather than being refused outright — model.NormalizeMode's own drop-and-default posture,
// exercised here through the real repo round trip rather than the pure function alone.
func TestWindowsRepoUnknownModeNormalizesToDefault(t *testing.T) {
	r := newWindowsRepo(t)
	if err := r.Create(model.WindowRecord{Key: "w1", Order: 0}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := r.DB.Exec(`UPDATE windows SET mode = 'bogus' WHERE key = 'w1'`); err != nil {
		t.Fatalf("seed bogus mode: %v", err)
	}

	got, err := r.GetMode("w1")
	if err != nil {
		t.Fatalf("GetMode: %v", err)
	}
	if got != model.DefaultWindowMode {
		t.Errorf("GetMode (bogus stored value) = %q, want %q", got, model.DefaultWindowMode)
	}

	records, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, rec := range records {
		if rec.Key != "w1" {
			continue
		}
		found = true
		if rec.Mode != model.DefaultWindowMode {
			t.Errorf("List: w1.Mode = %q, want %q", rec.Mode, model.DefaultWindowMode)
		}
	}
	if !found {
		t.Fatal("List did not return the w1 record")
	}
}

// SetMode on a key with no `windows` row behaves like SetBounds already does for the identical
// case: a real error naming the missing key, not a silent no-op.
func TestWindowsRepoSetModeMissingKey(t *testing.T) {
	r := newWindowsRepo(t)
	if err := r.SetMode("no-such-window", "api"); err == nil {
		t.Fatal("SetMode on a missing key: want an error, got nil")
	}
}

// GetMode on a key with no `windows` row is the same "no such window" shape.
func TestWindowsRepoGetModeMissingKey(t *testing.T) {
	r := newWindowsRepo(t)
	if _, err := r.GetMode("no-such-window"); err == nil {
		t.Fatal("GetMode on a missing key: want an error, got nil")
	}
}
