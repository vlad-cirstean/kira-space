package taskfiles

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Role is who writes. Agent roles go through the decision matrix; the user and Kira are accepted.
type Role string

// Writer roles: the headless run, the manual step session, the review agent, the orchestrator on
// its own task; the user (UI, or Accept on the disk banner) and Kira itself.
const (
	RoleRun    Role = "run"
	RoleManual Role = "manual"
	RoleReview Role = "review"
	RoleOrch   Role = "orch"
	RoleUser   Role = "user"
	RoleKira   Role = "kira"
)

// Agent reports whether the role is an agent.
func (r Role) Agent() bool {
	return r == RoleRun || r == RoleManual || r == RoleReview || r == RoleOrch
}

// Op is what a change does to its target.
type Op string

// Ops.
const (
	OpCreate Op = "create"
	OpEdit   Op = "edit"
	OpDelete Op = "delete"
)

// Outcome is the decision about one change.
type Outcome string

// Outcomes.
const (
	// Accept applies the change to the cache at once.
	Accept Outcome = "accept"
	// Pending holds the change for the user's approval.
	Pending Outcome = "pending"
	// Reject drops the change and removes the file.
	Reject Outcome = "reject"
	// Restore rewrites the file from the accepted copy.
	Restore Outcome = "restore"
	// Invalid refuses the change with a message the agent can act on.
	Invalid Outcome = "invalid"
)

// Cause is why a change needs approval; it picks the reason-error text.
type Cause string

// Causes.
const (
	CauseNone         Cause = ""
	CauseLink         Cause = "link"
	CauseSlot         Cause = "slot"
	CauseNote         Cause = "note"
	CauseHandover     Cause = "handover"
	CauseProof        Cause = "proof"
	CauseProofDelete  Cause = "proofdelete"
	CauseCollision    Cause = "collision"
	CauseWrongStage   Cause = "stage"
	CauseKiraReadOnly Cause = "readonly"
)

// Notify names a notification a decision carries.
type Notify string

// Notifications. A silent accept has none.
const (
	NotifyNone       Notify = ""
	NotifyProofReady Notify = "proofReady"
)

// Collision says another writer changed the file after the calling session's last sync.
type Collision struct {
	By string
	At time.Time
}

// Input is one change to decide.
type Input struct {
	Role Role
	Kind Kind
	Op   Op
	// Own is true for a link entry the writer's session reported itself, and for a new entry.
	Own bool
	// Slot marks a Jira or PR slot change; Op create means the slot was empty.
	Slot bool
	// Step is the step id of a draft file; CurrentStep is the task's current step id.
	Step, CurrentStep string
	// ProofAccepted is true when the proof folder already has an accepted proof.md.
	ProofAccepted bool
	Collision     *Collision
	Reason        string
}

// Decision is the outcome of Input, with what the user or agent is told.
type Decision struct {
	Outcome Outcome
	Cause   Cause
	Notify  Notify
	// Message is the line the reply carries for a non-accept outcome.
	Message string
	// Collision is set on a pending decision whose file another writer changed since the session's last sync.
	Collision *Collision
}

// Decide applies the matrix of plan section 7.3 to one change. rel names the file in messages.
func Decide(rel string, in Input) Decision {
	switch {
	case !in.Role.Agent():
		return Decision{Outcome: Accept}
	case in.Kind.KiraOwned():
		return Decision{Outcome: Restore, Cause: CauseKiraReadOnly, Message: rel + ": read-only, Kira restored it."}
	case in.Kind == KindDraft && in.CurrentStep != "" && in.Step != in.CurrentStep:
		return Decision{Outcome: Reject, Cause: CauseWrongStage,
			Message: fmt.Sprintf("%s: the task moved on to %s. Write handovers/draft-%s.md instead.", rel, in.CurrentStep, in.CurrentStep)}
	}
	cause, notify := approvalCause(in)
	if cause == CauseNone && in.Collision != nil {
		cause = CauseCollision
	}
	if cause == CauseNone {
		return Decision{Outcome: Accept, Notify: notify}
	}
	if strings.TrimSpace(in.Reason) == "" {
		return Decision{Outcome: Invalid, Cause: cause, Message: ReasonError(rel, cause)}
	}
	return Decision{Outcome: Pending, Cause: cause, Collision: in.Collision}
}

// approvalCause is the cause that puts an agent change behind the user, and the notification of an
// accepted one.
func approvalCause(in Input) (Cause, Notify) {
	switch in.Kind {
	case KindNote:
		return CauseNote, NotifyNone
	case KindHandover:
		return CauseHandover, NotifyNone
	case KindLinks:
		switch {
		case in.Slot && in.Op != OpCreate:
			return CauseSlot, NotifyNone
		case in.Slot, in.Op == OpCreate, in.Own:
			return CauseNone, NotifyNone
		}
		return CauseLink, NotifyNone
	case KindProof, KindProofData, KindProofImage:
		switch {
		case !in.ProofAccepted:
			if in.Kind == KindProof && in.Op == OpCreate {
				return CauseNone, NotifyProofReady
			}
			return CauseNone, NotifyNone
		case in.Op == OpDelete && in.Kind == KindProof:
			return CauseProofDelete, NotifyNone
		}
		return CauseProof, NotifyNone
	}
	return CauseNone, NotifyNone
}

var reasonText = map[Cause][2]string{
	CauseLink:        {"the user approves changes to another author's link", "link title fixed"},
	CauseSlot:        {"the user approves a replaced Jira or PR link", "PR moved to the new branch"},
	CauseNote:        {"the user approves changes to a note", "typo fixed in the note"},
	CauseHandover:    {"the user approves changes to a handed handover", "outcome line corrected"},
	CauseProof:       {"the user approves changes to an accepted proof", "order 12 corrected"},
	CauseProofDelete: {"the user approves deleting an accepted proof", "proof replaced by a new one"},
	CauseCollision:   {"the file changed after your last read", "kept my edit after reading the change"},
}

// ReasonError is the one-line message of a change that needs a reason and has none. It names the
// field and gives an example, so the agent retries once.
func ReasonError(rel string, cause Cause) string {
	t := reasonText[cause]
	if t[0] == "" {
		t = [2]string{"the user approves this change", "what changed and why"}
	}
	return fmt.Sprintf("%s: needs reason (%s). Call submit again with {\"reason\": %q}.", rel, t[0], t[1])
}

// Reason limits, in runes.
const (
	MinReasonRunes = 1
	MaxReasonRunes = 500
)

// CleanReason trims a reason and checks it is 1 to 500 runes of valid text.
func CleanReason(s string) (string, error) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n == 0 {
		return "", nil
	}
	if n > MaxReasonRunes || !utf8.ValidString(s) {
		return "", fmt.Errorf("reason must be %d to %d characters of text", MinReasonRunes, MaxReasonRunes)
	}
	return s, nil
}
