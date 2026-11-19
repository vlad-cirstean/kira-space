package jsengine

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func runOK(t *testing.T, src string) string {
	t.Helper()
	res, e := exec1(t, Run{Source: src, Limits: small})
	if e != nil {
		t.Fatalf("%v", e)
	}
	return string(res.JSON)
}

func TestSandboxGlobals(t *testing.T) {
	got := runOK(t, `return ["require","process","console","print","setTimeout","setInterval","queueMicrotask","fetch","XMLHttpRequest","WebAssembly","performance","WeakRef","FinalizationRegistry","Atomics","SharedArrayBuffer","Intl","structuredClone"].map(n => typeof globalThis[n])`)
	want := "[" + strings.TrimSuffix(strings.Repeat(`"undefined",`, 17), ",") + "]"
	if got != want {
		t.Fatalf("host globals leaked: %s", got)
	}
	if got := runOK(t, `return typeof require + typeof process`); got != `"undefinedundefined"` {
		t.Fatal(got)
	}
}

func TestDynamicCodeBlocked(t *testing.T) {
	cases := map[string]string{
		"Function call":        `Function('return 1')`,
		"Function new":         `new Function('return 1')`,
		"function constructor": `(function () {}).constructor('return 1')`,
		"generator":            `Object.getPrototypeOf(function* () {}).constructor('yield 1')`,
		"async":                `Object.getPrototypeOf(async function () {}).constructor('return 1')`,
		"Reflect.construct":    `Reflect.construct(Function, ['return 1'])`,
	}
	for name, expr := range cases {
		t.Run(name, func(t *testing.T) {
			if got := runOK(t, "try { "+expr+"; return 'ran' } catch (e) { return e.name }"); got != `"EvalError"` {
				t.Fatal(got)
			}
		})
	}
	for _, expr := range []string{`eval('1')`, `(0, eval)('1')`} {
		if got := runOK(t, "try { "+expr+"; return 'ran' } catch (e) { return e.name }"); got != `"ReferenceError"` {
			t.Fatalf("%s: %s", expr, got)
		}
	}
	if got := runOK(t, `return [(() => 1) instanceof Function, typeof Function.prototype.call]`); got != `[true,"function"]` {
		t.Fatalf("instanceof Function broke: %s", got)
	}
	if e := Check(`return import('os')`); e == nil || e.Kind != KindSyntax {
		t.Fatalf("dynamic import: %v", e)
	}
}

func TestFrozenWorld(t *testing.T) {
	for _, stmt := range []string{
		"Object.prototype.x = 1", "Math.random = () => 1", "JSON.parse = () => 1", "globalThis.y = 1",
		"Array.prototype.map = null", "v = 2", "delete JSON.stringify",
	} {
		got := runOK(t, `try { `+stmt+`; return 'ok' } catch (e) { return e.name }`)
		if got == `"ok"` {
			t.Errorf("%s succeeded", stmt)
		}
	}
	res, e := exec1(t, Run{Source: "try { v = 2; return 'ok' } catch (e) { return e.name }", Limits: small, Bindings: []Binding{{Name: "v", Value: []byte("1")}}})
	if e != nil || string(res.JSON) != `"TypeError"` {
		t.Fatalf("binding writable: %s %v", res.JSON, e)
	}
	// Breaking out of the wrapper runs more code in the same sandbox and gains nothing.
	_, e = exec1(t, Run{Source: "return 1;})();\nFunction('return 1');\n(function(){", Limits: small})
	if e == nil || e.Kind != KindRuntime || !strings.Contains(e.Message, "EvalError") {
		t.Fatalf("break-out: %v", e)
	}
}

func TestClockAndRandom(t *testing.T) {
	now := time.Date(2020, 1, 2, 3, 4, 5, 678_000_000, time.UTC)
	res, e := exec1(t, Run{Source: "return [Date.now(), new Date().getTime(), new Date().toISOString()]", Now: now, Limits: small})
	if e != nil || string(res.JSON) != `[1577934245678,1577934245678,"2020-01-02T03:04:05.678Z"]` {
		t.Fatalf("%s %v", res.JSON, e)
	}
	r := func(seed uint64) string {
		res, e := exec1(t, Run{Source: "return [Math.random(), Math.random()]", Seed: seed, Limits: small})
		if e != nil {
			t.Fatal(e)
		}
		return string(res.JSON)
	}
	if r(7) != r(7) || r(7) == r(8) {
		t.Fatal("Math.random is not a function of the seed")
	}
}

func TestLocalTimeIsUTC(t *testing.T) {
	t.Setenv("TZ", "America/New_York")
	got := runOK(t, `return [new Date().getTimezoneOffset(), new Date().getHours() === new Date().getUTCHours(), new Date(0).toString().endsWith('(UTC)')]`)
	if got != `[0,true,true]` {
		t.Fatal(got)
	}
}

func TestWorkerEnvironment(t *testing.T) {
	t.Setenv("KIRA_JS_SECRET", "s3cr3t")
	var cmd *exec.Cmd
	e := newEngine(func() *exec.Cmd { cmd = exec.Command(os.Args[0], "js-worker"); return cmd })
	if _, err := e.Exec(context.Background(), Run{Source: "return 1", Limits: small}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"GOTRACEBACK=none", "GOMAXPROCS=2", "TZ=UTC"}; !reflect.DeepEqual(cmd.Env, want) {
		t.Fatalf("worker env %v", cmd.Env)
	}
}

func TestPromiseReturnRefused(t *testing.T) {
	for _, src := range []string{"return Promise.resolve(1)", "return (async () => 1)()", "return {then() {}}"} {
		_, e := exec1(t, Run{Source: src, Limits: small})
		wantKind(t, e, KindOutput)
	}
}
