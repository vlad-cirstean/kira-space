// Package secrets is the Go analogue of src/main/secret-cipher.ts: one AES-256-GCM key held in the
// macOS Keychain (github.com/keybase/go-keychain), a kira:v3: envelope in the connections.password
// column, and the same per-platform SecretStorageStatus probe/reason strings the connection dialog
// already renders.
package secrets

import "github.com/kirathecat/kira-studio/packages/theme/src/i18n"

// Backend values mirror packages/shared/domain/secrets.ts's secretStorageStatusSchema.
const (
	BackendKeychain    = "keychain"
	BackendBasicText   = "basic_text"
	BackendUnavailable = "unavailable"
)

// Status mirrors secretStorageStatusSchema byte for byte — the shape
// apps/kira-studio/internal/bridge/connections.go carried as a P52-walking-skeleton stub; this is that
// struct, moved here unchanged.
type Status struct {
	Available        bool    `json:"available"`
	Backend          string  `json:"backend"`
	InsecureFallback bool    `json:"insecureFallback"`
	Reason           *string `json:"reason"`
}

func reason(s string) *string { return &s }

func darwinUnavailableReason() string {
	return i18n.T("backend.core.secrets.darwinUnavailable.body", nil)
}

func linuxUnavailableReason() string {
	return i18n.T("backend.core.secrets.linuxUnavailable.body", map[string]string{"variable": "KIRA_INSECURE_SECRETS"})
}

func otherPlatformReason() string {
	return i18n.T("backend.core.secrets.otherPlatformUnavailable.body", nil)
}
