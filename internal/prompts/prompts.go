// Package prompts routes every popup an app raises on its own (a schedule confirm, an MCP
// approval, a git credential, a phone pairing, an update) to exactly one window and posts one OS
// notification per kind. Owners keep their queues and answers; the router holds metadata only.
package prompts

// Kind is the popup's owner.
type Kind string

const (
	KindSchedule      Kind = "schedule"
	KindGitCredential Kind = "git-credential"
	KindHandover      Kind = "handover"
	KindMobilePairing Kind = "mobile-pairing"
	KindUpdate        Kind = "update"
	KindQuit          Kind = "quit"
	KindProof         Kind = "proof"
	// KindMasking is a connection with columns that need a masking review (apps/kira-studio/internal/maskreview); Ref is the connection id.
	KindMasking Kind = "masking"
	// KindApproval is one pending agent approval (internal/approvals); Ref is the item id.
	KindApproval Kind = "approval"
)

// Prompt is one popup. It never carries a statement, git prompt text, pairing code or params.
type Prompt struct {
	// ID is "<kind>:<ref>".
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	// Ref is the owner's id the kind's dialog answers.
	Ref string `json:"ref"`
	// Origin is the window key the popup came from; "" is generic (cron, MCP, phone, background git).
	Origin    string `json:"origin"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"createdAt"`
}

// Routed is a Prompt with the window that shows it; Target "" means queued until a window opens.
type Routed struct {
	Prompt
	Target string `json:"target"`
}

// Sink is how an owner registers and withdraws its popups. A nil Sink drops both.
type Sink interface {
	Open(Prompt)
	Close(id string)
}

// ID builds a prompt id from kind and ref.
func ID(k Kind, ref string) string { return string(k) + ":" + ref }
