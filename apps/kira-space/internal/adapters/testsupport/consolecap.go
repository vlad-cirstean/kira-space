package testsupport

import (
	"context"
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// ConsoleResultCap asserts the console stops at ConsoleRequest.Cap: a result with one row more
// than the cap reports Truncated and holds exactly cap rows; a result of exactly cap rows does not.
// rowsSQL returns a statement yielding n single-column rows in the engine's own dialect.
func ConsoleResultCap(path model.NodePath, rowsSQL func(n int) string) Scenario {
	const capRows = 5
	return Scenario{
		Name:     "execute: console result stops at the row cap",
		Requires: func(c adapters.Caps) bool { return c.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			// Distinct op ids: a capped run leaves its server-side query alive briefly, and ClickHouse rejects a reused query id.
			run := func(n int) page.TabularPage {
				t.Helper()
				pages, err := a.Execute(context.Background(), model.ConsoleRequest{
					Path: path, Statements: []string{rowsSQL(n)}, Cap: page.ResultCap{Rows: capRows},
				}, adapters.NewOpCtx(fmt.Sprintf("scenario-console-result-cap-%d", n)))
				if err != nil {
					t.Fatalf("Execute(%d rows): %v", n, err)
				}
				return pages[0].(page.TabularPage)
			}
			if p := run(capRows + 1); p.RowCount != capRows || !p.Position.Truncated {
				t.Errorf("cap+1 rows: RowCount = %d, Truncated = %v, want %d, true", p.RowCount, p.Position.Truncated, capRows)
			}
			if p := run(capRows); p.RowCount != capRows || p.Position.Truncated {
				t.Errorf("exactly cap rows: RowCount = %d, Truncated = %v, want %d, false", p.RowCount, p.Position.Truncated, capRows)
			}
		},
	}
}
