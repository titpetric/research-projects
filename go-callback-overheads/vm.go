package callbacks

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"unsafe"
)

// The multi-statement VM. A program is a list of calls whose results
// are bound to names; there are no operators, so every value is a
// literal, a name, or the result of another call.
//
// Two rules shape the compiled form:
//
// A trailing error result is never a value. It is stripped from the
// result list at compile time and checked after every call, so
// "req := http.NewRequest(...)" binds one name to a two-result call and
// a failure returns from Exec or Scan on the spot. No program text
// mentions an error.
//
// Every argument is optional. A parameter the statement does not supply
// is filled with the zero value of its type, which is how
// http.NewRequest is called with two arguments and a nil body.
//
// Names bound by the program carry a static type, so methods on them
// are resolved against that type when the statement compiles.
// json.NewEncoder is bound; Encode is not, and is reached through the
// *json.Encoder the binding returns. Names coming from the caller's
// stack, including dest, are opaque and are checked when they are read.

type vmArgKind int

const (
	vaConst vmArgKind = iota // a literal, or an omitted argument's zero value
	vaSlot                   // a name bound earlier in the program
	vaStack                  // a name read from the caller's stack map
	vaDest                   // the pointer Scan was handed
	vaCall                   // a nested call
	vaField                  // a struct field read off another value
)

// vmArg is one argument of a compiled call.
//
// assign is an inline cache for the assignability check on vaStack and
// vaDest, the only two kinds whose type is unknown until the value is
// read. reflect.Type.AssignableTo against an interface parameter walks
// the method tables comparing names, and it dominated the profile at
// 29% of samples; the same program is almost always handed the same
// concrete type, so one entry removes it. It is an atomic.Pointer
// rather than a plain field because a compiled program is shared
// between concurrent runs.
type vmArg struct {
	kind   vmArgKind
	val    reflect.Value // vaConst
	slot   int           // vaSlot
	name   string        // vaStack
	typ    reflect.Type  // the parameter type this argument fills
	sub    *vmCall       // vaCall
	assign atomic.Pointer[assignCache]

	// iface >= 0 when this argument fills a non-empty interface
	// parameter from a dynamic value and the interface type is in
	// ifaceConvs. The value is converted with a type assertion, which
	// is how the itab is obtained, and handed to reflect already typed
	// as the parameter. reflect.Value.call runs its own assignTo on
	// every argument, and for an interface parameter that means
	// reflect.implements walking method tables by name on every call;
	// arriving with identical types skips it.
	iface int
	conv  ifaceConv

	// vaField: src is the value the field is read from, index the
	// reflect field path, and deref says src is a pointer to the struct
	// rather than the struct.
	src   *vmArg
	index []int
	deref bool
}

// assignCache is one remembered answer to "is this concrete type
// assignable to the parameter type".
type assignCache struct {
	typ reflect.Type
	ok  bool
}

// assignable answers for rt, consulting and filling the inline cache.
func (a *vmArg) assignable(rt reflect.Type) bool {
	if c := a.assign.Load(); c != nil && c.typ == rt {
		return c.ok
	}
	ok := rt.AssignableTo(a.typ)
	a.assign.Store(&assignCache{typ: rt, ok: ok})
	return ok
}

// vmCall is one compiled call. A method call is the same structure with
// the receiver as args[0], because reflect.Method.Func takes the
// receiver as its first parameter.
type vmCall struct {
	fn     reflect.Value
	name   string // for diagnostics
	args   []*vmArg
	errIdx int // index of the trailing error result, -1 when there is none
	nres   int // results excluding that error
	off    int // this call's window into the per-run frame
}

// vmStmt is one statement: a call, the slots its results bind to, and
// whether it ends the program.
type vmStmt struct {
	call *vmCall
	out  []int // one slot per bound result
	ret  bool

	// lit is the value of a literal assignment, "x = 123", already
	// converted to the slot's type.
	lit reflect.Value
}

// slotInit is the zero value a var statement puts in scope before the
// program runs.
type slotInit struct {
	slot int
	zero reflect.Value
}

// vmProgram is a compiled program. It holds no per-call state: the
// slots are allocated per execution, so a compiled program is safe for
// concurrent use.
type vmProgram struct {
	stmts   []vmStmt
	nslots  int
	frame   int // total argument words across every call in the program
	nifaces int // interface arguments pre-converted per run

	// slotTypes is the static type of each named slot, which the step
	// JIT needs to lay out its frame. polymorphic records a slot
	// reassigned at a different type, which no single frame field can
	// hold.
	slotTypes   []reflect.Type
	polymorphic bool

	// inits are the var declarations, applied before the first
	// statement so a name reads as its type's zero value even when
	// nothing assigned it.
	inits []slotInit
}

// compileProgram builds a vmProgram. Slots and their static types are
// tracked as the statements are walked, so a name must be bound before
// it is used and a method must exist on the type of the name it is
// called on.
func (c *Compiler) compileProgram(prog *program) (*vmProgram, error) {
	p := &vmProgram{}
	slots := map[string]int{}
	env := map[string]reflect.Type{}

	// A name declared with var fixes its type before anything else is
	// compiled, so a literal assigned to it converts to that type.
	declared := map[string]reflect.Type{}
	for si := range prog.stmts {
		if s := prog.stmts[si]; s.varType != "" {
			t, ok := c.lookupType(s.varType)
			if !ok {
				return nil, fmt.Errorf("compile: unknown type %q, register it with BindType", s.varType)
			}
			declared[s.varName] = t
		}
	}

	newSlot := func(name string, t reflect.Type) int {
		slot, ok := slots[name]
		if !ok {
			slot = p.nslots
			p.nslots++
			p.slotTypes = append(p.slotTypes, nil)
			slots[name] = slot
		}
		if prev := p.slotTypes[slot]; prev != nil && prev != t {
			p.polymorphic = true
		}
		p.slotTypes[slot] = t
		env[name] = t
		return slot
	}

	for si := range prog.stmts {
		s := prog.stmts[si]

		if s.varType != "" {
			t := declared[s.varName]
			slot := newSlot(s.varName, t)
			p.inits = append(p.inits, slotInit{slot: slot, zero: reflect.Zero(t)})
			continue
		}

		if s.lit != nil {
			if len(s.lhs) != 1 {
				return nil, fmt.Errorf("compile: a literal assigns to exactly one name")
			}
			name := s.lhs[0]
			if name == "dest" {
				return nil, fmt.Errorf("compile: dest is supplied by Scan and cannot be assigned")
			}
			t, ok := env[name]
			if !ok {
				t = c.inferLiteralType(prog, name, *s.lit)
			}
			v, err := literalValue(*s.lit, t)
			if err != nil {
				return nil, fmt.Errorf("compile: %s: %w", name, err)
			}
			slot := newSlot(name, t)
			p.stmts = append(p.stmts, vmStmt{lit: v, out: []int{slot}})
			continue
		}

		if s.call == nil {
			// A bare "return;".
			p.stmts = append(p.stmts, vmStmt{ret: true})
			continue
		}
		call, _, err := c.compileExpr(slots, env, s.call)
		if err != nil {
			return nil, err
		}
		if len(s.lhs) > call.nres {
			return nil, fmt.Errorf("compile: %s returns %d values, cannot assign %d", call.name, call.nres, len(s.lhs))
		}

		out := make([]int, 0, len(s.lhs))
		for i, name := range s.lhs {
			if name == "dest" {
				return nil, fmt.Errorf("compile: dest is supplied by Scan and cannot be assigned")
			}
			if _, ok := slots[name]; !ok && !s.define {
				return nil, fmt.Errorf("compile: %s is not defined, use := or var", name)
			}
			out = append(out, newSlot(name, c.resultType(call, i)))
		}
		p.stmts = append(p.stmts, vmStmt{call: call, out: out, ret: s.ret})
	}

	for i := range p.stmts {
		if p.stmts[i].call != nil {
			p.assignFrame(p.stmts[i].call)
		}
	}
	return p, nil
}

// assignFrame gives every call in the program a disjoint window into
// the per-run argument frame, so one allocation covers them all and a
// nested call cannot overwrite the arguments its parent is still
// filling.
func (p *vmProgram) assignFrame(c *vmCall) {
	c.off = p.frame
	p.frame += len(c.args)
	for _, a := range c.args {
		for a.kind == vaField {
			a = a.src
		}
		switch a.kind {
		case vaCall:
			p.assignFrame(a.sub)
		case vaStack, vaDest:
			// Only a non-empty interface is worth pre-converting: for
			// an empty one reflect packs an eface directly and never
			// reaches implements.
			if a.typ.Kind() == reflect.Interface && a.typ.NumMethod() > 0 {
				if conv, ok := ifaceConvs[a.typ]; ok {
					a.conv = conv
					a.iface = p.nifaces
					p.nifaces++
				}
			}
		}
	}
}

// literalValue converts a parsed literal to type t.
func literalValue(a arg, t reflect.Type) (reflect.Value, error) {
	if a.kind == argString {
		v := reflect.ValueOf(a.str)
		if !v.Type().AssignableTo(t) {
			if t.Kind() == reflect.Interface && t.NumMethod() == 0 {
				return v, nil
			}
			return reflect.Value{}, fmt.Errorf("cannot use a string as %s", t)
		}
		return v, nil
	}
	return literalAs(a, t)
}

// inferLiteralType picks the type of a name a literal is assigned to,
// when no var statement fixed it. The first place the program passes
// the name to a binding decides, because that is the only type the
// value has to satisfy. Failing that the literal keeps the width the
// parser gave it.
func (c *Compiler) inferLiteralType(prog *program, name string, lit arg) reflect.Type {
	for si := range prog.stmts {
		if call := prog.stmts[si].call; call != nil {
			if t := c.useType(call, name); t != nil {
				return t
			}
		}
	}
	if lit.kind == argFloat {
		return reflect.TypeFor[float64]()
	}
	if lit.kind == argString {
		return reflect.TypeFor[string]()
	}
	return reflect.TypeFor[int64]()
}

// useType is the parameter type a call gives to name, looking through
// nested calls. Only a call whose whole path is a binding is
// considered: a method's receiver type may itself depend on a type not
// worked out yet.
func (c *Compiler) useType(e *callExpr, name string) reflect.Type {
	if b, ok := c.bindings[joinPath(e.path)]; ok && len(e.chain) == 0 {
		ft := b.rv.Type()
		for i, a := range e.args {
			if i >= ft.NumIn() {
				break
			}
			if a.kind == argVar && a.str == name {
				return ft.In(i)
			}
		}
	}
	for _, a := range e.args {
		if a.kind == argCall {
			if t := c.useType(a.sub, name); t != nil {
				return t
			}
		}
	}
	for _, l := range e.chain {
		for _, a := range l.args {
			if a.kind == argCall {
				if t := c.useType(a.sub, name); t != nil {
					return t
				}
			}
		}
	}
	return nil
}

// literalAs converts a numeric literal to the type of the parameter it
// fills. The parser only produces int64 and float64, so without this a
// binding taking int, int32 or float32 could not be given a literal at
// all. The value must be representable: a literal that does not fit is
// a compile error, not a wrap.
//
// An empty interface parameter takes the literal at its written width,
// int64 or float64, which is what the parser produced.
func literalAs(a arg, pt reflect.Type) (reflect.Value, error) {
	if pt.Kind() == reflect.Interface {
		if pt.NumMethod() != 0 {
			return reflect.Value{}, fmt.Errorf("cannot use a number as %s", pt)
		}
		if a.kind == argInt {
			return reflect.ValueOf(a.i), nil
		}
		return reflect.ValueOf(a.f), nil
	}
	v := reflect.New(pt).Elem()
	switch pt.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if a.kind == argFloat {
			return reflect.Value{}, fmt.Errorf("cannot use %v as %s, it has a decimal point", a.f, pt)
		}
		if v.OverflowInt(a.i) {
			return reflect.Value{}, fmt.Errorf("%d overflows %s", a.i, pt)
		}
		v.SetInt(a.i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if a.kind == argFloat {
			return reflect.Value{}, fmt.Errorf("cannot use %v as %s, it has a decimal point", a.f, pt)
		}
		if a.i < 0 {
			return reflect.Value{}, fmt.Errorf("cannot use %d as %s, it is negative", a.i, pt)
		}
		if v.OverflowUint(uint64(a.i)) {
			return reflect.Value{}, fmt.Errorf("%d overflows %s", a.i, pt)
		}
		v.SetUint(uint64(a.i))
	case reflect.Float32, reflect.Float64:
		f := a.f
		if a.kind == argInt {
			f = float64(a.i)
		}
		if v.OverflowFloat(f) {
			return reflect.Value{}, fmt.Errorf("%v overflows %s", f, pt)
		}
		v.SetFloat(f)
	default:
		return reflect.Value{}, fmt.Errorf("cannot use a number as %s", pt)
	}
	return v, nil
}

// fieldOf resolves name as an exported struct field on t,
// dereferencing a pointer to a struct. deref reports whether the value
// has to be dereferenced before the field is read.
func fieldOf(t reflect.Type, name string) (f reflect.StructField, deref bool, ok bool) {
	st := t
	if st.Kind() == reflect.Pointer {
		st, deref = st.Elem(), true
	}
	if st.Kind() != reflect.Struct {
		return reflect.StructField{}, false, false
	}
	f, ok = st.FieldByName(name)
	if !ok || f.PkgPath != "" {
		return reflect.StructField{}, false, false
	}
	return f, deref, true
}

// resultType is the static type of the i'th non-error result.
func (c *Compiler) resultType(call *vmCall, i int) reflect.Type {
	ft := call.fn.Type()
	n := 0
	for j := 0; j < ft.NumOut(); j++ {
		if j == call.errIdx {
			continue
		}
		if n == i {
			return ft.Out(j)
		}
		n++
	}
	return nil
}

// compileExpr compiles a call and the methods chained onto it,
// returning the outermost call and the static type of its first
// non-error result.
func (c *Compiler) compileExpr(slots map[string]int, env map[string]reflect.Type, e *callExpr) (*vmCall, reflect.Type, error) {
	var (
		curr     *vmCall
		currType reflect.Type
		methods  []string
	)

	// The longest prefix of the path that names a binding wins, so
	// http.NewRequest is one name while req.Cookies is a method on req.
	base := -1
	for i := len(e.path); i >= 1; i-- {
		if _, ok := c.bindings[joinPath(e.path[:i])]; ok {
			base = i
			break
		}
	}

	var recv *vmArg
	switch {
	case base >= 0:
		methods = e.path[base:]
	default:
		slot, ok := slots[e.path[0]]
		if !ok {
			return nil, nil, fmt.Errorf("compile: unknown binding %q", joinPath(e.path))
		}
		recv = &vmArg{kind: vaSlot, slot: slot, typ: env[e.path[0]], iface: -1}
		currType = env[e.path[0]]
		methods = e.path[1:]
		if len(methods) == 0 && len(e.chain) == 0 {
			return nil, nil, fmt.Errorf("compile: %s is a value, not a call", e.path[0])
		}
	}

	// Arguments written in the source belong to the last name in the
	// path. Everything before it is called with no arguments, which is
	// legal because every argument is optional.
	links := make([]link, 0, len(methods)+len(e.chain))
	for i, m := range methods {
		l := link{name: m}
		if i == len(methods)-1 {
			l.args = e.args
		}
		links = append(links, l)
	}
	links = append(links, e.chain...)

	if base >= 0 {
		name := joinPath(e.path[:base])
		var args []arg
		if len(methods) == 0 {
			args = e.args
		}
		call, err := c.compileCall(slots, env, c.bindings[name].rv, name, nil, args)
		if err != nil {
			return nil, nil, err
		}
		curr, currType = call, c.resultType(call, 0)
	}

	for _, l := range links {
		if curr != nil {
			recv = &vmArg{kind: vaCall, sub: curr, typ: currType}
		}
		if currType == nil {
			return nil, nil, fmt.Errorf("compile: cannot call %s on a value of unknown type", l.name)
		}
		if m, ok := currType.MethodByName(l.name); ok {
			call, err := c.compileCall(slots, env, m.Func, currType.String()+"."+l.name, recv, l.args)
			if err != nil {
				return nil, nil, err
			}
			curr, currType = call, c.resultType(call, 0)
			continue
		}
		f, deref, ok := fieldOf(currType, l.name)
		if !ok {
			return nil, nil, fmt.Errorf("compile: %s has no method or field %s", currType, l.name)
		}
		if len(l.args) > 0 {
			return nil, nil, fmt.Errorf("compile: %s.%s is a field, not a method", currType, l.name)
		}
		// The field becomes the receiver of whatever comes next.
		// curr goes back to nil so the next link does not overwrite it
		// with a call result.
		recv = &vmArg{kind: vaField, src: recv, index: f.Index, deref: deref, typ: f.Type, iface: -1}
		curr, currType = nil, f.Type
	}

	if curr == nil {
		return nil, nil, fmt.Errorf("compile: %q is not callable", joinPath(e.path))
	}
	return curr, currType, nil
}

// compileCall validates one call against a func value. recv is the
// receiver for a method call and fills parameter 0; src holds the
// arguments written in the source, which fill the parameters after it.
func (c *Compiler) compileCall(slots map[string]int, env map[string]reflect.Type, fn reflect.Value, name string, recv *vmArg, src []arg) (*vmCall, error) {
	ft := fn.Type()
	if ft.IsVariadic() {
		return nil, fmt.Errorf("compile: variadic binding %q not supported", name)
	}

	off := 0
	if recv != nil {
		off = 1
	}
	if len(src)+off > ft.NumIn() {
		return nil, fmt.Errorf("compile: %s takes %d arguments, got %d", name, ft.NumIn()-off, len(src))
	}

	call := &vmCall{fn: fn, name: name, errIdx: -1}
	call.args = make([]*vmArg, ft.NumIn())
	if recv != nil {
		if !recv.typ.AssignableTo(ft.In(0)) {
			return nil, fmt.Errorf("compile: %s: receiver is %s, want %s", name, recv.typ, ft.In(0))
		}
		call.args[0] = recv
	}

	for i := off; i < ft.NumIn(); i++ {
		pt := ft.In(i)
		// A parameter the source does not supply is its zero value:
		// "" for string, nil for io.Reader.
		if i-off >= len(src) {
			call.args[i] = &vmArg{kind: vaConst, val: reflect.Zero(pt), typ: pt, iface: -1}
			continue
		}
		a, err := c.compileArg(slots, env, name, i-off, pt, src[i-off])
		if err != nil {
			return nil, err
		}
		call.args[i] = a
	}

	for i := 0; i < ft.NumOut(); i++ {
		if ft.Out(i) == errType {
			call.errIdx = i
		}
	}
	call.nres = ft.NumOut()
	if call.errIdx >= 0 {
		call.nres--
	}
	return call, nil
}

func (c *Compiler) compileArg(slots map[string]int, env map[string]reflect.Type, name string, pos int, pt reflect.Type, a arg) (*vmArg, error) {
	var v reflect.Value
	switch a.kind {
	case argString:
		v = reflect.ValueOf(a.str)
	case argInt, argFloat:
		lit, err := literalAs(a, pt)
		if err != nil {
			return nil, fmt.Errorf("compile: %s argument %d: %w", name, pos+1, err)
		}
		v = lit
	case argCall:
		sub, st, err := c.compileExpr(slots, env, a.sub)
		if err != nil {
			return nil, err
		}
		if sub.nres == 0 {
			return nil, fmt.Errorf("compile: %s argument %d: %s returns no value", name, pos+1, sub.name)
		}
		if !st.AssignableTo(pt) {
			return nil, fmt.Errorf("compile: %s argument %d: cannot use %s as %s", name, pos+1, st, pt)
		}
		return &vmArg{kind: vaCall, sub: sub, typ: pt, iface: -1}, nil
	case argPath:
		slot, ok := slots[a.path[0]]
		if !ok {
			return nil, fmt.Errorf("compile: %s argument %d: %s is not a name bound by the program, so its fields are unknown", name, pos+1, a.path[0])
		}
		cur := &vmArg{kind: vaSlot, slot: slot, name: a.path[0], typ: env[a.path[0]], iface: -1}
		curType := env[a.path[0]]
		for _, seg := range a.path[1:] {
			f, deref, ok := fieldOf(curType, seg)
			if !ok {
				return nil, fmt.Errorf("compile: %s argument %d: %s has no field %s", name, pos+1, curType, seg)
			}
			cur = &vmArg{kind: vaField, src: cur, index: f.Index, deref: deref, typ: f.Type, iface: -1}
			curType = f.Type
		}
		if !curType.AssignableTo(pt) {
			return nil, fmt.Errorf("compile: %s argument %d: cannot use %s as %s", name, pos+1, curType, pt)
		}
		cur.typ = pt
		return cur, nil

	case argVar:
		if a.str == "dest" {
			return &vmArg{kind: vaDest, name: "dest", typ: pt, iface: -1}, nil
		}
		if slot, ok := slots[a.str]; ok {
			st := env[a.str]
			if st != nil && !st.AssignableTo(pt) {
				return nil, fmt.Errorf("compile: %s argument %d: cannot use %s as %s", name, pos+1, st, pt)
			}
			return &vmArg{kind: vaSlot, slot: slot, name: a.str, typ: pt, iface: -1}, nil
		}
		// Not bound by the program, so it comes off the caller's stack
		// and its type is only known when it is read.
		return &vmArg{kind: vaStack, name: a.str, typ: pt, iface: -1}, nil
	}
	if !v.IsValid() {
		return nil, fmt.Errorf("compile: %s argument %d: unsupported literal", name, pos+1)
	}
	if !v.Type().AssignableTo(pt) {
		return nil, fmt.Errorf("compile: %s argument %d: cannot use %s as %s", name, pos+1, v.Type(), pt)
	}
	return &vmArg{kind: vaConst, val: v, typ: pt, iface: -1}, nil
}

// run executes the program. Slots are allocated per execution, so
// concurrent runs of the same compiled program do not share state.
func (p *vmProgram) run(stack map[string]any, dest any) (any, error) {
	// One allocation per run for both the named slots and every call's
	// argument window. Each call owns a disjoint range, so a nested
	// call never overwrites the arguments its parent is still filling.
	mem := make([]reflect.Value, p.nslots+p.frame)
	slots, frame := mem[:p.nslots], mem[p.nslots:]
	var ifaces []ifacePair
	if p.nifaces > 0 {
		ifaces = make([]ifacePair, p.nifaces)
	}
	for _, in := range p.inits {
		slots[in.slot] = in.zero
	}
	for i := range p.stmts {
		s := &p.stmts[i]
		if s.lit.IsValid() {
			slots[s.out[0]] = s.lit
			continue
		}
		if s.call == nil {
			return nil, nil
		}
		out, err := s.call.invoke(slots, frame, ifaces, stack, dest)
		if err != nil {
			return nil, err
		}
		n := 0
		for j := range out {
			if j == s.call.errIdx {
				continue
			}
			if n < len(s.out) {
				slots[s.out[n]] = out[j]
			}
			n++
		}
		if s.ret {
			if s.call.nres == 0 {
				return nil, nil
			}
			return firstNonErr(out, s.call.errIdx).Interface(), nil
		}
	}
	return nil, nil
}

func firstNonErr(out []reflect.Value, errIdx int) reflect.Value {
	for i := range out {
		if i != errIdx {
			return out[i]
		}
	}
	return reflect.Value{}
}

// invoke evaluates the arguments and calls the func, returning the raw
// result list. A non-nil trailing error stops the program.
func (c *vmCall) invoke(slots, frame []reflect.Value, ifaces []ifacePair, stack map[string]any, dest any) ([]reflect.Value, error) {
	args := frame[c.off : c.off+len(c.args)]
	for i, a := range c.args {
		v, err := a.get(slots, frame, ifaces, stack, dest)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	out := c.fn.Call(args)
	if c.errIdx >= 0 {
		if e := out[c.errIdx]; !e.IsNil() {
			return nil, e.Interface().(error)
		}
	}
	return out, nil
}

// get resolves one argument for this call.
func (a *vmArg) get(slots, frame []reflect.Value, ifaces []ifacePair, stack map[string]any, dest any) (reflect.Value, error) {
	switch a.kind {
	case vaConst:
		return a.val, nil
	case vaSlot:
		v := slots[a.slot]
		if !v.IsValid() {
			return reflect.Zero(a.typ), nil
		}
		return v, nil
	case vaCall:
		out, err := a.sub.invoke(slots, frame, ifaces, stack, dest)
		if err != nil {
			return reflect.Value{}, err
		}
		return firstNonErr(out, a.sub.errIdx), nil
	case vaField:
		v, err := a.src.get(slots, frame, ifaces, stack, dest)
		if err != nil {
			return reflect.Value{}, err
		}
		if a.deref {
			if v.IsNil() {
				return reflect.Value{}, fmt.Errorf("exec: field read on a nil %s", v.Type())
			}
			v = v.Elem()
		}
		return v.FieldByIndex(a.index), nil
	case vaDest:
		if dest == nil {
			return reflect.Zero(a.typ), fmt.Errorf("exec: dest is only set by Scan")
		}
		return a.dynamic(dest, ifaces, "dest")
	case vaStack:
		v, ok := stack[a.name]
		if !ok || v == nil {
			return reflect.Zero(a.typ), nil
		}
		return a.dynamic(v, ifaces, a.name)
	}
	return reflect.Value{}, fmt.Errorf("exec: argument kind %d cannot be resolved", a.kind)
}

// dynamic resolves a value whose type is only known now: from the
// caller's stack, or from the pointer Scan was handed.
func (a *vmArg) dynamic(v any, ifaces []ifacePair, name string) (reflect.Value, error) {
	if a.iface >= 0 {
		// The assertion inside conv is both the check and the itab
		// lookup. Writing the result into the frame gives reflect a
		// value already typed as the parameter.
		p, ok := a.conv(v)
		if !ok {
			return reflect.Value{}, fmt.Errorf("exec: variable %q: cannot use %T as %s", name, v, a.typ)
		}
		ifaces[a.iface] = p
		return reflect.NewAt(a.typ, unsafe.Pointer(&ifaces[a.iface])).Elem(), nil
	}
	rv := reflect.ValueOf(v)
	if !a.assignable(rv.Type()) {
		return reflect.Value{}, fmt.Errorf("exec: variable %q: cannot use %s as %s", name, rv.Type(), a.typ)
	}
	return rv, nil
}
