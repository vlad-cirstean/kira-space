package mysqlfamily_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// P532 conformance: the live routine probe behind the table allow list, per kind.
func TestMariaDB_TableProbe(t *testing.T) {
	probeRoutines(t, "mariadb", testsupport.StartMariadb(t).Config, testsupport.RootMariaDSN)
}

func TestMySQL_TableProbe(t *testing.T) {
	probeRoutines(t, "mysql", testsupport.StartMysql(t).Config, testsupport.RootMysqlDSN)
}

func probeRoutines(t *testing.T, kind string, cfg model.ResolvedConnectionConfig, rootDSN func(host string, port int, database string) string) {
	a := connectedAdapter(t, kind, cfg)
	side, err := sql.Open("mysql", rootDSN(*cfg.Host, *cfg.Port, *cfg.Database))
	if err != nil {
		t.Fatalf("side open: %v", err)
	}
	defer side.Close()
	// MySQL refuses CREATE FUNCTION under binary logging unless creators are trusted; MariaDB has no such variable.
	_, _ = side.Exec("SET GLOBAL log_bin_trust_function_creators = 1")
	create := func(name string) {
		t.Helper()
		for _, stmt := range []string{
			"DROP FUNCTION IF EXISTS " + name,
			"CREATE FUNCTION " + name + "() RETURNS INT DETERMINISTIC RETURN 1",
		} {
			if _, err := side.Exec(stmt); err != nil {
				t.Fatalf("setup %q: %v", stmt, err)
			}
		}
		t.Cleanup(func() { _, _ = side.Exec("DROP FUNCTION IF EXISTS " + name) })
	}
	create("p532_fn")

	ref, ok := a.(adapters.TableReferencer)
	if !ok {
		t.Fatalf("%s adapter is not a TableReferencer", kind)
	}
	path := testsupport.NodePath(cfg.ID, seg("database", *cfg.Database))
	probe := func(stmt string) string {
		t.Helper()
		parsed := ref.ParseTables(stmt, path)
		if parsed.Refusal != "" {
			return parsed.Refusal
		}
		if !parsed.NeedsProbe() {
			return ""
		}
		_, refusal, err := ref.ResolveTables(context.Background(), parsed, path, adapters.NewOpCtx("tables"))
		if err != nil {
			t.Fatalf("ResolveTables(%q): %v", stmt, err)
		}
		return refusal
	}
	if probe("SELECT p532_fn()") == "" {
		t.Error("a user-defined function was not refused")
	}
	if r := probe("SELECT LOWER('A'), COUNT(*) FROM t"); r != "" {
		t.Errorf("a built-in function was refused: %s", r)
	}
	create("p532_late")
	if probe("SELECT p532_late()") == "" {
		t.Error("a function created after startup was not refused")
	}
}
