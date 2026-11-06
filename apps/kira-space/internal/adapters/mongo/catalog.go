package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// systemDatabases mirrors catalog.ts's SYSTEM_DATABASES — databases mongod itself owns and that no
// user connection meaningfully browses, the same exclusion mysql-family/catalog.go applies for
// information_schema et al.
var systemDatabases = map[string]bool{"admin": true, "local": true, "config": true}

// listDatabases ports catalog.ts's listDatabases.
func listDatabases(ctx context.Context, client *mongodriver.Client) ([]model.TreeNode, error) {
	result, err := client.ListDatabases(ctx, bson.D{}, options.ListDatabases().SetNameOnly(true))
	if err != nil {
		return nil, mapError(err)
	}
	nodes := make([]model.TreeNode, 0, len(result.Databases))
	for _, d := range result.Databases {
		if systemDatabases[d.Name] {
			continue
		}
		nodes = append(nodes, model.TreeNode{
			Kind:        "database",
			Name:        d.Name,
			Path:        model.EncodePath([]model.PathSegment{{Kind: "database", Name: d.Name}}),
			HasChildren: true,
		})
	}
	return nodes, nil
}

// listCollections ports catalog.ts's listCollections. §5.1: database -> collections; no
// routine/sequence-equivalent kind for Mongo.
func listCollections(ctx context.Context, db *mongodriver.Database) ([]model.TreeNode, error) {
	specs, err := db.ListCollectionSpecifications(ctx, bson.D{}, options.ListCollections().SetNameOnly(true))
	if err != nil {
		return nil, mapError(err)
	}
	nodes := make([]model.TreeNode, len(specs))
	for i, c := range specs {
		var detail *string
		if c.Type == "view" {
			view := "view"
			detail = &view
		}
		nodes[i] = model.TreeNode{
			Kind: "collection",
			Name: c.Name,
			Path: model.EncodePath([]model.PathSegment{
				{Kind: "database", Name: db.Name()},
				{Kind: "collection", Name: c.Name},
			}),
			// P19 D5's own SQL-relation precedent: a collection's indexes moved into the
			// definition view, so a collection is a leaf like a table — no tree expand arrow.
			HasChildren: false,
			Detail:      detail,
		}
	}
	sortTreeNodesByName(nodes)
	return nodes, nil
}

func sortTreeNodesByName(nodes []model.TreeNode) {
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0 && nodes[j-1].Name > nodes[j].Name; j-- {
			nodes[j-1], nodes[j] = nodes[j], nodes[j-1]
		}
	}
}

// collectionOptions is the definition view's own lookup (P19 D12) — never called from the tree's
// listCollections, which keeps NameOnly true so a database expand costs nothing extra per
// collection. Returns nil (not an error) when the collection has no recorded options document.
func collectionOptions(ctx context.Context, db *mongodriver.Database, collection string) (bson.D, error) {
	specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: collection}})
	if err != nil {
		return nil, mapError(err)
	}
	if len(specs) == 0 || len(specs[0].Options) == 0 {
		return nil, nil
	}
	var optsDoc bson.D
	if err := bson.Unmarshal(specs[0].Options, &optsDoc); err != nil {
		return nil, mapError(err)
	}
	return optsDoc, nil
}

// IndexInfo mirrors catalog.ts's MongoIndexInfo.
type IndexInfo struct {
	Name    string
	Columns []string
	Unique  bool
}

// describeIndexes ports catalog.ts's describeIndexes.
func describeIndexes(ctx context.Context, db *mongodriver.Database, collection string) ([]IndexInfo, error) {
	specs, err := db.Collection(collection).Indexes().ListSpecifications(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]IndexInfo, len(specs))
	for i, spec := range specs {
		elements, err := spec.KeysDocument.Elements()
		if err != nil {
			return nil, mapError(err)
		}
		columns := make([]string, len(elements))
		for j, e := range elements {
			columns[j] = e.Key()
		}
		out[i] = IndexInfo{
			Name:    spec.Name,
			Columns: columns,
			Unique:  spec.Unique != nil && *spec.Unique,
		}
	}
	return out, nil
}
