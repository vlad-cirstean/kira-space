package keptvars

import (
	"encoding/json"
	"strings"
	"testing"
)

func secret(name, value string) Entry {
	raw, _ := json.Marshal(value)
	return Entry{Meta: Meta{Name: name, Type: "string", Secret: true}, Value: raw}
}

func TestSecretReplacerLongestFirst(t *testing.T) {
	r := SecretReplacer([]Entry{secret("a", "tok"), secret("b", "tok-extended-1")})
	if got := r.Replace("x tok-extended-1 y tok z"); got != "x {{secret.b}} y {{secret.a}} z" {
		t.Fatalf("got %q", got)
	}
}

func TestSecretFormsEncodedOnlyFromEightBytes(t *testing.T) {
	short := SecretReplacer([]Entry{secret("a", "a b/c")})
	if got := short.Replace("a+b%2Fc a%20b%2Fc"); got != "a+b%2Fc a%20b%2Fc" {
		t.Fatalf("short value masked in encoded form: %q", got)
	}
	long := SecretReplacer([]Entry{secret("a", "a b/c+d=1")})
	for _, form := range []string{"a+b%2Fc%2Bd%3D1", "a%20b%2Fc+d=1", "YSBiL2MrZD0x", "YSBiL2MrZD0x"} {
		if got := long.Replace("v=" + form); got != "v={{secret.a}}" {
			t.Fatalf("form %q not masked: %q", form, got)
		}
	}
}

func TestSecretJSONEscapedForm(t *testing.T) {
	r := SecretReplacer([]Entry{secret("a", `p"q\r`)})
	if got := r.Replace(`{"k":"p\"q\\r"}`); strings.Contains(got, `p\"q`) {
		t.Fatalf("escaped form leaked: %q", got)
	}
	if got := r.Replace(`p"q\r`); got != "{{secret.a}}" {
		t.Fatalf("raw form: %q", got)
	}
}

func TestSecretEmptyValueMasksNothing(t *testing.T) {
	if r := SecretReplacer([]Entry{secret("a", "")}); r != nil {
		t.Fatal("empty value must not build a replacer")
	}
	if raw := SecretRaw([]Entry{secret("a", "")}); len(raw) != 0 {
		t.Fatalf("raw: %v", raw)
	}
}
