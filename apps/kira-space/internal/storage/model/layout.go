package model

// PanelProject is the repo-tree panel's own persisted state. Operations dock visibility is not
// persisted.
type PanelProject struct {
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

// PanelAde is the ADE side panel's own width in px; 0 means "half the window", not a zero-width panel.
type PanelAde struct {
	Width float64 `json:"width"`
}

// PanelCellEditor is the DB grid's cell editor dock height in px.
type PanelCellEditor struct {
	Height float64 `json:"height"`
}

type Layout struct {
	Panel struct {
		Project    PanelProject    `json:"project"`
		Ade        PanelAde        `json:"ade"`
		CellEditor PanelCellEditor `json:"cellEditor"`
	} `json:"panel"`
	// Modes maps every WindowModes.Valid entry to its panel sizes.
	Modes map[string]PanelSizes `json:"modes"`
}

func DefaultPanelSizes() PanelSizes {
	return PanelSizes{Project: ProjectSize{Width: 260}, Operations: OperationsSize{Height: 200}}
}

// ValidAdePanelWidth accepts 0 (="half the window") or an explicit 340..4000 drag width.
func ValidAdePanelWidth(v float64) bool {
	return v == 0 || (v >= 340 && v <= 4000)
}

// ValidCellEditorHeight accepts the dock's 120..480 resize range.
func ValidCellEditorHeight(v float64) bool {
	return v >= 120 && v <= 480
}

// DefaultLayout mirrors Kira Studio's own DefaultLayout, minus the two dropped panels.
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

// PanelProjectPatch mirrors Kira Studio's own PanelProjectPatch — the `.partial()` patch shape.
type PanelProjectPatch struct {
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

type PanelAdePatch struct {
	Width *float64 `json:"width,omitempty"`
}

type PanelCellEditorPatch struct {
	Height *float64 `json:"height,omitempty"`
}

type PanelsPatch struct {
	Project    *PanelProjectPatch    `json:"project,omitempty"`
	Ade        *PanelAdePatch        `json:"ade,omitempty"`
	CellEditor *PanelCellEditorPatch `json:"cellEditor,omitempty"`
}

type LayoutPatch struct {
	Panel *PanelsPatch               `json:"panel,omitempty"`
	Modes map[string]PanelSizesPatch `json:"modes,omitempty"`
}
