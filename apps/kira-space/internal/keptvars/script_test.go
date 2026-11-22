package keptvars

import (
	"reflect"
	"testing"
)

func TestScanScript(t *testing.T) {
	src := `plain.a = 1; const x = plain.b; delete plain.c; plain["d"]++; secret.e = 2;
if (plain.f == 1 || plain.g === 2) {} plain.h.i = 3; plain.j.push(1); o.plain.k = 1; $plain.l = 2;
plain.m += 1; plain.n => 1; --plain.o; ...plain.p; delete plain.q.r;`
	type s struct{ Name, Kind, Op string }
	var got []s
	for _, st := range ScanScript(src) {
		got = append(got, s{st.Name, st.Kind, st.Op})
	}
	want := []s{
		{"a", "plain", "write"}, {"b", "plain", "read"}, {"c", "plain", "delete"}, {"d", "plain", "write"},
		{"e", "secret", "write"}, {"f", "plain", "read"}, {"g", "plain", "read"}, {"h", "plain", "write"},
		{"j", "plain", "read"}, {"m", "plain", "write"}, {"n", "plain", "read"}, {"o", "plain", "write"},
		{"p", "plain", "read"}, {"q", "plain", "write"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}
