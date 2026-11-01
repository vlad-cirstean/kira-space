//go:build darwin && !cgo

package secrets

import "errors"

// keyring_darwin.go is a cgo file (github.com/keybase/go-keychain requires it), so
// CGO_ENABLED=0 GOOS=darwin excludes it regardless of its own build tag — leaving
// loadOrCreateKey undefined and this package (and internal/connections, internal/bridge, which
// import it) uncompilable under a cgo-free cross-vet. This app never
// ships that way (build/darwin/Taskfile.yml pins CGO_ENABLED=1 for every darwin build task), but
// go vet/go build should still type-check the combination — reporting the same "not available"
// answer keyring_other.go's non-darwin stub reports, so probe() treats it identically either way.
func loadOrCreateKey() ([]byte, error) {
	return nil, errors.New("secrets: the Keychain is only available on darwin")
}
