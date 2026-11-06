package apivars

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/localauth"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// revealReason mirrors connections.revealReason — LAContext.evaluatePolicy's localizedReason
// (P14 D11; macOS prefixes it with "Kira Studio wants to").
const revealReason = "reveal a saved variable value."

// The same four-outcome vocabulary connections.RevealResult already established (D8), redeclared
// rather than imported — see the package doc for why.
const (
	OutcomeRevealed             = "revealed"
	OutcomeCancelled            = "cancelled"
	OutcomeConfirmationRequired = "confirmation-required"
	OutcomeError                = "error"
)

// RevealResult never carries a value except on OutcomeRevealed.
type RevealResult struct {
	Value   *string `json:"value"`
	Error   *string `json:"error"`
	Outcome string  `json:"outcome"`
}

// Reveal never errors: an undecryptable secret, a declined prompt, or a required confirmation all
// come back as a RevealResult naming its own Outcome, not a rejected call. Gated through the SAME
// *localauth.Authorizer instance connections.Service.Reveal uses — a grace granted by either reveal
// covers the other.
func (s *Service) Reveal(variableID string, confirmed bool) RevealResult {
	if variableID == "" {
		msg := "variableId is required"
		return RevealResult{Outcome: OutcomeError, Error: &msg}
	}
	return s.reveal(revealReason, variableID, confirmed, s.revealVariable, "variable")
}

// RevealHistory is Reveal's sibling over api_variable_history (D13) — a secret's old value is
// exactly as sensitive as its current one, so it goes through the same gate.
func (s *Service) RevealHistory(historyID string, confirmed bool) RevealResult {
	if historyID == "" {
		msg := "historyId is required"
		return RevealResult{Outcome: OutcomeError, Error: &msg}
	}
	return s.reveal(revealReason, historyID, confirmed, s.deps.Repo.RevealHistoryValue, "variable history entry")
}

// revealVariable opens the secret a variable row points at. A secret that is not set is an error the
// caller turns into an error result.
func (s *Service) revealVariable(variableID string) (string, error) {
	ref, err := s.deps.Repo.SecretRefOf(variableID)
	if err != nil {
		return "", err
	}
	v, err := s.deps.Secrets.Reveal(ref)
	if errors.Is(err, configfs.ErrSecretMissing) {
		return "", fmt.Errorf("the secret %s is not set", ref)
	}
	return v, err
}

func (s *Service) reveal(reason, id string, confirmed bool, decrypt func(string) (string, error), subject string) RevealResult {
	outcome, value, err := localauth.Gated(s.deps.Auth.Authorize, reason, confirmed, func() (*string, error) {
		v, err := decrypt(id)
		if err != nil {
			return nil, err
		}
		return &v, nil
	})
	switch outcome {
	case localauth.GateAuthError:
		msg := err.Error()
		slog.Warn("local authentication errored before reveal", "scope", "apivars", "subject", subject, "id", id, "err", msg)
		return RevealResult{Outcome: OutcomeError, Error: &msg}
	case localauth.GateCancelled:
		// D11 (inherited via P14): the user cancelled on purpose — no message, that would nag.
		return RevealResult{Outcome: OutcomeCancelled}
	case localauth.GateConfirmationRequired:
		return RevealResult{Outcome: OutcomeConfirmationRequired}
	case localauth.GateFetchError:
		msg := err.Error()
		slog.Warn("reveal failed", "scope", "apivars", "subject", subject, "id", id, "err", msg)
		return RevealResult{Outcome: OutcomeError, Error: &msg}
	}
	// D5: the subject, never the value — connections.Service.Reveal's own precedent.
	slog.Info("secret revealed", "scope", "apivars", "subject", subject, "id", id)
	return RevealResult{Outcome: OutcomeRevealed, Value: value}
}
