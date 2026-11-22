package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// scriptEnvelope is the agent result of a scripted run_query.
type scriptEnvelope struct {
	Kind   string          `json:"kind"`
	Output json.RawMessage `json:"output"`
	Input  struct {
		Kind     string `json:"kind"`
		RowCount int    `json:"rowCount"`
	} `json:"input"`
}

func decodeEnvelope(t *testing.T, kind string, res *mcp.CallToolResult) scriptEnvelope {
	t.Helper()
	var env scriptEnvelope
	if res.IsError || json.Unmarshal([]byte(text(res)), &env) != nil || env.Kind != "script" {
		t.Fatalf("%s scripted run = %q, want a script envelope", kind, text(res))
	}
	return env
}

// inputKind is the rendered result kind each engine's script reads.
var inputKind = map[string]string{"mongodb": "document", "redis": "keyvalue"}

// wantTypes is what typesRead's four cells look like to a script: the big int keeps every digit on every
// engine, and a cell is the engine's text for the value, so a decimal or timestamp keeps the format the
// engine prints. NULL is the one non-string.
var wantTypes = map[string]string{
	"sqlite":   `[["string","9007199254740993"],["string","12.50"],["object","null"],["string","2026-01-02 03:04:05"]]`,
	"postgres": `[["string","9007199254740993"],["string","12.50"],["object","null"],["string","2026-01-02 03:04:05"]]`,
	"mysql":    `[["string","9007199254740993"],["string","12.50"],["object","null"],["string","2026-01-02 03:04:05"]]`,
	"mariadb":  `[["string","9007199254740993"],["string","12.50"],["object","null"],["string","2026-01-02 03:04:05"]]`,
	// clickhouse's JSON output prints Decimal32(2) 12.50 as the number 12.5: the scale goes, no digit of the value does.
	"clickhouse": `[["string","9007199254740993"],["string","12.5"],["object","null"],["string","2026-01-02 03:04:05"]]`,
}

// TestDbMcpEngineQueryScript: the result script of run_query on every engine: it reads the rendered result,
// keeps ids a later read binds, leaves a write's changes standing when it fails, reads masked cells redacted
// and is held for review with the same raw result an unscripted read gives.
// UI half: tests/ui/studio/e2e-db-script-chain.spec.ts (the same scenarios on sqlite).
func TestDbMcpEngineQueryScript(t *testing.T) {
	engineTest(t, nil, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		connID := f.engineConn(t, e, nil)
		sql := e.typesRead != ""
		scratch := e.scratch("script")
		e.setup(t, sess, connID, scratch)
		scripted := func(stmt, mode, script string, params ...string) *mcp.CallToolResult {
			args := e.args(connID, e.sql(stmt, scratch), mode)
			args["script"] = script
			if len(params) > 0 {
				args["params"] = params
			}
			return call(t, sess, args)
		}

		t.Run("render", func(t *testing.T) {
			env := decodeEnvelope(t, e.kind, scripted(e.read, "read", `return {kind: result.kind, rows: result.rows ?? result.documents ?? result.entries};`))
			want := "tabular"
			if k, ok := inputKind[e.kind]; ok {
				want = k
			}
			if env.Input.Kind != want || env.Input.RowCount == 0 || !strings.Contains(string(env.Output), `"kind":"`+want+`"`) {
				t.Fatalf("%s envelope = %+v output %s, want the %s render of the seeded row", e.kind, env, env.Output, want)
			}
			if sql && !strings.Contains(string(env.Output), `"a"`) {
				t.Fatalf("%s output = %s, want the seeded a", e.kind, env.Output)
			}
		})

		if sql {
			t.Run("types", func(t *testing.T) {
				env := decodeEnvelope(t, e.kind, call(t, sess, mergeArgs(e.args(connID, e.typesRead, "read"), map[string]any{"script": `return result.rows[0].map(c => [typeof c, String(c)])`})))
				if got := string(env.Output); got != wantTypes[e.kind] {
					t.Fatalf("%s types = %s, want %s", e.kind, got, wantTypes[e.kind])
				}
			})
		}

		t.Run("chain", func(t *testing.T) {
			if r := e.run(t, sess, connID, e.sql(e.write2, scratch), "write"); r.IsError {
				t.Fatalf("%s second write = %q", e.kind, text(r))
			}
			if !sql {
				res := scripted(e.read, "read", `kira.storeValue("n", 1); return 1;`)
				if res.IsError || !strings.Contains(text(res), `"kept":[{"name":"n"`) {
					t.Fatalf("%s kira.storeValue = %q, want it kept", e.kind, text(res))
				}
				args := e.args(connID, e.sql(e.read, scratch), "read")
				args["params"] = []string{"n"}
				if res := call(t, sess, args); !res.IsError || !strings.Contains(text(res), "takes no params") {
					t.Fatalf("%s params = %q, want a takes-no-params refusal", e.kind, text(res))
				}
				return
			}
			keep := scripted("select id from {t} order by id", "read", `kira.storeValue("ids", result.rows.map(r => Number(r[0]))); return result.rows.length;`)
			if keep.IsError || !strings.Contains(text(keep), `"output":2`) || !strings.Contains(text(keep), `"name":"ids","type":"array","items":2`) {
				t.Fatalf("%s keep = %q, want 2 ids kept", e.kind, text(keep))
			}
			k := keptSQLByKind[e.kind]
			args := e.args(connID, e.sql(k.byList, scratch), "read")
			args["params"] = k.listParams
			by := call(t, sess, args)
			if by.IsError || !strings.Contains(text(by), `"a"`) || !strings.Contains(text(by), `"b"`) {
				t.Fatalf("%s read by the kept ids = %q, want rows a and b", e.kind, text(by))
			}
		})

		t.Run("write fails", func(t *testing.T) {
			fresh := e.scratch("scriptw")
			e.setup(t, sess, connID, fresh)
			args := e.args(connID, e.sql(e.write2, fresh), "write")
			args["script"] = `throw new Error("boom")`
			res := call(t, sess, args)
			if !res.IsError || !strings.Contains(text(res), "boom") || !strings.Contains(text(res), "Its changes stay. Do not rerun it.") {
				t.Fatalf("%s failed write script = %q, want the changes-stay note", e.kind, text(res))
			}
			if sql {
				if c := e.run(t, sess, connID, e.sql(e.count, fresh), "read"); c.IsError || !strings.Contains(text(c), `["2"]`) && !strings.Contains(text(c), `[2]`) {
					t.Fatalf("%s count after the failed script = %q, want exactly one more row (2)", e.kind, text(c))
				}
				return
			}
			after := e.run(t, sess, connID, e.sql(strings.Replace(e.read, "{t}", "{t}_b", 1), fresh), "read")
			if e.kind == "mongodb" {
				after = e.run(t, sess, connID, e.sql(e.read, fresh), "read")
			}
			if after.IsError || !strings.Contains(text(after), "2") {
				t.Fatalf("%s read after the failed script = %q, want the write to have run", e.kind, text(after))
			}
		})

		if !sql {
			return
		}
		t.Run("review", func(t *testing.T) {
			plain := e.run(t, sess, connID, e.sql(e.read, scratch), "read")
			if plain.IsError {
				t.Fatalf("%s unscripted read = %q", e.kind, text(plain))
			}
			f.mcpSet(t, map[string]string{"review.read": "review"})
			args := e.args(connID, e.sql(e.read, scratch), "read")
			args["script"] = pickRows
			pending := callToolAsync(sess, "run_query", args)
			req := f.pending(t)
			held := f.pendingResponse(t, req.RequestID)
			if req.Stage != "response" || held.Raw != text(plain) {
				t.Fatalf("%s held raw = %s, want the unscripted render %s", e.kind, held.Raw, text(plain))
			}
			f.answer(t, true, req.RequestID)
			if res := wait(t, pending); res.IsError {
				t.Fatalf("%s sent = %q", e.kind, text(res))
			}
			f.mcpSet(t, map[string]string{"review.read": "send"})
		})

		t.Run("masked", func(t *testing.T) {
			table, column, prefix, _ := maskTarget(e)
			masked := f.engineConn(t, e, func(c *model.ConnectionFields) {
				if e.kind != "sqlite" {
					c.McpTables = []string{strings.TrimSuffix(prefix, ".")}
				}
			})
			if _, err := f.app.W.MaskRules.Upsert(bridge.MaskRulesUpsertArgs{ConnectionID: masked, Fields: model.MaskRuleFields{TableName: table, ColumnName: column, Kind: "redact"}}); err != nil {
				t.Fatal(err)
			}
			args := e.args(masked, "select "+column+" from "+e.customers+" order by id limit 2", "read")
			args["script"] = "return result.rows;"
			res := call(t, sess, args)
			if res.IsError || !strings.Contains(text(res), "[redacted]") || strings.Count(text(res), "[redacted]") != 2 {
				t.Fatalf("%s masked scripted read = %q, want redacted cells only", e.kind, text(res))
			}
		})
	})
}

const pickRows = `return result.rows;`
