// client_test.go is a pure unit test (no server) for buildConfig's sslmode handling.
package postgres

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func cfgWithSslmode(sslmode string) model.ResolvedConnectionConfig {
	host := "pg.example.com"
	return model.ResolvedConnectionConfig{
		Mode:    "fields",
		Host:    &host,
		Options: map[string]any{"sslmode": sslmode},
	}
}

// pgx.ParseConfig already resolves verify-ca correctly from a URI's own ?sslmode=, but this
// Options-driven override re-read the same value and hit its default: branch, refusing with
// "unknown sslmode" a config pgx had already gotten right — verify-ca is a standard libpq value
// (verify the CA chain, not the hostname), common for an internal CA or an IP-addressed host.
func TestBuildConfig_VerifyCA(t *testing.T) {
	cfg, err := buildConfig(cfgWithSslmode("verify-ca"), "", nil)
	if err != nil {
		t.Fatalf("buildConfig(verify-ca): %v", err)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("TLSConfig is nil for sslmode=verify-ca, want a hostname-skipping, chain-verifying config")
	}
	if !cfg.TLSConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = false for sslmode=verify-ca — the hostname check must be skipped")
	}
	if cfg.TLSConfig.VerifyPeerCertificate == nil {
		t.Fatal("VerifyPeerCertificate is nil for sslmode=verify-ca — chain verification must still run")
	}
}

// The other libpq sslmode values must keep behaving exactly as before this change.
func TestBuildConfig_ExistingSslmodesUnaffected(t *testing.T) {
	t.Run("disable leaves TLSConfig nil", func(t *testing.T) {
		cfg, err := buildConfig(cfgWithSslmode("disable"), "", nil)
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		if cfg.TLSConfig != nil {
			t.Fatalf("TLSConfig = %+v, want nil for sslmode=disable", cfg.TLSConfig)
		}
	})

	for _, mode := range []string{"require", "prefer"} {
		t.Run(mode+" encrypts without verifying (libpq convention)", func(t *testing.T) {
			cfg, err := buildConfig(cfgWithSslmode(mode), "", nil)
			if err != nil {
				t.Fatalf("buildConfig(%s): %v", mode, err)
			}
			if cfg.TLSConfig == nil || !cfg.TLSConfig.InsecureSkipVerify {
				t.Fatalf("TLSConfig = %+v for sslmode=%s, want InsecureSkipVerify:true", cfg.TLSConfig, mode)
			}
		})
	}

	t.Run("verify-full verifies the hostname", func(t *testing.T) {
		cfg, err := buildConfig(cfgWithSslmode("verify-full"), "", nil)
		if err != nil {
			t.Fatalf("buildConfig(verify-full): %v", err)
		}
		if cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify {
			t.Fatalf("TLSConfig = %+v for sslmode=verify-full, want a verifying config", cfg.TLSConfig)
		}
	})

	t.Run("an unrecognized sslmode still fails loudly", func(t *testing.T) {
		if _, err := buildConfig(cfgWithSslmode("bogus"), "", nil); err == nil {
			t.Fatal("buildConfig(bogus) returned nil error, want a connect error")
		}
	})
}

// TestBuildConfig_FallbacksClearedOnOverride: pgx.ParseConfig("")'s own default
// sslmode=prefer builds a TLS-primary + plaintext-fallback pair, each carrying its own Host/Port —
// buildConfig only ever overrides the primary, so pgconn.ConnectConfig's own retry-on-any-non-auth-
// error could otherwise silently fall through to an environment-derived host/port or a plaintext
// TLSConfig this override was never meant to allow. Every path that overrides Host/Port or
// TLSConfig must leave Fallbacks nil.
func TestBuildConfig_FallbacksClearedOnOverride(t *testing.T) {
	t.Run("fields mode with no sslmode override still clears Fallbacks (Host/Port overridden)", func(t *testing.T) {
		host := "pg.example.com"
		cfg, err := buildConfig(model.ResolvedConnectionConfig{Mode: "fields", Host: &host}, "", nil)
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		if cfg.Fallbacks != nil {
			t.Fatalf("Fallbacks = %+v, want nil once Host/Port were overridden", cfg.Fallbacks)
		}
	})

	for _, mode := range []string{"require", "prefer", "verify-full", "verify-ca"} {
		t.Run("sslmode="+mode+" clears Fallbacks", func(t *testing.T) {
			cfg, err := buildConfig(cfgWithSslmode(mode), "", nil)
			if err != nil {
				t.Fatalf("buildConfig(%s): %v", mode, err)
			}
			if cfg.Fallbacks != nil {
				t.Fatalf("Fallbacks = %+v for sslmode=%s, want nil once TLSConfig was overridden", cfg.Fallbacks, mode)
			}
		})
	}

	t.Run("uri mode with no override at all leaves Fallbacks untouched", func(t *testing.T) {
		uri := "postgres://u:p@pg.example.com:5432/db"
		before, err := buildConfig(model.ResolvedConnectionConfig{Mode: "uri", URI: &uri}, "", nil)
		if err != nil {
			t.Fatalf("buildConfig: %v", err)
		}
		reference, err := pgx.ParseConfig(uri)
		if err != nil {
			t.Fatalf("pgx.ParseConfig: %v", err)
		}
		if (before.Fallbacks == nil) != (reference.Fallbacks == nil) {
			t.Fatalf("Fallbacks presence changed with no override: got nil=%v, want nil=%v (this test only guards against buildConfig itself clearing Fallbacks when it made no override)",
				before.Fallbacks == nil, reference.Fallbacks == nil)
		}
	})
}

// selfSignedCA mints a CA cert plus one leaf certificate it actually signed, so
// VerifyPeerCertificate's chain-building can be exercised against a real (if tiny) PKI rather
// than asserting only that the field is non-nil.
func selfSignedCA(t *testing.T) (caDER, leafDER []byte) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err = x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "some-unrelated-hostname.internal"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err = x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}
	return caDER, leafDER
}

// TestVerifyChainSkipHostname exercises the real chain-building logic (not only that the field is
// non-nil) against a controlled root pool — buildConfig's own VerifyPeerCertificate always passes
// nil (the live system trust store), which a test cannot add a throwaway CA to, so this drives
// verifyChainSkipHostname directly with a pool containing the test CA instead.
func TestVerifyChainSkipHostname(t *testing.T) {
	t.Run("a leaf signed by a trusted CA chains successfully, hostname unchecked", func(t *testing.T) {
		caDER, leafDER := selfSignedCA(t)
		caCert, err := x509.ParseCertificate(caDER)
		if err != nil {
			t.Fatalf("parse CA cert: %v", err)
		}
		roots := x509.NewCertPool()
		roots.AddCert(caCert)

		// The leaf's CommonName ("some-unrelated-hostname.internal") deliberately does not match
		// any name buildConfig's host would carry — proving the hostname really isn't checked.
		if err := verifyChainSkipHostname([][]byte{leafDER}, roots); err != nil {
			t.Fatalf("verifyChainSkipHostname(trusted chain) = %v, want nil", err)
		}
	})

	t.Run("a certificate not signed by any trusted root fails to chain", func(t *testing.T) {
		untrustedRoots := x509.NewCertPool() // a real CA the leaf below was never signed by
		otherCADER, _ := selfSignedCA(t)
		otherCACert, err := x509.ParseCertificate(otherCADER)
		if err != nil {
			t.Fatalf("parse CA cert: %v", err)
		}
		untrustedRoots.AddCert(otherCACert)

		_, unrelatedLeafDER := selfSignedCA(t) // signed by a *different*, untrusted CA
		if err := verifyChainSkipHostname([][]byte{unrelatedLeafDER}, untrustedRoots); err == nil {
			t.Fatal("verifyChainSkipHostname(untrusted chain) = nil, want a chain-verification error")
		}
	})

	t.Run("no certificates presented is rejected", func(t *testing.T) {
		if err := verifyChainSkipHostname(nil, x509.NewCertPool()); err == nil {
			t.Fatal("verifyChainSkipHostname(no certs) = nil, want an error")
		}
	})
}
