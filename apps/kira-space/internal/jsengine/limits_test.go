package jsengine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "js-worker" {
		os.Exit(RunWorker(os.Args[2:]))
	}
	os.Exit(m.Run())
}

// testEngine re-execs the test binary as the worker.
func testEngine() *Engine {
	return newEngine(func() *exec.Cmd { return exec.Command(os.Args[0], "js-worker") })
}

// small keeps child memory low: a hard exit happens well under the container's memory.
var small = Limits{MemoryBytes: 64 << 20}

func exec1(t *testing.T, r Run) (Result, *Error) {
	t.Helper()
	res, err := testEngine().Exec(context.Background(), r)
	if err == nil {
		return res, nil
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("plain error: %v", err)
	}
	return res, e
}

func wantKind(t *testing.T, e *Error, k Kind) *Error {
	t.Helper()
	if e == nil || e.Kind != k {
		t.Fatalf("want kind %s, got %v", k, e)
	}
	return e
}

func TestTimeoutAndCancel(t *testing.T) {
	t.Run("infinite loop is positioned", func(t *testing.T) {
		start := time.Now()
		_, e := exec1(t, Run{Source: "let i = 0; for (;;) { i++ }", Limits: Limits{Timeout: 300 * time.Millisecond * timeScale}})
		wantKind(t, e, KindTimeout)
		if e.Line != 1 {
			t.Fatalf("no position: %v", e)
		}
		if d := time.Since(start); d > (300*time.Millisecond+time.Second)*timeScale {
			t.Fatalf("took %v", d)
		}
	})
	t.Run("blocked host function is killed by deadline plus grace", func(t *testing.T) {
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		e := testEngine()
		start := time.Now()
		_, err := e.Exec(context.Background(), Run{
			Source:   "h.wait(); return 1",
			Limits:   Limits{Timeout: 300 * time.Millisecond * timeScale},
			Bindings: []Binding{{Name: "h", Funcs: map[string]HostFunc{"wait": func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) { <-release; return nil, nil }}}},
		})
		var se *Error
		if !errors.As(err, &se) || se.Kind != KindTimeout {
			t.Fatalf("got %v", err)
		}
		if d := time.Since(start); d > (300*time.Millisecond+time.Second)*timeScale+killGrace {
			t.Fatalf("took %v", d)
		}
		if len(e.sem) != 0 {
			t.Fatal("slot not released")
		}
	})
	t.Run("cancel kills the child", func(t *testing.T) {
		var cmd *exec.Cmd
		e := newEngine(func() *exec.Cmd { cmd = exec.Command(os.Args[0], "js-worker"); return cmd })
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(150*time.Millisecond, cancel)
		_, err := e.Exec(ctx, Run{Source: "for (;;) {}", Limits: Limits{Timeout: 5 * time.Second}})
		var se *Error
		if !errors.As(err, &se) || se.Kind != KindCanceled {
			t.Fatalf("got %v", err)
		}
		if cmd.ProcessState == nil || len(e.sem) != 0 {
			t.Fatal("child still running or slot held")
		}
	})
	t.Run("deadline spans spawn and run", func(t *testing.T) {
		// A slow spawn eats into the budget: total wall stays near Timeout.
		e := newEngine(func() *exec.Cmd {
			return exec.Command("sh", "-c", `sleep 0.5; exec "$0" js-worker`, os.Args[0])
		})
		start := time.Now()
		_, err := e.Exec(context.Background(), Run{Source: "for (;;) {}", Limits: Limits{Timeout: 600 * time.Millisecond * timeScale}})
		var se *Error
		if !errors.As(err, &se) || se.Kind != KindTimeout {
			t.Fatalf("got %v", err)
		}
		if d := time.Since(start); d > 1050*time.Millisecond*timeScale {
			t.Fatalf("deadline did not include spawn: %v", d)
		}
	})
}

func heapObjects() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func TestMemoryAndStack(t *testing.T) {
	deep := small
	deep.MemoryBytes = 128 << 20
	cases := []struct {
		name string
		src  string
		lim  Limits
		kind Kind
		pos  bool // error carries a line
	}{
		{"push loop", "const a = []; for (;;) a.push({x: 1})", small, KindMemory, true},
		{"rope compare hard exit", "let s = 'a'; for (let i = 0; i < 29; i++) s = s + s; return s === s + 'x'", small, KindMemory, false},
		{"bigint pow folded at compile time", "return 2n ** (2n ** 30n)", Limits{MemoryBytes: 64 << 20, Timeout: 10 * time.Second}, KindMemory, false},
		{"bigint pow at run time", "const b = 2n; return b ** (b ** 30n)", Limits{MemoryBytes: 64 << 20, Timeout: 10 * time.Second}, KindMemory, false},
		{"deep JSON.parse is a runtime error", "return JSON.parse('['.repeat(300000))", small, KindRuntime, true},
		{"deep toString", "let a = []; for (let i = 0; i < 300000; i++) a = [a]; return String(a)", deep, KindMemory, true},
		{"deep flat", "let a = []; for (let i = 0; i < 300000; i++) a = [a]; return a.flat(Infinity)", deep, KindMemory, true},
		{"recursion", "function f() { return f() + 1 } return f()", small, KindStack, true},
		{"catastrophic regexp uncaught", "return /^(a+)+$/.test('a'.repeat(32) + 'b')", Limits{MemoryBytes: 64 << 20, Timeout: MaxTimeout}, KindRuntime, true},
		{"output over cap", "return 'x'.repeat(2 ** 21)", small, KindOutput, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := heapObjects()
			start := time.Now()
			_, e := exec1(t, Run{Source: c.src, Limits: c.lim})
			wantKind(t, e, c.kind)
			if c.pos && e.Line == 0 && !raceEnabled {
				t.Fatalf("no position: %v", e)
			}
			if c.name == "catastrophic regexp uncaught" && time.Since(start) >= MaxTimeout {
				t.Fatalf("regexp ran to the deadline: %v", time.Since(start))
			}
			if after := heapObjects(); after > before+16<<20 {
				t.Fatalf("parent heap grew %d bytes", after-before)
			}
		})
	}
	t.Run("catastrophic regexp caught", func(t *testing.T) {
		res, e := exec1(t, Run{Source: "try { /^(a+)+$/.test('a'.repeat(32) + 'b'); return 'no' } catch (e) { return e.name }", Limits: small})
		if e != nil || string(res.JSON) != `"RangeError"` {
			t.Fatalf("%s %v", res.JSON, e)
		}
	})
	t.Run("garbage churn is not a memory error", func(t *testing.T) {
		_, e := exec1(t, Run{Source: "for (;;) { const a = new Array(1000).fill(1); a.length = 0 }", Limits: Limits{MemoryBytes: 64 << 20, Timeout: 1500 * time.Millisecond * timeScale}})
		wantKind(t, e, KindTimeout)
	})
}

var guardLimits = Limits{MemoryBytes: 64 << 20, Timeout: 8 * time.Second}

func TestGuards(t *testing.T) {
	cases := []struct {
		name, over, under, want string
	}{
		{"G1 repeat", "'x'.repeat(2 ** 24)", "'x'.repeat(2 ** 22).length", "4194304"},
		{"G1 padStart", "'x'.padStart(2 ** 24)", "'x'.padStart(2 ** 22).length", "4194304"},
		{"G1 padEnd", "'x'.padEnd(2 ** 24)", "'x'.padEnd(1000).length", "1000"},
		{"G1 concat", "'x'.repeat(2 ** 22).concat('y'.repeat(2 ** 22), 'z'.repeat(2 ** 22), 'w'.repeat(2 ** 22))", "'a'.concat('b', 'c')", `"abc"`},
		{"G2 join", "Array(2 ** 20).fill('xxxxxxxxxxxxxxxxxxxxxxxx').join('')", "Array(1000).fill('x').join(',').length", "1999"},
		{"G2 join depth", "let a = []; for (let i = 0; i < 1000; i++) a = [a]; String(a)", "String([[1], [2, [3]]])", `"1,2,3"`},
		{"G3 fill", "Array(2 ** 24).fill(0)", "new Array(1000).fill(0).length", "1000"},
		{"G3 sparse forEach", "Array(2 ** 31).forEach(() => 1)", "let n = 0; [1, 2, 3].forEach(() => n++); n", "3"},
		{"G3 Array.from", "Array.from({length: 2 ** 23})", "Array.from({length: 3}).length", "3"},
		{"G3 flat nesting", "let a = []; for (let i = 0; i < 1000; i++) a = [a]; a.flat(Infinity)", "[[1, [2]], 3].flat(Infinity)", "[1,2,3]"},
		{"G4 Float64Array", "new Float64Array(2 ** 25)", "new Float64Array(1000).length", "1000"},
		{"G4 ArrayBuffer", "new ArrayBuffer(2 ** 25)", "new ArrayBuffer(100).byteLength", "100"},
		{"G5 apply", "Math.max.apply(null, {length: 70000})", "Math.max.apply(null, [1, 5, 2])", "5"},
		{"G6 JSON.parse", "JSON.parse(' '.repeat(2 ** 20))", "JSON.parse('[1,2]').length", "2"},
		{"G7 JSON.stringify", "JSON.stringify(Array(2000).fill('x'.repeat(2 ** 14)))", "JSON.stringify(Array(10).fill('x')).length", "41"},
		{"G8 asUintN", "BigInt.asUintN(2 ** 40, -1n)", "BigInt.asUintN(64, -1n)", "18446744073709551615"},
		{"G9 split", "'x'.repeat(2 ** 22).split('')", "'abc'.split('').length", "3"},
	}
	e := testEngine()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := e.Exec(context.Background(), Run{Source: "return (" + wrapStmt(c.over) + ")", Limits: guardLimits})
			var se *Error
			if !errors.As(err, &se) || se.Kind != KindMemory || (!raceEnabled && (se.Line == 0 || !strings.Contains(se.Message, "memory limit"))) {
				t.Fatalf("over: %v", err)
			}
			res, err := e.Exec(context.Background(), Run{Source: "return (" + wrapStmt(c.under) + ")", Limits: guardLimits})
			if err != nil || string(res.JSON) != c.want {
				t.Fatalf("under: %s %v", res.JSON, err)
			}
		})
	}
}

// wrapStmt turns a statement list ending in an expression into an IIFE returning it.
func wrapStmt(s string) string {
	i := strings.LastIndex(s, "; ")
	if i < 0 {
		return s
	}
	return "(() => { " + s[:i+1] + " return " + s[i+2:] + " })()"
}

func TestInputLimits(t *testing.T) {
	spawned := atomic.Int32{}
	e := newEngine(func() *exec.Cmd { spawned.Add(1); return exec.Command(os.Args[0], "js-worker") })
	big := json.RawMessage(`"` + strings.Repeat("x", 5<<20) + `"`)
	_, err := e.Exec(context.Background(), Run{Source: "return 1", Bindings: []Binding{{Name: "v", Value: big}}})
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindInput {
		t.Fatalf("over input cap: %v", err)
	}
	mid := json.RawMessage(`"` + strings.Repeat("x", 3<<20) + `"`)
	_, err = e.Exec(context.Background(), Run{Source: "return 1", Limits: small, Bindings: []Binding{{Name: "v", Value: mid}}})
	if !errors.As(err, &se) || se.Kind != KindInput || !strings.Contains(se.Message, "MemoryBytes") {
		t.Fatalf("input x24 over memory: %v", err)
	}
	if spawned.Load() != 0 {
		t.Fatal("spawned a worker for refused input")
	}
	if _, err := e.Exec(context.Background(), Run{Source: "return 1", Limits: Limits{Timeout: time.Hour}}); err == nil || errors.As(err, &se) {
		t.Fatalf("limit above ceiling must be a plain error: %v", err)
	}
}

func TestConcurrencySlots(t *testing.T) {
	spawned := make(chan struct{}, 16)
	e := newEngine(func() *exec.Cmd { spawned <- struct{}{}; return exec.Command(os.Args[0], "js-worker") })
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	b := []Binding{{Name: "h", Funcs: map[string]HostFunc{"wait": func(context.Context, json.RawMessage) (json.RawMessage, error) {
		started <- struct{}{}
		<-release
		return nil, nil
	}}}}
	done := make(chan error, 5)
	for range MaxConcurrent {
		go func() {
			_, err := e.Exec(context.Background(), Run{Source: "h.wait(); return 1", Bindings: b, Limits: Limits{Timeout: 10 * time.Second}})
			done <- err
		}()
	}
	for range MaxConcurrent {
		<-started
	}
	for range MaxConcurrent {
		<-spawned
	}
	go func() {
		_, err := e.Exec(context.Background(), Run{Source: "return 2", Limits: Limits{Timeout: 10 * time.Second}})
		done <- err
	}()
	select {
	case <-spawned:
		t.Fatal("fifth run spawned while four slots were held")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	for range MaxConcurrent + 1 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	// A waiter honours ctx.
	for range MaxConcurrent {
		e.sem <- struct{}{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := e.Exec(ctx, Run{Source: "return 1"})
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindCanceled {
		t.Fatalf("slot wait: %v", err)
	}
}
