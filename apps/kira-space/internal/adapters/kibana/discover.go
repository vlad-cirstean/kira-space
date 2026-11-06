package kibana

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

var _ adapters.Discoverer = (*Adapter)(nil)

func (a *Adapter) DataViews(ctx context.Context, op *adapters.OpCtx) ([]model.KibanaDataView, error) {
	h, err := a.requireHandle()
	if err != nil {
		return nil, err
	}
	op.SetCommand("GET /api/data_views")
	ctx, done := a.track(ctx, op.OpID)
	defer done()
	return h.listDataViews(ctx)
}

func (a *Adapter) DataView(ctx context.Context, id string, op *adapters.OpCtx) (model.KibanaDataView, error) {
	h, err := a.requireHandle()
	if err != nil {
		return model.KibanaDataView{}, err
	}
	op.SetCommand("GET /api/data_views/data_view/" + id)
	ctx, done := a.track(ctx, op.OpID)
	defer done()
	dv, err := h.fetchDataView(ctx, id)
	if err != nil {
		return model.KibanaDataView{}, err
	}
	a.views.put(dv)
	return dv.KibanaDataView, nil
}

// absoluteMs parses an ISO-8601 instant; histogram bounds cannot be date math.
func absoluteMs(s string) (int64, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli(), true
		}
	}
	return 0, false
}

func (a *Adapter) Histogram(ctx context.Context, req model.KibanaHistogramRequest, op *adapters.OpCtx) (model.KibanaHistogram, error) {
	h, err := a.requireHandle()
	if err != nil {
		return model.KibanaHistogram{}, err
	}
	ctx, done := a.track(ctx, op.OpID)
	defer done()
	dr := req.Request
	dv, err := a.dataView(ctx, h, dr.DataView)
	if err != nil {
		return model.KibanaHistogram{}, err
	}
	q, err := buildQuery(dv, dr)
	if err != nil {
		return model.KibanaHistogram{}, err
	}
	body := map[string]any{"query": q, "size": 0, "track_total_hits": true, "timeout": searchTimeout}
	addViewParts(body, dv)
	delete(body, "fields")
	var fromMs, toMs int64
	var key, value string
	if dv.TimeFieldName != nil {
		var okFrom, okTo bool
		if dr.TimeRange != nil {
			fromMs, okFrom = absoluteMs(dr.TimeRange.From)
			toMs, okTo = absoluteMs(dr.TimeRange.To)
		}
		if !okFrom || !okTo || toMs < fromMs {
			return model.KibanaHistogram{}, queryErr("backend.studio.adapter.kibanaAbsoluteRange.body", nil)
		}
		key, value = pickInterval(fromMs, toMs, req.Interval)
		body["aggs"] = map[string]any{"h": map[string]any{"date_histogram": map[string]any{
			"field": *dv.TimeFieldName, key: value, "time_zone": zoneOrUTC(dr.TimeZone),
			"min_doc_count": 0, "extended_bounds": map[string]any{"min": fromMs, "max": toMs},
		}}}
	}
	op.SetCommand(searchCommand(dv.Title, body))
	res, err := h.search(ctx, dv.Title, body)
	if err != nil {
		return model.KibanaHistogram{}, err
	}
	total, relation := res.total()
	out := model.KibanaHistogram{Total: total, Relation: relation, Interval: value, Buckets: []model.KibanaHistogramBucket{}}
	if raw, ok := res.Aggregations["h"]; ok {
		var agg struct {
			Buckets []struct {
				Key      int64 `json:"key"`
				DocCount int64 `json:"doc_count"`
			} `json:"buckets"`
		}
		if err := json.Unmarshal(raw, &agg); err != nil {
			return model.KibanaHistogram{}, adapters.New(adapters.CodeContract, "unexpected histogram aggregation: "+err.Error(), err)
		}
		out.Buckets = make([]model.KibanaHistogramBucket, len(agg.Buckets))
		for i, b := range agg.Buckets {
			out.Buckets[i] = model.KibanaHistogramBucket{Key: b.Key, Count: b.DocCount}
		}
	}
	return out, nil
}

func zoneOrUTC(tz string) string {
	if tz == "" {
		return "UTC"
	}
	return tz
}

const (
	suggestTerminateAfter = 100_000
	suggestTimeout        = "1000ms"
	suggestWide           = 100
	suggestNarrow         = 10
)

// suggestField picks the field whose terms are offered: the field itself when aggregatable, else
// its aggregatable keyword sub-field, as Kibana does for a text field.
func suggestField(dv dataView, name string) (model.KibanaField, bool) {
	f, ok := dv.field(name)
	if !ok {
		return f, false
	}
	if !f.Aggregatable {
		for _, c := range dv.Fields {
			if c.MultiParent != nil && *c.MultiParent == name && c.Aggregatable && c.Type == "string" {
				return c, true
			}
		}
		return f, false
	}
	switch f.Type {
	case "string", "ip", "boolean", "number":
		return f, true
	}
	return f, false
}

// luceneRegexEscape escapes what Elasticsearch's regexp include treats as syntax.
func luceneRegexEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`.?+*|{}[]()"\#@&<>~^$`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (a *Adapter) ValueSuggestions(ctx context.Context, req model.KibanaSuggestRequest, op *adapters.OpCtx) (model.KibanaSuggestions, error) {
	h, err := a.requireHandle()
	if err != nil {
		return model.KibanaSuggestions{}, err
	}
	ctx, done := a.track(ctx, op.OpID)
	defer done()
	dv, err := a.dataView(ctx, h, req.Request.DataView)
	if err != nil {
		return model.KibanaSuggestions{}, err
	}
	none := model.KibanaSuggestions{Values: []string{}, Complete: true}
	f, ok := suggestField(dv, req.Field)
	if !ok {
		return none, nil
	}
	if f.Type == "boolean" {
		out := model.KibanaSuggestions{Values: []string{}, Complete: true}
		for _, v := range []string{"true", "false"} {
			if strings.HasPrefix(v, strings.ToLower(req.Prefix)) {
				out.Values = append(out.Values, v)
			}
		}
		return out, nil
	}
	q, err := buildQuery(dv, req.Request)
	if err != nil {
		return model.KibanaSuggestions{}, err
	}
	n := suggestWide
	terms := map[string]any{"field": f.Name, "execution_hint": "map"}
	switch {
	case f.Type == "string" && req.Prefix != "":
		n = suggestNarrow
		terms["include"] = luceneRegexEscape(req.Prefix) + ".*"
	case f.Type == "string":
	default:
		// number and ip have no prefix include; the prefix is applied below, so fetch the wide set.
	}
	terms["size"], terms["shard_size"] = n, n
	body := map[string]any{
		"query": q, "size": 0, "terminate_after": suggestTerminateAfter, "timeout": suggestTimeout,
		"aggs": map[string]any{"s": map[string]any{"terms": terms}},
	}
	addViewParts(body, dv)
	delete(body, "fields")
	op.SetCommand(searchCommand(dv.Title, body))
	res, err := h.search(ctx, dv.Title, body)
	if err != nil {
		return model.KibanaSuggestions{}, err
	}
	var agg struct {
		SumOther int64 `json:"sum_other_doc_count"`
		Buckets  []struct {
			Key         json.RawMessage `json:"key"`
			KeyAsString string          `json:"key_as_string"`
		} `json:"buckets"`
	}
	if raw, ok := res.Aggregations["s"]; ok {
		if err := json.Unmarshal(raw, &agg); err != nil {
			return model.KibanaSuggestions{}, adapters.New(adapters.CodeContract, "unexpected terms aggregation: "+err.Error(), err)
		}
	}
	out := model.KibanaSuggestions{Values: make([]string, 0, len(agg.Buckets))}
	for _, b := range agg.Buckets {
		v := b.KeyAsString
		if v == "" {
			var s string
			if json.Unmarshal(b.Key, &s) == nil {
				v = s
			} else {
				v = strings.TrimSpace(string(b.Key))
			}
		}
		if f.Type != "string" && !strings.HasPrefix(v, req.Prefix) {
			continue
		}
		out.Values = append(out.Values, v)
	}
	if f.Type != "string" && req.Prefix != "" && len(out.Values) > suggestNarrow {
		out.Values = out.Values[:suggestNarrow]
	}
	out.Complete = len(agg.Buckets) < n && agg.SumOther == 0 && !res.TerminatedEarly && !res.TimedOut
	return out, nil
}
