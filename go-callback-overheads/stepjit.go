package callbacks

import (
	"fmt"
	"reflect"
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
	case lNone:
		return ""
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
	nodeP func(f unsafe.Pointer, stack map[string]any, dest any) (unsafe.Pointer, error)
	nodeS func(f unsafe.Pointer, stack map[string]any, dest any) (string, error)
	nodeI func(f unsafe.Pointer, stack map[string]any, dest any) (ifacePair, error)
	nodeL func(f unsafe.Pointer, stack map[string]any, dest any) (sliceHdr, error)
	nodeE func(f unsafe.Pointer, stack map[string]any, dest any) error
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
}

// jitProgram is a program whose every call JITs.
type jitProgram struct {
	frameType reflect.Type
	frameRT   unsafe.Pointer // nil when the program needs no slots
	stmts     []nodeE

	// retType and retOff describe the slot a trailing "return expr;"
	// leaves its value in. retType is nil when the program has no
	// value, which is the common case: the output goes through dest.
	retType reflect.Type
	retOff  uintptr
}

func (p *jitProgram) run(stack map[string]any, dest any) (any, error) {
	var f unsafe.Pointer
	if p.frameRT != nil {
		f = unsafeNew(p.frameRT)
	}
	for _, stmt := range p.stmts {
		if err := stmt(f, stack, dest); err != nil {
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
	stP_L    = func(unsafe.Pointer) sliceHdr
	stI_P    = func(ifacePair) unsafe.Pointer
	stPI_E   = func(unsafe.Pointer, ifacePair) ifacePair
	stSSI_PE = func(string, string, ifacePair) (unsafe.Pointer, ifacePair)
	stSS_PE  = func(string, string) (unsafe.Pointer, ifacePair)
	stS_PE   = func(string) (unsafe.Pointer, ifacePair)
	stI_PE   = func(ifacePair) (unsafe.Pointer, ifacePair)
	stS_P    = func(string) unsafe.Pointer
	stP_P    = func(unsafe.Pointer) unsafe.Pointer
	stP_S    = func(unsafe.Pointer) string
	stP_I    = func(unsafe.Pointer) ifacePair
	stP_E    = func(unsafe.Pointer) ifacePair
	stPP_E   = func(unsafe.Pointer, unsafe.Pointer) ifacePair
	stII_E   = func(ifacePair, ifacePair) ifacePair
	stSS_E   = func(string, string) ifacePair
	stPP_PE  = func(unsafe.Pointer, unsafe.Pointer) (unsafe.Pointer, ifacePair)
)

// callNode builds the node for one call from the nodes of its
// arguments, or reports that the shape is outside the table.
func callNode(key string, fptr unsafe.Pointer, a []node) (node, bool) {
	switch key {
	case "P_L":
		f, a0 := castFn[stP_L](fptr), a[0].P
		return node{class: lSlice, L: func(fr unsafe.Pointer, st map[string]any, d any) (sliceHdr, error) {
			p, err := a0(fr, st, d)
			if err != nil {
				return sliceHdr{}, err
			}
			return f(p), nil
		}}, true

	case "I_P":
		f, a0 := castFn[stI_P](fptr), a[0].I
		return node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			i0, err := a0(fr, st, d)
			if err != nil {
				return nil, err
			}
			return f(i0), nil
		}}, true

	case "PI_E":
		f, a0, a1 := castFn[stPI_E](fptr), a[0].P, a[1].I
		return node{class: lNone, E: func(fr unsafe.Pointer, st map[string]any, d any) error {
			p0, err := a0(fr, st, d)
			if err != nil {
				return err
			}
			i1, err := a1(fr, st, d)
			if err != nil {
				return err
			}
			return asError(f(p0, i1))
		}}, true

	case "SSI_PE":
		f, a0, a1, a2 := castFn[stSSI_PE](fptr), a[0].S, a[1].S, a[2].I
		return node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			s0, err := a0(fr, st, d)
			if err != nil {
				return nil, err
			}
			s1, err := a1(fr, st, d)
			if err != nil {
				return nil, err
			}
			i2, err := a2(fr, st, d)
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
		return node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			s0, err := a0(fr, st, d)
			if err != nil {
				return nil, err
			}
			s1, err := a1(fr, st, d)
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
		return node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			s0, err := a0(fr, st, d)
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
		return node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			i0, err := a0(fr, st, d)
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
		return node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			s0, err := a0(fr, st, d)
			if err != nil {
				return nil, err
			}
			return f(s0), nil
		}}, true

	case "P_P":
		f, a0 := castFn[stP_P](fptr), a[0].P
		return node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			p0, err := a0(fr, st, d)
			if err != nil {
				return nil, err
			}
			return f(p0), nil
		}}, true

	case "P_S":
		f, a0 := castFn[stP_S](fptr), a[0].P
		return node{class: lStr, S: func(fr unsafe.Pointer, st map[string]any, d any) (string, error) {
			p0, err := a0(fr, st, d)
			if err != nil {
				return "", err
			}
			return f(p0), nil
		}}, true

	case "P_I":
		f, a0 := castFn[stP_I](fptr), a[0].P
		return node{class: lIface, I: func(fr unsafe.Pointer, st map[string]any, d any) (ifacePair, error) {
			p0, err := a0(fr, st, d)
			if err != nil {
				return ifacePair{}, err
			}
			return f(p0), nil
		}}, true

	case "P_E":
		f, a0 := castFn[stP_E](fptr), a[0].P
		return node{class: lNone, E: func(fr unsafe.Pointer, st map[string]any, d any) error {
			p0, err := a0(fr, st, d)
			if err != nil {
				return err
			}
			return asError(f(p0))
		}}, true

	case "PP_PE":
		f, a0, a1 := castFn[stPP_PE](fptr), a[0].P, a[1].P
		return node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			p0, err := a0(fr, st, d)
			if err != nil {
				return nil, err
			}
			p1, err := a1(fr, st, d)
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
		return node{class: lNone, E: func(fr unsafe.Pointer, st map[string]any, d any) error {
			p0, err := a0(fr, st, d)
			if err != nil {
				return err
			}
			p1, err := a1(fr, st, d)
			if err != nil {
				return err
			}
			return asError(f(p0, p1))
		}}, true

	case "II_E":
		f, a0, a1 := castFn[stII_E](fptr), a[0].I, a[1].I
		return node{class: lNone, E: func(fr unsafe.Pointer, st map[string]any, d any) error {
			i0, err := a0(fr, st, d)
			if err != nil {
				return err
			}
			i1, err := a1(fr, st, d)
			if err != nil {
				return err
			}
			return asError(f(i0, i1))
		}}, true

	case "SS_E":
		f, a0, a1 := castFn[stSS_E](fptr), a[0].S, a[1].S
		return node{class: lNone, E: func(fr unsafe.Pointer, st map[string]any, d any) error {
			s0, err := a0(fr, st, d)
			if err != nil {
				return err
			}
			s1, err := a1(fr, st, d)
			if err != nil {
				return err
			}
			return asError(f(s0, s1))
		}}, true
	}
	return node{}, false
}

// jitCompiler holds the state of one program's compilation.
type jitCompiler struct {
	slotOf map[int]int // vmProgram slot -> frame field index
	types  []reflect.Type
	offs   []uintptr
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
	if len(p.inits) > 0 {
		// A var holds a value of any type, including the numeric ones
		// that have no layout class, and the closure tree has no store
		// for it.
		return nil, fmt.Errorf("a var declaration is not in the table")
	}

	stmts, live, writes, splices, err := planInline(p)
	if err != nil {
		return nil, err
	}

	c := &jitCompiler{slotOf: map[int]int{}, writes: writes, splices: splices}
	for slot := 0; slot < p.nslots; slot++ {
		if !live[slot] {
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

	for _, s := range stmts {
		stmt, err := c.stmtNode(s, jp)
		if err != nil {
			return nil, err
		}
		jp.stmts = append(jp.stmts, stmt)
	}
	return jp, nil
}

// plannedStmt is one statement after inlining, with the slot its result
// is stored to.
type plannedStmt struct {
	call *vmCall
	out  int // frame slot, -1 to discard
	ret  bool
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
func planInline(p *vmProgram) ([]plannedStmt, map[int]bool, map[int]int, map[*vmArg]*vmCall, error) {
	stmts := make([]plannedStmt, 0, len(p.stmts))
	for i := range p.stmts {
		s := &p.stmts[i]
		if s.lit.IsValid() {
			return nil, nil, nil, nil, fmt.Errorf("a literal assignment is not in the table")
		}
		if s.call == nil {
			continue // a bare "return;" leaves the program without a value
		}
		if s.ret && i != len(p.stmts)-1 {
			return nil, nil, nil, nil, fmt.Errorf("a return before the last statement is not a straight line")
		}
		out := -1
		if len(s.out) > 0 {
			out = s.out[0]
		}
		stmts = append(stmts, plannedStmt{call: s.call, out: out, ret: s.ret})
	}

	reads := map[int]int{}
	for _, s := range stmts {
		countReads(s.call, reads)
	}

	splices := map[*vmArg]*vmCall{}
	for i := 0; i+1 < len(stmts); i++ {
		s := stmts[i]
		if s.ret || s.out < 0 || s.call.nres != 1 || reads[s.out] != 1 {
			continue
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
	for _, s := range stmts {
		if s.out >= 0 {
			live[s.out] = true
			writes[s.out]++
		}
		if s.ret && s.call.nres > 0 && s.out < 0 {
			return nil, nil, nil, nil, fmt.Errorf("a returned value needs a slot")
		}
	}
	return stmts, live, writes, splices, nil
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
	n, err := c.exprNode(s.call)
	if err != nil {
		return nil, err
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

	switch n.class {
	case lPtr:
		f := n.P
		return func(fr unsafe.Pointer, st map[string]any, d any) error {
			v, err := f(fr, st, d)
			if err != nil {
				return err
			}
			*(*unsafe.Pointer)(unsafe.Add(fr, off)) = v
			return nil
		}, nil
	case lSlice:
		f := n.L
		return func(fr unsafe.Pointer, st map[string]any, d any) error {
			v, err := f(fr, st, d)
			if err != nil {
				return err
			}
			*(*sliceHdr)(unsafe.Add(fr, off)) = v
			return nil
		}, nil
	case lStr:
		f := n.S
		return func(fr unsafe.Pointer, st map[string]any, d any) error {
			v, err := f(fr, st, d)
			if err != nil {
				return err
			}
			*(*string)(unsafe.Add(fr, off)) = v
			return nil
		}, nil
	case lIface:
		f := n.I
		return func(fr unsafe.Pointer, st map[string]any, d any) error {
			v, err := f(fr, st, d)
			if err != nil {
				return err
			}
			*(*ifacePair)(unsafe.Add(fr, off)) = v
			return nil
		}, nil
	}
	return nil, fmt.Errorf("a result of class %s cannot be stored", n.class)
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
	switch n.class {
	case lNone:
		return n.E
	case lPtr:
		f := n.P
		return func(fr unsafe.Pointer, st map[string]any, d any) error { _, err := f(fr, st, d); return err }
	case lSlice:
		f := n.L
		return func(fr unsafe.Pointer, st map[string]any, d any) error { _, err := f(fr, st, d); return err }
	case lStr:
		f := n.S
		return func(fr unsafe.Pointer, st map[string]any, d any) error { _, err := f(fr, st, d); return err }
	case lIface:
		f := n.I
		return func(fr unsafe.Pointer, st map[string]any, d any) error { _, err := f(fr, st, d); return err }
	}
	return nil
}

// exprNode compiles one call and its arguments.
func (c *jitCompiler) exprNode(call *vmCall) (node, error) {
	ft := call.fn.Type()
	args := make([]node, ft.NumIn())
	key := ""
	for i := 0; i < ft.NumIn(); i++ {
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
		args[i] = a
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
				return node{class: lIface, I: func(fr unsafe.Pointer, _ map[string]any, _ any) (ifacePair, error) {
					return ifacePair{tab: tab, data: *(*unsafe.Pointer)(unsafe.Add(fr, off))}, nil
				}}, nil
			}
			// Stored indirectly: the interface points at the slot rather
			// than a copy, which is the allocation this saves. Only
			// legal while the slot is written once, since the frame
			// outlives the call and the callee may keep the interface.
			if c.writes[a.slot] != 1 {
				return node{}, fmt.Errorf("a name assigned more than once cannot fill a %s parameter", pt)
			}
			return node{class: lIface, I: func(fr unsafe.Pointer, _ map[string]any, _ any) (ifacePair, error) {
				return ifacePair{tab: tab, data: unsafe.Add(fr, off)}, nil
			}}, nil
		}
		if layoutOf(st) != cl {
			return node{}, fmt.Errorf("a %s name cannot fill a %s parameter", layoutOf(st), cl)
		}
		return slotNode(cl, off), nil

	case vaConst:
		return constNode(cl, a.val)

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

	load := func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
		p, err := sp(fr, st, d)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, fmt.Errorf("exec: field %s read on a nil %s", name, srcType)
		}
		return unsafe.Add(p, off), nil
	}

	var out node
	switch fcl {
	case lPtr:
		out = node{class: lPtr, P: func(fr unsafe.Pointer, st map[string]any, d any) (unsafe.Pointer, error) {
			at, err := load(fr, st, d)
			if err != nil {
				return nil, err
			}
			return *(*unsafe.Pointer)(at), nil
		}}
	case lStr:
		out = node{class: lStr, S: func(fr unsafe.Pointer, st map[string]any, d any) (string, error) {
			at, err := load(fr, st, d)
			if err != nil {
				return "", err
			}
			return *(*string)(at), nil
		}}
	case lSlice:
		out = node{class: lSlice, L: func(fr unsafe.Pointer, st map[string]any, d any) (sliceHdr, error) {
			at, err := load(fr, st, d)
			if err != nil {
				return sliceHdr{}, err
			}
			return *(*sliceHdr)(at), nil
		}}
	case lIface:
		out = node{class: lIface, I: func(fr unsafe.Pointer, st map[string]any, d any) (ifacePair, error) {
			at, err := load(fr, st, d)
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
	switch sub.class {
	case lPtr:
		f := sub.P
		return node{class: lIface, I: func(fr unsafe.Pointer, s map[string]any, d any) (ifacePair, error) {
			v, err := f(fr, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			return ifacePair{tab: tab, data: v}, nil
		}}, nil
	case lSlice:
		f := sub.L
		return node{class: lIface, I: func(fr unsafe.Pointer, s map[string]any, d any) (ifacePair, error) {
			v, err := f(fr, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			return ifacePair{tab: tab, data: unsafe.Pointer(&v)}, nil
		}}, nil
	case lStr:
		f := sub.S
		return node{class: lIface, I: func(fr unsafe.Pointer, s map[string]any, d any) (ifacePair, error) {
			v, err := f(fr, s, d)
			if err != nil {
				return ifacePair{}, err
			}
			return ifacePair{tab: tab, data: unsafe.Pointer(&v)}, nil
		}}, nil
	}
	return node{}, fmt.Errorf("a %s result cannot become an interface", sub.class)
}

func slotNode(cl layout, off uintptr) node {
	switch cl {
	case lPtr:
		return node{class: lPtr, P: func(fr unsafe.Pointer, _ map[string]any, _ any) (unsafe.Pointer, error) {
			return *(*unsafe.Pointer)(unsafe.Add(fr, off)), nil
		}}
	case lSlice:
		return node{class: lSlice, L: func(fr unsafe.Pointer, _ map[string]any, _ any) (sliceHdr, error) {
			return *(*sliceHdr)(unsafe.Add(fr, off)), nil
		}}
	case lStr:
		return node{class: lStr, S: func(fr unsafe.Pointer, _ map[string]any, _ any) (string, error) {
			return *(*string)(unsafe.Add(fr, off)), nil
		}}
	default:
		return node{class: lIface, I: func(fr unsafe.Pointer, _ map[string]any, _ any) (ifacePair, error) {
			return *(*ifacePair)(unsafe.Add(fr, off)), nil
		}}
	}
}

func constNode(cl layout, v reflect.Value) (node, error) {
	switch cl {
	case lStr:
		s := v.String()
		return node{class: lStr, S: func(unsafe.Pointer, map[string]any, any) (string, error) { return s, nil }}, nil
	case lPtr:
		if !v.IsZero() {
			return node{}, fmt.Errorf("only a zero pointer can be a constant")
		}
		return node{class: lPtr, P: func(unsafe.Pointer, map[string]any, any) (unsafe.Pointer, error) { return nil, nil }}, nil
	case lIface:
		if !v.IsZero() {
			return node{}, fmt.Errorf("only a nil interface can be a constant")
		}
		return node{class: lIface, I: func(unsafe.Pointer, map[string]any, any) (ifacePair, error) { return ifacePair{}, nil }}, nil
	case lSlice:
		if !v.IsZero() {
			return node{}, fmt.Errorf("only a nil slice can be a constant")
		}
		return node{class: lSlice, L: func(unsafe.Pointer, map[string]any, any) (sliceHdr, error) { return sliceHdr{}, nil }}, nil
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
		return node{class: lStr, S: func(_ unsafe.Pointer, st map[string]any, _ any) (string, error) {
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
			return node{class: lIface, I: func(_ unsafe.Pointer, _ map[string]any, d any) (ifacePair, error) {
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
		return node{class: lIface, I: func(_ unsafe.Pointer, st map[string]any, _ any) (ifacePair, error) {
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
	return node{}, fmt.Errorf("a stack value cannot fill a %s parameter", cl)
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
