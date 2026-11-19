package jsengine

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// Kind classifies a script failure.
type Kind string

const (
	KindSyntax   Kind = "syntax"
	KindRuntime  Kind = "runtime"
	KindTimeout  Kind = "timeout"
	KindCanceled Kind = "canceled"
	KindMemory   Kind = "memory"
	KindStack    Kind = "stack"
	KindSource   Kind = "source"
	KindInput    Kind = "input"
	KindOutput   Kind = "output"
	KindInternal Kind = "internal"
)

const (
	maxMessageBytes = 2 << 10
	maxFrames       = 10
)

// Error is a structured script failure. Line and Column are 1-based in the caller's source, 0 when
// unknown. Messages can echo data the script handled; callers mask them like any agent-bound text.
type Error struct {
	Kind    Kind    `json:"kind"`
	Message string  `json:"message"`
	Line    int     `json:"line,omitempty"`
	Column  int     `json:"column,omitempty"`
	Stack   []Frame `json:"stack,omitempty"`
}

// Frame is one user-source call frame.
type Frame struct {
	Function string `json:"function,omitempty"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("script %s error at line %d, column %d: %s", e.Kind, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("script %s error: %s", e.Kind, e.Message)
}

func newError(kind Kind, format string, a ...any) *Error {
	return &Error{Kind: kind, Message: capMessage(fmt.Sprintf(format, a...))}
}

func capMessage(s string) string {
	if len(s) <= maxMessageBytes {
		return s
	}
	s = s[:maxMessageBytes]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// Interrupt values passed to goja's Interrupt.
type interruptKind int

const (
	interruptTimeout interruptKind = iota + 1
	interruptMemory
)

const scriptName = "script"

// userFrames maps goja frames to user-source frames: script file only, wrapper line removed.
func userFrames(frames []goja.StackFrame, src srcInfo) []Frame {
	var out []Frame
	for i := range frames {
		f := &frames[i]
		if f.SrcName() != scriptName || strings.HasPrefix(f.FuncName(), "__kira_") {
			continue
		}
		p := f.Position()
		line := p.Line - wrapperLines
		if line < 1 || line > src.lines {
			continue
		}
		name := f.FuncName()
		if name == "<anonymous>" {
			name = ""
		}
		out = append(out, Frame{Function: name, Line: line, Column: p.Column})
	}
	return out
}

func withFrames(e *Error, frames []Frame) *Error {
	if len(frames) > 0 {
		e.Line, e.Column = frames[0].Line, frames[0].Column
		if len(frames) > maxFrames {
			frames = frames[:maxFrames]
		}
		e.Stack = frames
	}
	return e
}

// fromGoja maps an error returned by goja to a structured Error. guards holds the RangeError
// objects the size guards threw.
func fromGoja(err error, src srcInfo, guards map[*goja.Object]bool, safeString func(goja.Value) string) *Error {
	var syn *goja.CompilerSyntaxError
	if errors.As(err, &syn) {
		return syntaxError(syn, src)
	}
	var intr *goja.InterruptedError
	if errors.As(err, &intr) {
		frames := userFrames(intr.Stack(), src)
		switch intr.Value() {
		case interruptTimeout:
			return withFrames(newError(KindTimeout, "script exceeded its time limit"), frames)
		case interruptMemory:
			return withFrames(newError(KindMemory, "script exceeded its memory limit"), frames)
		}
		return withFrames(newError(KindInternal, "script interrupted"), frames)
	}
	var so *goja.StackOverflowError
	if errors.As(err, &so) {
		return withFrames(newError(KindStack, "call stack too deep (limit %d frames)", maxCallStack), userFrames(so.Stack(), src))
	}
	var exc *goja.Exception
	if errors.As(err, &exc) {
		frames := userFrames(exc.Stack(), src)
		val := exc.Value()
		kind := KindRuntime
		if obj, ok := val.(*goja.Object); ok && guards[obj] {
			kind = KindMemory
		}
		return withFrames(newError(kind, "%s", describeThrown(val, safeString)), frames)
	}
	return newError(KindInternal, "internal error")
}

func describeThrown(val goja.Value, safeString func(goja.Value) string) string {
	if val == nil {
		return "Uncaught undefined"
	}
	if obj, ok := val.(*goja.Object); ok {
		msg := obj.Get("message")
		if msg != nil && !goja.IsUndefined(msg) {
			name := "Error"
			if n := obj.Get("name"); n != nil && !goja.IsUndefined(n) {
				name = safeString(n)
			}
			return name + ": " + safeString(msg)
		}
	}
	return "Uncaught " + safeString(val)
}

func syntaxError(syn *goja.CompilerSyntaxError, src srcInfo) *Error {
	e := newError(KindSyntax, "SyntaxError: %s", syn.Message)
	if syn.File == nil {
		return e
	}
	p := syn.File.Position(syn.Offset)
	e.Line, e.Column = p.Line-wrapperLines, p.Column
	if e.Line < 1 {
		e.Line = 1
	}
	if e.Line > src.lines { // error at the wrapper's closing line: clamp to the end of the source
		e.Line, e.Column = src.lines, src.lastCol
	}
	return e
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

var exitStatusRE = regexp.MustCompile(`exit status (\d+)`)

// exitError maps a worker that ended without a final line. diag is workerproc's Diagnostics text;
// it is parsed, never returned: a Go panic value could carry script data. The crash dump head
// has no "fatal error:" line (the runtime prints that to stderr only), so frames also count.
func exitError(diag string) *Error {
	code := -1
	if m := exitStatusRE.FindStringSubmatch(diag); m != nil {
		fmt.Sscanf(m[1], "%d", &code)
	}
	switch {
	case code == exitMemory, containsAny(diag, "out of memory", "runtime.sysMap", "runtime.(*mheap).grow"):
		return newError(KindMemory, "script exceeded its memory limit")
	case containsAny(diag, "stack exceeds", "stack overflow", "runtime.newstack"):
		return newError(KindStack, "script recursed too deeply in a built-in")
	}
	return newError(KindInternal, "script worker failed")
}
