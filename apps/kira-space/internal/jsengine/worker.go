package jsengine

import (
	"encoding/json"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/dlclark/regexp2/v2"
	"github.com/dop251/goja"
)

// runState is one run's worker-side state.
type runState struct {
	vm        *goja.Runtime
	limits    Limits
	src       srcInfo
	markerRaw string // NUL + hex, the string BigInt.prototype.toJSON emits while encoding
	markerEsc string // `"\u0000` + hex, how the marker starts in encoded JSON text
	encoding  bool
	overflow  bool
	hostCalls int
	deadline  time.Time
	joinDepth int

	origStringify goja.Callable
	gopd          goja.Callable // Reflect.getOwnPropertyDescriptor, captured before the script runs
	jsonObj       *goja.Object
	ctors         map[string]goja.Value
	guardErrs     map[*goja.Object]bool

	enc     *json.Encoder
	replies chan replyMsg
}

// RunWorker is the js-worker subcommand: read one request, run it, reply, exit. Nothing but
// protocol lines reaches stdout.
func RunWorker(args []string) int {
	debug.SetMaxStack(maxGoStack)
	// fd 3 is the parent's crash pipe; absent when run by hand, then the call just fails.
	_ = debug.SetCrashOutput(os.NewFile(3, "crash"), debug.CrashOptions{})
	time.Local = time.UTC
	enc := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(os.Stdin)
	if err := enc.Encode(workerMsg{Ready: true}); err != nil {
		return 1
	}
	var req workerRequest
	if err := dec.Decode(&req); err != nil {
		return 1
	}
	received := time.Now()
	st := &runState{
		limits:    req.Limits,
		src:       newSrcInfo(req.Source),
		markerRaw: "\x00" + req.Marker,
		markerEsc: `"\u0000` + req.Marker,
		deadline:  received.Add(time.Duration(req.TimeoutNs)),
		ctors:     map[string]goja.Value{},
		guardErrs: map[*goja.Object]bool{},
		enc:       enc,
		replies:   make(chan replyMsg, 1),
	}
	go func() {
		for {
			var m parentMsg
			if err := dec.Decode(&m); err != nil {
				close(st.replies)
				return
			}
			if m.Reply != nil {
				st.replies <- *m.Reply
			}
		}
	}()
	// Catastrophic regexps surface as a catchable RangeError instead of never returning; half the
	// time left, so an uncaught one ends the run before the deadline interrupt.
	regexp2.DefaultMatchTimeout = max(time.Until(st.deadline)/2, time.Millisecond)

	if e := st.newVM(&req); e != nil {
		return st.finish(workerMsg{Error: e})
	}
	baseHeap, baseTotal := heapStats()
	debug.SetMemoryLimit(int64(baseTotal) + int64(st.limits.MemoryBytes))
	stopWatch := startWatchdog(st.vm, baseHeap, uint64(st.limits.MemoryBytes), baseTotal+uint64(st.limits.hardCeiling()))
	timer := time.AfterFunc(time.Until(st.deadline), func() { st.vm.Interrupt(interruptTimeout) })
	// Compile under the watchdog: goja folds constant expressions while compiling.
	prg, cerr := compileSource(req.Source)
	if cerr != nil {
		stopWatch()
		timer.Stop()
		return st.finish(workerMsg{Error: cerr})
	}

	start := time.Now()
	res, err := st.run(prg)
	elapsed := time.Since(start)
	var readBack map[string]ReadBack
	var rerr *Error
	if err == nil {
		// Under the deadline and memory watchdog: reading back runs user getters.
		readBack, rerr = st.readBack(req.Bindings)
	}
	timer.Stop()
	stopWatch()
	if err != nil {
		return st.finish(workerMsg{Error: fromGoja(err, st.src, st.guardErrs, st.safeString)})
	}
	if rerr != nil {
		return st.finish(workerMsg{Error: rerr})
	}
	out, typ, oerr := st.output(res)
	if oerr != nil {
		return st.finish(workerMsg{Error: oerr})
	}
	return st.finish(workerMsg{Result: &resultMsg{JSON: json.RawMessage(out), Type: typ, ElapsedNs: elapsed.Nanoseconds(), ReadBack: readBack}})
}

func (st *runState) run(prg *goja.Program) (goja.Value, error) {
	v, err := st.vm.RunProgram(prg)
	if err != nil {
		return nil, err
	}
	fn, ok := goja.AssertFunction(v)
	if !ok {
		return nil, newError(KindInternal, "script wrapper is not a function")
	}
	return fn(goja.Undefined())
}

// readBack reports the own enumerable string keys of each read-back global, under the per-value
// and key caps rather than the result cap. A key holding a function, symbol, undefined or an
// accessor goes to Invalid with its JS type.
func (st *runState) readBack(bs []wireBinding) (map[string]ReadBack, *Error) {
	var out map[string]ReadBack
	keys := 0
	for _, b := range bs {
		if !b.ReadBack {
			continue
		}
		obj, ok := st.vm.GlobalObject().Get(b.Name).(*goja.Object)
		if !ok {
			continue
		}
		rb := ReadBack{JSON: map[string]json.RawMessage{}, Invalid: map[string]string{}}
		for _, k := range obj.Keys() {
			desc, derr := st.gopd(goja.Undefined(), obj, st.vm.ToValue(k))
			if derr == nil {
				if d, isObj := desc.(*goja.Object); isObj && (d.Get("get") != nil || d.Get("set") != nil) {
					rb.Invalid[k] = "accessor"
					continue
				}
			}
			v := obj.Get(k)
			if typ := invalidType(v); typ != "" {
				rb.Invalid[k] = typ
				continue
			}
			if keys++; keys > ReadBackMaxKeys {
				return nil, newError(KindSaved, "a script can hold at most %d saved values", ReadBackMaxKeys)
			}
			text, eerr := st.encode(v, ReadBackValueBytes)
			if eerr != nil {
				if strings.HasPrefix(eerr.Message, "result exceeds") {
					return nil, newError(KindSaved, "%s.%s is over %d bytes; a saved value is capped at %d bytes", b.Name, k, ReadBackValueBytes, ReadBackValueBytes)
				}
				return nil, newError(KindSaved, "%s.%s cannot be saved: %s", b.Name, k, strings.TrimPrefix(eerr.Message, "result "))
			}
			rb.JSON[k] = json.RawMessage(text)
		}
		if out == nil {
			out = map[string]ReadBack{}
		}
		out[b.Name] = rb
	}
	return out, nil
}

// invalidType names the JS type of a value that has no JSON form, or "".
func invalidType(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if _, ok := v.(*goja.Symbol); ok {
		return "symbol"
	}
	if _, ok := goja.AssertFunction(v); ok {
		return "function"
	}
	return ""
}

// send writes one line; a failed write means the parent is gone.
func (st *runState) send(m workerMsg) {
	if err := st.enc.Encode(m); err != nil {
		os.Exit(1)
	}
}

// finish sends the final line, then waits for the parent to close stdin: exiting first could lose
// the line when the parent reaps the process.
func (st *runState) finish(m workerMsg) int {
	st.send(m)
	t := time.NewTimer(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case _, ok := <-st.replies:
			if !ok {
				return 0
			}
		case <-t.C:
			return 0
		}
	}
}

func (st *runState) awaitReply() replyMsg {
	t := time.NewTimer(time.Until(st.deadline))
	select {
	case m, ok := <-st.replies:
		t.Stop()
		if !ok {
			os.Exit(1)
		}
		return m
	case <-t.C:
		// A stuck host function cannot hold the worker past the deadline; the VM is inside a
		// native call, so report from here.
		e := withFrames(newError(KindTimeout, "script exceeded its time limit"), userFrames(st.vm.CaptureCallStack(0, nil), st.src))
		os.Exit(st.finish(workerMsg{Error: e}))
		return replyMsg{}
	}
}

// output encodes the script's return value.
func (st *runState) output(v goja.Value) (text, typ string, err *Error) {
	if v == nil || goja.IsUndefined(v) {
		return "null", "undefined", nil
	}
	if goja.IsNull(v) {
		return "null", "null", nil
	}
	if obj, ok := v.(*goja.Object); ok {
		if _, isPromise := obj.Export().(*goja.Promise); isPromise {
			return "", "", newError(KindOutput, "script returned a Promise; return a plain value")
		}
		if then, isFn := goja.AssertFunction(obj.Get("then")); isFn && then != nil {
			return "", "", newError(KindOutput, "script returned a Promise; return a plain value")
		}
		if _, isFn := goja.AssertFunction(v); isFn {
			return "", "", newError(KindOutput, "script returned a function; return a plain value")
		}
		typ = "object"
		if obj.ClassName() == "Array" {
			typ = "array"
		}
	} else if _, ok := v.(*goja.Symbol); ok {
		return "", "", newError(KindOutput, "script returned a symbol; return a plain value")
	} else {
		switch v.ExportType().Kind().String() {
		case "bool":
			typ = "boolean"
		case "string":
			typ = "string"
		case "ptr":
			typ = "bigint"
		default:
			typ = "number"
		}
	}
	text, err = st.encode(v, st.limits.OutputBytes)
	return text, typ, err
}
