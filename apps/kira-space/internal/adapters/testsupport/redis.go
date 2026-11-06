package testsupport

import (
	"context"
	"fmt"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// packages/db-fixtures/support/redis.ts's own constants, ported verbatim (C21).
const (
	RedisImage            = "redis:8.10.2"
	RedisPassword         = FixturePassword
	RedisPrimaryDbIndex   = 0
	RedisSecondaryDbIndex = 1
	redisPort             = "6379/tcp"
	redisStartupTimeout   = 60 * time.Second

	// 0004_redis_seed.ts's own constants.
	RedisListKey        = "queue:jobs"
	RedisListLength     = 30
	RedisBigListKey     = "queue:big-jobs"
	RedisBigListLength  = 1200
	RedisSetKey         = "tags:featured"
	RedisZSetKey        = "leaderboard"
	RedisStreamKey      = "events:log"
	RedisHashKey        = "user:1:profile"
	RedisTTLKey         = "session:abc"
	RedisBigHashKey     = "user:1:bighash"
	RedisBigHashLength  = 5000
	RedisBigSetKey      = "tags:big-featured"
	RedisBigSetLength   = 5000
	RedisStreamEntries  = 5
	RedisSecondaryDbKey = "other-db:marker"
)

// RedisSetMembers mirrors 0004_redis_seed.ts's own SET_MEMBERS.
var RedisSetMembers = []string{"red", "green", "blue"}

// RedisHashFields mirrors 0004_redis_seed.ts's own HASH_FIELDS.
var RedisHashFields = map[string]string{"age": "30", "city": "NYC"}

// RedisStreamFields is the field order every seeded stream entry's own XAdd uses below —
// deliberately not alphabetical ("zLast" sorts last, but is added first), so a read path that
// discards XADD's field order and falls back to Go's default map/JSON key sort (alphabetical)
// is caught rather than accidentally matching by coincidence.
var RedisStreamFields = []string{"zLast", "type", "seq"}

// RedisFixture is support/redis.ts's RedisFixture.
type RedisFixture struct {
	Config    model.ResolvedConnectionConfig
	Host      string
	Port      int
	container testcontainers.Container
}

// ConnectionID satisfies hasConnectionID (P107 I2-28, FixtureNodePath's own constraint).
func (f RedisFixture) ConnectionID() string { return f.Config.ID }

var redisMemo = fixture[RedisFixture]{reset: resetRedis}

// StartRedis is support/redis.ts's startRedis. Skips the test when Docker is unreachable.
func StartRedis(t *testing.T) *RedisFixture {
	return redisMemo.start(t, "redis container", startRedis)
}

// StopRedis terminates the memoized container, if one was ever started. Call once, from the test
// binary's own TestMain, after m.Run() returns — never from an individual test.
func StopRedis() {
	redisMemo.stop(func(f *RedisFixture) { _ = f.container.Terminate(context.Background()) })
}

func startRedis() (*RedisFixture, error) {
	ctx := context.Background()

	// modules/redis has no WithPassword option (M7.0's own TC-3-adjacent finding) — the plain
	// image's own --requirepass flag, via GenericContainer, is what support/redis.ts's
	// RedisContainer().withPassword() does under the hood anyway.
	req := testcontainers.ContainerRequest{
		Image:        ImageFor("redis", RedisImage),
		ExposedPorts: []string{redisPort},
		Cmd:          []string{"redis-server", "--requirepass", RedisPassword},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(redisStartupTimeout),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}
	mappedPort, err := container.MappedPort(ctx, redisPort)
	if err != nil {
		return nil, err
	}
	port := int(mappedPort.Num())

	if err := seedRedisDatabases(ctx, host, port); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	cfg := model.ResolvedConnectionConfig{
		ID: "test-redis", SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: "Test Redis", Kind: "redis", Color: "red", Mode: "fields", ReadOnly: false,
		Host: Strp(host), Port: intp(port), Database: Strp("0"), Username: nil,
		Options: map[string]any{}, Password: Strp(RedisPassword),
	}
	return &RedisFixture{Config: cfg, Host: host, Port: port, container: container}, nil
}

func seedRedisDatabases(ctx context.Context, host string, port int) error {
	primary := goredis.NewClient(&goredis.Options{
		Addr: fmt.Sprintf("%s:%d", host, port), Password: RedisPassword, DB: RedisPrimaryDbIndex, Protocol: 2,
	})
	secondary := goredis.NewClient(&goredis.Options{
		Addr: fmt.Sprintf("%s:%d", host, port), Password: RedisPassword, DB: RedisSecondaryDbIndex, Protocol: 2,
	})
	defer primary.Close()
	defer secondary.Close()

	if err := seedRedis(ctx, primary); err != nil {
		return err
	}
	return secondary.Set(ctx, RedisSecondaryDbKey, "present", 0).Err()
}

// resetRedis flushes every database and reseeds.
func resetRedis(f *RedisFixture) error {
	ctx := context.Background()
	admin := goredis.NewClient(&goredis.Options{
		Addr: fmt.Sprintf("%s:%d", f.Host, f.Port), Password: RedisPassword, Protocol: 2,
	})
	defer admin.Close()
	if err := admin.FlushAll(ctx).Err(); err != nil {
		return err
	}
	return seedRedisDatabases(ctx, f.Host, f.Port)
}

// RedisAclUser is a Principal that creates a Redis ACL user with the given rule tokens (bare ACL
// SETUSER tokens, e.g. "~*", "+@all", "-@dangerous") over a side admin connection, torn down when
// the test ends. Optional sugar over ACL SETUSER for the complete matrix's own Redis cases —
// every rule is passed through to the server verbatim, so the matrix table stays the
// source of truth for which ACL shape each case exercises.
func RedisAclUser(name, password string, rules ...string) *Principal {
	return &Principal{
		Name: name,
		Setup: func(t *testing.T, f any) {
			t.Helper()
			fx, ok := f.(*RedisFixture)
			if !ok {
				t.Fatalf("RedisAclUser: fixture is %T, want *RedisFixture", f)
			}
			admin := goredis.NewClient(&goredis.Options{
				Addr: fmt.Sprintf("%s:%d", fx.Host, fx.Port), Password: RedisPassword, Protocol: 2,
			})
			defer admin.Close()
			args := append([]any{"ACL", "SETUSER", name, "on", ">" + password}, rulesToAny(rules)...)
			if err := admin.Do(context.Background(), args...).Err(); err != nil {
				t.Fatalf("ACL SETUSER %s: %v", name, err)
			}
			t.Cleanup(func() {
				side := goredis.NewClient(&goredis.Options{
					Addr: fmt.Sprintf("%s:%d", fx.Host, fx.Port), Password: RedisPassword, Protocol: 2,
				})
				defer side.Close()
				_ = side.Do(context.Background(), "ACL", "DELUSER", name).Err()
			})
		},
	}
}

func rulesToAny(rules []string) []any {
	out := make([]any, len(rules))
	for i, r := range rules {
		out[i] = r
	}
	return out
}

func seedRedis(ctx context.Context, conn *goredis.Client) error {
	// A root-level key with no ':' — namespace splitting must still surface it as a 'key' leaf
	// directly under the db, not swallow it while walking namespaces.
	if err := conn.Set(ctx, "counter", "42", 0).Err(); err != nil {
		return err
	}

	if err := conn.Set(ctx, RedisTTLKey, "token-abc", 0).Err(); err != nil {
		return err
	}
	if err := conn.Expire(ctx, RedisTTLKey, 10_000*time.Second).Err(); err != nil {
		return err
	}

	if err := conn.Set(ctx, "user:1:name", "Alice", 0).Err(); err != nil {
		return err
	}
	if err := conn.Set(ctx, "user:1:email", "alice@example.com", 0).Err(); err != nil {
		return err
	}
	if err := conn.HSet(ctx, RedisHashKey, RedisHashFields).Err(); err != nil {
		return err
	}
	if err := conn.Set(ctx, "user:2:name", "Bob", 0).Err(); err != nil {
		return err
	}

	bigHashFields := make(map[string]string, RedisBigHashLength)
	for i := 0; i < RedisBigHashLength; i++ {
		bigHashFields[fmt.Sprintf("f%d", i)] = fmt.Sprintf("v%d", i)
	}
	if err := conn.HSet(ctx, RedisBigHashKey, bigHashFields).Err(); err != nil {
		return err
	}

	jobs := make([]any, RedisListLength)
	for i := range jobs {
		jobs[i] = fmt.Sprintf("job-%d", i)
	}
	if err := conn.RPush(ctx, RedisListKey, jobs...).Err(); err != nil {
		return err
	}

	bigJobs := make([]any, RedisBigListLength)
	for i := range bigJobs {
		bigJobs[i] = fmt.Sprintf("big-job-%d", i)
	}
	if err := conn.RPush(ctx, RedisBigListKey, bigJobs...).Err(); err != nil {
		return err
	}

	setMembers := make([]any, len(RedisSetMembers))
	for i, m := range RedisSetMembers {
		setMembers[i] = m
	}
	if err := conn.SAdd(ctx, RedisSetKey, setMembers...).Err(); err != nil {
		return err
	}

	bigSetMembers := make([]any, RedisBigSetLength)
	for i := range bigSetMembers {
		bigSetMembers[i] = fmt.Sprintf("member-%d", i)
	}
	if err := conn.SAdd(ctx, RedisBigSetKey, bigSetMembers...).Err(); err != nil {
		return err
	}

	zMembers := []goredis.Z{
		{Score: 10, Member: "alice"},
		{Score: 20, Member: "bob"},
		{Score: 30, Member: "carol"},
	}
	if err := conn.ZAdd(ctx, RedisZSetKey, zMembers...).Err(); err != nil {
		return err
	}

	for i := 0; i < RedisStreamEntries; i++ {
		// A []interface{} Values, not a map, deliberately: XAddArgs.Values as a map does not
		// preserve field order (go-redis's own documented caveat), so a map here would defeat the
		// point of RedisStreamFields below, which exists to catch read.go relabelling this
		// (deliberately non-alphabetical) order back to alphabetical.
		if err := conn.XAdd(ctx, &goredis.XAddArgs{
			Stream: RedisStreamKey, ID: "*",
			Values: []interface{}{"zLast", "z-value", "type", "click", "seq", fmt.Sprintf("%d", i)},
		}).Err(); err != nil {
			return err
		}
	}
	return nil
}
