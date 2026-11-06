//go:build darwin && cgo

package datagrip

import (
	"fmt"
	"unicode/utf8"

	"github.com/keybase/go-keychain"
)

// keychainSupported lets Scan (D9) classify a row's passwordOutlook without ever calling the
// query below — a compile-time platform fact, not an I/O probe.
const keychainSupported = true

// serviceNameForDataSource is F4's generateServiceName("DB", uuid) — CredentialAttributes.kt's
// exact format, U+2014 EM DASH with a regular space on each side, corroborated byte for byte by
// four independent implementations (F4).
func serviceNameForDataSource(uuid string) string {
	return fmt.Sprintf("IntelliJ Platform DB — %s", uuid)
}

// readKeychainPassword queries the macOS Keychain for uuid's data source password — the same
// query shape internal/secrets/keyring_darwin.go already uses, by service only with no
// account constraint, since DataGrip's own item may or may not carry one. This always raises
// the system's "…wants to access…" authorization panel the first time Kira Studio asks for a
// given item, so the scan step must never call this — only Apply does, once per selected row.
func readKeychainPassword(uuid string) (password, username string, err error) {
	service := serviceNameForDataSource(uuid)

	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(service)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnAttributes(true)
	query.SetReturnData(true)

	results, qerr := keychain.QueryItem(query)
	if qerr != nil {
		// F11: a denied or cancelled authorization panel surfaces here as a real Go error from
		// SecItemCopyMatching — never conflated with "no such item" (QueryItem's own contract
		// already reports that as (nil, nil), handled below).
		return "", "", &RefusalError{
			Code:    ReasonCredentialStoreLocked,
			Message: fmt.Sprintf("the macOS Keychain denied access to the item for %q: %s", service, qerr),
		}
	}
	if len(results) == 0 {
		return "", "", errPasswordNotFound
	}
	data := results[0].Data
	if len(data) == 0 {
		return "", "", errPasswordNotFound
	}
	// D12: a stored value that isn't valid UTF-8 is reported as unreadable, never as mojibake.
	if !utf8.Valid(data) {
		return "", "", &RefusalError{
			Code:    ReasonCredentialStoreUnsupported,
			Message: fmt.Sprintf("the stored value for %q is not readable text", service),
		}
	}
	return string(data), results[0].Account, nil
}
