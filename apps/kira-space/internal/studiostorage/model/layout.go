package model

type PanelProject struct {
	Visible bool `json:"visible"`
}

type PanelOperations struct {
	Visible bool `json:"visible"`
}

// PanelSizes is one module's own panel geometry (P312); visibility stays app-wide in Layout.Panel.
type PanelSizes struct {
	Project    ProjectSize    `json:"project"`
	Operations OperationsSize `json:"operations"`
}

type ProjectSize struct {
	Width float64 `json:"width"`
	// WidthUserSet marks a width the user dragged; the Git review widen leaves those alone.
	WidthUserSet bool `json:"widthUserSet"`
}

type OperationsSize struct {
	Height float64 `json:"height"`
}

type PanelCellEditor struct {
	Height float64 `json:"height"`
}

type Layout struct {
	Panel struct {
		Project    PanelProject    `json:"project"`
		Operations PanelOperations `json:"operations"`
		CellEditor PanelCellEditor `json:"cellEditor"`
	} `json:"panel"`
	// Modes maps every WindowModes.Valid entry to its panel sizes.
	Modes map[string]PanelSizes `json:"modes"`
}

func DefaultPanelSizes() PanelSizes {
	return PanelSizes{Project: ProjectSize{Width: 260}, Operations: OperationsSize{Height: 200}}
}

// DefaultLayout mirrors packages/shared/domain/layout.ts's defaultLayout verbatim. The window
// rectangle used to live here too (a single `window.bounds` leaf for every window there has ever
// been, P8 F5) — it is now per-window, in the `windows` table (model.WindowRecord, repos.WindowsRepo).
func DefaultLayout() Layout {
	var l Layout
	l.Panel.Project = PanelProject{Visible: true}
	l.Panel.CellEditor = PanelCellEditor{Height: 180}
	l.Modes = make(map[string]PanelSizes, len(WindowModes.Valid))
	for _, m := range WindowModes.Valid {
		l.Modes[m] = DefaultPanelSizes()
	}
	return l
}

// PanelProjectPatch, PanelOperationsPatch and PanelCellEditorPatch mirror layout.ts's
// `.partial()` per-panel patch shapes.
type PanelProjectPatch struct {
	Visible *bool `json:"visible,omitempty"`
}

type PanelOperationsPatch struct {
	Visible *bool `json:"visible,omitempty"`
}

type ProjectSizePatch struct {
	Width        *float64 `json:"width,omitempty"`
	WidthUserSet *bool    `json:"widthUserSet,omitempty"`
}

type OperationsSizePatch struct {
	Height *float64 `json:"height,omitempty"`
}

type PanelSizesPatch struct {
	Project    *ProjectSizePatch    `json:"project,omitempty"`
	Operations *OperationsSizePatch `json:"operations,omitempty"`
}

type PanelCellEditorPatch struct {
	Height *float64 `json:"height,omitempty"`
}

type PanelsPatch struct {
	Project    *PanelProjectPatch    `json:"project,omitempty"`
	Operations *PanelOperationsPatch `json:"operations,omitempty"`
	CellEditor *PanelCellEditorPatch `json:"cellEditor,omitempty"`
}

type LayoutPatch struct {
	Panel *PanelsPatch               `json:"panel,omitempty"`
	Modes map[string]PanelSizesPatch `json:"modes,omitempty"`
}
