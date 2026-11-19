package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// keptSQL is one engine's spelling of the same three calls: write two kept values, read by a kept list,
// read by a kept text.
type keptSQL struct {
	insert, byList, byText string
	// params names the kept variables of each statement, in placeholder order.
	insertParams, listParams, textParams []string
}

var keptSQLByKind = map[string]keptSQL{
	"postgres": {
		insert: "insert into {t} (id, v) values ($1, $2)", insertParams: []string{"rid", "word"},
		byList: "select v from {t} where id = any($1) order by id", listParams: []string{"ids"},
		byText: "select count(*) as n from {t} where v = $1", textParams: []string{"word"},
	},
	"mysql": {
		insert: "insert into {t} (id, v) values (?, ?)", insertParams: []string{"rid", "word"},
		byList: "select v from {t} where id in (select j.v from json_table(?, '$[*]' columns (v bigint path '$')) j) order by id", listParams: []string{"ids"},
		byText: "select count(*) as n from {t} where v = ?", textParams: []string{"word"},
	},
	"sqlite": {
		insert: "insert into {t} (id, v) values (?, ?)", insertParams: []string{"rid", "word"},
		byList: "select v from {t} where id in (select value from json_each(?)) order by id", listParams: []string{"ids"},
		byText: "select count(*) as n from {t} where v = ?", textParams: []string{"word"},
	},
	"clickhouse": {
		insert: "insert into {t} select {rid:UInt32}, {word:String}", insertParams: []string{"rid", "word"},
		byList: "select v from {t} where id in {ids:Array(UInt32)} order by id", listParams: []string{"ids"},
		byText: "select count() as n from {t} where v = {word:String}", textParams: []string{"word"},
	},
}

func init() { keptSQLByKind["mariadb"] = keptSQLByKind["mysql"] }

// TestDbMcpEngineKeptParams: on every SQL engine a kept text value (with a quote and a drop statement in it) and
// a kept list bind as parameters: the row is stored and found as text, the table survives, and a kept
// variable on mongodb or redis is refused.
// UI half: tests/ui/studio/dbmcp-kept-params.spec.ts.
func TestDbMcpEngineKeptParams(t *testing.T) {
	engineTest(t, nil, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		connID := f.engineConn(t, e, nil)
		scratch := e.scratch("kept")
		e.setup(t, sess, connID, scratch)
		evil := "x'); drop t; --" // fits v varchar(20)
		quoted, err := json.Marshal(evil)
		if err != nil {
			t.Fatal(err)
		}
		f.keep(t, externalScope, "rid", "2")
		f.keep(t, externalScope, "word", string(quoted))
		f.keep(t, externalScope, "ids", "[1,2]")

		k, ok := keptSQLByKind[e.kind]
		if !ok {
			args := e.args(connID, e.sql(e.read, scratch), "read")
			args["params"] = []string{"rid"}
			res := call(t, sess, args)
			if !res.IsError || !strings.Contains(text(res), "takes no params") {
				t.Fatalf("%s params = %q, want a takes-no-params refusal", e.kind, text(res))
			}
			return
		}
		run := func(stmt, mode string, params []string) *mcp.CallToolResult {
			args := e.args(connID, e.sql(stmt, scratch), mode)
			args["params"] = params
			return call(t, sess, args)
		}
		if res := run(k.insert, "write", k.insertParams); res.IsError {
			t.Fatalf("%s bound insert = %q, want it to run", e.kind, text(res))
		}
		list := run(k.byList, "read", k.listParams)
		if list.IsError || !strings.Contains(text(list), `"a"`) || !strings.Contains(text(list), evil) {
			t.Fatalf("%s list read = %q, want the seeded row and the stored text", e.kind, text(list))
		}
		by := run(k.byText, "read", k.textParams)
		if by.IsError || !strings.Contains(text(by), `["1"]`) && !strings.Contains(text(by), `[1]`) {
			t.Fatalf("%s read by the text = %q, want one row", e.kind, text(by))
		}
		if res := e.run(t, sess, connID, e.sql(e.count, scratch), "read"); res.IsError || !strings.Contains(text(res), "2") {
			t.Fatalf("%s count after the injection attempt = %q, want the table intact with 2 rows", e.kind, text(res))
		}
	})
}
