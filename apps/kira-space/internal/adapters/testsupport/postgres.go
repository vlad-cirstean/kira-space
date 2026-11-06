// Package testsupport is the Go analogue of packages/db-fixtures/support/*.ts: the Docker gate,
// real container startup, and seed-SQL loading each adapter's own acceptance spec needs. One
// container per test binary, started lazily on first call and reused by every later call in the
// same process — a fresh container per test would make the suite unusable.
package testsupport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// DockerUnavailableMessage mirrors packages/db-fixtures/support/docker.ts's own DOCKER_UNAVAILABLE_MESSAGE,
// re-pointed at this environment's own startup procedure (CLAUDE.md's Docker section) rather than
// Colima's, since this is the Go test tier, not the dev machine's.
const DockerUnavailableMessage = "Docker daemon unreachable — see CLAUDE.md's Docker section for " +
	"how to start it (nohup dockerd, or colima start on macOS) and retry."

const dockerProbeTimeout = 5 * time.Second

var (
	dockerProbeOnce   sync.Once
	dockerProbeResult bool
)

// IsDockerAvailable is docker.ts's isDockerAvailable. Memoized per test binary (sync.Once): the
// daemon does not appear or disappear mid-run, and this is called from every Start<Kind> plus
// Prewarm — 144 call sites across the suite before this change, each a 60ms `docker info` shell-out.
func IsDockerAvailable() bool {
	dockerProbeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), dockerProbeTimeout)
		defer cancel()
		dockerProbeResult = exec.CommandContext(ctx, "docker", "info").Run() == nil
	})
	return dockerProbeResult
}

// PgFixture is postgres.ts's PgFixture.
type PgFixture struct {
	URI       string
	Config    model.ResolvedConnectionConfig
	container testcontainers.Container
}

// ConnectionID satisfies hasConnectionID (P107 I2-28, FixtureNodePath's own constraint).
func (f PgFixture) ConnectionID() string { return f.Config.ID }

var pgMemo fixture[PgFixture]

// repoRoot resolves the repository root relative to this source file, so the seed SQL path does
// not depend on the test binary's own working directory.
func repoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// this file: apps/kira-space/internal/adapters/testsupport/postgres.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
}

const (
	defaultPostgresImage = "postgres:17.11-alpine"
	password             = FixturePassword
	database             = "kira_test"
)

// StartPostgres is postgres.ts's startPostgres. Skips the test (never a silent pass, never a hard
// failure that makes go test ./internal/adapters/postgres unusable without Docker) when the
// daemon is unreachable.
//
// Memoized process-wide by fixture[T] (bun:test's own beforeAll/afterAll-per-file precedent) —
// termination is deliberately NOT wired to any one test's t.Cleanup: see fixture.go's
// own doc comment for why. Call StopPostgres from the package's own TestMain instead.
func StartPostgres(t *testing.T) *PgFixture {
	return pgMemo.start(t, "postgres container", startPostgres)
}

// StopPostgres terminates the memoized container, if one was ever started. Call once, from the
// test binary's own TestMain, after m.Run() returns — never from an individual test.
func StopPostgres() {
	pgMemo.stop(func(f *PgFixture) { _ = f.container.Terminate(context.Background()) })
}

func startPostgres() (*PgFixture, error) {
	ctx := context.Background()

	// The same wait strategy confirmed working in this sandbox: Postgres logs
	// "database system is ready to accept connections" twice during initdb's own double-boot, and
	// waiting for the second occurrence is what @testcontainers/postgresql's own pg_isready-based
	// healthcheck solves on the TypeScript side — testcontainers-go's postgres module has no
	// equivalent built-in healthcheck, so this is spelled out explicitly here instead.
	container, err := tcpostgres.Run(ctx, ImageFor("postgres", defaultPostgresImage),
		tcpostgres.WithDatabase(database),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword(password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120*time.Second),
		),
	)
	if err != nil {
		return nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, err
	}
	uri := "postgres://postgres:" + password + "@" + host + ":" + port.Port() + "/" + database

	seedPath := filepath.Join(repoRoot(), "packages", "db-fixtures", "fixtures", "0001_seed.sql")
	seedSQL, err := os.ReadFile(seedPath)
	if err != nil {
		return nil, err
	}
	if err := seedDatabase(ctx, uri, string(seedSQL)); err != nil {
		return nil, err
	}
	// postgres.ts's own start(): app.big_rows is created by the seed file but populated and
	// ANALYZEd separately (opts?.seedBigTable ?? true) — done unconditionally here since every one
	// of this package's own tests wants the real 1,000,000-row table, not the empty-table
	// shortcut the TS harness offers callers that don't need paging.
	if err := seedBigRows(ctx, uri); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	hostCopy, portInt := host, int(port.Num())
	cfg := model.ResolvedConnectionConfig{
		ID: "test-postgres", SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: "Test Postgres", Kind: "postgres", Color: "blue", Mode: "fields", ReadOnly: false,
		Host: &hostCopy, Port: &portInt, Database: Strp(database), Username: Strp("postgres"),
		Options: map[string]any{}, Password: Strp(password),
	}
	return &PgFixture{URI: uri, Config: cfg, container: container}, nil
}
