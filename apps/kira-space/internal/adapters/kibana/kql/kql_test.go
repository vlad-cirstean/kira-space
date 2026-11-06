package kql_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/kibana/kql"
)

var fields = []kql.Field{
	{Name: "message", Type: "string", Searchable: true},
	{Name: "status", Type: "number", Searchable: true},
	{Name: "@timestamp", Type: "date", Searchable: true},
	{Name: "user.name", Type: "string", Nested: "user", Searchable: true},
	{Name: "host.name", Type: "string", Searchable: true},
	{Name: "host.os", Type: "string", Searchable: true},
}

func jsonEq(t *testing.T, want, got string) {
	t.Helper()
	var w, g any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w, g) {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestCompile(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{"blank", "  ", `{"match_all":{}}`},
		{"match", `status:200`, `{"match":{"status":{"lenient":true,"query":"200"}}}`},
		{"phrase", `message:"hello world"`, `{"match_phrase":{"message":"hello world"}}`},
		{"multiword", `message:hello world`, `{"match":{"message":{"lenient":true,"query":"hello world"}}}`},
		{"exists", `message:*`, `{"exists":{"field":"message"}}`},
		{"wildcard", `message:he*o`, `{"query_string":{"analyze_wildcard":true,"fields":["message"],"query":"he*o"}}`},
		{"range", `status>=500`, `{"range":{"status":{"gte":"500"}}}`},
		{"daterange tz", `@timestamp<"2026-01-02"`, `{"range":{"@timestamp":{"lt":"2026-01-02","time_zone":"UTC"}}}`},
		{"not", `not status:200`, `{"bool":{"must_not":[{"match":{"status":{"lenient":true,"query":"200"}}}]}}`},
		{"and or precedence", `a:1 or b:2 and c:3`,
			`{"bool":{"minimum_should_match":1,"should":[{"match":{"a":{"lenient":true,"query":"1"}}},{"bool":{"filter":[{"match":{"b":{"lenient":true,"query":"2"}}},{"match":{"c":{"lenient":true,"query":"3"}}}]}}]}}`},
		{"value list", `status:(200 or 404)`,
			`{"bool":{"minimum_should_match":1,"should":[{"match":{"status":{"lenient":true,"query":"200"}}},{"match":{"status":{"lenient":true,"query":"404"}}}]}}`},
		{"free text", `error`, `{"multi_match":{"lenient":true,"query":"error","type":"best_fields"}}`},
		{"free phrase", `"disk full"`, `{"multi_match":{"lenient":true,"query":"disk full","type":"phrase"}}`},
		{"field wildcard", `host.*:x`,
			`{"bool":{"minimum_should_match":1,"should":[{"match":{"host.name":{"lenient":true,"query":"x"}}},{"match":{"host.os":{"lenient":true,"query":"x"}}}]}}`},
		{"field wildcard none", `nope.*:x`, `{"match_none":{}}`},
		{"nested block", `user:{ name:bob }`,
			`{"nested":{"path":"user","query":{"match":{"user.name":{"lenient":true,"query":"bob"}}},"score_mode":"none"}}`},
		{"nested implicit", `user.name:bob`,
			`{"nested":{"path":"user","query":{"match":{"user.name":{"lenient":true,"query":"bob"}}},"score_mode":"none"}}`},
		{"escaped keyword", `message:\and`, `{"match":{"message":{"lenient":true,"query":"and"}}}`},
		{"escaped special", `message:a\:b`, `{"match":{"message":{"lenient":true,"query":"a:b"}}}`},
		{"wildcard escapes reserved", `message:a-b*`, `{"query_string":{"analyze_wildcard":true,"fields":["message"],"query":"a\\-b*"}}`},
		{"keyword case", `status:1 AND NOT status:2`,
			`{"bool":{"filter":[{"match":{"status":{"lenient":true,"query":"1"}}},{"bool":{"must_not":[{"match":{"status":{"lenient":true,"query":"2"}}}]}}]}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := kql.Compile(c.query, fields, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(got)
			jsonEq(t, c.want, string(b))
		})
	}
}

func TestSyntaxErrors(t *testing.T) {
	cases := []struct {
		name, query string
		pos         int
	}{
		{"unclosed paren", `(a:1`, 5},
		{"dangling and", `a:1 and`, 8},
		{"missing value", `a:`, 3},
		{"stray paren", `a:1)`, 4},
		{"unclosed quote", `a:"x`, 5},
		{"unclosed nested", `a:{ b:1`, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := kql.Compile(c.query, fields, "")
			var se *kql.SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want a SyntaxError", err)
			}
			if se.Pos != c.pos {
				t.Errorf("pos = %d, want %d", se.Pos, c.pos)
			}
		})
	}
}

func TestLuceneQuery(t *testing.T) {
	b, _ := json.Marshal(kql.LuceneQuery("status:[400 TO 499]", "UTC"))
	jsonEq(t, `{"query_string":{"analyze_wildcard":true,"query":"status:[400 TO 499]","time_zone":"UTC"}}`, string(b))
	b, _ = json.Marshal(kql.LuceneQuery(" ", ""))
	jsonEq(t, `{"match_all":{}}`, string(b))
}
