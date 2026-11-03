package openedfiles

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ErrNotOpenable is OpenAt's error for a path that is not an existing, supported regular file.
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

// Openable reports per path whether it names an existing file Kira Space can show.
func (s *Service) Openable(cwd, home string, paths []string) []bool {
	out := make([]bool, len(paths))
	for i, p := range paths {
		abs, ok := TerminalPath(p, cwd, home)
		if !ok {
			continue
		}
		// Repo matching never changes the answer; nil repos skips its per-root symlink walks.
		_, err := Resolve(abs, nil)
		out[i] = err == nil
	}
	return out
}

// OpenAt opens abs, placing the cursor at line and col (zero for none), in a window of its own.
func (s *Service) OpenAt(abs string, line, col int) error {
	t, err := Resolve(abs, s.repos())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotOpenable, err)
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
