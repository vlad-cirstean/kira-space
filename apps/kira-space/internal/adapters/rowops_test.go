package adapters

import (
	"reflect"
	"testing"
)

func TestParseHeaderPairs_KeepsOrderAndRepeats(t *testing.T) {
	raw := `{"z":"1","a":"2","z":"3"}`
	got, err := ParseHeaderPairs(&raw)
	if err != nil {
		t.Fatalf("ParseHeaderPairs: %v", err)
	}
	want := []HeaderPair{{"z", "1"}, {"a", "2"}, {"z", "3"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pairs = %v, want %v", got, want)
	}
	m, err := ParseHeaderJSON(&raw)
	if err != nil || m["z"] != "3" || m["a"] != "2" {
		t.Errorf("ParseHeaderJSON = %v, %v; want the last repeated value", m, err)
	}
}

func TestParseHeaderPairs_Rejects(t *testing.T) {
	for _, raw := range []string{`[]`, `5`, `null`, `{"a":1}`, `{"a":{"b":"c"}}`, `{"a":"x"} extra`, `{"a":"x"`, `{`, `nope`} {
		if _, err := ParseHeaderPairs(&raw); err == nil {
			t.Errorf("%s: want an error", raw)
		}
	}
	for _, raw := range []string{"", `{}`} {
		if got, err := ParseHeaderPairs(&raw); err != nil || len(got) != 0 {
			t.Errorf("%q: got %v, %v; want no pairs", raw, got, err)
		}
	}
}
