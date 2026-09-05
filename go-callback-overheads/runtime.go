package callbacks

import (
	"fmt"
	"reflect"
	"sync"
)

// CompiledFunc is the constructed closure a statement compiles to: the
// JIT'd direct call when the binding fits a shape in the table,
// otherwise the reflect path. The result is the callee's return value
// boxed into an any; a pointer result boxes without an allocation.
//
// It is a defined type rather than a plain func so that Exec and Scan
// can hang off it as generic methods.
type CompiledFunc func(stack map[string]any) (any, error)

// Runtime holds the bindings and the expression -> func cache. Create
// one with NewRuntime, register functions with Bind, then use Eval, or
// Compile once and Exec/Scan many times.
type Runtime struct {
	mu       sync.RWMutex
	compiler Compiler
	cache    map[string]CompiledFunc
}

// NewRuntime returns an empty Runtime.
func NewRuntime() *Runtime {
	return &Runtime{
		compiler: Compiler{bindings: map[string]binding{}},
		cache:    map[string]CompiledFunc{},
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
func (r *Runtime) Compile(stmt string) (CompiledFunc, error) {
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
func (r *Runtime) compileUncached(stmt string) (CompiledFunc, error) {
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
// it against the stack. T is the expected result type and cannot be
// inferred, so it is always instantiated explicitly:
// rt.Eval[*http.Request](stmt, stack).
func (r *Runtime) Eval[T any](stmt string, stack map[string]any) (T, error) {
	fn, err := r.Compile(stmt)
	if err != nil {
		var zero T
		return zero, err
	}
	return fn.Exec[T](stack)
}

// Exec runs a compiled statement against a stack and returns the result
// as T. Like Eval, T appears only in the result and is instantiated
// explicitly.
func (fn CompiledFunc) Exec[T any](stack map[string]any) (T, error) {
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

// Scan runs a compiled statement and copies the result into dest. When
// the result is a *T it is dereferenced, so a *http.Request result
// scans into a caller-allocated http.Request without going through an
// interface. T is inferred from dest.
func (fn CompiledFunc) Scan[T any](dest *T, stack map[string]any) error {
	if dest == nil {
		return fmt.Errorf("scan: dest must be a non-nil *%T", *new(T))
	}
	res, err := fn(stack)
	if err != nil {
		return err
	}
	de := reflect.ValueOf(dest).Elem()
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
