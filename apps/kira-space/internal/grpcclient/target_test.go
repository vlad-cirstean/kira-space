package grpcclient

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestNormalizeTarget is F12's own measured-cases table, plus the no-port case — the highest-value
// test in the phase relative to its size (§6.2): every case is a real failure mode a user will
// hit, and the current-behaviour column is measured (the throwaway probe module), not assumed.
func TestNormalizeTarget(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		tlsRequested bool
		wantDial     string
		wantTLS      bool
		wantErr      string // substring, "" means no error
	}{
		{name: "bare host:port, plaintext requested", raw: "127.0.0.1:1", tlsRequested: false, wantDial: "127.0.0.1:1", wantTLS: false},
		{name: "https scheme overrides TLS toggle and strips", raw: "https://127.0.0.1:1", tlsRequested: false, wantDial: "127.0.0.1:1", wantTLS: true},
		{name: "http scheme overrides TLS toggle and strips", raw: "http://127.0.0.1:1", tlsRequested: true, wantDial: "127.0.0.1:1", wantTLS: false},
		{name: "grpc scheme strips, implies plaintext", raw: "grpc://127.0.0.1:1", tlsRequested: true, wantDial: "127.0.0.1:1", wantTLS: false},
		{name: "grpcs scheme strips, implies TLS", raw: "grpcs://127.0.0.1:1", tlsRequested: false, wantDial: "127.0.0.1:1", wantTLS: true},
		{name: "dns resolver scheme passes through untouched", raw: "dns:///127.0.0.1:1", tlsRequested: false, wantDial: "dns:///127.0.0.1:1", wantTLS: false},
		{name: "unix resolver scheme passes through untouched", raw: "unix:///tmp/nope.sock", tlsRequested: false, wantDial: "unix:///tmp/nope.sock", wantTLS: false},
		{name: "a path is refused, naming the offending part", raw: "127.0.0.1:1/some/path", tlsRequested: false, wantErr: `"/some/path"`},
		{name: "empty target is refused", raw: "", tlsRequested: false, wantErr: "a target is required"},
		{name: "whitespace-only target is refused", raw: "   ", tlsRequested: false, wantErr: "a target is required"},
		{name: "bare host, no port, TLS -> :443 default", raw: "api.example.com", tlsRequested: true, wantDial: "api.example.com:443", wantTLS: true},
		{name: "bare host, no port, plaintext -> refused", raw: "api.example.com", tlsRequested: false, wantErr: "a plaintext target needs an explicit port"},
		{name: "https scheme with no port -> :443 default", raw: "https://api.example.com", tlsRequested: false, wantDial: "api.example.com:443", wantTLS: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeTarget(c.raw, c.tlsRequested)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("NormalizeTarget(%q, %v) = %+v, want error containing %q", c.raw, c.tlsRequested, got, c.wantErr)
				}
				if gerr, ok := err.(*Error); !ok || gerr.Code != CodeBadRequest {
					t.Errorf("error = %v, want a *Error with code %s", err, CodeBadRequest)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeTarget(%q, %v) unexpected error: %v", c.raw, c.tlsRequested, err)
			}
			if got.Dial != c.wantDial || got.TLS != c.wantTLS {
				t.Errorf("NormalizeTarget(%q, %v) = %+v, want {Dial: %q, TLS: %v}", c.raw, c.tlsRequested, got, c.wantDial, c.wantTLS)
			}
		})
	}
}

// TestDialConn_CAFile covers the CA-file case (§6.2): a missing or unreadable CA file is refused
// with a legible E_BAD_REQUEST rather than reaching grpc-go at all.
func TestDialConn_CAFile(t *testing.T) {
	_, err := dialConn(context.Background(), "127.0.0.1:1", TLSConfig{Enabled: true, CAFile: filepath.Join(t.TempDir(), "missing.pem")})
	if err == nil {
		t.Fatal("dialConn with a missing CA file: want an error, got nil")
	}
	gerr, ok := err.(*Error)
	if !ok || gerr.Code != CodeBadRequest {
		t.Errorf("error = %v, want a *Error with code %s", err, CodeBadRequest)
	}
}
