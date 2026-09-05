package callbacks

import (
	"fmt"
	"reflect"
	"unsafe"
	_ "unsafe" // for go:linkname
)

// unsafeNew is the allocator reflect.New itself calls. Going straight
// to it skips reflect.New's pointer-type lookup, which showed up as 12%
// of the JIT'd program's time for an allocation the callee pays for
// anyway. The argument is the frame struct's rtype, so the block comes
// back with the right size and pointer map and the collector scans the
// slots correctly.
//
//go:linkname unsafeNew reflect.unsafe_New
func unsafeNew(rtype unsafe.Pointer) unsafe.Pointer

// The step JIT: the shape table applied to every call of a program
// rather than to a single statement.
//
// Three things make it possible.
//
// Slots live in one struct. reflect.StructOf builds a type whose fields
// are exactly the program's slot types, so reflect.New gives a single
// allocation with a correct pointer map and a stable address per field.
// Raw words are read and written at those offsets; because the fields
// carry the real types, every store through them emits the write
// barrier the garbage collector needs.
//
// Calls are made through layout classes, not types. A parameter or
// result is one of four shapes: pointer-shaped (one word), string (two),
// interface (two) or slice (three). A bound func whose classes match a
// shape in the table is reinterpreted as the shape type and called
// directly, exactly as jit.go does for a single statement.
//
// Interface arguments carry a precomputed itab. Both the concrete type
// and the interface type are known when the program compiles, and an
// itab depends on nothing else, so the pair is built once with reflect
// and only the data word varies per call.
//
// A program JITs whole or not at all. One step outside the table sends
// the program to the reflect evaluator in vm.go, which stays the
// general mechanism, and the equivalence test in stepjit_test.go pins
// the two against each other.

// sliceHdr mirrors the three words of a slice.
type sliceHdr struct {
	ptr      unsafe.Pointer
	len, cap int
}

// layout is the ABI shape of a value: what the callee reads, ignoring
// the type's name.
type layout int

const (
	lBad   layout = iota
	lPtr          // one word, pointer-shaped
	lStr          // two words
	lIface        // two words
	lSlice        // three words
	lErr          // a trailing error result, two words
)

func layoutOf(t reflect.Type) layout {
	switch t.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Map, reflect.Chan, reflect.Func:
		return lPtr
	case reflect.String:
		return lStr
	case reflect.Interface:
		return lIface
	case reflect.Slice:
		return lSlice
	}
	return lBad
}

func (l layout) String() string {
	switch l {
	case lPtr:
		return "P"
	case lStr:
		return "S"
	case lIface:
		return "I"
	case lSlice:
		return "L"
	case lErr:
		return "E"
	}
	return "?"
}

// itabFor returns the interface table for holding a value of concrete
// type ct in interface type it. The pair of types fixes the table, so
// this runs once per argument at compile time and only the data word
// varies per call. For an empty interface the word is the concrete
// type's rtype, which is the same position in the same two words.
func itabFor(ct, it reflect.Type) (unsafe.Pointer, bool) {
	if ct.Kind() == reflect.Interface || !ct.Implements(it) {
		return nil, false
	}
	cell := reflect.New(it).Elem()
	cell.Set(reflect.Zero(ct))
	pair := *(*ifacePair)(cell.Addr().UnsafePointer())
	if pair.tab == nil {
		return nil, false
	}
	return pair.tab, true
}

// jitArgKind is where one argument of a JIT'd step comes from.
type jitArgKind int

const (
	jaConst jitArgKind = iota
	jaSlot
	jaStack
	jaDest
)

// jitArg is one argument of a JIT'd step, resolved to raw words.
type jitArg struct {
	kind jitArgKind
	off  uintptr // jaSlot: byte offset of the slot within the frame

	cstr   string // jaConst
	cptr   unsafe.Pointer
	ciface ifacePair

	// Filling an interface parameter from a slot that holds a concrete
	// type. tab is that pair's interface table; direct says the slot
	// word is itself the data word rather than being pointed at.
	tab    unsafe.Pointer
	direct bool

	name string       // jaStack diagnostics
	typ  reflect.Type // the parameter type
	conv ifaceConv    // jaStack and jaDest into an interface parameter
}

func (a *jitArg) str(f unsafe.Pointer, stack map[string]any) (string, error) {
	switch a.kind {
	case jaConst:
		return a.cstr, nil
	case jaSlot:
		return *(*string)(unsafe.Add(f, a.off)), nil
	default:
		v, ok := stack[a.name]
		if !ok || v == nil {
			return "", nil
		}
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("exec: variable %q: cannot use %T as string", a.name, v)
		}
		return s, nil
	}
}

func (a *jitArg) ptr(f unsafe.Pointer) unsafe.Pointer {
	if a.kind == jaSlot {
		return *(*unsafe.Pointer)(unsafe.Add(f, a.off))
	}
	return a.cptr
}

func (a *jitArg) iface(f unsafe.Pointer, stack map[string]any, dest any) (ifacePair, error) {
	switch a.kind {
	case jaConst:
		return a.ciface, nil
	case jaSlot:
		p := unsafe.Add(f, a.off)
		switch {
		case a.tab == nil:
			// The slot is itself an interface: both words travel.
			return *(*ifacePair)(p), nil
		case a.direct:
			return ifacePair{tab: a.tab, data: *(*unsafe.Pointer)(p)}, nil
		default:
			// Indirect: the data word points at the slot. The frame is
			// heap-allocated and outlives the call, and the compiler
			// only takes this path for a slot written once, so nothing
			// can change the value behind a live interface.
			return ifacePair{tab: a.tab, data: p}, nil
		}
	case jaDest:
		if dest == nil {
			return ifacePair{}, fmt.Errorf("exec: dest is only set by Scan")
		}
		pair, ok := a.conv(dest)
		if !ok {
			return ifacePair{}, fmt.Errorf("exec: variable \"dest\": cannot use %T as %s", dest, a.typ)
		}
		return pair, nil
	default:
		v, ok := stack[a.name]
		if !ok || v == nil {
			return ifacePair{}, nil
		}
		pair, ok := a.conv(v)
		if !ok {
			return ifacePair{}, fmt.Errorf("exec: variable %q: cannot use %T as %s", a.name, v, a.typ)
		}
		return pair, nil
	}
}

// jitStep is one compiled call: it reads its arguments out of the frame,
// calls the binding directly, and writes the results back. A non-nil
// error result stops the program.
type jitStep func(f unsafe.Pointer, stack map[string]any, dest any) error

// jitProgram is a program whose every step JITs.
type jitProgram struct {
	frameType reflect.Type
	frameRT   unsafe.Pointer // frameType's rtype, for unsafeNew
	steps     []jitStep

	// retType and retOff describe the slot a trailing "return expr;"
	// leaves its value in. retType is nil when the program has no
	// value, which is the common case: the output goes through dest.
	retType reflect.Type
	retOff  uintptr
}

func (p *jitProgram) run(stack map[string]any, dest any) (any, error) {
	f := unsafeNew(p.frameRT)
	for _, step := range p.steps {
		if err := step(f, stack, dest); err != nil {
			return nil, err
		}
	}
	if p.retType == nil {
		return nil, nil
	}
	return reflect.NewAt(p.retType, unsafe.Add(f, p.retOff)).Elem().Interface(), nil
}

// asError turns the raw words of an error result into an error.
func asError(e ifacePair) error {
	if e.tab == nil {
		return nil
	}
	return *(*error)(unsafe.Pointer(&e))
}

func storeSlice(f unsafe.Pointer, off uintptr, h sliceHdr) { *(*sliceHdr)(unsafe.Add(f, off)) = h }
func storeIface(f unsafe.Pointer, off uintptr, i ifacePair) {
	*(*ifacePair)(unsafe.Add(f, off)) = i
}
func storeStr(f unsafe.Pointer, off uintptr, s string) { *(*string)(unsafe.Add(f, off)) = s }
func storePtr(f unsafe.Pointer, off uintptr, p unsafe.Pointer) {
	*(*unsafe.Pointer)(unsafe.Add(f, off)) = p
}

// Step shape types. The name lists the parameter classes, an underscore,
// then the result classes, with E for a trailing error. Adding a shape
// is one type and one case in stepFor.
type (
	stP_L    = func(unsafe.Pointer) sliceHdr
	stI_P    = func(ifacePair) unsafe.Pointer
	stPI_E   = func(unsafe.Pointer, ifacePair) ifacePair
	stSSI_PE = func(string, string, ifacePair) (unsafe.Pointer, ifacePair)
	stSS_PE  = func(string, string) (unsafe.Pointer, ifacePair)
	stS_PE   = func(string) (unsafe.Pointer, ifacePair)
	stS_P    = func(string) unsafe.Pointer
	stP_P    = func(unsafe.Pointer) unsafe.Pointer
	stP_S    = func(unsafe.Pointer) string
	stP_E    = func(unsafe.Pointer) ifacePair
	stPP_E   = func(unsafe.Pointer, unsafe.Pointer) ifacePair
	stII_E   = func(ifacePair, ifacePair) ifacePair
	stP_I    = func(unsafe.Pointer) ifacePair
	stI_PE   = func(ifacePair) (unsafe.Pointer, ifacePair)
)

// dropSlot marks a result the program does not bind.
const dropSlot = ^uintptr(0)

// stepFor builds the closure for one call, or reports that the shape is
// outside the table. outs holds a frame offset for each non-error
// result, in declaration order.
func stepFor(key string, fptr unsafe.Pointer, args []*jitArg, outs []uintptr) (jitStep, bool) {
	switch key {
	case "P_L":
		f, a0, o0 := castFn[stP_L](fptr), args[0], outs[0]
		return func(fr unsafe.Pointer, _ map[string]any, _ any) error {
			h := f(a0.ptr(fr))
			if o0 != dropSlot {
				storeSlice(fr, o0, h)
			}
			return nil
		}, true

	case "I_P":
		f, a0, o0 := castFn[stI_P](fptr), args[0], outs[0]
		return func(fr unsafe.Pointer, stack map[string]any, dest any) error {
			i0, err := a0.iface(fr, stack, dest)
			if err != nil {
				return err
			}
			if p := f(i0); o0 != dropSlot {
				storePtr(fr, o0, p)
			}
			return nil
		}, true

	case "PI_E":
		f, a0, a1 := castFn[stPI_E](fptr), args[0], args[1]
		return func(fr unsafe.Pointer, stack map[string]any, dest any) error {
			i1, err := a1.iface(fr, stack, dest)
			if err != nil {
				return err
			}
			return asError(f(a0.ptr(fr), i1))
		}, true

	case "SSI_PE":
		f, a0, a1, a2, o0 := castFn[stSSI_PE](fptr), args[0], args[1], args[2], outs[0]
		return func(fr unsafe.Pointer, stack map[string]any, dest any) error {
			s0, err := a0.str(fr, stack)
			if err != nil {
				return err
			}
			s1, err := a1.str(fr, stack)
			if err != nil {
				return err
			}
			i2, err := a2.iface(fr, stack, dest)
			if err != nil {
				return err
			}
			p, e := f(s0, s1, i2)
			if err := asError(e); err != nil {
				return err
			}
			if o0 != dropSlot {
				storePtr(fr, o0, p)
			}
			return nil
		}, true

	case "SS_PE":
		f, a0, a1, o0 := castFn[stSS_PE](fptr), args[0], args[1], outs[0]
		return func(fr unsafe.Pointer, stack map[string]any, _ any) error {
			s0, err := a0.str(fr, stack)
			if err != nil {
				return err
			}
			s1, err := a1.str(fr, stack)
			if err != nil {
				return err
			}
			p, e := f(s0, s1)
			if err := asError(e); err != nil {
				return err
			}
			if o0 != dropSlot {
				storePtr(fr, o0, p)
			}
			return nil
		}, true

	case "S_PE":
		f, a0, o0 := castFn[stS_PE](fptr), args[0], outs[0]
		return func(fr unsafe.Pointer, stack map[string]any, _ any) error {
			s0, err := a0.str(fr, stack)
			if err != nil {
				return err
			}
			p, e := f(s0)
			if err := asError(e); err != nil {
				return err
			}
			if o0 != dropSlot {
				storePtr(fr, o0, p)
			}
			return nil
		}, true

	case "I_PE":
		f, a0, o0 := castFn[stI_PE](fptr), args[0], outs[0]
		return func(fr unsafe.Pointer, stack map[string]any, dest any) error {
			i0, err := a0.iface(fr, stack, dest)
			if err != nil {
				return err
			}
			p, e := f(i0)
			if err := asError(e); err != nil {
				return err
			}
			if o0 != dropSlot {
				storePtr(fr, o0, p)
			}
			return nil
		}, true

	case "S_P":
		f, a0, o0 := castFn[stS_P](fptr), args[0], outs[0]
		return func(fr unsafe.Pointer, stack map[string]any, _ any) error {
			s0, err := a0.str(fr, stack)
			if err != nil {
				return err
			}
			if p := f(s0); o0 != dropSlot {
				storePtr(fr, o0, p)
			}
			return nil
		}, true

	case "P_P":
		f, a0, o0 := castFn[stP_P](fptr), args[0], outs[0]
		return func(fr unsafe.Pointer, _ map[string]any, _ any) error {
			if p := f(a0.ptr(fr)); o0 != dropSlot {
				storePtr(fr, o0, p)
			}
			return nil
		}, true

	case "P_S":
		f, a0, o0 := castFn[stP_S](fptr), args[0], outs[0]
		return func(fr unsafe.Pointer, _ map[string]any, _ any) error {
			if s := f(a0.ptr(fr)); o0 != dropSlot {
				storeStr(fr, o0, s)
			}
			return nil
		}, true

	case "P_I":
		f, a0, o0 := castFn[stP_I](fptr), args[0], outs[0]
		return func(fr unsafe.Pointer, _ map[string]any, _ any) error {
			if i := f(a0.ptr(fr)); o0 != dropSlot {
				storeIface(fr, o0, i)
			}
			return nil
		}, true

	case "P_E":
		f, a0 := castFn[stP_E](fptr), args[0]
		return func(fr unsafe.Pointer, _ map[string]any, _ any) error {
			return asError(f(a0.ptr(fr)))
		}, true

	case "PP_E":
		f, a0, a1 := castFn[stPP_E](fptr), args[0], args[1]
		return func(fr unsafe.Pointer, _ map[string]any, _ any) error {
			return asError(f(a0.ptr(fr), a1.ptr(fr)))
		}, true

	case "II_E":
		f, a0, a1 := castFn[stII_E](fptr), args[0], args[1]
		return func(fr unsafe.Pointer, stack map[string]any, dest any) error {
			i0, err := a0.iface(fr, stack, dest)
			if err != nil {
				return err
			}
			i1, err := a1.iface(fr, stack, dest)
			if err != nil {
				return err
			}
			return asError(f(i0, i1))
		}, true
	}
	return nil, false
}

// planned is one call in execution order with the slots its non-error
// results are written to. A nested call is planned before its caller,
// into a temporary slot.
type planned struct {
	call *vmCall
	outs []int
}

// jitCompileProgram builds the all-JIT form of a compiled program, or
// returns nil when any step is outside the shape table.
func jitCompileProgram(p *vmProgram) *jitProgram {
	if p.polymorphic {
		return nil
	}
	types := append([]reflect.Type(nil), p.slotTypes...)
	argSlot := map[*vmArg]int{}
	var plan []planned

	retSlot := -1
	for i := range p.stmts {
		s := &p.stmts[i]
		if s.call == nil {
			continue // a bare "return;" leaves the program without a value
		}
		if s.ret && i != len(p.stmts)-1 {
			return nil // an early return is not a straight line of steps
		}
		outs := make([]int, s.call.nres)
		for j := range outs {
			outs[j] = -1
		}
		for j, slot := range s.out {
			outs[j] = slot
		}
		if s.ret && s.call.nres > 0 && outs[0] < 0 {
			t := callResultType(s.call, 0)
			if t == nil || layoutOf(t) == lBad {
				return nil
			}
			types = append(types, t)
			outs[0] = len(types) - 1
			retSlot = outs[0]
		}
		if !planCall(s.call, outs, &plan, &types, argSlot) {
			return nil
		}
	}

	// Count writes per slot: an interface argument may only alias a
	// slot's storage when nothing can overwrite it later.
	writes := make([]int, len(types))
	for _, pc := range plan {
		for _, slot := range pc.outs {
			if slot >= 0 {
				writes[slot]++
			}
		}
	}

	fields := make([]reflect.StructField, len(types))
	for i, t := range types {
		if t == nil || layoutOf(t) == lBad {
			return nil
		}
		fields[i] = reflect.StructField{Name: fmt.Sprintf("F%d", i), Type: t}
	}
	frameType := reflect.StructOf(fields)
	offs := make([]uintptr, len(types))
	for i := range types {
		offs[i] = frameType.Field(i).Offset
	}

	jp := &jitProgram{frameType: frameType, frameRT: rtypePtr(frameType)}
	for _, pc := range plan {
		step, ok := buildStep(pc, types, offs, writes, argSlot)
		if !ok {
			return nil
		}
		jp.steps = append(jp.steps, step)
	}
	if retSlot >= 0 {
		jp.retType, jp.retOff = types[retSlot], offs[retSlot]
	}
	return jp
}

// planCall lays out a call and everything nested inside it, deepest
// first, so every step reads its arguments from slots already written.
func planCall(c *vmCall, outs []int, plan *[]planned, types *[]reflect.Type, argSlot map[*vmArg]int) bool {
	for _, a := range c.args {
		if a.kind != vaCall {
			continue
		}
		if a.sub.nres < 1 {
			return false
		}
		t := callResultType(a.sub, 0)
		if t == nil || layoutOf(t) == lBad {
			return false
		}
		*types = append(*types, t)
		slot := len(*types) - 1
		argSlot[a] = slot

		sub := make([]int, a.sub.nres)
		for i := range sub {
			sub[i] = -1
		}
		sub[0] = slot
		if !planCall(a.sub, sub, plan, types, argSlot) {
			return false
		}
	}
	*plan = append(*plan, planned{call: c, outs: outs})
	return true
}

// callResultType is the static type of a call's i'th non-error result.
func callResultType(c *vmCall, i int) reflect.Type {
	ft := c.fn.Type()
	n := 0
	for j := 0; j < ft.NumOut(); j++ {
		if j == c.errIdx {
			continue
		}
		if n == i {
			return ft.Out(j)
		}
		n++
	}
	return nil
}

// buildStep resolves one planned call into arguments, a shape key and a
// closure.
func buildStep(pc planned, types []reflect.Type, offs []uintptr, writes []int, argSlot map[*vmArg]int) (jitStep, bool) {
	c := pc.call
	ft := c.fn.Type()

	params := make([]layout, ft.NumIn())
	args := make([]*jitArg, ft.NumIn())
	for i := 0; i < ft.NumIn(); i++ {
		pt := ft.In(i)
		cl := layoutOf(pt)
		if cl == lBad {
			return nil, false
		}
		params[i] = cl
		a, ok := buildArg(c.args[i], pt, cl, types, offs, writes, argSlot)
		if !ok {
			return nil, false
		}
		args[i] = a
	}

	results := make([]layout, ft.NumOut())
	outs := make([]uintptr, 0, c.nres)
	n := 0
	for j := 0; j < ft.NumOut(); j++ {
		if j == c.errIdx {
			results[j] = lErr
			continue
		}
		cl := layoutOf(ft.Out(j))
		if cl == lBad {
			return nil, false
		}
		results[j] = cl
		off := dropSlot
		if n < len(pc.outs) && pc.outs[n] >= 0 {
			off = offs[pc.outs[n]]
		}
		outs = append(outs, off)
		n++
	}

	key := ""
	for _, l := range params {
		key += l.String()
	}
	key += "_"
	for _, l := range results {
		key += l.String()
	}
	return stepFor(key, funcPtrOf(c.fn), args, outs)
}

// funcPtrOf takes the funcval pointer out of a reflect.Value holding a
// func. Value.UnsafePointer returns the code pointer for a func value,
// which is what a funcval points at for a top-level func, but a method
// value or closure needs the funcval itself; boxing through an any and
// reading the eface data word gives that in every case.
func funcPtrOf(v reflect.Value) unsafe.Pointer {
	return funcPtr(v.Interface())
}

// buildArg resolves one argument of a planned call.
func buildArg(a *vmArg, pt reflect.Type, cl layout, types []reflect.Type, offs []uintptr, writes []int, argSlot map[*vmArg]int) (*jitArg, bool) {
	out := &jitArg{typ: pt}

	slot := -1
	switch a.kind {
	case vaSlot:
		slot = a.slot
	case vaCall:
		s, ok := argSlot[a]
		if !ok {
			return nil, false
		}
		slot = s
	}

	if slot >= 0 {
		out.kind, out.off = jaSlot, offs[slot]
		st := types[slot]
		if cl != lIface {
			// The slot must already hold exactly the parameter's shape.
			if layoutOf(st) != cl {
				return nil, false
			}
			return out, true
		}
		if st.Kind() == reflect.Interface {
			if st != pt {
				return nil, false
			}
			return out, true
		}
		tab, ok := itabFor(st, pt)
		if !ok {
			return nil, false
		}
		out.tab = tab
		out.direct = layoutOf(st) == lPtr
		if !out.direct && writes[slot] != 1 {
			// Aliasing the slot would let a later assignment change the
			// value behind an interface the callee may still hold.
			return nil, false
		}
		return out, true
	}

	switch a.kind {
	case vaConst:
		out.kind = jaConst
		switch cl {
		case lStr:
			out.cstr = a.val.String()
		case lPtr:
			if !a.val.IsZero() {
				return nil, false
			}
		case lIface, lSlice:
			if !a.val.IsZero() {
				return nil, false
			}
		}
		return out, true

	case vaStack, vaDest:
		if a.kind == vaDest {
			out.kind = jaDest
		} else {
			out.kind, out.name = jaStack, a.name
		}
		switch cl {
		case lStr:
			return out, true
		case lIface:
			conv, ok := ifaceConvs[pt]
			if !ok {
				return nil, false
			}
			out.conv = conv
			return out, true
		}
		return nil, false
	}
	return nil, false
}
