package kibana_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func window() *model.KibanaTimeRange {
	return &model.KibanaTimeRange{From: testsupport.KibanaWindowFrom, To: testsupport.KibanaWindowTo}
}

func statement(t *testing.T, r model.KibanaDiscoverRequest) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func run(a adapters.Adapter, cfg model.ResolvedConnectionConfig, statements ...string) (page.DocumentPage, error) {
	pages, err := a.Execute(context.Background(), model.ConsoleRequest{Path: rootPath(cfg), Statements: statements}, opCtx("execute"))
	if err != nil {
		return page.DocumentPage{}, err
	}
	return pages[0].(page.DocumentPage), nil
}

func hits(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig, r model.KibanaDiscoverRequest) page.DocumentPage {
	t.Helper()
	if r.DataView == "" {
		r.DataView = testsupport.KibanaLogsView
	}
	if r.TimeRange == nil {
		r.TimeRange = window()
	}
	p, err := run(a, cfg, statement(t, r))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return p
}

func body(t *testing.T, p page.DocumentPage, row int) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(*testsupport.DocBodyAt(t, p, row)), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestKibanaSearch(t *testing.T) {
	f := testsupport.StartKibana(t)
	cfg := f.Config
	a := connected(t, cfg)

	counts := []struct {
		name string
		req  model.KibanaDiscoverRequest
		want int
	}{
		{"all", model.KibanaDiscoverRequest{Size: 1000}, testsupport.KibanaDocCount},
		{"kql term", model.KibanaDiscoverRequest{Query: "level:error"}, 50},
		{"kql list", model.KibanaDiscoverRequest{Query: "status:(500 or 503)"}, 80},
		{"kql range", model.KibanaDiscoverRequest{Query: "status >= 500 and not level:error"}, 60},
		{"kql wildcard", model.KibanaDiscoverRequest{Query: "level:err*"}, 50},
		{"kql exists", model.KibanaDiscoverRequest{Query: "tags:*"}, testsupport.KibanaDocCount},
		{"kql nested", model.KibanaDiscoverRequest{Query: "user:{ name:bob }"}, 67},
		{"kql dotted nested", model.KibanaDiscoverRequest{Query: "user.name:carol"}, 66},
		{"kql ip", model.KibanaDiscoverRequest{Query: "client.ip:10.0.0.1"}, 4},
		{"kql runtime field", model.KibanaDiscoverRequest{Query: "status_class:error"}, 80},
		{"kql free text", model.KibanaDiscoverRequest{Query: `"worker-3"`}, 29},
		{"lucene", model.KibanaDiscoverRequest{Language: "lucene", Query: "level:error AND status:[500 TO 503]"}, 20},
		{"lucene or", model.KibanaDiscoverRequest{Language: "lucene", Query: "level:error OR level:warn"}, 100},
		{"narrow time", model.KibanaDiscoverRequest{TimeRange: &model.KibanaTimeRange{From: "2026-01-01T00:00:00Z", To: "2026-01-01T11:59:59Z"}}, 100},
		{"filter is_not", model.KibanaDiscoverRequest{Filters: []model.KibanaFilter{{Field: "level", Operator: "is_not", Values: []string{"error"}}}}, 150},
		{"filter one of", model.KibanaDiscoverRequest{Filters: []model.KibanaFilter{{Field: "status", Operator: "is_one_of", Values: []string{"200", "404"}}}}, 80},
		{"filter between", model.KibanaDiscoverRequest{Filters: []model.KibanaFilter{{Field: "status", Operator: "between", Values: []string{"500", "503"}}}}, 40},
		{"filter exists", model.KibanaDiscoverRequest{Filters: []model.KibanaFilter{{Field: "ok", Operator: "exists"}}}, testsupport.KibanaDocCount},
		{"filter disabled", model.KibanaDiscoverRequest{Filters: []model.KibanaFilter{{Field: "level", Operator: "is", Values: []string{"error"}, Disabled: true}}}, testsupport.KibanaDocCount},
		{"filter plus query", model.KibanaDiscoverRequest{Query: "ok:true", Filters: []model.KibanaFilter{{Field: "level", Operator: "is", Values: []string{"error"}}}}, 50},
	}
	for _, c := range counts {
		t.Run("hits "+c.name, func(t *testing.T) {
			c.req.Size = max(c.req.Size, 1000)
			if p := hits(t, a, cfg, c.req); p.RowCount != c.want {
				t.Errorf("rows = %d, want %d", p.RowCount, c.want)
			}
		})
	}

	t.Run("default sort is time descending", func(t *testing.T) {
		p := hits(t, a, cfg, model.KibanaDiscoverRequest{Size: 5})
		if id := *testsupport.DocIDAt(t, p, 0); id != "doc-199" {
			t.Errorf("first id = %s, want doc-199", id)
		}
	})

	t.Run("explicit sort", func(t *testing.T) {
		p := hits(t, a, cfg, model.KibanaDiscoverRequest{Size: 3, Sort: []model.KibanaSort{{Field: "status", Order: "desc"}}})
		if got := body(t, p, 0)["status"]; got != float64(503) {
			t.Errorf("status = %v, want 503", got)
		}
	})

	t.Run("body carries id, index, source and runtime fields", func(t *testing.T) {
		p := hits(t, a, cfg, model.KibanaDiscoverRequest{Size: 1, Query: "status:503"})
		b := body(t, p, 0)
		if b["_index"] != "kira-logs" || b["_id"] == nil || b["message"] == nil || b["status_class"] != "error" {
			t.Errorf("body = %v", b)
		}
	})

	t.Run("size over the page truncates", func(t *testing.T) {
		p := hits(t, a, cfg, model.KibanaDiscoverRequest{Size: 10})
		if p.RowCount != 10 || !p.Position.Truncated {
			t.Errorf("rows = %d truncated = %v, want 10 true", p.RowCount, p.Position.Truncated)
		}
	})

	t.Run("data view path supplies the view", func(t *testing.T) {
		stmt := statement(t, model.KibanaDiscoverRequest{Query: "name:item-3", Size: 50})
		pages, err := a.Execute(context.Background(), model.ConsoleRequest{Path: viewPath(cfg, testsupport.KibanaNoTimeView), Statements: []string{stmt}}, opCtx("execute"))
		if err != nil {
			t.Fatal(err)
		}
		if n := pages[0].Rows(); n != 1 {
			t.Errorf("rows = %d, want 1", n)
		}
	})

	t.Run("view without a time field ignores the range", func(t *testing.T) {
		p := hits(t, a, cfg, model.KibanaDiscoverRequest{DataView: testsupport.KibanaNoTimeView, Size: 100})
		if p.RowCount != 20 {
			t.Errorf("rows = %d, want 20", p.RowCount)
		}
	})

	t.Run("titled view searches its pattern", func(t *testing.T) {
		p := hits(t, a, cfg, model.KibanaDiscoverRequest{DataView: testsupport.KibanaTitledView, Size: 1000})
		if p.RowCount != testsupport.KibanaDocCount {
			t.Errorf("rows = %d, want %d", p.RowCount, testsupport.KibanaDocCount)
		}
	})

	rejects := []struct {
		name  string
		stmts []string
		req   *model.KibanaDiscoverRequest
		code  adapters.ErrorCode
	}{
		{"syntax error", nil, &model.KibanaDiscoverRequest{Query: "level:(error"}, adapters.CodeQuery},
		{"unknown filter field", nil, &model.KibanaDiscoverRequest{Filters: []model.KibanaFilter{{Field: "nope", Operator: "exists"}}}, adapters.CodeQuery},
		{"unsortable field", nil, &model.KibanaDiscoverRequest{Sort: []model.KibanaSort{{Field: "message", Order: "asc"}}}, adapters.CodeQuery},
		{"bad language", nil, &model.KibanaDiscoverRequest{Language: "sql"}, adapters.CodeQuery},
		{"unknown data view", nil, &model.KibanaDiscoverRequest{DataView: "no-such-view"}, adapters.CodeNotFound},
		{"not json", []string{"level:error"}, nil, adapters.CodeQuery},
		{"unknown json key", []string{`{"dataView":"kira-logs","bogus":1}`}, nil, adapters.CodeQuery},
		{"two statements", []string{`{"dataView":"kira-logs"}`, `{"dataView":"kira-logs"}`}, nil, adapters.CodeQuery},
		{"no statement", []string{"  "}, nil, adapters.CodeQuery},
		{"no data view", []string{`{}`}, nil, adapters.CodeQuery},
		{"lucene syntax error", nil, &model.KibanaDiscoverRequest{Language: "lucene", Query: "level:["}, adapters.CodeQuery},
	}
	for _, c := range rejects {
		t.Run("rejects "+c.name, func(t *testing.T) {
			stmts := c.stmts
			if c.req != nil {
				if c.req.DataView == "" {
					c.req.DataView = testsupport.KibanaLogsView
				}
				stmts = []string{statement(t, *c.req)}
			}
			_, err := run(a, cfg, stmts...)
			wantCode(t, err, c.code)
		})
	}

	t.Run("syntax error names the position", func(t *testing.T) {
		_, err := run(a, cfg, statement(t, model.KibanaDiscoverRequest{DataView: testsupport.KibanaLogsView, Query: "level:(error"}))
		if err == nil || !strings.Contains(err.Error(), "position 13") {
			t.Errorf("err = %v, want position 13", err)
		}
	})

	t.Run("classifies as read", func(t *testing.T) {
		c, err := a.(adapters.StatementClassifier).ClassifyStatement(context.Background(), `{"query":"x"}`)
		if err != nil || c != adapters.ClassRead {
			t.Errorf("class = %v err = %v", c, err)
		}
	})

	t.Run("read pages by offset", func(t *testing.T) {
		read := func(offset int, filter *string) page.DocumentPage {
			p, err := a.Read(context.Background(), adapters.ReadRequest{
				Path: viewPath(cfg, testsupport.KibanaLogsView), PageSize: 80, Filter: filter,
				Cursor: model.PageCursor{Mode: "offset", Offset: offset},
			}, opCtx("read"))
			if err != nil {
				t.Fatal(err)
			}
			return p.(page.DocumentPage)
		}
		first := read(0, nil)
		if first.RowCount != 80 || !first.Position.HasMore {
			t.Errorf("first: rows %d more %v", first.RowCount, first.Position.HasMore)
		}
		last := read(160, nil)
		if last.RowCount != 40 || last.Position.HasMore {
			t.Errorf("last: rows %d more %v", last.RowCount, last.Position.HasMore)
		}
		if *testsupport.DocIDAt(t, first, 0) == *testsupport.DocIDAt(t, last, 0) {
			t.Error("pages overlap")
		}
		q := "level:error"
		if n := read(0, &q).RowCount; n != 50 {
			t.Errorf("filtered rows = %d, want 50", n)
		}
		if n := read(10000, nil).RowCount; n != 0 {
			t.Errorf("rows past the window = %d, want 0", n)
		}
	})

	t.Run("read rejects a keyset cursor", func(t *testing.T) {
		_, err := a.Read(context.Background(), adapters.ReadRequest{
			Path: viewPath(cfg, testsupport.KibanaLogsView), PageSize: 10, Cursor: model.PageCursor{Mode: "after", Token: "x"},
		}, opCtx("read"))
		wantCode(t, err, adapters.CodeQuery)
	})

	t.Run("count is exact", func(t *testing.T) {
		q := "level:warn"
		for filter, want := range map[*string]int64{nil: testsupport.KibanaDocCount, &q: 50} {
			res, err := a.Count(context.Background(), adapters.CountRequest{Path: viewPath(cfg, testsupport.KibanaLogsView), Filter: filter}, opCtx("count"))
			if err != nil || res.Value != want || !res.Exact {
				t.Errorf("count = %+v err = %v, want %d exact", res, err, want)
			}
		}
	})
}

func TestKibanaColumns(t *testing.T) {
	f := testsupport.StartKibana(t)
	cfg := f.Config
	a := connected(t, cfg)

	table := func(t *testing.T, r model.KibanaDiscoverRequest, cap page.ResultCap) page.TabularPage {
		t.Helper()
		if r.DataView == "" {
			r.DataView = testsupport.KibanaLogsView
		}
		if r.TimeRange == nil {
			r.TimeRange = window()
		}
		pages, err := a.Execute(context.Background(), model.ConsoleRequest{Path: rootPath(cfg), Statements: []string{statement(t, r)}, Cap: cap}, opCtx("execute"))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		tp, ok := pages[0].(page.TabularPage)
		if !ok {
			t.Fatalf("page is %T, want a table", pages[0])
		}
		return tp
	}
	names := func(p page.TabularPage) []string {
		out := make([]string, len(p.Columns))
		for i, c := range p.Columns {
			out[i] = c.Name
		}
		return out
	}
	cell := func(t *testing.T, p page.TabularPage, col string, row int) *string {
		t.Helper()
		for i, c := range p.Columns {
			if c.Name == col {
				return testsupport.CellAt(t, p, i, row)
			}
		}
		t.Fatalf("no column %s in %v", col, names(p))
		return nil
	}
	req := func(cols ...string) model.KibanaDiscoverRequest {
		return model.KibanaDiscoverRequest{Size: 5, Query: "status:503", Columns: cols}
	}

	t.Run("time field comes first", func(t *testing.T) {
		p := table(t, req("level", "status"), page.ResultCap{})
		if got := strings.Join(names(p), ","); got != "@timestamp,level,status" {
			t.Errorf("columns = %s", got)
		}
		if p.RowCount != 5 || !p.Position.Truncated {
			t.Errorf("rows = %d truncated = %v", p.RowCount, p.Position.Truncated)
		}
	})

	t.Run("an explicit time column keeps its place", func(t *testing.T) {
		p := table(t, req("level", "@timestamp"), page.ResultCap{})
		if got := strings.Join(names(p), ","); got != "level,@timestamp" {
			t.Errorf("columns = %s", got)
		}
	})

	t.Run("type classes", func(t *testing.T) {
		p := table(t, req("level", "status", "ok"), page.ResultCap{})
		want := map[string]page.TypeClass{"@timestamp": page.TypeClassTemporal, "level": page.TypeClassText, "status": page.TypeClassNumber, "ok": page.TypeClassBoolean}
		for _, c := range p.Columns {
			if want[c.Name] != c.TypeClass {
				t.Errorf("%s class = %s, want %s", c.Name, c.TypeClass, want[c.Name])
			}
		}
		if v := cell(t, p, "status", 0); v == nil || *v != "503" {
			t.Errorf("status = %v", v)
		}
	})

	t.Run("runtime, multi-field and nested values", func(t *testing.T) {
		p := table(t, req("status_class", "message.keyword", "user.name"), page.ResultCap{})
		if v := cell(t, p, "status_class", 0); v == nil || *v != "error" {
			t.Errorf("status_class = %v", v)
		}
		if v := cell(t, p, "message.keyword", 0); v == nil || !strings.HasPrefix(*v, "request ") {
			t.Errorf("message.keyword = %v", v)
		}
		if v := cell(t, p, "user.name", 0); v == nil || (*v != "alice" && *v != "bob" && *v != "carol") {
			t.Errorf("user.name = %v", v)
		}
	})

	t.Run("several values render as a JSON array", func(t *testing.T) {
		r := model.KibanaDiscoverRequest{Size: 5, Query: "tags:a and tags:b", Columns: []string{"tags"}}
		p := table(t, r, page.ResultCap{})
		if v := cell(t, p, "tags", 0); v == nil || *v != `["a","b"]` {
			t.Errorf("tags = %v", v)
		}
	})

	t.Run("a missing value is NULL", func(t *testing.T) {
		r := model.KibanaDiscoverRequest{DataView: testsupport.KibanaNoTimeView, Size: 5, Columns: []string{"name", "count"}}
		p := table(t, r, page.ResultCap{})
		if got := strings.Join(names(p), ","); got != "name,count" {
			t.Errorf("columns = %s", got)
		}
		r.Columns = []string{"name"}
		r.Filters = []model.KibanaFilter{{Field: "name", Operator: "is", Values: []string{"item-3"}}}
		if p = table(t, r, page.ResultCap{}); p.RowCount != 1 {
			t.Errorf("rows = %d", p.RowCount)
		}
	})

	t.Run("rejects unknown and duplicate columns", func(t *testing.T) {
		for _, cols := range [][]string{{"nope"}, {"level", "level"}} {
			_, err := run(a, cfg, statement(t, model.KibanaDiscoverRequest{DataView: testsupport.KibanaLogsView, Columns: cols}))
			wantCode(t, err, adapters.CodeQuery)
		}
	})

	t.Run("the byte cap stops early", func(t *testing.T) {
		p := table(t, model.KibanaDiscoverRequest{Size: 100, Columns: []string{"message"}}, page.ResultCap{Bytes: 200})
		if p.RowCount >= 100 || p.RowCount == 0 || !p.Position.Truncated {
			t.Errorf("rows = %d truncated = %v", p.RowCount, p.Position.Truncated)
		}
	})

	t.Run("no columns keeps the document page", func(t *testing.T) {
		if p := hits(t, a, cfg, model.KibanaDiscoverRequest{Size: 3}); p.RowCount != 3 {
			t.Errorf("rows = %d", p.RowCount)
		}
	})
}
