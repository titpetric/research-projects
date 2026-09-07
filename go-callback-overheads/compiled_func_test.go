package callbacks

import (
	"context"
	"net/http"
	"testing"
)

// TestCompiledFunc calls the closure directly, the way Exec and Scan
// do underneath: the result comes back boxed in an any.
func TestCompiledFunc(t *testing.T) {
	rt := newRuntime(t)
	fn, err := rt.Compile(`return NewRequest("GET", "https://example.com/raw");`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := fn(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	req, ok := res.(*http.Request)
	if !ok {
		t.Fatalf("result is %T, want *http.Request", res)
	}
	if req.URL.Path != "/raw" {
		t.Errorf("path = %q, want /raw", req.URL.Path)
	}
}

func TestCompiledFunc_Exec(t *testing.T) {
	rt := newRuntime(t)
	fn, err := rt.Compile(`return NewRequest("GET", "https://example.com/exec");`)
	if err != nil {
		t.Fatal(err)
	}
	req, err := fn.Exec[*http.Request](nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/exec" {
		t.Errorf("path = %q, want /exec", req.URL.Path)
	}
	// T not matching the result is an error, not a zero value.
	if _, err := fn.Exec[string](nil); err == nil {
		t.Error("expected a type mismatch asking for a string")
	}
}

func TestCompiledFunc_ExecContext(t *testing.T) {
	rt := newRuntime(t)
	fn, err := rt.Compile(`return NewRequest("GET", "https://example.com/ctx");`)
	if err != nil {
		t.Fatal(err)
	}
	req, err := fn.ExecContext[*http.Request](context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/ctx" {
		t.Errorf("path = %q, want /ctx", req.URL.Path)
	}
}

func TestCompiledFunc_Scan(t *testing.T) {
	rt := newRuntime(t)
	fn, err := rt.Compile(`return NewRequest("GET", "https://example.com/scan");`)
	if err != nil {
		t.Fatal(err)
	}
	// The *http.Request result dereferences into a caller-allocated
	// value.
	var req http.Request
	if err := fn.Scan(&req, nil); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/scan" {
		t.Errorf("path = %q, want /scan", req.URL.Path)
	}
	if err := fn.Scan[http.Request](nil, nil); err == nil {
		t.Error("expected an error for a nil dest")
	}
}

func TestCompiledFunc_ScanContext(t *testing.T) {
	rt := newRuntime(t)
	fn, err := rt.Compile(`return NewRequest("GET", "https://example.com/scanctx");`)
	if err != nil {
		t.Fatal(err)
	}
	var req http.Request
	if err := fn.ScanContext(context.Background(), &req, nil); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/scanctx" {
		t.Errorf("path = %q, want /scanctx", req.URL.Path)
	}
	if err := fn.ScanContext[http.Request](context.Background(), nil, nil); err == nil {
		t.Error("expected an error for a nil dest")
	}
}
