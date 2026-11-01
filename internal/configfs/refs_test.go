package configfs

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRefs(t *testing.T) {
	refs, p := Refs("a ${HOME}/x ${secret:api.token} $${LITERAL} $ ${_X1}")
	if p != nil {
		t.Fatal(p.Message)
	}
	want := []Ref{{RefEnv, "HOME", 2, 9}, {RefSecret, "api.token", 12, 31}, {RefEnv, "_X1", 46, 52}}
	if len(refs) != len(want) {
		t.Fatalf("refs %+v", refs)
	}
	for i, w := range want {
		if refs[i] != w {
			t.Errorf("ref %d: %+v want %+v", i, refs[i], w)
		}
	}
	for _, bad := range []string{"${", "${A", "${A${B}}", "${}", "${1A}", "${secret:}", "${secret:Up}", "${other:x}", "${a b}"} {
		if _, p := Refs(bad); p == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestWholeSecretAndResolve(t *testing.T) {
	if !wholeSecret("${secret:a}") || wholeSecret("x${secret:a}") || wholeSecret("${secret:a}${secret:b}") || wholeSecret("${A}") {
		t.Fatal("wholeSecret rule")
	}
	r := &Resolver{
		Env: func(n string) (string, bool) { return "h", n == "HOST" },
		Secrets: SecretSourceFunc(func(_ context.Context, ns, name string) (string, error) {
			if ns != "mod" || name != "tok" {
				return "", ErrSecretMissing
			}
			return "S3CRET", nil
		}),
		Namespace: "mod",
	}
	got, used, err := r.Resolve(context.Background(), "https://${HOST}/$${HOST}?t=${secret:tok}")
	if err != nil || got != "https://h/${HOST}?t=S3CRET" || len(used) != 1 || used[0].Placeholder != "${secret:tok}" {
		t.Fatalf("%q %+v %v", got, used, err)
	}
	if _, _, err := r.Resolve(context.Background(), "${NOPE}"); err == nil || !strings.Contains(err.Error(), "NOPE") {
		t.Fatalf("env: %v", err)
	}
	if _, _, err := r.Resolve(context.Background(), "${secret:gone}"); !errors.Is(err, ErrSecretMissing) || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("secret: %v", err)
	}
}

func TestSchema_referenceFields(t *testing.T) {
	d := probeDecode(t)
	_, p := d("probe.yaml", []byte("name: a\ntarget:\n  url: ${HOST}/x $${ok}\n  headers:\n    auth: Bearer abc123\n"))
	if p == nil || p.Line != 5 || strings.Contains(p.Message, "abc123") {
		t.Fatalf("literal secret: %+v", p)
	}
	_, p = d("probe.yaml", []byte("name: a\ntarget:\n  url: ${bad name}\n"))
	if p == nil || p.Line != 3 {
		t.Fatalf("bad ref: %+v", p)
	}
	if _, p = d("probe.yaml", []byte("name: a\ntarget:\n  url: ${HOST}\n  headers:\n    auth: ${secret:tok}\n")); p != nil {
		t.Fatal(p.Message)
	}
}
