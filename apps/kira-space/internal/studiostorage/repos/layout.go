package repos

import (
	"database/sql"
	"encoding/json"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/appstorage"
)

// LayoutRepo reads and writes the `ui_layout` table — same per-leaf-row shape as SettingsRepo,
// for the reason layout.ts's own comment gives (a per-key row survives an old build's missing
// keys without a schema migration).
type LayoutRepo struct {
	DB *sql.DB

	// selectAll is prepared once by repos.New (P52 §5.4); nil when constructed directly, which
	// falls back to an ad-hoc query with identical SQL.
	selectAll *sql.Stmt
}

func modeWidthKey(mode string) string { return "modes." + mode + ".project.width" }
func modeWidthUserSetKey(mode string) string {
	return "modes." + mode + ".project.widthUserSet"
}
func modeHeightKey(mode string) string { return "modes." + mode + ".operations.height" }

// decodeLayout builds model.Layout from an already-scanned leaf map — the part of GetAll's read
// path that Set's own apply callback needs too, reading the same leaves inside its own
// transaction (P107 I2-1).
func decodeLayout(stored map[string]json.RawMessage) model.Layout {
	result := model.DefaultLayout()
	appsettings.Leaf(stored, "panel.project.visible", &result.Panel.Project.Visible)
	appsettings.Leaf(stored, "panel.operations.visible", &result.Panel.Operations.Visible)
	appsettings.Leaf(stored, "panel.cellEditor.height", &result.Panel.CellEditor.Height)
	for mode, sizes := range result.Modes {
		appsettings.Leaf(stored, modeWidthKey(mode), &sizes.Project.Width)
		appsettings.Leaf(stored, modeWidthUserSetKey(mode), &sizes.Project.WidthUserSet)
		appsettings.Leaf(stored, modeHeightKey(mode), &sizes.Operations.Height)
		result.Modes[mode] = sizes
	}
	return result
}

// applyLayoutPatch merges patch's set leaves onto merged — split out of Set's own closure to keep
// its cognitive complexity within this repo's linted bound (gocognit).
func applyLayoutPatch(merged *model.Layout, patch model.LayoutPatch) {
	applyModesPatch(merged, patch.Modes)
	p := patch.Panel
	if p == nil {
		return
	}
	if p.Project != nil {
		if p.Project.Visible != nil {
			merged.Panel.Project.Visible = *p.Project.Visible
		}
	}
	if p.Operations != nil {
		if p.Operations.Visible != nil {
			merged.Panel.Operations.Visible = *p.Operations.Visible
		}
	}
	if p.CellEditor != nil && p.CellEditor.Height != nil {
		merged.Panel.CellEditor.Height = *p.CellEditor.Height
	}
}

// applyModesPatch skips a mode outside the app's vocabulary; the bridge rejects those up front.
func applyModesPatch(merged *model.Layout, modes map[string]model.PanelSizesPatch) {
	for mode, patch := range modes {
		sizes, ok := merged.Modes[mode]
		if !ok {
			continue
		}
		if patch.Project != nil && patch.Project.Width != nil {
			sizes.Project.Width = *patch.Project.Width
		}
		if patch.Project != nil && patch.Project.WidthUserSet != nil {
			sizes.Project.WidthUserSet = *patch.Project.WidthUserSet
		}
		if patch.Operations != nil && patch.Operations.Height != nil {
			sizes.Operations.Height = *patch.Operations.Height
		}
		merged.Modes[mode] = sizes
	}
}

func (r *LayoutRepo) GetAll() (model.Layout, error) {
	stored, err := appstorage.QueryLeaves(r.DB, r.selectAll, appstorage.LayoutSelectAllSQL)
	if err != nil {
		return model.Layout{}, err
	}
	return decodeLayout(stored), nil
}

// Set writes every leaf every time (unlike SettingsRepo.Set's patched-leaves-only write),
// mirroring layout.ts's flatten(merged).
//
// The read that feeds the merge runs inside this same transaction, not before
// Begin() the way it used to: two concurrent Set calls patching disjoint leaves would otherwise
// each compute their merge from a pre-write snapshot, and the loser's upsert of all six leaves
// would silently overwrite the winner's — measured at 109 lost patches out of 200 rounds against
// the pre-fix code. storage/db.go's SetMaxOpenConns(1) means Begin() here holds
// the database's one connection exclusively until Commit/Rollback, so no other Set's read or
// write can land between this one's own read and write.
func (r *LayoutRepo) Set(patch model.LayoutPatch) (model.Layout, error) {
	var merged model.Layout
	err := appstorage.UpdateLeaves(r.DB, r.selectAll, appstorage.LayoutSelectAllSQL, func(tx *sql.Tx, stored map[string]json.RawMessage) error {
		merged = decodeLayout(stored)
		applyLayoutPatch(&merged, patch)

		leaves := make([]appstorage.Leaf, 0, 3+3*len(merged.Modes))
		leaves = append(leaves,
			appstorage.Leaf{Key: "panel.project.visible", Value: merged.Panel.Project.Visible},
			appstorage.Leaf{Key: "panel.operations.visible", Value: merged.Panel.Operations.Visible},
			appstorage.Leaf{Key: "panel.cellEditor.height", Value: merged.Panel.CellEditor.Height},
		)
		for mode, sizes := range merged.Modes {
			leaves = append(leaves,
				appstorage.Leaf{Key: modeWidthKey(mode), Value: sizes.Project.Width},
				appstorage.Leaf{Key: modeWidthUserSetKey(mode), Value: sizes.Project.WidthUserSet},
				appstorage.Leaf{Key: modeHeightKey(mode), Value: sizes.Operations.Height},
			)
		}
		return appstorage.UpsertLeafList(tx, "ui_layout", leaves)
	})
	if err != nil {
		return model.Layout{}, err
	}
	return merged, nil
}
