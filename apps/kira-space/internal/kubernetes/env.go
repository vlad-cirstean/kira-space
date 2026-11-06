package kubernetes

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

const (
	envMarker       = "__KIRA_KUBE_ENV__"
	envProbeTimeout = 5 * time.Second
	envProbeScript  = `printf '\n__KIRA_KUBE_ENV__%s\0%s\0' "$PATH" "$KUBECONFIG"`
)

// Env is the environment the module resolves kubeconfig and credential plugins from.
type Env struct {
	Path       string
	Kubeconfig string
}

// shellEnv resolves Env once: a Finder-launched app has no KUBECONFIG and a PATH without Homebrew,
// so exec credential plugins would not be found. The user's login shell knows both.
type shellEnv struct {
	once sync.Once
	env  Env
}

func (s *shellEnv) get() Env {
	s.once.Do(func() { s.env = probeShellEnv() })
	return s.env
}

func probeShellEnv() Env {
	base := Env{Path: os.Getenv("PATH"), Kubeconfig: os.Getenv("KUBECONFIG")}
	shell, login := loginshell.ResolveShell(os.Getenv, loginshell.IsExecutableFile)
	if !login {
		return base
	}
	ctx, cancel := context.WithTimeout(context.Background(), envProbeTimeout)
	defer cancel()
	out, err := toolexec.RunIO(ctx, shell, []string{"-l", "-i", "-c", envProbeScript}, "", os.Environ(), nil)
	if err != nil {
		slog.Warn("shell environment probe failed", "scope", "kubernetes", "err", err)
		return base
	}
	i := bytes.LastIndex(out, []byte(envMarker))
	if i < 0 {
		slog.Warn("shell environment probe printed no marker", "scope", "kubernetes")
		return base
	}
	parts := bytes.Split(out[i+len(envMarker):], []byte{0})
	if len(parts) < 2 {
		return base
	}
	if p := string(parts[0]); p != "" {
		base.Path = p
	}
	if base.Kubeconfig == "" {
		base.Kubeconfig = string(parts[1])
	}
	return base
}

// resolveCommand finds a relative exec-plugin command on path. It returns "" when none matches.
func resolveCommand(command, path string) string {
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, command)
		if toolexec.IsExecutable(os.Stat, cand) {
			return cand
		}
	}
	return ""
}
