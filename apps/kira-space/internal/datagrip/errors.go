// Package datagrip imports connections (and their saved passwords) from a JetBrains DataGrip
// project directory: parsing .idea/dataSources.xml and dataSources.local.xml, locating the
// IntelliJ Platform PasswordSafe credential store, and reading a data source's password from the
// macOS Keychain — the only backend this package supports.
package datagrip

// The closed enum of skip/refusal reasons and report warnings — rendered by the UI, counted in
// the import report. Every one of these leaves the *connection* importable and only drops the
// *password* (or, for the four Reason* codes above the line, drops the whole row) — never a
// silent wrong password, never an unexplained miss.
const (
	ReasonUnsupportedEngine          = "unsupported-engine"
	ReasonUnrepresentableURL         = "unrepresentable-url"
	ReasonSQLitePathNotAbsolute      = "sqlite-path-not-absolute"
	ReasonNoJDBCURL                  = "no-jdbc-url"
	ReasonPasswordNotSaved           = "password-not-saved"
	ReasonPasswordNotFound           = "password-not-found"
	ReasonCredentialStoreNotFound    = "credential-store-not-found"
	ReasonCredentialStoreUnsupported = "credential-store-unsupported"
	ReasonCredentialStoreLocked      = "credential-store-locked"
	ReasonSecretStorageUnavailable   = "secret-storage-unavailable"
	WarnNameTruncated                = "name-truncated"
	WarnSSHTunnelDropped             = "ssh-tunnel-dropped"
)

// RefusalError is D12's "every uncertainty fails loudly" contract: a Code from the enum above,
// naming the case, plus a Message naming the file/path/version — never a bare fallthrough, never
// an empty-string password treated as success, never a `_ = err`.
type RefusalError struct {
	Code    string
	Message string
}

func (e *RefusalError) Error() string { return e.Message }

// errPasswordNotFound is D12's last bullet and case 18/19: a clean miss (a readable store with no
// entry for this uuid, or an entry whose password is the empty string) — distinguishable from
// every decode/decrypt failure above it, which all report ReasonCredentialStoreUnsupported instead.
// Consumed only by keychain_darwin.go (darwin && cgo) — invisible to a default-target lint run.
//
//nolint:unused // build-tag-only consumer, see above
var errPasswordNotFound = &RefusalError{
	Code:    ReasonPasswordNotFound,
	Message: "no password is stored for this data source",
}
