package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Tables lists the live tables, views and collections of a connection for the MCP tab's table allow list.
// A connection that is not connected fails with the tree's own error.
func (s *DbMcpService) Tables(ctx context.Context, args ConnectionsIDArgs) (dbmcp.TablesListing, error) {
	if args.ID == "" {
		return dbmcp.TablesListing{}, ipcerr.Required("id")
	}
	conns, err := s.Deps.Connections.List()
	if err != nil {
		return dbmcp.TablesListing{}, ipcerr.Wrap(err)
	}
	for _, c := range conns {
		if c.ID == args.ID {
			listing, err := dbmcp.ListTables(ctx, s.Deps.Tree, c)
			return listing, ipcerr.Wrap(err)
		}
	}
	return dbmcp.TablesListing{}, ipcerr.NotFoundNamed(args.ID)
}
