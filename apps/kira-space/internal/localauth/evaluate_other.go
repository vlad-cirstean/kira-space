//go:build !darwin || !cgo

package localauth

// evaluate_other.go covers every platform (and build) without a real LocalAuthentication shim —
// Linux dev/CI, and a darwin build compiled with CGO_ENABLED=0 (which never ships:
// build/darwin/Taskfile.yml pins CGO_ENABLED=1 for every darwin build task, but the combination
// must still type-check). Always unavailable, so Authorize always falls through to the
// in-app confirmation; New()'s own startup log line (localauth.go) is what makes a build that
// accidentally ships this way say so out loud, mirroring secrets/keyring_other.go's shape
// alongside cipher.go's own probe-and-log New().

// Evaluate is this platform's EvaluateFunc. Never actually called — Authorize only calls
// evaluate() once available() has answered true, and Available (below) never does.
func Evaluate(reason string) (Outcome, error) {
	return Unavailable, nil
}

// Available is this platform's AvailableFunc.
func Available() bool {
	return false
}
