package callbacks

import (
	"fmt"
	"reflect"
)

var errType = reflect.TypeOf((*error)(nil)).Elem()

// binding is one bound function: its reflect.Value for the general
// call path and the raw func value the JIT takes its funcval pointer
// from.
type binding struct {
	rv  reflect.Value
	raw any
}

// Compiler turns a parsed callExpr into a Statement against a set of
// bound functions. All argument validation happens here, at compile
// time: literal types must be assignable to the parameter types of the
// binding. Variable references are only type-checked at execution time,
// when their value is known.
type Compiler struct {
	bindings map[string]binding
}

// Compile validates the call against the binding and builds a Statement.
func (c *Compiler) Compile(call *callExpr) (*Statement, error) {
	b, ok := c.bindings[call.name]
	if !ok {
		return nil, fmt.Errorf("compile: unknown binding %q", call.name)
	}
	fn := b.rv
	ft := fn.Type()
	if ft.IsVariadic() {
		return nil, fmt.Errorf("compile: variadic binding %q not supported", call.name)
	}
	if len(call.args) > ft.NumIn() {
		return nil, fmt.Errorf("compile: %s takes %d arguments, got %d", call.name, ft.NumIn(), len(call.args))
	}

	s := &Statement{
		fn:       fn,
		args:     make([]reflect.Value, ft.NumIn()),
		outIndex: -1,
		errIndex: -1,
	}

	for i := 0; i < ft.NumIn(); i++ {
		pt := ft.In(i)
		// Missing arguments are filled with their zero value: "" for
		// string, nil for io.Reader and other pointer-shaped types.
		if i >= len(call.args) {
			s.args[i] = reflect.Zero(pt)
			continue
		}
		a := call.args[i]
		var v reflect.Value
		switch a.kind {
		case argString:
			v = reflect.ValueOf(a.str)
		case argInt:
			v = reflect.ValueOf(a.i)
		case argFloat:
			v = reflect.ValueOf(a.f)
		case argVar:
			s.vars = append(s.vars, varSlot{index: i, name: a.str, typ: pt, zero: reflect.Zero(pt)})
			s.args[i] = reflect.Zero(pt)
			continue
		}
		if !v.Type().AssignableTo(pt) {
			return nil, fmt.Errorf("compile: %s argument %d: cannot use %s as %s", call.name, i+1, v.Type(), pt)
		}
		s.args[i] = v
	}

	for i := 0; i < ft.NumOut(); i++ {
		if ft.Out(i) == errType {
			s.errIndex = i
		} else if s.outIndex < 0 {
			s.outIndex = i
		}
	}

	s.fast = jitCompile(b.raw, ft, call)
	return s, nil
}
