package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/datagrip"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// DataGripService is the importer: a folder (the DataGrip project's own) crosses the bridge as
// a path, exactly like CollectionsService.Import (the "only the path crosses the bridge"
// convention) — here it is a security property too, since the decrypted password never crosses
// the bridge at all. Both methods return their internal/datagrip result type directly
// (Preview/Report already carry the JSON tags the wire shape needs), the same way
// ConnectionsService.List returns model.ConnectionSummary directly rather than through a mirror
// type.
type DataGripService struct {
	Deps appcore.Deps
}

type DataGripScanArgs struct {
	Path string `json:"path"`
}

// Scan is D9's first step — file reads only, never a credential store (F11).
func (s *DataGripService) Scan(args DataGripScanArgs) (datagrip.Preview, error) {
	if args.Path == "" {
		return datagrip.Preview{}, ipcerr.Required("path")
	}
	preview, err := datagrip.Scan(args.Path)
	if err != nil {
		// The project folder/files themselves are what's wrong here (no .idea/dataSources.xml,
		// an oversized or malformed file) — the user's to fix, not an internal failure.
		return datagrip.Preview{}, ipcerr.BadRequest(err.Error())
	}
	return *preview, nil
}

type DataGripImportArgs struct {
	Path          string   `json:"path"`
	SelectedUUIDs []string `json:"selectedUuids"`
}

// datagripCreator adapts *connections.Service into datagrip.Creator — the one place
// datagrip.CreatorInput (D3's field-for-field mirror of connections.Input) becomes the real
// connections.Input, so internal/datagrip itself never has to import internal/connections.
type datagripCreator struct{ svc *connections.Service }

func (c datagripCreator) Create(in datagrip.CreatorInput) (model.ConnectionSummary, error) {
	return c.svc.Create(connections.Input{ConnectionFields: in.ConnectionFields, Password: in.Password})
}

// Apply re-parses, fetches and decrypts each selected row's password once, and calls Create
// through the adapter above. This app's own secret-storage availability (not DataGrip's) is
// checked once here rather than inside internal/datagrip, which has no dependency on
// internal/secrets at all.
func (s *DataGripService) Import(args DataGripImportArgs) (datagrip.Report, error) {
	if args.Path == "" {
		return datagrip.Report{}, ipcerr.Required("path")
	}
	if len(args.SelectedUUIDs) == 0 {
		return datagrip.Report{}, ipcerr.Required("selectedUuids")
	}
	status := s.Deps.Connections.SecretsStatus()
	report, err := datagrip.Apply(args.Path, args.SelectedUUIDs, status.Available, datagripCreator{svc: s.Deps.Connections})
	if err != nil {
		return datagrip.Report{}, ipcerr.BadRequest(err.Error())
	}
	return report, nil
}
