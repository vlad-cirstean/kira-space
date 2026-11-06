package model

import (
	"fmt"
	"strings"
)

// nodeKinds mirrors domain/tree.ts's nodeKindSchema — every project-tree level across all eleven
// adapters, confirmed against that file for P55 §4.6.
var nodeKinds = map[string]bool{
	"connection": true, "database": true, "schema": true, "table": true, "view": true,
	"matview": true, "function": true, "sequence": true, "column": true, "collection": true,
	"namespace": true, "key": true, "topic": true, "partition": true, "consumerGroup": true,
	"queue": true, "bucket": true, "prefix": true, "object": true, "exchange": true,
}

// ValidNodeKind mirrors domain/tree.ts's nodeKindSchema.
func ValidNodeKind(v string) bool { return nodeKinds[v] }

// PathSegment mirrors domain/tree.ts's PathSegment.
type PathSegment struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// NodePath mirrors domain/tree.ts's NodePath.
type NodePath struct {
	ConnectionID string        `json:"connectionId"`
	Segments     []PathSegment `json:"segments"`
}

// TreeNode mirrors domain/tree.ts's treeNodeSchema.
type TreeNode struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"` // the raw identifier, used to build SQL and to copy
	Path        string   `json:"path"` // encoded, relative to the connection
	HasChildren bool     `json:"hasChildren"`
	Detail      *string  `json:"detail,omitempty"`
	Badges      []string `json:"badges,omitempty"`
}

// ColumnMeta mirrors domain/tree.ts's columnMetaSchema.
type ColumnMeta struct {
	Name         string  `json:"name"`
	Position     int     `json:"position"`
	DataType     string  `json:"dataType"`
	Nullable     bool    `json:"nullable"`
	DefaultExpr  *string `json:"defaultExpr"`
	IsPrimaryKey bool    `json:"isPrimaryKey"`
	Comment      *string `json:"comment"`
}

// IndexMeta mirrors domain/tree.ts's indexMetaSchema.
type IndexMeta struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Primary bool     `json:"primary"`
	Method  *string  `json:"method"`
}

// ForeignKeyMeta mirrors domain/tree.ts's foreignKeyMetaSchema.
type ForeignKeyMeta struct {
	Name              string   `json:"name"`
	Columns           []string `json:"columns"`
	ReferencedPath    string   `json:"referencedPath"` // encoded path of the referenced table (P7)
	ReferencedColumns []string `json:"referencedColumns"`
	OnDelete          *string  `json:"onDelete"`
	OnUpdate          *string  `json:"onUpdate"`
}

// ObjectMeta mirrors domain/tree.ts's objectMetaSchema.
type ObjectMeta struct {
	Path          string           `json:"path"`
	Kind          string           `json:"kind"`
	Name          string           `json:"name"`
	QualifiedName string           `json:"qualifiedName"`
	Columns       []ColumnMeta     `json:"columns"`
	PrimaryKey    []string         `json:"primaryKey"`
	ForeignKeys   []ForeignKeyMeta `json:"foreignKeys"`
	ReferencedBy  []ForeignKeyMeta `json:"referencedBy"` // D17
	Indexes       []IndexMeta      `json:"indexes"`
	RowEstimate   *int             `json:"rowEstimate"`
	Comment       *string          `json:"comment"`
}

// RelationColumns is SchemaColumns' own element. One relation's columns, reusing
// ColumnMeta verbatim (never a second, parallel column shape) so a completion source built from
// this can never disagree with Describe about the same column's type. Deliberately smaller than
// ObjectMeta: no PrimaryKey/ForeignKeys/ReferencedBy/Indexes/RowEstimate/object-level Comment —
// SchemaColumns exists to spell a column name and its type across a whole container in one round
// trip, not to fetch a schema's entire constraint graph.
type RelationColumns struct {
	Name    string       `json:"name"`
	Kind    string       `json:"kind"` // "table" | "view" | "matview"
	Columns []ColumnMeta `json:"columns"`
}

var relationColumnsKinds = map[string]bool{"table": true, "view": true, "matview": true}

// ValidateRelationColumns mirrors ValidateObjectMeta's discipline for the metadata_cache's
// "columns" payload kind: every relation's Kind must be one that has columns,
// and Name/Columns must be well-formed. A nil Columns list normalizes to [] first, the same
// json.Unmarshal-leaves-an-absent-array-nil reason ValidateObjectMeta already documents.
func ValidateRelationColumns(rels []RelationColumns) bool {
	for i := range rels {
		if rels[i].Columns == nil {
			rels[i].Columns = []ColumnMeta{}
		}
		if rels[i].Name == "" || !relationColumnsKinds[rels[i].Kind] {
			return false
		}
	}
	return true
}

// EncodePath mirrors tree.ts:33-35's encodePath: 'schema:public/table:order%2Fitems' — the
// connection id is not part of the string (D6).
func EncodePath(segments []PathSegment) string {
	parts := make([]string, len(segments))
	for i, s := range segments {
		parts[i] = s.Kind + ":" + EncodeURIComponent(s.Name)
	}
	return strings.Join(parts, "/")
}

// DecodePath ports tree.ts:37-49 exactly: "" -> no segments; split on '/'; each segment is
// <kind>:<encodeURIComponent(name)>; an unknown kind or a missing ':' is an error (the TS
// throws).
func DecodePath(connectionID, encoded string) (NodePath, error) {
	if encoded == "" {
		return NodePath{ConnectionID: connectionID, Segments: []PathSegment{}}, nil
	}
	raw := strings.Split(encoded, "/")
	segments := make([]PathSegment, len(raw))
	for i, seg := range raw {
		sep := strings.IndexByte(seg, ':')
		if sep < 0 {
			return NodePath{}, fmt.Errorf("model: malformed path segment: %s", seg)
		}
		kind := seg[:sep]
		if !ValidNodeKind(kind) {
			return NodePath{}, fmt.Errorf("model: unknown node kind in path segment: %s", kind)
		}
		segments[i] = PathSegment{Kind: kind, Name: DecodeURIComponent(seg[sep+1:])}
	}
	return NodePath{ConnectionID: connectionID, Segments: segments}, nil
}

// ValidateTreeNodes is the explicit check that replaces zod's safeParse for a cached []TreeNode
// row (P55 §1.6): every element's Kind must be a real NodeKind, and Name/Path non-empty.
func ValidateTreeNodes(nodes []TreeNode) bool {
	for _, n := range nodes {
		if !ValidNodeKind(n.Kind) || n.Name == "" || n.Path == "" {
			return false
		}
	}
	return true
}

// ValidateObjectMeta reports whether meta is well-formed, normalizing its four list fields from
// nil to [] first — json.Unmarshal leaves an absent JSON array as nil, not zod's own
// always-an-array guarantee, so a valid meta with an empty list must still come back usable.
func ValidateObjectMeta(meta *ObjectMeta) bool {
	if meta.Columns == nil {
		meta.Columns = []ColumnMeta{}
	}
	if meta.PrimaryKey == nil {
		meta.PrimaryKey = []string{}
	}
	if meta.ForeignKeys == nil {
		meta.ForeignKeys = []ForeignKeyMeta{}
	}
	if meta.ReferencedBy == nil {
		meta.ReferencedBy = []ForeignKeyMeta{}
	}
	if meta.Indexes == nil {
		meta.Indexes = []IndexMeta{}
	}
	return ValidNodeKind(meta.Kind) && meta.Path != "" && meta.Name != "" && meta.QualifiedName != ""
}
