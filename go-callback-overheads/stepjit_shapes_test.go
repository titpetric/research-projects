package callbacks

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

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
