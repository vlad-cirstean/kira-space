package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// SchemaService is the DDL half of P18 (v1.1)'s language service: Get/Set for the schema_ddl text in
// a connection's file.
type SchemaService struct {
	Deps appcore.StudioDeps
}

type SchemaGetArgs struct {
	ConnectionID string `json:"connectionId"`
}

func (s *SchemaService) Get(args SchemaGetArgs) (model.ConnectionDDL, error) {
	if args.ConnectionID == "" {
		return model.ConnectionDDL{}, ipcerr.Required("connectionId")
	}
	rec, ok := s.Deps.ConnectionFiles.RecordOf(args.ConnectionID)
	if !ok {
		return model.ConnectionDDL{}, connfiles.IPCError(connfiles.ErrNotFound)
	}
	out := model.ConnectionDDL{ConnectionID: args.ConnectionID, DDL: rec.File.SchemaDDL}
	if row, err := s.Deps.Repos.Connections.Get(args.ConnectionID); err == nil && row != nil {
		out.UpdatedAt = row.UpdatedAt
	}
	return out, nil
}

type SchemaSetArgs struct {
	ConnectionID string `json:"connectionId"`
	DDL          string `json:"ddl"`
}

// Set writes the DDL text into the file. The load that follows broadcasts the change, so a console open
// in another window picks it up.
func (s *SchemaService) Set(args SchemaSetArgs) (model.ConnectionDDL, error) {
	if args.ConnectionID == "" {
		return model.ConnectionDDL{}, ipcerr.Required("connectionId")
	}
	if err := s.Deps.ConnectionFiles.Patch(args.ConnectionID, configfs.NodeSet{Path: []string{"schema_ddl"}, Value: args.DDL}); err != nil {
		return model.ConnectionDDL{}, connfiles.IPCError(err)
	}
	return s.Get(SchemaGetArgs{ConnectionID: args.ConnectionID})
}
