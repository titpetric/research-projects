package callbacks

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func newRuntime(t *testing.T) *Runtime {
	t.Helper()
	rt := NewRuntime()
	if err := rt.Bind("NewRequest", http.NewRequest); err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestEval(t *testing.T) {
	rt := newRuntime(t)
	req, err := rt.Eval[*http.Request](`return NewRequest("GET", "https://example.com/index.html");`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "GET" {
		t.Errorf("method = %q, want GET", req.Method)
	}
	if req.URL.Path != "/index.html" {
		t.Errorf("path = %q, want /index.html", req.URL.Path)
	}
	if req.Body != nil {
		t.Errorf("body = %v, want nil from zero-filled io.Reader", req.Body)
	}
}

func TestEvalSingleQuoted(t *testing.T) {
	rt := newRuntime(t)
	req, err := rt.Eval[*http.Request](`return NewRequest('GET', 'https://example.com/sq');`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/sq" {
		t.Errorf("path = %q, want /sq", req.URL.Path)
	}
}

// TestEvalURLVariable feeds several links from a []string through the
// stack into the second parameter of NewRequest and checks the path of
// every returned and scanned value.
func TestEvalURLVariable(t *testing.T) {
	urls := []string{
		"https://example.com/one",
		"https://example.com/two/three",
		"https://example.com/",
		"https://example.com/a%20b",
	}

	rt := newRuntime(t)
	fn, err := rt.Compile(`return NewRequest("GET", url);`)
	if err != nil {
		t.Fatal(err)
	}

	stack := map[string]any{}
	for _, link := range urls {
		want, err := url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		stack["url"] = link

		req, err := fn.Exec[*http.Request](stack)
		if err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		if req.URL.Path != want.Path {
			t.Errorf("%s: exec path = %q, want %q", link, req.URL.Path, want.Path)
		}

		var scanned http.Request
		if err := fn.Scan(&scanned, stack); err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		if scanned.URL.Path != want.Path {
			t.Errorf("%s: scan path = %q, want %q", link, scanned.URL.Path, want.Path)
		}
	}
}

func TestEvalUnsetVariable(t *testing.T) {
	rt := newRuntime(t)
	// url is not on the stack: it zero-fills to "" and NewRequest gets
	// an empty URL.
	req, err := rt.Eval[*http.Request](`return NewRequest("GET", url);`, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.String() != "" {
		t.Errorf("url = %q, want empty from zero-filled variable", req.URL)
	}
}

func TestEvalBodyVariable(t *testing.T) {
	rt := newRuntime(t)
	req, err := rt.Eval[*http.Request](`return NewRequest("POST", "https://example.com/post", body);`, map[string]any{
		"body": strings.NewReader("payload"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Body == nil {
		t.Fatal("body = nil, want reader")
	}
}

func TestCompileCache(t *testing.T) {
	rt := newRuntime(t)
	stmt := `return NewRequest("GET", "https://example.com");`
	a, err := rt.Compile(stmt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rt.Compile(stmt)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.ValueOf(a).Pointer() != reflect.ValueOf(b).Pointer() {
		t.Error("second Compile did not return the cached func")
	}
}

func TestCompileErrors(t *testing.T) {
	rt := newRuntime(t)
	for name, stmt := range map[string]string{
		"unknown binding": `return Missing("GET");`,
		"too many args":   `return NewRequest("GET", "https://example.com", body, "extra");`,
		"int literal":     `return NewRequest(42, "https://example.com");`,
		"float literal":   `return NewRequest(4.2, "https://example.com");`,
		"no return":       `NewRequest("GET")`,
		"trailing input":  `return NewRequest("GET"); extra`,
		"unterminated":    `return NewRequest("GET`,
	} {
		if _, err := rt.Compile(stmt); err == nil {
			t.Errorf("%s: expected compile error for %q", name, stmt)
		}
	}
}

func TestExecVariableTypeMismatch(t *testing.T) {
	rt := newRuntime(t)
	fn, err := rt.Compile(`return NewRequest("GET", url);`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fn.Exec[*http.Request](map[string]any{"url": 42}); err == nil {
		t.Fatal("expected type mismatch for int stack value in string slot")
	}
}

// TestJITMatchesReflect pins the unsafe shape-cast against the reflect
// path: the same compiled Statement runs through both s.fast and
// s.call, on the literal and the variable statement, on the success and
// the error path, and the results must agree.
func TestJITMatchesReflect(t *testing.T) {
	rt := newRuntime(t)
	for name, tc := range map[string]struct {
		stmt  string
		stack map[string]any
	}{
		"literal":   {`return NewRequest("GET", "https://example.com/jit");`, nil},
		"variable":  {`return NewRequest("GET", url);`, map[string]any{"url": "https://example.com/var"}},
		"unset var": {`return NewRequest("GET", url);`, map[string]any{}},
		"error":     {`return NewRequest("bad method", "https://example.com/");`, nil},
	} {
		call, err := (&Parser{}).Parse(tc.stmt)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		s, err := rt.compiler.Compile(call)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s.fast == nil {
			t.Fatalf("%s: statement did not JIT", name)
		}

		fastRes, fastErr := s.fast(tc.stack)
		slowRes, slowErr := s.call(tc.stack)
		if (fastErr == nil) != (slowErr == nil) {
			t.Fatalf("%s: err = %v (jit) vs %v (reflect)", name, fastErr, slowErr)
		}
		if fastErr != nil {
			if fastErr.Error() != slowErr.Error() {
				t.Errorf("%s: err = %q (jit) vs %q (reflect)", name, fastErr, slowErr)
			}
			continue
		}
		fastReq, ok := fastRes.(*http.Request)
		if !ok {
			t.Fatalf("%s: jit result is %T, want *http.Request", name, fastRes)
		}
		slowReq := slowRes.(*http.Request)
		if fastReq.Method != slowReq.Method || fastReq.URL.String() != slowReq.URL.String() {
			t.Errorf("%s: jit %s %s vs reflect %s %s",
				name, fastReq.Method, fastReq.URL, slowReq.Method, slowReq.URL)
		}
	}
}

// TestJITFallback checks that a statement outside the shape table (an
// interface parameter fed from the stack) compiles to the reflect path.
func TestJITFallback(t *testing.T) {
	rt := newRuntime(t)
	call, err := (&Parser{}).Parse(`return NewRequest("POST", "https://example.com/", body);`)
	if err != nil {
		t.Fatal(err)
	}
	s, err := rt.compiler.Compile(call)
	if err != nil {
		t.Fatal(err)
	}
	if s.fast != nil {
		t.Fatal("interface variable statement should not JIT")
	}
}

func TestBindRejectsNonFunc(t *testing.T) {
	rt := NewRuntime()
	if err := rt.Bind("x", 42); err == nil {
		t.Fatal("expected error binding a non-func")
	}
}
