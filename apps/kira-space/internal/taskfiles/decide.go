package taskfiles

import (
	"fmt"
	"time"
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
)

// Cause is why a change needs approval; it picks the label and the reply phrase.
type Cause string

// Causes.
const (
	CauseNone         Cause = ""
	CauseLink         Cause = "link"
	CauseNote         Cause = "note"
	CauseHandover     Cause = "handover"
	CauseProof        Cause = "proof"
	CauseProofDelete  Cause = "proofdelete"
	CauseCollision    Cause = "collision"
	CauseWrongStage   Cause = "stage"
	CauseKiraReadOnly Cause = "readonly"
)

// Collision says another writer changed the file after the calling session's last sync.
type Collision struct {
	By string
	At time.Time
}

// Author is who wrote the target a change touches, from the calling session's view.
type Author string

// Authors. AuthorNone covers a target nobody wrote yet, the user's, Kira's and an orchestrator pass's.
const (
	AuthorNone  Author = ""
	AuthorSelf  Author = "self"
	AuthorAgent Author = "agent"
)

// AuthorOf classifies a target: agent is whether an agent wrote it, session is the writer's key and
// caller the calling session's key. An agent target with another or no session id is another agent's.
func AuthorOf(agent bool, session, caller string) Author {
	switch {
	case !agent:
		return AuthorNone
	case session != "" && session == caller:
		return AuthorSelf
	}
	return AuthorAgent
}

// Input is one change to decide.
type Input struct {
	Role Role
	Kind Kind
	Op   Op
	// Author is who wrote the target. Only another agent's content asks the user.
	Author Author
	// Slot marks a Jira or PR slot change; slots carry no author and never ask.
	Slot bool
	// Step is the step id of a draft file; CurrentStep is the task's current step id.
	Step, CurrentStep string
	// ProofAccepted is true when the proof folder already has an accepted proof.md.
	ProofAccepted bool
	// Latest is true for the newest note, handed handover or proof of the task: an agent edit of it
	// needs no approval.
	Latest    bool
	Collision *Collision
}

// Decision is the outcome of Input, with what the user or agent is told.
type Decision struct {
	Outcome Outcome
	Cause   Cause
	// Message is the line the reply carries for a non-accept outcome.
	Message string
	// Collision is set on a pending decision whose file another writer changed since the session's last sync.
	Collision *Collision
}

// Decide applies the author rule to one change. rel names the file in messages.
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
	cause := approvalCause(in)
	if cause == CauseNone && in.Collision != nil {
		cause = CauseCollision
	}
	if cause == CauseNone {
		return Decision{Outcome: Accept}
	}
	return Decision{Outcome: Pending, Cause: cause, Collision: in.Collision}
}

// approvalCause is the cause that puts an agent change behind the user. Only another agent's content asks.
func approvalCause(in Input) Cause {
	other := in.Author == AuthorAgent
	switch in.Kind {
	case KindNote:
		if other && (in.Op == OpDelete || (in.Op == OpEdit && !in.Latest)) {
			return CauseNote
		}
	case KindHandover:
		if other && (in.Op == OpDelete || (in.Op == OpEdit && !in.Latest)) {
			return CauseHandover
		}
	case KindLinks:
		if other && !in.Slot && in.Op != OpCreate {
			return CauseLink
		}
	case KindProof, KindProofData, KindProofImage:
		switch {
		case !in.ProofAccepted:
		case !other:
		case in.Op == OpDelete && in.Kind == KindProof:
			return CauseProofDelete
		case in.Op == OpDelete || !in.Latest:
			return CauseProof
		}
	}
	return CauseNone
}

// causeText is the phrase a pending reply line gives for a cause; collision adds the writer.
var causeText = map[Cause]string{
	CauseLink:        "another agent's link",
	CauseNote:        "another agent's note",
	CauseHandover:    "another agent's handover",
	CauseProof:       "another agent's proof",
	CauseProofDelete: "deleting another agent's proof",
}
