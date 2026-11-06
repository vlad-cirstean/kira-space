// Ported from packages/db-fixtures/redis.spec.ts, case by case where practical — the spec's own
// numbering is kept in each test's name so the two can be diffed. docs/HISTORY.md names the cases
// that carry the most weight: hash/set/zset paging asserting "some rows, more to come" rather
// than an exact count where overshoot is possible, the offset-cursor-restarts-the-scan contract,
// a vanished key being E_QUERY not E_NOT_FOUND, truncated only when the round cap cut a scan short,
// children() of a key leaf marshalling as [] not null, and cancel being a permanent, honest
// no-op.
package redis_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	redisadapter "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/redis"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.StopRedis()
	os.Exit(code)
}

var (
	deps           = adapters.Deps{Log: func(level, message string) {}}
	regexpRedisVer = testsupport.VersionPattern("Redis", testsupport.RedisServerMajor())
	seg            = testsupport.Seg
	childNames     = testsupport.ChildNames
	containsName   = testsupport.ContainsName
	kvPairs        = testsupport.KVPairs
	kvValueAt      = testsupport.KVValueAt
	kvFieldAt      = testsupport.KVFieldAt
)

func newAdapter(t *testing.T) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter("redis", deps)
	if err != nil {
		t.Fatalf("CreateAdapter: %v", err)
	}
	return a
}

func connectedAdapter(t *testing.T, fixture *testsupport.RedisFixture) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

// nodePath is testsupport.FixtureNodePath instantiated for *RedisFixture (P107 I2-28).
var nodePath = testsupport.FixtureNodePath[*testsupport.RedisFixture]

// 1. connect / disconnect
func TestRedis_ConnectDisconnect(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := newAdapter(t)

	info, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("op-1"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !regexpRedisVer.MatchString(info.ServerVersion) {
		t.Errorf("ServerVersion = %q, want to match %s", info.ServerVersion, regexpRedisVer)
	}
	if info.Details["database"] != "db0" {
		t.Errorf("Details[database] = %q, want db0", info.Details["database"])
	}
	if err := a.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
}

// 2 (re-baselined against the Go driver): wrong password is E_AUTH.
func TestRedis_Connect_WrongPasswordIsAuthError(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	badCfg := fixture.Config
	badPassword := "not-the-password"
	badCfg.Password = &badPassword

	a := newAdapter(t)
	_, err := a.Connect(context.Background(), badCfg, adapters.NewOpCtx("op-2"))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeAuth {
		t.Errorf("code = %v, want E_AUTH", code)
	}
}

// P25 §1.3/§2.1(3): a least-privilege ACL user (~* +@all -@dangerous — the single most commonly
// recommended application ACL) must be able to connect at all. Pins INFO's non-fatal handling by
// asserting the *degraded* "Redis unknown" version string, not merely that Connect succeeds.
func TestRedis_Connect_LeastPrivilegeAclUser(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	admin := goredis.NewClient(&goredis.Options{
		Addr: fmt.Sprintf("%s:%d", fixture.Host, fixture.Port), Password: testsupport.RedisPassword, Protocol: 2,
	})
	defer admin.Close()

	const username, password = "p25_least_priv", "p25_pw"
	if err := admin.Do(context.Background(), "ACL", "SETUSER", username, "on", ">"+password,
		"~*", "+@all", "-@dangerous").Err(); err != nil {
		t.Fatalf("ACL SETUSER: %v", err)
	}
	t.Cleanup(func() { _ = admin.Do(context.Background(), "ACL", "DELUSER", username).Err() })

	cfg := fixture.Config
	cfg.Username = testsupport.Strp(username)
	cfg.Password = testsupport.Strp(password)

	a := newAdapter(t)
	info, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-acl"))
	if err != nil {
		t.Fatalf("Connect as ~* +@all -@dangerous: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	if info.ServerVersion != "Redis unknown" {
		t.Errorf("ServerVersion = %q, want %q (INFO must be non-fatal for a user without @dangerous)",
			info.ServerVersion, "Redis unknown")
	}
}

// 3. tree: databases, numeric sort, and the exact root namespace/key listing.
func TestRedis_Children_Databases(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(), nodePath(fixture), adapters.NewOpCtx("op-3"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	names := childNames(t, children)
	if !containsName(names, "db0") || !containsName(names, "db1") {
		t.Errorf("names = %v, want both db0 and db1", names)
	}
}

func TestRedis_Children_RootNamespacesAndKeys(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(), nodePath(fixture, seg("database", "db0")), adapters.NewOpCtx("op-4"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	var namespaces, keys []string
	for _, n := range children.Nodes {
		if n.Kind == "namespace" {
			namespaces = append(namespaces, n.Name)
		} else {
			keys = append(keys, n.Name)
		}
	}
	wantNamespaces := []string{"events", "queue", "session", "tags", "user"}
	for _, w := range wantNamespaces {
		if !containsName(namespaces, w) {
			t.Errorf("namespaces = %v, want to contain %q", namespaces, w)
		}
	}
	wantKeys := []string{"counter", "leaderboard"}
	for _, w := range wantKeys {
		if !containsName(keys, w) {
			t.Errorf("keys = %v, want to contain %q", keys, w)
		}
	}
}

// children() of a key leaf marshals as "nodes":[] (C16), never null.
func TestRedis_Children_KeyIsLeaf(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(),
		nodePath(fixture, seg("database", "db0"), seg("key", "counter")),
		adapters.NewOpCtx("op-5"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if children.Nodes == nil {
		t.Fatal("Nodes is nil, want an empty non-nil slice")
	}
	b, err := json.Marshal(children)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !regexp.MustCompile(`"Nodes":\[\]`).Match(b) {
		t.Errorf("marshalled = %s, want a \"Nodes\":[] field, not null", b)
	}
}

// 8/9/10: hash/set/zset paging asserts "some rows, more to come" for the big hash, and the exact
// set of pairs for the small ones — never an exact row count on the paged case, since HSCAN's
// COUNT is a hint and readScanFamily can overshoot.
func TestRedis_Read_Hash_Small(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisHashKey))

	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-8"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	pairs := kvPairs(t, kv)
	for field, value := range testsupport.RedisHashFields {
		if pairs[field] != value {
			t.Errorf("pairs[%q] = %q, want %q", field, pairs[field], value)
		}
	}
}

func TestRedis_Read_Hash_Big(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisBigHashKey))

	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 50, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-9"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	if kv.RowCount == 0 {
		t.Fatal("RowCount = 0, want > 0")
	}
	if !kv.Position.HasMore {
		t.Error("HasMore = false, want true (5000-field hash, page size 50)")
	}
	if kv.Position.NextToken == nil {
		t.Fatal("NextToken is nil, want a cursor token")
	}

	// An offset cursor on this cursor-paged key restarts the scan rather than seeking or
	// erroring (redis 24) — the second read must succeed and simply start over from cursor 0.
	p2, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 50, Cursor: model.PageCursor{Mode: "offset", Offset: 40},
	}, adapters.NewOpCtx("op-9b"))
	if err != nil {
		t.Fatalf("Read (offset cursor on a cursor-paged key): %v", err)
	}
	kv2 := p2.(page.KeyValuePage)
	if kv2.RowCount == 0 {
		t.Error("restart read: RowCount = 0, want > 0")
	}
}

func TestRedis_Read_Set(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisSetKey))

	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-10"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	if kv.RowCount != len(testsupport.RedisSetMembers) {
		t.Fatalf("RowCount = %d, want %d", kv.RowCount, len(testsupport.RedisSetMembers))
	}
	seen := map[string]bool{}
	for i := 0; i < kv.RowCount; i++ {
		v := kvValueAt(t, kv, i)
		if v != nil {
			seen[*v] = true
		}
	}
	for _, m := range testsupport.RedisSetMembers {
		if !seen[m] {
			t.Errorf("set members = %v, missing %q", seen, m)
		}
	}
}

// Regression: a set's per-row display index (its only "field" — a member has no natural
// key) is a running total carried across pages, not reset to 0 each call. Without the fix, page 2
// relabels its rows starting at "0" again, duplicating page 1's own labels instead of continuing
// from where page 1 left off.
func TestRedis_Read_Set_Big_RowNumberingContinuesAcrossPages(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisBigSetKey))

	p1, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 50, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-set-big-1"))
	if err != nil {
		t.Fatalf("Read page 1: %v", err)
	}
	kv1 := p1.(page.KeyValuePage)
	if kv1.RowCount == 0 {
		t.Fatal("page 1: RowCount = 0, want > 0")
	}
	if !kv1.Position.HasMore || kv1.Position.NextToken == nil {
		t.Fatalf("page 1: HasMore = %v, NextToken = %v, want a truncated first page (5000-member set, page size 50)", kv1.Position.HasMore, kv1.Position.NextToken)
	}
	lastLabel1, err := strconv.Atoi(*kvFieldAt(t, kv1, kv1.RowCount-1))
	if err != nil {
		t.Fatalf("page 1: last row label not numeric: %v", err)
	}

	p2, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 50, Cursor: model.PageCursor{Mode: "after", Token: *kv1.Position.NextToken},
	}, adapters.NewOpCtx("op-set-big-2"))
	if err != nil {
		t.Fatalf("Read page 2: %v", err)
	}
	kv2 := p2.(page.KeyValuePage)
	if kv2.RowCount == 0 {
		t.Fatal("page 2: RowCount = 0, want > 0")
	}
	firstLabel2, err := strconv.Atoi(*kvFieldAt(t, kv2, 0))
	if err != nil {
		t.Fatalf("page 2: first row label not numeric: %v", err)
	}
	if firstLabel2 != lastLabel1+1 {
		t.Errorf("page 2's first row label = %d, want %d (continuing from page 1's last label %d, not restarting)", firstLabel2, lastLabel1+1, lastLabel1)
	}
}

func TestRedis_Read_ZSet(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisZSetKey))

	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-11"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	pairs := kvPairs(t, kv)
	want := map[string]string{"alice": "10", "bob": "20", "carol": "30"}
	for member, score := range want {
		if pairs[member] != score {
			t.Errorf("pairs[%q] = %q, want %q", member, pairs[member], score)
		}
	}
}

// list: offset paging honours the requested page size in full — no LIST_WINDOW clamp (P43 iter2
// D25/F18).
func TestRedis_Read_List_NoWindowClamp(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisBigListKey))

	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 1000, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-12"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	if kv.RowCount != 1000 {
		t.Fatalf("RowCount = %d, want 1000 (the deleted LIST_WINDOW clamp must not come back)", kv.RowCount)
	}
	if !kv.Position.HasMore {
		t.Error("HasMore = false, want true (1200-element list, page size 1000)")
	}
}

// stream: XRANGE with the +1 probe.
func TestRedis_Read_Stream(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisStreamKey))

	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-13"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	if kv.RowCount != testsupport.RedisStreamEntries {
		t.Fatalf("RowCount = %d, want %d", kv.RowCount, testsupport.RedisStreamEntries)
	}
	body := kvValueAt(t, kv, 0)
	if body == nil {
		t.Fatal("body is nil")
	}
	var fields map[string]string
	if err := json.Unmarshal([]byte(*body), &fields); err != nil {
		t.Fatalf("Unmarshal stream entry body: %v", err)
	}
	if fields["type"] != "click" {
		t.Errorf("fields[type] = %q, want click", fields["type"])
	}

	// P2 R1 regression: the seed fixture's own XAdd order is "zLast", "type", "seq" (deliberately
	// not alphabetical) — a read path that discards that order and falls back to a Go map/JSON
	// key sort would instead emit "seq", "type", "zLast", failing this position check.
	var positions []int
	for _, field := range testsupport.RedisStreamFields {
		pos := strings.Index(*body, `"`+field+`"`)
		if pos < 0 {
			t.Fatalf("body = %s, missing field %q", *body, field)
		}
		positions = append(positions, pos)
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] < positions[i-1] {
			t.Errorf("body = %s, want field order %v (XADD's own order), not re-sorted", *body, testsupport.RedisStreamFields)
			break
		}
	}
}

// string type + TTL surfaced.
func TestRedis_Read_StringWithTTL(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisTTLKey))

	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-14"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	if kv.RedisType != "string" {
		t.Errorf("RedisType = %q, want string", kv.RedisType)
	}
	if kv.TTLMs == nil || *kv.TTLMs <= 0 {
		t.Errorf("TTLMs = %v, want a positive value", kv.TTLMs)
	}
	v := kvValueAt(t, kv, 0)
	if v == nil || *v != "token-abc" {
		t.Errorf("value = %v, want token-abc", v)
	}
}

// 15: a vanished key is E_QUERY, not E_NOT_FOUND (P9 D10) — the wrong code gates a tab behind
// "Reconnect & load" for a key that merely expired.
func TestRedis_Read_VanishedKeyIsQueryError(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", "does-not-exist"))

	_, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-15"))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY (not E_NOT_FOUND)", code)
	}
}

// count: exact via O(1) type-length commands.
func TestRedis_Count(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db0"), seg("key", testsupport.RedisListKey))

	result, err := a.Count(context.Background(), adapters.CountRequest{Path: path}, adapters.NewOpCtx("op-16"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if !result.Exact {
		t.Error("Exact = false, want true")
	}
	if result.Value != testsupport.RedisListLength {
		t.Errorf("Value = %d, want %d", result.Value, testsupport.RedisListLength)
	}
}

// mutate: insert (SET NX) / update (SET) / delete (DEL), against db index 1 (C23 — every mutating
// test runs against the secondary db, never db0, whose exact root listing scenario 3 asserts).
func TestRedis_Mutate_InsertUpdateDelete(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db1"))

	insertPlan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind: "insert",
		Values: model.RowValues{
			{Name: "_key", Value: strp("mutate:probe")},
			{Name: "$value", Value: strp("hello")},
		},
	}}}
	preview, err := a.Preview(insertPlan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(preview) != 1 || preview[0] != "SET mutate:probe hello NX" {
		t.Errorf("Preview = %v", preview)
	}
	result, err := a.Mutate(context.Background(), insertPlan, adapters.NewOpCtx("op-17"))
	if err != nil {
		t.Fatalf("Mutate (insert): %v", err)
	}
	if result.AffectedRows != 1 {
		t.Fatalf("AffectedRows = %d, want 1", result.AffectedRows)
	}

	// Inserting the same key again must fail (NX).
	_, err = a.Mutate(context.Background(), insertPlan, adapters.NewOpCtx("op-17b"))
	if err == nil {
		t.Fatal("second insert: want an error (key already exists), got nil")
	}

	updatePlan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind: "update",
		Key:  model.RowValues{{Name: "_key", Value: strp("mutate:probe")}},
		Changes: model.RowValues{
			{Name: "$value", Value: strp("updated")},
		},
	}}}
	if _, err := a.Mutate(context.Background(), updatePlan, adapters.NewOpCtx("op-18")); err != nil {
		t.Fatalf("Mutate (update): %v", err)
	}
	readPath := nodePath(fixture, seg("database", "db1"), seg("key", "mutate:probe"))
	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: readPath, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-18b"))
	if err != nil {
		t.Fatalf("Read after update: %v", err)
	}
	v := kvValueAt(t, p.(page.KeyValuePage), 0)
	if v == nil || *v != "updated" {
		t.Errorf("value after update = %v, want updated", v)
	}

	// P2 R1: editing a value must not silently clear the key's existing TTL — a plain SET
	// (without KEEPTTL) would reset it to none.
	side := goredis.NewClient(&goredis.Options{
		Addr: fixture.Host + ":" + strconv.Itoa(fixture.Port), Password: testsupport.RedisPassword,
		DB: testsupport.RedisSecondaryDbIndex, Protocol: 2,
	})
	defer side.Close()
	if err := side.Expire(context.Background(), "mutate:probe", time.Hour).Err(); err != nil {
		t.Fatalf("seed TTL: %v", err)
	}
	ttlPlan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind: "update",
		Key:  model.RowValues{{Name: "_key", Value: strp("mutate:probe")}},
		Changes: model.RowValues{
			{Name: "$value", Value: strp("updated again")},
		},
	}}}
	if _, err := a.Mutate(context.Background(), ttlPlan, adapters.NewOpCtx("op-18c")); err != nil {
		t.Fatalf("Mutate (update, with TTL): %v", err)
	}
	ttl, err := side.TTL(context.Background(), "mutate:probe").Result()
	if err != nil {
		t.Fatalf("TTL after update: %v", err)
	}
	if ttl <= 0 {
		t.Errorf("TTL after update = %v, want it preserved (> 0)", ttl)
	}

	deletePlan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind: "delete",
		Key:  model.RowValues{{Name: "_key", Value: strp("mutate:probe")}},
	}}}
	deleteResult, err := a.Mutate(context.Background(), deletePlan, adapters.NewOpCtx("op-19"))
	if err != nil {
		t.Fatalf("Mutate (delete): %v", err)
	}
	if deleteResult.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", deleteResult.AffectedRows)
	}
}

func strp(s string) *string { return &s }

// mutate: editing a non-string-type key is rejected server-side too (§8.12's standard).
func TestRedis_Mutate_NonStringTypeUneditable(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db1"))

	// A real list key, seeded directly into db1 (C23) — mutate.go itself has no way to create a
	// non-string-typed key, so the type this test needs to reject has to come from a side client.
	const listKey = "mutate:list-probe"
	side := goredis.NewClient(&goredis.Options{
		Addr: fixture.Host + ":" + strconv.Itoa(fixture.Port), Password: testsupport.RedisPassword,
		DB: testsupport.RedisSecondaryDbIndex, Protocol: 2,
	})
	defer side.Close()
	if err := side.RPush(context.Background(), listKey, "a", "b").Err(); err != nil {
		t.Fatalf("seed list key: %v", err)
	}

	updatePlan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind: "update",
		Key:  model.RowValues{{Name: "_key", Value: strp(listKey)}},
		Changes: model.RowValues{
			{Name: "$value", Value: strp("nope")},
		},
	}}}
	_, err := a.Mutate(context.Background(), updatePlan, adapters.NewOpCtx("op-20"))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeUnsupported {
		t.Errorf("code = %v, want E_UNSUPPORTED", code)
	}
}

// 21: cancel is a permanent no-op, and honest about it — caps.Cancel is true anyway (C9).
func TestRedis_Cancel_PermanentNoOp(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)

	killed, err := a.Cancel(context.Background(), "any-op-id")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if killed {
		t.Error("Cancel returned true, want false (permanent no-op, D7/D8)")
	}
	if !a.Caps().Cancel {
		t.Error("Caps().Cancel = false, want true")
	}
}

// console: a generic command dispatch through client.Do.
func TestRedis_Console_Execute(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db1"))

	pages, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path:       path,
		Statements: []string{"SET console:probe hello", "GET console:probe"},
	}, adapters.NewOpCtx("op-22"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("len(pages) = %d, want 2", len(pages))
	}
	getPage, ok := pages[1].(page.KeyValuePage)
	if !ok {
		t.Fatalf("pages[1] = %T, want page.KeyValuePage", pages[1])
	}
	v := kvValueAt(t, getPage, 0)
	if v == nil || *v != "hello" {
		t.Errorf("GET result = %v, want hello", v)
	}
}

// P174: collection reads stop at the cap through their bounded forms (SCAN, HSCAN, clamped LRANGE).
func TestRedis_Console_CollectionReadsStopAtResultCap(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", "db1"))
	if _, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path: path,
		Statements: []string{
			"RPUSH cap:list 1 2 3 4 5 6", "HSET cap:hash a 1 b 2 c 3 d 4 e 5 f 6",
			"SET cap:k1 x", "SET cap:k2 x", "SET cap:k3 x", "SET cap:k4 x", "SET cap:k5 x", "SET cap:k6 x",
		},
	}, adapters.NewOpCtx("op-cap-seed")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, stmt := range []string{"LRANGE cap:list 0 -1", "HGETALL cap:hash", "KEYS cap:k*"} {
		for _, tc := range []struct {
			limit     int
			rows      int
			truncated bool
		}{{5, 5, true}, {6, 6, false}} {
			pages, err := a.Execute(context.Background(), model.ConsoleRequest{
				Path: path, Statements: []string{stmt}, Cap: page.ResultCap{Rows: tc.limit},
			}, adapters.NewOpCtx("op-cap"))
			if err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
			p := pages[0].(page.KeyValuePage)
			if p.RowCount != tc.rows || p.Position.Truncated != tc.truncated {
				t.Errorf("%s cap %d: RowCount = %d, Truncated = %v, want %d, %v", stmt, tc.limit, p.RowCount, p.Position.Truncated, tc.rows, tc.truncated)
			}
		}
	}
}

// Redis is the only adapter whose read-only enforcement is per-command, resolved
// against the server's own COMMAND table (client.go's isReadOnlyCommand, console.go:132) — a
// genuinely non-obvious mechanism with no container-backed test at all before this (console_test.go
// is 70 lines of tokenizer unit tests only).
func TestRedis_ReadOnlyConnection_ConsoleAllowsReadsRefusesWrites(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	ro := fixture.Config
	ro.ReadOnly = true
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), ro, adapters.NewOpCtx("op-ro-connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer a.Disconnect(context.Background())

	path := nodePath(fixture, seg("database", "db0"))
	getPages, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path: path, Statements: []string{"GET counter"},
	}, adapters.NewOpCtx("op-ro-get"))
	if err != nil {
		t.Fatalf("Execute(GET) on a read-only connection: %v", err)
	}
	getPage, ok := getPages[0].(page.KeyValuePage)
	if !ok {
		t.Fatalf("pages[0] = %T, want page.KeyValuePage", getPages[0])
	}
	if v := kvValueAt(t, getPage, 0); v == nil || *v != "42" {
		t.Errorf("GET counter = %v, want 42", v)
	}

	_, err = a.Execute(context.Background(), model.ConsoleRequest{
		Path: path, Statements: []string{"SET counter 999"},
	}, adapters.NewOpCtx("op-ro-set"))
	if err == nil {
		t.Fatal("Execute(SET) on a read-only connection: want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeUnsupported {
		t.Errorf("code = %v, want E_UNSUPPORTED", code)
	}

	afterPages, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path: path, Statements: []string{"GET counter"},
	}, adapters.NewOpCtx("op-ro-get-after"))
	if err != nil {
		t.Fatalf("Execute(GET) after refused SET: %v", err)
	}
	afterPage := afterPages[0].(page.KeyValuePage)
	if v := kvValueAt(t, afterPage, 0); v == nil || *v != "42" {
		t.Errorf("GET counter after refused SET = %v, want unchanged 42", v)
	}
}

// P26 §3.6(2): Redis's implicit-creation analogue and the only place its four surfaces (console,
// tree, read, mutate) are asserted to agree with each other.
func TestRedis_ConsoleCreatedKeyAppearsInTreeAndReads(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	const key = "p26scratch"

	side := goredis.NewClient(&goredis.Options{
		Addr: fixture.Host + ":" + strconv.Itoa(fixture.Port), Password: testsupport.RedisPassword,
		DB: testsupport.RedisPrimaryDbIndex, Protocol: 2,
	})
	// Registered first, so its LIFO position is last: the DEL cleanup below must run against a
	// still-open connection — a plain `defer side.Close()` would close it before t.Cleanup
	// callbacks even start (postgres_test.go's own pattern).
	t.Cleanup(func() { _ = side.Close() })
	t.Cleanup(func() { _ = side.Del(context.Background(), key).Err() })

	if _, err := a.Execute(ctx, model.ConsoleRequest{
		Path: nodePath(fixture, seg("database", "db0")), Statements: []string{"HSET p26scratch a 1"},
	}, adapters.NewOpCtx("op-create")); err != nil {
		t.Fatalf("Execute(HSET): %v", err)
	}

	children, err := a.Children(ctx, nodePath(fixture, seg("database", "db0")), adapters.NewOpCtx("op-children"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if !containsName(childNames(t, children), key) {
		t.Fatalf("Children(db0) = %v, want %s present", childNames(t, children), key)
	}

	read, err := a.Read(ctx, adapters.ReadRequest{
		Path: nodePath(fixture, seg("database", "db0"), seg("key", key)), PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-read"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kvPage := read.(page.KeyValuePage)
	pairs := kvPairs(t, kvPage)
	if pairs["a"] != "1" {
		t.Errorf("pairs = %v, want a=1", pairs)
	}

	deletePlan := model.MutationPlan{
		Path: nodePath(fixture, seg("database", "db0")),
		Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "_key", Value: strp(key)}}}},
	}
	if _, err := a.Mutate(ctx, deletePlan, adapters.NewOpCtx("op-delete")); err != nil {
		t.Fatalf("Mutate(delete): %v", err)
	}
	childrenAfter, err := a.Children(ctx, nodePath(fixture, seg("database", "db0")), adapters.NewOpCtx("op-children-after"))
	if err != nil {
		t.Fatalf("Children after delete: %v", err)
	}
	if containsName(childNames(t, childrenAfter), key) {
		t.Errorf("Children(db0) after delete = %v, must not contain %s", childNames(t, childrenAfter), key)
	}
}

// caps honesty.
func TestRedis_Caps(t *testing.T) {
	a := newAdapter(t)
	c := a.Caps()
	if !c.KeyValue || c.Tabular || c.Documents || c.Stream {
		t.Errorf("Caps page-kind flags = %+v, want keyValue-only", c)
	}
	if c.Pagination != adapters.PaginationCursor {
		t.Errorf("Pagination = %v, want cursor", c.Pagination)
	}
	if !c.ExactCount {
		t.Error("ExactCount = false, want true")
	}
	if c.Definition || c.Describe {
		t.Error("Definition/Describe = true, want both false")
	}
	if c.SchemaColumns {
		t.Error("SchemaColumns = true, want false (a redis key has no field-level schema)")
	}
}

// P22c §4.1: the non-SQL kinds report the capability false and the method is not implemented.
func TestRedis_SchemaColumns_Unsupported(t *testing.T) {
	a := newAdapter(t)
	_, err := a.SchemaColumns(context.Background(), model.NodePath{}, adapters.NewOpCtx("op-schema-columns"))
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
		t.Fatalf("got %v, want E_UNSUPPORTED", err)
	}
}

func consoleRun(a adapters.Adapter, fixture *testsupport.RedisFixture, ctx context.Context, opID string, statements ...string) ([]page.Page, error) {
	return a.Execute(ctx, model.ConsoleRequest{Path: nodePath(fixture, seg("database", "db0")), Statements: statements}, adapters.NewOpCtx(opID))
}

func calibratedLoopIterations(t *testing.T, root *goredis.Client, target time.Duration) int64 {
	t.Helper()
	const calib = 50_000_000
	start := time.Now()
	if err := root.Eval(context.Background(), `local x = 0; for i = 1, tonumber(ARGV[1]) do x = x + i end; return 0`, nil, calib).Err(); err != nil {
		t.Fatalf("calibration EVAL: %v", err)
	}
	elapsed := max(time.Since(start), time.Millisecond)
	return int64(float64(calib) * target.Seconds() / elapsed.Seconds())
}

// F9: a reply slower than the read timeout is reported as E_TIMEOUT and never re-sent (a retried
// script would INCR twice).
func TestRedis_Console_SlowCommandTimesOutWithoutResend(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)
	defer redisadapter.SetConsoleReadTimeout(time.Second)()
	root := goredis.NewClient(&goredis.Options{Addr: fmt.Sprintf("%s:%d", fixture.Host, fixture.Port), Password: testsupport.RedisPassword})
	defer root.Close()
	const key = "slow:resend:probe"
	root.Del(context.Background(), key)

	// Loop length is calibrated to this server's speed; TIME is frozen inside a script.
	const target = 3 * time.Second
	iters := calibratedLoopIterations(t, root, target)
	script := `local x = 0; for i = 1, tonumber(ARGV[1]) do x = x + i end; return redis.call('INCR', KEYS[1])`
	_, err := consoleRun(a, fixture, context.Background(), "op-slow", fmt.Sprintf("EVAL %q 1 %s %d", script, key, iters))
	if code, _ := adapters.CodeOf(err); code != adapters.CodeTimeout {
		t.Fatalf("err = %v, want E_TIMEOUT", err)
	}
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if root.Exists(context.Background(), key).Val() == 1 {
			break
		}
	}
	time.Sleep(target + time.Second) // a re-sent script would land a second INCR
	if n, err := root.Get(context.Background(), key).Int(); err != nil || n != 1 {
		t.Fatalf("script ran %d times (err %v), want exactly once", n, err)
	}
}

// F9: blocking commands honour their own timeout, an unbounded block is refused, and Stop returns
// at once instead of waiting out the socket deadline.
func TestRedis_Console_BlockingCommands(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	a := connectedAdapter(t, fixture)

	if _, err := consoleRun(a, fixture, context.Background(), "op-push", "RPUSH blk:q x"); err != nil {
		t.Fatalf("RPUSH: %v", err)
	}
	if _, err := consoleRun(a, fixture, context.Background(), "op-blpop", "BLPOP blk:q 1"); err != nil {
		t.Fatalf("BLPOP with a 1s timeout: %v", err)
	}
	_, err := consoleRun(a, fixture, context.Background(), "op-blpop0", "BLPOP blk:nokey 0")
	if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Fatalf("BLPOP 0: err = %v, want E_QUERY", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(500 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err = consoleRun(a, fixture, ctx, "op-blpop-stop", "BLPOP blk:nokey 30")
	if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled {
		t.Fatalf("stopped BLPOP: err = %v, want E_CANCELLED", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Stop took %v to return", elapsed)
	}
}

// F10: container commands are classified by their subcommand's own flags.
func TestRedis_ReadOnlyConnection_ContainerSubcommands(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	ro := fixture.Config
	ro.ReadOnly = true
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), ro, adapters.NewOpCtx("op-ro-container")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer a.Disconnect(context.Background())

	for _, stmt := range []string{"OBJECT ENCODING counter", "MEMORY USAGE counter"} {
		if _, err := consoleRun(a, fixture, context.Background(), "op-ro-ok", stmt); err != nil {
			t.Errorf("%s on a read-only connection: %v", stmt, err)
		}
	}
	for _, stmt := range []string{"CONFIG SET maxmemory 0", "FUNCTION FLUSH", "CLIENT KILL ID 1", "XGROUP CREATE s g $ MKSTREAM", "NOSUCH SUB"} {
		_, err := consoleRun(a, fixture, context.Background(), "op-ro-deny", stmt)
		if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported && code != adapters.CodeQuery {
			t.Errorf("%s on a read-only connection: err = %v, want a refusal", stmt, err)
		}
	}
}

func TestRedis_Connect_CancelledCtxReturns(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	proxy := testsupport.StartPausableProxy(t, fixture.Config)
	testsupport.ConnectCancelScenario(t, newAdapter(t), proxy.Config(), proxy)
}

// DB MCP read lane: redis has no read-only session, so the lane relies on Kira's own COMMAND-flag check.
func TestRedis_ReadLane(t *testing.T) {
	fixture := testsupport.StartRedis(t)
	testsupport.RunScenarios(t, connectedAdapter(t, fixture), fixture.Config,
		testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
			Path: nodePath(fixture), Write: "SET kira_lane_probe 1", WantErr: "read-only",
		}),
	)
}
