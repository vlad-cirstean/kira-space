package repos

import (
	"database/sql"
	"encoding/json"

	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

const proofShotLocationKey = "proof.shotLocation"

// ProofShotLocation returns the custom folder proof screenshots are saved under; "" means the Desktop default.
func (r *SettingsRepo) ProofShotLocation() (string, error) {
	stored, err := sqlb.QueryLeaves(r.DB, r.Dialect, r.selectAll, settingsLeaves)
	if err != nil {
		return "", err
	}
	var path string
	appsettings.Leaf(stored, proofShotLocationKey, &path)
	return path, nil
}

// SetProofShotLocation stores the custom folder; "" resets to the default. Kept out of SettingsPatch so
// the renderer never sends a path.
func (r *SettingsRepo) SetProofShotLocation(path string) error {
	return sqlb.UpdateLeaves(r.DB, r.Dialect, r.selectAll, settingsLeaves, func(tx *sql.Tx, _ map[string]json.RawMessage) error {
		return appsettings.UpsertLeaf(tx, proofShotLocationKey, path)
	})
}
