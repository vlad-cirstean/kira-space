// Package agentapi plans and sends the free-form API requests an agent asks for. It enforces what an
// agent may use: only variables the user marked available, a secret only toward its allowed hosts,
// TLS verification always on, and responses masked before they leave. It never imports the bridge or
// the MCP server.
package agentapi

import (
	"context"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apiaccess"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
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
	// Scope is the kept-variable scope of the caller; empty means none.
	Scope string
	// ScriptBodyBytes is the masked body cap for a response a transform script reads (0: no transform,
	// MaxResponseBytes applies). Bounded to MaxScriptBodyBytes; not part of sameRequest.
	ScriptBodyBytes int
	// Inputs are the values the agent asks the user for; each is used as {{kept.<name>}}.
	Inputs []InputDecl
	// ScriptReads names the kept variables the request's scripts read by literal kept.get.
	ScriptReads []string
	// Once holds the values the user typed for this request only, by input name. They are read before the store
	// and never written to it; the caller wipes their bytes once the request ends.
	Once map[string]keptvars.Entry `json:"-"`
}

// MaxScriptBodyBytes bounds ScriptBodyBytes.
const MaxScriptBodyBytes = 2 << 20

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
	// Review is the environment's agentReview: inherit, review or send per class; "" is inherit.
	Review ReviewSetting
}

// ReviewSetting is an environment's response review per request class.
type ReviewSetting struct {
	Read, Write string
}

// Planned is a request checked against the policy but not yet sent. It holds no secret value: URL,
// headers, body and form keep a secret as {{name}}.
type Planned struct {
	access      apiaccess.Policy
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
	// Kept lists the kept variables the request uses, as they stood at plan time.
	Kept []KeptUse
	// Inputs are the declared inputs and whether each has a stored value; Origin is the request's
	// scheme://host[:port], the key a user value is bound to.
	Inputs []InputUse
	Origin string

	// AllowedByRule: an allow rule matched, so the request needs no approval.
	AllowedByRule bool

	FollowRedirects  bool
	TimeoutSeconds   int
	MaxResponseBytes int

	// bodyCap is the masked body cap Send applies; 0 means MaxResponseBytes.
	bodyCap int

	// base is the URL template before the agent's query entries (extra) are appended.
	base  string
	extra []NameValue
	creds []credential
	// keptValues holds the kept values for one send; set only by Send's own plan.
	keptValues map[string]keptValue
}

// sameRequest reports whether o would send the same method, URL, headers, query, body and form.
func (p Planned) sameRequest(o Planned) bool {
	return p.Method == o.Method && p.URL == o.URL && p.Body == o.Body && p.ContentType == o.ContentType &&
		slices.Equal(p.Headers, o.Headers) && slices.Equal(p.Query, o.Query) && slices.Equal(p.Form, o.Form)
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
	// Inputs says what happened to each declared input; set by the caller that kept them.
	Inputs []InputStatus `json:"inputs,omitempty"`

	// Mask is the request's text masker: it removes secrets from anything derived from the response,
	// such as script output or error text. Nil on a zero Result.
	Mask func(string) string `json:"-"`
}

// InputStatus is what became of a declared input: supplied, reused, replaced or once (used for this request only,
// ExpiresInSeconds 0).
type InputStatus struct {
	Name             string `json:"name"`
	Status           string `json:"status"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
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
	// Review stays off the wire of ApiConfigService.AgentAccess; dbmcp resolves it for agents.
	Review ReviewSetting `json:"-"`
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
	// Access is the user's API access rules, read fresh per plan; nil means no rules.
	Access func() (apiaccess.Policy, error)
	// Kept resolves {{kept.<name>}} references; nil refuses them all.
	Kept KeptReader
}
