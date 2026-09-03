package callbacks

import (
	"fmt"
	"reflect"
	"sync"
)

// compiledFunc is the type of the constructed closure a statement
// compiles to: the JIT'd direct call when the binding fits a shape in
// the table, otherwise the reflect path. The result is the callee's
// return value boxed into an any; a pointer result boxes without an
// allocation.
type compiledFunc = func(stack map[string]any) (any, error)

// Runtime holds the bindings and the expression -> func cache. Create
// one with NewRuntime, register functions with Bind, then use Eval, or
// Compile once and Exec/Scan many times.
type Runtime struct {
	mu       sync.RWMutex
	compiler Compiler
	cache    map[string]compiledFunc
}

// NewRuntime returns an empty Runtime.
func NewRuntime() *Runtime {
	return &Runtime{
		compiler: Compiler{bindings: map[string]binding{}},
		cache:    map[string]compiledFunc{},
	}
}

// Bind registers a Go function under a name, e.g.
// Bind("NewRequest", http.NewRequest).
func (r *Runtime) Bind(name string, fn any) error {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		return fmt.Errorf("bind: %s is %s, want func", name, v.Kind())
	}
	r.mu.Lock()
	r.compiler.bindings[name] = binding{rv: v, raw: fn}
	r.mu.Unlock()
	return nil
}

// Compile parses and compiles a statement into its constructed func.
// Results are cached per statement string: the second Compile of the
// same statement is a map lookup.
func (r *Runtime) Compile(stmt string) (compiledFunc, error) {
	r.mu.RLock()
	fn, ok := r.cache[stmt]
	r.mu.RUnlock()
	if ok {
		return fn, nil
	}

	fn, err := r.compileUncached(stmt)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.cache[stmt] = fn
	r.mu.Unlock()
	return fn, nil
}

// compileUncached parses and compiles a statement without touching the
// expression cache. This is the full first-run cost of a statement.
func (r *Runtime) compileUncached(stmt string) (compiledFunc, error) {
	call, err := (&Parser{}).Parse(stmt)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	s, err := r.compiler.Compile(call)
	r.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	return s.Func(), nil
}

// Eval compiles (or reuses the cached compilation of) stmt and executes
// it against the stack.
func Eval[T any](r *Runtime, stmt string, stack map[string]any) (T, error) {
	fn, err := r.Compile(stmt)
	if err != nil {
		var zero T
		return zero, err
	}
	return Exec[T](fn, stack)
}

// Exec runs a compiled statement against a stack and returns the result
// as T.
func Exec[T any](fn compiledFunc, stack map[string]any) (T, error) {
	var zero T
	out, err := fn(stack)
	if err != nil || out == nil {
		return zero, err
	}
	v, ok := out.(T)
	if !ok {
		return zero, fmt.Errorf("exec: result is %T, want %T", out, zero)
	}
	return v, nil
}

// Scan runs a compiled statement and copies the result into dest, which
// must be a non-nil pointer. When the result is a pointer to dest's
// element type it is dereferenced, so a *http.Request result scans into
// a caller-allocated http.Request without going through an interface.
func Scan[T any](dest T, fn compiledFunc, stack map[string]any) error {
	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return fmt.Errorf("scan: dest must be a non-nil pointer, got %T", dest)
	}
	res, err := fn(stack)
	if err != nil {
		return err
	}
	de := dv.Elem()
	if res == nil {
		de.SetZero()
		return nil
	}
	out := reflect.ValueOf(res)
	switch {
	case out.Type().AssignableTo(de.Type()):
		de.Set(out)
	case out.Kind() == reflect.Pointer && out.Type().Elem() == de.Type():
		if out.IsNil() {
			de.SetZero()
		} else {
			de.Set(out.Elem())
		}
	default:
		return fmt.Errorf("scan: cannot scan %s into %s", out.Type(), de.Type())
	}
	return nil
}
