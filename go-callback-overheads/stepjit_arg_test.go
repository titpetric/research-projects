package callbacks

import (
	"bytes"
	"context"
	"net/url"
	"testing"
)

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
