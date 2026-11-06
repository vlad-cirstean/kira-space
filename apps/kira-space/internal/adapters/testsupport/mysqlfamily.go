package testsupport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/testcontainers/testcontainers-go"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// mysqlFamilyFixture is MysqlFixture's and MariaFixture's own shared shape (P107 T2-5): both are
// aliases of this one type, since nothing about the fixture itself differs between the two
// engines — only how it's built (mysqlFamilySpec, below).
type mysqlFamilyFixture struct {
	URI       string
	Config    model.ResolvedConnectionConfig
	container testcontainers.Container
}

// mysqlFamilySpec is startMysql's and startMariadb's own shared container-run/wait/seed chain,
// factored over what genuinely differs per engine: how the container itself is
// built, the seed fixture file, the root-DSN builder, and the extras seed (mysqlfamily_seed.go
// already shares the fixture-file seed runner — this is everything around it).
type mysqlFamilySpec struct {
	runContainer func(ctx context.Context) (testcontainers.Container, error)
	seedFixture  string // filename under packages/db-fixtures/fixtures
	dsn          func(host string, port int, database string) string
	seedExtras   func(ctx context.Context, host string, port int) error

	database, username, password                  string
	uriScheme                                     string
	configID, configName, configKind, configColor string
	options                                       map[string]any
}

func startMysqlFamily(spec mysqlFamilySpec) (*mysqlFamilyFixture, error) {
	ctx := context.Background()

	container, err := spec.runContainer(ctx)
	if err != nil {
		return nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return nil, err
	}
	portNum := int(port.Num())

	seedPath := filepath.Join(repoRoot(), "packages", "db-fixtures", "fixtures", spec.seedFixture)
	seedSQL, err := os.ReadFile(seedPath)
	if err != nil {
		return nil, err
	}

	rootDSN := spec.dsn(host, portNum, spec.database)
	if err := seedMysqlFamilyDatabase(ctx, rootDSN, string(seedSQL)); err != nil {
		return nil, err
	}
	if err := spec.seedExtras(ctx, host, portNum); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	hostCopy, portCopy := host, portNum
	cfg := model.ResolvedConnectionConfig{
		ID: spec.configID, SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: spec.configName, Kind: spec.configKind, Color: spec.configColor, Mode: "fields", ReadOnly: false,
		Host: &hostCopy, Port: &portCopy, Database: Strp(spec.database), Username: Strp(spec.username),
		Options: spec.options, Password: Strp(spec.password),
	}
	uri := fmt.Sprintf("%s://%s:%s@%s:%d/%s", spec.uriScheme, spec.username, spec.password, host, portNum, spec.database)
	return &mysqlFamilyFixture{URI: uri, Config: cfg, container: container}, nil
}
