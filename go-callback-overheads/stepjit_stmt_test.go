package callbacks

import (
	"bytes"
	"context"
	"testing"
)

// TestValueStructInFrame pins the struct-by-value slot support: the
// frame holds the struct itself, its zero value is the zeroed frame,
// a field read is an offset from the frame pointer and a field write
// stores through the same offset. Both tiers must agree.
func TestValueStructInFrame(t *testing.T) {
	rt := pairRuntime(t)
	const src = `
		var u url.URL;
		u.Path = "/in-frame";
		u.Scheme = "https";
		json.NewEncoder(dest).Encode(u.Path);
	`
	if err := rt.Supports(src); err != nil {
		t.Fatalf("a value struct var should now JIT: %v", err)
	}
	jit, slow := compilePair(t, rt, src)
	var a, b bytes.Buffer
	if _, err := jit(context.Background(), nil, &a); err != nil {
		t.Fatal(err)
	}
	if _, err := slow(context.Background(), nil, &b); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() || a.String() != "\"/in-frame\"\n" {
		t.Errorf("dest = %q (jit) vs %q (reflect)", a.String(), b.String())
	}
}
