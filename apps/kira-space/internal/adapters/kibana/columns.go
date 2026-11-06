package kibana

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// tableColumns validates the requested columns and puts the time field first, as Discover does.
func tableColumns(dv dataView, cols []string) ([]model.KibanaField, error) {
	seen := map[string]bool{}
	for _, c := range cols {
		if seen[c] {
			return nil, queryErr("backend.studio.adapter.kibanaDuplicateColumn.body", map[string]string{"field": c})
		}
		seen[c] = true
		if _, ok := dv.field(c); !ok {
			return nil, queryErr("backend.studio.adapter.kibanaUnknownColumn.body", map[string]string{"field": c})
		}
	}
	names := slices.Clone(cols)
	if dv.TimeFieldName != nil && !seen[*dv.TimeFieldName] {
		if _, ok := dv.field(*dv.TimeFieldName); ok {
			names = slices.Insert(names, 0, *dv.TimeFieldName)
		}
	}
	out := make([]model.KibanaField, len(names))
	for i, n := range names {
		out[i], _ = dv.field(n)
	}
	return out, nil
}

// columnsBody switches a hits body to the fields API for the table columns.
func columnsBody(body map[string]any, fields []model.KibanaField) {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
		if f.Nested != nil && !slices.Contains(names, *f.Nested) {
			names = append(names, *f.Nested)
		}
	}
	body["_source"] = false
	body["fields"] = names
}

func typeClass(kibanaType string) page.TypeClass {
	switch kibanaType {
	case "number":
		return page.TypeClassNumber
	case "date", "date_nanos":
		return page.TypeClassTemporal
	case "boolean":
		return page.TypeClassBoolean
	case "geo_point", "geo_shape":
		return page.TypeClassJSON
	default:
		return page.TypeClassText
	}
}

// cellText renders the values of one field: none is NULL, one is its text, several a JSON array.
func cellText(vals []json.RawMessage) *string {
	switch len(vals) {
	case 0:
		return nil
	case 1:
		s := scalarText(vals[0])
		return &s
	}
	b, _ := json.Marshal(vals)
	s := string(b)
	return &s
}

func scalarText(v json.RawMessage) string {
	v = bytes.TrimSpace(v)
	if len(v) > 0 && v[0] == '"' {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
	}
	var buf bytes.Buffer
	if json.Compact(&buf, v) == nil {
		return buf.String()
	}
	return string(v)
}

// hitValues returns the values of one column. Elasticsearch groups a nested field's values per
// nested object under the nested path.
func hitValues(h searchHit, f model.KibanaField) []json.RawMessage {
	if f.Nested == nil {
		return h.Fields[f.Name]
	}
	rel := strings.TrimPrefix(f.Name, *f.Nested+".")
	var out []json.RawMessage
	for _, raw := range h.Fields[*f.Nested] {
		var obj map[string][]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		if vals, ok := obj[rel]; ok {
			out = append(out, vals...)
		} else {
			out = append(out, obj[f.Name]...)
		}
	}
	return out
}

// columnPage renders hits as a table of fields. It reports whether the cap stopped it early.
func columnPage(fields []model.KibanaField, hits []searchHit, limit page.ResultCap) (*page.TabularPageBuilder, bool, error) {
	descs := make([]page.ColumnDescriptor, len(fields))
	for i, f := range fields {
		descs[i] = page.ColumnDescriptor{Name: f.Name, DataType: f.Type, TypeClass: typeClass(f.Type), Nullable: true}
	}
	builder := page.NewTabularPageBuilder(descs)
	for _, h := range hits {
		if limit.Reached(builder.RowCount(), builder.Bytes()) {
			return builder, true, nil
		}
		row := make([]*string, len(fields))
		for i, f := range fields {
			row[i] = cellText(hitValues(h, f))
		}
		if err := builder.AppendRow(row); err != nil {
			return nil, false, err
		}
	}
	return builder, false, nil
}
