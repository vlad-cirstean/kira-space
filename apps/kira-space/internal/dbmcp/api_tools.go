package dbmcp

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// APIRequester is dbmcp's consumer-declared view of the agent API service: *agentapi.Service.
type APIRequester interface {
	// Environments lists the environments that share variables with agents; never a value.
	Environments() ([]agentapi.EnvView, error)
	// Plan checks a request against the policy without sending it.
	Plan(in agentapi.Input) (agentapi.Planned, error)
	// Send re-plans and sends the request, refusing when approved (the plan the user saw) no longer
	// matches; the result is masked and capped.
	Send(ctx context.Context, in agentapi.Input, approved *agentapi.Planned) (agentapi.Result, error)
}

// APISettings is the user's switch for the API tools.
type APISettings struct {
	// Enabled lists the tools to agents.
	Enabled bool
	// Approval is "always" (each request waits for the user) or "never".
	Approval string
}

func (s *Server) apiSettings() APISettings {
	if s.cfg.APISettings == nil {
		return APISettings{Approval: "always"}
	}
	return s.cfg.APISettings()
}

func (s *Server) apiEnabled() bool { return s.cfg.API != nil && s.apiSettings().Enabled }

type apiPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type apiRequestArgs struct {
	EnvironmentID    string    `json:"environmentId,omitempty"`
	Method           string    `json:"method"`
	URL              string    `json:"url"`
	Headers          []apiPair `json:"headers,omitempty"`
	Query            []apiPair `json:"query,omitempty"`
	Body             string    `json:"body,omitempty"`
	ContentType      string    `json:"contentType,omitempty"`
	Form             []apiPair `json:"form,omitempty"`
	TimeoutSeconds   int       `json:"timeoutSeconds,omitempty"`
	MaxResponseBytes int       `json:"maxResponseBytes,omitempty"`
	FollowRedirects  *bool     `json:"followRedirects,omitempty"`
}

func pairs(in []apiPair) []agentapi.NameValue {
	out := make([]agentapi.NameValue, len(in))
	for i, p := range in {
		out[i] = agentapi.NameValue(p)
	}
	return out
}

func (a apiRequestArgs) input() agentapi.Input {
	return agentapi.Input{
		EnvironmentID: a.EnvironmentID, Method: a.Method, URL: a.URL,
		Headers: pairs(a.Headers), Query: pairs(a.Query), Body: a.Body, ContentType: a.ContentType, Form: pairs(a.Form),
		TimeoutSeconds: a.TimeoutSeconds, MaxResponseBytes: a.MaxResponseBytes, FollowRedirects: a.FollowRedirects,
	}
}

const apiRequestsOff = "API requests are turned off in Kira Space's database MCP settings"

func (s *Server) apiListEnvironments(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	if !s.apiEnabled() {
		return errResult(apiRequestsOff)
	}
	envs, err := s.cfg.API.Environments()
	if err != nil {
		return errResult("the environments could not be read")
	}
	return jsonResult(map[string]any{
		"environments": envs,
		"note":         "Requests without environmentId are allowed; they can use no variables.",
	})
}

func (s *Server) apiRequest(ctx context.Context, _ *mcp.CallToolRequest, a apiRequestArgs) (*mcp.CallToolResult, any, error) {
	settings := s.apiSettings()
	if !settings.Enabled {
		return errResult(apiRequestsOff)
	}
	in := a.input()
	plan, err := s.cfg.API.Plan(in)
	if err != nil {
		return errResult(err.Error())
	}
	if needsAPIApproval(settings, plan) {
		if res, err := s.approveAPI(ctx, plan); res != nil || err != nil {
			return res, nil, err
		}
	}
	out, err := s.cfg.API.Send(ctx, in, &plan)
	if err != nil {
		return errResult(err.Error())
	}
	return jsonResult(out)
}

// needsAPIApproval says whether the request waits for the user: always, unless the user turned
// approval off or the environment lets body-less reads through.
func needsAPIApproval(st APISettings, p agentapi.Planned) bool {
	if st.Approval == "never" {
		return false
	}
	return !(p.Env.AutoApproveReads && isReadMethod(p.Method) && !p.HasBody())
}

func isReadMethod(m string) bool { return m == "GET" || m == "HEAD" || m == "OPTIONS" }

// approveAPI asks the user. A non-nil result stops the call.
func (s *Server) approveAPI(ctx context.Context, plan agentapi.Planned) (*mcp.CallToolResult, error) {
	outcome := s.cfg.Approvals.Request(ctx, ApprovalRequest{
		Subject: approvals.KindAPIRequest, ConnectionName: plan.Host, Kind: "http", Class: adapters.ClassUnknown,
		Statement: plan.Method + " " + plan.URL, Reason: ApprovalReasonPermission, Origin: originFrom(ctx), API: &plan,
	})
	if outcome != ApprovalApproved {
		res, _, err := approvalOutcomeError(outcome, "request")
		return res, err
	}
	return nil, nil
}

// addAPITools registers api_list_environments and api_request.
func (s *Server) addAPITools(srv *mcp.Server, src llmprompts.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "api_list_environments", Description: llmprompts.DbmcpToolAPIListEnvironments.Text(src, llmprompts.NoVars{}),
	}, withPanicRecovery(s, "api_list_environments", s.apiListEnvironments))
	limits := map[string]any{
		"timeoutSeconds":   llmprompts.LimitVars{Default: 30, Max: 300},
		"maxResponseBytes": llmprompts.LimitVars{Default: 64 << 10, Max: 512 << 10},
	}
	sch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[apiRequestArgs](src, llmprompts.DbmcpToolAPIRequest.ID(), limits), "method", "url")
	sch.Properties["method"].Enum = []any{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"}
	mcp.AddTool(srv, &mcp.Tool{
		Name: "api_request", Description: llmprompts.DbmcpToolAPIRequest.Text(src, llmprompts.NoVars{}), InputSchema: sch,
	}, withPanicRecovery(s, "api_request", s.apiRequest))
}

// --- approval item ---

const apiBodyDisplayBytes = 64 << 10

type apiEnvPayload struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type apiHeaderPayload struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Masked bool   `json:"masked"`
}

type apiAuthPayload struct {
	Scheme  string `json:"scheme"`
	Display string `json:"display"`
}

type apiBodyPayload struct {
	ContentType string    `json:"contentType"`
	Text        string    `json:"text"`
	Truncated   bool      `json:"truncated"`
	Form        []apiPair `json:"form"`
}

type apiPayload struct {
	Method          string                `json:"method"`
	URL             string                `json:"url"`
	Host            string                `json:"host"`
	Environment     *apiEnvPayload        `json:"environment"`
	Query           []apiPair             `json:"query"`
	Headers         []apiHeaderPayload    `json:"headers"`
	Auth            *apiAuthPayload       `json:"auth"`
	Body            *apiBodyPayload       `json:"body"`
	Variables       []agentapi.VarUse     `json:"variables"`
	Credentials     []agentapi.Credential `json:"credentials"`
	FollowRedirects bool                  `json:"followRedirects"`
	TimeoutSeconds  int                   `json:"timeoutSeconds"`
}

// apiItemParts projects a planned request into its approval item. It reads the plan only, which holds
// no secret value.
func apiItemParts(p *agentapi.Planned) (approvals.Target, approvals.Operation, []byte) {
	if p == nil {
		p = &agentapi.Planned{}
	}
	location := "No environment"
	color := "none"
	if p.Env.ID != "" {
		location, color = p.Env.Name, p.Env.Color
	}
	target := approvals.Target{Name: p.Host, Kind: "http", Color: color, Location: location}
	op := approvals.Operation{Class: "request", Tone: methodTone(p.Method)}
	return target, op, mustJSON(apiPayloadOf(p))
}

func methodTone(m string) string {
	switch m {
	case "GET", "HEAD", "OPTIONS":
		return "neutral"
	case "DELETE":
		return "danger"
	}
	return "caution"
}

func apiPayloadOf(p *agentapi.Planned) apiPayload {
	out := apiPayload{
		Method: p.Method, URL: p.URL, Host: p.Host, Query: nonNilPairs(pairsOf(p.Query)),
		Headers: apiHeaders(p), Variables: nonNilVars(p.Variables), Credentials: nonNilCreds(p.Credentials),
		FollowRedirects: p.FollowRedirects, TimeoutSeconds: p.TimeoutSeconds,
	}
	if p.Env.ID != "" {
		out.Environment = &apiEnvPayload{ID: p.Env.ID, Name: p.Env.Name, Color: p.Env.Color}
	}
	for _, h := range out.Headers {
		if strings.EqualFold(h.Name, "Authorization") {
			scheme, _, _ := strings.Cut(strings.TrimSpace(h.Value), " ")
			out.Auth = &apiAuthPayload{Scheme: scheme, Display: h.Value}
		}
	}
	out.Body = apiBody(p)
	return out
}

func pairsOf(in []agentapi.NameValue) []apiPair {
	out := make([]apiPair, len(in))
	for i, p := range in {
		out[i] = apiPair(p)
	}
	return out
}

func nonNilPairs(in []apiPair) []apiPair {
	if in == nil {
		return []apiPair{}
	}
	return in
}

func nonNilVars(in []agentapi.VarUse) []agentapi.VarUse {
	if in == nil {
		return []agentapi.VarUse{}
	}
	return in
}

func nonNilCreds(in []agentapi.Credential) []agentapi.Credential {
	if in == nil {
		return []agentapi.Credential{}
	}
	return in
}

// apiHeaders marks a header whose value references a credential.
func apiHeaders(p *agentapi.Planned) []apiHeaderPayload {
	secret := map[string]bool{}
	for _, c := range p.Credentials {
		secret[c.Name] = true
	}
	out := make([]apiHeaderPayload, len(p.Headers))
	for i, h := range p.Headers {
		masked := false
		for _, n := range apivars.Names(h.Value) {
			masked = masked || secret[n]
		}
		out[i] = apiHeaderPayload{Name: h.Name, Value: h.Value, Masked: masked}
	}
	return out
}

func apiBody(p *agentapi.Planned) *apiBodyPayload {
	switch {
	case p.Body != "":
		text, cut := capUTF8(p.Body, apiBodyDisplayBytes)
		return &apiBodyPayload{ContentType: bodyContentType(p), Text: text, Truncated: cut}
	case len(p.Form) > 0:
		return &apiBodyPayload{ContentType: "application/x-www-form-urlencoded", Form: pairsOf(p.Form)}
	}
	return nil
}

// bodyContentType is the contentType argument, or the Content-Type header the agent set.
func bodyContentType(p *agentapi.Planned) string {
	if p.ContentType != "" {
		return p.ContentType
	}
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, "Content-Type") {
			return h.Value
		}
	}
	return ""
}

func capUTF8(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n], true
}
