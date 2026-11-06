// Package kibana_test is the conformance suite for the Kibana adapter against a real Kibana and
// Elasticsearch pair (testsupport.StartKibana).
package kibana_test

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/kibana"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.StopKibana()
	os.Exit(code)
}

var (
	deps      = adapters.Deps{Log: func(level, message string) {}}
	opCounter atomic.Int64
)

func opCtx(name string) *adapters.OpCtx {
	return adapters.NewOpCtx(name + "-" + strconv.FormatInt(opCounter.Add(1), 10))
}

func newAdapter(t *testing.T) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter("kibana", deps)
	if err != nil {
		t.Fatalf("CreateAdapter(kibana): %v", err)
	}
	return a
}

func connected(t *testing.T, cfg model.ResolvedConnectionConfig) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), cfg, opCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

func wantCode(t *testing.T, err error, want adapters.ErrorCode) {
	t.Helper()
	if code, ok := adapters.CodeOf(err); !ok || code != want {
		t.Fatalf("err = %v, want code %s", err, want)
	}
}

func rootPath(cfg model.ResolvedConnectionConfig) model.NodePath { return testsupport.NodePath(cfg.ID) }

func viewPath(cfg model.ResolvedConnectionConfig, id string) model.NodePath {
	return testsupport.NodePath(cfg.ID, testsupport.Seg("collection", id))
}

func TestKibana(t *testing.T) {
	f := testsupport.StartKibana(t)
	cfg := f.Config

	t.Run("connect reports the Kibana version", func(t *testing.T) {
		a := newAdapter(t)
		info, err := a.Connect(context.Background(), f.AdminConfig, opCtx("connect"))
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer a.Disconnect(context.Background())
		want := "Kibana " + testsupport.KibanaServerMajor() + "."
		if len(info.ServerVersion) < len(want) || info.ServerVersion[:len(want)] != want {
			t.Errorf("ServerVersion = %q, want prefix %q", info.ServerVersion, want)
		}
	})

	t.Run("connect to a space", func(t *testing.T) {
		spaced := cfg
		spaced.Database = testsupport.Strp(testsupport.KibanaSpace)
		a := connected(t, spaced)
		children, err := a.Children(context.Background(), rootPath(spaced), opCtx("children"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		names := testsupport.ChildNames(t, children)
		if !slices.Equal(names, []string{"space-logs"}) {
			t.Errorf("names = %v, want only the space's own data view", names)
		}
	})

	t.Run("unknown space is not found", func(t *testing.T) {
		bad := cfg
		bad.Database = testsupport.Strp("no-such-space")
		_, err := newAdapter(t).Connect(context.Background(), bad, opCtx("connect"))
		wantCode(t, err, adapters.CodeNotFound)
	})

	t.Run("wrong password is an auth error", func(t *testing.T) {
		bad := cfg
		bad.Password = testsupport.Strp("definitely-wrong")
		_, err := newAdapter(t).Connect(context.Background(), bad, opCtx("connect"))
		wantCode(t, err, adapters.CodeAuth)
	})

	t.Run("closed port is a connect error", func(t *testing.T) {
		bad := cfg
		bad.Port = new(int)
		*bad.Port = 1
		_, err := newAdapter(t).Connect(context.Background(), bad, opCtx("connect"))
		wantCode(t, err, adapters.CodeConnect)
	})

	t.Run("children lists data views by name", func(t *testing.T) {
		a := connected(t, cfg)
		children, err := a.Children(context.Background(), rootPath(cfg), opCtx("children"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		names := testsupport.ChildNames(t, children)
		want := []string{"kira-logs", "kira-notime", "Wildcard logs"}
		slices.SortFunc(want, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
		if !slices.Equal(names, want) {
			t.Fatalf("names = %v, want %v", names, want)
		}
		for _, n := range children.Nodes {
			if n.Kind != "collection" || n.HasChildren {
				t.Errorf("node %+v: want a leaf collection", n)
			}
			if n.Name == "Wildcard logs" {
				if n.Detail == nil || *n.Detail != "kira-log*" {
					t.Errorf("titled view detail = %v, want its index pattern", n.Detail)
				}
				if n.Path != "collection:"+testsupport.KibanaTitledView {
					t.Errorf("path = %q, want the data view id", n.Path)
				}
			}
		}
	})

	t.Run("a data view is a leaf", func(t *testing.T) {
		a := connected(t, cfg)
		children, err := a.Children(context.Background(), viewPath(cfg, testsupport.KibanaLogsView), opCtx("children"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		if len(children.Nodes) != 0 {
			t.Errorf("nodes = %v, want none", children.Nodes)
		}
	})
}
