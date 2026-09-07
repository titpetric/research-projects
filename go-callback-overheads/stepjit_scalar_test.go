package gozero

import (
	"bytes"
	"net/url"
	"testing"
)

// TestScalarFromStackJITs pins the tier for a scalar read off the
// caller's stack, which used to send the program to reflect.
func TestScalarFromStackJITs(t *testing.T) {
	rt, seen := litRuntime(t)
	const src = `wantI64(n); json.NewEncoder(dest).Encode("ok");`
	if err := rt.Supports(src); err != nil {
		t.Fatalf("did not JIT: %v", err)
	}
	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	for _, tc := range []struct {
		stack map[string]any
		want  any
		fails bool
	}{
		{map[string]any{"n": int64(9)}, int64(9), false},
		{nil, int64(0), false},
		{map[string]any{"n": "not a number"}, nil, true},
	} {
		*seen = nil
		b.Reset()
		err := fn.Scan(&b, tc.stack)
		if tc.fails {
			if err == nil {
				t.Errorf("stack %v: expected a type error", tc.stack)
			}
			continue
		}
		if err != nil {
			t.Errorf("stack %v: %v", tc.stack, err)
			continue
		}
		if *seen != tc.want {
			t.Errorf("stack %v: callee saw %v, want %v", tc.stack, *seen, tc.want)
		}
	}
}

// TestMixedScalarShapeJITs pins the tier for a call mixing a string and
// a scalar parameter, which used to be outside the table.
func TestMixedScalarShapeJITs(t *testing.T) {
	rt, seen := litRuntime(t)
	const src = `u := mixed("a", 5); return u;`
	if err := rt.Supports(src); err != nil {
		t.Fatalf("did not JIT: %v", err)
	}
	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	*seen = nil
	u, err := fn.Exec[*url.URL](nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "a" || *seen != int64(5) {
		t.Errorf("path=%q seen=%v", u.Path, *seen)
	}
}
