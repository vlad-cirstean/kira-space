// Package kibana is the adapter for Kibana's HTTP API: data views as tree nodes, Discover-style
// KQL and Lucene search, a hit histogram and value suggestions. It never talks to Elasticsearch
// directly and is read-only.
package kibana

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func init() {
	adapters.Register("kibana", func(deps adapters.Deps) (adapters.Adapter, error) {
		return &Adapter{deps: deps}, nil
	})
}

// minMajor and minMinor are the oldest Kibana this adapter supports.
const (
	minMajor = 8
	minMinor = 19
)

type connState struct {
	handle *Handle
}

// running is one in-flight request's cancel hook, so Cancel can stop it.
type running struct{ cancel context.CancelFunc }

// Adapter is the Kibana adapter.
type Adapter struct {
	deps    adapters.Deps
	state   adapters.Guarded[connState]
	tracker adapters.QueryTracker[*running]
	views   viewCache
}

func (a *Adapter) Kind() string        { return "kibana" }
func (a *Adapter) Caps() adapters.Caps { return caps }

func (a *Adapter) requireHandle() (*Handle, error) {
	h := a.state.Load().handle
	if h == nil {
		return nil, adapters.Contract("kibana adapter used before Connect", nil)
	}
	return h, nil
}

// track derives a cancellable ctx for one request and registers it under the op id.
func (a *Adapter) track(ctx context.Context, opID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	release := a.tracker.TrackerFor(opID)(&running{cancel: cancel})
	return ctx, func() {
		release()
		cancel()
	}
}

func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	h, err := OpenClient(cfg)
	if err != nil {
		return adapters.ConnectInfo{}, err
	}
	a.state.Update(func(s *connState) { s.handle = h })
	fail := func(err error) (adapters.ConnectInfo, error) {
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, err
	}

	op.SetCommand("GET /api/status")
	var status struct {
		Version struct {
			Number string `json:"number"`
		} `json:"version"`
	}
	if err := h.request(ctx, http.MethodGet, "/api/status", false, false, nil, &status); err != nil {
		return fail(err)
	}
	// A low-privilege user on Kibana 9 gets no version here; the floor is then unchecked.
	version := status.Version.Number
	if version != "" && !atLeast(version, minMajor, minMinor) {
		return fail(adapters.T(adapters.CodeConnect, "backend.studio.adapter.kibanaMinVersion.body",
			map[string]string{"version": version}, nil))
	}
	if h.Space != "" {
		op.SetCommand("GET /api/spaces/space/" + h.Space)
		if err := h.request(ctx, http.MethodGet, "/api/spaces/space/"+url.PathEscape(h.Space), false, false, nil, nil); err != nil {
			if code, _ := adapters.CodeOf(err); code == adapters.CodeNotFound {
				err = adapters.T(adapters.CodeNotFound, "backend.studio.adapter.kibanaSpaceMissing.body",
					map[string]string{"space": h.Space}, err)
			}
			return fail(err)
		}
	}
	// /api/status answers anyone; listing data views is what proves the credentials.
	op.SetCommand("GET /api/data_views")
	if _, err := h.listDataViews(ctx); err != nil {
		return fail(err)
	}

	info := adapters.ConnectInfo{ServerVersion: "Kibana", Details: map[string]string{"url": h.Base}}
	if version != "" {
		info.ServerVersion = "Kibana " + version
	}
	if h.Space != "" {
		info.Details["space"] = h.Space
	}
	return info, nil
}

// atLeast reports whether version ("9.5.3", "8.19.0-SNAPSHOT") is at least major.minor.
func atLeast(version string, major, minor int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return maj > major || (maj == major && min >= minor)
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	for _, opID := range a.tracker.Snapshot() {
		_, _ = a.Cancel(ctx, opID)
	}
	a.tracker.Drain(ctx)
	if h := a.state.Load().handle; h != nil {
		h.Client.CloseIdleConnections()
	}
	a.state.Update(func(s *connState) { s.handle = nil })
	a.views.clear()
	return nil
}

func (a *Adapter) Cancel(_ context.Context, opID string) (bool, error) {
	r, ok := a.tracker.PopRunning(opID)
	if !ok {
		return false, nil
	}
	r.cancel()
	return true, nil
}

// Children lists data views at the root; a data view is a leaf.
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	h, err := a.requireHandle()
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	switch len(path.Segments) {
	case 0:
		op.SetCommand("GET /api/data_views")
		ctx, done := a.track(ctx, op.OpID)
		defer done()
		views, err := h.listDataViews(ctx)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		a.views.clear()
		return adapters.TreeChildren{Nodes: treeNodes(views)}, nil
	case 1:
		return adapters.LeafChildren(path.Segments[0].Kind, map[string]bool{"collection": true})
	}
	return adapters.TreeChildren{}, adapters.UnexpectedPathKind(len(path.Segments)-1, path.Segments[len(path.Segments)-1].Kind)
}

func (a *Adapter) Describe(context.Context, model.NodePath, *adapters.OpCtx) (model.ObjectMeta, error) {
	return model.ObjectMeta{}, adapters.Unsupported("kibana", "describe")
}

func (a *Adapter) SchemaColumns(context.Context, model.NodePath, *adapters.OpCtx) ([]model.RelationColumns, error) {
	return nil, adapters.Unsupported("kibana", "schema columns")
}

func (a *Adapter) Definition(context.Context, model.NodePath, *adapters.OpCtx) (model.ObjectDefinition, error) {
	return model.ObjectDefinition{}, adapters.Unsupported("kibana", "definition")
}

func (a *Adapter) Preview(model.MutationPlan) ([]string, error) {
	return nil, adapters.Unsupported("kibana", "preview")
}

func (a *Adapter) Mutate(context.Context, model.MutationPlan, *adapters.OpCtx) (model.MutationResult, error) {
	return model.MutationResult{}, adapters.Unsupported("kibana", "mutate")
}

func (a *Adapter) DownloadObject(context.Context, model.ObjectDownloadRequest, *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{}, adapters.Unsupported("kibana", "download")
}

func (a *Adapter) KeyTypes(context.Context, []model.NodePath, *adapters.OpCtx) ([]string, error) {
	return nil, adapters.Unsupported("kibana", "key types")
}
