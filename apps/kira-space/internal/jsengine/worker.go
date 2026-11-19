package jsengine

import (
	"encoding/json"
	"os"
	"runtime/debug"
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
	timer.Stop()
	stopWatch()
	if err != nil {
		return st.finish(workerMsg{Error: fromGoja(err, st.src, st.guardErrs, st.safeString)})
	}
	out, typ, oerr := st.output(res)
	if oerr != nil {
		return st.finish(workerMsg{Error: oerr})
	}
	return st.finish(workerMsg{Result: &resultMsg{JSON: json.RawMessage(out), Type: typ, ElapsedNs: elapsed.Nanoseconds()}})
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
