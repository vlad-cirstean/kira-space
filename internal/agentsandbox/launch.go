package agentsandbox

import (
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// Flags are the agent permission flags that back the profile on every platform: the CLI refuses
// early with a clear message, and on Linux they are the only layer. The denials are the provider's.
func Flags(p Policy, agent harness.Sandboxed) (addDirs, disallowed []string) {
	addDirs = appendUnique(addDirs, p.Read...)
	addDirs = appendUnique(addDirs, p.Allow...)
	addDirs = appendUnique(addDirs, p.Scratch)
	if agent != nil {
		disallowed = agent.SandboxDenies(p.Access == AccessRead, p.Read, p.Network)
	}
	return addDirs, disallowed
}

// Env is the sandboxed run's environment overrides. proxyURL is set when the network is off.
func Env(p Policy, rt Runtime, proxyURL string) []string {
	cache := func(name string) string { return filepath.Join(p.Cache, name) }
	env := []string{
		"TMPDIR=" + filepath.Join(p.Scratch, "tmp"),
		"XDG_CACHE_HOME=" + cache("xdg"),
		"GOCACHE=" + cache("go-build"),
		"GOMODCACHE=" + cache("gomod"),
		"GOFLAGS=-modcacherw",
		"npm_config_cache=" + cache("npm"),
		"BUN_INSTALL_CACHE_DIR=" + cache("bun"),
		"PIP_CACHE_DIR=" + cache("pip"),
		"YARN_CACHE_FOLDER=" + cache("yarn"),
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0",
		"GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false",
	}
	if p.Access == AccessRead {
		env = append(env, "GIT_OPTIONAL_LOCKS=0")
	}
	if proxyURL != "" {
		env = append(env, "HTTPS_PROXY="+proxyURL, "HTTP_PROXY="+proxyURL, "https_proxy="+proxyURL, "http_proxy="+proxyURL,
			"NO_PROXY=localhost,127.0.0.1,::1")
		env = append(env, rt.Agent.QuietEnv...)
	}
	return env
}

// MergeEnv appends overrides to environ, dropping what they replace and the SSH agent socket.
func MergeEnv(environ, overrides []string) []string {
	drop := map[string]bool{"SSH_AUTH_SOCK": true}
	for _, kv := range overrides {
		k, _, _ := strings.Cut(kv, "=")
		drop[k] = true
	}
	out := make([]string, 0, len(environ)+len(overrides))
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			out = append(out, kv)
		}
	}
	return append(out, overrides...)
}

// ExecArgv is the argv prefix that runs a command under e's profile: the sandbox runner, then its profile.
// The env and unset half of e is MergeEnv's job.
func ExecArgv(e *harness.SandboxExec) []string {
	bin := e.Bin
	if bin == "" {
		bin = harness.DefaultSandboxBin
	}
	return []string{bin, "-f", e.Profile}
}
