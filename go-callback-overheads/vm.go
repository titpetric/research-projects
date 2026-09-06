package callbacks

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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
	vaCtx                    // the execution context, auto-filled
)

var ctxType = reflect.TypeFor[context.Context]()

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
	// spread marks a variadic call whose last argument is the slice
	// itself, f(xs...): the invocation goes through CallSlice.
	spread bool
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

	// retArg is a return statement's value when it is not a call:
	// a name, a field read, or a literal.
	retArg *vmArg

	// fieldSet writes a value through a field: req.Method = "POST".
	fieldSet *vmFieldSet
}

// fieldStep is one selector of a field-assignment target.
type fieldStep struct {
	index []int
	deref bool
}

// vmFieldSet is a compiled field assignment: the slot the base name
// lives in, the selectors to the field, and the value.
type vmFieldSet struct {
	base  int // slot of the base name
	steps []fieldStep
	val   *vmArg
	field string // for diagnostics
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

	// A name that collides with a binding can never be read back:
	// path resolution prefers the binding, so url := ... with url.Parse
	// bound compiles and then silently resolves the other way. Shadowing
	// is rejected instead.
	reserved := map[string]bool{"dest": true, "true": true, "false": true, "nil": true, "var": true, "return": true}
	for name := range c.bindings {
		if i := strings.IndexByte(name, '.'); i > 0 {
			reserved[name[:i]] = true
		} else {
			reserved[name] = true
		}
	}
	checkName := func(name string) error {
		if reserved[name] {
			return fmt.Errorf("compile: %s shadows a binding or keyword and cannot be assigned", name)
		}
		return nil
	}

	// A name declared with var fixes its type before anything else is
	// compiled, so a literal assigned to it converts to that type.
	declared := map[string]reflect.Type{}
	for si := range prog.stmts {
		if s := prog.stmts[si]; s.varType != "" {
			if err := checkName(s.varName); err != nil {
				return nil, err
			}
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

		if s.fieldLhs != nil {
			fs, err := c.compileFieldSet(slots, env, s)
			if err != nil {
				return nil, err
			}
			p.stmts = append(p.stmts, vmStmt{fieldSet: fs})
			continue
		}
		if s.lit != nil {
			if len(s.lhs) != 1 {
				return nil, fmt.Errorf("compile: a literal assigns to exactly one name")
			}
			name := s.lhs[0]
			if err := checkName(name); err != nil {
				return nil, err
			}
			t, ok := env[name]
			if !ok {
				t = c.inferLiteralType(prog, name, *s.lit)
				if t == nil {
					return nil, fmt.Errorf("compile: %s = nil needs a var declaration or a use to take a type from", name)
				}
			}
			v, err := literalValue(*s.lit, t)
			if err != nil {
				return nil, fmt.Errorf("compile: %s: %w", name, err)
			}
			slot := newSlot(name, t)
			p.stmts = append(p.stmts, vmStmt{lit: v, out: []int{slot}})
			continue
		}

		if s.retVal != nil {
			ra, err := c.compileRetVal(slots, env, *s.retVal)
			if err != nil {
				return nil, err
			}
			p.stmts = append(p.stmts, vmStmt{ret: true, retArg: ra})
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
			if err := checkName(name); err != nil {
				return nil, err
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

// nilAs is the value the nil literal takes for a parameter type:
// the zero value when the type can hold nil, an error when it cannot.
func nilAs(pt reflect.Type) (reflect.Value, error) {
	switch pt.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Slice,
		reflect.Map, reflect.Chan, reflect.Func:
		return reflect.Zero(pt), nil
	}
	return reflect.Value{}, fmt.Errorf("cannot use nil as %s", pt)
}

// literalValue converts a parsed literal to type t.
func literalValue(a arg, t reflect.Type) (reflect.Value, error) {
	if a.kind == argBool {
		v := reflect.ValueOf(a.b)
		if t.Kind() == reflect.Interface && t.NumMethod() == 0 {
			return v, nil
		}
		if !v.Type().AssignableTo(t) {
			return reflect.Value{}, fmt.Errorf("cannot use %v as %s", a.b, t)
		}
		return v, nil
	}
	if a.kind == argNil {
		return nilAs(t)
	}
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
	switch lit.kind {
	case argFloat:
		return reflect.TypeFor[float64]()
	case argString:
		return reflect.TypeFor[string]()
	case argBool:
		return reflect.TypeFor[bool]()
	case argNil:
		// nil alone names no type; the any it would infer to is never
		// what the program meant, so the caller reports it.
		return nil
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
		fixed := ft.NumIn()
		if ft.IsVariadic() {
			fixed--
		}
		i := 0
		for _, a := range e.args {
			// Mirror compileCall: a context parameter consumes no
			// written argument.
			for i < fixed && ft.In(i) == ctxType {
				i++
			}
			var pt reflect.Type
			switch {
			case i < fixed:
				pt = ft.In(i)
			case ft.IsVariadic():
				pt = ft.In(fixed).Elem()
			default:
				return nil
			}
			if a.kind == argVar && a.str == name {
				// An empty interface accepts anything, so it says
				// nothing about what the name should be.
				if pt.Kind() == reflect.Interface && pt.NumMethod() == 0 {
					return nil
				}
				return pt
			}
			i++
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// unspread strips the spread flag for compiling the slice argument
// itself.
func unspread(a arg) arg {
	a.spread = false
	return a
}

// staticType is the compile-time type of an argument when it has one:
// a program-bound name. Everything else answers nil, which for the
// context rule means auto-fill.
func (c *Compiler) staticType(env map[string]reflect.Type, a arg) reflect.Type {
	if a.kind == argVar {
		return env[a.str]
	}
	return nil
}

// compileFieldSet compiles req.Method = value. The base is a
// program-bound name, every selector is an exported field, and the
// value is a literal or a call whose result is assignable to the field.
func (c *Compiler) compileFieldSet(slots map[string]int, env map[string]reflect.Type, s stmt) (*vmFieldSet, error) {
	base := s.fieldLhs[0]
	slot, ok := slots[base]
	if !ok {
		return nil, fmt.Errorf("compile: %s is not a name bound by the program, so its fields cannot be assigned", base)
	}
	t := env[base]
	fs := &vmFieldSet{base: slot, field: joinPath(s.fieldLhs)}
	for _, seg := range s.fieldLhs[1:] {
		f, deref, ok := fieldOf(t, seg)
		if !ok {
			return nil, fmt.Errorf("compile: %s has no field %s", t, seg)
		}
		fs.steps = append(fs.steps, fieldStep{index: f.Index, deref: deref})
		t = f.Type
	}

	if s.lit != nil {
		v, err := literalValue(*s.lit, t)
		if err != nil {
			return nil, fmt.Errorf("compile: %s: %w", fs.field, err)
		}
		fs.val = &vmArg{kind: vaConst, val: v, typ: t, iface: -1}
		return fs, nil
	}
	call, rt, err := c.compileExpr(slots, env, s.call)
	if err != nil {
		return nil, err
	}
	if rt == nil || !rt.AssignableTo(t) {
		return nil, fmt.Errorf("compile: %s: cannot assign %s to %s", fs.field, rt, t)
	}
	fs.val = &vmArg{kind: vaCall, sub: call, typ: t, iface: -1}
	return fs, nil
}

// apply writes the value through the field chain. Addressability comes
// from a pointer in the chain; a struct held by value in a slot is only
// settable when the slot was created addressable by a var declaration.
func (fs *vmFieldSet) apply(ctx context.Context, slots, frame []reflect.Value, ifaces []ifacePair, stack map[string]any, dest any) error {
	v := slots[fs.base]
	if !v.IsValid() {
		return fmt.Errorf("exec: %s: the base is not set", fs.field)
	}
	for _, st := range fs.steps {
		if st.deref {
			if v.IsNil() {
				return fmt.Errorf("exec: %s: field write on a nil %s", fs.field, v.Type())
			}
			v = v.Elem()
		}
		v = v.FieldByIndex(st.index)
	}
	if !v.CanSet() {
		return fmt.Errorf("exec: %s: the value is not addressable, declare the base with var or hold it behind a pointer", fs.field)
	}
	val, err := fs.val.get(ctx, slots, frame, ifaces, stack, dest)
	if err != nil {
		return err
	}
	v.Set(val)
	return nil
}

// compileRetVal compiles the value of a "return x;" form. The
// parameter type it is compiled against is its own: a program-bound
// name uses its static type, a stack name has none and is returned as
// it is, a literal keeps its natural width.
func (c *Compiler) compileRetVal(slots map[string]int, env map[string]reflect.Type, a arg) (*vmArg, error) {
	pt := reflect.TypeFor[any]()
	switch a.kind {
	case argVar:
		if t, ok := env[a.str]; ok && t != nil {
			pt = t
		}
	case argString:
		pt = reflect.TypeFor[string]()
	case argInt:
		pt = reflect.TypeFor[int64]()
	case argFloat:
		pt = reflect.TypeFor[float64]()
	case argBool:
		pt = reflect.TypeFor[bool]()
	case argNil:
		return nil, fmt.Errorf("compile: return nil returns no value, use return;")
	case argPath:
		// compileArg resolves the fields and checks assignability
		// against pt, so any is what lets the field keep its own type.
	}
	return c.compileArg(slots, env, "return", 0, pt, a)
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

	call := &vmCall{fn: fn, name: name, errIdx: -1}
	if recv != nil {
		if !recv.typ.AssignableTo(ft.In(0)) {
			return nil, fmt.Errorf("compile: %s: receiver is %s, want %s", name, recv.typ, ft.In(0))
		}
		call.args = append(call.args, recv)
	}

	// Parameters and written arguments advance independently: a
	// context.Context parameter is filled from the execution context
	// and consumes no argument, unless the argument standing at that
	// position is itself statically a context, which is how a program
	// passes one it made on purpose.
	fixed := ft.NumIn()
	if ft.IsVariadic() {
		fixed--
	}
	j := 0
	for i := len(call.args); i < fixed; i++ {
		pt := ft.In(i)
		if pt == ctxType && !(j < len(src) && c.staticType(env, src[j]) == ctxType) {
			call.args = append(call.args, &vmArg{kind: vaCtx, typ: pt, iface: -1})
			continue
		}
		// A parameter the source does not supply is its zero value:
		// "" for string, nil for io.Reader.
		if j >= len(src) {
			call.args = append(call.args, &vmArg{kind: vaConst, val: reflect.Zero(pt), typ: pt, iface: -1})
			continue
		}
		if src[j].spread {
			return nil, fmt.Errorf("compile: %s argument %d: ... spreads only into a variadic parameter", name, j+1)
		}
		a, err := c.compileArg(slots, env, name, j, pt, src[j])
		if err != nil {
			return nil, err
		}
		call.args = append(call.args, a)
		j++
	}

	rest := src[min(j, len(src)):]
	switch {
	case !ft.IsVariadic():
		if len(rest) > 0 {
			return nil, fmt.Errorf("compile: %s takes %d arguments, got %d", name, fixed-boolToInt(recv != nil), len(src))
		}
	case len(rest) == 1 && rest[0].spread:
		// f(xs...): the slice is passed whole and the invocation goes
		// through CallSlice.
		st := ft.In(fixed)
		a, err := c.compileArg(slots, env, name, j, st, unspread(rest[0]))
		if err != nil {
			return nil, err
		}
		call.args = append(call.args, a)
		call.spread = true
	default:
		// Individual arguments pack into the variadic slot, each
		// compiled against the element type; reflect.Value.Call packs
		// them the way a Go call site would.
		et := ft.In(fixed).Elem()
		for _, a := range rest {
			if a.spread {
				return nil, fmt.Errorf("compile: %s: ... must be the only variadic argument", name)
			}
			va, err := c.compileArg(slots, env, name, j, et, a)
			if err != nil {
				return nil, err
			}
			call.args = append(call.args, va)
			j++
		}
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
	case argBool:
		v = reflect.ValueOf(a.b)
	case argNil:
		z, err := nilAs(pt)
		if err != nil {
			return nil, fmt.Errorf("compile: %s argument %d: %w", name, pos+1, err)
		}
		return &vmArg{kind: vaConst, val: z, typ: pt, iface: -1}, nil
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
func (p *vmProgram) run(ctx context.Context, stack map[string]any, dest any) (any, error) {
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
		// New rather than Zero, so a field of a var-declared struct is
		// settable in place.
		slots[in.slot] = reflect.New(in.zero.Type()).Elem()
	}
	for i := range p.stmts {
		s := &p.stmts[i]
		if s.lit.IsValid() {
			slots[s.out[0]] = s.lit
			continue
		}
		if s.fieldSet != nil {
			if err := s.fieldSet.apply(ctx, slots, frame, ifaces, stack, dest); err != nil {
				return nil, err
			}
			continue
		}
		if s.retArg != nil {
			v, err := s.retArg.get(ctx, slots, frame, ifaces, stack, dest)
			if err != nil {
				return nil, err
			}
			if !v.IsValid() {
				return nil, nil
			}
			return v.Interface(), nil
		}
		if s.call == nil {
			return nil, nil
		}
		out, err := s.call.invoke(ctx, slots, frame, ifaces, stack, dest)
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
func (c *vmCall) invoke(ctx context.Context, slots, frame []reflect.Value, ifaces []ifacePair, stack map[string]any, dest any) ([]reflect.Value, error) {
	args := frame[c.off : c.off+len(c.args)]
	for i, a := range c.args {
		v, err := a.get(ctx, slots, frame, ifaces, stack, dest)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	var out []reflect.Value
	if c.spread {
		out = c.fn.CallSlice(args)
	} else {
		out = c.fn.Call(args)
	}
	if c.errIdx >= 0 {
		if e := out[c.errIdx]; !e.IsNil() {
			return nil, e.Interface().(error)
		}
	}
	return out, nil
}

// get resolves one argument for this call.
func (a *vmArg) get(ctx context.Context, slots, frame []reflect.Value, ifaces []ifacePair, stack map[string]any, dest any) (reflect.Value, error) {
	switch a.kind {
	case vaConst:
		return a.val, nil
	case vaSlot:
		v := slots[a.slot]
		if !v.IsValid() {
			return reflect.Zero(a.typ), nil
		}
		return v, nil
	case vaCtx:
		return reflect.ValueOf(ctx), nil
	case vaCall:
		out, err := a.sub.invoke(ctx, slots, frame, ifaces, stack, dest)
		if err != nil {
			return reflect.Value{}, err
		}
		return firstNonErr(out, a.sub.errIdx), nil
	case vaField:
		v, err := a.src.get(ctx, slots, frame, ifaces, stack, dest)
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
