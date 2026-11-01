package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// envelopePrefix bumps to kira:v2: the cipher genuinely changes (AES-256-GCM under our own key,
// vs Chromium safeStorage's AES-128-CBC), so reusing kira:v1: would let a v1 value reach a v2
// decrypt and fail confusingly.
//
// It bumps again, to kira:v3:: Encrypt/Decrypt now take a Scope, and that scope becomes AES-GCM's
// additional authenticated data — a kira:v2: blob would fail to authenticate under the new AAD
// regardless, so the prefix bump exists to turn that failure into a clear "re-enter it" message
// instead of a generic tag mismatch. There is no dual-read path: every kira:v2: (and kira:v1:)
// value is refused, by design — see docs/HISTORY.md.
const envelopePrefix = "kira:v3:"

// insecureKeyMaterial is a hardcoded compile-time constant, deliberately: the Linux
// development fallback has the same threat model and the same honesty as Chromium's
// basic_text — obfuscation under a key anyone can read, not encryption. A file-backed keyring
// would look more secure than it is and would need a passphrase prompt in a headless container.
const insecureKeyMaterial = "kira-studio:v2:insecure-development-key"

// Cipher is the Go analogue of secret-cipher.ts's SecretCipher. It never fails to construct: an
// unavailable backend is a valid Cipher whose Encrypt/Decrypt refuse with E_SECRET_STORE.
type Cipher struct {
	status Status
	aead   cipher.AEAD // nil when status.Available is false
}

// New probes once and returns a Cipher whose Status never changes for the life of the process
// (secret-cipher.ts's createSecretCipher, called once after startup with no Electron
// app.whenReady() analogue needed — P52 §6.5).
func New() *Cipher {
	status, key := probe(runtime.GOOS, os.Getenv("KIRA_INSECURE_SECRETS"), loadOrCreateKey)

	suffix := ""
	if status.InsecureFallback {
		suffix = " — Linux development fallback (KIRA_INSECURE_SECRETS=1): credentials are obfuscated with a hardcoded key, not a real keychain"
	}
	if status.InsecureFallback {
		slog.Warn("secret storage", "scope", "secrets", "backend", status.Backend, "available", status.Available, "note", suffix)
	} else {
		slog.Info("secret storage", "scope", "secrets", "backend", status.Backend, "available", status.Available)
	}

	c := &Cipher{status: status}
	if status.Available {
		block, err := aes.NewCipher(key)
		if err != nil {
			// The only ways aes.NewCipher can fail are a wrong key length, which both key
			// sources here always produce (32 bytes) — this is a real invariant violation, not
			// a recoverable runtime condition.
			panic(fmt.Sprintf("secrets: invalid key length: %v", err))
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			panic(fmt.Sprintf("secrets: invalid GCM configuration: %v", err))
		}
		c.aead = aead
	}
	return c
}

func (c *Cipher) Status() Status { return c.status }

// Encrypt satisfies repos.Cipher.
//
// Scope becomes AES-GCM's additional
// authenticated data (Scope.aad), so a ciphertext sealed for one kind of secret refuses to
// authenticate anywhere but a column holding that same kind — an attacker who can write
// ~/.kira-studio/kira.db but cannot read the Keychain item can no longer move a
// connections.password ciphertext into an api_variables.secret_value row and get the user to
// decrypt it through the lower-friction variables Reveal gate.
//
// The AAD is bound to *kind*, not to a row: ConnectionsRepo.InsertDuplicateWithSecret and
// VariablesRepo.DuplicateEnvironment both copy a raw ciphertext column into a new row of the same
// kind (connection → connection, variable → variable) without ever touching the cipher, and that
// must keep working — a per-row AAD would break it outright (would need a decrypt/re-encrypt on
// every duplicate, requiring the OS key to be available just to copy a row). VariablesRepo's own
// recordHistory is the one place that copies *across* kinds (a live api_variables.secret_value
// blob into api_variable_history.secret_value) — it does not use this raw-copy pattern, and instead
// re-seals under ScopeVariableHistory using the plaintext it already holds, because that copy is
// genuinely cross-kind.
//
// kira:v2: (and kira:v1:) values are refused with no migration, by explicit decision — see
// docs/HISTORY.md.
func (c *Cipher) Encrypt(scope Scope, plain string) (string, error) {
	if !scope.valid() {
		return "", ipcerr.Contract("%q is not a known secret scope", scope)
	}
	if !c.status.Available {
		return "", ipcerr.SecretStore(*c.status.Reason)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", ipcerr.T(ipcerr.CodeSecretStore, "backend.core.secrets.nonceFailed.error", map[string]string{"detail": err.Error()})
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plain), scope.aad())
	return envelopePrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt satisfies repos.Cipher.
func (c *Cipher) Decrypt(scope Scope, stored string) (string, error) {
	if !scope.valid() {
		return "", ipcerr.Contract("%q is not a known secret scope", scope)
	}
	if !strings.HasPrefix(stored, envelopePrefix) {
		// P52 §6.4 retires the pre-P25 plaintext passthrough: a non-enveloped value is an error
		// naming the problem, never returned verbatim.
		return "", ipcerr.T(ipcerr.CodeSecretStore, "backend.core.secrets.unknownEnvelope.body", nil)
	}
	if !c.status.Available {
		return "", ipcerr.SecretStore(*c.status.Reason)
	}

	raw, err := base64.StdEncoding.DecodeString(stored[len(envelopePrefix):])
	if err != nil {
		return "", decryptFailure(err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", decryptFailure(fmt.Errorf("ciphertext too short"))
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plain, err := c.aead.Open(nil, nonce, sealed, scope.aad())
	if err != nil {
		return "", decryptFailure(err)
	}
	return string(plain), nil
}

func decryptFailure(err error) error {
	return ipcerr.T(ipcerr.CodeSecretStore, "backend.core.secrets.decryptFailed.error", map[string]string{"detail": err.Error()})
}

// probe is New's whole platform switch, with the OS key source injected so every branch is
// testable off its own platform (P55 §2 D3) — the darwin key source itself stays a real Keychain
// call in production; only this function's caller decides which loadKey to pass.
func probe(goos, insecureEnv string, loadKey func() ([]byte, error)) (Status, []byte) {
	switch goos {
	case "darwin":
		key, err := loadKey()
		if err != nil {
			return Status{Available: false, Backend: BackendUnavailable, Reason: reason(darwinUnavailableReason())}, nil
		}
		return Status{Available: true, Backend: BackendKeychain}, key
	case "linux":
		if insecureEnv != "" && insecureEnv != "0" && insecureEnv != "false" {
			sum := sha256.Sum256([]byte(insecureKeyMaterial))
			return Status{Available: true, Backend: BackendBasicText, InsecureFallback: true}, sum[:]
		}
		return Status{Available: false, Backend: BackendUnavailable, Reason: reason(linuxUnavailableReason())}, nil
	default:
		return Status{Available: false, Backend: BackendUnavailable, Reason: reason(otherPlatformReason())}, nil
	}
}
