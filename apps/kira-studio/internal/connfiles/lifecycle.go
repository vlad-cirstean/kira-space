package connfiles

import (
	"fmt"
	"os"
	"path/filepath"
)

// DeleteFile removes a file that has no connection row, one that never held a valid connection. A
// file with a row is refused: Delete removes the connection with its state.
func (l *Loader) DeleteFile(file string) error {
	stem, ok := StemOf(file)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, file)
	}
	l.mu.Lock()
	_, found := l.folder.Get(file)
	l.mu.Unlock()
	if !found {
		l.flush()
		return fmt.Errorf("%w: %s", ErrNotFound, file)
	}
	ids, err := l.idsByStem()
	if err != nil {
		return err
	}
	if ids[stem] != "" {
		return fmt.Errorf("%w: %s holds a connection; delete the connection instead", ErrExists, file)
	}
	if err := os.Remove(filepath.Join(l.o.Dir, file)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("connfiles: remove %s: %w", file, err)
	}
	l.Reload()
	return nil
}
