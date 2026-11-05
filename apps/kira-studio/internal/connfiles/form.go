package connfiles

import "github.com/kirathecat/kira-studio/internal/configfs"

// Form applies the form's edits to the draft text as YAML node edits and decodes the result with
// the connection file rules. Stateless: nothing is written; the dialog saves the text with
// SaveFile.
func (l *Loader) Form(file, text string, edits []configfs.Edit) configfs.FormResult {
	return configfs.Form(l.folder.Decode, file, []byte(text), edits)
}
