package jsengine

import (
	"fmt"
	"strconv"

	"github.com/dop251/goja"
)

// Guards turn the common single-call blow-ups into a positioned, catchable RangeError before
// anything is allocated. The worker ceilings stay the safety net; guards give clear errors.
// Sizes: strings count 2 bytes per code unit (rope length reads without flattening), array slots 16.
const (
	maxArrayLen   = 1 << 24
	slotBytes     = 16
	maxApplyArgs  = 65536
	maxBigIntBits = 1 << 30
)

var (
	// Array methods that build a result of about this.length slots.
	allocatingArrayMethods = []string{
		"fill", "map", "filter", "flat", "flatMap", "slice", "splice", "sort", "reverse", "with",
		"toSorted", "toReversed", "toSpliced", "copyWithin", "concat",
	}
	// Array methods that only walk this.length elements: a huge sparse length never returns.
	walkingArrayMethods = []string{
		"forEach", "every", "some", "reduce", "reduceRight", "indexOf", "lastIndexOf", "includes",
		"find", "findIndex", "findLast", "findLastIndex",
	}
	typedArrayNames = []string{
		"Int8Array", "Uint8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array", "Int32Array",
		"Uint32Array", "Float16Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array",
	}
)

func (st *runState) installGuards() {
	vm := st.vm
	obj := func(v goja.Value) *goja.Object {
		o, _ := v.(*goja.Object)
		return o
	}
	global := func(name string) *goja.Object { return obj(vm.Get(name)) }
	proto := func(name string) *goja.Object {
		if c := global(name); c != nil {
			return obj(c.Get("prototype"))
		}
		return nil
	}
	callCap := int64(st.limits.callCap())

	if sp := proto("String"); sp != nil {
		st.guardG1(sp, callCap)
		st.guard(sp, "split", func(call goja.FunctionCall) {
			if s, ok := call.Argument(0).(goja.String); ok && s.Length() == 0 && goja.IsUndefined(call.Argument(1)) {
				if st.strLen(call.This)*64 > callCap {
					st.throwGuard("String.prototype.split('') result is too large")
				}
			}
		})
	}
	if ap := proto("Array"); ap != nil {
		st.guardJoin(ap, callCap)
		for _, name := range allocatingArrayMethods {
			st.guard(ap, name, func(call goja.FunctionCall) {
				n := st.lengthOf(call.This)
				if name == "concat" {
					for _, a := range call.Arguments {
						if o, ok := a.(*goja.Object); ok && o.ClassName() == "Array" {
							n += st.lengthOf(o)
						} else {
							n++
						}
					}
				}
				st.checkSlots(name, n, callCap)
			})
		}
		for _, name := range walkingArrayMethods {
			st.guard(ap, name, func(call goja.FunctionCall) { st.checkLen(name, st.lengthOf(call.This)) })
		}
		for _, name := range []string{"flat", "flatMap"} {
			st.guardFlat(ap, name, callCap)
		}
	}
	if arr := global("Array"); arr != nil {
		st.guard(arr, "from", func(call goja.FunctionCall) {
			if o, ok := call.Argument(0).(*goja.Object); ok && o.Get("length") != nil {
				st.checkSlots("Array.from", st.lengthOf(o), callCap)
			}
		})
	}
	st.guardTypedArrays(callCap)

	checkArgList := func(name string, list goja.Value) {
		if o, ok := list.(*goja.Object); ok && st.lengthOf(o) > maxApplyArgs {
			st.throwGuard(fmt.Sprintf("%s argument list longer than %d", name, maxApplyArgs))
		}
	}
	if fp := proto("Function"); fp != nil {
		st.guard(fp, "apply", func(call goja.FunctionCall) { checkArgList("Function.prototype.apply", call.Argument(1)) })
	}
	if refl := global("Reflect"); refl != nil {
		st.guard(refl, "apply", func(call goja.FunctionCall) { checkArgList("Reflect.apply", call.Argument(2)) })
		st.guard(refl, "construct", func(call goja.FunctionCall) { checkArgList("Reflect.construct", call.Argument(1)) })
	}

	st.guard(st.jsonObj, "parse", func(call goja.FunctionCall) {
		text := call.Argument(0).String()
		if int64(len(text))*memoryPerInput > callCap {
			st.throwGuard(fmt.Sprintf("JSON.parse text of %d bytes is too large", len(text)))
		}
		if nestingExceeds(text, MaxJSONDepth) {
			st.throwError("SyntaxError", errJSONDepth.Error())
		}
	})
	st.guardStringify(callCap)

	if bi := global("BigInt"); bi != nil {
		maxBits := min(int64(maxBigIntBits), callCap*8)
		for _, name := range []string{"asUintN", "asIntN"} {
			st.guard(bi, name, func(call goja.FunctionCall) {
				if call.Argument(0).ToInteger() > maxBits {
					st.throwGuard("BigInt." + name + " width is too large")
				}
			})
		}
	}
}

func (st *runState) lengthOf(v goja.Value) int64 {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return 0
	}
	o := v.ToObject(st.vm)
	l := o.Get("length")
	if l == nil {
		return 0
	}
	return max(l.ToInteger(), 0)
}

func (st *runState) strLen(v goja.Value) int64 {
	if s, ok := v.(goja.String); ok {
		return int64(s.Length())
	}
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return 0
	}
	return int64(len(v.String()))
}

func (st *runState) checkLen(name string, n int64) {
	if n > maxArrayLen {
		st.throwGuard(fmt.Sprintf("%s on a length of %d (limit %d)", name, n, maxArrayLen))
	}
}

func (st *runState) checkSlots(name string, n, callCap int64) {
	st.checkLen(name, n)
	if n*slotBytes > callCap {
		st.throwGuard(fmt.Sprintf("%s would allocate about %d bytes (per-call limit %d)", name, n*slotBytes, callCap))
	}
}

// replace swaps a function property for a Go function that gets the original; name, length and
// attributes are kept.
func (st *runState) replace(holder *goja.Object, name string, impl func(call goja.FunctionCall, orig goja.Callable) goja.Value) {
	if holder == nil {
		return
	}
	origV := holder.Get(name)
	orig, ok := goja.AssertFunction(origV)
	if !ok {
		return
	}
	w := st.vm.ToValue(func(call goja.FunctionCall) goja.Value { return impl(call, orig) }).(*goja.Object)
	o := origV.(*goja.Object)
	_ = w.DefineDataProperty("name", o.Get("name"), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = w.DefineDataProperty("length", o.Get("length"), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = holder.DefineDataProperty(name, w, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
}

// guard runs check, then the original.
func (st *runState) guard(holder *goja.Object, name string, check func(call goja.FunctionCall)) {
	st.replace(holder, name, func(call goja.FunctionCall, orig goja.Callable) goja.Value {
		check(call)
		return callOrig(orig, call)
	})
}

// callOrig calls the original and rethrows what it threw, interrupts included.
func callOrig(orig goja.Callable, call goja.FunctionCall) goja.Value {
	res, err := orig(call.This, call.Arguments...)
	if err != nil {
		panic(err)
	}
	return res
}

// G1: repeat, padStart, padEnd, concat.
func (st *runState) guardG1(sp *goja.Object, callCap int64) {
	tooBig := func(name string) { st.throwGuard("String.prototype." + name + " result is too large") }
	st.guard(sp, "repeat", func(call goja.FunctionCall) {
		n, count := st.strLen(call.This), call.Argument(0).ToInteger()
		if n > 0 && count > 0 && count > callCap/(2*n) {
			tooBig("repeat")
		}
	})
	for _, name := range []string{"padStart", "padEnd"} {
		st.guard(sp, name, func(call goja.FunctionCall) {
			if f, ok := call.Argument(1).(goja.String); ok && f.Length() == 0 {
				return
			}
			if call.Argument(0).ToInteger() > callCap/2 {
				tooBig(name)
			}
		})
	}
	st.guard(sp, "concat", func(call goja.FunctionCall) {
		n := st.strLen(call.This)
		for _, a := range call.Arguments {
			if s, ok := a.(goja.String); ok {
				n += int64(s.Length())
			}
		}
		if n*2 > callCap {
			tooBig("concat")
		}
	})
}

// G2: Array.prototype.join, which toString reaches. Bounds the result and the nesting of join
// inside join (native recursion).
func (st *runState) guardJoin(ap *goja.Object, callCap int64) {
	st.replace(ap, "join", func(call goja.FunctionCall, orig goja.Callable) goja.Value {
		if st.joinDepth++; st.joinDepth > MaxJSONDepth {
			st.joinDepth--
			st.throwGuard(fmt.Sprintf("join nested deeper than %d", MaxJSONDepth))
		}
		defer func() { st.joinDepth-- }()
		if !(goja.IsUndefined(call.This) || goja.IsNull(call.This)) {
			st.checkJoin(call, callCap)
		}
		return callOrig(orig, call)
	})
}

func (st *runState) checkJoin(call goja.FunctionCall, callCap int64) {
	o := call.This.ToObject(st.vm)
	n := st.lengthOf(o)
	st.checkLen("Array.prototype.join", n)
	sep := int64(1)
	if a := call.Argument(0); !goja.IsUndefined(a) {
		sep = st.strLen(a)
	}
	est := max(n-1, 0) * sep * 2
	for i := int64(0); i < n && est <= callCap; i++ {
		switch e := o.Get(strconv.FormatInt(i, 10)).(type) {
		case nil:
		case goja.String:
			est += int64(e.Length()) * 2
		default:
			if !goja.IsUndefined(e) && !goja.IsNull(e) {
				est += 24
			}
		}
	}
	if est > callCap {
		st.throwGuard(fmt.Sprintf("Array.prototype.join result would be over %d bytes", callCap))
	}
}

// guardFlat: depth and produced-element bound, with a Go pre-count that stops at the cap.
func (st *runState) guardFlat(ap *goja.Object, name string, callCap int64) {
	st.guard(ap, name, func(call goja.FunctionCall) {
		n := st.lengthOf(call.This)
		st.checkSlots("Array.prototype."+name, n, callCap)
		depth := int64(1)
		if name == "flat" && !goja.IsUndefined(call.Argument(0)) {
			depth = call.Argument(0).ToInteger()
		}
		if depth > MaxJSONDepth {
			st.throwGuard(fmt.Sprintf("Array.prototype.%s depth over %d", name, MaxJSONDepth))
		}
		if name == "flat" {
			budget := callCap / slotBytes
			if !st.countFlat(call.This.ToObject(st.vm), depth, &budget) {
				st.throwGuard(fmt.Sprintf("Array.prototype.flat result would be over %d bytes", callCap))
			}
		}
	})
}

// countFlat walks nested arrays charging one unit per visited element; false when the budget ran out.
func (st *runState) countFlat(a *goja.Object, depth int64, budget *int64) bool {
	n := st.lengthOf(a)
	for i := int64(0); i < n; i++ {
		if *budget--; *budget < 0 {
			return false
		}
		if depth > 0 {
			if child, ok := a.Get(strconv.FormatInt(i, 10)).(*goja.Object); ok && child.ClassName() == "Array" {
				if !st.countFlat(child, depth-1, budget) {
					return false
				}
			}
		}
	}
	return true
}

// guardTypedArrays is G4: ArrayBuffer and typed-array constructors, resize and %TypedArray%.from.
// slice, map and filter derive from an existing buffer, which already fit under the cap.
func (st *runState) guardTypedArrays(callCap int64) {
	vm := st.vm
	tooBig := func(name string, n int64) {
		st.throwGuard(fmt.Sprintf("%s of %d bytes (per-call limit %d)", name, n, callCap))
	}
	g, _ := vm.Get("ArrayBuffer").(*goja.Object)
	if g != nil {
		st.guardCtor("ArrayBuffer", func(args []goja.Value) {
			if len(args) > 0 && args[0].ToInteger() > callCap {
				tooBig("ArrayBuffer", args[0].ToInteger())
			}
			if len(args) > 1 {
				if o, ok := args[1].(*goja.Object); ok {
					if m := o.Get("maxByteLength"); m != nil && !goja.IsUndefined(m) && m.ToInteger() > callCap {
						tooBig("ArrayBuffer maxByteLength", m.ToInteger())
					}
				}
			}
		})
		if p, _ := g.Get("prototype").(*goja.Object); p != nil {
			st.guard(p, "resize", func(call goja.FunctionCall) {
				if n := call.Argument(0).ToInteger(); n > callCap {
					tooBig("ArrayBuffer.prototype.resize", n)
				}
			})
		}
	}
	for _, name := range typedArrayNames {
		ctor, _ := vm.Get(name).(*goja.Object)
		if ctor == nil {
			continue
		}
		bpe := int64(1)
		if b := ctor.Get("BYTES_PER_ELEMENT"); b != nil {
			bpe = max(b.ToInteger(), 1)
		}
		st.guardCtor(name, func(args []goja.Value) {
			if len(args) == 0 {
				return
			}
			var n int64
			switch a := args[0].(type) {
			case *goja.Object:
				if a.ClassName() == "ArrayBuffer" {
					return
				}
				n = st.lengthOf(a)
			default:
				n = a.ToInteger()
			}
			if n > callCap/bpe {
				tooBig(name, n*bpe)
			}
		})
	}
	// %TypedArray%.from: the intrinsic is the prototype of every typed array constructor.
	if u8, _ := vm.Get("Uint8Array").(*goja.Object); u8 != nil {
		st.guard(u8.Prototype(), "from", func(call goja.FunctionCall) {
			if o, ok := call.Argument(0).(*goja.Object); ok && o.Get("length") != nil {
				bpe := int64(8)
				if c, ok := call.This.(*goja.Object); ok {
					if b := c.Get("BYTES_PER_ELEMENT"); b != nil {
						bpe = max(b.ToInteger(), 1)
					}
				}
				if st.lengthOf(o) > callCap/bpe {
					tooBig("%TypedArray%.from", st.lengthOf(o)*bpe)
				}
			}
		})
	}
}

// guardCtor replaces a global constructor with a checking one that constructs the original through
// Reflect.construct, so subclasses and new.target keep working. Statics, the prototype link and
// the prototype's constructor are carried over.
func (st *runState) guardCtor(name string, check func(args []goja.Value)) {
	vm := st.vm
	origV, ok := vm.Get(name).(*goja.Object)
	if !ok {
		return
	}
	construct, _ := goja.AssertFunction(vm.Get("Reflect").(*goja.Object).Get("construct"))
	var w *goja.Object
	w = vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		check(call.Arguments)
		args := make([]any, len(call.Arguments))
		for i, a := range call.Arguments {
			args[i] = a
		}
		nt := call.NewTarget
		if nt == nil {
			nt = w
		}
		res, err := construct(goja.Undefined(), origV, vm.NewArray(args...), nt)
		if err != nil {
			panic(err)
		}
		return res.ToObject(vm)
	}).(*goja.Object)
	if _, err := vm.RunString(carryOverSrc); err != nil {
		return
	}
	carry, _ := goja.AssertFunction(vm.Get("__kira_carry"))
	_, _ = carry(goja.Undefined(), origV, w, vm.ToValue(name))
	_ = vm.GlobalObject().Delete("__kira_carry")
}

// carryOverSrc defines the helper that copies a constructor's own properties onto its replacement.
const carryOverSrc = `globalThis.__kira_carry = function __kira_carry(orig, w, name) {
	for (const k of Reflect.ownKeys(orig)) {
		Reflect.defineProperty(w, k, Reflect.getOwnPropertyDescriptor(orig, k));
	}
	Reflect.setPrototypeOf(w, Reflect.getPrototypeOf(orig));
	Reflect.defineProperty(orig.prototype, "constructor", { value: w, writable: true, configurable: true });
	Reflect.defineProperty(globalThis, name, { value: w, writable: true, configurable: true });
};`

// guardStringify is G7: JSON.stringify under a counting replacer composed over the caller's.
func (st *runState) guardStringify(callCap int64) {
	st.replace(st.jsonObj, "stringify", func(call goja.FunctionCall, orig goja.Callable) goja.Value {
		vm := st.vm
		gap := 0
		switch s := call.Argument(2).(type) {
		case goja.String:
			gap = min(s.Length(), 10)
		default:
			if s != nil && s.ExportType() != nil && s.ExportType().Kind().String() != "string" {
				gap = int(min(max(s.ToInteger(), 0), 10))
			}
		}
		userFn, _ := goja.AssertFunction(call.Argument(1))
		var allow []string
		allowSet := false
		if o, ok := call.Argument(1).(*goja.Object); ok && userFn == nil && o.ClassName() == "Array" {
			allowSet = true
			seen := map[string]bool{}
			n := st.lengthOf(o)
			for i := int64(0); i < min(n, maxArrayLen); i++ {
				switch e := o.Get(strconv.FormatInt(i, 10)).(type) {
				case goja.String:
					if k := e.String(); !seen[k] {
						seen[k] = true
						allow = append(allow, k)
					}
				default:
					if e != nil && !goja.IsUndefined(e) && !goja.IsNull(e) {
						if _, isObj := e.(*goja.Object); !isObj && e.ExportType().Kind().String() != "bool" {
							if k := e.String(); !seen[k] {
								seen[k] = true
								allow = append(allow, k)
							}
						}
					}
				}
			}
		}
		depths := map[*goja.Object]int{}
		var counted int64
		replacer := vm.ToValue(func(c goja.FunctionCall) goja.Value {
			key, v := c.Argument(0), c.Argument(1)
			if userFn != nil {
				r, err := userFn(c.This, key, v)
				if err != nil {
					panic(err)
				}
				v = r
			}
			holder, _ := c.This.(*goja.Object)
			vo, isObj := v.(*goja.Object)
			if allowSet && isObj && vo.ClassName() == "Object" {
				filtered := vm.NewObject()
				for _, k := range allow {
					if pv := vo.Get(k); pv != nil && !goja.IsUndefined(pv) {
						_ = filtered.DefineDataProperty(k, pv, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
					}
				}
				v, vo = filtered, filtered
			}
			counted += int64(countValue(key, v))
			if gap > 0 {
				d := depths[holder]
				counted += int64(gap * (d + 1))
				if isObj {
					depths[vo] = d + 1
				}
			}
			if counted > callCap {
				st.throwGuard(fmt.Sprintf("JSON.stringify output would be over %d bytes", callCap))
			}
			return v
		})
		res, err := orig(call.This, call.Argument(0), replacer, call.Argument(2))
		if err != nil {
			panic(err)
		}
		return res
	})
}
