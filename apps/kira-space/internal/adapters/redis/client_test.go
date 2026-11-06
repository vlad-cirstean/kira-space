package redis

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// require/prefer used to map to
// InsecureSkipVerify:true — a connection accepting any certificate, including an attacker's, with
// no indication anywhere in the UI, and with no native Redis convention (unlike libpq's "require")
// to justify it. require/prefer/verify-full now all verify, matching the Kafka adapter; an
// explicit verify-none/insecure value is the new escape hatch for anyone who genuinely needs the
// old behaviour.
func TestResolveFields_TLSModes(t *testing.T) {
	host := "redis.example.com"
	cfg := func(sslmode string) model.ResolvedConnectionConfig {
		return model.ResolvedConnectionConfig{
			Host:    &host,
			Options: map[string]any{"sslmode": sslmode},
		}
	}

	t.Run("disable leaves tlsConfig nil", func(t *testing.T) {
		fields, _, err := resolveFields(cfg("disable"), nil)
		if err != nil {
			t.Fatalf("resolveFields: %v", err)
		}
		if fields.tlsConfig != nil {
			t.Fatalf("tlsConfig = %+v, want nil for sslmode=disable", fields.tlsConfig)
		}
	})

	for _, mode := range []string{"require", "prefer", "verify-full"} {
		t.Run(mode+" verifies the certificate", func(t *testing.T) {
			fields, _, err := resolveFields(cfg(mode), nil)
			if err != nil {
				t.Fatalf("resolveFields: %v", err)
			}
			if fields.tlsConfig == nil {
				t.Fatal("tlsConfig is nil, want a verifying *tls.Config")
			}
			if fields.tlsConfig.InsecureSkipVerify {
				t.Fatalf("InsecureSkipVerify = true for sslmode=%s — certificate verification is disabled", mode)
			}
			if fields.tlsConfig.ServerName != host {
				t.Fatalf("ServerName = %q, want %q", fields.tlsConfig.ServerName, host)
			}
		})
	}

	for _, mode := range []string{"verify-none", "insecure"} {
		t.Run(mode+" is the explicit opt-out", func(t *testing.T) {
			fields, _, err := resolveFields(cfg(mode), nil)
			if err != nil {
				t.Fatalf("resolveFields: %v", err)
			}
			if fields.tlsConfig == nil || !fields.tlsConfig.InsecureSkipVerify {
				t.Fatalf("tlsConfig = %+v, want InsecureSkipVerify:true for sslmode=%s", fields.tlsConfig, mode)
			}
		})
	}
}

// P21 round 1 architecture/security finding 2: resolveFields never read the URI scheme, so
// rediss://... — the standard, universally documented way to spell a TLS Redis connection —
// silently produced a plaintext connection whenever no sslmode option happened to be set.
func TestResolveFields_RedissSchemeEnablesTLS(t *testing.T) {
	uri := "rediss://user:pw@redis.example.com:6380/0"
	fields, _, err := resolveFields(model.ResolvedConnectionConfig{Mode: "uri", URI: &uri}, nil)
	if err != nil {
		t.Fatalf("resolveFields: %v", err)
	}
	if fields.tlsConfig == nil {
		t.Fatal("tlsConfig is nil for a rediss:// URI — TLS was never attempted")
	}
	if fields.tlsConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = true for a bare rediss:// URI — should verify by default")
	}
	if fields.tlsConfig.ServerName != "redis.example.com" {
		t.Fatalf("ServerName = %q, want %q", fields.tlsConfig.ServerName, "redis.example.com")
	}
}

// A plain redis:// URI must stay plaintext with no sslmode option set — the rediss:// fix must
// not turn every URI into a TLS attempt.
func TestResolveFields_PlainRedisSchemeStaysPlaintext(t *testing.T) {
	uri := "redis://user:pw@redis.example.com:6379/0"
	fields, _, err := resolveFields(model.ResolvedConnectionConfig{Mode: "uri", URI: &uri}, nil)
	if err != nil {
		t.Fatalf("resolveFields: %v", err)
	}
	if fields.tlsConfig != nil {
		t.Fatalf("tlsConfig = %+v, want nil for a plain redis:// URI", fields.tlsConfig)
	}
}

// Every other URI-parsing adapter (clickhouse, mysqlfamily, sqlite, kafka)
// returns a connect error when url.Parse fails; resolveFields used to silently fall through with
// every field left at its zero value, which host/port then defaulted to localhost:6379 with no
// password — a malformed URI silently connected to whatever Redis is running on the user's own
// machine, unauthenticated, instead of surfacing the parse failure.
func TestResolveFields_UnparsableURIIsConnectError(t *testing.T) {
	for _, uri := range []string{
		"redis://host:port/0",  // non-numeric port
		"redis://ho st:6379/0", // unescaped space in host
		"redis://a:b:c/0",      // unbracketed multi-colon host, e.g. an IPv6 literal
	} {
		t.Run(uri, func(t *testing.T) {
			cfg := model.ResolvedConnectionConfig{Mode: "uri", URI: &uri}
			if _, _, err := resolveFields(cfg, nil); err == nil {
				t.Fatalf("resolveFields(%q) err = nil, want a connect error", uri)
			}
		})
	}
}
