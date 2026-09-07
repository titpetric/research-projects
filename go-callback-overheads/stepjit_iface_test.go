package gozero

import (
	"bytes"
	"context"
	"net/url"
	"reflect"
	"testing"
	"unsafe"
)

// TestValueStructIntoInterface pins the aliasing rule for a struct
// slot handed to an interface parameter: written once it aliases the
// frame, and a later field write would be visible behind a retained
// interface, so a field write counts as a write and the alias is
// refused in favour of the bridge.
func TestValueStructIntoInterface(t *testing.T) {
	rt := pairRuntime(t)
	var kept any
	if err := rt.Bind("keep", func(v any) (*url.URL, error) {
		kept = v
		return &url.URL{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	const src = `
		var u url.URL;
		u.Path = "/before";
		keep(u);
		u.Path = "/after";
		json.NewEncoder(dest).Encode(u.Path);
	`
	fn, err := rt.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	var dest bytes.Buffer
	if err := fn.Scan(&dest, nil); err != nil {
		t.Fatal(err)
	}
	u, ok := kept.(url.URL)
	if !ok {
		t.Fatalf("callee kept %T, want url.URL", kept)
	}
	if u.Path != "/before" {
		t.Errorf("callee kept Path=%q, want /before: the later write leaked through", u.Path)
	}
	if got, want := dest.String(), "\"/after\"\n"; got != want {
		t.Errorf("dest = %q, want %q", got, want)
	}
}

// TestScalarIfaceSmallBox pins the static box table: bits under 256
// alias staticBoxes at every width, sign patterns included, and
// anything larger still boxes a fresh cell with the right value.
func TestScalarIfaceSmallBox(t *testing.T) {
	anyT := reflect.TypeFor[any]()
	for name, tc := range map[string]struct {
		typ  reflect.Type
		bits uint64
		want any
	}{
		"bool true":    {reflect.TypeFor[bool](), 1, true},
		"bool false":   {reflect.TypeFor[bool](), 0, false},
		"int8 -1":      {reflect.TypeFor[int8](), 0xff, int8(-1)},
		"int8 7":       {reflect.TypeFor[int8](), 7, int8(7)},
		"int16 255":    {reflect.TypeFor[int16](), 255, int16(255)},
		"int16 -1":     {reflect.TypeFor[int16](), 0xffff, int16(-1)},
		"uint16 300":   {reflect.TypeFor[uint16](), 300, uint16(300)},
		"int32 41":     {reflect.TypeFor[int32](), 41, int32(41)},
		"int64 8":      {reflect.TypeFor[int64](), 8, int64(8)},
		"int64 -1":     {reflect.TypeFor[int64](), ^uint64(0), int64(-1)},
		"uint64 large": {reflect.TypeFor[uint64](), 1 << 40, uint64(1) << 40},
	} {
		tab, ok := itabFor(tc.typ, anyT)
		if !ok {
			t.Fatalf("%s: no itab", name)
		}
		bits := tc.bits
		n, err := scalarIface(tab, node{class: layoutOf(tc.typ), N: func(unsafe.Pointer, context.Context, map[string]any, any) (uint64, error) {
			return bits, nil
		}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		pair, err := n.I(nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := *(*any)(unsafe.Pointer(&pair))
		if got != tc.want {
			t.Errorf("%s: boxed %v (%T), want %v (%T)", name, got, got, tc.want, tc.want)
		}
	}
}
