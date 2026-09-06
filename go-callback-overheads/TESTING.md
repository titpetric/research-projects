# Fixture tests: programs evaluated hot

The programs under `testdata/` are the VM's test suite and its use case
demonstration at once. Each `.txt` file is one program in the VM's own
language, compiled and executed inside a running Go test, against live
bindings, with the test's own `testing.TB` handed in on the stack. The
program makes its own assertions.

```
// http: build a request, read and write its fields, check the context
// arrives without being written in the source.
req := http.NewRequestWithContext("GET", "https://example.com/a/b");
assert.Equal(tb, "GET", req.Method);

req.Method = "POST";
assert.Equal(tb, "POST", req.Method);
```

Hot evaluation means there is no build step between the text and the
run. The runner reads the file, `Compile` turns it into a tree of typed
closures over the live binding funcvals, and the program executes in
the same process, in the same test, immediately. Editing a fixture and
re-running `go test` is the whole loop; nothing is generated, nothing
is linked, and the same text can be run against changed bindings
without touching it.

## How the runner works

`TestFixtures` globs `testdata/*.txt` and runs one subtest per file.
Each subtest:

1. builds a runtime and binds a standard library surface per scope:
   `http`, `url`, `json`, `bytes`, `fmt`, `strings`, `path`, plus
   `assert`,
2. compiles the file's source, cached per source string,
3. logs the tier: `tier: JIT` when every call is direct, otherwise the
   calls that bridge through reflect, by name,
4. executes with `ExecContext`, the context derived from `t.Context()`
   so cancellation and deadlines flow into the bindings, and the
   subtest's `testing.TB` on the stack as `tb`.

A failing assertion inside the program calls `tb.Errorf` through
testify, which fails the subtest the ordinary way. The runner adds
nothing: the program is the test. `TestAssertFailurePropagates` pins
that a failing assertion actually reaches the TB, so the suite cannot
pass vacuously.

Adding a test is adding a file. Scope it the way the existing ones are
scoped, one concern per file: `http.txt`, `url.txt`, `json.txt`,
`fmt.txt`, `types.txt`, `variadic.txt`.

## The assert bindings

`assert.Equal` is not bound as `assert.Equal`. Its real signature ends
in `msgAndArgs ...interface{}`, and a packed `...interface{}` of
arbitrary arity is a reflect call. The fixture runtime binds a wrapper
without the tail:

```go
"Equal": func(tb any, want, got any, message string) { ... }
```

Every parameter has a layout class, so an assertion is one direct call,
and the message stays optional because every trailing argument is:
`assert.Equal(tb, want, got)` zero-fills it to `""`. `tb` is typed
`any` and asserted to testify's `TestingT` inside the wrapper, which
keeps `testing` out of the runtime's own types.

This is the pattern for binding any variadic API into the hot path:
wrap it at the arity the scripts use and let zero-fill make the tail
optional. The general forms still work unwrapped, spread through the
slice ABI directly and pack through a compiled slice-building closure,
with the reflect bridge as the floor for anything else.

## The context

No fixture ever writes a context. `http.NewRequestWithContext` is
called with two arguments and the runtime fills the `context.Context`
parameter with the execution context, which the runner derives from
`t.Context()`. The http fixture proves the plumbing end to end:

```
ctx := req.Context();
assert.Equal(tb, "fixture", ctxValue(ctx));
```

`ctxValue` is a test binding that reads a value the runner attached to
the execution context. It comes back `"fixture"` only if the context
travelled from the subtest, through the auto-filled parameter, into
the request, and back out.

## What this costs

`BenchmarkFixtures` runs every fixture against a handwritten mirror: a
`testFixtures` method doing the same work with the same assertions,
context from `tb.Context()` on both sides. All six fixtures run fully
on the direct-call tier. Pinned core, inlining disabled:

| fixture | vm | native | ratio |
|---|---|---|---|
| fmt | 2.0us, 8 allocs | 1.1us, 4 | 1.8x |
| http | 3.6us, 9 allocs | 3.1us, 9 | 1.2x |
| json | 5.3us, 17 allocs | 4.4us, 16 | 1.2x |
| types | 2.8us, 10 allocs | 1.6us, 4 | 1.8x |
| url | 6.0us, 15 allocs | 4.9us, 14 | 1.2x |
| variadic | 1.0us, 4 allocs | 0.6us, 3 | 1.6x |

http reaches allocation parity with its mirror; json, url and variadic
are within one allocation, which is the frame. The fixtures that lean
on formatting and assertion plumbing sit under 2x. For the bridge cost
of a call the shape table cannot express, and for the work-only
comparison without assertions, see the benchmarks in
`fixture_bench_test.go`.

## Structs by value

`types.txt` declares `var u url.URL` and reads `u.Path`. A struct held
by value has no layout class, so it cannot travel between calls as a
closure return value; what it can do is live in the frame. The frame is
one `reflect.StructOf` allocation, so the struct is a field in it, the
declaration's zero value is the zeroed frame, and a field read or write
is an offset from the frame pointer. A whole-struct value only needs
transport when it is passed to a binding, where it goes through the
interface alias or, when the slot is written more than once, a copy:
`TestValueStructIntoInterface` pins that a binding that retains the
interface sees the value as it was at the call, not what a later field
write put in the slot.
