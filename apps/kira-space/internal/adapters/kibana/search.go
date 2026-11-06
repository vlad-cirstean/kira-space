package kibana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/kibana/kql"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const (
	defaultSampleSize = 500
	searchTimeout     = "30s"
)

func queryErr(key string, params map[string]string) error {
	return adapters.T(adapters.CodeQuery, key, params, nil)
}

func kqlFields(dv dataView) []kql.Field {
	out := make([]kql.Field, len(dv.Fields))
	for i, f := range dv.Fields {
		out[i] = kql.Field{Name: f.Name, Type: f.Type, Searchable: f.Searchable}
		if f.Nested != nil {
			out[i].Nested = *f.Nested
		}
	}
	return out
}

func (dv dataView) field(name string) (model.KibanaField, bool) {
	for _, f := range dv.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return model.KibanaField{}, false
}

func isDateMath(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "now") }

// timeClause is the data view's time field restricted to the request's range.
func timeClause(dv dataView, req model.KibanaDiscoverRequest) map[string]any {
	if dv.TimeFieldName == nil || req.TimeRange == nil {
		return nil
	}
	r := map[string]any{}
	if req.TimeRange.From != "" {
		r["gte"] = req.TimeRange.From
	}
	if req.TimeRange.To != "" {
		r["lte"] = req.TimeRange.To
	}
	if len(r) == 0 {
		return nil
	}
	if !isDateMath(req.TimeRange.From) && !isDateMath(req.TimeRange.To) {
		r["format"] = "strict_date_optional_time"
	} else if req.TimeZone != "" {
		r["time_zone"] = req.TimeZone
	}
	return map[string]any{"range": map[string]any{*dv.TimeFieldName: r}}
}

// filterClause compiles one pill. A nil clause with nil error is a disabled filter.
func filterClause(dv dataView, f model.KibanaFilter) (clause map[string]any, negate bool, err error) {
	if f.Disabled {
		return nil, false, nil
	}
	info, ok := dv.field(f.Field)
	if !ok {
		return nil, false, queryErr("backend.studio.adapter.kibanaUnknownField.body", map[string]string{"field": f.Field})
	}
	phrase := func(v string) map[string]any {
		return map[string]any{"match_phrase": map[string]any{f.Field: v}}
	}
	oneOf := func() map[string]any {
		should := make([]any, len(f.Values))
		for i, v := range f.Values {
			should[i] = phrase(v)
		}
		return map[string]any{"bool": map[string]any{"should": should, "minimum_should_match": 1}}
	}
	switch f.Operator {
	case "is", "is_not":
		if len(f.Values) != 1 {
			return nil, false, queryErr("backend.studio.adapter.kibanaFilterValues.body", map[string]string{"field": f.Field})
		}
		clause, negate = phrase(f.Values[0]), f.Operator == "is_not"
	case "is_one_of", "is_not_one_of":
		if len(f.Values) == 0 {
			return nil, false, queryErr("backend.studio.adapter.kibanaFilterValues.body", map[string]string{"field": f.Field})
		}
		clause, negate = oneOf(), f.Operator == "is_not_one_of"
	case "exists", "does_not_exist":
		clause, negate = map[string]any{"exists": map[string]any{"field": f.Field}}, f.Operator == "does_not_exist"
	case "between":
		if len(f.Values) != 2 {
			return nil, false, queryErr("backend.studio.adapter.kibanaFilterValues.body", map[string]string{"field": f.Field})
		}
		r := map[string]any{}
		if f.Values[0] != "" {
			r["gte"] = f.Values[0]
		}
		if f.Values[1] != "" {
			r["lt"] = f.Values[1]
		}
		clause = map[string]any{"range": map[string]any{f.Field: r}}
	default:
		return nil, false, queryErr("backend.studio.adapter.kibanaFilterOperator.body", map[string]string{"operator": f.Operator})
	}
	if info.Nested != nil {
		clause = map[string]any{"nested": map[string]any{"path": *info.Nested, "score_mode": "none", "query": clause}}
	}
	return clause, negate, nil
}

func sortClauses(dv dataView, sorts []model.KibanaSort) ([]any, error) {
	if len(sorts) == 0 {
		if dv.TimeFieldName != nil {
			return []any{map[string]any{*dv.TimeFieldName: map[string]any{"order": "desc", "unmapped_type": "boolean"}}}, nil
		}
		return []any{"_doc"}, nil
	}
	out := make([]any, 0, len(sorts))
	for _, s := range sorts {
		f, ok := dv.field(s.Field)
		if !ok || !f.Aggregatable {
			return nil, queryErr("backend.studio.adapter.kibanaUnsortable.body", map[string]string{"field": s.Field})
		}
		if s.Order != "asc" && s.Order != "desc" {
			return nil, queryErr("backend.studio.adapter.kibanaSortOrder.body", map[string]string{"order": s.Order})
		}
		out = append(out, map[string]any{s.Field: map[string]any{"order": s.Order, "unmapped_type": "boolean"}})
	}
	return out, nil
}

// buildQuery is the bool query shared by hits, count, histogram and suggestions.
func buildQuery(dv dataView, req model.KibanaDiscoverRequest) (map[string]any, error) {
	var text map[string]any
	switch req.Language {
	case "", "kuery":
		var err error
		if text, err = kql.Compile(req.Query, kqlFields(dv), req.TimeZone); err != nil {
			var se *kql.SyntaxError
			if errors.As(err, &se) {
				return nil, adapters.New(adapters.CodeQuery, se.Error(), err)
			}
			return nil, err
		}
	case "lucene":
		text = kql.LuceneQuery(req.Query, req.TimeZone)
	default:
		return nil, queryErr("backend.studio.adapter.kibanaLanguage.body", map[string]string{"language": req.Language})
	}
	filter := []any{text}
	if t := timeClause(dv, req); t != nil {
		filter = append(filter, t)
	}
	var mustNot []any
	for _, f := range req.Filters {
		clause, negate, err := filterClause(dv, f)
		if err != nil {
			return nil, err
		}
		switch {
		case clause == nil:
		case negate:
			mustNot = append(mustNot, clause)
		default:
			filter = append(filter, clause)
		}
	}
	b := map[string]any{"filter": filter}
	if len(mustNot) > 0 {
		b["must_not"] = mustNot
	}
	return map[string]any{"bool": b}, nil
}

// buildBody is the full search body for a hits request, minus size and from.
func buildBody(dv dataView, req model.KibanaDiscoverRequest) (map[string]any, error) {
	q, err := buildQuery(dv, req)
	if err != nil {
		return nil, err
	}
	sort, err := sortClauses(dv, req.Sort)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"query": q, "sort": sort, "track_total_hits": true, "timeout": searchTimeout}
	addViewParts(body, dv)
	return body, nil
}

// addViewParts adds what the data view itself contributes to any search against it.
func addViewParts(body map[string]any, dv dataView) {
	if len(dv.runtime) > 0 {
		body["runtime_mappings"] = dv.runtime
		body["fields"] = runtimeNames(dv)
	}
	if len(dv.excludes) > 0 {
		body["_source"] = map[string]any{"excludes": dv.excludes}
	}
}

func runtimeNames(dv dataView) []string {
	names := make([]string, 0, len(dv.runtime))
	for n := range dv.runtime {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// hitBody renders one hit as the document page shows it: _id and _index, the source, and the
// runtime fields. Elasticsearch forbids those two keys inside _source, so nothing collides.
func hitBody(h searchHit, runtime []string) (string, error) {
	src := bytes.TrimSpace(h.Source)
	if len(src) == 0 || string(src) == "null" {
		src = []byte("{}")
	}
	if len(runtime) > 0 {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(src, &m); err != nil {
			return "", adapters.New(adapters.CodeContract, "unexpected _source: "+err.Error(), err)
		}
		for _, n := range runtime {
			vals := h.Fields[n]
			switch len(vals) {
			case 0:
			case 1:
				m[n] = vals[0]
			default:
				joined, _ := json.Marshal(vals)
				m[n] = joined
			}
		}
		src, _ = json.Marshal(m)
	}
	id, _ := json.Marshal(h.ID)
	idx, _ := json.Marshal(h.Index)
	var b bytes.Buffer
	b.WriteString(`{"_id":`)
	b.Write(id)
	b.WriteString(`,"_index":`)
	b.Write(idx)
	if inner := bytes.TrimSpace(src[1 : len(src)-1]); len(inner) > 0 {
		b.WriteByte(',')
		b.Write(inner)
	}
	b.WriteByte('}')
	return b.String(), nil
}

// pushHits adds hits to a page until the cap, returning whether it stopped early.
func pushHits(hits []searchHit, runtime []string, limit page.ResultCap) (*page.DocumentPageBuilder, bool, error) {
	builder := page.NewDocumentPageBuilder(false)
	for _, h := range hits {
		if limit.Reached(builder.RowCount(), builder.Bytes()) {
			return builder, true, nil
		}
		body, err := hitBody(h, runtime)
		if err != nil {
			return nil, false, err
		}
		builder.Push(h.ID, body)
	}
	return builder, false, nil
}

func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	h, err := a.requireHandle()
	if err != nil {
		return nil, err
	}
	var stmts []string
	for _, s := range req.Statements {
		if strings.TrimSpace(s) != "" {
			stmts = append(stmts, s)
		}
	}
	if len(stmts) != 1 {
		return nil, queryErr("backend.studio.adapter.kibanaOneStatement.body", nil)
	}
	var dr model.KibanaDiscoverRequest
	dec := json.NewDecoder(strings.NewReader(stmts[0]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dr); err != nil {
		return nil, queryErr("backend.studio.adapter.kibanaBadRequest.body", map[string]string{"detail": err.Error()})
	}
	if dr.DataView == "" && len(req.Path.Segments) == 1 {
		dr.DataView = req.Path.Segments[0].Name
	}
	if dr.DataView == "" {
		return nil, queryErr("backend.studio.adapter.kibanaNoDataView.body", nil)
	}
	op.SetCommand(stmts[0])
	ctx, done := a.track(ctx, op.OpID)
	defer done()
	dv, err := a.dataView(ctx, h, dr.DataView)
	if err != nil {
		return nil, err
	}
	limit := adapters.ConsoleCapFor(req)
	size := dr.Size
	if size <= 0 {
		size = defaultSampleSize
	}
	size = min(size, limit.Rows, maxResultWindow)
	body, err := buildBody(dv, dr)
	if err != nil {
		return nil, err
	}
	body["size"] = size + 1
	var cols []model.KibanaField
	if len(dr.Columns) > 0 {
		if cols, err = tableColumns(dv, dr.Columns); err != nil {
			return nil, err
		}
		columnsBody(body, cols)
	}
	res, err := h.search(ctx, dv.Title, body)
	if err != nil {
		return nil, err
	}
	hits := res.Hits.Hits
	truncated := len(hits) > size
	if truncated {
		hits = hits[:size]
	}
	if cols != nil {
		builder, capped, err := columnPage(cols, hits, limit)
		if err != nil {
			return nil, err
		}
		op.SetRows(builder.RowCount())
		return []page.Page{builder.Finish(page.CappedPosition(builder.RowCount(), truncated || capped))}, nil
	}
	builder, capped, err := pushHits(hits, runtimeNames(dv), limit)
	if err != nil {
		return nil, err
	}
	op.SetRows(builder.RowCount())
	return []page.Page{builder.Finish(page.CappedPosition(builder.RowCount(), truncated || capped))}, nil
}

// readTarget resolves a data view path and the request it implies for Read and Count.
func (a *Adapter) readTarget(ctx context.Context, h *Handle, path model.NodePath, op string, filter *string) (dataView, model.KibanaDiscoverRequest, error) {
	segs, err := adapters.RequirePath(path, op, adapters.Seg("data view", "collection"))
	if err != nil {
		return dataView{}, model.KibanaDiscoverRequest{}, err
	}
	dv, err := a.dataView(ctx, h, segs[0].Name)
	if err != nil {
		return dataView{}, model.KibanaDiscoverRequest{}, err
	}
	req := model.KibanaDiscoverRequest{DataView: dv.ID, Language: "kuery"}
	if filter != nil {
		req.Query = *filter
	}
	return dv, req, nil
}

func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	if err := adapters.RejectConditions("kibana", req.Conditions); err != nil {
		return nil, err
	}
	h, err := a.requireHandle()
	if err != nil {
		return nil, err
	}
	ctx, done := a.track(ctx, op.OpID)
	defer done()
	dv, dr, err := a.readTarget(ctx, h, req.Path, "read", req.Filter)
	if err != nil {
		return nil, err
	}
	if req.Sort != nil {
		if req.Sort.Kind != "structured" {
			return nil, queryErr("backend.studio.adapter.kibanaTextSort.body", nil)
		}
		for _, t := range req.Sort.Terms {
			dr.Sort = append(dr.Sort, model.KibanaSort{Field: t.Column, Order: t.Direction})
		}
	}
	if req.Cursor.Mode != "" && req.Cursor.Mode != "offset" {
		return nil, queryErr("backend.studio.adapter.kibanaOffsetOnly.body", nil)
	}
	from, err := adapters.SafeInt(req.Cursor.Offset, "offset")
	if err != nil {
		return nil, err
	}
	body, err := buildBody(dv, dr)
	if err != nil {
		return nil, err
	}
	// The window ends at 10,000 hits; one extra hit tells whether the window or the data ended.
	want := min(req.PageSize, max(maxResultWindow-from, 0))
	body["from"], body["size"] = from, want+1
	if want == 0 {
		body["size"] = 0
	}
	op.SetCommand(searchCommand(dv.Title, body))
	res, err := h.search(ctx, dv.Title, body)
	if err != nil {
		return nil, err
	}
	hits := res.Hits.Hits
	more := len(hits) > want
	if more {
		hits = hits[:want]
	}
	total, _ := res.total()
	windowEnd := from+want >= maxResultWindow && total > int64(maxResultWindow)
	builder, capped, err := pushHits(hits, runtimeNames(dv), adapters.DefaultConsoleCap)
	if err != nil {
		return nil, err
	}
	op.SetRows(builder.RowCount())
	offset := from
	return builder.Finish(page.PagePosition{
		Offset: &offset, PageSize: req.PageSize, HasMore: more && !windowEnd,
		Strategy: "offset", Truncated: capped || windowEnd && builder.RowCount() == want,
	}), nil
}

func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	h, err := a.requireHandle()
	if err != nil {
		return adapters.CountResult{}, err
	}
	ctx, done := a.track(ctx, op.OpID)
	defer done()
	dv, dr, err := a.readTarget(ctx, h, req.Path, "count", req.Filter)
	if err != nil {
		return adapters.CountResult{}, err
	}
	q, err := buildQuery(dv, dr)
	if err != nil {
		return adapters.CountResult{}, err
	}
	body := map[string]any{"query": q, "size": 0, "track_total_hits": true, "timeout": searchTimeout}
	addViewParts(body, dv)
	delete(body, "fields")
	op.SetCommand(searchCommand(dv.Title, body))
	res, err := h.search(ctx, dv.Title, body)
	if err != nil {
		return adapters.CountResult{}, err
	}
	n, rel := res.total()
	return adapters.CountResult{Value: n, Exact: rel == "eq"}, nil
}

// searchCommand is the op log text of a search: the target index and the exact body.
func searchCommand(index string, body map[string]any) string {
	b, _ := json.Marshal(body)
	return "POST " + index + "/_search " + string(b)
}

// ClassifyStatement: Discover only reads.
func (a *Adapter) ClassifyStatement(context.Context, string) (adapters.OpClass, error) {
	return adapters.ClassRead, nil
}
