package callbacks

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestBridgedCallStaysInTheTree replaces the whole-program fallback
// test: a call outside the shape table no longer sends the program to
// the reflect evaluator, it compiles to a reflect bridge and its
// neighbours stay direct calls. Supports reports the bridge, and the
// bridged call still runs correctly.
func TestBridgedCallStaysInTheTree(t *testing.T) {
	rt := pairRuntime(t)
	// Five string parameters is a shape the table does not carry, so
	// the call bridges while its neighbours stay direct.
	if err := rt.Bind("join5", func(a, b, c, d, e string) (*url.URL, error) {
		return &url.URL{Path: "/" + a + b + c + d + e}, nil
	}); err != nil {
		t.Fatal(err)
	}
	const src = `
		u := join5("b", "r", "i", "d", "ge");
		json.NewEncoder(dest).Encode(u.Path);
	`
	err := rt.Supports(src)
	if err == nil {
		t.Fatal("a five string call should be reported as bridged")
	}
	if !strings.Contains(err.Error(), "bridge") || !strings.Contains(err.Error(), "join5") {
		t.Fatalf("Supports = %v, want it to name the bridged call", err)
	}

	prog, perr := (&Parser{}).Parse(src)
	if perr != nil {
		t.Fatal(perr)
	}
	p, perr := rt.compiler.compileProgram(prog)
	if perr != nil {
		t.Fatal(perr)
	}
	jp, jerr := jitCompileProgram(p)
	if jerr != nil {
		t.Fatalf("the program should still compile to the tree: %v", jerr)
	}
	if len(jp.bridged) == 0 {
		t.Fatal("expected a bridged call")
	}

	var dest bytes.Buffer
	if _, err := jp.run(context.Background(), nil, &dest); err != nil {
		t.Fatal(err)
	}
	if got, want := dest.String(), "\"/bridge\"\n"; got != want {
		t.Errorf("dest = %q, want %q", got, want)
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
