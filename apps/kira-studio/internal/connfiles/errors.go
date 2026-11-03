package connfiles

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// IPCError maps a file read or write failure to its IPC error. ErrExists passes through so a create
// can retry on a fresh stem.
func IPCError(err error) error {
	var broken *BrokenError
	var invalid *InvalidError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrExists):
		return err
	case errors.Is(err, ErrNotFound):
		return ipcerr.T(ipcerr.CodeNotFound, "backend.studio.connectionFiles.notFound.body", nil)
	case errors.Is(err, configfs.ErrConflict):
		return ipcerr.T(ipcerr.CodeConflict, "backend.studio.apiFiles.changed.body", nil)
	case errors.As(err, &broken):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.apiFiles.fileBroken.body", map[string]string{"file": broken.File, "line": fmt.Sprint(broken.Line)})
	case errors.As(err, &invalid):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.connectionFiles.editInvalid.body", map[string]string{"detail": strings.TrimSuffix(invalid.Problem.Message, ".")})
	}
	return ipcerr.Wrap(err)
}
