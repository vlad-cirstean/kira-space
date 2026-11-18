package openedfiles

import (
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ErrNotOpenable is OpenTerminalPath's error for a path that is not an existing, supported regular file.
var ErrNotOpenable = errors.New("not an openable file")

// TerminalPath turns a path printed in a terminal into an absolute one: `~` and `~/x` expand
// against home, a relative path joins cwd. `~user` forms and a relative path with no cwd give false.
func TerminalPath(raw, cwd, home string) (string, bool) {
	switch {
	case raw == "":
		return "", false
	case raw == "~" || strings.HasPrefix(raw, "~/"):
		if home == "" {
			return "", false
		}
		return filepath.Join(home, raw[1:]), true
	case strings.HasPrefix(raw, "~"):
		return "", false
	case filepath.IsAbs(raw):
		return filepath.Clean(raw), true
	case cwd == "":
		return "", false
	default:
		return filepath.Join(cwd, raw), true
	}
}

// terminalCandidates lists the absolute paths raw may name, in try order: raw against each cwd,
// then, for a relative raw with a git diff `a/` or `b/` prefix, the stripped path against each cwd.
func terminalCandidates(raw string, cwds []string, home string) []string {
	var out []string
	add := func(r string) {
		if len(cwds) == 0 {
			cwds = []string{""}
		}
		for _, cwd := range cwds {
			if abs, ok := TerminalPath(r, cwd, home); ok && !slices.Contains(out, abs) {
				out = append(out, abs)
			}
		}
	}
	add(raw)
	if strings.HasPrefix(raw, "a/") || strings.HasPrefix(raw, "b/") {
		add(raw[2:])
	}
	return out
}

// firstOpenable resolves the first candidate of raw that names an openable file.
func firstOpenable(repos []model.CodeRepo, raw string, cwds []string, home string) (Target, string, bool) {
	for _, c := range terminalCandidates(raw, cwds, home) {
		if t, abs, err := resolve(c, repos, true); err == nil {
			return t, abs, true
		}
	}
	return Target{}, "", false
}

// Openable returns per path the absolute path of the file it names, or "" when none is openable.
// cwds are tried in order.
func (s *Service) Openable(cwds []string, home string, paths []string) []string {
	repos := s.repos()
	out := make([]string, len(paths))
	for i, p := range paths {
		if _, abs, ok := firstOpenable(repos, p, cwds, home); ok {
			out[i] = abs
		}
	}
	return out
}

// OpenTerminalPath opens the file raw names, placing the cursor at line and col (zero for none), in
// a window of its own.
func (s *Service) OpenTerminalPath(raw string, cwds []string, home string, line, col int) error {
	t, _, ok := firstOpenable(s.repos(), raw, cwds, home)
	if !ok {
		return ErrNotOpenable
	}
	t.Line, t.Col = line, col
	s.deliver([]Target{t})
	return nil
}

func (s *Service) repos() []model.CodeRepo {
	repos, err := s.Repos.List()
	if err != nil {
		slog.Warn("opened files: list repos; opening as standalone files", "scope", "openedfiles", "err", err)
		return nil
	}
	return repos
}
