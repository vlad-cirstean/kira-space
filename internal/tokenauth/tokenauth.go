// Package tokenauth mints, hashes and verifies a bearer token: crypto/rand bytes encoded
// base64url on the wire, salted SHA-256 at rest, constant-time compare on verify. Both
// internal/pairing (device trust tokens) and apps/kira-space's own
// internal/mcpauth (DB-MCP bearer tokens) used this identical shape; each keeps
// its own persistence, TTL and dummy-comparison discipline around this.
package tokenauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

const (
	TokenBytes = 32
	SaltBytes  = 16
)

// Mint returns a fresh token: plain (base64url, what crosses the wire) and its salted hash and
// salt (what gets persisted). A short read or crypto/rand error is a hard failure — never a
// weaker token.
func Mint() (plain string, hash, salt []byte, err error) {
	tok := make([]byte, TokenBytes)
	if _, err := rand.Read(tok); err != nil {
		return "", nil, nil, err
	}
	salt = make([]byte, SaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", nil, nil, err
	}
	return base64.RawURLEncoding.EncodeToString(tok), Hash(tok, salt), salt, nil
}

// Hash returns sha256(salt‖tok).
func Hash(tok, salt []byte) []byte {
	sum := sha256.Sum256(append(append([]byte{}, salt...), tok...))
	return sum[:]
}

// Verify recomputes Hash(presented, salt) and compares against hash in constant time. An
// undecodable presented token fails like any other mismatch, after still doing the comparison so
// its cost doesn't itself leak information.
func Verify(presented string, hash, salt []byte) bool {
	tok, err := base64.RawURLEncoding.DecodeString(presented)
	if err != nil {
		tok = nil
	}
	return subtle.ConstantTimeCompare(Hash(tok, salt), hash) == 1
}
