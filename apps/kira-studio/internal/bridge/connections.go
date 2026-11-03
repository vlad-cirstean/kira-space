package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

type ConnectionsService struct {
	Deps appcore.Deps
}

func (s *ConnectionsService) List() ([]model.ConnectionSummary, error) {
	return s.Deps.Connections.List()
}

// ConnectionsCreateArgs is the connection form itself; the alias keeps the wire JSON flat.
type ConnectionsCreateArgs = connections.Input

func (s *ConnectionsService) Create(input ConnectionsCreateArgs) (model.ConnectionSummary, error) {
	return s.Deps.Connections.Create(input)
}

// ConnectionsCredentialsArgs updates the username and password of a fields-mode connection. A nil
// field stays as it is.
type ConnectionsCredentialsArgs struct {
	ID       string  `json:"id"`
	Username *string `json:"username"`
	Password *string `json:"password"`
}

// UpdateCredentials edits the username in the connection's file and seals the password under the
// secret the file names. A live connection reconnects.
func (s *ConnectionsService) UpdateCredentials(args ConnectionsCredentialsArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	return s.Deps.Connections.UpdateCredentials(args.ID, args.Username, args.Password)
}

// ConnectionsIDArgs is shared by every method below that needs nothing but a connection id.
type ConnectionsIDArgs struct {
	ID string `json:"id"`
}

func (s *ConnectionsService) Duplicate(args ConnectionsIDArgs) (model.ConnectionSummary, error) {
	if args.ID == "" {
		return model.ConnectionSummary{}, ipcerr.Required("id")
	}
	return s.Deps.Connections.Duplicate(args.ID)
}

func (s *ConnectionsService) Delete(args ConnectionsIDArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	return s.Deps.Connections.Remove(args.ID)
}

// ConnectionsReorderArgs carries the full connection order, top to bottom.
type ConnectionsReorderArgs struct {
	IDs []string `json:"ids"`
}

func (s *ConnectionsService) Reorder(args ConnectionsReorderArgs) ([]model.ConnectionSummary, error) {
	return s.Deps.Connections.Reorder(args.IDs)
}

// ConnectionsRevealArgs is Reveal's own args struct (P14 D6) — it leaves the shared
// ConnectionsIDArgs above, whose own comment scopes it to methods "that need nothing but a
// connection id", which is no longer true here.
type ConnectionsRevealArgs struct {
	ID string `json:"id"`
	// Confirmed is honoured ONLY when the backend has itself determined OS authentication is
	// unavailable — on a machine where LocalAuthentication works, this field cannot
	// influence the result; internal/connections.Service.Reveal's own Authorizer call enforces
	// that, not this method.
	Confirmed bool `json:"confirmed"`
}

// Reveal never errors (P25 D9) — an undecryptable secret, a declined prompt, or a required
// confirmation all come back as a RevealResult carrying its own Outcome, not a rejected call. A
// bare id is reported the same way, rather than as a rejected call, for consistency with that
// rule.
func (s *ConnectionsService) Reveal(args ConnectionsRevealArgs) connections.RevealResult {
	if args.ID == "" {
		msg := "id is required"
		return connections.RevealResult{Outcome: "error", Error: &msg}
	}
	return s.Deps.Connections.Reveal(args.ID, args.Confirmed)
}

// Test never errors: failure is reported inside TestResult. It tests the saved file.
func (s *ConnectionsService) Test(args ConnectionsIDArgs) connections.TestResult {
	if args.ID == "" {
		msg := "id is required"
		return connections.TestResult{Error: &msg}
	}
	return s.Deps.Connections.Test(args.ID)
}

func (s *ConnectionsService) Connect(args ConnectionsIDArgs) (model.ConnectionState, error) {
	if args.ID == "" {
		return model.ConnectionState{}, ipcerr.Required("id")
	}
	return s.Deps.Connections.Connect(args.ID)
}

func (s *ConnectionsService) Disconnect(args ConnectionsIDArgs) (model.ConnectionState, error) {
	if args.ID == "" {
		return model.ConnectionState{}, ipcerr.Required("id")
	}
	return s.Deps.Connections.Disconnect(args.ID)
}

// States is in-memory, not DB-backed (mirrors src/main/connections.ts's own `states` Map),
// sorted by connection id (P55 §2 D7).
func (s *ConnectionsService) States() ([]model.ConnectionState, error) {
	return s.Deps.Connections.States(), nil
}

// SecretStorageStatus mirrors packages/shared/domain/secrets.ts's secretStorageStatusSchema.
type SecretStorageStatus struct {
	Available        bool    `json:"available"`
	Backend          string  `json:"backend"`
	InsecureFallback bool    `json:"insecureFallback"`
	Reason           *string `json:"reason"`
}

// SecretsStatus reports the real Keychain-backed cipher's status (P55 M1). secrets.Status and
// SecretStorageStatus are field-for-field identical (that struct moved to internal/secrets
// unchanged, per P55 §1.3) so a plain conversion is exact, not a coincidental shortcut.
func (s *ConnectionsService) SecretsStatus() (SecretStorageStatus, error) {
	return SecretStorageStatus(s.Deps.Connections.SecretsStatus()), nil
}
