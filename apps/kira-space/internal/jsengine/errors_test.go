package jsengine

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSyntaxPositions(t *testing.T) {
	cases := []struct {
		name, src string
		line, col int
	}{
		{"first line", "var = 1;", 1, 5},
		{"middle line", "var a = 1;\nvar b = ;\nreturn b;", 2, 9},
		{"unterminated brace at end", "if (true) {\n  return 1;", 2, 12},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ce := Check(c.src)
			wantKind(t, ce, KindSyntax)
			if ce.Line != c.line || ce.Column != c.col {
				t.Fatalf("Check at %d:%d (%s), want %d:%d", ce.Line, ce.Column, ce.Message, c.line, c.col)
			}
			_, ee := exec1(t, Run{Source: c.src, Limits: small})
			wantKind(t, ee, KindSyntax)
			if ee.Line != ce.Line || ee.Column != ce.Column || ee.Message != ce.Message {
				t.Fatalf("Exec %v differs from Check %v", ee, ce)
			}
		})
	}
}

func TestRuntimeFrames(t *testing.T) {
	src := "function a() {\n  return b();\n}\nfunction b() {\n  throw new Error('deep');\n}\nreturn a();"
	_, e := exec1(t, Run{Source: src, Limits: small})
	wantKind(t, e, KindRuntime)
	if e.Message != "Error: deep" || e.Line != 5 || e.Column == 0 {
		t.Fatalf("%+v", e)
	}
	if len(e.Stack) < 3 || e.Stack[0].Function != "b" || e.Stack[0].Line != 5 || e.Stack[1].Function != "a" || e.Stack[1].Line != 2 || e.Stack[2].Line != 7 {
		t.Fatalf("stack %+v", e.Stack)
	}
	for _, f := range e.Stack {
		if strings.HasPrefix(f.Function, "__kira_") {
			t.Fatalf("internal frame leaked: %+v", f)
		}
	}
	_, e = exec1(t, Run{Source: "const x = 1;\nreturn kira.parseJSON('{bad');", Limits: small})
	wantKind(t, e, KindRuntime)
	if e.Line != 2 || e.Column != 22 {
		t.Fatalf("call site not on top: %+v", e)
	}
	_, e = exec1(t, Run{Source: "throw new Error('x'.repeat(5000))", Limits: small})
	wantKind(t, e, KindRuntime)
	if len(e.Message) > maxMessageBytes {
		t.Fatalf("message %d bytes", len(e.Message))
	}
	_, e = exec1(t, Run{Source: "throw null", Limits: small})
	if e == nil || e.Message != "Uncaught null" {
		t.Fatalf("%v", e)
	}
}

func TestSourceChecks(t *testing.T) {
	cases := []struct {
		name, src string
		line, col int
	}{
		{"nul", "return 1;\n\x00", 2, 1},
		{"invalid utf8", "return '\xff'", 1, 9},
		{"right-to-left override", "return 'a‮b'", 1, 10},
		{"isolate", "return 1;\n  // ⁧", 2, 6},
		{"over 64 KiB", strings.Repeat("a", MaxSourceBytes+1), 0, 0},
		{"blank", " \n\t", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Check(c.src)
			wantKind(t, e, KindSource)
			if e.Line != c.line || e.Column != c.col {
				t.Fatalf("at %d:%d, want %d:%d (%s)", e.Line, e.Column, c.line, c.col, e.Message)
			}
		})
	}
	start := time.Now()
	if e := Check("for (;;) {}"); e != nil || time.Since(start) > time.Second {
		t.Fatalf("Check ran the script: %v", e)
	}
}

func TestErrorString(t *testing.T) {
	if got := (&Error{Kind: KindRuntime, Message: "m", Line: 2, Column: 3}).Error(); got != "script runtime error at line 2, column 3: m" {
		t.Fatal(got)
	}
	if got := (&Error{Kind: KindTimeout, Message: "slow"}).Error(); got != "script timeout error: slow" {
		t.Fatal(got)
	}
}

func TestWorkerExitMapping(t *testing.T) {
	cases := []struct {
		name, script string
		kind         Kind
	}{
		{"memory exit code", `exit 75`, KindMemory},
		{"out of memory header", `echo "fatal error: runtime: out of memory" >&2; exit 2`, KindMemory},
		{"stack exceeds", `echo "runtime: goroutine stack exceeds 67108864-byte limit" >&2; exit 2`, KindStack},
		{"plain exit", `exit 1`, KindInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEngine(func() *exec.Cmd { return exec.Command("sh", "-c", c.script) })
			_, err := e.Exec(context.Background(), Run{Source: "return 1"})
			se, ok := err.(*Error)
			if !ok || se.Kind != c.kind {
				t.Fatalf("got %v", err)
			}
			if se.Line != 0 || strings.Contains(se.Message, "goroutine") || strings.Contains(se.Message, "fatal") {
				t.Fatalf("raw worker text returned: %+v", se)
			}
		})
	}
	_ = os.Args
}
