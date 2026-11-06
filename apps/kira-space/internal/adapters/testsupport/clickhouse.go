package testsupport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcclickhouse "github.com/testcontainers/testcontainers-go/modules/clickhouse"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ClickHouseFixture is support/clickhouse.ts's ClickHouseFixture.
type ClickHouseFixture struct {
	Config         model.ResolvedConnectionConfig
	ReadOnlyConfig model.ResolvedConnectionConfig
	BaseURL        string
	container      testcontainers.Container
}

var clickhouseMemo fixture[ClickHouseFixture]

const (
	clickhouseImage      = "clickhouse/clickhouse-server:26.3.58"
	clickhouseAdminUser  = "kira_admin"
	clickhouseAdminPass  = FixturePassword
	clickhouseDatabase   = "kira_test"
	clickhouseUsername   = "kira"
	clickhousePassword   = FixturePassword
	clickhouseROUsername = "kira_ro"
	clickhouseROPassword = FixturePassword
)

// StartClickHouse is support/clickhouse.ts's startClickHouse. See fixture.go's own doc comment for
// why termination is never wired to t.Cleanup (B15).
func StartClickHouse(t *testing.T) *ClickHouseFixture {
	return clickhouseMemo.start(t, "clickhouse container", startClickHouse)
}

// StopClickHouse terminates the memoized container, if one was ever started. Call once, from the
// test binary's own TestMain, after m.Run() returns — never from an individual test.
func StopClickHouse() {
	clickhouseMemo.stop(func(f *ClickHouseFixture) { _ = f.container.Terminate(context.Background()) })
}

func startClickHouse() (*ClickHouseFixture, error) {
	ctx := context.Background()

	// Without CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1, the official image's own bootstrap grants
	// kira_admin plain GRANT ALL ON *.* — which excludes ACCESS MANAGEMENT (CREATE USER, GRANT,
	// ...) — so the CREATE USER statements below would fail with ACCESS_DENIED.
	container, err := tcclickhouse.Run(ctx, ImageFor("clickhouse", clickhouseImage),
		tcclickhouse.WithDatabase(clickhouseDatabase),
		tcclickhouse.WithUsername(clickhouseAdminUser),
		tcclickhouse.WithPassword(clickhouseAdminPass),
		testcontainers.WithEnv(map[string]string{"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT": "1"}),
	)
	if err != nil {
		return nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}
	port, err := container.MappedPort(ctx, "8123/tcp")
	if err != nil {
		return nil, err
	}
	baseURL := fmt.Sprintf("http://%s:%s", host, port.Port())

	seedSQL, err := os.ReadFile(clickhouseSeedPath())
	if err != nil {
		return nil, err
	}
	if err := runClickHouseStatements(ctx, baseURL, clickhouseAdminUser, clickhouseAdminPass, clickhouseDatabase, string(seedSQL)); err != nil {
		return nil, err
	}

	if err := grantClickHouseUsers(ctx, baseURL); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	portNum := int(port.Num())
	config := model.ResolvedConnectionConfig{
		ID: "test-clickhouse", SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: "Test ClickHouse", Kind: "clickhouse", Color: "orange", Mode: "fields", ReadOnly: false,
		Host: &host, Port: &portNum, Database: Strp(clickhouseDatabase), Username: Strp(clickhouseUsername),
		Options: map[string]any{}, Password: Strp(clickhousePassword),
	}
	readOnlyConfig := config
	readOnlyConfig.ID = "test-clickhouse-ro"
	readOnlyConfig.Name = "Test ClickHouse (read-only)"
	readOnlyConfig.ReadOnly = true
	readOnlyConfig.Username = Strp(clickhouseROUsername)
	readOnlyConfig.Password = Strp(clickhouseROPassword)

	return &ClickHouseFixture{Config: config, ReadOnlyConfig: readOnlyConfig, BaseURL: baseURL, container: container}, nil
}

// grantClickHouseUsers is idempotent. Two unprivileged users, created only after the seed lands —
// the same root-seeds/app-user-connects split support/mysql.go's own comment explains, which is
// what makes the cancel assertion and the read-only assertion meaningful: neither is a
// superuser connection.
// kira's own ALTER carries CREATE TABLE and DROP TABLE too (ClickHouse's ALTER privilege does not
// itself cover object creation/deletion) — needed for the DDL round-trip scratch table the tests
// create and drop over this same principal.
func grantClickHouseUsers(ctx context.Context, baseURL string) error {
	grants := fmt.Sprintf(`
		CREATE USER IF NOT EXISTS %[1]s IDENTIFIED WITH plaintext_password BY '%[2]s';
		GRANT SELECT, INSERT, ALTER, CREATE TABLE, DROP TABLE ON %[3]s.* TO %[1]s;
		GRANT SELECT ON system.* TO %[1]s;
		GRANT SELECT ON default.* TO %[1]s;
		CREATE USER IF NOT EXISTS %[4]s IDENTIFIED WITH plaintext_password BY '%[5]s';
		GRANT SELECT ON %[3]s.* TO %[4]s;
		GRANT SELECT ON system.* TO %[4]s;
		GRANT SELECT ON default.* TO %[4]s;
	`, clickhouseUsername, clickhousePassword, clickhouseDatabase, clickhouseROUsername, clickhouseROPassword)
	return runClickHouseStatements(ctx, baseURL, clickhouseAdminUser, clickhouseAdminPass, clickhouseDatabase, grants)
}

// ClickHouseRun resets the shared container to its seeded state: the memoized container outlives a
// test iteration, so a second `-count` run would otherwise see the first run's inserts and scratch tables.
// Call it first in every top-level test, after StartClickHouse.
func ClickHouseRun(t *testing.T, f *ClickHouseFixture) {
	t.Helper()
	ctx := context.Background()
	seedSQL, err := os.ReadFile(clickhouseSeedPath())
	if err != nil {
		t.Fatalf("ClickHouseRun: %v", err)
	}
	reset := fmt.Sprintf("DROP DATABASE IF EXISTS %[1]s SYNC;\nCREATE DATABASE %[1]s;", clickhouseDatabase)
	if err := runClickHouseStatements(ctx, f.BaseURL, clickhouseAdminUser, clickhouseAdminPass, "default", reset); err != nil {
		t.Fatalf("ClickHouseRun: %v", err)
	}
	if err := runClickHouseStatements(ctx, f.BaseURL, clickhouseAdminUser, clickhouseAdminPass, clickhouseDatabase, string(seedSQL)); err != nil {
		t.Fatalf("ClickHouseRun: %v", err)
	}
	if err := grantClickHouseUsers(ctx, f.BaseURL); err != nil {
		t.Fatalf("ClickHouseRun: %v", err)
	}
}

func clickhouseSeedPath() string {
	return filepath.Join(repoRoot(), "packages", "db-fixtures", "fixtures", "0010_clickhouse_seed.sql")
}

// AdminStatements runs sql as ClickHouse's own admin user (kira_admin) — for a matrix case's own
// runtime role/grant statements, reusing the same statement splitter and HTTP path the fixture's
// own seed step already goes through.
func AdminStatements(t *testing.T, f *ClickHouseFixture, sql string) {
	t.Helper()
	if err := runClickHouseStatements(context.Background(), f.BaseURL, clickhouseAdminUser, clickhouseAdminPass, clickhouseDatabase, sql); err != nil {
		t.Fatalf("AdminStatements: %v", err)
	}
}

// firstTopLevelSemicolon mirrors sqlite/query.go's own scanner (comments and quoted literals are
// skipped over so a semicolon inside a string value never counts) — generalized here to find every
// boundary rather than just the first, since this fixture's own job (unlike the adapter's guard)
// is to split a whole seed file into individually-POSTed statements: the ClickHouse HTTP interface
// has no multi-statement exec at all, unlike mariadb's importFile or node-postgres's own
// multi-statement query.
func splitClickHouseStatements(sql string) []string {
	const (
		normal = iota
		lineComment
		blockComment
		single
		backtick
	)
	state := normal
	var statements []string
	start := 0
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch state {
		case normal:
			switch {
			case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
				state = lineComment
				i++
			case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
				state = blockComment
				i++
			case c == '\'':
				state = single
			case c == '`':
				state = backtick
			case c == ';':
				statements = append(statements, strings.TrimSpace(sql[start:i]))
				start = i + 1
			}
		case lineComment:
			if c == '\n' {
				state = normal
			}
		case blockComment:
			if c == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				state = normal
				i++
			}
		case single:
			if c == '\\' {
				i++
			} else if c == '\'' {
				state = normal
			}
		case backtick:
			if c == '`' {
				state = normal
			}
		}
	}
	if strings.TrimSpace(sql[start:]) != "" {
		statements = append(statements, strings.TrimSpace(sql[start:]))
	}
	out := statements[:0]
	for _, s := range statements {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func runClickHouseStatements(ctx context.Context, baseURL, user, password, database, sql string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	for _, stmt := range splitClickHouseStatements(sql) {
		u := baseURL + "/?database=" + database
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(stmt))
		if err != nil {
			return err
		}
		req.Header.Set("X-ClickHouse-User", user)
		req.Header.Set("X-ClickHouse-Key", password)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("clickhouse seed statement failed (status %d): %s\nstatement: %s", resp.StatusCode, body, stmt)
		}
	}
	return nil
}
