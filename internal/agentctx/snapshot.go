package agentctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

const (
	snapshotName = "AGENTS.md"
	snapshotKeep = 30 * 24 * time.Hour
)

// ContextFile returns the path of an immutable copy of the context text in effect, "" when there is
// none (no file, blank, or invalid with no last good text). Launches pass this path, never the live
// file: a save during a launch cannot change what that launch reads. A write error is returned, so a
// launch fails rather than start without its context.
func (s *Store) ContextFile() (string, error) {
	e, ok := s.folder.Get(File)
	if !ok || e.Value == nil || strings.TrimSpace(*e.Value) == "" {
		return "", nil
	}
	text := []byte(*e.Value)
	dir := filepath.Join(s.o.SnapshotDir, configfs.TextHash(text)[:16])
	path := filepath.Join(dir, snapshotName)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	fresh := false
	if _, err := os.Stat(path); err != nil {
		if err := configfs.WriteAtomic(path, text, 0o600); err != nil {
			return "", fmt.Errorf("agentctx: write snapshot: %w", err)
		}
		fresh = true
	} else if err := os.Chtimes(path, now, now); err != nil {
		return "", fmt.Errorf("agentctx: touch snapshot: %w", err)
	}
	if fresh || !s.pruned {
		s.pruned = true
		s.prune(filepath.Base(dir), now)
	}
	return path, nil
}

// prune drops snapshot dirs unused for 30 days, except keep. Best effort.
func (s *Store) prune(keep string, now time.Time) {
	entries, err := os.ReadDir(s.o.SnapshotDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == keep {
			continue
		}
		info, err := os.Stat(filepath.Join(s.o.SnapshotDir, e.Name(), snapshotName))
		if err != nil {
			info, err = e.Info()
			if err != nil {
				continue
			}
		}
		if now.Sub(info.ModTime()) > snapshotKeep {
			_ = os.RemoveAll(filepath.Join(s.o.SnapshotDir, e.Name()))
		}
	}
}
