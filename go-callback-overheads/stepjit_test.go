package callbacks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"testing"
)

// compilePair compiles src twice: once as the step-JIT program and once
// as the reflect evaluator, so a test can run the same source down both
// paths and compare. It fails when the source does not JIT, because a
// silent fallback would make an equivalence test pass by running the
// reflect path twice.
func compilePair(t *testing.T, rt *Runtime, src string) (jit, slow CompiledFunc) {
	t.Helper()
	prog, err := (&Parser{}).Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p, err := rt.compiler.compileProgram(prog)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	jp, err := jitCompileProgram(p)
	if err != nil {
		t.Fatalf("program did not JIT: %s: %v", src, err)
	}
	return jp.run, p.run
}

func pairRuntime(t *testing.T) *Runtime {
	t.Helper()
	rt := NewRuntime()
	for name, fns := range map[string]map[string]any{
		"http": {"NewRequest": http.NewRequest},
		"json": {"NewEncoder": json.NewEncoder},
		"url":  {"Parse": url.Parse},
	} {
		if err := rt.BindScope(name, fns); err != nil {
			t.Fatal(err)
		}
	}
	return rt
}

// TestStepJITMatchesReflect runs the same program through both tiers
// and requires identical output. Every unsafe path in stepjit.go is
// reachable from one of these: the frame layout, the raw stores, the
// precomputed itabs on both the direct and indirect sides, and the
// error words.
func TestStepJITMatchesReflect(t *testing.T) {
	rt := pairRuntime(t)

	// A pointer slot filling an interface parameter takes the direct
	// path; a slice slot takes the indirect one. Both are covered by
	// recording what the callee actually received.
	// Describing the value rather than storing it proves the callee can
	// read through the interface it was handed: a wrong itab or data
	// word would fault or produce nonsense here, where comparing
	// pointers would only compare two different allocations.
	var seen []string
	if err := rt.Bind("record", func(v any) (*url.URL, error) {
		switch x := v.(type) {
		case *http.Request:
			seen = append(seen, fmt.Sprintf("%T %s %s", x, x.Method, x.URL.Path))
		case []*http.Cookie:
			seen = append(seen, fmt.Sprintf("%T len=%d", x, len(x)))
		case *url.URL:
			seen = append(seen, fmt.Sprintf("%T %s", x, x.Path))
		default:
			seen = append(seen, fmt.Sprintf("%T %v", v, v))
		}
		return &url.URL{Path: "/recorded"}, nil
	}); err != nil {
		t.Fatal(err)
	}

	for name, src := range map[string]string{
		"write through dest": `
			req := http.NewRequest("GET", "/");
			cookies := req.Cookies();
			enc := json.NewEncoder(dest);
			enc.Encode(cookies);
		`,
		"chained and nested": `
			json.NewEncoder(dest).Encode(http.NewRequest("GET", "/").Cookies());
		`,
		"pointer into any (direct)": `
			req := http.NewRequest("GET", "/");
			u := record(req);
			json.NewEncoder(dest).Encode(u);
		`,
		"slice into any (indirect)": `
			cookies := http.NewRequest("GET", "/").Cookies();
			u := record(cookies);
			json.NewEncoder(dest).Encode(u);
		`,
		"omitted argument": `
			json.NewEncoder(dest).Encode(url.Parse("https://example.com/a"));
		`,
		"string from the stack": `
			json.NewEncoder(dest).Encode(url.Parse(link));
		`,
		"field as argument": `
			req := http.NewRequest("GET", "/");
			json.NewEncoder(dest).Encode(req.Header);
		`,
		"field of a field": `
			req := http.NewRequest("GET", "/");
			json.NewEncoder(dest).Encode(req.URL.Path);
		`,
		"field through record": `
			req := http.NewRequest("GET", "/");
			u := record(req.URL);
			json.NewEncoder(dest).Encode(u);
		`,
	} {
		stack := map[string]any{"link": "https://example.com/from-stack"}
		jit, slow := compilePair(t, rt, src)

		seen = nil
		var jitBuf bytes.Buffer
		jitRes, jitErr := jit(context.Background(), stack, &jitBuf)
		jitSeen := fmt.Sprintf("%v", seen)

		seen = nil
		var slowBuf bytes.Buffer
		slowRes, slowErr := slow(context.Background(), stack, &slowBuf)
		slowSeen := fmt.Sprintf("%v", seen)

		switch {
		case (jitErr == nil) != (slowErr == nil):
			t.Errorf("%s: err = %v (jit) vs %v (reflect)", name, jitErr, slowErr)
		case jitErr != nil && jitErr.Error() != slowErr.Error():
			t.Errorf("%s: err = %q (jit) vs %q (reflect)", name, jitErr, slowErr)
		}
		if jitBuf.String() != slowBuf.String() {
			t.Errorf("%s: dest = %q (jit) vs %q (reflect)", name, jitBuf.String(), slowBuf.String())
		}
		if jitSeen != slowSeen {
			t.Errorf("%s: callee saw %s (jit) vs %s (reflect)", name, jitSeen, slowSeen)
		}
		if fmt.Sprintf("%v", jitRes) != fmt.Sprintf("%v", slowRes) {
			t.Errorf("%s: result = %v (jit) vs %v (reflect)", name, jitRes, slowRes)
		}
	}
}

// TestStepJITErrorsMatch checks the error words on the JIT side against
// the reflect side, on a failing binding and on a bad stack value.
func TestStepJITErrorsMatch(t *testing.T) {
	rt := pairRuntime(t)
	if err := rt.Bind("fail", func(s string) (*url.URL, error) {
		return nil, errors.New("boom: " + s)
	}); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		src   string
		stack map[string]any
	}{
		"binding error":  {`u := fail("x"); json.NewEncoder(dest).Encode(u);`, nil},
		"bad url":        {`json.NewEncoder(dest).Encode(url.Parse(link));`, map[string]any{"link": "://bad"}},
		"wrong var type": {`json.NewEncoder(dest).Encode(url.Parse(link));`, map[string]any{"link": 42}},
	} {
		jit, slow := compilePair(t, rt, tc.src)
		var a, b bytes.Buffer
		_, jitErr := jit(context.Background(), tc.stack, &a)
		_, slowErr := slow(context.Background(), tc.stack, &b)
		if jitErr == nil || slowErr == nil {
			t.Errorf("%s: expected errors, got %v (jit) %v (reflect)", name, jitErr, slowErr)
			continue
		}
		if jitErr.Error() != slowErr.Error() {
			t.Errorf("%s: err = %q (jit) vs %q (reflect)", name, jitErr, slowErr)
		}
	}
}

// TestStepJITFrameSurvivesGC forces collections between the steps'
// stores and their reads. A frame allocated with the wrong pointer map,
// or a store that skipped its write barrier, shows up here as a lost or
// corrupted value rather than as a wrong benchmark.
func TestStepJITFrameSurvivesGC(t *testing.T) {
	rt := pairRuntime(t)
	if err := rt.Bind("churn", func(v any) (*url.URL, error) {
		for i := 0; i < 24; i++ {
			runtime.GC()
			_ = make([]byte, 1<<15)
		}
		// v is reached only through the frame slot the previous step
		// wrote, and must still be intact after the collections.
		return &url.URL{Path: fmt.Sprintf("/%d", len(fmt.Sprint(v)))}, nil
	}); err != nil {
		t.Fatal(err)
	}

	const src = `
		cookies := http.NewRequest("GET", "/").Cookies();
		u := churn(cookies);
		json.NewEncoder(dest).Encode(u);
	`
	jit, slow := compilePair(t, rt, src)
	var a, b bytes.Buffer
	if _, err := jit(context.Background(), nil, &a); err != nil {
		t.Fatal(err)
	}
	if _, err := slow(context.Background(), nil, &b); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Errorf("after GC: %q (jit) vs %q (reflect)", a.String(), b.String())
	}
}

// TestStepJITReassignedSlotIsCopied pins the rule behind the aliasing.
// An interface argument taken from a slot normally points at the slot
// rather than a copy, which is what reaches native's allocation count.
// That is only legal while the slot is written once: here it is written
// twice, so the value handed to the callee must be a copy taken at the
// call, not a window onto whatever the slot holds later.
func TestStepJITReassignedSlotIsCopied(t *testing.T) {
	rt := pairRuntime(t)
	var kept any
	if err := rt.Bind("keep", func(v any) (*url.URL, error) {
		kept = v // the callee retains the interface past the call
		return &url.URL{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	const src = `
		s := url.Parse("/a").String();
		keep(s);
		s = url.Parse("/b").String();
		json.NewEncoder(dest).Encode(s);
	`
	if err := rt.Supports(src); err != nil {
		t.Fatalf("expected this to JIT: %v", err)
	}
	jit, slow := compilePair(t, rt, src)
	for _, tc := range []struct {
		name string
		fn   CompiledFunc
	}{{"jit", jit}, {"reflect", slow}} {
		kept = nil
		var dest bytes.Buffer
		if _, err := tc.fn(context.Background(), nil, &dest); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if kept != "/a" {
			t.Errorf("%s: callee kept %v, want /a: the reassignment leaked through the interface", tc.name, kept)
		}
		if got, want := dest.String(), "\"/b\"\n"; got != want {
			t.Errorf("%s: dest = %q, want %q", tc.name, got, want)
		}
	}
}

// TestStepJITFallsBackWholeProgram checks that one step outside the
// table sends the entire program to reflect rather than mixing tiers.
func TestStepJITFallsBackWholeProgram(t *testing.T) {
	rt := pairRuntime(t)
	// An int result has no layout class in the table.
	if err := rt.Bind("count", func() int { return 7 }); err != nil {
		t.Fatal(err)
	}
	prog, err := (&Parser{}).Parse(`
		n := count();
		json.NewEncoder(dest).Encode(url.Parse("/x"));
	`)
	if err != nil {
		t.Fatal(err)
	}
	p, err := rt.compiler.compileProgram(prog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jitCompileProgram(p); err == nil {
		t.Fatal("an int result should keep the program on the reflect tier")
	}

	// The program must still run, and still be correct.
	fn, err := rt.Compile(`
		n := count();
		json.NewEncoder(dest).Encode(url.Parse("/x"));
	`)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := fn.Scan(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Error("fallback program wrote nothing")
	}
}

// TestPlanInlineLeavesTheTreeAlone guards the interaction between the
// two tiers. planInline decides that a value can travel as a return
// value instead of through a slot, which drops the statement that
// produced it. If it recorded that by editing the call tree, a program
// that then failed to JIT would reach the reflect evaluator with the
// producer both spliced into its reader and still present as its own
// statement, and would run it twice.
func TestPlanInlineLeavesTheTreeAlone(t *testing.T) {
	rt := pairRuntime(t)
	calls := 0
	if err := rt.Bind("once", func(s string) (*url.URL, error) {
		calls++
		return &url.URL{Path: s}, nil
	}); err != nil {
		t.Fatal(err)
	}
	// An int result has no layout class, so the program compiles the
	// inline plan and then declines to JIT.
	if err := rt.Bind("count", func() int { return 7 }); err != nil {
		t.Fatal(err)
	}

	const src = `
		u := once("/a");
		s := u.String();
		n := count();
		json.NewEncoder(dest).Encode(s);
	`
	prog, err := (&Parser{}).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	p, err := rt.compiler.compileProgram(prog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jitCompileProgram(p); err == nil {
		t.Fatal("this program is meant to fall back to reflect")
	}
	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	var dest bytes.Buffer
	if err := fn.Scan(&dest, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("once called %d times, want 1", calls)
	}
	if got, want := dest.String(), "\"/a\"\n"; got != want {
		t.Errorf("dest = %q, want %q", got, want)
	}
}

// TestSupportsReportsTheReason checks the public gate: a program that
// JITs reports nil, and one that does not names what stopped it.
func TestSupportsReportsTheReason(t *testing.T) {
	rt := pairRuntime(t)
	if err := rt.Bind("count", func() int { return 7 }); err != nil {
		t.Fatal(err)
	}
	if err := rt.Supports(`json.NewEncoder(dest).Encode(url.Parse("/x"));`); err != nil {
		t.Errorf("supported program reported %v", err)
	}
	err := rt.Supports(`n := count(); json.NewEncoder(dest).Encode("x");`)
	if err == nil {
		t.Fatal("an int result should be reported as unsupported")
	}
	t.Log(err)
	if _, err := rt.Supports(`this is not a program`), error(nil); err != nil {
		_ = err
	}
	if rt.Supports(`nope(`) == nil {
		t.Error("a parse error should be reported")
	}
}

// TestPanicBoundary checks that a panic raised inside a binding comes
// back as an error rather than unwinding through the JIT's raw stores,
// on both tiers.
func TestPanicBoundary(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"jit", `u := boom("x"); json.NewEncoder(dest).Encode(u);`},
		{"reflect", `n := count(); u := boom("x"); json.NewEncoder(dest).Encode(u);`},
	} {
		rt := pairRuntime(t)
		if err := rt.Bind("boom", func(s string) (*url.URL, error) {
			panic("binding exploded: " + s)
		}); err != nil {
			t.Fatal(err)
		}
		if err := rt.Bind("count", func() int { return 7 }); err != nil {
			t.Fatal(err)
		}
		supported := rt.Supports(tc.src) == nil
		if supported != (tc.name == "jit") {
			t.Fatalf("%s: Supports = %v, want %v", tc.name, supported, tc.name == "jit")
		}

		fn, err := rt.Compile(tc.src)
		if err != nil {
			t.Fatal(err)
		}
		var dest bytes.Buffer
		err = fn.Scan(&dest, nil)
		if err == nil {
			t.Fatalf("%s: expected the panic to come back as an error", tc.name)
		}
		var pe *PanicError
		if !errors.As(err, &pe) {
			t.Fatalf("%s: err = %T (%v), want *PanicError", tc.name, err, err)
		}
		if pe.Value != "binding exploded: x" {
			t.Errorf("%s: value = %v", tc.name, pe.Value)
		}
		if len(pe.Stack) == 0 {
			t.Errorf("%s: no stack captured", tc.name)
		}
	}
}

// TestAllocationBudget pins the allocation count of the compiled
// program against the native code it stands for, so a regression fails
// the suite rather than only showing up in a benchmark.
func TestAllocationBudget(t *testing.T) {
	rt := pairRuntime(t)
	if err := rt.Supports(vmProgramSrc); err != nil {
		t.Fatalf("the benchmark program must JIT: %v", err)
	}
	fn, err := rt.Compile(vmProgramSrc)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	got := testing.AllocsPerRun(200, func() {
		buf.Reset()
		if err := fn.Scan(&buf, nil); err != nil {
			panic(err)
		}
	})
	want := testing.AllocsPerRun(200, func() {
		buf.Reset()
		req, _ := http.NewRequest("GET", "/", nil)
		_ = json.NewEncoder(&buf).Encode(req.Cookies())
	})
	// The budget is native plus one. The extra is materialising the
	// cookie slice for the interface Encode takes: the Go compiler can
	// prove that value does not outlive the call and keep it on the
	// stack, while the JIT hands it to a func it reinterpreted and has
	// to assume it escapes. Everything else matches.
	t.Logf("program %.0f allocations/run, native %.0f", got, want)
	if got > want+1 {
		t.Errorf("program allocates %.0f/run, native %.0f, budget %.0f", got, want, want+1)
	}
}

// TestReadCountSeesFieldReads guards the analysis that decides whether a
// producer can be spliced into its reader and its slot dropped.
//
// req.Header is a read of req. Missing it undercounts, and a name read
// once directly and once through a field then looked read-once: the
// producer was spliced into the direct read and its statement dropped,
// leaving the field read pointing at a slot nothing writes. It failed
// safe only because the argument compiler happened to report the
// missing slot, which is not a guarantee.
func TestReadCountSeesFieldReads(t *testing.T) {
	rt := pairRuntime(t)
	if err := rt.Bind("both", func(r *http.Request, h *url.URL) (*url.URL, error) {
		return &url.URL{Path: r.URL.Path + "|" + h.Path}, nil
	}); err != nil {
		t.Fatal(err)
	}
	const src = `
		req := http.NewRequest("GET", "/p");
		u := both(req, req.URL);
		json.NewEncoder(dest).Encode(u.Path);
	`
	prog, err := (&Parser{}).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	p, err := rt.compiler.compileProgram(prog)
	if err != nil {
		t.Fatal(err)
	}
	reads := map[int]int{}
	for i := range p.stmts {
		if p.stmts[i].call != nil {
			countReads(p.stmts[i].call, reads)
		}
	}
	if reads[0] != 2 {
		t.Errorf("req counted %d reads, want 2 (one direct, one through a field)", reads[0])
	}

	jit, slow := compilePair(t, rt, src)
	var a, b bytes.Buffer
	if _, err := jit(context.Background(), nil, &a); err != nil {
		t.Fatal(err)
	}
	if _, err := slow(context.Background(), nil, &b); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Errorf("%q (jit) vs %q (reflect)", a.String(), b.String())
	}
	if got, want := a.String(), "\"/p|/p\"\n"; got != want {
		t.Errorf("dest = %q, want %q", got, want)
	}
}

// TestContextShapeMatchesReflect pins the ISSI_PE shape, added for
// http.NewRequestWithContext: the auto-filled context and both string
// arguments must arrive identically on both tiers.
func TestContextShapeMatchesReflect(t *testing.T) {
	rt := pairRuntime(t)
	if err := rt.Bind("http.NewRequestWithContext", http.NewRequestWithContext); err != nil {
		t.Fatal(err)
	}
	const src = `
		req := http.NewRequestWithContext("PATCH", "https://example.com/two-tier");
		json.NewEncoder(dest).Encode(req.Method);
	`
	jit, slow := compilePair(t, rt, src)
	ctx := context.Background()
	var a, b bytes.Buffer
	if _, err := jit(ctx, nil, &a); err != nil {
		t.Fatal(err)
	}
	if _, err := slow(ctx, nil, &b); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() || a.String() != "\"PATCH\"\n" {
		t.Errorf("dest = %q (jit) vs %q (reflect), want %q", a.String(), b.String(), "\"PATCH\"\n")
	}
}
