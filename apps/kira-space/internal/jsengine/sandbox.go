package jsengine

import (
	_ "embed"
	"errors"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
)

//go:embed freeze.js
var freezeSrc string

// newVM builds the sandboxed runtime: no host objects, deterministic clock and PRNG, dynamic code
// off, guards installed, bindings read-only, every intrinsic frozen.
func (st *runState) newVM(req *workerRequest) *Error {
	vm := goja.New()
	st.vm = vm
	// Source maps off: goja's default loader reads the path in a //# sourceMappingURL comment.
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	vm.SetMaxCallStackSize(maxCallStack)
	now := time.UnixMilli(req.NowMs).UTC()
	vm.SetTimeSource(func() time.Time { return now })
	rng := rand.New(rand.NewPCG(req.Seed, 0x6b6972612d6a73))
	vm.SetRandSource(rng.Float64)

	for _, n := range []string{"RangeError", "SyntaxError", "TypeError", "EvalError", "Error"} {
		st.ctors[n] = vm.Get(n)
	}
	jsonObj, ok := vm.Get("JSON").(*goja.Object)
	if !ok {
		return newError(KindInternal, "JSON missing")
	}
	st.jsonObj = jsonObj
	var okFn bool
	if st.origStringify, okFn = goja.AssertFunction(jsonObj.Get("stringify")); !okFn {
		return newError(KindInternal, "JSON.stringify missing")
	}

	if reflectObj, isObj := vm.Get("Reflect").(*goja.Object); isObj {
		st.gopd, _ = goja.AssertFunction(reflectObj.Get("getOwnPropertyDescriptor"))
	}
	if st.gopd == nil {
		return newError(KindInternal, "Reflect.getOwnPropertyDescriptor missing")
	}

	global := vm.GlobalObject()
	if err := global.Delete("eval"); err != nil {
		return newError(KindInternal, "cannot remove eval")
	}
	if err := st.blockDynamicCode(); err != nil {
		return newError(KindInternal, "cannot block dynamic code: %v", err)
	}
	st.installGuards()
	if err := st.installBigIntJSON(); err != nil {
		return newError(KindInternal, "cannot install BigInt hook: %v", err)
	}

	intrinsics := slices.DeleteFunc(global.GetOwnPropertyNames(), func(n string) bool { return n == "globalThis" })
	kira := vm.NewObject()
	_ = kira.DefineDataProperty("parseJSON", vm.ToValue(st.parseJSON), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	if err := global.DefineDataProperty("kira", kira, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
		return newError(KindInternal, "cannot define kira")
	}
	if e := st.installBindings(req.Bindings); e != nil {
		return e
	}
	freeze, err := vm.RunString(freezeSrc)
	if err != nil {
		return newError(KindInternal, "freeze setup failed: %v", err)
	}
	fn, _ := goja.AssertFunction(freeze)
	if _, err := fn(goja.Undefined(), global, vm.ToValue(intrinsics), kira); err != nil {
		return newError(KindInternal, "freeze failed: %v", err)
	}
	return nil
}

// blockDynamicCode replaces Function and the constructors of the function prototypes with a
// thrower. The approval card shows the script; code built from data at run time would run logic the
// user never saw. goja has no async generators, so only plain, generator and async remain.
func (st *runState) blockDynamicCode() error {
	vm := st.vm
	thrower := vm.ToValue(func(goja.ConstructorCall) *goja.Object {
		st.throwError("EvalError", "dynamic code is disabled")
		return nil
	}).(*goja.Object)
	orig, ok := vm.Get("Function").(*goja.Object)
	if !ok {
		return errors.New("Function missing")
	}
	proto := orig.Get("prototype")
	_ = thrower.DefineDataProperty("name", vm.ToValue("Function"), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	if err := thrower.DefineDataProperty("prototype", proto, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
		return err
	}
	protos, err := vm.RunString(`[Object.getPrototypeOf(function*(){}), Object.getPrototypeOf(async function(){})]`)
	if err != nil {
		return err
	}
	list := protos.ToObject(vm)
	targets := []*goja.Object{proto.ToObject(vm), list.Get("0").ToObject(vm), list.Get("1").ToObject(vm)}
	for _, t := range targets {
		if err := t.DefineDataProperty("constructor", thrower, goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
			return err
		}
	}
	return vm.GlobalObject().Set("Function", thrower)
}

// installBigIntJSON makes JSON.stringify of a BigInt yield a digit string instead of goja's
// "Do not know how to serialize a BigInt", and marks digits while the output encoder runs so
// exact integer literals can be restored. A replacer cannot do this: toJSON runs before it.
func (st *runState) installBigIntJSON() error {
	vm := st.vm
	bigint, ok := vm.Get("BigInt").(*goja.Object)
	if !ok {
		return errors.New("BigInt missing")
	}
	toJSON := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		s := call.This.String()
		if st.encoding {
			s = st.markerRaw + s
		}
		return vm.ToValue(s)
	}).(*goja.Object)
	_ = toJSON.DefineDataProperty("name", vm.ToValue("toJSON"), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	return bigint.Get("prototype").ToObject(vm).DefineDataProperty("toJSON", toJSON, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
}

// parseJSON is kira.parseJSON: lossless JSON.parse (integers beyond 2^53 become BigInt).
func (st *runState) parseJSON(call goja.FunctionCall) goja.Value {
	text := call.Argument(0).String()
	if len(text) > st.limits.InputBytes {
		st.throwError("RangeError", "kira.parseJSON: text exceeds the input limit")
	}
	if nestingExceeds([]byte(text), MaxJSONDepth) {
		st.throwError("SyntaxError", errJSONDepth.Error())
	}
	v, err := st.fromJSON([]byte(text))
	if err != nil {
		st.throwError("SyntaxError", "kira.parseJSON: "+err.Error())
	}
	return v
}

// throwError throws a JS error of the named built-in constructor.
func (st *runState) throwError(ctor, msg string) {
	obj, err := st.vm.New(st.ctors[ctor], st.vm.ToValue(msg))
	if err != nil {
		panic(st.vm.NewTypeError(msg))
	}
	panic(obj)
}

// throwGuard throws a RangeError "memory limit: ..." and remembers it, so an uncaught one ends the
// run as KindMemory with its position.
func (st *runState) throwGuard(msg string) {
	obj, err := st.vm.New(st.ctors["RangeError"], st.vm.ToValue("memory limit: "+msg))
	if err != nil {
		panic(st.vm.NewTypeError(msg))
	}
	st.guardErrs[obj] = true
	panic(obj)
}

func (st *runState) safeString(v goja.Value) (s string) {
	defer func() {
		if recover() != nil {
			s = "[unprintable]"
		}
	}()
	return v.String()
}
