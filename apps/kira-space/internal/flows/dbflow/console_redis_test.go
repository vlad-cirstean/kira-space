package dbflow

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

const consoleRedisScenario = "studio-console-redis"

// J-CONS-025 (contract studio-console-redis): the console's command set, nil reply, tokenizer quoting,
// binary reply marker, refused connection-state commands and server errors.
func TestConsoleRedis(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartRedis(t)
	raw := "a\x00b\xff\xfec"
	if err := redisClient(t, fx).Set(ctx, "cons:bin", raw, 0).Err(); err != nil {
		t.Fatal(err)
	}
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)

	run := func(st string) flowharness.DataPage {
		t.Helper()
		pages, err := ds.Execute(conn.ID, "database:db0", st)
		if err != nil || len(pages) != 1 {
			t.Fatalf("%s: pages=%d err=%v", st, len(pages), err)
		}
		return pages[0]
	}
	rows := map[string][][]any{}
	for key, st := range map[string]string{
		"set":       `SET k v`,
		"get":       `GET k`,
		"nil":       `GET missing`,
		"hset":      `HSET h a 1 b 2`,
		"hgetall":   `HGETALL h`,
		"rpush":     `RPUSH l x y`,
		"lrange":    `LRANGE l 0 -1`,
		"zadd":      `ZADD z 1 m`,
		"zrange":    `ZRANGE z 0 -1 WITHSCORES`,
		"quotedSet": `SET "a b" "x\ny"`,
		"quotedGet": `GET "a b"`,
		"eval":      `EVAL "return {1,{2,'x'}}" 0`,
	} {
		rows[key] = run(st).Rows
	}
	if got := rows["nil"]; len(got) != 1 || got[0][1] != any("(nil)") {
		t.Fatalf("nil reply: %q", got)
	}
	if got := rows["quotedGet"]; len(got) != 1 || got[0][1] != any("x\ny") {
		t.Fatalf("quoted escape: %q", got)
	}

	bin := run(`GET cons:bin`)
	if len(bin.Binary) != 1 || len(bin.Binary[0]) != 2 || bin.Binary[0][0] || !bin.Binary[0][1] {
		t.Fatalf("binary flags: %v", bin.Binary)
	}
	back, err := base64.StdEncoding.DecodeString(bin.Rows[0][1].(string))
	if err != nil || string(back) != raw {
		t.Fatalf("binary round trip: %q err=%v", back, err)
	}

	refused := map[string]map[string]string{}
	for key, st := range map[string]string{
		"select":    `SELECT 2`,
		"multi":     `MULTI`,
		"subscribe": `SUBSCRIBE ch`,
		"unknown":   `NOSUCH`,
		"arity":     `GET`,
		"blpop":     `BLPOP l 0`,
	} {
		_, err := ds.Execute(conn.ID, "database:db0", st)
		var de *flowharness.DataError
		if !errors.As(err, &de) {
			t.Fatalf("%s: want DataError, got %v", st, err)
		}
		refused[key] = map[string]string{"code": de.Code, "message": de.Message}
	}
	if refused["select"]["code"] != "E_UNSUPPORTED" || refused["unknown"]["code"] != "E_QUERY" {
		t.Fatalf("codes: %v", refused)
	}
	app.Contract(t, consoleRedisScenario, "rows", rows)
	app.Contract(t, consoleRedisScenario, "binary", map[string]any{"statement": `GET cons:bin`, "rows": bin.Rows, "binary": bin.Binary})
	app.Contract(t, consoleRedisScenario, "refused", refused)
}
