# Inlining and the vm/native gap

`BenchmarkFixtures` compared across two builds of the same tree: the
default build, and `-gcflags=all=-l`, which disables inlining in every
package including the standard library. Pinned core (`taskset -c 3`),
Intel N150, go1.27, `-count 3`, medians reported.

## With inlining (default build)

| fixture | vm | native | ratio |
|---|---|---|---|
| fmt | 980ns, 152 B, 7 allocs | 668ns, 48 B, 4 | 1.5x |
| http | 1927ns, 616 B, 9 allocs | 1725ns, 640 B, 9 | 1.1x |
| json | 2400ns, 1016 B, 17 allocs | 2093ns, 728 B, 14 | 1.1x |
| types | 1296ns, 272 B, 9 allocs | 904ns, 38 B, 4 | 1.4x |
| url | 3038ns, 816 B, 15 allocs | 2510ns, 384 B, 12 | 1.2x |
| variadic | 431ns, 85 B, 4 allocs | 351ns, 69 B, 3 | 1.2x |

## Without inlining (-gcflags=all=-l)

| fixture | vm | native | ratio |
|---|---|---|---|
| fmt | 2017ns, 152 B, 7 allocs | 1169ns, 48 B, 4 | 1.7x |
| http | 3633ns, 616 B, 9 allocs | 3081ns, 640 B, 9 | 1.2x |
| json | 5090ns, 1016 B, 17 allocs | 4273ns, 984 B, 16 | 1.2x |
| types | 2637ns, 272 B, 9 allocs | 1586ns, 38 B, 4 | 1.7x |
| url | 5604ns, 816 B, 15 allocs | 4885ns, 784 B, 14 | 1.1x |
| variadic | 932ns, 85 B, 4 allocs | 585ns, 69 B, 3 | 1.6x |

## What the difference says

Disabling inlining roughly doubles both columns: most of every
fixture's time is shared work in fmt, testify and net/*, and that work
inflates equally on both sides. The ratios still move, in two
directions.

The compiled program is a graph of closures called through function
pointers. The compiler can never inline across those calls, in either
build, so the vm column changes only by what the standard library
loses. The native column additionally loses the inlining of its own
statements. Fixtures whose per-statement work is small show it most:
fmt goes 1.5x to 1.7x and variadic 1.2x to 1.6x, because a fixed
per-node dispatch cost stands out once the statements around it stop
being folded away.

Inlining also feeds escape analysis, which shows in the alloc columns.
With inlining, json/native drops from 16 allocs to 14 and url/native
from 14 to 12: inlined callees let the compiler prove pointers do not
escape and keep values on the stack. The vm's counts are identical in
both builds, because its values travel through closure returns and
interface boxes the compiler cannot see through.

So the no-inline ratio is the structural cost of interpreting: one
indirect call and one boxed transport per node, immune to compiler
optimisation. The default-build ratio is what a caller sees, and is
lower because the interpreter's fixed cost is diluted by library work
that inlining has already made faster on both sides.

## Reproducing

```
taskset -c 3 go test -run '^$' -bench 'BenchmarkFixtures/' -benchmem -benchtime 1s -count 3
taskset -c 3 go test -run '^$' -bench 'BenchmarkFixtures/' -benchmem -benchtime 1s -count 3 -gcflags=all=-l
```
