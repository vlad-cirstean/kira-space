package dbflow

import (
	"errors"
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

	// The popover's single-line input cannot carry CR/LF, so the UI half sends this text instead.
	uiOps := []model.MutationRowOp{upd(vals("_key", "flow:ttl"), vals("$value", "tab\t\"q\" ünï 😀 "))}
	app.Contract(t, "studio-redis-edit", "mutate:ui-ops", flowharness.MutateWire(conn.ID, db, uiOps), contractMask)

	// The key expires between read and save: the save fails and the key stays gone.
	second.Set(ctx, "flow:gone", "v", 0)
	second.PExpire(ctx, "flow:gone", time.Millisecond)
	testx.WaitUntil(t, 5*time.Second, func() bool { n, _ := second.Exists(ctx, "flow:gone").Result(); return n == 0 })
	_, err = ds.Mutate(conn.ID, db, []model.MutationRowOp{upd(vals("_key", "flow:gone"), vals("$value", "late"))})
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("save of an expired key = %v, want a no-longer-exists failure", err)
	}
	var gone *flowharness.DataError
	if !errors.As(err, &gone) {
		t.Fatalf("expired-key error %T, want *flowharness.DataError", err)
	}
	app.Contract(t, "studio-redis-edit", "mutate:expiredError", gone, contractMask)
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

// redisKeyPath is the node path of a key in db0.
func redisKeyPath(name string) string {
	return model.EncodePath([]model.PathSegment{{Kind: "database", Name: "db0"}, {Kind: "key", Name: name}})
}

// J-KV-010: a hash, list, set and sorted set show their members; the engine edits strings only, so an
// edit of any other type is refused naming the type, changes nothing and keeps the TTL; a hash of
// thousands of fields pages by cursor without losing or repeating a field.
func TestRedisKeyTypesReadAndRefuseEdit(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartRedis(t)
	second := redisClient(t, fx)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)

	second.RPush(ctx, "kt:list", "a", "b", "c")
	second.SAdd(ctx, "kt:set", "m1")
	second.ZAdd(ctx, "kt:zset", goredis.Z{Score: 1.5, Member: "z1"})
	second.HSet(ctx, "kt:hash", "f", "1")
	for _, k := range []string{"kt:list", "kt:set", "kt:zset", "kt:hash"} {
		second.Expire(ctx, k, 300*time.Second)
	}
	for _, c := range []struct{ key, typ string }{{"kt:list", "list"}, {"kt:set", "set"}, {"kt:zset", "zset"}, {"kt:hash", "hash"}} {
		path := redisKeyPath(c.key)
		p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: path})
		if err != nil || p.Kind != "keyvalue" || len(p.Rows) == 0 {
			t.Fatalf("read of %s = %s, %d rows, %v", c.key, p.Kind, len(p.Rows), err)
		}
		_, err = ds.Mutate(conn.ID, "database:db0", []model.MutationRowOp{upd(vals("_key", c.key), vals("$value", "x"))})
		if err == nil || !strings.Contains(err.Error(), c.typ) {
			t.Fatalf("edit of a %s = %v, want a refusal naming %q", c.typ, err, c.typ)
		}
		if c.typ == "hash" {
			var derr *flowharness.DataError
			if !errors.As(err, &derr) {
				t.Fatalf("hash refusal %T, want *flowharness.DataError", err)
			}
			app.Contract(t, "studio-redis-edit", "mutate:typeError", derr, contractMask)
		}
		if ttl, _ := second.TTL(ctx, c.key).Result(); ttl <= 0 {
			t.Fatalf("%s lost its TTL after a refused edit", c.key)
		}
	}
	wantList, _ := second.LRange(ctx, "kt:list", 0, -1).Result()
	if strings.Join(wantList, ",") != "a,b,c" {
		t.Fatalf("list = %v after a refused edit", wantList)
	}

	// A large hash pages by cursor: every field appears exactly once.
	const fields = 5000
	pairs := make([]any, 0, fields*2)
	for i := range fields {
		pairs = append(pairs, "f"+strconv.Itoa(i), "v")
	}
	second.HSet(ctx, "kt:bighash", pairs...)
	path := redisKeyPath("kt:bighash")
	seen := map[string]int{}
	args := flowharness.ReadArgs{ConnectionID: conn.ID, Path: path}
	pages := 0
	for {
		p, _, err := ds.Read(args)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, row := range p.Rows {
			seen[row[0].(string)]++
		}
		if p.NextToken == "" || !p.HasMore {
			break
		}
		args.Cursor = model.PageCursor{Mode: "after", Token: p.NextToken}
		if pages > fields {
			t.Fatal("hash paging did not end")
		}
	}
	for f, n := range seen {
		if n != 1 {
			t.Fatalf("hash field %s returned %d times", f, n)
		}
	}
	if len(seen) != fields || pages < 2 {
		t.Fatalf("paged %d fields over %d pages, want %d fields over several pages", len(seen), pages, fields)
	}
}
