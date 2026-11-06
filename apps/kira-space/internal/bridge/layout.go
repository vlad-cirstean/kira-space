package bridge

import (
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// LayoutService is Kira Studio's own LayoutService (internal/bridge/layout.go), unchanged shape —
// only model.Layout itself is trimmed (storage/model/layout.go).
type LayoutService struct {
	Deps appcore.Deps
}

func (s *LayoutService) GetAll() (model.Layout, error) {
	return ipcerr.InternalResult(s.Deps.Repos.Layout.GetAll())
}

type LayoutSetArgs struct {
	Patch model.LayoutPatch `json:"patch"`
}

// Set merges and persists the patch, then broadcasts the merged layout unconditionally — panel
// layout is app-wide by design, so every window must agree.
func (s *LayoutService) Set(args LayoutSetArgs) (model.Layout, error) {
	if p := args.Patch.Panel; p != nil && p.Ade != nil && p.Ade.Width != nil && !model.ValidAdePanelWidth(*p.Ade.Width) {
		return model.Layout{}, ipcerr.Contract("layout: panel.ade.width: invalid value %v", *p.Ade.Width)
	}
	if p := args.Patch.Panel; p != nil && p.CellEditor != nil && p.CellEditor.Height != nil && !model.ValidCellEditorHeight(*p.CellEditor.Height) {
		return model.Layout{}, ipcerr.Contract("layout: panel.cellEditor.height: invalid value %v", *p.CellEditor.Height)
	}
	for mode := range args.Patch.Modes {
		if !slices.Contains(model.WindowModes.Valid, mode) {
			return model.Layout{}, ipcerr.Contract("layout: modes: unknown mode %q", mode)
		}
	}
	merged, err := s.Deps.Repos.Layout.Set(args.Patch)
	if err != nil {
		return model.Layout{}, ipcerr.InternalErr(err)
	}
	s.Deps.Events.Emit(ChannelLayoutChanged, merged)
	return merged, nil
}
