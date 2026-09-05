package callbacks

import (
	"fmt"
	"reflect"
)

// varSlot is an argument filled from the stack on every call.
type varSlot struct {
	index int
	name  string
	typ   reflect.Type
	zero  reflect.Value
}

// Statement is the compiled form of one return statement. It
// internalizes the reflect.Value of the bound function and a prebuilt
// argument slice: literals are filled once at compile time, variable
// slots are patched from the stack on each call, and the invocation is
// a single fn.Call.
//
// The argument slice is reused between calls, so a Statement is not
// safe for concurrent use on the reflect path. A JIT'd statement (fast
// != nil) keeps no per-call state and is safe for concurrent use.
type Statement struct {
	fn   reflect.Value
	fast CompiledFunc // JIT'd direct call, nil when out of shape
	args []reflect.Value
	vars []varSlot

	outIndex int // index of the result value, -1 if none
	errIndex int // index of a trailing error value, -1 if none
}

// Func returns the constructed closure over the Statement: the JIT'd
// direct call when the binding fits a shape, otherwise the reflect
// path. This is the value Runtime.Compile caches.
func (s *Statement) Func() CompiledFunc {
	if s.fast != nil {
		return s.fast
	}
	return s.call
}

func (s *Statement) call(stack map[string]any) (any, error) {
	for i := range s.vars {
		v, err := __get(stack, &s.vars[i])
		if err != nil {
			return nil, err
		}
		s.args[s.vars[i].index] = v
	}
	out := s.fn.Call(s.args)
	if s.errIndex >= 0 {
		if e := out[s.errIndex]; !e.IsNil() {
			return nil, e.Interface().(error)
		}
	}
	if s.outIndex < 0 {
		return nil, nil
	}
	return out[s.outIndex].Interface(), nil
}

// __get resolves a variable slot against the stack. Unset or nil
// entries produce the parameter's zero value; a set entry must hold a
// value assignable to the parameter type.
func __get(stack map[string]any, vs *varSlot) (reflect.Value, error) {
	v, ok := stack[vs.name]
	if !ok || v == nil {
		return vs.zero, nil
	}
	rv := reflect.ValueOf(v)
	if !rv.Type().AssignableTo(vs.typ) {
		return reflect.Value{}, fmt.Errorf("exec: variable %q: cannot use %s as %s", vs.name, rv.Type(), vs.typ)
	}
	return rv, nil
}
