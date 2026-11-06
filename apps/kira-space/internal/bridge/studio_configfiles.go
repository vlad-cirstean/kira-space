package bridge

import "github.com/kirathecat/kira-studio/internal/configfs/configsvc"

// StudioConfigFilesService is Kira Studio's own binding-name shim over configsvc.Service (see
// WindowsService for why the type is embedded here).
type StudioConfigFilesService struct {
	*configsvc.Service
}
