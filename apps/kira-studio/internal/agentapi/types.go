// Package agentapi plans and sends the free-form API requests an agent asks for. It enforces what an
// agent may use: only variables the user marked available, a secret only toward its allowed hosts,
// TLS verification always on, and responses masked before they leave. It never imports the bridge or
// the MCP server.
package agentapi

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apifiles"
)

// NameValue is one header, query or form entry.
type NameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Input is what an agent asked for, exactly as sent. It may hold {{name}} references.
type Input struct {
	EnvironmentID string
	Method        string
	URL           string
	Headers       []NameValue
	Query         []NameValue
	Body          string
	ContentType   string
	Form          []NameValue
	// TimeoutSeconds, MaxResponseBytes and FollowRedirects take their defaults when zero or nil.
	TimeoutSeconds   int
	MaxResponseBytes int
	FollowRedirects  *bool
}

// Credential is a secret a request uses, with the hosts it may go to. Never a value.
type Credential struct {
	Name  string   `json:"name"`
	Hosts []string `json:"hosts"`
}

// VarUse is a variable the request references. A secret's value is always "".
type VarUse struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// EnvInfo names the environment a request runs in.
type EnvInfo struct {
	ID    string
	Name  string
	Color string
	// AutoApproveReads is the environment's agentAutoApproveReads.
	AutoApproveReads bool
}

// Planned is a request checked against the policy but not yet sent. It holds no secret value: URL,
// headers, body and form keep a secret as {{name}}.
type Planned struct {
	Env         EnvInfo
	Method      string
	URL         string
	Host        string
	Query       []NameValue
	Headers     []NameValue
	Body        string
	ContentType string
	Form        []NameValue
	Variables   []VarUse
	Credentials []Credential

	FollowRedirects  bool
	TimeoutSeconds   int
	MaxResponseBytes int

	// base is the URL template before the agent's query entries (extra) are appended.
	base  string
	extra []NameValue
	creds []credential
}

// HasBody reports whether the request sends a body.
func (p Planned) HasBody() bool { return p.Body != "" || len(p.Form) > 0 }

// Redirect is one followed hop.
type Redirect struct {
	Status int    `json:"status"`
	URL    string `json:"url"`
}

// Result is a response, masked and capped. Body is empty for a binary response.
type Result struct {
	Status      int         `json:"status"`
	StatusText  string      `json:"statusText"`
	Headers     []NameValue `json:"headers"`
	ContentType string      `json:"contentType"`
	Body        string      `json:"body,omitempty"`
	Binary      bool        `json:"binary,omitempty"`
	BodyBytes   int         `json:"bodyBytes"`
	Truncated   bool        `json:"truncated"`
	ElapsedMs   int         `json:"elapsedMs"`
	FinalURL    string      `json:"finalUrl"`
	Redirects   []Redirect  `json:"redirects"`
}

// VarView is an available variable as the listing shows it. Never a value.
type VarView struct {
	Name   string   `json:"name"`
	Secret bool     `json:"secret"`
	Hosts  []string `json:"hosts,omitempty"`
}

// EnvView is an environment with at least one variable available to agents.
type EnvView struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Color            string    `json:"color"`
	AutoApproveReads bool      `json:"autoApproveReads"`
	Variables        []VarView `json:"variables"`
}

// Globals are the user's own API settings an agent request inherits.
type Globals struct {
	HTTPVersion   string
	MaxResponseMb int
	MaxRedirects  int
}

// EnvSource reads environment files (apifiles.Store satisfies it).
type EnvSource interface {
	EnvironmentList() ([]apifiles.EnvironmentRef, error)
	EnvironmentByID(id string) (apifiles.EnvironmentRef, error)
}

// SecretOpener decrypts the Api secrets (apifiles.Secrets satisfies it).
type SecretOpener interface {
	OpenAll() (*apifiles.Opened, error)
}

// OpRunner runs a logged operation (adapterhost.Host satisfies it).
type OpRunner interface {
	RunOp(ctx context.Context, spec adapterhost.OpSpec, fn func(context.Context, *adapters.OpCtx) (any, error)) (string, any, error)
}

// Service plans and sends agent requests.
type Service struct {
	Envs    EnvSource
	Secrets SecretOpener
	Ops     OpRunner
	// Globals is read fresh per send.
	Globals func() (Globals, error)
}
