package kibana

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// detailTTL bounds how long a fetched data view serves searches before its fields are re-read.
const detailTTL = 30 * time.Second

// dataView is a data view plus what a search body needs beyond its fields.
type dataView struct {
	model.KibanaDataView
	runtime  map[string]json.RawMessage // runtime_mappings, in Elasticsearch's own shape
	excludes []string                   // _source.excludes from the view's source filters
	fetched  time.Time
}

// viewCache holds the data view list's names and the fetched details.
type viewCache struct {
	mu      sync.Mutex
	details map[string]dataView
}

func (c *viewCache) get(id string) (dataView, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	dv, ok := c.details[id]
	return dv, ok && time.Since(dv.fetched) < detailTTL
}

func (c *viewCache) put(dv dataView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.details == nil {
		c.details = map[string]dataView{}
	}
	c.details[dv.ID] = dv
}

func (c *viewCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.details = nil
}

type rawField struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	ESTypes      []string `json:"esTypes"`
	Searchable   bool     `json:"searchable"`
	Aggregatable bool     `json:"aggregatable"`
	SubType      *struct {
		Multi  *struct{ Parent string } `json:"multi"`
		Nested *struct{ Path string }   `json:"nested"`
	} `json:"subType"`
	RuntimeField json.RawMessage `json:"runtimeField"`
}

// keptMetaFields are the only underscore fields a user can query or show.
var keptMetaFields = map[string]bool{"_id": true, "_index": true}

func (f rawField) toModel(runtimeNames map[string]json.RawMessage) (model.KibanaField, bool) {
	if strings.HasPrefix(f.Name, "_") && !keptMetaFields[f.Name] {
		return model.KibanaField{}, false
	}
	out := model.KibanaField{
		Name: f.Name, Type: f.Type, ESTypes: f.ESTypes, Searchable: f.Searchable, Aggregatable: f.Aggregatable,
	}
	if out.ESTypes == nil {
		out.ESTypes = []string{}
	}
	if f.SubType != nil {
		if f.SubType.Nested != nil {
			p := f.SubType.Nested.Path
			out.Nested = &p
		}
		if f.SubType.Multi != nil {
			p := f.SubType.Multi.Parent
			out.MultiParent = &p
		}
	}
	_, inRuntime := runtimeNames[f.Name]
	out.Runtime = len(f.RuntimeField) > 0 || inRuntime
	return out, true
}

type viewSummary struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Title         string  `json:"title"`
	TimeFieldName *string `json:"timeFieldName"`
}

func (h *Handle) listDataViews(ctx context.Context) ([]model.KibanaDataView, error) {
	var out struct {
		DataView []viewSummary `json:"data_view"`
	}
	if err := h.getJSON(ctx, "/api/data_views", &out); err != nil {
		return nil, err
	}
	views := make([]model.KibanaDataView, 0, len(out.DataView))
	for _, v := range out.DataView {
		name := v.Name
		if name == "" {
			name = v.Title
		}
		views = append(views, model.KibanaDataView{
			ID: v.ID, Name: name, Title: v.Title, TimeFieldName: blankToNil(v.TimeFieldName), Fields: []model.KibanaField{},
		})
	}
	slices.SortFunc(views, func(a, b model.KibanaDataView) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return views, nil
}

func blankToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

// fetchDataView reads one data view with its fields. A view whose stored field list is empty falls
// back to the field caps route.
func (h *Handle) fetchDataView(ctx context.Context, id string) (dataView, error) {
	var out struct {
		DataView struct {
			ID              string                     `json:"id"`
			Title           string                     `json:"title"`
			Name            string                     `json:"name"`
			TimeFieldName   *string                    `json:"timeFieldName"`
			Fields          map[string]rawField        `json:"fields"`
			RuntimeFieldMap map[string]json.RawMessage `json:"runtimeFieldMap"`
			SourceFilters   []struct {
				Value string `json:"value"`
			} `json:"sourceFilters"`
		} `json:"data_view"`
	}
	if err := h.getJSON(ctx, "/api/data_views/data_view/"+url.PathEscape(id), &out); err != nil {
		return dataView{}, err
	}
	d := out.DataView
	name := d.Name
	if name == "" {
		name = d.Title
	}
	dv := dataView{
		KibanaDataView: model.KibanaDataView{ID: d.ID, Name: name, Title: d.Title, TimeFieldName: blankToNil(d.TimeFieldName)},
		runtime:        d.RuntimeFieldMap,
		fetched:        time.Now(),
	}
	for _, f := range d.SourceFilters {
		dv.excludes = append(dv.excludes, f.Value)
	}
	fields := d.Fields
	if len(fields) == 0 {
		var err error
		if fields, err = h.fieldCaps(ctx, d.Title); err != nil {
			return dataView{}, err
		}
	}
	dv.Fields = make([]model.KibanaField, 0, len(fields))
	for key, f := range fields {
		if f.Name == "" {
			f.Name = key
		}
		if mf, ok := f.toModel(d.RuntimeFieldMap); ok {
			dv.Fields = append(dv.Fields, mf)
		}
	}
	slices.SortFunc(dv.Fields, func(a, b model.KibanaField) int { return strings.Compare(a.Name, b.Name) })
	return dv, nil
}

// fieldCaps is the fallback when a data view stores no fields.
func (h *Handle) fieldCaps(ctx context.Context, pattern string) (map[string]rawField, error) {
	var out struct {
		Fields []rawField `json:"fields"`
	}
	q := url.Values{"pattern": {pattern}, "meta_fields": {"_id", "_index"}}
	if err := h.request(ctx, http.MethodGet, "/internal/data_views/fields?"+q.Encode(), true, true, nil, &out); err != nil {
		return nil, err
	}
	m := make(map[string]rawField, len(out.Fields))
	for _, f := range out.Fields {
		m[f.Name] = f
	}
	return m, nil
}

// dataView returns the cached detail, fetching it when absent or stale.
func (a *Adapter) dataView(ctx context.Context, h *Handle, id string) (dataView, error) {
	if dv, ok := a.views.get(id); ok {
		return dv, nil
	}
	dv, err := h.fetchDataView(ctx, id)
	if err != nil {
		return dataView{}, err
	}
	a.views.put(dv)
	return dv, nil
}

// treeNodes names the views for the tree: display name, the title as detail when it differs, and
// the id in brackets when two views share a name. The path segment is the id, which is stable.
func treeNodes(views []model.KibanaDataView) []model.TreeNode {
	seen := map[string]int{}
	for _, v := range views {
		seen[strings.ToLower(v.Name)]++
	}
	nodes := make([]model.TreeNode, 0, len(views))
	for _, v := range views {
		name := v.Name
		if seen[strings.ToLower(name)] > 1 {
			name += " [" + v.ID + "]"
		}
		node := model.TreeNode{
			Kind: "collection", Name: name,
			Path: model.EncodePath([]model.PathSegment{{Kind: "collection", Name: v.ID}}),
		}
		if v.Title != v.Name {
			title := v.Title
			node.Detail = &title
		}
		nodes = append(nodes, node)
	}
	return nodes
}
