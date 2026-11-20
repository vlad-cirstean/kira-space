package configsync

import (
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// HasBase reports whether the shared repo last delivered module/file: its base copy exists under .sync.
// A file with a base keeps its name, so later pulls keep merging instead of conflicting.
func HasBase(home, module, file string) bool {
	_, err := fileservice.Host().Lstat(filepath.Join(syncDir(home), baseDir, module, file))
	return err == nil
}
