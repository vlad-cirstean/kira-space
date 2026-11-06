package postman

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestDecodeVariableEntry runs one entry table through both the collection and environment
// decoders: one rule set, identical output.
func TestDecodeVariableEntry(t *testing.T) {
	tests := []struct {
		name         string
		entry        string
		want         []Variable
		wantDisabled int
		wantEmpty    int
		wantUnnamed  int
	}{
		{name: "string default", entry: `{"key":"a","value":"x","type":"default","enabled":true}`,
			want: []Variable{{Name: "a", Value: "x", Type: "default"}}},
		{name: "secret", entry: `{"key":"a","value":"s","type":"secret"}`,
			want: []Variable{{Name: "a", Value: "s", Secret: true, Type: "secret"}}},
		{name: "api type any", entry: `{"key":"a","value":"x","type":"any"}`,
			want: []Variable{{Name: "a", Value: "x", Type: "any"}}},
		{name: "number", entry: `{"key":"a","value":3}`, want: []Variable{{Name: "a", Value: "3"}}},
		{name: "bool", entry: `{"key":"a","value":true}`, want: []Variable{{Name: "a", Value: "true"}}},
		{name: "object", entry: `{"key":"a","value":{ "a": 1 }}`, want: []Variable{{Name: "a", Value: `{"a":1}`}}},
		{name: "array", entry: `{"key":"a","value":[ 1, 2 ]}`, want: []Variable{{Name: "a", Value: `[1,2]`}}},
		{name: "null", entry: `{"key":"a","value":null}`, want: []Variable{{Name: "a"}}, wantEmpty: 1},
		{name: "absent", entry: `{"key":"a"}`, want: []Variable{{Name: "a"}}, wantEmpty: 1},
		{name: "empty string", entry: `{"key":"a","value":""}`, want: []Variable{{Name: "a"}}, wantEmpty: 1},
		{name: "enabled false", entry: `{"key":"a","value":"x","enabled":false}`, wantDisabled: 1},
		{name: "disabled true", entry: `{"key":"a","value":"x","disabled":true}`, wantDisabled: 1},
		{name: "enabled absent", entry: `{"key":"a","value":"x"}`, want: []Variable{{Name: "a", Value: "x"}}},
		{name: "empty key", entry: `{"key":"","value":"x"}`,
			want: []Variable{{Name: "unnamed_1", Value: "x"}}, wantUnnamed: 1},
		{name: "non-scalar key", entry: `{"key":{},"value":"x"}`,
			want: []Variable{{Name: "unnamed_1", Value: "x"}}, wantUnnamed: 1},
		{name: "absent key", entry: `{"value":"x"}`,
			want: []Variable{{Name: "unnamed_1", Value: "x"}}, wantUnnamed: 1},
		{name: "null key", entry: `{"key":null,"value":"x"}`,
			want: []Variable{{Name: "unnamed_1", Value: "x"}}, wantUnnamed: 1},
		{name: "two unnamed", entry: `{"value":"a"},{"value":"b"}`,
			want: []Variable{{Name: "unnamed_1", Value: "a"}, {Name: "unnamed_2", Value: "b"}}, wantUnnamed: 2},
		{name: "disabled unnamed takes no number", entry: `{"value":"a","disabled":true},{"value":"b"}`,
			want: []Variable{{Name: "unnamed_1", Value: "b"}}, wantDisabled: 1, wantUnnamed: 1},
		{name: "generated key skips a real one", entry: `{"key":"unnamed_1","value":"a"},{"value":"b"}`,
			want: []Variable{{Name: "unnamed_1", Value: "a"}, {Name: "unnamed_2", Value: "b"}}, wantUnnamed: 1},
		{name: "description object", entry: `{"key":"a","value":"x","description":{"content":"d"}}`,
			want: []Variable{{Name: "a", Value: "x", Description: "d"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			arr := json.RawMessage("[" + tc.entry + "]")
			gotVars, got := decodeVariables(arr)
			if len(tc.want) == 0 {
				tc.want = []Variable{}
			}
			if diff := cmp.Diff(tc.want, gotVars); diff != "" {
				t.Fatalf("collection variables (-want +got):\n%s", diff)
			}
			if want := (variableCounts{tc.wantDisabled, tc.wantEmpty, tc.wantUnnamed}); got != want {
				t.Fatalf("collection counts = %+v; want %+v", got, want)
			}

			env := parseEnvironment(map[string]json.RawMessage{"values": arr})
			if diff := cmp.Diff(tc.want, env.Variables); diff != "" {
				t.Fatalf("environment variables (-want +got):\n%s", diff)
			}
			if got := (variableCounts{env.Disabled, env.Empty, env.Unnamed}); got != (variableCounts{tc.wantDisabled, tc.wantEmpty, tc.wantUnnamed}) {
				t.Fatalf("environment counts = %+v; want %d, %d, %d", got, tc.wantDisabled, tc.wantEmpty, tc.wantUnnamed)
			}
		})
	}
}
