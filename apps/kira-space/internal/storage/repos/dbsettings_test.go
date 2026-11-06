package repos_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

func newSettingsRepo(t *testing.T) *repos.DbSettingsRepo {
	return &repos.DbSettingsRepo{DB: newRepos(t).DB}
}

// TestSettingsRepo_LogLevelDefault is the code review's own regression guard:
// advanced.logLevel (Studio's own leaf — see migration 0028) was added to the zod schema and the
// Settings dialog but never wired into GetAll/Set/DefaultSettings — a patch silently dropped the
// write. Unset, it must read back settings.ts's own default.
func TestSettingsRepo_LogLevelDefault(t *testing.T) {
	r := newSettingsRepo(t)
	got, err := r.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if got.Advanced.LogLevel != "info" {
		t.Fatalf("Advanced.LogLevel = %q, want %q", got.Advanced.LogLevel, "info")
	}
}

func TestSettingsRepo_LogLevelRoundTrip(t *testing.T) {
	r := newSettingsRepo(t)
	logLevel := "debug"
	if _, err := r.Set(model.DbSettingsPatch{
		Advanced: &model.DbAdvancedPatch{LogLevel: &logLevel},
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := r.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if got.Advanced.LogLevel != "debug" {
		t.Fatalf("Advanced.LogLevel = %q, want %q", got.Advanced.LogLevel, "debug")
	}
}

func TestSettingsRepo_LogLevelRejectInvalid(t *testing.T) {
	r := newSettingsRepo(t)
	bad := "sideways"
	if _, err := r.Set(model.DbSettingsPatch{Advanced: &model.DbAdvancedPatch{LogLevel: &bad}}); err == nil {
		t.Fatal("Set(logLevel: \"sideways\") = nil error, want a validation error")
	}
}
