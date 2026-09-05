package callbacks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func vmRuntime(t *testing.T) *Runtime {
	t.Helper()
	rt := NewRuntime()
	if err := rt.BindScope("http", map[string]any{
		"NewRequest": http.NewRequest,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.BindScope("json", map[string]any{
		"NewEncoder": json.NewEncoder,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.BindScope("url", map[string]any{
		"Parse": url.Parse,
	}); err != nil {
		t.Fatal(err)
	}
	return rt
}

// TestProgramWritesThroughDest is the target program: four statements,
// no error is named anywhere, no return, and the output arrives through
// the pointer Scan was handed. Cookies and Encode are not bound; they
// are reached through the result types of http.NewRequest and
// json.NewEncoder.
func TestProgramWritesThroughDest(t *testing.T) {
	rt := vmRuntime(t)
	const src = `
		req := http.NewRequest("GET", "/");
		cookies := req.Cookies();
		enc := json.NewEncoder(dest);
		enc.Encode(cookies);
	`
	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	var dest bytes.Buffer
	if err := fn.Scan(&dest, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := dest.String(), "[]\n"; got != want {
		t.Errorf("dest = %q, want %q", got, want)
	}
}

// TestProgramChainedAndNested is the same program as one statement:
// json.NewEncoder(dest).Encode(...) chains a method onto a call result,
// and http.NewRequest("GET", "/").Cookies() is that same chain nested
// as an argument.
func TestProgramChainedAndNested(t *testing.T) {
	rt := vmRuntime(t)
	const src = `json.NewEncoder(dest).Encode(http.NewRequest("GET", "/").Cookies());`

	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	var dest bytes.Buffer
	if err := fn.Scan(&dest, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := dest.String(), "[]\n"; got != want {
		t.Errorf("dest = %q, want %q", got, want)
	}
}

// TestProgramOmittedArgument checks the third parameter of
// http.NewRequest being filled with its zero value: the request is
// built with a nil body.
func TestProgramOmittedArgument(t *testing.T) {
	rt := vmRuntime(t)
	fn, err := rt.Compile(`return http.NewRequest("GET", "https://example.com/x");`)
	if err != nil {
		t.Fatal(err)
	}
	req, err := fn.Exec[*http.Request](nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Body != nil {
		t.Errorf("Body = %v, want nil from the omitted argument", req.Body)
	}
	if req.URL.Path != "/x" {
		t.Errorf("path = %q, want /x", req.URL.Path)
	}
}

// TestProgramErrorBubbles checks that a failing call stops the program
// and surfaces through Scan without the source naming an error.
func TestProgramErrorBubbles(t *testing.T) {
	rt := vmRuntime(t)
	const src = `
		req := http.NewRequest("bad method", "/");
		enc := json.NewEncoder(dest);
		enc.Encode(req);
	`
	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	var dest bytes.Buffer
	err = fn.Scan(&dest, nil)
	if err == nil {
		t.Fatal("expected the NewRequest error to bubble up")
	}
	if dest.Len() != 0 {
		t.Errorf("dest = %q, want nothing written after the error", dest.String())
	}
}

// TestProgramDiscardedResult checks a statement with no left-hand side:
// the request is dropped but its error is still checked.
func TestProgramDiscardedResult(t *testing.T) {
	rt := vmRuntime(t)
	const src = `
		http.NewRequest("bad method", "/");
		enc := json.NewEncoder(dest);
		enc.Encode("reached");
	`
	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	var dest bytes.Buffer
	if err := fn.Scan(&dest, nil); err == nil {
		t.Fatal("expected the discarded call's error to bubble up")
	}

	const ok = `
		http.NewRequest("GET", "/");
		enc := json.NewEncoder(dest);
		enc.Encode("reached");
	`
	fn, err = rt.Compile(ok)
	if err != nil {
		t.Fatal(err)
	}
	dest.Reset()
	if err := fn.Scan(&dest, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := dest.String(), "\"reached\"\n"; got != want {
		t.Errorf("dest = %q, want %q", got, want)
	}
}

// TestProgramUnknownMethod checks that a method missing from the result
// type is a compile error, not an execution one.
func TestProgramUnknownMethod(t *testing.T) {
	rt := vmRuntime(t)
	_, err := rt.Compile(`
		req := http.NewRequest("GET", "/");
		req.Nope();
	`)
	if err == nil {
		t.Fatal("expected a compile error for the unknown method")
	}
	t.Log(err)
}

// TestProgramValueResult checks that a returned value reaches Exec and
// that Scan copies it into dest.
func TestProgramValueResult(t *testing.T) {
	rt := vmRuntime(t)
	const src = `
		req := http.NewRequest("GET", "https://example.com/a/b");
		return url.Parse("https://example.com/a/b");
	`
	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	u, err := fn.Exec[*url.URL](nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/a/b" {
		t.Errorf("path = %q, want /a/b", u.Path)
	}

	var scanned url.URL
	if err := fn.Scan(&scanned, nil); err != nil {
		t.Fatal(err)
	}
	if scanned.Path != "/a/b" {
		t.Errorf("scanned path = %q, want /a/b", scanned.Path)
	}
}

// TestProgramDestOnlyFromScan checks that a program naming dest fails
// under Exec, which has no destination to give it.
func TestProgramDestOnlyFromScan(t *testing.T) {
	rt := vmRuntime(t)
	fn, err := rt.Compile(`json.NewEncoder(dest).Encode("x");`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fn.Exec[any](nil); err == nil {
		t.Fatal("expected an error naming dest under Exec")
	}
}

// TestFieldAccess covers reading a struct field, which is not a method
// and so is resolved separately: as an argument, as the receiver of a
// method, and through a chain of two fields.
func TestFieldAccess(t *testing.T) {
	rt := vmRuntime(t)
	for _, tc := range []struct{ name, src, want string }{
		{
			"field as argument",
			`req := http.NewRequest("GET", "/");
			 json.NewEncoder(dest).Encode(req.Header);`,
			"{}\n",
		},
		{
			"field of a field",
			`req := http.NewRequest("GET", "/a/b");
			 json.NewEncoder(dest).Encode(req.URL.Path);`,
			"\"/a/b\"\n",
		},
		{
			"method on a field mutates through it",
			`req := http.NewRequest("GET", "/");
			 req.Header.Set("X-A", "1");
			 json.NewEncoder(dest).Encode(req.Header);`,
			"{\"X-A\":[\"1\"]}\n",
		},
	} {
		fn, err := rt.Compile(tc.src)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		var dest bytes.Buffer
		if err := fn.Scan(&dest, nil); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got := dest.String(); got != tc.want {
			t.Errorf("%s: dest = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFieldAccessErrors checks what a field cannot do: be called, and
// be read when it is unexported or absent.
func TestFieldAccessErrors(t *testing.T) {
	rt := vmRuntime(t)
	for _, tc := range []struct{ name, src string }{
		{"called as a method", `req := http.NewRequest("GET", "/"); req.Header();`},
		{"unexported", `req := http.NewRequest("GET", "/"); json.NewEncoder(dest).Encode(req.ctx);`},
		{"absent", `req := http.NewRequest("GET", "/"); json.NewEncoder(dest).Encode(req.Nope);`},
		{"on an opaque stack name", `json.NewEncoder(dest).Encode(v.Field);`},
	} {
		if _, err := rt.Compile(tc.src); err == nil {
			t.Errorf("%s: expected a compile error", tc.name)
		} else {
			t.Logf("%s: %v", tc.name, err)
		}
	}
}
