package scripts

import (
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

type FormArgs struct {
	File  string          `json:"file"`
	Text  string          `json:"text"`
	Edits []configfs.Edit `json:"edits"`
}

// Form applies the form's edits to the draft text and decodes the result. Stateless: nothing is
// written; the dialog saves the returned text with SaveFile.
func (s *Service) Form(args FormArgs) (configfs.FormResult, error) {
	if args.File == "" {
		return configfs.FormResult{}, ipcerr.Required("file")
	}
	if len(args.Text) > configfs.MaxFileBytes {
		return configfs.FormResult{}, ipcerr.BadRequest(configfs.ErrTooLarge.Error())
	}
	res := configfs.Form(s.Loader.folder.Decode, args.File, []byte(args.Text), args.Edits)
	if len(res.Text) > configfs.MaxFileBytes {
		return configfs.FormResult{}, ipcerr.BadRequest(configfs.ErrTooLarge.Error())
	}
	return res, nil
}
