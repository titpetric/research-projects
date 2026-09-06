package callbacks

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"unsafe"
	_ "unsafe" // for go:linkname
)

// unsafeNew is the allocator reflect.New itself calls. Going straight
// to it skips reflect.New's pointer-type lookup. The argument is the
// frame struct's rtype, so the block comes back with the right size and
// pointer map and the collector scans the slots correctly.
//
//go:linkname unsafeNew reflect.unsafe_New
func unsafeNew(rtype unsafe.Pointer) unsafe.Pointer

// The step JIT compiles a program into a tree of typed closures.
//
// Calls are made through layout classes, not types. A parameter or
// result is one of four shapes: pointer-shaped (one word), string (two),
// interface (two) or slice (three). A bound func whose classes match a
// shape in the table is reinterpreted as the shape type and called
// directly, exactly as jit.go does for a single statement.
//
// A value flows from the call that produces it to the call that
// consumes it as the return value of a closure, so it lives in a Go
// local and needs no storage of its own. Only a name read by a later
// statement gets a frame slot, and a program with no such name
// allocates nothing beyond what its bindings allocate.
//
// Interface arguments carry a precomputed itab. Both the concrete type
// and the interface type are known when the program compiles, and an
// itab depends on nothing else, so the pair is built once with reflect
// and only the data word varies per call.
//
// A program JITs whole or not at all. One call outside the table sends
// it to the reflect evaluator in vm.go, which stays the general
// mechanism, and the equivalence test in stepjit_test.go pins the two
// against each other.

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
	lNone         // a call with no result other than a possible error

	// Scalars. Each width is its own class because the cast that makes
	// a direct call needs the exact Go type: an int32 parameter is four
	// bytes where an int64 is eight, and a float travels in a different
	// register file from an integer of the same width.
	lBool
	lI8
	lI16
	lI32
	lI64
	lU8
	lU16
	lU32
	lU64
	lF32
	lF64
)

// scalar reports whether a class travels through the node tree as a
// machine word rather than as a pointer, string, interface or slice.
func (l layout) scalar() bool { return l >= lBool }

// float reports whether a scalar class is carried as a float64.
func (l layout) float() bool { return l == lF32 || l == lF64 }

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
	case reflect.Bool:
		return lBool
	case reflect.Int8:
		return lI8
	case reflect.Int16:
		return lI16
	case reflect.Int32:
		return lI32
	case reflect.Int64:
		return lI64
	case reflect.Uint8:
		return lU8
	case reflect.Uint16:
		return lU16
	case reflect.Uint32:
		return lU32
	case reflect.Uint64:
		return lU64
	case reflect.Float32:
		return lF32
	case reflect.Float64:
		return lF64
	case reflect.Int:
		// int and uint are int64 and uint64 on a 64 bit platform and
		// int32 and uint32 on a 32 bit one, so the class follows the
		// size rather than the kind.
		if t.Size() == 8 {
			return lI64
		}
		return lI32
	case reflect.Uint, reflect.Uintptr:
		if t.Size() == 8 {
			return lU64
		}
		return lU32
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
	case lNone:
		return ""
	case lBool:
		return "b"
	case lI8:
		return "i8"
	case lI16:
		return "i16"
	case lI32:
		return "i32"
	case lI64:
		return "i64"
	case lU8:
		return "u8"
	case lU16:
		return "u16"
	case lU32:
		return "u32"
	case lU64:
		return "u64"
	case lF32:
		return "f32"
	case lF64:
		return "f64"
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

// The node types, one per layout class. Each evaluates its subtree and
// returns the value in that class. f is the frame, nil for a program
// that needs no slots.
type (
	nodeP func(f unsafe.Pointer, ctx context.Context, stack map[string]any, dest any) (unsafe.Pointer, error)
	nodeS func(f unsafe.Pointer, ctx context.Context, stack map[string]any, dest any) (string, error)
	nodeI func(f unsafe.Pointer, ctx context.Context, stack map[string]any, dest any) (ifacePair, error)
	nodeL func(f unsafe.Pointer, ctx context.Context, stack map[string]any, dest any) (sliceHdr, error)
	nodeE func(f unsafe.Pointer, ctx context.Context, stack map[string]any, dest any) error

	// nodeN carries every integer and bool class as the bits of its own
	// width, zero-extended. One closure type covers all of them because
	// the exact Go type is only needed where the call is made, and the
	// shape case there casts back. nodeF does the same for floats.
	nodeN func(f unsafe.Pointer, ctx context.Context, stack map[string]any, dest any) (uint64, error)
	nodeF func(f unsafe.Pointer, ctx context.Context, stack map[string]any, dest any) (float64, error)
)

// node is one compiled expression. Exactly one closure is set, named by
// class; lNone marks a call with no result, which only E carries.
type node struct {
	class layout
	P     nodeP
	S     nodeS
	I     nodeI
	L     nodeL
	E     nodeE
	N     nodeN
	F     nodeF
}

// loadN reads a scalar of class cl out of the frame as raw bits.
func loadN(cl layout, at unsafe.Pointer) uint64 {
	switch cl {
	case lBool:
		if *(*bool)(at) {
			return 1
		}
		return 0
	case lI8:
		return uint64(uint8(*(*int8)(at)))
	case lI16:
		return uint64(uint16(*(*int16)(at)))
	case lI32:
		return uint64(uint32(*(*int32)(at)))
	case lU8:
		return uint64(*(*uint8)(at))
	case lU16:
		return uint64(*(*uint16)(at))
	case lU32:
		return uint64(*(*uint32)(at))
	}
	return *(*uint64)(at) // lI64, lU64
}

func storeN(cl layout, at unsafe.Pointer, v uint64) {
	switch cl {
	case lBool:
		*(*bool)(at) = v != 0
	case lI8:
		*(*int8)(at) = int8(uint8(v))
	case lI16:
		*(*int16)(at) = int16(uint16(v))
	case lI32:
		*(*int32)(at) = int32(uint32(v))
	case lU8:
		*(*uint8)(at) = uint8(v)
	case lU16:
		*(*uint16)(at) = uint16(v)
	case lU32:
		*(*uint32)(at) = uint32(v)
	default:
		*(*uint64)(at) = v
	}
}

func loadF(cl layout, at unsafe.Pointer) float64 {
	if cl == lF32 {
		return float64(*(*float32)(at))
	}
	return *(*float64)(at)
}

func storeF(cl layout, at unsafe.Pointer, v float64) {
	if cl == lF32 {
		*(*float32)(at) = float32(v)
		return
	}
	*(*float64)(at) = v
}

// scalarBits turns a compile-time constant into the representation
// nodeN and nodeF carry.
func scalarBits(cl layout, v reflect.Value) (uint64, float64) {
	switch cl {
	case lBool:
		if v.Bool() {
			return 1, 0
		}
		return 0, 0
	case lI8, lI16, lI32, lI64:
		return uint64(v.Int()), 0
	case lU8, lU16, lU32, lU64:
		return v.Uint(), 0
	}
	return 0, v.Float()
}

// jitProgram is a program whose every call JITs.
type jitProgram struct {
	// bridged lists calls that go through reflect, empty when the whole
	// program is direct calls.
	bridged   []string
	frameType reflect.Type
	frameRT   unsafe.Pointer // nil when the program needs no slots
	stmts     []nodeE

	// retType and retOff describe the slot a trailing "return expr;"
	// leaves its value in. retType is nil when the program has no
	// value, which is the common case: the output goes through dest.
	retType reflect.Type
	retOff  uintptr
}

func (p *jitProgram) run(ctx context.Context, stack map[string]any, dest any) (any, error) {
	var f unsafe.Pointer
	if p.frameRT != nil {
		f = unsafeNew(p.frameRT)
	}
	for _, stmt := range p.stmts {
		if err := stmt(f, ctx, stack, dest); err != nil {
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

// Shape types. The name lists the parameter classes, an underscore,
// then the result classes, with E for a trailing error. Adding a shape
// is one type and one case in callNode.
type (
	stP_L     = func(unsafe.Pointer) sliceHdr
	stI_P     = func(ifacePair) unsafe.Pointer
	stPI_E    = func(unsafe.Pointer, ifacePair) ifacePair
	stSSI_PE  = func(string, string, ifacePair) (unsafe.Pointer, ifacePair)
	stISSI_PE = func(ifacePair, string, string, ifacePair) (unsafe.Pointer, ifacePair)
	stSS_PE   = func(string, string) (unsafe.Pointer, ifacePair)
	stS_PE    = func(string) (unsafe.Pointer, ifacePair)
	stI_PE    = func(ifacePair) (unsafe.Pointer, ifacePair)
	stS_P     = func(string) unsafe.Pointer
	stP_P     = func(unsafe.Pointer) unsafe.Pointer
	stP_S     = func(unsafe.Pointer) string
	stP_I     = func(unsafe.Pointer) ifacePair
	stP_E     = func(unsafe.Pointer) ifacePair
	stPP_E    = func(unsafe.Pointer, unsafe.Pointer) ifacePair
	stII_E    = func(ifacePair, ifacePair) ifacePair
	stSS_E    = func(string, string) ifacePair
	stPP_PE   = func(unsafe.Pointer, unsafe.Pointer) (unsafe.Pointer, ifacePair)
)

// nPE and the helpers beside it are the scalar call families. They are
// generic over the parameter's Go type so one body covers every width:
// the cast needs the exact type, but nothing else does.
func nPE[T any](fptr unsafe.Pointer, a0 nodeN, conv func(uint64) T) node {
	f := castFn[func(T) (unsafe.Pointer, ifacePair)](fptr)
	return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
		n, err := a0(fr, ctx, st, d)
		if err != nil {
			return nil, err
		}
		p, e := f(conv(n))
		if err := asError(e); err != nil {
			return nil, err
		}
		return p, nil
	}}
}

func nE[T any](fptr unsafe.Pointer, a0 nodeN, conv func(uint64) T) node {
	f := castFn[func(T) ifacePair](fptr)
	return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
		n, err := a0(fr, ctx, st, d)
		if err != nil {
			return err
		}
		return asError(f(conv(n)))
	}}
}

func fPE[T any](fptr unsafe.Pointer, a0 nodeF, conv func(float64) T) node {
	f := castFn[func(T) (unsafe.Pointer, ifacePair)](fptr)
	return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
		v, err := a0(fr, ctx, st, d)
		if err != nil {
			return nil, err
		}
		p, e := f(conv(v))
		if err := asError(e); err != nil {
			return nil, err
		}
		return p, nil
	}}
}

func fE[T any](fptr unsafe.Pointer, a0 nodeF, conv func(float64) T) node {
	f := castFn[func(T) ifacePair](fptr)
	return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
		v, err := a0(fr, ctx, st, d)
		if err != nil {
			return err
		}
		return asError(f(conv(v)))
	}}
}

// pN is a pointer parameter and a scalar result, the shape a getter
// method has.
func pN[T any](fptr unsafe.Pointer, a0 nodeP, cl layout, up func(T) uint64) node {
	f := castFn[func(unsafe.Pointer) T](fptr)
	return node{class: cl, N: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (uint64, error) {
		p, err := a0(fr, ctx, st, d)
		if err != nil {
			return 0, err
		}
		return up(f(p)), nil
	}}
}

func pF[T any](fptr unsafe.Pointer, a0 nodeP, cl layout, up func(T) float64) node {
	f := castFn[func(unsafe.Pointer) T](fptr)
	return node{class: cl, F: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (float64, error) {
		p, err := a0(fr, ctx, st, d)
		if err != nil {
			return 0, err
		}
		return up(f(p)), nil
	}}
}

// scalarCall covers a call whose only parameter is a scalar. res is the
// result half of the shape key.
func scalarCall(fptr unsafe.Pointer, in layout, res string, a node) (node, bool) {
	switch res {
	case "PE":
		switch in {
		case lBool:
			return nPE(fptr, a.N, func(n uint64) bool { return n != 0 }), true
		case lI8:
			return nPE(fptr, a.N, func(n uint64) int8 { return int8(uint8(n)) }), true
		case lI16:
			return nPE(fptr, a.N, func(n uint64) int16 { return int16(uint16(n)) }), true
		case lI32:
			return nPE(fptr, a.N, func(n uint64) int32 { return int32(uint32(n)) }), true
		case lI64:
			return nPE(fptr, a.N, func(n uint64) int64 { return int64(n) }), true
		case lU8:
			return nPE(fptr, a.N, func(n uint64) uint8 { return uint8(n) }), true
		case lU16:
			return nPE(fptr, a.N, func(n uint64) uint16 { return uint16(n) }), true
		case lU32:
			return nPE(fptr, a.N, func(n uint64) uint32 { return uint32(n) }), true
		case lU64:
			return nPE(fptr, a.N, func(n uint64) uint64 { return n }), true
		case lF32:
			return fPE(fptr, a.F, func(v float64) float32 { return float32(v) }), true
		case lF64:
			return fPE(fptr, a.F, func(v float64) float64 { return v }), true
		}
	case "E":
		switch in {
		case lBool:
			return nE(fptr, a.N, func(n uint64) bool { return n != 0 }), true
		case lI8:
			return nE(fptr, a.N, func(n uint64) int8 { return int8(uint8(n)) }), true
		case lI16:
			return nE(fptr, a.N, func(n uint64) int16 { return int16(uint16(n)) }), true
		case lI32:
			return nE(fptr, a.N, func(n uint64) int32 { return int32(uint32(n)) }), true
		case lI64:
			return nE(fptr, a.N, func(n uint64) int64 { return int64(n) }), true
		case lU8:
			return nE(fptr, a.N, func(n uint64) uint8 { return uint8(n) }), true
		case lU16:
			return nE(fptr, a.N, func(n uint64) uint16 { return uint16(n) }), true
		case lU32:
			return nE(fptr, a.N, func(n uint64) uint32 { return uint32(n) }), true
		case lU64:
			return nE(fptr, a.N, func(n uint64) uint64 { return n }), true
		case lF32:
			return fE(fptr, a.F, func(v float64) float32 { return float32(v) }), true
		case lF64:
			return fE(fptr, a.F, func(v float64) float64 { return v }), true
		}
	}
	return node{}, false
}

// ptrScalarCall covers a pointer parameter and a scalar result.
func ptrScalarCall(fptr unsafe.Pointer, out layout, a node) (node, bool) {
	switch out {
	case lBool:
		return pN(fptr, a.P, out, func(v bool) uint64 {
			if v {
				return 1
			}
			return 0
		}), true
	case lI8:
		return pN(fptr, a.P, out, func(v int8) uint64 { return uint64(uint8(v)) }), true
	case lI16:
		return pN(fptr, a.P, out, func(v int16) uint64 { return uint64(uint16(v)) }), true
	case lI32:
		return pN(fptr, a.P, out, func(v int32) uint64 { return uint64(uint32(v)) }), true
	case lI64:
		return pN(fptr, a.P, out, func(v int64) uint64 { return uint64(v) }), true
	case lU8:
		return pN(fptr, a.P, out, func(v uint8) uint64 { return uint64(v) }), true
	case lU16:
		return pN(fptr, a.P, out, func(v uint16) uint64 { return uint64(v) }), true
	case lU32:
		return pN(fptr, a.P, out, func(v uint32) uint64 { return uint64(v) }), true
	case lU64:
		return pN(fptr, a.P, out, func(v uint64) uint64 { return v }), true
	case lF32:
		return pF(fptr, a.P, out, func(v float32) float64 { return float64(v) }), true
	case lF64:
		return pF(fptr, a.P, out, func(v float64) float64 { return v }), true
	}
	return node{}, false
}

// getterFor adapts a scalar node into a typed getter, converting from
// the width-erased carriers back to the exact Go type T.
func getterFor[T any](a node, fromN func(uint64) T, fromF func(float64) T) func(unsafe.Pointer, context.Context, map[string]any, any) (T, error) {
	if a.class.float() {
		f := a.F
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (T, error) {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				var zero T
				return zero, err
			}
			return fromF(v), nil
		}
	}
	f := a.N
	return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (T, error) {
		v, err := f(fr, ctx, st, d)
		if err != nil {
			var zero T
			return zero, err
		}
		return fromN(v), nil
	}
}

// mixed2 builds the node for a two-parameter call whose second
// parameter is a scalar of type T and whose first is a string or a
// pointer. res is the result half of the shape key. One generic body
// covers every width; the per-width dispatch below only bakes the
// conversion.
func mixed2[T any](fptr unsafe.Pointer, first layout, res string, a0 node, get func(unsafe.Pointer, context.Context, map[string]any, any) (T, error)) (node, bool) {
	switch {
	case first == lStr && res == "PE":
		f, s0 := castFn[func(string, T) (unsafe.Pointer, ifacePair)](fptr), a0.S
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			x0, err := s0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			x1, err := get(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p, e := f(x0, x1)
			if err := asError(e); err != nil {
				return nil, err
			}
			return p, nil
		}}, true
	case first == lStr && res == "E":
		f, s0 := castFn[func(string, T) ifacePair](fptr), a0.S
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			x0, err := s0(fr, ctx, st, d)
			if err != nil {
				return err
			}
			x1, err := get(fr, ctx, st, d)
			if err != nil {
				return err
			}
			return asError(f(x0, x1))
		}}, true
	case first == lPtr && res == "PE":
		f, p0 := castFn[func(unsafe.Pointer, T) (unsafe.Pointer, ifacePair)](fptr), a0.P
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			x0, err := p0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			x1, err := get(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p, e := f(x0, x1)
			if err := asError(e); err != nil {
				return nil, err
			}
			return p, nil
		}}, true
	case first == lPtr && res == "E":
		f, p0 := castFn[func(unsafe.Pointer, T) ifacePair](fptr), a0.P
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			x0, err := p0(fr, ctx, st, d)
			if err != nil {
				return err
			}
			x1, err := get(fr, ctx, st, d)
			if err != nil {
				return err
			}
			return asError(f(x0, x1))
		}}, true
	}
	return node{}, false
}

// mixedScalarCall dispatches a (fixed, scalar) call to mixed2 with the
// width baked into the getter.
func mixedScalarCall(fptr unsafe.Pointer, first layout, res string, a0, a1 node) (node, bool) {
	switch a1.class {
	case lBool:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) bool { return n != 0 }, nil))
	case lI8:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) int8 { return int8(uint8(n)) }, nil))
	case lI16:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) int16 { return int16(uint16(n)) }, nil))
	case lI32:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) int32 { return int32(uint32(n)) }, nil))
	case lI64:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) int64 { return int64(n) }, nil))
	case lU8:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) uint8 { return uint8(n) }, nil))
	case lU16:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) uint16 { return uint16(n) }, nil))
	case lU32:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) uint32 { return uint32(n) }, nil))
	case lU64:
		return mixed2(fptr, first, res, a0, getterFor(a1, func(n uint64) uint64 { return n }, nil))
	case lF32:
		return mixed2(fptr, first, res, a0, getterFor(a1, nil, func(v float64) float32 { return float32(v) }))
	case lF64:
		return mixed2(fptr, first, res, a0, getterFor(a1, nil, func(v float64) float64 { return v }))
	}
	return node{}, false
}

// callNode builds the node for one call from the nodes of its
// arguments, or reports that the shape is outside the table.
func callNode(key string, fptr unsafe.Pointer, a []node) (node, bool) {
	if i := strings.IndexByte(key, '_'); i > 0 && len(a) == 2 {
		if (a[0].class == lStr || a[0].class == lPtr) && a[1].class.scalar() {
			if n, ok := mixedScalarCall(fptr, a[0].class, key[i+1:], a[0], a[1]); ok {
				return n, true
			}
		}
	}
	if i := strings.IndexByte(key, '_'); i > 0 && len(a) == 1 {
		if a[0].class.scalar() {
			if n, ok := scalarCall(fptr, a[0].class, key[i+1:], a[0]); ok {
				return n, true
			}
		}
		if a[0].class == lPtr {
			if out, ok := classOf(key[i+1:]); ok && out.scalar() {
				return ptrScalarCall(fptr, out, a[0])
			}
		}
	}
	switch key {
	case "SL_S":
		f, a0, a1 := castFn[func(string, sliceHdr) string](fptr), a[0].S, a[1].L
		return node{class: lStr, S: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (string, error) {
			s0, err := a0(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			h, err := a1(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			return f(s0, h), nil
		}}, true

	case "PS_S":
		f, a0, a1 := castFn[func(unsafe.Pointer, string) string](fptr), a[0].P, a[1].S
		return node{class: lStr, S: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (string, error) {
			p0, err := a0(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			s1, err := a1(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			return f(p0, s1), nil
		}}, true

	case "S_L":
		f, a0 := castFn[func(string) sliceHdr](fptr), a[0].S
		return node{class: lSlice, L: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (sliceHdr, error) {
			s0, err := a0(fr, ctx, st, d)
			if err != nil {
				return sliceHdr{}, err
			}
			return f(s0), nil
		}}, true

	case "I_S":
		f, a0 := castFn[func(ifacePair) string](fptr), a[0].I
		return node{class: lStr, S: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (string, error) {
			i0, err := a0(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			return f(i0), nil
		}}, true

	case "IIIS_":
		f, a0, a1, a2, a3 := castFn[func(ifacePair, ifacePair, ifacePair, string)](fptr), a[0].I, a[1].I, a[2].I, a[3].S
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			i0, err := a0(fr, ctx, st, d)
			if err != nil {
				return err
			}
			i1, err := a1(fr, ctx, st, d)
			if err != nil {
				return err
			}
			i2, err := a2(fr, ctx, st, d)
			if err != nil {
				return err
			}
			s3, err := a3(fr, ctx, st, d)
			if err != nil {
				return err
			}
			f(i0, i1, i2, s3)
			return nil
		}}, true

	case "L_S":
		f, a0 := castFn[func(sliceHdr) string](fptr), a[0].L
		return node{class: lStr, S: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (string, error) {
			h, err := a0(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			return f(h), nil
		}}, true

	case "P_L":
		f, a0 := castFn[stP_L](fptr), a[0].P
		return node{class: lSlice, L: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (sliceHdr, error) {
			p, err := a0(fr, ctx, st, d)
			if err != nil {
				return sliceHdr{}, err
			}
			return f(p), nil
		}}, true

	case "I_P":
		f, a0 := castFn[stI_P](fptr), a[0].I
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			i0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			return f(i0), nil
		}}, true

	case "PI_E":
		f, a0, a1 := castFn[stPI_E](fptr), a[0].P, a[1].I
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			p0, err := a0(fr, ctx, st, d)
			if err != nil {
				return err
			}
			i1, err := a1(fr, ctx, st, d)
			if err != nil {
				return err
			}
			return asError(f(p0, i1))
		}}, true

	case "ISSI_PE":
		f, a0, a1, a2, a3 := castFn[stISSI_PE](fptr), a[0].I, a[1].S, a[2].S, a[3].I
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			i0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			s1, err := a1(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			s2, err := a2(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			i3, err := a3(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p, e := f(i0, s1, s2, i3)
			if err := asError(e); err != nil {
				return nil, err
			}
			return p, nil
		}}, true

	case "SSI_PE":
		f, a0, a1, a2 := castFn[stSSI_PE](fptr), a[0].S, a[1].S, a[2].I
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			s0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			s1, err := a1(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			i2, err := a2(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p, e := f(s0, s1, i2)
			if err := asError(e); err != nil {
				return nil, err
			}
			return p, nil
		}}, true

	case "SS_PE":
		f, a0, a1 := castFn[stSS_PE](fptr), a[0].S, a[1].S
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			s0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			s1, err := a1(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p, e := f(s0, s1)
			if err := asError(e); err != nil {
				return nil, err
			}
			return p, nil
		}}, true

	case "S_PE":
		f, a0 := castFn[stS_PE](fptr), a[0].S
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			s0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p, e := f(s0)
			if err := asError(e); err != nil {
				return nil, err
			}
			return p, nil
		}}, true

	case "I_PE":
		f, a0 := castFn[stI_PE](fptr), a[0].I
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			i0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p, e := f(i0)
			if err := asError(e); err != nil {
				return nil, err
			}
			return p, nil
		}}, true

	case "S_P":
		f, a0 := castFn[stS_P](fptr), a[0].S
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			s0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			return f(s0), nil
		}}, true

	case "P_P":
		f, a0 := castFn[stP_P](fptr), a[0].P
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			p0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			return f(p0), nil
		}}, true

	case "P_S":
		f, a0 := castFn[stP_S](fptr), a[0].P
		return node{class: lStr, S: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (string, error) {
			p0, err := a0(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			return f(p0), nil
		}}, true

	case "P_I":
		f, a0 := castFn[stP_I](fptr), a[0].P
		return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (ifacePair, error) {
			p0, err := a0(fr, ctx, st, d)
			if err != nil {
				return ifacePair{}, err
			}
			return f(p0), nil
		}}, true

	case "P_E":
		f, a0 := castFn[stP_E](fptr), a[0].P
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			p0, err := a0(fr, ctx, st, d)
			if err != nil {
				return err
			}
			return asError(f(p0))
		}}, true

	case "PP_PE":
		f, a0, a1 := castFn[stPP_PE](fptr), a[0].P, a[1].P
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			p0, err := a0(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p1, err := a1(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			p, e := f(p0, p1)
			if err := asError(e); err != nil {
				return nil, err
			}
			return p, nil
		}}, true

	case "PP_E":
		f, a0, a1 := castFn[stPP_E](fptr), a[0].P, a[1].P
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			p0, err := a0(fr, ctx, st, d)
			if err != nil {
				return err
			}
			p1, err := a1(fr, ctx, st, d)
			if err != nil {
				return err
			}
			return asError(f(p0, p1))
		}}, true

	case "II_E":
		f, a0, a1 := castFn[stII_E](fptr), a[0].I, a[1].I
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			i0, err := a0(fr, ctx, st, d)
			if err != nil {
				return err
			}
			i1, err := a1(fr, ctx, st, d)
			if err != nil {
				return err
			}
			return asError(f(i0, i1))
		}}, true

	case "SS_E":
		f, a0, a1 := castFn[stSS_E](fptr), a[0].S, a[1].S
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			s0, err := a0(fr, ctx, st, d)
			if err != nil {
				return err
			}
			s1, err := a1(fr, ctx, st, d)
			if err != nil {
				return err
			}
			return asError(f(s0, s1))
		}}, true
	}
	return node{}, false
}

// classOf is the inverse of layout.String, for reading a shape key.
func classOf(s string) (layout, bool) {
	for l := lBool; l <= lF64; l++ {
		if l.String() == s {
			return l, true
		}
	}
	return lBad, false
}

// jitCompiler holds the state of one program's compilation.
type jitCompiler struct {
	slotOf map[int]int // vmProgram slot -> frame field index
	types  []reflect.Type
	offs   []uintptr
	// bridged names the calls that compile to a reflect bridge rather
	// than a direct call. The program still runs on this tier; Supports
	// reports them so a benchmark knows which statements pay reflect.
	bridged []string
	// writes counts the assignments to each slot. A slot written more
	// than once cannot back an interface argument, because the argument
	// aliases the slot and a later assignment would change the value
	// behind an interface the callee may still hold.
	writes map[int]int
	// splices maps an argument that reads a name to the call that
	// produced it, where planInline decided the value can travel as a
	// return value instead of through a slot. It is a side table rather
	// than an edit of the call tree, because the same tree is what the
	// reflect evaluator runs when the JIT declines: rewriting it would
	// leave the producer both spliced in and still a statement, and the
	// fallback would run it twice.
	splices map[*vmArg]*vmCall
}

// jitCompileProgram builds the all-JIT form of a compiled program, or
// returns the reason it cannot. The error is what Runtime.Supports
// reports, so it names the call and the shape that stopped it.
func jitCompileProgram(p *vmProgram) (*jitProgram, error) {
	if p.polymorphic {
		return nil, fmt.Errorf("a name is reassigned at a different type")
	}

	plan, err := planInline(p)
	if err != nil {
		return nil, err
	}

	c := &jitCompiler{slotOf: map[int]int{}, writes: plan.writes, splices: plan.splices}
	for slot := 0; slot < p.nslots; slot++ {
		if !plan.live[slot] {
			continue
		}
		t := p.slotTypes[slot]
		if t == nil || layoutOf(t) == lBad {
			return nil, fmt.Errorf("a name of type %s has no layout class", t)
		}
		c.slotOf[slot] = len(c.types)
		c.types = append(c.types, t)
	}

	jp := &jitProgram{}
	if len(c.types) > 0 {
		fields := make([]reflect.StructField, len(c.types))
		for i, t := range c.types {
			fields[i] = reflect.StructField{Name: fmt.Sprintf("F%d", i), Type: t}
		}
		jp.frameType = reflect.StructOf(fields)
		jp.frameRT = rtypePtr(jp.frameType)
		c.offs = make([]uintptr, len(c.types))
		for i := range c.types {
			c.offs[i] = jp.frameType.Field(i).Offset
		}
	}

	for _, s := range plan.stmts {
		stmt, err := c.stmtNode(s, jp)
		if err != nil {
			return nil, err
		}
		jp.stmts = append(jp.stmts, stmt)
	}
	if plan.retSlot >= 0 {
		field, ok := c.slotOf[plan.retSlot]
		if !ok {
			return nil, fmt.Errorf("a returned name has no slot")
		}
		jp.retType, jp.retOff = c.types[field], c.offs[field]
	}
	jp.bridged = c.bridged
	return jp, nil
}

// plannedStmt is one statement after inlining, with the slot its result
// is stored to.
type plannedStmt struct {
	call *vmCall
	out  int // frame slot, -1 to discard
	ret  bool

	// lit is a literal assignment, which has no call to compile.
	lit reflect.Value

	// fieldSet is a field assignment, compiled to a typed store.
	fieldSet *vmFieldSet
}

// jitPlan is everything planInline works out for the compiler.
type jitPlan struct {
	stmts   []plannedStmt
	live    map[int]bool
	writes  map[int]int
	splices map[*vmArg]*vmCall
	// retSlot is the slot a "return name;" reads, -1 when the program
	// returns through a trailing call or not at all.
	retSlot int
}

// planInline drops a statement whose single result is read exactly once
// by a later statement, splicing the call into the reader's argument
// tree. The value then travels as a closure's return value and needs no
// slot at all.
//
// The splice only happens when every argument the reader evaluates
// before it is free of side effects, which keeps the order of calls the
// source wrote. A receiver is argument zero and always qualifies, so a
// method chain written across statements collapses exactly as the same
// chain written on one line does.
func planInline(p *vmProgram) (*jitPlan, error) {
	retSlot := -1
	stmts := make([]plannedStmt, 0, len(p.stmts))
	for i := range p.stmts {
		s := &p.stmts[i]
		if s.retArg != nil {
			// Only a name that already has a slot returns on this
			// tier. A field read, a literal or a stack name in return
			// position is rare enough that the reflect evaluator keeps
			// it; what must not happen is the statement being skipped,
			// which would silently return nil where reflect returns
			// the value.
			if s.retArg.kind != vaSlot {
				return nil, fmt.Errorf("a returned expression is not in the table")
			}
			retSlot = s.retArg.slot
			continue
		}
		if s.fieldSet != nil {
			stmts = append(stmts, plannedStmt{fieldSet: s.fieldSet, out: -1})
			continue
		}
		if s.lit.IsValid() {
			out := -1
			if len(s.out) > 0 {
				out = s.out[0]
			}
			stmts = append(stmts, plannedStmt{lit: s.lit, out: out})
			continue
		}
		if s.call == nil {
			continue // a bare "return;" leaves the program without a value
		}
		if s.ret && i != len(p.stmts)-1 {
			return nil, fmt.Errorf("a return before the last statement is not a straight line")
		}
		out := -1
		if len(s.out) > 0 {
			out = s.out[0]
		}
		stmts = append(stmts, plannedStmt{call: s.call, out: out, ret: s.ret})
	}

	reads := map[int]int{}
	if retSlot >= 0 {
		reads[retSlot]++
	}
	for _, s := range stmts {
		if s.call != nil {
			countReads(s.call, reads)
		}
		if s.fieldSet != nil {
			reads[s.fieldSet.base]++
			if sub := s.fieldSet.val.sub; sub != nil {
				countReads(sub, reads)
			}
		}
	}

	splices := map[*vmArg]*vmCall{}
	for i := 0; i+1 < len(stmts); i++ {
		s := stmts[i]
		if s.call == nil || s.ret || s.out < 0 || s.call.nres != 1 || reads[s.out] != 1 {
			continue
		}
		if stmts[i+1].call == nil {
			continue // a literal assignment has no argument to splice into
		}
		// Only into the statement immediately after. Every statement
		// runs a call, so moving a producer past one would reorder two
		// calls the source wrote in the other order.
		arg := findSplice(stmts[i+1].call, s.out, splices)
		if arg == nil {
			continue
		}
		splices[arg] = s.call
		stmts = append(stmts[:i], stmts[i+1:]...)
		i--
	}

	live := map[int]bool{}
	writes := map[int]int{}
	if retSlot >= 0 {
		live[retSlot] = true
	}
	// A var declaration puts a name in scope whether or not anything
	// assigns it, so its slot is live from the start. The frame comes
	// back zeroed, which is exactly the zero value the declaration
	// promises, so there is nothing to run for it.
	for _, in := range p.inits {
		live[in.slot] = true
		writes[in.slot]++
	}
	for _, s := range stmts {
		if s.fieldSet != nil {
			live[s.fieldSet.base] = true
			// Writing a field of a struct held by value mutates the
			// slot an aliased interface may point at, so it counts as
			// a write and the aliasing falls back to a copy.
			if t := p.slotTypes[s.fieldSet.base]; t != nil && t.Kind() != reflect.Pointer {
				writes[s.fieldSet.base]++
			}
			continue
		}
		if s.out >= 0 {
			live[s.out] = true
			writes[s.out]++
		}
		if s.ret && s.call.nres > 0 && s.out < 0 {
			return nil, fmt.Errorf("a returned value needs a slot")
		}
	}
	return &jitPlan{stmts: stmts, live: live, writes: writes, splices: splices, retSlot: retSlot}, nil
}

// countReads tallies how many times each name is read, which decides
// whether its producer can be spliced into the reader and the slot
// dropped. It descends through a field, because req.Header is a read of
// req: missing those undercounts, and a name read once directly and
// once through a field would have its producer spliced away while the
// field read still pointed at the dropped slot.
func countReads(c *vmCall, reads map[int]int) {
	for _, a := range c.args {
		for a.kind == vaField {
			a = a.src
		}
		switch a.kind {
		case vaSlot:
			reads[a.slot]++
		case vaCall:
			countReads(a.sub, reads)
		}
	}
}

// findSplice returns the argument of c that reads slot and can take the
// producer in place, or nil. It refuses when anything c evaluates
// earlier has a side effect, which keeps the order of calls the source
// wrote; a receiver is argument zero and always qualifies.
func findSplice(c *vmCall, slot int, splices map[*vmArg]*vmCall) *vmArg {
	for i, a := range c.args {
		if a.kind == vaSlot && a.slot == slot && splices[a] == nil {
			for _, before := range c.args[:i] {
				if before.kind == vaCall || splices[before] != nil {
					return nil
				}
			}
			return a
		}
		if sub := subCall(a, splices); sub != nil {
			if found := findSplice(sub, slot, splices); found != nil {
				return found
			}
		}
	}
	return nil
}

// subCall is the call an argument evaluates, whether it was written
// nested or spliced in by planInline.
func subCall(a *vmArg, splices map[*vmArg]*vmCall) *vmCall {
	for a.kind == vaField {
		a = a.src
	}
	if a.kind == vaCall {
		return a.sub
	}
	return splices[a]
}

// stmtNode compiles one statement into the closure the program runs.
func (c *jitCompiler) stmtNode(s plannedStmt, jp *jitProgram) (nodeE, error) {
	if s.fieldSet != nil {
		return c.fieldSetNode(s.fieldSet)
	}
	var n node
	if s.lit.IsValid() {
		field, ok := c.slotOf[s.out]
		if !ok {
			return nil, fmt.Errorf("a literal is assigned to a name with no slot")
		}
		cl := layoutOf(c.types[field])
		if cl == lBad {
			return nil, fmt.Errorf("a literal of type %s has no layout class", c.types[field])
		}
		lit, err := constNode(cl, s.lit)
		if err != nil {
			return nil, err
		}
		n = lit
	} else {
		var err error
		n, err = c.exprNode(s.call)
		if err != nil {
			return nil, err
		}
	}
	if s.out < 0 {
		// The result is dropped, but the call still runs and its error
		// still ends the program.
		return c.dropped(n)
	}

	field, ok := c.slotOf[s.out]
	if !ok {
		return c.dropped(n)
	}
	off := c.offs[field]
	if s.ret {
		jp.retType, jp.retOff = c.types[field], off
	}

	if n.class.scalar() {
		cl := n.class
		if cl.float() {
			f := n.F
			return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
				v, err := f(fr, ctx, st, d)
				if err != nil {
					return err
				}
				storeF(cl, unsafe.Add(fr, off), v)
				return nil
			}, nil
		}
		f := n.N
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			storeN(cl, unsafe.Add(fr, off), v)
			return nil
		}, nil
	}
	switch n.class {
	case lPtr:
		f := n.P
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			*(*unsafe.Pointer)(unsafe.Add(fr, off)) = v
			return nil
		}, nil
	case lSlice:
		f := n.L
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			*(*sliceHdr)(unsafe.Add(fr, off)) = v
			return nil
		}, nil
	case lStr:
		f := n.S
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			*(*string)(unsafe.Add(fr, off)) = v
			return nil
		}, nil
	case lIface:
		f := n.I
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			*(*ifacePair)(unsafe.Add(fr, off)) = v
			return nil
		}, nil
	}
	return nil, fmt.Errorf("a result of class %s cannot be stored", n.class)
}

// fieldSetNode compiles req.Method = value to a typed store at a
// compile-time offset. Only the single field of a pointer to a struct
// is in the table; a value-struct base or a deeper chain stays on the
// reflect evaluator.
func (c *jitCompiler) fieldSetNode(fs *vmFieldSet) (nodeE, error) {
	field, ok := c.slotOf[fs.base]
	if !ok {
		return nil, fmt.Errorf("a field target has no slot")
	}
	bt := c.types[field]
	if bt.Kind() != reflect.Pointer || bt.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("a field of a %s is not in the table", bt)
	}
	if len(fs.steps) != 1 || !fs.steps[0].deref || len(fs.steps[0].index) != 1 {
		return nil, fmt.Errorf("only a single field of a pointer to a struct is in the table")
	}
	sf := bt.Elem().Field(fs.steps[0].index[0])
	cl := layoutOf(sf.Type)
	if cl == lBad {
		return nil, fmt.Errorf("field %s of type %s has no layout class", sf.Name, sf.Type)
	}

	var val node
	switch fs.val.kind {
	case vaConst:
		v, err := constNode(cl, fs.val.val)
		if err != nil {
			return nil, err
		}
		val = v
	case vaCall:
		v, err := c.exprNode(fs.val.sub)
		if err != nil {
			return nil, err
		}
		if v.class != cl {
			return nil, fmt.Errorf("a %s result cannot fill a %s field", v.class, cl)
		}
		val = v
	default:
		return nil, fmt.Errorf("this field value is not in the table")
	}

	baseOff, fieldOff, name, srcType := c.offs[field], sf.Offset, fs.field, bt
	target := func(fr unsafe.Pointer) (unsafe.Pointer, error) {
		base := *(*unsafe.Pointer)(unsafe.Add(fr, baseOff))
		if base == nil {
			return nil, fmt.Errorf("exec: %s: field write on a nil %s", name, srcType)
		}
		return unsafe.Add(base, fieldOff), nil
	}

	if cl.scalar() {
		if cl.float() {
			f := val.F
			return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
				v, err := f(fr, ctx, st, d)
				if err != nil {
					return err
				}
				at, err := target(fr)
				if err != nil {
					return err
				}
				storeF(cl, at, v)
				return nil
			}, nil
		}
		f := val.N
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			at, err := target(fr)
			if err != nil {
				return err
			}
			storeN(cl, at, v)
			return nil
		}, nil
	}
	switch cl {
	case lStr:
		f := val.S
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			at, err := target(fr)
			if err != nil {
				return err
			}
			*(*string)(at) = v
			return nil
		}, nil
	case lPtr:
		f := val.P
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			at, err := target(fr)
			if err != nil {
				return err
			}
			*(*unsafe.Pointer)(at) = v
			return nil
		}, nil
	case lSlice:
		f := val.L
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			at, err := target(fr)
			if err != nil {
				return err
			}
			*(*sliceHdr)(at) = v
			return nil
		}, nil
	case lIface:
		f := val.I
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			v, err := f(fr, ctx, st, d)
			if err != nil {
				return err
			}
			at, err := target(fr)
			if err != nil {
				return err
			}
			*(*ifacePair)(at) = v
			return nil
		}, nil
	}
	return nil, fmt.Errorf("a %s field cannot be stored", cl)
}

// dropped wraps a node whose value nothing binds, reporting a class
// that cannot be run rather than returning a nil closure.
func (c *jitCompiler) dropped(n node) (nodeE, error) {
	if e := dropNode(n); e != nil {
		return e, nil
	}
	return nil, fmt.Errorf("a result of class %s cannot be discarded", n.class)
}

// dropNode runs a node for its effects and discards its value. It
// returns nil for a class it cannot run, which the caller reports.
func dropNode(n node) nodeE {
	if n.class.scalar() {
		if n.class.float() {
			f := n.F
			return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
				_, err := f(fr, ctx, st, d)
				return err
			}
		}
		f := n.N
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			_, err := f(fr, ctx, st, d)
			return err
		}
	}
	switch n.class {
	case lNone:
		return n.E
	case lPtr:
		f := n.P
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			_, err := f(fr, ctx, st, d)
			return err
		}
	case lSlice:
		f := n.L
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			_, err := f(fr, ctx, st, d)
			return err
		}
	case lStr:
		f := n.S
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			_, err := f(fr, ctx, st, d)
			return err
		}
	case lIface:
		f := n.I
		return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			_, err := f(fr, ctx, st, d)
			return err
		}
	}
	return nil
}

// exprNode compiles one call and its arguments. A call the shape table
// cannot express compiles to a reflect bridge instead of failing the
// program: one call pays reflect.Value.Call, its neighbours stay
// direct. The vmCall tree was already type-checked by the program
// compiler, so a miss here is a capability gap, never a type error.
func (c *jitCompiler) exprNode(call *vmCall) (node, error) {
	n, err := c.directNode(call)
	if err == nil {
		return n, nil
	}
	c.bridged = append(c.bridged, fmt.Sprintf("%s (%v)", call.name, err))
	return c.bridgeNode(call)
}

// directNode is the in-table compilation.
func (c *jitCompiler) directNode(call *vmCall) (node, error) {
	ft := call.fn.Type()
	// A variadic function is ABI-identical to the same signature with
	// the variadic parameter as a plain slice: the callee receives the
	// slice header either way, verified in TestVariadicSliceABI. A
	// spread call passes the slice it was given; a packed call builds
	// one from its element nodes, which is the allocation the Go
	// compiler makes at an escaping call site.
	fixed := ft.NumIn()
	if ft.IsVariadic() && !call.spread {
		fixed--
	}
	args := make([]node, 0, ft.NumIn())
	key := ""
	for i := 0; i < fixed; i++ {
		pt := ft.In(i)
		cl := layoutOf(pt)
		if cl == lBad {
			return node{}, fmt.Errorf("%s: parameter %d of type %s has no layout class", call.name, i+1, pt)
		}
		key += cl.String()
		a, err := c.argNode(call.args[i], pt, cl)
		if err != nil {
			return node{}, fmt.Errorf("%s: argument %d: %w", call.name, i+1, err)
		}
		args = append(args, a)
	}
	if ft.IsVariadic() && !call.spread {
		st := ft.In(ft.NumIn() - 1)
		pack, err := c.packNode(call, st, call.args[fixed:])
		if err != nil {
			return node{}, fmt.Errorf("%s: %w", call.name, err)
		}
		key += lSlice.String()
		args = append(args, pack)
	}
	key += "_"
	for j := 0; j < ft.NumOut(); j++ {
		if j == call.errIdx {
			key += lErr.String()
			continue
		}
		cl := layoutOf(ft.Out(j))
		if cl == lBad {
			return node{}, fmt.Errorf("%s: result %d of type %s has no layout class", call.name, j+1, ft.Out(j))
		}
		key += cl.String()
	}

	n, ok := callNode(key, funcPtr(call.fn.Interface()), args)
	if !ok {
		return node{}, fmt.Errorf("%s: shape %q is not in the table", call.name, key)
	}
	return n, nil
}

// packNode builds the variadic slice from the element nodes, for the
// element types worth special-casing: []any and []string cover the
// printf and assert families. Anything else bridges. The slice is
// allocated per call because the callee may keep it, which is the same
// escape the Go compiler assumes at a call site whose arguments escape.
func (c *jitCompiler) packNode(call *vmCall, st reflect.Type, elems []*vmArg) (node, error) {
	et := st.Elem()
	cl := layoutOf(et)
	if cl == lBad {
		return node{}, fmt.Errorf("a variadic element of type %s has no layout class", et)
	}
	nodes := make([]node, len(elems))
	for i, a := range elems {
		n, err := c.argNode(a, et, cl)
		if err != nil {
			return node{}, err
		}
		nodes[i] = n
	}
	switch {
	case et.Kind() == reflect.Interface && et.NumMethod() == 0:
		getters := make([]nodeI, len(nodes))
		for i, n := range nodes {
			getters[i] = n.I
		}
		return node{class: lSlice, L: func(fr unsafe.Pointer, ctx context.Context, stk map[string]any, d any) (sliceHdr, error) {
			out := make([]any, len(getters))
			for i, g := range getters {
				pair, err := g(fr, ctx, stk, d)
				if err != nil {
					return sliceHdr{}, err
				}
				// A typed pointer store into the slice element keeps
				// the write barrier.
				*(*ifacePair)(unsafe.Pointer(&out[i])) = pair
			}
			return *(*sliceHdr)(unsafe.Pointer(&out)), nil
		}}, nil
	case et.Kind() == reflect.String:
		getters := make([]nodeS, len(nodes))
		for i, n := range nodes {
			getters[i] = n.S
		}
		return node{class: lSlice, L: func(fr unsafe.Pointer, ctx context.Context, stk map[string]any, d any) (sliceHdr, error) {
			out := make([]string, len(getters))
			for i, g := range getters {
				v, err := g(fr, ctx, stk, d)
				if err != nil {
					return sliceHdr{}, err
				}
				out[i] = v
			}
			return *(*sliceHdr)(unsafe.Pointer(&out)), nil
		}}, nil
	}
	return node{}, fmt.Errorf("packing []%s is not in the table", et)
}

// bridgeNode compiles a call to one reflect.Value.Call, its arguments
// resolved against the same frame the direct calls use. A slot argument
// is read in place through reflect.NewAt, so the bridge shares state
// with its JIT'd neighbours rather than needing the reflect
// evaluator's slot array.
func (c *jitCompiler) bridgeNode(call *vmCall) (node, error) {
	getters := make([]func(unsafe.Pointer, context.Context, map[string]any, any) (reflect.Value, error), len(call.args))
	for i, a := range call.args {
		g, err := c.bridgeArg(a)
		if err != nil {
			return node{}, err
		}
		getters[i] = g
	}

	fn, name, errIdx, spread := call.fn, call.name, call.errIdx, call.spread
	invoke := func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) ([]reflect.Value, error) {
		args := make([]reflect.Value, len(getters))
		for i, g := range getters {
			v, err := g(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
		var out []reflect.Value
		if spread {
			out = fn.CallSlice(args)
		} else {
			out = fn.Call(args)
		}
		if errIdx >= 0 {
			if e := out[errIdx]; !e.IsNil() {
				return nil, e.Interface().(error)
			}
		}
		return out, nil
	}
	_ = name

	if call.nres == 0 {
		return node{class: lNone, E: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
			_, err := invoke(fr, ctx, st, d)
			return err
		}}, nil
	}
	rt := callResultType(call, 0)
	cl := layoutOf(rt)
	if cl == lBad {
		return node{}, fmt.Errorf("%s: a bridged result of type %s has no layout class", call.name, rt)
	}
	resIdx := 0
	if call.errIdx == 0 {
		resIdx = 1
	}
	// The result value's words are read out of a typed cell, which is
	// the boxing reflect did anyway.
	pick := func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
		out, err := invoke(fr, ctx, st, d)
		if err != nil {
			return nil, err
		}
		cell := reflect.New(rt)
		cell.Elem().Set(out[resIdx])
		return cell.UnsafePointer(), nil
	}
	switch {
	case cl == lPtr:
		return node{class: cl, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			at, err := pick(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			return *(*unsafe.Pointer)(at), nil
		}}, nil
	case cl == lStr:
		return node{class: cl, S: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (string, error) {
			at, err := pick(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			return *(*string)(at), nil
		}}, nil
	case cl == lSlice:
		return node{class: cl, L: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (sliceHdr, error) {
			at, err := pick(fr, ctx, st, d)
			if err != nil {
				return sliceHdr{}, err
			}
			return *(*sliceHdr)(at), nil
		}}, nil
	case cl == lIface:
		return node{class: cl, I: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (ifacePair, error) {
			at, err := pick(fr, ctx, st, d)
			if err != nil {
				return ifacePair{}, err
			}
			return *(*ifacePair)(at), nil
		}}, nil
	case cl.float():
		return node{class: cl, F: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (float64, error) {
			at, err := pick(fr, ctx, st, d)
			if err != nil {
				return 0, err
			}
			return loadF(cl, at), nil
		}}, nil
	default:
		return node{class: cl, N: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (uint64, error) {
			at, err := pick(fr, ctx, st, d)
			if err != nil {
				return 0, err
			}
			return loadN(cl, at), nil
		}}, nil
	}
}

// bridgeArg resolves one argument of a bridged call to a reflect.Value.
func (c *jitCompiler) bridgeArg(a *vmArg) (func(unsafe.Pointer, context.Context, map[string]any, any) (reflect.Value, error), error) {
	switch a.kind {
	case vaConst:
		v := a.val
		return func(unsafe.Pointer, context.Context, map[string]any, any) (reflect.Value, error) { return v, nil }, nil

	case vaCtx:
		return func(_ unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (reflect.Value, error) {
			return reflect.ValueOf(ctx), nil
		}, nil

	case vaSlot:
		if producer := c.splices[a]; producer != nil {
			sub, err := c.exprNode(producer)
			if err != nil {
				return nil, err
			}
			return c.nodeToValue(sub, callResultType(producer, 0))
		}
		field, ok := c.slotOf[a.slot]
		if !ok {
			return nil, fmt.Errorf("a bridged name has no slot")
		}
		off, st := c.offs[field], c.types[field]
		if !st.AssignableTo(a.typ) {
			return nil, fmt.Errorf("cannot use %s as %s", st, a.typ)
		}
		// The frame is typed storage, so the value is read in place.
		return func(fr unsafe.Pointer, _ context.Context, _ map[string]any, _ any) (reflect.Value, error) {
			return reflect.NewAt(st, unsafe.Add(fr, off)).Elem(), nil
		}, nil

	case vaStack, vaDest:
		arg, typ := a, a.typ
		return func(_ unsafe.Pointer, _ context.Context, stack map[string]any, dest any) (reflect.Value, error) {
			v := dest
			name := "dest"
			if arg.kind == vaStack {
				var ok bool
				name = arg.name
				v, ok = stack[name]
				if !ok || v == nil {
					return reflect.Zero(typ), nil
				}
			} else if v == nil {
				return reflect.Value{}, fmt.Errorf("exec: dest is only set by Scan")
			}
			rv := reflect.ValueOf(v)
			if !arg.assignable(rv.Type()) {
				return reflect.Value{}, fmt.Errorf("exec: variable %q: cannot use %s as %s", name, rv.Type(), typ)
			}
			return rv, nil
		}, nil

	case vaCall:
		sub, err := c.exprNode(a.sub)
		if err != nil {
			return nil, err
		}
		rt := callResultType(a.sub, 0)
		return c.nodeToValue(sub, rt)

	case vaField:
		fieldNode, err := c.argNode(a, a.typ, layoutOf(a.typ))
		if err != nil {
			return nil, err
		}
		return c.nodeToValue(fieldNode, a.typ)
	}
	return nil, fmt.Errorf("a bridged argument of kind %d is not supported", a.kind)
}

// nodeToValue adapts a compiled node into a reflect.Value producer, by
// writing the node's words into a typed cell.
func (c *jitCompiler) nodeToValue(n node, rt reflect.Type) (func(unsafe.Pointer, context.Context, map[string]any, any) (reflect.Value, error), error) {
	if rt == nil {
		return nil, fmt.Errorf("a bridged argument with no result type")
	}
	store := func(cell reflect.Value, fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) error {
		at := cell.UnsafePointer()
		switch n.class {
		case lPtr:
			v, err := n.P(fr, ctx, st, d)
			if err != nil {
				return err
			}
			*(*unsafe.Pointer)(at) = v
		case lStr:
			v, err := n.S(fr, ctx, st, d)
			if err != nil {
				return err
			}
			*(*string)(at) = v
		case lSlice:
			v, err := n.L(fr, ctx, st, d)
			if err != nil {
				return err
			}
			*(*sliceHdr)(at) = v
		case lIface:
			v, err := n.I(fr, ctx, st, d)
			if err != nil {
				return err
			}
			*(*ifacePair)(at) = v
		default:
			if n.class.float() {
				v, err := n.F(fr, ctx, st, d)
				if err != nil {
					return err
				}
				storeF(n.class, at, v)
			} else {
				v, err := n.N(fr, ctx, st, d)
				if err != nil {
					return err
				}
				storeN(n.class, at, v)
			}
		}
		return nil
	}
	return func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (reflect.Value, error) {
		cell := reflect.New(rt)
		if err := store(cell, fr, ctx, st, d); err != nil {
			return reflect.Value{}, err
		}
		return cell.Elem(), nil
	}, nil
}

// argNode compiles one argument to the class its parameter wants.
func (c *jitCompiler) argNode(a *vmArg, pt reflect.Type, cl layout) (node, error) {
	switch a.kind {
	case vaCall:
		sub, err := c.exprNode(a.sub)
		if err != nil {
			return node{}, err
		}
		if sub.class == cl {
			return sub, nil
		}
		if cl == lIface {
			return c.toIface(sub, callResultType(a.sub, 0), pt)
		}
		return node{}, fmt.Errorf("a %s result cannot fill a %s parameter", sub.class, cl)

	case vaSlot:
		if producer := c.splices[a]; producer != nil {
			sub, err := c.exprNode(producer)
			if err != nil {
				return node{}, err
			}
			if sub.class == cl {
				return sub, nil
			}
			if cl == lIface {
				return c.toIface(sub, callResultType(producer, 0), pt)
			}
			return node{}, fmt.Errorf("a %s result cannot fill a %s parameter", sub.class, cl)
		}
		field, ok := c.slotOf[a.slot]
		if !ok {
			return node{}, fmt.Errorf("a name has no slot")
		}
		off, st := c.offs[field], c.types[field]
		if cl == lIface && st.Kind() != reflect.Interface {
			tab, ok := itabFor(st, pt)
			if !ok {
				return node{}, fmt.Errorf("%s does not implement %s", st, pt)
			}
			if layoutOf(st) == lPtr {
				// Stored directly: the data word is the pointer itself,
				// read out of the slot, so nothing aliases the frame.
				return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (ifacePair, error) {
					return ifacePair{tab: tab, data: *(*unsafe.Pointer)(unsafe.Add(fr, off))}, nil
				}}, nil
			}
			// Stored indirectly. Pointing the interface at the slot
			// rather than a copy saves the allocation, but is only
			// legal while the slot is written once: the frame outlives
			// the call and the callee may keep the interface. A slot
			// written more than once, and any scalar, is copied
			// instead, which is what the Go compiler does anyway.
			if c.writes[a.slot] == 1 && !layoutOf(st).scalar() {
				return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (ifacePair, error) {
					return ifacePair{tab: tab, data: unsafe.Add(fr, off)}, nil
				}}, nil
			}
			return c.toIface(slotNode(layoutOf(st), off), st, pt)
		}
		if layoutOf(st) != cl {
			return node{}, fmt.Errorf("a %s name cannot fill a %s parameter", layoutOf(st), cl)
		}
		return slotNode(cl, off), nil

	case vaConst:
		return constNode(cl, a.val)

	case vaCtx:
		// The execution context is already the exact interface type the
		// parameter wants, so the two words copy straight through.
		return node{class: lIface, I: func(_ unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (ifacePair, error) {
			return *(*ifacePair)(unsafe.Pointer(&ctx)), nil
		}}, nil

	case vaField:
		return c.fieldNode(a, pt, cl)

	case vaStack, vaDest:
		return c.dynamicNode(a, pt, cl)
	}
	// Every kind is named above. Reaching here means a new one was
	// added without a case, and the previous shape of this switch sent
	// it to dynamicNode, which looked up the empty name, found nothing
	// and produced a nil interface: a wrong answer with no error. That
	// is what adding vaField did.
	return node{}, fmt.Errorf("argument kind %d has no node", a.kind)
}

// fieldNode reads one struct field out of a pointer. The offset is
// known when the program compiles, so the load is an add and a move
// rather than a call.
//
// Only a single field of a pointer to a struct is in the table. A
// deeper index reaches through embedded types whose offsets do not
// simply add when one of them is itself a pointer, so those go to the
// reflect evaluator.
func (c *jitCompiler) fieldNode(a *vmArg, pt reflect.Type, cl layout) (node, error) {
	if !a.deref || len(a.index) != 1 {
		return node{}, fmt.Errorf("only a single field of a pointer to a struct is in the table")
	}
	srcType := a.src.typ
	if srcType == nil || srcType.Kind() != reflect.Pointer || srcType.Elem().Kind() != reflect.Struct {
		return node{}, fmt.Errorf("a field source must be a pointer to a struct")
	}
	sf := srcType.Elem().Field(a.index[0])
	fcl := layoutOf(sf.Type)
	if fcl == lBad {
		return node{}, fmt.Errorf("field %s of type %s has no layout class", sf.Name, sf.Type)
	}
	src, err := c.argNode(a.src, srcType, lPtr)
	if err != nil {
		return node{}, err
	}
	sp, off, name := src.P, sf.Offset, sf.Name

	load := func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
		p, err := sp(fr, ctx, st, d)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, fmt.Errorf("exec: field %s read on a nil %s", name, srcType)
		}
		return unsafe.Add(p, off), nil
	}

	var out node
	if fcl.scalar() {
		if fcl.float() {
			out = node{class: fcl, F: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (float64, error) {
				at, err := load(fr, ctx, st, d)
				if err != nil {
					return 0, err
				}
				return loadF(fcl, at), nil
			}}
		} else {
			out = node{class: fcl, N: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (uint64, error) {
				at, err := load(fr, ctx, st, d)
				if err != nil {
					return 0, err
				}
				return loadN(fcl, at), nil
			}}
		}
	}
	switch fcl {
	case lPtr:
		out = node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (unsafe.Pointer, error) {
			at, err := load(fr, ctx, st, d)
			if err != nil {
				return nil, err
			}
			return *(*unsafe.Pointer)(at), nil
		}}
	case lStr:
		out = node{class: lStr, S: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (string, error) {
			at, err := load(fr, ctx, st, d)
			if err != nil {
				return "", err
			}
			return *(*string)(at), nil
		}}
	case lSlice:
		out = node{class: lSlice, L: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (sliceHdr, error) {
			at, err := load(fr, ctx, st, d)
			if err != nil {
				return sliceHdr{}, err
			}
			return *(*sliceHdr)(at), nil
		}}
	case lIface:
		out = node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, st map[string]any, d any) (ifacePair, error) {
			at, err := load(fr, ctx, st, d)
			if err != nil {
				return ifacePair{}, err
			}
			return *(*ifacePair)(at), nil
		}}
	}

	if out.class == cl {
		return out, nil
	}
	if cl == lIface {
		return c.toIface(out, sf.Type, pt)
	}
	return node{}, fmt.Errorf("a %s field cannot fill a %s parameter", out.class, cl)
}

// toIface wraps a computed value as an interface. A pointer-shaped
// value is the data word itself; anything wider is stored indirectly,
// which is the allocation the Go compiler makes at the same place.
func (c *jitCompiler) toIface(sub node, st, pt reflect.Type) (node, error) {
	if st == nil {
		return node{}, fmt.Errorf("a call with no result cannot become an interface")
	}
	tab, ok := itabFor(st, pt)
	if !ok {
		return node{}, fmt.Errorf("%s does not implement %s", st, pt)
	}
	if sub.class.scalar() {
		return scalarIface(sub, tab)
	}
	switch sub.class {
	case lPtr:
		f := sub.P
		return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, s map[string]any, d any) (ifacePair, error) {
			v, err := f(fr, ctx, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			return ifacePair{tab: tab, data: v}, nil
		}}, nil
	case lSlice:
		f := sub.L
		return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, s map[string]any, d any) (ifacePair, error) {
			v, err := f(fr, ctx, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			return ifacePair{tab: tab, data: unsafe.Pointer(&v)}, nil
		}}, nil
	case lStr:
		f := sub.S
		return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, s map[string]any, d any) (ifacePair, error) {
			v, err := f(fr, ctx, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			return ifacePair{tab: tab, data: unsafe.Pointer(&v)}, nil
		}}, nil
	}
	return node{}, fmt.Errorf("a %s result cannot become an interface", sub.class)
}

// scalarIface boxes a scalar into an interface. The data word has to
// point at a value of the concrete width, so each class materialises
// one of its own type; that escape is the allocation the Go compiler
// makes at the same place.
func scalarIface(sub node, tab unsafe.Pointer) (node, error) {
	mk := func(box func(uint64) unsafe.Pointer) (node, error) {
		f := sub.N
		return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, s map[string]any, d any) (ifacePair, error) {
			n, err := f(fr, ctx, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			return ifacePair{tab: tab, data: box(n)}, nil
		}}, nil
	}
	switch sub.class {
	case lBool:
		return mk(func(n uint64) unsafe.Pointer { v := n != 0; return unsafe.Pointer(&v) })
	case lI8:
		return mk(func(n uint64) unsafe.Pointer { v := int8(uint8(n)); return unsafe.Pointer(&v) })
	case lI16:
		return mk(func(n uint64) unsafe.Pointer { v := int16(uint16(n)); return unsafe.Pointer(&v) })
	case lI32:
		return mk(func(n uint64) unsafe.Pointer { v := int32(uint32(n)); return unsafe.Pointer(&v) })
	case lI64:
		return mk(func(n uint64) unsafe.Pointer { v := int64(n); return unsafe.Pointer(&v) })
	case lU8:
		return mk(func(n uint64) unsafe.Pointer { v := uint8(n); return unsafe.Pointer(&v) })
	case lU16:
		return mk(func(n uint64) unsafe.Pointer { v := uint16(n); return unsafe.Pointer(&v) })
	case lU32:
		return mk(func(n uint64) unsafe.Pointer { v := uint32(n); return unsafe.Pointer(&v) })
	case lU64:
		return mk(func(n uint64) unsafe.Pointer { v := n; return unsafe.Pointer(&v) })
	case lF32:
		f := sub.F
		return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, s map[string]any, d any) (ifacePair, error) {
			x, err := f(fr, ctx, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			v := float32(x)
			return ifacePair{tab: tab, data: unsafe.Pointer(&v)}, nil
		}}, nil
	case lF64:
		f := sub.F
		return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, s map[string]any, d any) (ifacePair, error) {
			v, err := f(fr, ctx, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			return ifacePair{tab: tab, data: unsafe.Pointer(&v)}, nil
		}}, nil
	}
	return node{}, fmt.Errorf("a %s cannot become an interface", sub.class)
}

func slotNode(cl layout, off uintptr) node {
	if cl.scalar() {
		if cl.float() {
			return node{class: cl, F: func(fr unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (float64, error) {
				return loadF(cl, unsafe.Add(fr, off)), nil
			}}
		}
		return node{class: cl, N: func(fr unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (uint64, error) {
			return loadN(cl, unsafe.Add(fr, off)), nil
		}}
	}
	switch cl {
	case lPtr:
		return node{class: lPtr, P: func(fr unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (unsafe.Pointer, error) {
			return *(*unsafe.Pointer)(unsafe.Add(fr, off)), nil
		}}
	case lSlice:
		return node{class: lSlice, L: func(fr unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (sliceHdr, error) {
			return *(*sliceHdr)(unsafe.Add(fr, off)), nil
		}}
	case lStr:
		return node{class: lStr, S: func(fr unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (string, error) {
			return *(*string)(unsafe.Add(fr, off)), nil
		}}
	default:
		return node{class: lIface, I: func(fr unsafe.Pointer, ctx context.Context, _ map[string]any, _ any) (ifacePair, error) {
			return *(*ifacePair)(unsafe.Add(fr, off)), nil
		}}
	}
}

func constNode(cl layout, v reflect.Value) (node, error) {
	if cl.scalar() {
		n, f := scalarBits(cl, v)
		if cl.float() {
			return node{class: cl, F: func(unsafe.Pointer, context.Context, map[string]any, any) (float64, error) { return f, nil }}, nil
		}
		return node{class: cl, N: func(unsafe.Pointer, context.Context, map[string]any, any) (uint64, error) { return n, nil }}, nil
	}
	switch cl {
	case lStr:
		s := v.String()
		return node{class: lStr, S: func(unsafe.Pointer, context.Context, map[string]any, any) (string, error) { return s, nil }}, nil
	case lPtr:
		if !v.IsZero() {
			return node{}, fmt.Errorf("only a zero pointer can be a constant")
		}
		return node{class: lPtr, P: func(unsafe.Pointer, context.Context, map[string]any, any) (unsafe.Pointer, error) { return nil, nil }}, nil
	case lIface:
		if v.IsZero() && v.Kind() == reflect.Interface {
			return node{class: lIface, I: func(unsafe.Pointer, context.Context, map[string]any, any) (ifacePair, error) { return ifacePair{}, nil }}, nil
		}
		// A non-nil constant boxes once at compile time. The cell is a
		// heap value reflect allocated; the closure keeps it reachable,
		// and both words of the pair are pointers the collector traces.
		// Sharing one box across calls is safe because a literal is
		// immutable.
		cell := reflect.New(reflect.TypeFor[any]()).Elem()
		cell.Set(v)
		pair := *(*ifacePair)(cell.Addr().UnsafePointer())
		keep := cell
		return node{class: lIface, I: func(unsafe.Pointer, context.Context, map[string]any, any) (ifacePair, error) {
			_ = keep
			return pair, nil
		}}, nil
	case lSlice:
		if !v.IsZero() {
			return node{}, fmt.Errorf("only a nil slice can be a constant")
		}
		return node{class: lSlice, L: func(unsafe.Pointer, context.Context, map[string]any, any) (sliceHdr, error) { return sliceHdr{}, nil }}, nil
	}
	return node{}, fmt.Errorf("a constant of class %s is not supported", cl)
}

// dynamicNode compiles a value whose type is only known when it is
// read: from the caller's stack, or the pointer Scan was handed.
func (c *jitCompiler) dynamicNode(a *vmArg, pt reflect.Type, cl layout) (node, error) {
	isDest := a.kind == vaDest
	name := a.name
	if isDest {
		name = "dest"
	}
	switch cl {
	case lStr:
		if isDest {
			return node{}, fmt.Errorf("dest cannot fill a string parameter")
		}
		return node{class: lStr, S: func(_ unsafe.Pointer, _ context.Context, st map[string]any, _ any) (string, error) {
			v, ok := st[name]
			if !ok || v == nil {
				return "", nil
			}
			s, ok := v.(string)
			if !ok {
				return "", fmt.Errorf("exec: variable %q: cannot use %T as string", name, v)
			}
			return s, nil
		}}, nil
	case lIface:
		conv, ok := ifaceConvs[pt]
		if !ok {
			return node{}, fmt.Errorf("%s is not in ifaceConvs", pt)
		}
		if isDest {
			return node{class: lIface, I: func(_ unsafe.Pointer, _ context.Context, _ map[string]any, d any) (ifacePair, error) {
				if d == nil {
					return ifacePair{}, fmt.Errorf("exec: dest is only set by Scan")
				}
				pair, ok := conv(d)
				if !ok {
					return ifacePair{}, fmt.Errorf("exec: variable %q: cannot use %T as %s", name, d, pt)
				}
				return pair, nil
			}}, nil
		}
		return node{class: lIface, I: func(_ unsafe.Pointer, _ context.Context, st map[string]any, _ any) (ifacePair, error) {
			v, ok := st[name]
			if !ok || v == nil {
				return ifacePair{}, nil
			}
			pair, ok := conv(v)
			if !ok {
				return ifacePair{}, fmt.Errorf("exec: variable %q: cannot use %T as %s", name, v, pt)
			}
			return pair, nil
		}}, nil
	}
	if cl.scalar() {
		if isDest {
			return node{}, fmt.Errorf("dest cannot fill a %s parameter", cl)
		}
		return stackScalarNode(name, pt, cl)
	}
	return node{}, fmt.Errorf("a stack value cannot fill a %s parameter", cl)
}

// stackScalarNode reads a scalar off the caller's stack. The stack
// holds any, so the read is one type assertion against the exact
// parameter type; a named scalar type would need reflect to check and
// stays on the reflect evaluator. An unset or nil entry is the zero
// value, like every other stack read.
func stackScalarNode(name string, pt reflect.Type, cl layout) (node, error) {
	get := stackScalarConvs[pt]
	if get == nil {
		return node{}, fmt.Errorf("a stack value of named type %s stays on the reflect tier", pt)
	}
	if cl.float() {
		return node{class: cl, F: func(_ unsafe.Pointer, _ context.Context, st map[string]any, _ any) (float64, error) {
			v, ok := st[name]
			if !ok || v == nil {
				return 0, nil
			}
			bits, fl, ok := get(v)
			if !ok {
				return 0, fmt.Errorf("exec: variable %q: cannot use %T as %s", name, v, pt)
			}
			_ = bits
			return fl, nil
		}}, nil
	}
	return node{class: cl, N: func(_ unsafe.Pointer, _ context.Context, st map[string]any, _ any) (uint64, error) {
		v, ok := st[name]
		if !ok || v == nil {
			return 0, nil
		}
		bits, _, ok := get(v)
		if !ok {
			return 0, fmt.Errorf("exec: variable %q: cannot use %T as %s", name, v, pt)
		}
		return bits, nil
	}}, nil
}

// stackScalarConvs asserts a stack any to each predeclared scalar type
// and reports its bits. Keyed by exact type, so a named type misses.
var stackScalarConvs = map[reflect.Type]func(any) (uint64, float64, bool){
	reflect.TypeFor[bool](): func(v any) (uint64, float64, bool) {
		b, ok := v.(bool)
		if b {
			return 1, 0, ok
		}
		return 0, 0, ok
	},
	reflect.TypeFor[int](): func(v any) (uint64, float64, bool) {
		n, ok := v.(int)
		return uint64(int64(n)), 0, ok
	},
	reflect.TypeFor[int8](): func(v any) (uint64, float64, bool) {
		n, ok := v.(int8)
		return uint64(uint8(n)), 0, ok
	},
	reflect.TypeFor[int16](): func(v any) (uint64, float64, bool) {
		n, ok := v.(int16)
		return uint64(uint16(n)), 0, ok
	},
	reflect.TypeFor[int32](): func(v any) (uint64, float64, bool) {
		n, ok := v.(int32)
		return uint64(uint32(n)), 0, ok
	},
	reflect.TypeFor[int64](): func(v any) (uint64, float64, bool) {
		n, ok := v.(int64)
		return uint64(n), 0, ok
	},
	reflect.TypeFor[uint](): func(v any) (uint64, float64, bool) {
		n, ok := v.(uint)
		return uint64(n), 0, ok
	},
	reflect.TypeFor[uint8](): func(v any) (uint64, float64, bool) {
		n, ok := v.(uint8)
		return uint64(n), 0, ok
	},
	reflect.TypeFor[uint16](): func(v any) (uint64, float64, bool) {
		n, ok := v.(uint16)
		return uint64(n), 0, ok
	},
	reflect.TypeFor[uint32](): func(v any) (uint64, float64, bool) {
		n, ok := v.(uint32)
		return uint64(n), 0, ok
	},
	reflect.TypeFor[uint64](): func(v any) (uint64, float64, bool) {
		n, ok := v.(uint64)
		return n, 0, ok
	},
	reflect.TypeFor[float32](): func(v any) (uint64, float64, bool) {
		f, ok := v.(float32)
		return 0, float64(f), ok
	},
	reflect.TypeFor[float64](): func(v any) (uint64, float64, bool) {
		f, ok := v.(float64)
		return 0, f, ok
	},
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
