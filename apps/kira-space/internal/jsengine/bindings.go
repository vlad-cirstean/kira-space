package jsengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"

	"github.com/dop251/goja"
)

// Binding is a read-only global the script sees: an eager JSON value, or an object of host methods
// (Value nil). Values are not frozen; they are per run. ReadBack (Value must be a JSON object)
// installs a null-prototype object whose own keys the run's Result returns after the script ends,
// and refuses a script that declares its own binding of the name.
type Binding struct {
	Name     string
	Value    json.RawMessage
	Funcs    map[string]HostFunc
	ReadBack bool
}

// HostFunc gets the JS call's arguments as one JSON array (big integers lossless) and returns one
// JSON value. An error throws Error(err.Error()) in the script; a panic is recovered and the
// script sees "internal error". It runs in the app process, called over the worker pipe.
type HostFunc func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

var errHostPanic = errors.New("host function panicked") // the panic value is never logged: it could carry data

var (
	bindingNameRE = regexp.MustCompile(`^[a-z][A-Za-z0-9_]{0,31}$`)
	funcNameRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,31}$`)
	reservedWords = []string{
		"await", "break", "case", "catch", "class", "const", "continue", "debugger", "default", "delete",
		"do", "else", "enum", "export", "extends", "false", "finally", "for", "function", "if", "implements",
		"import", "in", "instanceof", "interface", "let", "new", "null", "package", "private", "protected",
		"public", "return", "static", "super", "switch", "this", "throw", "true", "try", "typeof",
		"var", "void", "while", "with", "yield", "undefined", "eval", "arguments", "async", "of", "kira",
	}
)

// validateBindings checks names and values before anything is spawned.
func validateBindings(bs []Binding, inputBytes int) *Error {
	seen := map[string]bool{}
	total := 0
	for _, b := range bs {
		switch {
		case !bindingNameRE.MatchString(b.Name):
			return newError(KindInput, "binding name %q must match %s", b.Name, bindingNameRE)
		case slices.Contains(reservedWords, b.Name) && !(b.Name == "kira" && b.Funcs != nil):
			return newError(KindInput, "binding name %q is reserved", b.Name)
		case seen[b.Name]:
			return newError(KindInput, "binding %q is defined twice", b.Name)
		case (b.Funcs == nil) == (len(b.Value) == 0):
			return newError(KindInput, "binding %q needs exactly one of Value and Funcs", b.Name)
		case b.ReadBack && !isJSONObject(b.Value):
			return newError(KindInput, "read-back binding %q needs a JSON object value", b.Name)
		}
		seen[b.Name] = true
		for fn := range b.Funcs {
			if !funcNameRE.MatchString(fn) || fn == "__proto__" {
				return newError(KindInput, "binding %q method name %q is invalid", b.Name, fn)
			}
		}
		if len(b.Value) > 0 {
			if !json.Valid(b.Value) {
				return newError(KindInput, "binding %q is not valid JSON", b.Name)
			}
			if nestingExceeds(b.Value, MaxJSONDepth) {
				return newError(KindInput, "binding %q nests deeper than %d", b.Name, MaxJSONDepth)
			}
		}
		total += len(b.Value)
	}
	if total > inputBytes {
		return newError(KindInput, "input is %d bytes; the limit is %d", total, inputBytes)
	}
	return nil
}

func isJSONObject(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{'
}

// installBindings defines each binding as a read-only global (worker side).
func (st *runState) installBindings(bs []wireBinding) *Error {
	global := st.vm.GlobalObject()
	for _, b := range bs {
		if b.Name == "kira" && b.Value == nil {
			if err := st.extendKira(global, b); err != nil {
				return err
			}
			continue
		}
		if global.Get(b.Name) != nil {
			return newError(KindInput, "binding name %q is already a global", b.Name)
		}
		v, err := st.bindingValue(b)
		if err != nil {
			return err
		}
		if err := global.DefineDataProperty(b.Name, v, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
			return newError(KindInternal, "binding %q: %v", b.Name, err)
		}
	}
	return nil
}

// extendKira joins host methods to the sandbox's own kira object (parseJSON); freeze seals it afterwards.
func (st *runState) extendKira(global *goja.Object, b wireBinding) *Error {
	kira := global.Get("kira").ToObject(st.vm)
	for _, fn := range b.Funcs {
		if fn == "parseJSON" || kira.Get(fn) != nil {
			return newError(KindInput, "kira method %q is already defined", fn)
		}
		_ = kira.DefineDataProperty(fn, st.namedHostFunc(b.Name, fn), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	return nil
}

// bindingValue builds a binding's value: decoded JSON, or an object of host functions.
func (st *runState) bindingValue(b wireBinding) (goja.Value, *Error) {
	if b.Value == nil {
		obj := st.vm.NewObject()
		for _, fn := range b.Funcs {
			_ = obj.DefineDataProperty(fn, st.namedHostFunc(b.Name, fn), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
		}
		return obj, nil
	}
	v, err := st.fromJSON(b.Value)
	if err != nil {
		return nil, newError(KindInput, "binding %q: %v", b.Name, err)
	}
	if b.ReadBack {
		if err := v.(*goja.Object).SetPrototype(nil); err != nil {
			return nil, newError(KindInternal, "binding %q: %v", b.Name, err)
		}
	}
	return v, nil
}

// namedHostFunc wraps hostFunc as a JS function whose name property is fn.
func (st *runState) namedHostFunc(binding, fn string) goja.Value {
	f := st.vm.ToValue(st.hostFunc(binding, fn))
	_ = f.(*goja.Object).DefineDataProperty("name", st.vm.ToValue(fn), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	return f
}

// hostFunc is the worker-side trampoline: encode arguments, send a call line, wait for the reply
// or the deadline.
func (st *runState) hostFunc(binding, fn string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if st.hostCalls++; st.hostCalls > MaxHostCalls {
			st.throwError("Error", fmt.Sprintf("host call limit exceeded (%d per run)", MaxHostCalls))
		}
		args := make([]any, len(call.Arguments))
		for i, a := range call.Arguments {
			args[i] = a
		}
		text, eerr := st.encode(st.vm.NewArray(args...), st.limits.InputBytes)
		if eerr != nil {
			st.throwError("RangeError", "host call arguments: "+eerr.Message)
		}
		st.send(workerMsg{Call: &callMsg{ID: st.hostCalls, Binding: binding, Fn: fn, Args: json.RawMessage(text)}})
		reply := st.awaitReply()
		if reply.Error != "" {
			st.throwError("Error", reply.Error)
		}
		v, err := st.fromJSON(reply.Result)
		if err != nil {
			st.throwError("Error", "host function returned invalid JSON: "+err.Error())
		}
		return v
	}
}

// callHost runs a HostFunc in the parent, recovering a panic into an error reply.
func callHost(ctx context.Context, b map[string]Binding, c *callMsg, inputBytes int) (reply replyMsg) {
	reply.ID = c.ID
	defer func() {
		if r := recover(); r != nil {
			slog.Error("host function panicked", "scope", "jsengine", "err", errHostPanic)
			reply = replyMsg{ID: c.ID, Error: "internal error"}
		}
	}()
	fn := b[c.Binding].Funcs[c.Fn]
	if fn == nil {
		return replyMsg{ID: c.ID, Error: "internal error"}
	}
	res, err := fn(ctx, c.Args)
	switch {
	case err != nil:
		reply.Error = capMessage(err.Error())
	case len(res) == 0:
		reply.Result = json.RawMessage("null")
	case len(res) > inputBytes:
		reply.Error = fmt.Sprintf("host result is %d bytes; the limit is %d", len(res), inputBytes)
	case !json.Valid(res) || nestingExceeds(res, MaxJSONDepth):
		reply.Error = "host function returned invalid JSON or nesting deeper than 256"
	default:
		reply.Result = res
	}
	return reply
}
