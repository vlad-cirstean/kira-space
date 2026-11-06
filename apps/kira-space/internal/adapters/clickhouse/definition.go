package clickhouse

import (
	"context"
	"strconv"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

func toConstraintMeta(row constraintRow) model.ConstraintMeta {
	return model.ConstraintMeta{Name: row.Name, Type: "check", Definition: row.Expression}
}

// buildTableSection is definition.ts's own — the "Table properties" section, this
// adapter's first real use of the generic name/value block every other SQL engine's definition()
// leaves empty, since none of Engine, Sorting key, Partition key or the sparse Primary key
// expression has anywhere else in ObjectDefinition to live.
func buildTableSection(target ReadTarget) model.DefinitionSection {
	sortingKey := target.SortingKey
	if sortingKey == "" {
		sortingKey = "(none)"
	}
	primaryKey := target.PrimaryKeyExpression
	if primaryKey == "" {
		primaryKey = "(none)"
	}
	partitionKey := target.PartitionKey
	if partitionKey == "" {
		partitionKey = "(none)"
	}
	totalRows := "(unknown)"
	if target.TotalRows != nil {
		totalRows = formatThousands(*target.TotalRows)
	}
	rows := []model.DefinitionSectionRow{
		{Name: "Engine", Value: target.Engine},
		{Name: "Sorting key", Value: sortingKey},
		{Name: "Primary key", Value: primaryKey},
		{Name: "Partition key", Value: partitionKey},
		{Name: "Total rows", Value: totalRows},
	}
	if target.Comment != nil && *target.Comment != "" {
		rows = append(rows, model.DefinitionSectionRow{Name: "Comment", Value: *target.Comment})
	}
	return model.DefinitionSection{Title: i18n.T("backend.definition.section.tableProperties.title", nil), Rows: rows}
}

// formatThousands is toLocaleString()'s own thousands-grouping, the one piece of that call site
// worth porting rather than dropping: a plain strconv.FormatInt would silently lose the grouping
// dots/commas the TS's own definition view has always shown.
func formatThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := ""
	if s[0] == '-' {
		neg, s = "-", s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return neg + string(out)
}

// buildDefinition is definition.ts's own.
func buildDefinition(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, segments []model.PathSegment, schema string, kind, name string) (model.ObjectDefinition, error) {
	target, err := getReadTarget(ctx, h, queryID, op, track, schema, name)
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	statement := adapters.StripOneTrailingSemicolon(target.CreateTableQuery)
	if statement == "" {
		return model.ObjectDefinition{}, adapters.New(adapters.CodeQuery, `no definition returned for "`+schema+`"."`+name+`"`, nil)
	}

	checkRows := listCheckConstraints(target.CreateTableQuery)
	constraints := make([]model.ConstraintMeta, len(checkRows))
	for i, r := range checkRows {
		constraints[i] = toConstraintMeta(r)
	}

	notes := []string{
		"A MergeTree PRIMARY KEY is a sparse index, not a uniqueness constraint — see Table properties.",
		"ClickHouse has no foreign keys.",
	}

	return model.ObjectDefinition{
		Path: model.EncodePath(segments), Kind: kind, QualifiedName: schema + "." + name,
		Statements: []string{statement}, Language: "sql", Origin: "server", Notes: notes,
		Constraints: constraints, DocumentSchema: nil, Sections: []model.DefinitionSection{buildTableSection(target)},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}
