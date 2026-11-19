package jsengine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestInputNumbers(t *testing.T) {
	cases := []struct{ in, want string }{
		{"9007199254740991", `["number",9007199254740991]`},
		{"9007199254740993", `["bigint",9007199254740993]`},
		{"-9223372036854775808", `["bigint",-9223372036854775808]`},
		{"18446744073709551616", `["bigint",18446744073709551616]`},
		{"1234567890123456789012345678901234567890", `["bigint",1234567890123456789012345678901234567890]`},
		{"1.5", `["number",1.5]`},
		{"1e3", `["number",1000]`},
		{"1e400", `["number",null]`},
		{"[[1,[2,{\"a\":18446744073709551617}]],\"s\"]", `["object",[[1,[2,{"a":18446744073709551617}]],"s"]]`},
		{`{"b":1,"2":2,"a":3,"1":4}`, `["object",{"1":4,"2":2,"b":1,"a":3}]`}, // integer-like keys first, as ECMA-262
		{`{"a":1,"a":2}`, `["object",{"a":2}]`},
		{`{"__proto__":{"x":1}}`, `["object",{"__proto__":{"x":1}}]`},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			res, e := exec1(t, Run{Source: "return [typeof v, v]", Limits: small, Bindings: []Binding{{Name: "v", Value: json.RawMessage(c.in)}}})
			if e != nil || string(res.JSON) != c.want {
				t.Fatalf("%s %v", res.JSON, e)
			}
		})
	}
	res, e := exec1(t, Run{Source: "return [Object.is(v, -0), 1 / v]", Limits: small, Bindings: []Binding{{Name: "v", Value: json.RawMessage("-0")}}})
	if e != nil || string(res.JSON) != `[true,null]` {
		t.Fatalf("negative zero: %s %v", res.JSON, e)
	}
}

func TestInputDepth(t *testing.T) {
	nest := func(n int) json.RawMessage { return json.RawMessage(strings.Repeat("[", n) + strings.Repeat("]", n)) }
	if _, e := exec1(t, Run{Source: "return typeof v", Limits: small, Bindings: []Binding{{Name: "v", Value: nest(256)}}}); e != nil {
		t.Fatalf("depth 256: %v", e)
	}
	_, e := exec1(t, Run{Source: "return 1", Limits: small, Bindings: []Binding{{Name: "v", Value: nest(257)}}})
	wantKind(t, e, KindInput)
}

func TestScriptJSON(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"kira.parseJSON lossless", `return kira.parseJSON('{"a":18446744073709551617,"b":[1.5]}')`, `{"a":18446744073709551617,"b":[1.5]}`},
		{"JSON.parse stays lossy", `return JSON.parse('18446744073709551617')`, `18446744073709552000`},
		{"stringify of a bigint is a string", `return JSON.stringify(2n ** 64n)`, `"\"18446744073709551616\""`},
		{"foreign marker-like string stays a string", `return "\u0000" + "0123456789abcdef0123456789abcdef" + "42"`, `"\u00000123456789abcdef0123456789abcdef42"`},
		{"bigint arithmetic survives output", `return {n: 2n ** 70n + 1n}`, `{"n":1180591620717411303425}`},
		{"stringify with function replacer", `const o={a:1,b:{c:[1,2,{d:3}],e:"x"}}; return JSON.stringify(o,(k,v)=>typeof v==='number'?v*2:v)`, `"{\"a\":2,\"b\":{\"c\":[2,4,{\"d\":6}],\"e\":\"x\"}}"`},
		{"stringify with allow list", `const o={a:1,b:{c:[1,2,{d:3}],e:"x"}}; return JSON.stringify(o,['b','c','d'])`, `"{\"b\":{\"c\":[1,2,{\"d\":3}]}}"`},
		{"stringify allow list orders keys", `return JSON.stringify({a:1,b:2},['b','a'])`, `"{\"b\":2,\"a\":1}"`},
		{"stringify with indent", `return JSON.stringify({a:[1]},null,2)`, `"{\n  \"a\": [\n    1\n  ]\n}"`},
		{"undefined result", `return undefined`, `null`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, e := exec1(t, Run{Source: c.src, Limits: small})
			if e != nil || string(res.JSON) != c.want {
				t.Fatalf("%s %v", res.JSON, e)
			}
		})
	}
	_, e := exec1(t, Run{Source: "const a = {}; a.a = a; return a", Limits: small})
	wantKind(t, e, KindOutput)
}

func TestHostCalls(t *testing.T) {
	var gotArgs string
	calls := 0
	h := Binding{Name: "h", Funcs: map[string]HostFunc{
		"echo": func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			gotArgs = string(args)
			return args, nil
		},
		"fail": func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, errors.New("nope") },
		"boom": func(context.Context, json.RawMessage) (json.RawMessage, error) { panic("secret token in panic") },
		"ok":   func(context.Context, json.RawMessage) (json.RawMessage, error) { return json.RawMessage(`7`), nil },
		"n": func(context.Context, json.RawMessage) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`1`), nil
		},
	}}
	run := func(src string) (Result, *Error) {
		return exec1(t, Run{Source: src, Limits: Limits{Timeout: 10 * time.Second}, Bindings: []Binding{h}})
	}
	t.Run("big integer round trip", func(t *testing.T) {
		res, e := run("return h.echo(2n ** 64n + 1n, 'x', {a: [1]})")
		if e != nil || gotArgs != `[18446744073709551617,"x",{"a":[1]}]` || string(res.JSON) != `[18446744073709551617,"x",{"a":[1]}]` {
			t.Fatalf("%s | %s | %v", gotArgs, res.JSON, e)
		}
	})
	t.Run("error text reaches the script", func(t *testing.T) {
		res, e := run("try { h.fail() } catch (e) { return [e instanceof Error, e.message] }")
		if e != nil || string(res.JSON) != `[true,"nope"]` {
			t.Fatalf("%s %v", res.JSON, e)
		}
	})
	t.Run("panic is recovered and hidden", func(t *testing.T) {
		res, e := run("let m; try { h.boom() } catch (e) { m = e.message } return [m, h.ok()]")
		if e != nil || string(res.JSON) != `["internal error",7]` {
			t.Fatalf("%s %v", res.JSON, e)
		}
	})
	t.Run("10001st call throws", func(t *testing.T) {
		calls = 0
		res, e := run("let c = 0; try { for (;;) { h.n(); c++ } } catch (e) { return [c, e.message] }")
		if e != nil || string(res.JSON) != `[10000,"host call limit exceeded (10000 per run)"]` || calls != MaxHostCalls {
			t.Fatalf("%s %v (%d host calls)", res.JSON, e, calls)
		}
	})
}

func TestBindingValidation(t *testing.T) {
	for _, name := range []string{"Upper", "1a", "kira", "class", "a-b", "", strings.Repeat("a", 33), "eval", "undefined"} {
		_, e := exec1(t, Run{Source: "return 1", Bindings: []Binding{{Name: name, Value: json.RawMessage("1")}}})
		if e == nil || e.Kind != KindInput {
			t.Errorf("name %q accepted: %v", name, e)
		}
	}
	_, e := exec1(t, Run{Source: "return 1", Bindings: []Binding{{Name: "parseInt", Value: json.RawMessage("1")}}})
	wantKind(t, e, KindInput) // an existing global, caught in the worker
	_, e = exec1(t, Run{Source: "return 1", Bindings: []Binding{{Name: "a", Value: json.RawMessage("1")}, {Name: "a", Value: json.RawMessage("2")}}})
	wantKind(t, e, KindInput)
}
