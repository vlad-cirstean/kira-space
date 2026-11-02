package secrets

import "testing"

func TestMasker(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		in     string
		want   string
	}{
		{"nil masks nothing", nil, "abc", "abc"},
		{"empty value ignored", []string{""}, "abc", "abc"},
		{"longest first on overlap", []string{"pass", "password1"}, "x password1 y pass", "x •••• y ••••"},
		{"multi-line value masks each line", []string{"line1\nline2"}, "got line2 then line1", "got •••• then ••••"},
		{"json-escaped form", []string{`a"b\c`}, `{"k":"a\"b\\c"} a"b\c`, `{"k":"••••"} ••••`},
		{"json-escaped newline", []string{"l1\nl2"}, `"l1\nl2"`, `"••••"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewMasker(tt.values...).Mask(tt.in); got != tt.want {
				t.Errorf("Mask(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
