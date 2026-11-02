package appsettings

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// Agent is the agent.* section both apps share. Provider is the id of the harness every new agent
// run, launch and structured call uses. Registration is not checked on write: an unknown id falls
// back to the default provider when it is resolved, so a removed provider's stored id still boots.
// Hermetic: agents the app starts load none of the user's own agent config (default on).
type Agent struct {
	Provider string `json:"provider"`
	Hermetic bool   `json:"hermetic"`
}

// AgentPatch mirrors Agent's own `.partial()` shape.
type AgentPatch struct {
	Provider *string `json:"provider,omitempty"`
	Hermetic *bool   `json:"hermetic,omitempty"`
}

// DefaultAgent mirrors settings.ts's agent defaults.
func DefaultAgent() Agent { return Agent{Provider: harness.DefaultProvider, Hermetic: true} }

var providerIDRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ValidProviderID is the syntax of a provider id.
func ValidProviderID(v string) bool { return providerIDRE.MatchString(v) }

// ValidateAgent checks the patch's syntax.
func ValidateAgent(a *AgentPatch) error {
	if a != nil && a.Provider != nil && !ValidProviderID(*a.Provider) {
		return fmt.Errorf("appsettings: agent.provider: invalid value %q", *a.Provider)
	}
	return nil
}

// ReadAgent reads the agent.* leaves from stored on top of DefaultAgent().
func ReadAgent(stored map[string]json.RawMessage) Agent {
	result := DefaultAgent()
	LeafValid(stored, "agent.provider", &result.Provider, ValidProviderID)
	LeafValid(stored, "agent.hermetic", &result.Hermetic, AlwaysValid[bool])
	return result
}

// UpsertAgent writes the patched agent.* leaves.
func UpsertAgent(tx *sql.Tx, a *AgentPatch) error {
	if a == nil {
		return nil
	}
	if err := UpsertOptional(tx, "agent.provider", a.Provider); err != nil {
		return err
	}
	return UpsertOptional(tx, "agent.hermetic", a.Hermetic)
}
