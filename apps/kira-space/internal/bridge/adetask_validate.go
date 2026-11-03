package bridge

import (
	"regexp"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// adeMaxMessageBytes bounds a message sent to or launched in a session: generous for a prompt, small
// enough that the quoted launch command stays under MaxCommandBytes.
const adeMaxMessageBytes = 32 * 1024

// adeMaxBranchBytes bounds a branch or item id.
const adeMaxBranchBytes = 255

var (
	adeJiraKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)
	adeEstRe     = regexp.MustCompile(`^\d+(\.\d+)?[hd]$`)
)

const (
	adeMaxNotesBytes = 1 << 20 // 1 MiB
	adeMaxNameBytes  = 500
	adeMaxURLBytes   = 2048
)

// validateAdeBranchName checks a value that must name a real git ref (branch/branchName/
// startFrom): non-empty, no NUL/space/`:`/leading `-`, <= 255 bytes — real ref validity is git's
// own (an unresolved branch comes back `exists: false` rather than erroring here).
func validateAdeBranchName(value, field string) error {
	if value == "" {
		return ipcerr.Required(field)
	}
	if len(value) > adeMaxBranchBytes {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.branchNameTooLong.body", nil)
	}
	if strings.ContainsAny(value, "\x00 :") || strings.HasPrefix(value, "-") {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.branchNameInvalid.body", nil)
	}
	return nil
}

// validateAdeItemID checks a value naming a queue item (a branch, or a new-work id — "nw:<uuid>",
// which itself carries a `:` — so this is deliberately looser than validateAdeBranchName's own
// git-ref shape: item and after (the "Items in SetPlan/SetQueuedAfter..." rule) name either
// kind of item, never only a branch).
func validateAdeItemID(value, field string) error {
	if value == "" {
		return ipcerr.Required(field)
	}
	if len(value) > adeMaxBranchBytes {
		return ipcerr.Contract("%s is too long", field)
	}
	if strings.ContainsAny(value, "\x00\n") {
		return ipcerr.Contract("%s must not contain NUL or newline", field)
	}
	return nil
}

func validateAdeName(value, field string) error {
	if len(value) > adeMaxNameBytes {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.nameTooLong.body", nil)
	}
	return nil
}

func adeMessageTooLong() error {
	return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.messageTooLong.body", nil)
}

func adeYamlTooLarge() error {
	return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.yamlTooLarge.body", nil)
}

func validateAdeNotes(notes string) error {
	if len(notes) > adeMaxNotesBytes {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.notesTooLong.body", nil)
	}
	return nil
}

func validateAdeEst(est string) error {
	if est == "" {
		return nil
	}
	if !adeEstRe.MatchString(est) {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.estimateInvalid.body", nil)
	}
	if repos.EstOverCap(est) {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.estimateTooLong.body", nil)
	}
	return nil
}

func validateAdeURL(value, field string) error {
	if value == "" {
		return nil
	}
	if len(value) > adeMaxURLBytes {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.urlTooLong.body", nil)
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.urlScheme.body", nil)
	}
	return nil
}

func validateAdeJira(key, url string) error {
	if key != "" && !adeJiraKeyRe.MatchString(key) {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.jiraKeyInvalid.body", nil)
	}
	return validateAdeURL(url, "jiraUrl")
}

func validateAdeISODate(value string) error {
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return ipcerr.Contract("date must be YYYY-MM-DD")
	}
	return nil
}
