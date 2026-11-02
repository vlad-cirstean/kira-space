package loginshell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/toolexec"
)

const (
	probeMarker  = "__KIRA_ENV_PROBE__"
	probeTimeout = 5 * time.Second
)

var probeNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ProbeEnv reads the named variables from the user's login shell (-l -i): a Finder-launched app lacks
// what the shell profile exports. A variable the shell leaves unset is absent from the result. With no
// login shell it returns an error, and the caller falls back to its own environment.
func ProbeEnv(ctx context.Context, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	var script strings.Builder
	script.WriteString(`printf '\n` + probeMarker + `'`)
	for _, n := range names {
		if !probeNameRe.MatchString(n) {
			return nil, fmt.Errorf("loginshell: invalid variable name %q", n)
		}
		script.WriteString(`; if [ -n "${` + n + `+x}" ]; then printf '1%s\0' "$` + n + `"; else printf '0\0'; fi`)
	}
	shell, login := ResolveShell(os.Getenv, IsExecutableFile)
	if !login {
		return nil, errors.New("loginshell: no login shell")
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := toolexec.RunIO(ctx, shell, []string{"-l", "-i", "-c", script.String()}, "", os.Environ(), nil)
	if err != nil {
		return nil, fmt.Errorf("loginshell: probe: %w", err)
	}
	i := bytes.LastIndex(out, []byte(probeMarker))
	if i < 0 {
		return nil, errors.New("loginshell: probe printed no marker")
	}
	parts := bytes.Split(out[i+len(probeMarker):], []byte{0})
	if len(parts) < len(names) {
		return nil, errors.New("loginshell: probe output is short")
	}
	res := make(map[string]string, len(names))
	for k, n := range names {
		if p := parts[k]; len(p) > 0 && p[0] == '1' {
			res[n] = string(p[1:])
		}
	}
	return res, nil
}
