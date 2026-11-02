//go:build !darwin

package secrets

import "errors"

// loadOrCreateKey has no non-darwin implementation — the Keychain probe always fails on other
// platforms, which is exactly the {available:false} status probe() reports for them. This
// symbol exists purely so cipher.go can call it on every platform with no runtime.GOOS guard
// around the import of github.com/keybase/go-keychain, which must never be imported outside a
// darwin-tagged file (that package still compiles, but is empty of anything useful, on Linux).
func loadOrCreateKey() ([]byte, error) {
	return nil, errors.New("secrets: the Keychain is only available on darwin")
}
