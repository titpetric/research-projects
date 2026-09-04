# go-callback-overheads

A minimal virtual machine evaluator for Go API callbacks. Go functions
are bound by name, a single-statement template invokes them, and the
invocation is compiled once into a constructed func that is cached per
expression string. The question the repository answers is what that
costs against calling the function directly, and how much of the cost
can be removed.

The answer is in the results table: a shape-table JIT over the bound
function reruns a cached statement with no allocations on top of the
native `http.NewRequest` call, against the reflect path it replaces.

## Design requirements

- A runtime is created with `runtime := NewRuntime()`. Go functions are
  registered with `Bind("NewRequest", http.NewRequest)`. Reflection over
  the bound function is the general invocation mechanism; the JIT is an
  optimization of that cost, not a second capability.
- The VM implements exactly one statement form, a return statement:
  `return NewRequest("GET", "https://example.com");`. Nothing else is in
  scope, and no binding other than NewRequest is added or tested; anything
  more would extend the implementation scope.
- Type literals are single-quoted and double-quoted strings. Numbers map
  to int64 (no decimal point) or float64 (decimal point) and no other
  numeric type.
- The stack is a `map[string]any`. A bare identifier in the statement is
  a variable reference resolved against the stack at execution time. The
  stack may hold any value; an unset or nil entry fills the argument with
  its zero value (`""` for string, nil for a pointer-shaped type such as
  io.Reader).
- Missing trailing arguments are filled with their zero values the same
  way: `NewRequest("GET", url)` passes nil for the io.Reader body.
- Arguments are validated at compilation time. If the NewRequest binding
  takes `string, string, io.Reader`, only those types can be passed:
  a numeric literal in a string slot is a compile error, as is an excess
  argument or an unknown binding.
- Compilation internalizes the binding into a constructed
  `func(map[string]any) (any, error)`, cached per expression string, so
  the same statement rerun with a different stack skips parsing and
  compilation entirely.

## Execution tiers

Compilation picks one of two tiers for the constructed func.

**Direct call (jit.go).** Under Go's internal ABI, argument and result
passing depends only on the layout classes of the types, not their
names: `io.Reader` and `struct{ tab, data unsafe.Pointer }` are the
same two pointer words, `*http.Request` and `unsafe.Pointer` the same
one, `string` is `string`. A bound func whose signature matches a shape
in the table is reinterpreted via unsafe as the shape type and called
directly, skipping `reflect.Value.Call` and the argument frame and
results slice it allocates.

The shape table covers:

- `(string) (ptr, error)`
- `(string, string) (ptr, error)`
- `(string, string, iface) (ptr, error)`, interface zero-filled by the
  statement

String arguments are literals or stack variables; a variable resolves
with a map lookup and a type assertion per call. Anything outside the
table, including an interface parameter fed from the stack (it would
need an itab conversion), compiles to the reflect tier. Extending the
table is additive: one shape type and one constructor case.

**Reflect fallback (compiler.go).** The prebuilt `[]reflect.Value`
argument slice, variable slots patched from the stack, one `fn.Call`.
This tier is what the package did before the JIT, and what
BenchmarkCostNaive measures.

A JIT'd compiled func keeps no per-call state and is safe for
concurrent use. The reflect fallback reuses its argument slice between
calls and is not.

## What the unsafe layer consists of

The direct call rests on four one-liners, each a fact about Go's
runtime representation:

- A func value held in an `any` is pointer-shaped, so the eface data
  word is the funcval pointer. This holds for top-level functions,
  closures and method values.
- Writing that pointer over a zero func variable of the shape type is
  the whole conversion; a func variable is a single pointer to its
  funcval.
- A pointer result boxes into an `any` by writing the two interface
  words directly: the type word is the `*rtype` behind the binding's
  `reflect.Type` (the data word of that interface), the data word is
  the pointer. This produces the same representation the compiler
  would, without an allocation.
- An error result read back as `struct{ tab, data unsafe.Pointer }` is
  already a valid error interface when `tab != nil`, and converts with
  a pointer cast.

The cast assumes Go's internal ABI layout classes (holds on amd64 and
arm64, verified here on go1.27.0/amd64, including under the race
detector and with inlining disabled). TestJITMatchesReflect runs the
same compiled Statement through both tiers, on the success and the
error path, so a toolchain that changes the assumption fails the test
suite.

## API

```go
runtime := NewRuntime()
runtime.Bind("NewRequest", http.NewRequest)

// Parse + compile + execute, cached per statement string.
req, err := Eval[*http.Request](runtime, `return NewRequest("GET", "https://example.com");`, stack)

// Split form: compile once, execute many times with different stacks.
fn, err := runtime.Compile(`return NewRequest("GET", url);`)
req, err := Exec[*http.Request](fn, stack)

// Scan copies the result into caller-allocated memory.
var req http.Request
err := Scan[*http.Request](&req, fn, stack)
```

`Compile` returns the constructed `func(map[string]any) (any, error)`.
`Eval`, `Exec` and `Scan` are package-level generic functions because Go
methods cannot take type parameters.

## Benchmarks

Method, from atkins.yml: every run is pinned to one core at top
priority (`taskset -c 3 nice -n -20`), a 3s warmup run is discarded
first, and inlining is disabled (`-gcflags=all=-l`) so the native
baseline is not inlined into the loop body. The sweep runs with
`-benchtime 10s -count 3 -benchmem`; the profile job repeats
BenchmarkCostAmortizedCache with `-cpuprofile` and `-memprofile`.

There are exactly four benchmarks. All of them run the statement
`return NewRequest("GET", url);` with the compiled form reused verbatim
between executions against a stack holding the url:

| Benchmark          | Measures                                                 |
|--------------------|----------------------------------------------------------|
| Native             | the direct Go call the statement compiles down to        |
| CostWithoutCaching | parse + compile + execute per op, bypassing the cache    |
| CostNaive          | the once-compiled statement run through the reflect tier |
| CostAmortizedCache | the once-compiled statement run through the JIT tier     |

CostNaive and CostAmortizedCache are the same compiled Statement over
the same binding, taking the same `func(map[string]any) (any, error)`
and resolving the variable from whatever stack they are handed. The
only difference is `Statement.call`, which builds the argument slice
and goes through `reflect.Value.Call`, against the JIT'd direct call.

Each cost benchmark times its own native baseline inline: after the
measured loop ends (`b.Loop` stops the timer, so the extra work is not
measured), it runs `b.N` iterations of `http.NewRequest` under its own
stopwatch and reports `native-ns/op` alongside `cost-ns/op`, the
statement's cost over native. CostWithoutCaching's cost-ns/op is
(no-cache - native), the price of parsing and compilation; CostNaive's
is (reflect - native) and CostAmortizedCache's is (cached - native),
the amortized overhead of going through the VM on either tier. The
baseline includes neither the stack map lookup nor the type assertion,
so both count toward the VM's cost.

Artifacts: `bench.txt` (raw sweep), `cpu.out`/`mem.out` with the
`callbacks.test` binary for `go tool pprof`. Reproduce with
`atkins save` and `atkins profile`.

### Results

`benchstat bench.txt`, medians of the sweep, pinned, Intel N150,
go1.27.0:

| Benchmark          |    sec/op |     B/op | allocs/op | cost-sec/op | native-sec/op |
|--------------------|----------:|---------:|----------:|------------:|--------------:|
| Native             |    600.7n |    512.0 |     3.000 |           - |             - |
| CostWithoutCaching |    1.785µ |  1.062Ki |     11.00 |      1.187µ |        587.6n |
| CostNaive          |    1.303µ |    576.0 |     5.000 |      692.6n |        596.6n |
| CostAmortizedCache |    633.2n |    512.0 |     3.000 |      25.86n |        612.7n |
