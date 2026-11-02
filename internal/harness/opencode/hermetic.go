package opencode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// Isolate fills dir for one hermetic TUI launch: a config home that drops the user's OpenCode
// config (harness.Isolator).
func (*Provider) Isolate(dir string, environ []string) error {
	_, err := redirectConfigHome(dir, environ)
	return err
}

// envWords are the K=V words and the names to unset that go on /usr/bin/env in front of opencode.
// They run after the login shell's profile, so an export there cannot undo them.
type envWords struct{ set, unset []string }

func (e envWords) empty() bool { return len(e.set) == 0 && len(e.unset) == 0 }

// quoted renders the words: `-u NAME` pairs first, then the assignments.
func (e envWords) quoted() []string {
	out := make([]string, 0, 2*len(e.unset)+len(e.set))
	for _, n := range e.unset {
		out = append(out, "-u", loginshell.Quote(n))
	}
	for _, kv := range e.set {
		out = append(out, loginshell.Quote(kv))
	}
	return out
}

// unsetLayers are the layerEnv names a profile export could still layer user config through: all
// except those the launch sets itself (an unset would remove Kira's own).
func unsetLayers(own []string) []string {
	var out []string
	for _, n := range layerEnv {
		if !slices.Contains(own, n) && n != hookConfigDirEnv {
			out = append(out, n)
		}
	}
	return out
}

// isolatedConfigHome is the redirected config home Isolate made in dir; "" when dir is "".
func isolatedConfigHome(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "xdg")
}

// hermeticWords is the env of a hermetic launch: the redirected config home, the hermetic env and
// the layer names to unset. xdg "" omits the redirect.
func hermeticWords(xdg string, own []string) envWords {
	w := envWords{unset: unsetLayers(own)}
	if xdg != "" {
		w.set = append(w.set, "XDG_CONFIG_HOME="+xdg)
	}
	w.set = append(w.set, hermeticEnv...)
	return w
}

// ownLayers are the layerEnv names a launch's env entries set.
func ownLayers(env []string) []string {
	var own []string
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if slices.Contains(layerEnv, name) {
			own = append(own, name)
		}
	}
	return own
}

var (
	profileXDGMu  sync.Mutex
	profileXDGKey string
	profileXDG    string
	profileXDGSet bool
)

// profileConfigHome is XDG_CONFIG_HOME as the login shell exports it: a GUI-launched app lacks a
// profile-only value. Probed once per process env value; "" when unset or no login shell.
func profileConfigHome() string {
	key := os.Getenv("XDG_CONFIG_HOME")
	profileXDGMu.Lock()
	defer profileXDGMu.Unlock()
	if profileXDGSet && profileXDGKey == key {
		return profileXDG
	}
	vals, _ := loginshell.ProbeEnv(context.Background(), []string{"XDG_CONFIG_HOME"})
	profileXDGKey, profileXDG, profileXDGSet = key, vals["XDG_CONFIG_HOME"], true
	return profileXDG
}

// redirectConfigHome makes a config home under dir that links every entry of the real one except
// OpenCode's own, so git and other tools keep their config while user MCP servers, the global
// AGENTS.md and other OpenCode config drop out. Login lives in the data dir and stays.
func redirectConfigHome(dir string, environ []string) (string, error) {
	real := ""
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "XDG_CONFIG_HOME="); ok {
			real = v
		}
	}
	if real == "" {
		real = profileConfigHome()
	}
	if real == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		real = filepath.Join(home, ".config")
	}
	target := filepath.Join(dir, "xdg")
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(real)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return target, nil
		}
		return "", err
	}
	for _, e := range entries {
		if e.Name() == "opencode" {
			continue
		}
		if err := os.Symlink(filepath.Join(real, e.Name()), filepath.Join(target, e.Name())); err != nil {
			return "", err
		}
	}
	return target, nil
}
