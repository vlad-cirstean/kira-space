// Package credshape decides whether a name or a value looks like a credential.
package credshape

import (
	"regexp"
	"strings"
)

var (
	secretNameParts = []string{"token", "secret", "passw", "pwd", "apikey", "api_key", "auth", "credential", "bearer", "private", "cookie"}
	secretNameEnds  = []string{"_pat", "_key"}

	valuePrefixes = []string{"sk-", "ghp_", "gho_", "github_pat_", "glpat-", "AKIA", "AIza"}
	slackToken    = regexp.MustCompile(`^xox[abp]-`)
	jwt           = regexp.MustCompile(`^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*$`)
	hexValue      = regexp.MustCompile(`^[0-9a-fA-F]{32,}$`)
	b64Value      = regexp.MustCompile(`^[A-Za-z0-9_-]{32,}$`)
)

// Name reports whether a variable, header-less flag or query parameter name marks a credential.
func Name(name string) bool {
	n := strings.ReplaceAll(strings.ToLower(name), "-", "_")
	for _, p := range secretNameParts {
		if strings.Contains(n, p) {
			return true
		}
	}
	for _, e := range secretNameEnds {
		if strings.HasSuffix(n, e) {
			return true
		}
	}
	return false
}

// Value reports whether v looks like a credential on its own: a known prefix, a JWT, or a long
// hex or mixed-case base64url run.
func Value(v string) bool {
	for _, p := range valuePrefixes {
		if strings.HasPrefix(v, p) && len(v) > len(p) {
			return true
		}
	}
	if slackToken.MatchString(v) || jwt.MatchString(v) || hexValue.MatchString(v) {
		return true
	}
	return b64Value.MatchString(v) && strings.ContainsAny(v, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") &&
		strings.ContainsAny(v, "abcdefghijklmnopqrstuvwxyz") && strings.ContainsAny(v, "0123456789")
}
