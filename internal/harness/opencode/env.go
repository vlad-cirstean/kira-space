package opencode

import (
	"slices"
	"strings"
)

// ScrubEnv drops the variables that tie a child to an OpenCode session running this process.
func (*Provider) ScrubEnv(environ []string) []string { return dropEnv(environ, sessionEnv) }

// dropEnv removes every KEY=value entry whose key is in names.
func dropEnv(environ, names []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(names, name) {
			out = append(out, kv)
		}
	}
	return out
}
