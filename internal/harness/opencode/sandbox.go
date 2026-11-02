package opencode

import (
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// This file names OpenCode's home dirs and config files as sandbox paths. Kira never opens them.

// homeConfigDirs are the home dirs OpenCode reads and writes, config first (V4, verified).
var homeConfigDirs = []string{".config/opencode", ".local/share/opencode", ".local/state/opencode", ".cache/opencode"}

// protectFiles and protectDirs are the config entries inside the config dir that run code or hold
// instructions, so a sandboxed run never writes them (plugin dirs UNVERIFIED, U12).
var (
	protectFiles = []string{"opencode.json", "opencode.jsonc", "AGENTS.md"}
	protectDirs  = []string{"plugin", "plugins", "agent", "command", "mode"}
)

// SandboxNeeds: the install tree is two levels above bin/opencode for an npm-style install (the
// launcher is a symlink into the package), else the binary's folder.
func (*Provider) SandboxNeeds(home, bin string) harness.SandboxNeeds {
	n := harness.SandboxNeeds{QuietEnv: quietEnv}
	if bin != "" {
		if resolved, err := filepath.EvalSymlinks(bin); err == nil {
			bin = resolved
		}
		dir := filepath.Dir(bin)
		n.InstallPrefix = dir
		if filepath.Base(dir) == "bin" {
			n.InstallPrefix = filepath.Dir(dir)
		}
	}
	if home == "" {
		return n
	}
	for _, d := range homeConfigDirs {
		n.ConfigDirs = append(n.ConfigDirs, filepath.Join(home, filepath.FromSlash(d)))
	}
	n.ProtectFiles = protectFiles
	n.ProtectDirs = protectDirs
	return n
}

// SandboxDenies returns tool denials in Kira's vocabulary (Claude's names); RunHeadless translates
// them. Twin of claude's SandboxDenies: a shared helper across provider packages is not allowed.
func (*Provider) SandboxDenies(readOnly bool, readDirs []string, network bool) []string {
	var out []string
	if readOnly {
		out = append(out, "Edit", "Write", "NotebookEdit")
	} else {
		for _, r := range readDirs {
			out = append(out, "Edit(/"+filepath.ToSlash(r)+"/**)")
		}
	}
	for _, sub := range []string{"branch", "worktree", "checkout -b", "switch -c", "push", "fetch", "tag", "stash"} {
		out = append(out, "Bash(git "+sub+":*)")
	}
	if !network {
		out = append(out, "WebFetch", "WebSearch")
	}
	return out
}

// APIHosts: the model API suffixes only (U16); the base-URL variables of each provider are unknown.
func (*Provider) APIHosts(func(string) string) (exact, suffixes []string) {
	return nil, apiSuffixes
}
