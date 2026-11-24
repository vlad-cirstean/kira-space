package dbmcp

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// maxConfirmRunes caps the agent's confirm sentence.
const maxConfirmRunes = 300

// confirmText trims s. refusal is non-empty when s is over the cap. An empty text means no confirm.
// nothing finishes the refusal: what did not happen ("Nothing ran.").
func confirmText(s, nothing string) (text, refusal string) {
	text = strings.TrimSpace(s)
	if utf8.RuneCountInString(text) > maxConfirmRunes {
		return "", fmt.Sprintf("confirm is one sentence for the user, at most %d characters. %s", maxConfirmRunes, nothing)
	}
	return text, ""
}

// confirmReason is the reason of a card the policy may or may not raise by itself.
func confirmReason(policyAsks bool) ApprovalReason {
	if policyAsks {
		return ApprovalReasonPermission
	}
	return ApprovalReasonConfirm
}
