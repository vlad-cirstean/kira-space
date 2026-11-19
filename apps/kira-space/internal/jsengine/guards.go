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

func (st *runState) global(name string) *goja.Object {
	o, _ := st.vm.Get(name).(*goja.Object)
	return o
}

func (st *runState) proto(name string) *goja.Object {
	if c := st.global(name); c != nil {
		o, _ := c.Get("prototype").(*goja.Object)
		return o
	}
	return nil
}

func (st *runState) installGuards() {
	callCap := int64(st.limits.callCap())
	st.guardStrings(callCap)
	st.guardArrays(callCap)
	st.guardTypedArrays(callCap)
	st.guardApply()
	st.guardParse(callCap)
	st.guardStringify(callCap)
	st.guardBigInt(callCap)
}

// guardStrings is G1 and G9.
func (st *runState) guardStrings(callCap int64) {
	sp := st.proto("String")
	if sp == nil {
		return
	}
	st.guardG1(sp, callCap)
	st.guard(sp, "split", func(call goja.FunctionCall) {
		if s, ok := call.Argument(0).(goja.String); ok && s.Length() == 0 && goja.IsUndefined(call.Argument(1)) {
			if st.strLen(call.This)*64 > callCap {
				st.throwGuard("String.prototype.split('') result is too large")
			}
		}
	})
}

// guardArrays is G2 and G3.
func (st *runState) guardArrays(callCap int64) {
	if ap := st.proto("Array"); ap != nil {
		st.guardJoin(ap, callCap)
		for _, name := range allocatingArrayMethods {
			st.guard(ap, name, func(call goja.FunctionCall) {
				n := st.lengthOf(call.This)
				if name == "concat" {
					n += st.concatExtra(call.Arguments)
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
	if arr := st.global("Array"); arr != nil {
		st.guard(arr, "from", func(call goja.FunctionCall) {
			if o, ok := call.Argument(0).(*goja.Object); ok && o.Get("length") != nil {
				st.checkSlots("Array.from", st.lengthOf(o), callCap)
			}
		})
	}
}

func (st *runState) concatExtra(args []goja.Value) int64 {
	var n int64
	for _, a := range args {
		if o, ok := a.(*goja.Object); ok && o.ClassName() == "Array" {
			n += st.lengthOf(o)
		} else {
			n++
		}
	}
	return n
}

// guardApply is G5.
func (st *runState) guardApply() {
	checkArgList := func(name string, list goja.Value) {
		if o, ok := list.(*goja.Object); ok && st.lengthOf(o) > maxApplyArgs {
			st.throwGuard(fmt.Sprintf("%s argument list longer than %d", name, maxApplyArgs))
		}
	}
	if fp := st.proto("Function"); fp != nil {
		st.guard(fp, "apply", func(call goja.FunctionCall) { checkArgList("Function.prototype.apply", call.Argument(1)) })
	}
	if refl := st.global("Reflect"); refl != nil {
		st.guard(refl, "apply", func(call goja.FunctionCall) { checkArgList("Reflect.apply", call.Argument(2)) })
		st.guard(refl, "construct", func(call goja.FunctionCall) { checkArgList("Reflect.construct", call.Argument(1)) })
	}
}

// guardParse is G6.
func (st *runState) guardParse(callCap int64) {
	st.guard(st.jsonObj, "parse", func(call goja.FunctionCall) {
		text := call.Argument(0).String()
		if int64(len(text))*memoryPerInput > callCap {
			st.throwGuard(fmt.Sprintf("JSON.parse text of %d bytes is too large", len(text)))
		}
		if nestingExceeds(text, MaxJSONDepth) {
			st.throwError("SyntaxError", errJSONDepth.Error())
		}
	})
}

// guardBigInt is G8.
func (st *runState) guardBigInt(callCap int64) {
	bi := st.global("BigInt")
	if bi == nil {
		return
	}
	maxBits := min(int64(maxBigIntBits), callCap*8)
	for _, name := range []string{"asUintN", "asIntN"} {
		st.guard(bi, name, func(call goja.FunctionCall) {
			if call.Argument(0).ToInteger() > maxBits {
				st.throwGuard("BigInt." + name + " width is too large")
			}
		})
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

// guardFlat: produced-element bound with a Go pre-count that stops at the cap. flat(Infinity)
// stays allowed on shallow data; nesting past MaxJSONDepth would recurse natively, so it is refused.
func (st *runState) guardFlat(ap *goja.Object, name string, callCap int64) {
	st.guard(ap, name, func(call goja.FunctionCall) {
		n := st.lengthOf(call.This)
		st.checkSlots("Array.prototype."+name, n, callCap)
		if name != "flat" {
			return
		}
		depth := int64(1)
		if !goja.IsUndefined(call.Argument(0)) {
			depth = call.Argument(0).ToInteger()
		}
		budget := callCap / slotBytes
		switch st.countFlat(call.This.ToObject(st.vm), depth, 0, &budget) {
		case flatTooDeep:
			st.throwGuard(fmt.Sprintf("Array.prototype.flat nesting deeper than %d", MaxJSONDepth))
		case flatTooBig:
			st.throwGuard(fmt.Sprintf("Array.prototype.flat result would be over %d bytes", callCap))
		}
	})
}

const (
	flatOK = iota
	flatTooDeep
	flatTooBig
)

// countFlat walks nested arrays charging one unit per visited element.
func (st *runState) countFlat(a *goja.Object, depth int64, level int, budget *int64) int {
	if level > MaxJSONDepth {
		return flatTooDeep
	}
	n := st.lengthOf(a)
	for i := int64(0); i < n; i++ {
		if *budget--; *budget < 0 {
			return flatTooBig
		}
		if depth > 0 {
			if child, ok := a.Get(strconv.FormatInt(i, 10)).(*goja.Object); ok && child.ClassName() == "Array" {
				if r := st.countFlat(child, depth-1, level+1, budget); r != flatOK {
					return r
				}
			}
		}
	}
	return flatOK
}

// guardTypedArrays is G4: ArrayBuffer and typed-array constructors, resize and %TypedArray%.from.
// slice, map and filter derive from an existing buffer, which already fit under the cap.
func (st *runState) guardTypedArrays(callCap int64) {
	tooBig := func(name string, n int64) {
		st.throwGuard(fmt.Sprintf("%s of %d bytes (per-call limit %d)", name, n, callCap))
	}
	st.guardBuffer(callCap, tooBig)
	for _, name := range typedArrayNames {
		st.guardTypedCtor(name, callCap, tooBig)
	}
	// %TypedArray%.from: the intrinsic is the prototype of every typed array constructor.
	if u8 := st.global("Uint8Array"); u8 != nil {
		st.guard(u8.Prototype(), "from", func(call goja.FunctionCall) {
			o, ok := call.Argument(0).(*goja.Object)
			if !ok || o.Get("length") == nil {
				return
			}
			bpe := int64(8)
			if c, ok := call.This.(*goja.Object); ok {
				bpe = bytesPerElement(c, bpe)
			}
			if st.lengthOf(o) > callCap/bpe {
				tooBig("%TypedArray%.from", st.lengthOf(o)*bpe)
			}
		})
	}
}

func bytesPerElement(c *goja.Object, def int64) int64 {
	if b := c.Get("BYTES_PER_ELEMENT"); b != nil {
		return max(b.ToInteger(), 1)
	}
	return def
}

func (st *runState) guardBuffer(callCap int64, tooBig func(string, int64)) {
	g := st.global("ArrayBuffer")
	if g == nil {
		return
	}
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

func (st *runState) guardTypedCtor(name string, callCap int64, tooBig func(string, int64)) {
	ctor := st.global(name)
	if ctor == nil {
		return
	}
	bpe := bytesPerElement(ctor, 1)
	st.guardCtor(name, func(args []goja.Value) {
		if len(args) == 0 {
			return
		}
		var n int64
		if a, ok := args[0].(*goja.Object); ok {
			if a.ClassName() == "ArrayBuffer" {
				return
			}
			n = st.lengthOf(a)
		} else {
			n = args[0].ToInteger()
		}
		if n > callCap/bpe {
			tooBig(name, n*bpe)
		}
	})
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

// stringifyGap is the indent width JSON.stringify will use.
func stringifyGap(v goja.Value) int {
	switch s := v.(type) {
	case goja.String:
		return min(s.Length(), 10)
	case nil:
		return 0
	}
	if v.ExportType() != nil && v.ExportType().Kind().String() != "string" {
		return int(min(max(v.ToInteger(), 0), 10))
	}
	return 0
}

// allowList reads an array replacer into its ordered, deduplicated key list.
func (st *runState) allowList(o *goja.Object) []string {
	var allow []string
	seen := map[string]bool{}
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			allow = append(allow, k)
		}
	}
	n := st.lengthOf(o)
	for i := int64(0); i < min(n, maxArrayLen); i++ {
		e := o.Get(strconv.FormatInt(i, 10))
		if s, ok := e.(goja.String); ok {
			add(s.String())
			continue
		}
		if e == nil || goja.IsUndefined(e) || goja.IsNull(e) {
			continue
		}
		if _, isObj := e.(*goja.Object); !isObj && e.ExportType().Kind().String() != "bool" {
			add(e.String())
		}
	}
	return allow
}

// stringifyCount is one JSON.stringify call's counting replacer state.
type stringifyCount struct {
	st       *runState
	callCap  int64
	gap      int
	userFn   goja.Callable
	allow    []string
	allowSet bool
	depths   map[*goja.Object]int
	counted  int64
}

func (sc *stringifyCount) replace(c goja.FunctionCall) goja.Value {
	st := sc.st
	key, v := c.Argument(0), c.Argument(1)
	if sc.userFn != nil {
		r, err := sc.userFn(c.This, key, v)
		if err != nil {
			panic(err)
		}
		v = r
	}
	holder, _ := c.This.(*goja.Object)
	vo, isObj := v.(*goja.Object)
	if sc.allowSet && isObj && vo.ClassName() == "Object" {
		filtered := st.vm.NewObject()
		for _, k := range sc.allow {
			if pv := vo.Get(k); pv != nil && !goja.IsUndefined(pv) {
				_ = filtered.DefineDataProperty(k, pv, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
			}
		}
		v, vo = filtered, filtered
	}
	sc.counted += int64(countValue(key, v))
	if sc.gap > 0 {
		d := sc.depths[holder]
		sc.counted += int64(sc.gap * (d + 1))
		if isObj {
			sc.depths[vo] = d + 1
		}
	}
	if sc.counted > sc.callCap {
		st.throwGuard(fmt.Sprintf("JSON.stringify output would be over %d bytes", sc.callCap))
	}
	return v
}

// guardStringify is G7: JSON.stringify under a counting replacer composed over the caller's.
func (st *runState) guardStringify(callCap int64) {
	st.replace(st.jsonObj, "stringify", func(call goja.FunctionCall, orig goja.Callable) goja.Value {
		sc := &stringifyCount{st: st, callCap: callCap, gap: stringifyGap(call.Argument(2)), depths: map[*goja.Object]int{}}
		sc.userFn, _ = goja.AssertFunction(call.Argument(1))
		if o, ok := call.Argument(1).(*goja.Object); ok && sc.userFn == nil && o.ClassName() == "Array" {
			sc.allowSet = true
			sc.allow = st.allowList(o)
		}
		res, err := orig(call.This, call.Argument(0), st.vm.ToValue(sc.replace), call.Argument(2))
		if err != nil {
			panic(err)
		}
		return res
	})
}
