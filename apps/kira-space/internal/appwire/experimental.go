package appwire

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// seedExperimentalMemory keeps Memory visible for installs that already use it. memory.db is
// created only on first use, so its presence marks a user the new default-off flag would surprise.
// Runs once: after the row exists, an explicit off survives.
func seedExperimentalMemory(settings *repos.SettingsRepo, memoryDB string) error {
	stored, err := settings.LeafStored("experimental.memory")
	if err != nil || stored {
		return err
	}
	if _, err := os.Stat(memoryDB); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat memory db: %w", err)
	}
	on := true
	_, err = settings.Set(model.SettingsPatch{Experimental: &model.ExperimentalPatch{Memory: &on}})
	return err
}
