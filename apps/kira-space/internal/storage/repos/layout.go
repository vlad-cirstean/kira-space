package repos

import (
	"database/sql"
	"encoding/json"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

var layoutLeaves = sqlb.LeafTable{Table: kiraschema.UiLayout.Table, Key: kiraschema.UiLayout.Key, Value: kiraschema.UiLayout.Value}

// layoutSelectAllSQL is the "every stored leaf" query repos.New prepares once.
var layoutSelectAllSQL = func() string {
	q, _ := sqlb.SQLite.Select(layoutLeaves.Key, layoutLeaves.Value).From(layoutLeaves.Table).Build()
	return q
}()

// LayoutRepo reads and writes the `ui_layout` table — Kira Studio's own LayoutRepo, trimmed to
// this app's own model.Layout (P100 Part 2).
type LayoutRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect

	// selectAll is prepared once by repos.New; nil when constructed directly, which falls back to
	// an ad-hoc query with identical SQL.
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
	for mode, sizes := range result.Modes {
		appsettings.Leaf(stored, modeWidthKey(mode), &sizes.Project.Width)
		appsettings.Leaf(stored, modeWidthUserSetKey(mode), &sizes.Project.WidthUserSet)
		appsettings.Leaf(stored, modeHeightKey(mode), &sizes.Operations.Height)
		result.Modes[mode] = sizes
	}
	appsettings.LeafValid(stored, "panel.ade.width", &result.Panel.Ade.Width, model.ValidAdePanelWidth)
	appsettings.LeafValid(stored, "panel.cellEditor.height", &result.Panel.CellEditor.Height, model.ValidCellEditorHeight)
	return result
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
	stored, err := sqlb.QueryLeaves(r.DB, r.Dialect, r.selectAll, layoutLeaves)
	if err != nil {
		return model.Layout{}, err
	}
	return decodeLayout(stored), nil
}

// Set writes every leaf every time, mirroring Kira Studio's own LayoutRepo.Set (the read-modify-
// write runs inside one transaction — storage/db.go's SetMaxOpenConns(1) makes that transaction's
// Begin() hold the database's one connection exclusively until Commit/Rollback, so no concurrent
// Set can race it — same fix Kira Studio's own copy documents).
func (r *LayoutRepo) Set(patch model.LayoutPatch) (model.Layout, error) {
	var merged model.Layout
	err := sqlb.UpdateLeaves(r.DB, r.Dialect, r.selectAll, layoutLeaves, func(tx *sql.Tx, stored map[string]json.RawMessage) error {
		merged = decodeLayout(stored)
		applyModesPatch(&merged, patch.Modes)
		if p := patch.Panel; p != nil {
			if p.Project != nil && p.Project.Visible != nil {
				merged.Panel.Project.Visible = *p.Project.Visible
			}
			if p.Ade != nil && p.Ade.Width != nil {
				merged.Panel.Ade.Width = *p.Ade.Width
			}
			if p.CellEditor != nil && p.CellEditor.Height != nil {
				merged.Panel.CellEditor.Height = *p.CellEditor.Height
			}
		}

		leaves := make([]sqlb.Leaf, 0, 3+3*len(merged.Modes))
		leaves = append(leaves,
			sqlb.Leaf{Key: "panel.project.visible", Value: merged.Panel.Project.Visible},
			sqlb.Leaf{Key: "panel.ade.width", Value: merged.Panel.Ade.Width},
			sqlb.Leaf{Key: "panel.cellEditor.height", Value: merged.Panel.CellEditor.Height},
		)
		for mode, sizes := range merged.Modes {
			leaves = append(leaves,
				sqlb.Leaf{Key: modeWidthKey(mode), Value: sizes.Project.Width},
				sqlb.Leaf{Key: modeWidthUserSetKey(mode), Value: sizes.Project.WidthUserSet},
				sqlb.Leaf{Key: modeHeightKey(mode), Value: sizes.Operations.Height},
			)
		}
		return sqlb.UpsertLeafList(tx, r.Dialect, layoutLeaves, leaves)
	})
	if err != nil {
		return model.Layout{}, err
	}
	return merged, nil
}
