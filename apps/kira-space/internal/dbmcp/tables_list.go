package dbmcp

import (
	"context"
	"sort"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

// TableEntry is one relation the MCP tab can list.
type TableEntry struct {
	Ref  string `json:"ref"`  // canonical allow list entry, tableref.Format
	Kind string `json:"kind"` // table, view, matview, collection
}

// TablesListing is the live relations of a connection.
type TablesListing struct {
	Tables    []TableEntry `json:"tables"`
	Truncated bool         `json:"truncated"`
}

// tablesListMax caps the walk so a huge catalog cannot stall the dialog.
const tablesListMax = 5000

// ListTables walks the tree from the root through database and schema nodes and collects the relations, the
// same cached metadata calls the left panel makes. A kind the allow list does not apply to lists nothing.
func ListTables(ctx context.Context, tree MetadataReader, summary model.ConnectionSummary) (TablesListing, error) {
	out := TablesListing{Tables: []TableEntry{}}
	if !tableref.Applies(summary.Kind) {
		return out, nil
	}
	var walk func(path string) error
	walk = func(path string) error {
		res, err := tree.Children(ctx, summary.ID, path, false)
		if err != nil {
			return err
		}
		for _, n := range res.Nodes {
			if out.Truncated {
				return nil
			}
			switch {
			case n.Kind == "database" || n.Kind == "schema":
				if err := walk(n.Path); err != nil {
					return err
				}
			case tableref.IsRelationKind(n.Kind):
				p, err := model.DecodePath(summary.ID, n.Path)
				if err != nil {
					continue
				}
				ref, ok := tableref.FromPath(summary.Kind, p.Segments)
				if !ok {
					continue
				}
				if len(out.Tables) >= tablesListMax {
					out.Truncated = true
					return nil
				}
				out.Tables = append(out.Tables, TableEntry{Ref: tableref.Format(summary.Kind, ref), Kind: n.Kind})
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return TablesListing{}, err
	}
	sort.Slice(out.Tables, func(i, j int) bool { return out.Tables[i].Ref < out.Tables[j].Ref })
	return out, nil
}
