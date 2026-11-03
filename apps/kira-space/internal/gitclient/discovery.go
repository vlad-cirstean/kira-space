package gitclient

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// RequiredVersion is the hard floor (docs/HISTORY.md (v1.3), "The user's own Git, at 2.38 or newer") —
// `git merge-tree --write-tree` (2.38) is what a later phase's conflict prediction needs to be
// exact rather than heuristic.
const RequiredVersion = "2.38.0"

// GitPathSettingID is the settings key GitStatus.SettingID names — `git-core`'s SETTINGS schema
// key verbatim (packages/git-core/src/settings/schema.ts), kept here as a literal rather than
// imported (Go cannot import TypeScript; OQ-2 defers real settings-surface integration).
const GitPathSettingID = "git.path"

// GitStatus is the blocking-state discriminant — the wire shape crossing as 'app.init's `git`
// field, structurally matching @kira/git-ipc's own GitStatus union (contract.ts) field-for-field;
// git-ui's GitBlockedPanel.vue/gitBlockedCopy.ts already render every non-"ok" Kind unchanged.
// Every field below is used by exactly one Kind — see gitBlockedCopy.ts's own switch for which.
type GitStatus struct {
	Kind      string   `json:"kind"` // "ok" | "notFound" | "tooOld" | "unusable"
	Path      string   `json:"path,omitempty"`
	Version   string   `json:"version,omitempty"`
	Probed    []string `json:"probed,omitempty"`
	Detected  string   `json:"detected,omitempty"`
	Required  string   `json:"required,omitempty"`
	SettingID string   `json:"settingId,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

// Locator resolves a candidate git binary path via one platform's own probe order, without
// running or validating it — Discovery.Status (below) is what runs --version and enforces
// RequiredVersion. Kept separate from Discovery so the per-platform strategy is a small,
// independently fakeable seam (darwinLocator's own test never has to run a real --version).
type Locator interface {
	// Locate returns the first usable candidate, and the full list of paths considered (in probe
	// order) either way — notFound's own Probed field is exactly this list on a miss.
	Locate(configuredPath string) (path string, probed []string, found bool)
}

// clToolsShim is the Xcode Command Line Tools' own git shim — the one path D3's own trap is
// about. It is filepath.Clean'd on both sides of every comparison below so a caller-supplied
// git.path of "/usr/bin/git/" or similar still matches.
const clToolsShim = "/usr/bin/git"

// darwinLocator implements the exact probe order: git.path setting -> PATH ->
// /opt/homebrew/bin -> /usr/local/bin -> /usr/bin/git. A PATH resolution that lands on the CL
// Tools shim is deliberately NOT accepted at the PATH step (unlike every other step, which
// accepts its candidate immediately) — it falls through to the final, xcode-select-gated step
// instead, so the shim is never treated as "found via PATH" without that gate having run. This is
// what keeps the probe order itself the single place that decides whether the shim gets spawned
// at all, rather than an incidental PATH hit bypassing the gate.
type darwinLocator struct {
	lookPath      func(string) (string, error)
	stat          func(string) (os.FileInfo, error)
	cltsInstalled func() bool
}

// NewDarwinLocator returns the real macOS Locator: exec.LookPath, os.Stat, and a real
// `xcode-select -p` probe for the Command Line Tools gate.
func NewDarwinLocator() Locator {
	return &darwinLocator{lookPath: exec.LookPath, stat: os.Stat, cltsInstalled: commandLineToolsInstalled}
}

// commandLineToolsInstalled runs `xcode-select -p` — not a git command, so this carries none of
// clToolsShim's own risk. Exit 0 means CL Tools (or a full Xcode install) is present; exit 2 means
// it is not. Any other failure (xcode-select itself missing, which would be unusual) is treated
// as "not installed" — the safe direction, since the alternative is spawning the shim blind.
func commandLineToolsInstalled() bool {
	return exec.Command("xcode-select", "-p").Run() == nil
}

func (l *darwinLocator) Locate(configuredPath string) (string, []string, bool) {
	var probed []string

	if configuredPath != "" {
		probed = append(probed, configuredPath)
		if toolexec.IsExecutable(l.stat, configuredPath) {
			return configuredPath, probed, true
		}
	}

	if resolved, err := l.lookPath("git"); err == nil && filepath.Clean(resolved) != clToolsShim {
		probed = append(probed, resolved)
		return resolved, probed, true
	}
	probed = append(probed, "git (on PATH)")

	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		candidate := filepath.Join(dir, "git")
		probed = append(probed, candidate)
		if toolexec.IsExecutable(l.stat, candidate) {
			return candidate, probed, true
		}
	}

	if l.cltsInstalled() {
		probed = append(probed, clToolsShim)
		if toolexec.IsExecutable(l.stat, clToolsShim) {
			return clToolsShim, probed, true
		}
	} else {
		probed = append(probed, clToolsShim+" (skipped: Xcode Command Line Tools not installed)")
	}

	return "", probed, false
}

// unsupportedLocator is D3's explicit non-macOS case: "Windows/Linux ... returning a 'platform
// not supported yet' error" rather than attempting (and silently misbehaving on) a probe order
// designed around macOS's own filesystem layout.
type unsupportedLocator struct{ platform string }

func (u unsupportedLocator) Locate(string) (string, []string, bool) {
	return "", []string{"git discovery is not implemented on " + u.platform + " yet"}, false
}

// NewPlatformLocator selects D3's named per-platform strategy on runtime.GOOS — only darwin is
// implemented (docs/HISTORY.md (v1.3), "macOS only").
func NewPlatformLocator() Locator {
	if runtime.GOOS == "darwin" {
		return NewDarwinLocator()
	}
	return unsupportedLocator{platform: runtime.GOOS}
}

// pathLocator is NewHostLocator's non-macOS probe: the configured path, then PATH.
type pathLocator struct{}

func (pathLocator) Locate(configuredPath string) (string, []string, bool) {
	var probed []string
	if configuredPath != "" {
		probed = append(probed, configuredPath)
		if toolexec.IsExecutable(os.Stat, configuredPath) {
			return configuredPath, probed, true
		}
	}
	if resolved, err := exec.LookPath("git"); err == nil {
		return resolved, append(probed, resolved), true
	}
	return "", append(probed, "git (on PATH)"), false
}

// NewHostLocator is the locator for builds that run real git off macOS, the -tags server sandbox
// build and the flow-test harness: the macOS probe order on darwin, the configured path then PATH
// elsewhere. Never used by a shipped build.
func NewHostLocator() Locator {
	if runtime.GOOS == "darwin" {
		return NewDarwinLocator()
	}
	return pathLocator{}
}

// parseGitVersion extracts the dotted version token from `git --version`'s stdout — "git version
// 2.42.0" or "git version 2.42.0 (Apple Git-135)" on macOS. strings.Fields is the whole parser
// (§0.2: anything more than this is P2's porcelain-parsing territory, not P1's).
func parseGitVersion(output string) (string, bool) {
	fields := strings.Fields(output)
	if len(fields) < 3 || fields[0] != "git" || fields[1] != "version" {
		return "", false
	}
	return fields[2], true
}

// versionTriple parses the leading digits of up to the first three dot-separated segments,
// tolerating a trailing non-numeric suffix a real build sometimes carries (e.g. "2.38.0.windows.1"
// truncates cleanly at the third segment; "2.38.0-rc1"'s third segment reads its leading "0").
func versionTriple(v string) [3]int {
	var out [3]int
	segments := strings.SplitN(v, ".", 4)
	for i := 0; i < 3 && i < len(segments); i++ {
		digits := segments[i]
		for j, r := range digits {
			if r < '0' || r > '9' {
				digits = digits[:j]
				break
			}
		}
		if n, err := strconv.Atoi(digits); err == nil {
			out[i] = n
		}
	}
	return out
}

// versionLess reports whether a < b as dotted-numeric versions.
func versionLess(a, b string) bool {
	ta, tb := versionTriple(a), versionTriple(b)
	for i := 0; i < 3; i++ {
		if ta[i] != tb[i] {
			return ta[i] < tb[i]
		}
	}
	return false
}

// discoveryTTL bounds how long a resolved GitStatus is trusted before Status re-probes — long
// enough that app.init and a repo.open moments later share one probe (the version check is a
// real spawn), short enough that installing/upgrading git and switching back to this app picks it
// up within a session without a restart.
const discoveryTTL = 30 * time.Second

// Discovery is D3 end to end: Locate a candidate, run --version on it, and classify the result
// into GitStatus — cached per configuredPath for discoveryTTL via Clock (D2), so a fake clock can
// prove both "cached within the window" and "re-probed after it" without a real sleep.
type Discovery struct {
	locator Locator
	runner  Runner
	clock   Clock

	mu               sync.Mutex
	cachedAt         time.Time
	cachedConfigured string
	cachedHasValue   bool
	cached           GitStatus
}

// NewDiscovery constructs a Discovery over the given Locator/Runner/Clock — all three D2 seams,
// so a test can fake any combination independently.
func NewDiscovery(locator Locator, runner Runner, clock Clock) *Discovery {
	return &Discovery{locator: locator, runner: runner, clock: clock}
}

// Status resolves and classifies git for configuredPath (the git.path setting's current value,
// "" meaning "use discovery" — OQ-2), reusing a cached result within discoveryTTL.
func (d *Discovery) Status(ctx context.Context, configuredPath string) GitStatus {
	d.mu.Lock()
	if d.cachedHasValue && d.cachedConfigured == configuredPath &&
		d.clock.Now().Sub(d.cachedAt) < discoveryTTL {
		cached := d.cached
		d.mu.Unlock()
		return cached
	}
	d.mu.Unlock()

	status := d.probe(ctx, configuredPath)

	// A disconnect reliably cancels an in-flight repo.open's ctx, where it
	// used to usually race and lose, which made this bug deterministic rather than latent. A
	// caller-cancelled ctx kills the --version probe mid-flight, and probe() used to have no way
	// to tell that apart from "git itself is unusable" — so it got cached process-wide for
	// discoveryTTL, poisoning every OTHER connection's app.init/repo.open for up to 30s over an
	// event that says nothing about git at all. Skip the cache write when the CALLER's own ctx
	// (not probeCtx's own 5s sub-timeout, which is a real "git took too long" signal worth
	// caching) was cancelled out from under this probe — the next caller, with its own fresh ctx,
	// deserves a real probe, not this one's leftovers.
	if errors.Is(ctx.Err(), context.Canceled) {
		return status
	}

	d.mu.Lock()
	d.cached = status
	d.cachedAt = d.clock.Now()
	d.cachedConfigured = configuredPath
	d.cachedHasValue = true
	d.mu.Unlock()
	return status
}

// versionProbeTimeout bounds the --version probe: the caller's own ctx has no
// deadline of its own (a request context is undeadlined), so without this a wedged
// binary — the exact macOS Command Line Tools trap darwinLocator's own gate exists to route
// around — would hang app.init for as long as the extension is willing to wait.
const versionProbeTimeout = 5 * time.Second

func (d *Discovery) probe(ctx context.Context, configuredPath string) GitStatus {
	path, probed, found := d.locator.Locate(configuredPath)
	if !found {
		return GitStatus{Kind: "notFound", Probed: probed}
	}

	probeCtx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()
	res, err := Run(probeCtx, d.runner, path, Spec{Args: []string{"--version"}, ReadOnly: true})
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
		return GitStatus{Kind: "unusable", Path: path, Reason: "git --version did not respond within 5s"}
	}
	// The caller's own ctx (not probeCtx's 5s sub-timeout above) was cancelled mid-probe — the
	// killed process's exit code/error below says nothing about whether git itself works, so
	// don't report it as though it did (Status's own caller-cancelled check is what keeps this
	// out of the 30s cache; this just keeps the immediate Reason honest for anyone who logs it).
	if errors.Is(ctx.Err(), context.Canceled) {
		return GitStatus{Kind: "unusable", Path: path, Reason: "the request was cancelled"}
	}
	if err != nil {
		return GitStatus{Kind: "unusable", Path: path, Reason: err.Error()}
	}
	if res.ExitCode != 0 {
		return GitStatus{Kind: "unusable", Path: path, Reason: strings.TrimSpace(string(res.Stderr))}
	}
	version, ok := parseGitVersion(string(res.Stdout))
	if !ok {
		return GitStatus{
			Kind: "unusable", Path: path,
			Reason: "could not parse `git --version` output: " + strings.TrimSpace(string(res.Stdout)),
		}
	}
	if versionLess(version, RequiredVersion) {
		return GitStatus{
			Kind: "tooOld", Path: path, Detected: version, Required: RequiredVersion,
			SettingID: GitPathSettingID,
		}
	}
	return GitStatus{Kind: "ok", Path: path, Version: version}
}
