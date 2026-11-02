package agentctx

import (
	"errors"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Service is the bound surface of the editor: read the context file and save it. The app's own bridge
// type embeds it so binding names stay app-local.
type Service struct {
	Store *Store
}

// SaveArgs is one save: the whole text and the hash of the text the editor loaded.
type SaveArgs struct {
	Text     string `json:"text"`
	BaseHash string `json:"baseHash"`
}

// Get returns the file's state.
func (s *Service) Get() (View, error) { return s.Store.View(), nil }

// Save writes the file. A file changed since BaseHash is a conflict error.
func (s *Service) Save(args SaveArgs) (View, error) {
	v, err := s.Store.Save(args.Text, args.BaseHash)
	switch {
	case err == nil:
		return v, nil
	case errors.Is(err, configfs.ErrConflict):
		return View{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.agentContext.changed.body", nil)
	case errors.Is(err, configfs.ErrInvalid):
		return View{}, ipcerr.BadRequest(err.Error())
	}
	return View{}, ipcerr.InternalErr(err)
}
