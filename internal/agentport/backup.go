package agentport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/renameio/v2"
)

// backupStamp is the UTC time suffix of a backup name.
const backupStamp = "20060102T150405Z"

// resolveTarget follows a symlink (a dotfile manager's) to the file that gets written; the link stays.
func resolveTarget(path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	return real, err
}

// backup copies path to <path>.kira-backup-<UTC time> beside it, keeping the mode (the copy holds the
// same credentials as the original). It returns the backup's path.
func backup(path string, data []byte, mode os.FileMode, now time.Time) (string, error) {
	dst := path + ".kira-backup-" + now.UTC().Format(backupStamp)
	if err := renameio.WriteFile(dst, data, mode.Perm()|0o600); err != nil {
		return "", fmt.Errorf("agentport: back up %s: %w", filepath.Base(path), err)
	}
	return dst, nil
}

// writeFile writes data atomically, creating parent directories 0700; an existing file keeps its mode.
func writeFile(path string, data []byte, defMode os.FileMode) error {
	mode := defMode
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("agentport: create %s: %w", filepath.Dir(path), err)
	}
	if err := renameio.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("agentport: write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// readIfExists reads path; exists is false for a missing file. Files over max bytes are an error.
func readIfExists(path string, max int64) (data []byte, mode os.FileMode, exists bool, err error) {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if st.IsDir() {
		return nil, 0, false, fmt.Errorf("agentport: %s is a directory", filepath.Base(path))
	}
	if st.Size() > max {
		return nil, 0, true, errTooLarge
	}
	data, err = os.ReadFile(path)
	return data, st.Mode().Perm(), true, err
}

var errTooLarge = errors.New("agentport: file is too large")
