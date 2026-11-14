package dbflow

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func redisClient(t *testing.T, fx *testsupport.RedisFixture) *goredis.Client {
	t.Helper()
	c := goredis.NewClient(&goredis.Options{Addr: net.JoinHostPort(fx.Host, strconv.Itoa(fx.Port)), Password: *fx.Config.Password})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func redisSet(key, value string) model.RowValues { return vals("_key", key, "$value", value) }

// J-KV-010 (contract studio-redis-edit): saving a string keeps its TTL, writes the exact bytes, and
// never recreates a key that expired or was deleted after the user read it.
func TestRedisEditKeepsTTLAndNeverResurrects(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartRedis(t)
	second := redisClient(t, fx)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	const db = "database:db0"

	exact := "line1\r\nline2\t\"q\" ünï 😀 "
	second.Set(ctx, "flow:ttl", "old", 300*time.Second)
	ops := []model.MutationRowOp{upd(vals("_key", "flow:ttl"), vals("$value", exact))}
	app.Contract(t, "studio-redis-edit", "mutate:ops", flowharness.MutateWire(conn.ID, db, ops), contractMask)
	stmts, err := ds.Preview(conn.ID, db, ops)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "studio-redis-edit", "preview:statements", stmts, contractMask)
	if _, err := ds.Mutate(conn.ID, db, ops); err != nil {
		t.Fatal(err)
	}
	if got, _ := second.Get(ctx, "flow:ttl").Result(); got != exact {
		t.Fatalf("value = %q, want exact bytes", got)
	}
	if ttl, _ := second.TTL(ctx, "flow:ttl").Result(); ttl <= 0 || ttl > 300*time.Second {
		t.Fatalf("TTL after edit = %v, want it kept", ttl)
	}

	// The key expires between read and save: the save fails and the key stays gone.
	second.Set(ctx, "flow:gone", "v", 0)
	second.PExpire(ctx, "flow:gone", time.Millisecond)
	testx.WaitUntil(t, 5*time.Second, func() bool { n, _ := second.Exists(ctx, "flow:gone").Result(); return n == 0 })
	_, err = ds.Mutate(conn.ID, db, []model.MutationRowOp{upd(vals("_key", "flow:gone"), vals("$value", "late"))})
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("save of an expired key = %v, want a no-longer-exists failure", err)
	}
	if n, _ := second.Exists(ctx, "flow:gone").Result(); n != 0 {
		t.Fatal("the save resurrected an expired key")
	}

	// Insert never overwrites; editing a hash is outside the editable scope and changes nothing.
	if _, err := ds.Mutate(conn.ID, db, []model.MutationRowOp{ins(redisSet("flow:ttl", "x"))}); err == nil {
		t.Fatal("insert over an existing key succeeded")
	}
	second.HSet(ctx, "flow:hash", "f", "1")
	if _, err := ds.Mutate(conn.ID, db, []model.MutationRowOp{upd(vals("_key", "flow:hash"), vals("$value", "x"))}); err == nil {
		t.Fatal("update of a hash key succeeded")
	}
	if got, _ := second.HGet(ctx, "flow:hash", "f").Result(); got != "1" {
		t.Fatalf("hash field = %q after a refused edit", got)
	}
}
