package configfs

import (
	"bytes"
	"strconv"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Text decodes a prose file (Markdown): UTF-8 text of at most max bytes, no further schema.
func Text(max int) DecodeFunc[string] {
	return func(_ string, src []byte) (string, *Problem) {
		if len(src) > max {
			return "", &Problem{Message: i18n.T("backend.core.config.textTooLarge.body", map[string]string{"limit": strconv.Itoa(max)})}
		}
		if !utf8.Valid(src) {
			bad := 0
			for bad < len(src) {
				r, size := utf8.DecodeRune(src[bad:])
				if r == utf8.RuneError && size <= 1 {
					break
				}
				bad += size
			}
			return "", &Problem{Line: 1 + bytes.Count(src[:bad], []byte("\n")), Message: i18n.T("backend.core.config.textEncoding.body", nil)}
		}
		return string(src), nil
	}
}
