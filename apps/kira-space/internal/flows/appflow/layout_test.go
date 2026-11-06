package appflow_test

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// P266: the ADE panel width is a layout leaf, patched alone, kept across a restart and range-checked.
func TestLayoutAdePanelWidth(t *testing.T) {
	app := flowharness.New(t)
	width := 480.0
	patch := model.LayoutPatch{Panel: &model.PanelsPatch{Ade: &model.PanelAdePatch{Width: &width}}}
	merged, err := app.W.Layout.Set(bridge.LayoutSetArgs{Patch: patch})
	if err != nil || merged.Panel.Ade.Width != 480 {
		t.Fatalf("Layout.Set = %+v, %v, want ade width 480", merged.Panel.Ade, err)
	}
	if got := merged.Modes["git"].Project.Width; got != 260 {
		t.Errorf("project width = %v, want the 260 default untouched", got)
	}

	app.Restart()
	got, err := app.W.Layout.GetAll()
	if err != nil || got.Panel.Ade.Width != 480 {
		t.Fatalf("Layout.GetAll after restart = %+v, %v, want ade width 480", got.Panel.Ade, err)
	}

	tooNarrow := 100.0
	bad := model.LayoutPatch{Panel: &model.PanelsPatch{Ade: &model.PanelAdePatch{Width: &tooNarrow}}}
	_, err = app.W.Layout.Set(bridge.LayoutSetArgs{Patch: bad})
	var coded *ipcerr.Error
	if !errors.As(err, &coded) || coded.Code != string(ipcerr.CodeContract) {
		t.Fatalf("Layout.Set(width 100) = %v, want %s", err, ipcerr.CodeContract)
	}
}

// P538: the cell editor dock height is a layout leaf, kept across a restart and range-checked.
func TestLayoutCellEditorHeight(t *testing.T) {
	app := flowharness.New(t)
	set := func(h float64) (model.Layout, error) {
		return app.W.Layout.Set(bridge.LayoutSetArgs{Patch: model.LayoutPatch{Panel: &model.PanelsPatch{CellEditor: &model.PanelCellEditorPatch{Height: &h}}}})
	}
	if got, err := app.W.Layout.GetAll(); err != nil || got.Panel.CellEditor.Height != 180 {
		t.Fatalf("Layout.GetAll default = %+v, %v, want cell editor height 180", got.Panel.CellEditor, err)
	}
	if merged, err := set(300); err != nil || merged.Panel.CellEditor.Height != 300 {
		t.Fatalf("Layout.Set = %+v, %v, want cell editor height 300", merged.Panel.CellEditor, err)
	}

	app.Restart()
	got, err := app.W.Layout.GetAll()
	if err != nil || got.Panel.CellEditor.Height != 300 {
		t.Fatalf("Layout.GetAll after restart = %+v, %v, want cell editor height 300", got.Panel.CellEditor, err)
	}
	app.Contract(t, "layout-cell-editor", "LayoutService.GetAll#cellEditor", got)

	for _, h := range []float64{50, 900} {
		_, err = set(h)
		var coded *ipcerr.Error
		if !errors.As(err, &coded) || coded.Code != string(ipcerr.CodeContract) {
			t.Fatalf("Layout.Set(height %v) = %v, want %s", h, err, ipcerr.CodeContract)
		}
	}
}

// P312: panel sizes are per module; a patch to one module leaves the others at their own values.
func TestLayoutPanelSizesPerModule(t *testing.T) {
	app := flowharness.New(t)
	width, height, userSet := 410.0, 310.0, true
	patch := model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{
		"git": {Project: &model.ProjectSizePatch{Width: &width, WidthUserSet: &userSet}, Operations: &model.OperationsSizePatch{Height: &height}},
	}}
	merged, err := app.W.Layout.Set(bridge.LayoutSetArgs{Patch: patch})
	if err != nil || merged.Modes["git"].Project.Width != width || merged.Modes["git"].Operations.Height != height {
		t.Fatalf("Layout.Set = %+v, %v, want 410/310", merged.Modes["git"], err)
	}
	if got := merged.Modes["automations"]; got != model.DefaultPanelSizes() {
		t.Errorf("automations sizes = %+v, want the defaults untouched", got)
	}

	app.Restart()
	got, err := app.W.Layout.GetAll()
	if err != nil || got.Modes["git"].Project.Width != width || !got.Modes["git"].Project.WidthUserSet || got.Modes["automations"] != model.DefaultPanelSizes() {
		t.Fatalf("Layout.GetAll after restart = %+v, %v", got.Modes, err)
	}

	unknown := model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{"bogus": {}}}
	_, err = app.W.Layout.Set(bridge.LayoutSetArgs{Patch: unknown})
	var coded *ipcerr.Error
	if !errors.As(err, &coded) || coded.Code != string(ipcerr.CodeContract) {
		t.Fatalf("Layout.Set(unknown mode) = %v, want %s", err, ipcerr.CodeContract)
	}
}
