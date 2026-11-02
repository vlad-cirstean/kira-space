package bridge

import "github.com/kirathecat/kira-studio/internal/configfs/configsvc"

// ConfigFilesService is Kira Space's own binding-name shim over configsvc.Service (see
// WindowsService for why the type is embedded here).
type ConfigFilesService struct {
	*configsvc.Service
}
