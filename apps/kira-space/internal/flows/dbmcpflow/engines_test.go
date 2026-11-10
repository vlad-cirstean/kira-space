package dbmcpflow

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// P602 engine table: the complete suite's TestDbMcpEngine* tests run one scenario per real engine on
// the memoized testsupport containers (the same ones the adapter suites use), SQLite on a temp file.
// Scratch objects are named p602_<area>_<kind>, created through MCP in the test and dropped in
// t.Cleanup; seeded tables are only read. No test here runs in parallel: the containers are shared and
// the timing assertions need a quiet engine.

// engine describes how one connection kind is started and spoken to. A {t} token in a statement
// is replaced with the scratch table (or collection or key) name.
type engine struct {
	kind string
	// path is run_query's path argument.
	path string
	// config starts the engine's fixture and returns its connection; nil for SQLite, which uses the
	// fixture app's own file.
	config func(t *testing.T) model.ResolvedConnectionConfig
	// scratch derives the scratch object name for an area.
	scratch func(area string) string
	// Statements. An empty ddl means the engine has no DDL class (the write creates the object).
	ddl, write, write2, read, count, drop string
	// slow* run well past a 1 s limit; medium runs about 2 s and succeeds.
	slowRead, slowWrite, slowDDL, medium string
	// big is a seeded table with about a million rows and its key column; empty where the engine has none.
	big, bigPath string
	// customers is the seeded people table for masking; masks and browse are skipped where empty.
	customers, customersPath string
	// castErr is a statement that fails on the engine with a message naming data.
	castErr string
	// hiddenWrite creates a function or other object that writes when a read calls it; empty where
	// the engine has no such path. hiddenCall runs under mode=read.
	hiddenSetup []string
	hiddenCall  string
	hiddenTable string
	// enforcement is list_connections statementTimeout.enforcement for read, write, ddl.
	enforcement string
	// rowEstimate is whether the engine's plan carries a row estimate.
	rowEstimate bool
	// structured is whether browse_table works (SQL kinds).
	structured bool
}

func pgScratch(area string) string { return "app.p602_" + area + "_pg" }
func flatScratch(kind string) func(string) string {
	return func(area string) string { return "p602_" + area + "_" + kind }
}

const (
	chEndless = "select sum(cityHash64(number)) from numbers(1000000000000)"
	// slowSQLiteBody is the SQLite recursive count slowRead uses, as a subquery source.
	slowSQLiteBody = "with recursive c(x) as (select 1 union all select x+1 from c) select x from c limit 400000000"
	// slowTwoRows is a MySQL or MariaDB read the engine interrupts: SLEEP in a WHERE over two rows.
	slowTwoRows = "(select 1 as n union all select 2) d where sleep(5)"
)

// all lists every engine the container tests iterate. SQLite reuses the flow fixture's file.
func allEngines() []engine {
	return []engine{
		{
			kind: "postgres", path: "database:kira_test", scratch: pgScratch,
			config: func(t *testing.T) model.ResolvedConnectionConfig { return testsupport.StartPostgres(t).Config },
			ddl:    "create table {t} (id int primary key, v text)",
			write:  "insert into {t} (id, v) values (1, 'a')",
			write2: "insert into {t} (id, v) values (2, 'b')", count: "select count(*) as n from {t}",
			read: "select v from {t}", drop: "drop table if exists {t}",
			slowRead:  "select pg_sleep(5)",
			slowWrite: "update {t} set v = 'z' where pg_sleep(5) is not null",
			slowDDL:   "create table {t} as select pg_sleep(5)",
			medium:    "select pg_sleep(2)",
			big:       "app.big_rows", bigPath: tablePath("kira_test", "app", "big_rows"),
			customers: "app.customers", customersPath: tablePath("kira_test", "app", "customers"),
			castErr:     "select name::int from app.customers",
			enforcement: `{"ddl":"session","read":"session","write":"session"}`,
			rowEstimate: true, structured: true,
		},
		mysqlEngine("mysql", func(t *testing.T) model.ResolvedConnectionConfig { return testsupport.StartMysql(t).Config },
			`{"ddl":"app","read":"session","write":"app"}`),
		mysqlEngine("mariadb", func(t *testing.T) model.ResolvedConnectionConfig { return testsupport.StartMariadb(t).Config },
			`{"ddl":"app","read":"session","write":"session"}`),
		{
			kind: "clickhouse", path: "", scratch: flatScratch("ch"),
			config: func(t *testing.T) model.ResolvedConnectionConfig { return testsupport.StartClickHouse(t).Config },
			ddl:    "create table {t} (id UInt32, v String) engine = MergeTree order by id",
			write:  "insert into {t} values (1, 'a')",
			write2: "insert into {t} values (2, 'b')", count: "select count() as n from {t}",
			read: "select v from {t}", drop: "drop table if exists {t}",
			slowRead:  chEndless,
			slowWrite: "insert into {t} select toUInt32(sum(cityHash64(number)) % 2), 'x' from numbers(1000000000000)",
			slowDDL:   "create table {t} engine = Memory as select sum(cityHash64(number)) as s from numbers(1000000000000)",
			medium:    "select sleepEachRow(1) from numbers(2) settings max_block_size = 1",
			big:       "big_rows", bigPath: tablePath("kira_test", "big_rows"),
			customers: "customers", customersPath: tablePath("kira_test", "customers"),
			castErr:     "select toInt32(name) from customers",
			enforcement: `{"ddl":"app","read":"session","write":"app"}`,
			rowEstimate: true, structured: true,
		},
		{
			kind: "sqlite", path: "", scratch: flatScratch("sl"),
			ddl:    "create table {t} (id integer primary key, v text)",
			write:  "insert into {t} (id, v) values (1, 'a')",
			write2: "insert into {t} (id, v) values (2, 'b')", count: "select count(*) as n from {t}",
			read: "select v from {t}", drop: "drop table if exists {t}",
			slowRead:  "select count(*) from (" + slowSQLiteBody + ")",
			slowWrite: "insert into {t} (v) select 'x' from (" + slowSQLiteBody + ")",
			slowDDL:   "create table {t} as select count(*) as s from (" + slowSQLiteBody + ")",
			medium:    cte(mediumCTE),
			customers: "notes", castErr: "select * from no_such_table_p602",
			enforcement: `{"ddl":"app","read":"app","write":"app"}`,
			structured:  true,
		},
		{
			kind: "mongodb", path: "database:" + testsupport.MongoDatabase, scratch: flatScratch("mo"),
			config:      func(t *testing.T) model.ResolvedConnectionConfig { return testsupport.StartMongo(t).Config },
			write:       `db.{t}.insertOne({ n: 1 })`,
			write2:      `db.{t}.insertOne({ n: 2 })`,
			read:        `db.{t}.find({})`,
			drop:        `db.{t}.drop()`,
			slowRead:    `db.{t}.find({$where: "sleep(5000) || true"})`,
			slowWrite:   `db.{t}.updateMany({$where: "sleep(5000) || true"}, {$set: {v: 1}})`,
			medium:      `db.{t}.find({$where: "sleep(2000) || true"})`,
			enforcement: `{"ddl":"app","read":"app","write":"app"}`,
		},
		{
			kind: "redis", path: "", scratch: flatScratch("rd"),
			config:      func(t *testing.T) model.ResolvedConnectionConfig { return testsupport.StartRedis(t).Config },
			write:       "SET {t} 1",
			write2:      "SET {t}_b 2",
			read:        "GET {t}",
			drop:        "DEL {t}",
			slowRead:    "XREAD BLOCK 5000 STREAMS {t}_s $",
			slowWrite:   "BLPOP {t}_list 5",
			medium:      "XREAD BLOCK 2000 STREAMS {t}_s $",
			enforcement: `{"ddl":"app","read":"app","write":"app"}`,
		},
	}
}

func mysqlEngine(kind string, config func(*testing.T) model.ResolvedConnectionConfig, enforcement string) engine {
	return engine{
		kind: kind, path: "database:kira_test", scratch: flatScratch(kind[:2]),
		config: config,
		ddl:    "create table {t} (id int primary key, v varchar(20))",
		write:  "insert into {t} (id, v) values (1, 'a')",
		write2: "insert into {t} (id, v) values (2, 'b')", count: "select count(*) as n from {t}",
		read: "select v from {t}", drop: "drop table if exists {t}",
		slowRead:  "select 1 from " + slowTwoRows,
		slowWrite: "update {t} set v = 'z' where sleep(5) = 0",
		slowDDL:   "create table {t} as select sleep(5) as s",
		medium:    "select sleep(2)",
		big:       "big_rows", bigPath: tablePath("kira_test", "big_rows"),
		customers: "customers", customersPath: tablePath("kira_test", "customers"),
		castErr:     "select * from no_such_table_p602",
		enforcement: enforcement, rowEstimate: true, structured: true,
		hiddenSetup: []string{
			"create table {t}_log (id int)",
			"create function {t}_fn() returns int modifies sql data begin insert into {t}_log values (1); return 1; end",
		},
		hiddenCall: "select {t}_fn()", hiddenTable: "{t}_log",
	}
}

// engines returns the table filtered to kinds (all when none), in table order.
func engines(kinds ...string) []engine {
	var out []engine
	for _, e := range allEngines() {
		if len(kinds) == 0 {
			out = append(out, e)
			continue
		}
		for _, k := range kinds {
			if e.kind == k {
				out = append(out, e)
			}
		}
	}
	return out
}

// sql substitutes the scratch name into a statement.
func (e engine) sql(stmt, scratch string) string { return strings.ReplaceAll(stmt, "{t}", scratch) }

// fields maps a testsupport connection onto Studio connection fields.
func fieldsOf(cfg model.ResolvedConnectionConfig, name string) connections.Input {
	opts := cfg.Options
	if opts == nil {
		opts = map[string]any{}
	}
	return connections.Input{ConnectionFields: model.ConnectionFields{
		Name: name, Kind: cfg.Kind, Color: "teal", Mode: cfg.Mode,
		Host: cfg.Host, Port: cfg.Port, Database: cfg.Database, Username: cfg.Username, Options: opts,
	}, Password: cfg.Password}
}

// engineConn starts the engine's fixture, adds an exposed connection with every mode allowed (tweak
// edits the fields first) and points the flow fixture at it. It returns the connection id.
func (f *fixture) engineConn(t *testing.T, e engine, tweak func(*model.ConnectionFields)) string {
	t.Helper()
	var in connections.Input
	if e.config == nil {
		in = connections.Input{ConnectionFields: model.ConnectionFields{
			Name: "p602 " + e.kind, Kind: e.kind, Color: "teal", Mode: "fields", Database: ptr(f.dbPath), Options: map[string]any{},
		}}
	} else {
		in = fieldsOf(e.config(t), "p602 "+e.kind)
	}
	in.McpEnabled, in.McpAutoExplain = true, false
	in.McpReadMode, in.McpWriteMode, in.McpDdlMode = "allow", "allow", "allow"
	if tweak != nil {
		tweak(&in.ConnectionFields)
	}
	conn, err := f.app.W.Connections.Create(in)
	if err != nil {
		t.Fatalf("create %s connection: %v", e.kind, err)
	}
	f.connID = conn.ID
	return conn.ID
}

// args builds run_query arguments for this engine.
func (e engine) args(connID, sqlText, mode string) map[string]any {
	return map[string]any{"connectionId": connID, "sql": sqlText, "path": e.path, "mode": mode}
}

func (e engine) run(t *testing.T, sess *mcp.ClientSession, connID, sqlText, mode string) *mcp.CallToolResult {
	t.Helper()
	return call(t, sess, e.args(connID, sqlText, mode))
}

func (e engine) runAsync(sess *mcp.ClientSession, connID, sqlText, mode string, extra map[string]any) chan callOutcome {
	ch := make(chan callOutcome, 1)
	args := e.args(connID, sqlText, mode)
	for k, v := range extra {
		args[k] = v
	}
	go func() {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: args})
		ch <- callOutcome{res, err}
	}()
	return ch
}

// setup creates the scratch object (ddl, then a seed write) and registers its drop. The statements
// run as an allow-all connection, so it fails the test on any refusal.
func (e engine) setup(t *testing.T, sess *mcp.ClientSession, connID, scratch string) {
	t.Helper()
	t.Cleanup(func() {
		sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: e.args(connID, e.sql(e.drop, scratch), "ddl")}) //nolint:errcheck // best-effort cleanup
	})
	if e.ddl != "" {
		if r := e.run(t, sess, connID, e.sql(e.ddl, scratch), "ddl"); r.IsError {
			t.Fatalf("%s ddl = %q", e.kind, text(r))
		}
	}
	if r := e.run(t, sess, connID, e.sql(e.write, scratch), "write"); r.IsError {
		t.Fatalf("%s seed write = %q", e.kind, text(r))
	}
}

// Stop every memoized container once the package's tests are done. A no-op for any never started.
func stopFixtures() {
	testsupport.StopConcurrently(
		testsupport.StopPostgres, testsupport.StopMysql, testsupport.StopMariadb, testsupport.StopClickHouse,
		testsupport.StopMongo, testsupport.StopRedis, testsupport.StopKibana,
	)
}

// engineTest is the shared prologue: complete suite, Docker present, a fresh flow app per engine.
func engineTest(t *testing.T, kinds []string, body func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession)) {
	t.Helper()
	flowharness.Complete(t)
	for _, e := range engines(kinds...) {
		t.Run(e.kind, func(t *testing.T) {
			if e.config != nil {
				flowharness.RequireDocker(t)
			}
			f := newFixture(t)
			body(t, e, f, f.session(t))
		})
	}
}
