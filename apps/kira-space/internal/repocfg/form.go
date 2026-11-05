package repocfg

import (
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// FormArgs is one form call: the draft text and the edits to apply to it. File is the file the
// draft belongs to; a repo with no file yet sends "" and the template text.
type FormArgs struct {
	File  string          `json:"file"`
	Text  string          `json:"text"`
	Edits []configfs.Edit `json:"edits"`
}

// Form applies the form's edits to the draft as YAML node edits and decodes the result with the
// repo file rules. Stateless: nothing is written; the dialog saves the text with SaveFile.
func (s *Service) Form(args FormArgs) (configfs.FormResult, error) {
	if len(args.Text) > configfs.MaxFileBytes {
		return configfs.FormResult{}, ipcerr.BadRequest(configfs.ErrTooLarge.Error())
	}
	name := args.File
	if name == "" {
		name = "new.yaml"
	}
	res := configfs.Form(s.Store.folder.Decode, name, []byte(args.Text), args.Edits)
	if len(res.Text) > configfs.MaxFileBytes {
		return configfs.FormResult{}, ipcerr.BadRequest(configfs.ErrTooLarge.Error())
	}
	return res, nil
}
