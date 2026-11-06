package postman

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseFile(t *testing.T) {
	open := func(name string) string {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	tests := []struct {
		name    string
		in      string
		wantErr string
		want    *Environment
		wantCol string
	}{
		{name: "collection v2.0 refused", in: open("v2_0.json"), wantErr: "v2.0.0"},
		{name: "collection classified", in: open("no_schema.json"), wantCol: "set"},
		{
			name: "environment",
			in:   open("environment.json"),
			want: &Environment{
				Name: "Staging",
				Variables: []Variable{
					{Name: "host", Value: "https://staging.example.test", Type: "default"},
					{Name: "token", Value: "s3cret", Secret: true, Type: "secret"},
					{Name: "retries", Value: "3"},
					{Name: "implicit", Value: "on"},
					{Name: "config", Value: `{"retry":[1,2]}`, Type: "any"},
					{Name: "blank", Type: "default"},
				},
				Disabled: 1,
				Empty:    1,
			},
		},
		{name: "globals refused", in: open("globals.json"), wantErr: "globals export is not supported"},
		{name: "not postman", in: `{"hello":"world"}`, wantErr: "not a Postman collection or environment"},
		{
			name: "environment api wrapper",
			in:   `{"environment":{"name":"W","values":[{"key":"a","value":{"x":1}},{"key":"b"}]}}`,
			want: &Environment{
				Name:      "W",
				Variables: []Variable{{Name: "a", Value: `{"x":1}`}, {Name: "b"}},
				Empty:     1,
			},
		},
		{name: "collection api wrapper", in: `{"collection":{"info":{"name":"C"},"item":[]}}`, wantCol: "C"},
		{name: "collection wins over environment", in: `{"collection":{"info":{"name":"C"}},"environment":{"values":[]}}`, wantCol: "C"},
		{name: "plain collection ignores collection member", in: `{"info":{"name":"P"},"collection":{"info":{"name":"X"}}}`, wantCol: "P"},
		{name: "collection member not an object", in: `{"collection":"x"}`, wantErr: "not a Postman collection or environment"},
		{name: "wrapper without info", in: `{"collection":{"item":[]}}`, wantErr: "not a Postman collection or environment"},
		{name: "wrapper two levels refused", in: `{"collection":{"collection":{"info":{}}}}`, wantErr: "not a Postman collection or environment"},
		{
			name: "blank name falls back",
			in:   `{"name":"  ","values":[]}`,
			want: &Environment{Name: "Imported environment", Variables: []Variable{}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := ParseFile(strings.NewReader(tc.in))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantCol != "" {
				if f.Collection == nil || f.Environment != nil {
					t.Fatalf("File = %+v, want a collection", f)
				}
				if f.Collection.Name != tc.wantCol && tc.wantCol != "set" {
					t.Fatalf("collection name = %q, want %q", f.Collection.Name, tc.wantCol)
				}
				return
			}
			if f.Collection != nil {
				t.Fatalf("File has a collection, want an environment")
			}
			if diff := cmp.Diff(tc.want, f.Environment); diff != "" {
				t.Fatalf("environment (-want +got):\n%s", diff)
			}
		})
	}
}
