package pprofserve

import "testing"

func TestAllowedHost(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1:6060":    true,
		"localhost:6060":    true,
		"[::1]:6060":        true,
		"evil.example:6060": false,
		"127.0.0.1:6061":    false,
		"127.0.0.1":         false,
		"":                  false,
	} {
		if got := allowedHost(host, "6060"); got != want {
			t.Errorf("allowedHost(%q) = %v, want %v", host, got, want)
		}
	}
}
