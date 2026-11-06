package ade

import "strings"

// Bracketed paste (§4.5): a terminal in bracketed-paste mode treats everything between these two
// sequences as pasted text rather than typed keystrokes — the TUI's own line-editing never
// interprets a newline inside the message as "submit".
const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

// normalizeMessage prepares message text for either the initial CLI prompt or a forwarded paste:
// CRLF and bare CR become LF (a message typed on Windows or pasted from one source must not leave
// a stray \r inside the pty write), and every other control char (ESC, C1, DEL) is dropped, keeping \n and \t. Dropping ESC alone
// defeats a nested \x1b[20\x1b[201~1~ that a single-pass replace would reassemble into end-of-paste.
func normalizeMessage(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

// pasteBytes renders message as a bracketed-paste write followed by the Enter that submits it —
// two separate byte slices so Tracker.Send can write them as two separate WriteTerminal
// calls: whether Claude Code's TUI needs a pause between paste-end and Enter is unverified,
// and keeping them apart lets a delay be added later without restructuring the call.
func pasteBytes(message string) (paste []byte, enter []byte) {
	normalized := normalizeMessage(message)
	paste = []byte(bracketedPasteStart + normalized + bracketedPasteEnd)
	enter = []byte("\r")
	return paste, enter
}
