package callbacks

import (
	"fmt"
	"reflect"
	"unsafe"
)

// The JIT: a shape table of typed call templates. Under Go's internal
// ABI, argument and result passing depends only on the layout classes
// of the types, not their names: io.Reader and struct{ tab, data
// unsafe.Pointer } are the same two pointer words, *http.Request and
// unsafe.Pointer the same one, string is string. A bound func whose
// signature matches a shape in the table is reinterpreted as the shape
// type via unsafe and called directly - no reflect.Value.Call, no
// argument frame, no results slice, zero allocations on top of the
// callee's own.
//
// A signature outside the table falls back to the reflect path in
// Statement.call, which stays the general mechanism. Extending the JIT
// is additive: one more shape type and one more constructor case.
//
// The cast relies on Go's internal ABI layout classes; the equivalence
// test in runtime_test.go pins it against the reflect path.

// eface mirrors the two words of an empty interface (any).
type eface struct{ typ, data unsafe.Pointer }

// ifacePair mirrors the two words of any interface value, empty or not.
type ifacePair struct{ tab, data unsafe.Pointer }

// funcPtr returns the funcval pointer of a func value held in an any.
// Func values are pointer-shaped, so the eface data word is the funcval
// pointer itself; this holds for top-level funcs, closures and method
// values alike.
func funcPtr(fn any) unsafe.Pointer {
	return (*eface)(unsafe.Pointer(&fn)).data
}

// rtypePtr returns the *rtype behind a reflect.Type: the data word of
// the interface. Used as the type word when boxing a JIT result into an
// any.
func rtypePtr(t reflect.Type) unsafe.Pointer {
	return (*ifacePair)(unsafe.Pointer(&t)).data
}

// castFn reinterprets a funcval pointer as a func of the shape type F.
// A func variable is a single pointer to its funcval, so writing the
// pointer over a zero F is the whole conversion.
func castFn[F any](p unsafe.Pointer) F {
	var f F
	*(*unsafe.Pointer)(unsafe.Pointer(&f)) = p
	return f
}

// Shape types. s = string, e = interface (two words), p = pointer
// result. Every shape here returns (pointer, error).
type (
	fnS_PE   = func(string) (unsafe.Pointer, ifacePair)
	fnSS_PE  = func(string, string) (unsafe.Pointer, ifacePair)
	fnSSE_PE = func(string, string, ifacePair) (unsafe.Pointer, ifacePair)
)

var strType = reflect.TypeOf("")

// strArg is one string parameter of a JIT'd call: a literal value, or a
// variable name resolved against the stack per call with the same
// semantics as __get (unset or nil fills "", a non-string is an error).
type strArg struct {
	name string // variable name; "" for a literal
	val  string // literal value
}

func (a *strArg) get(stack map[string]any) (string, error) {
	if a.name == "" {
		return a.val, nil
	}
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

// jitResult boxes the two raw result words back into (any, error). The
// error pair, when set, already is a valid error interface. The pointer
// result becomes an any by writing the eface words directly: type word
// from the binding's result type, data word the pointer - the exact
// representation the compiler would produce, without an allocation.
func jitResult(out, p unsafe.Pointer, e ifacePair) (any, error) {
	if e.tab != nil {
		return nil, *(*error)(unsafe.Pointer(&e))
	}
	var res any
	*(*eface)(unsafe.Pointer(&res)) = eface{typ: out, data: p}
	return res, nil
}

// jitCompile builds the direct-call closure for a validated statement,
// or returns nil when the binding's signature is outside the shape
// table. Supported: results (pointer, error); parameters that are plain
// strings (literal, variable, or missing and zero-filled to ""),
// optionally followed by one trailing interface parameter the statement
// leaves zero-filled (nil). An interface parameter fed from the stack
// needs an itab conversion and stays on the reflect path.
func jitCompile(fn any, ft reflect.Type, call *callExpr) compiledFunc {
	if fn == nil {
		return nil
	}
	if ft.NumOut() != 2 || ft.Out(0).Kind() != reflect.Pointer || ft.Out(1) != errType {
		return nil
	}

	n := ft.NumIn()
	var sa [2]strArg
	ns, ne := 0, 0 // leading string params, trailing zero-filled iface params
	for i := 0; i < n; i++ {
		pt := ft.In(i)
		switch {
		case pt == strType && ne == 0 && ns < len(sa):
			if i < len(call.args) {
				switch call.args[i].kind {
				case argString:
					sa[ns].val = call.args[i].str
				case argVar:
					sa[ns].name = call.args[i].str
				default:
					return nil
				}
			}
			ns++
		case pt.Kind() == reflect.Interface && i >= len(call.args) && ne == 0:
			ne++
		default:
			return nil
		}
	}

	out := rtypePtr(ft.Out(0))
	fptr := funcPtr(fn)
	switch {
	case ns == 1 && ne == 0 && n == 1:
		f, a0 := castFn[fnS_PE](fptr), sa[0]
		return func(stack map[string]any) (any, error) {
			s0, err := a0.get(stack)
			if err != nil {
				return nil, err
			}
			p, e := f(s0)
			return jitResult(out, p, e)
		}
	case ns == 2 && ne == 0 && n == 2:
		f, a0, a1 := castFn[fnSS_PE](fptr), sa[0], sa[1]
		return func(stack map[string]any) (any, error) {
			s0, err := a0.get(stack)
			if err != nil {
				return nil, err
			}
			s1, err := a1.get(stack)
			if err != nil {
				return nil, err
			}
			p, e := f(s0, s1)
			return jitResult(out, p, e)
		}
	case ns == 2 && ne == 1 && n == 3:
		f, a0, a1 := castFn[fnSSE_PE](fptr), sa[0], sa[1]
		return func(stack map[string]any) (any, error) {
			s0, err := a0.get(stack)
			if err != nil {
				return nil, err
			}
			s1, err := a1.get(stack)
			if err != nil {
				return nil, err
			}
			p, e := f(s0, s1, ifacePair{})
			return jitResult(out, p, e)
		}
	}
	return nil
}
