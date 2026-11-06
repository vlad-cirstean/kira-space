// authmatrix_test.go is the complete suite's table for kibana: credentials, auth mechanism,
// space and address. One fixture; each case rebuilds its config from the fixture's principals.
package kibana_test

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/kibana"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestKibana_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartKibana(t)

	testsupport.RunMatrix(t, "kibana", f, f.Config, []testsupport.Case{
		{
			Name:   "read-only role user, basic auth",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"url": f.BaseURL}},
		},
		{
			Name:   "elastic superuser",
			Config: func(model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return f.AdminConfig },
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name:   "API key, no username",
			Config: func(model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return f.APIKeyConfig },
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "read-only role user in a named space",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = testsupport.Strp(testsupport.KibanaSpace)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"space": testsupport.KibanaSpace}},
		},
		{
			Name: "unknown space",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = testsupport.Strp("no-such-space")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeNotFound},
		},
		{
			// Kibana answers the data view list with 200 and no rows for a user holding no Kibana
			// privilege, so it connects and shows an empty tree rather than an auth failure.
			Name:   "user without Kibana privileges connects to an empty tree",
			Config: func(model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return f.NoPrivConfig },
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{{
				Name: "lists no data views",
				Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
					children, err := a.Children(context.Background(), rootPath(cfg), opCtx("matrix-nopriv"))
					if err != nil {
						t.Fatalf("Children: %v", err)
					}
					if names := testsupport.ChildNames(t, children); len(names) != 0 {
						t.Errorf("data views = %v, want none", names)
					}
				},
			}},
		},
		{
			Name: "wrong password",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Password = testsupport.Strp("definitely-wrong")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "empty password",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Password = testsupport.Strp("")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "wrong API key",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c = f.APIKeyConfig
				c.Password = testsupport.Strp("bm90LWEta2V5")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "unreachable host:port",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Host, c.Port = testsupport.Strp("127.0.0.1"), intp(1)
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
	})
}

func intp(n int) *int { return &n }
