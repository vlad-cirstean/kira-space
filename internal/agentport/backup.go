package agentport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// backupStamp is the UTC time suffix of a backup name.
const backupStamp = "20060102T150405Z"

// maxBackupNames bounds the -N suffixes tried for one second.
const maxBackupNames = 1000

// resolveTarget follows a symlink (a dotfile manager's) to the file that gets written; the link stays.
func resolveTarget(path string) (string, error) {
	real, err := fileservice.Host().Resolve(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	return real, err
}

// backup copies path to <path>.kira-backup-<UTC time> beside it, keeping the mode (the copy holds the
// same credentials as the original). It never replaces a backup: a name taken in the same second gets
// a -N suffix. It returns the backup's path.
func backup(path string, data []byte, mode os.FileMode, now time.Time) (string, error) {
	stem := path + ".kira-backup-" + now.UTC().Format(backupStamp)
	perm := mode.Perm() | 0o600
	for i := 0; i < maxBackupNames; i++ {
		dst := stem
		if i > 0 {
			dst = fmt.Sprintf("%s-%d", stem, i)
		}
		_, err := fileservice.Host().Write(dst, data, fileservice.WriteOptions{Perm: perm, Static: true, Exclusive: true})
		if errors.Is(err, fileservice.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("agentport: back up %s: %w", filepath.Base(path), err)
		}
		return dst, nil
	}
	return "", fmt.Errorf("agentport: back up %s: too many backups in one second", filepath.Base(path))
}

// writeFile writes data atomically, creating parent directories 0700; an existing file keeps its mode.
func writeFile(path string, data []byte, defMode os.FileMode) error {
	mode := defMode
	if st, err := fileservice.Host().Stat(path); err == nil {
		mode = st.Mode.Perm()
	}
	if err := fileservice.HostPrivate().MkdirAll(filepath.Dir(path)); err != nil {
		return fmt.Errorf("agentport: create %s: %w", filepath.Dir(path), err)
	}
	if err := fileservice.Host().WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("agentport: write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// readIfExists reads path; exists is false for a missing file. Files over max bytes are an error.
func readIfExists(path string, max int64) (data []byte, mode os.FileMode, exists bool, err error) {
	st, err := fileservice.Host().Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if st.Dir {
		return nil, 0, false, fmt.Errorf("agentport: %s is a directory", filepath.Base(path))
	}
	if st.Size > max {
		return nil, 0, true, errTooLarge
	}
	data, err = fileservice.Host().ReadFile(path)
	return data, st.Mode.Perm(), true, err
}

var errTooLarge = errors.New("agentport: file is too large")
