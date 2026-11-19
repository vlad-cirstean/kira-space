package testsupport

import (
	"context"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// BoundProbe is one parameterised statement. Rows are compared as "cell|cell" text with NULL as
// "NULL".
type BoundProbe struct {
	Name   string
	SQL    string
	Params []model.BindParam
	// Lane runs the probe on the read lane (the DB MCP read mode) instead of the main connection.
	Lane bool
	Want []string
	// WantErr, when set, is a substring of the error the engine must return; nothing may have run.
	WantErr string
	// SameAs is a statement run without parameters whose rows this probe must equal: the text path
	// against the bound path, for engines whose bound rows travel a different protocol.
	SameAs string
}

// BoundParamsCase drives BoundParams. Probes need no table beyond the seeded fixture. Setup and
// Teardown create and drop a scratch table on the main connection for Writes, run in order there;
// Verify then reads the result back. Leave Setup empty on a connection that may not create tables.
type BoundParamsCase struct {
	Path            model.NodePath
	Probes          []BoundProbe
	Setup, Teardown []string
	Writes          []BoundProbe
	Verify          BoundProbe
}

// Bind builds a BindParam from a label and compact JSON.
func Bind(label, name, value string) model.BindParam {
	return model.BindParam{Label: label, Name: name, Value: []byte(value)}
}

// BoundParams asserts the console path binds values as server-side parameters on the main connection
// and on the read lane: scalars, lists through the kind's idiom, big integers and exact decimals, NULL,
// a write through a bound value, and that a wrong parameter count or a second statement is an error
// rather than a run. Kinds without bound parameters are skipped.
func BoundParams(c BoundParamsCase) Scenario {
	return Scenario{
		Name:     "bound parameters: values bind server-side, never into the text",
		Requires: func(caps adapters.Caps) bool { return caps.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			if _, _, ok := adapters.ParamDialectOf(a.Kind()); !ok {
				t.Skipf("%s takes no bound parameters", a.Kind())
			}
			var lane adapters.Adapter
			for _, p := range c.Probes {
				t.Run(p.Name, func(t *testing.T) {
					target := a
					if p.Lane {
						if lane == nil {
							lane = laneAdapter(t, a, cfg)
						}
						target = lane
					}
					runProbe(t, target, c.Path, p)
				})
			}
			if len(c.Setup) == 0 {
				return
			}
			t.Run("write and read back", func(t *testing.T) {
				execAll(t, a, c.Path, c.Setup)
				t.Cleanup(func() { execAll(t, a, c.Path, c.Teardown) })
				for _, w := range c.Writes {
					runProbe(t, a, c.Path, w)
				}
				runProbe(t, a, c.Path, c.Verify)
			})
		},
	}
}

func runProbe(t *testing.T, a adapters.Adapter, path model.NodePath, p BoundProbe) {
	t.Helper()
	pages, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path: path, Statements: []string{p.SQL}, Params: p.Params, ReadOnly: p.Lane,
	}, adapters.NewOpCtx("bound-params"))
	if p.WantErr != "" {
		if err == nil {
			t.Fatalf("%s: want an error containing %q, got rows %v", p.SQL, p.WantErr, rowsOf(pages))
		}
		if !strings.Contains(err.Error(), p.WantErr) {
			t.Fatalf("%s: error %q does not contain %q", p.SQL, err, p.WantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: %v", p.SQL, err)
	}
	got := rowsOf(pages)
	if p.SameAs != "" {
		want := runPlain(t, a, path, p.SameAs, p.Lane)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("bound rows %v differ from the text path's %v", got, want)
		}
	}
	if p.Want != nil && strings.Join(got, "\n") != strings.Join(p.Want, "\n") {
		t.Fatalf("%s: rows = %v, want %v", p.SQL, got, p.Want)
	}
}

func runPlain(t *testing.T, a adapters.Adapter, path model.NodePath, sql string, lane bool) []string {
	t.Helper()
	pages, err := a.Execute(context.Background(), model.ConsoleRequest{Path: path, Statements: []string{sql}, ReadOnly: lane},
		adapters.NewOpCtx("bound-params-plain"))
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return rowsOf(pages)
}

func rowsOf(pages []page.Page) []string {
	if len(pages) == 0 {
		return nil
	}
	tp, ok := pages[0].(page.TabularPage)
	if !ok {
		return nil
	}
	rows := make([]string, tp.RowCount)
	for r := 0; r < tp.RowCount; r++ {
		cells := make([]string, len(tp.Chunks))
		for i, ch := range tp.Chunks {
			if page.IsNull(ch, r) {
				cells[i] = "NULL"
			} else {
				cells[i] = page.CellText(ch, r)
			}
		}
		rows[r] = strings.Join(cells, "|")
	}
	return rows
}
