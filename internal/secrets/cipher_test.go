package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/testx"
)

func fakeLoadOK() ([]byte, error) { return bytes.Repeat([]byte{0x01}, 32), nil }

func fakeLoadErr() ([]byte, error) { return nil, errors.New("no keychain") }

// TestProbe covers probe's three goos branches against the insecureEnv values a user might
// plausibly set — in particular P29 F5's regression: "0" and "false" must read as *off*, matching
// config.IsDev()'s own parsing convention for KIRA_DEV, not silently enable the insecure fallback.
func TestProbe(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		insecureEnv string
		loadKey     func() ([]byte, error)
		wantBackend string
	}{
		{"darwin ignores insecureEnv, key available", "darwin", "1", fakeLoadOK, BackendKeychain},
		{"darwin ignores insecureEnv, key unavailable", "darwin", "1", fakeLoadErr, BackendUnavailable},
		{"linux unset", "linux", "", fakeLoadOK, BackendUnavailable},
		{"linux empty stays unset", "linux", "", fakeLoadErr, BackendUnavailable},
		{`linux "0" is off`, "linux", "0", fakeLoadOK, BackendUnavailable},
		{`linux "false" is off`, "linux", "false", fakeLoadOK, BackendUnavailable},
		{`linux "1" is on`, "linux", "1", fakeLoadOK, BackendBasicText},
		{`linux any non-empty is on`, "linux", "yes", fakeLoadOK, BackendBasicText},
		{"other platform", "windows", "1", fakeLoadOK, BackendUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := probe(tt.goos, tt.insecureEnv, tt.loadKey)
			if status.Backend != tt.wantBackend {
				t.Errorf("probe(%q, %q) backend = %v, want %v", tt.goos, tt.insecureEnv, status.Backend, tt.wantBackend)
			}
			wantInsecure := tt.wantBackend == BackendBasicText
			if status.InsecureFallback != wantInsecure {
				t.Errorf("probe(%q, %q) InsecureFallback = %v, want %v", tt.goos, tt.insecureEnv, status.InsecureFallback, wantInsecure)
			}
		})
	}
}

func availableCipher(t *testing.T) *Cipher {
	t.Helper()
	status, key := probe("darwin", "", fakeLoadOK)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("build cipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("build GCM: %v", err)
	}
	return &Cipher{status: status, aead: aead}
}

// asIpcErr is testx.AsIpcErr (P107 I2-28).
var asIpcErr = testx.AsIpcErr

// TestEncryptUsesAFreshNoncePerCall is the property that makes the envelope safe to store: two
// encryptions of the same credential must never produce the same ciphertext, or a reused nonce
// leaks the plaintext relationship between two stored secrets.
func TestEncryptUsesAFreshNoncePerCall(t *testing.T) {
	c := availableCipher(t)
	const plain = "pässwörd 🔐"

	enc1, err := c.Encrypt(ScopeConnection, plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	enc2, err := c.Encrypt(ScopeConnection, plain)
	if err != nil {
		t.Fatalf("Encrypt again: %v", err)
	}
	if enc1 == enc2 {
		t.Error("two encryptions of the same value produced identical envelopes, want a fresh nonce")
	}
	if !strings.HasPrefix(enc1, "kira:v3:") {
		t.Errorf("Encrypt envelope = %q, want it to start with kira:v3:", enc1)
	}
	got, err := c.Decrypt(ScopeConnection, enc1)
	if err != nil || got != plain {
		t.Errorf("Decrypt = (%q, %v), want (%q, nil)", got, err, plain)
	}
}

// TestTamperDetectionFailsAuthentication is the property AES-GCM is chosen for: a stored
// credential whose nonce, ciphertext or auth tag has been altered — or that has simply been
// truncated — must fail authentication with a named error, never decrypt to garbage.
func TestTamperDetectionFailsAuthentication(t *testing.T) {
	c := availableCipher(t)
	enc, err := c.Encrypt(ScopeConnection, "hunter2")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(enc[len(envelopePrefix):])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	tamper := func(b []byte, i int) string {
		cp := append([]byte(nil), b...)
		cp[i] ^= 0xFF
		return envelopePrefix + base64.StdEncoding.EncodeToString(cp)
	}

	tests := []struct {
		name  string
		value string
	}{
		{"flip nonce byte", tamper(raw, 0)},
		{"flip ciphertext byte", tamper(raw, 20)},
		{"flip tag byte", tamper(raw, len(raw)-1)},
		{"truncated", envelopePrefix + base64.StdEncoding.EncodeToString(raw[:len(raw)-5])},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Decrypt(ScopeConnection, tt.value)
			ie := asIpcErr(t, err)
			if ie.Code != "E_SECRET_STORE" || !strings.Contains(ie.Message, "couldn’t be decrypted") {
				t.Errorf("Decrypt(%s) error = %+v, want the decrypt-failure sentence", tt.name, ie)
			}
		})
	}
}

// TestScopeBindsCiphertextToItsKind is the headline proof of P29: a ciphertext sealed under one
// scope must refuse to authenticate under any other. Every diagonal cell (same scope both sides)
// round-trips; every off-diagonal cell is refused.
func TestScopeBindsCiphertextToItsKind(t *testing.T) {
	c := availableCipher(t)
	scopes := []Scope{ScopeConnection, ScopeVariable, ScopeVariableHistory}

	for _, sealScope := range scopes {
		enc, err := c.Encrypt(sealScope, "s3cr3t")
		if err != nil {
			t.Fatalf("Encrypt(%s): %v", sealScope, err)
		}
		for _, openScope := range scopes {
			t.Run(string(sealScope)+"->"+string(openScope), func(t *testing.T) {
				got, err := c.Decrypt(openScope, enc)
				if sealScope == openScope {
					if err != nil || got != "s3cr3t" {
						t.Fatalf("Decrypt(%s) on a %s-sealed envelope = (%q, %v), want (%q, nil)", openScope, sealScope, got, err, "s3cr3t")
					}
					return
				}
				if err == nil {
					t.Fatalf("Decrypt(%s) on a %s-sealed envelope succeeded with %q, want a refusal", openScope, sealScope, got)
				}
				ie := asIpcErr(t, err)
				if ie.Code != "E_SECRET_STORE" || !strings.Contains(ie.Message, "couldn’t be decrypted") {
					t.Fatalf("Decrypt(%s) on a %s-sealed envelope error = %+v, want the decrypt-failure sentence", openScope, sealScope, ie)
				}
			})
		}
	}
}

// TestDecryptRefusesAnOlderEnvelopeEvenWithTheRightKey is the property that makes "no dual-read
// path" a fact the suite checks rather than an intention: a byte-perfect kira:v3: envelope, sealed
// under the very key the process holds, is refused the instant its prefix is downgraded to
// kira:v2: or kira:v1: — before any key or tag work runs at all, since the sentence is produced by
// the prefix check, not by a failed Open.
func TestDecryptRefusesAnOlderEnvelopeEvenWithTheRightKey(t *testing.T) {
	c := availableCipher(t)
	enc, err := c.Encrypt(ScopeConnection, "hunter2")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	body := enc[len(envelopePrefix):]

	for _, oldPrefix := range []string{"kira:v2:", "kira:v1:"} {
		t.Run(oldPrefix, func(t *testing.T) {
			downgraded := oldPrefix + body
			_, err := c.Decrypt(ScopeConnection, downgraded)
			ie := asIpcErr(t, err)
			if ie.Code != "E_SECRET_STORE" {
				t.Fatalf("Decrypt(%s) code = %s, want E_SECRET_STORE", oldPrefix, ie.Code)
			}
			if !strings.Contains(ie.Message, "must be entered again") {
				t.Errorf("Decrypt(%s) message = %q, want D6's wrong-envelope sentence", oldPrefix, ie.Message)
			}
		})
	}
}

// TestAnUnknownScopeIsRefusedByBothDirections turns D1's residual typo risk (an untyped string
// constant converts implicitly to Scope) into a loud, tested failure: an unrecognised scope is
// refused by Encrypt and by Decrypt, and Encrypt produces no envelope at all when refused.
func TestAnUnknownScopeIsRefusedByBothDirections(t *testing.T) {
	c := availableCipher(t)
	bad := []Scope{Scope("connections"), Scope("")}

	for _, scope := range bad {
		t.Run(string(scope), func(t *testing.T) {
			enc, err := c.Encrypt(scope, "s3cr3t")
			if err == nil {
				t.Fatalf("Encrypt(%q) succeeded with %q, want a refusal", scope, enc)
			}
			if enc != "" {
				t.Errorf("Encrypt(%q) envelope = %q, want empty on refusal", scope, enc)
			}
			ie := asIpcErr(t, err)
			if ie.Code != "E_CONTRACT" {
				t.Errorf("Encrypt(%q) code = %s, want E_CONTRACT", scope, ie.Code)
			}

			enc2, err := c.Encrypt(ScopeConnection, "s3cr3t")
			if err != nil {
				t.Fatalf("Encrypt(ScopeConnection) to build a fixture: %v", err)
			}
			_, err = c.Decrypt(scope, enc2)
			if err == nil {
				t.Fatalf("Decrypt(%q) succeeded, want a refusal", scope)
			}
			ie2 := asIpcErr(t, err)
			if ie2.Code != "E_CONTRACT" {
				t.Errorf("Decrypt(%q) code = %s, want E_CONTRACT", scope, ie2.Code)
			}
		})
	}
}

// TestScopeStringsAndEnvelopePrefixAreFrozen guards the storage format itself: each of these six
// literals is load-bearing for every secret already on disk under it, so a rename here must be a
// deliberate, visible diff — not an accidental refactor slipping through.
func TestScopeStringsAndEnvelopePrefixAreFrozen(t *testing.T) {
	if envelopePrefix != "kira:v3:" {
		t.Errorf("envelopePrefix = %q, want %q", envelopePrefix, "kira:v3:")
	}
	if ScopeConnection != "connection" {
		t.Errorf("ScopeConnection = %q, want %q", ScopeConnection, "connection")
	}
	if ScopeVariable != "variable" {
		t.Errorf("ScopeVariable = %q, want %q", ScopeVariable, "variable")
	}
	if ScopeVariableHistory != "variable-history" {
		t.Errorf("ScopeVariableHistory = %q, want %q", ScopeVariableHistory, "variable-history")
	}
	if ScopeMaskKey != "mask-key" {
		t.Errorf("ScopeMaskKey = %q, want %q", ScopeMaskKey, "mask-key")
	}
	if ScopeConnectionURI != "connection-uri" {
		t.Errorf("ScopeConnectionURI = %q, want %q", ScopeConnectionURI, "connection-uri")
	}
	if ScopeScriptParam != "script-param" {
		t.Errorf("ScopeScriptParam = %q, want %q", ScopeScriptParam, "script-param")
	}
}
