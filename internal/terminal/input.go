package terminal

import "bytes"

var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
)

// inputState follows the user's keystrokes to tell whether text waits in the agent's prompt box.
// A bracketed paste is text even when it carries a carriage return, which submits nothing.
type inputState struct {
	inPaste bool
	draft   bool
}

// feed folds one write into the state. Control sequences (arrows, focus and mouse reports) change
// nothing; Enter, Ctrl-C and Ctrl-U empty the box; any other byte leaves text in it.
func (in *inputState) feed(b []byte) {
	for len(b) > 0 {
		if in.inPaste {
			i := bytes.Index(b, pasteEnd)
			if i < 0 {
				return
			}
			in.inPaste = false
			b = b[i+len(pasteEnd):]
			continue
		}
		switch {
		case bytes.HasPrefix(b, pasteStart):
			in.inPaste, in.draft = true, true
			b = b[len(pasteStart):]
		case len(b) > 1 && b[0] == 0x1b && b[1] == '[':
			i := 2
			for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
				i++
			}
			b = b[min(i+1, len(b)):]
		case b[0] == '\r' || b[0] == 0x03 || b[0] == 0x15:
			in.draft = false
			b = b[1:]
		default:
			in.draft = true
			b = b[1:]
		}
	}
}
