package configsync

import (
	"os"
	"path/filepath"
)

// HasBase reports whether the shared repo last delivered module/file: its base copy exists under .sync.
// A file with a base keeps its name, so later pulls keep merging instead of conflicting.
func HasBase(home, module, file string) bool {
	_, err := os.Lstat(filepath.Join(syncDir(home), baseDir, module, file))
	return err == nil
}
