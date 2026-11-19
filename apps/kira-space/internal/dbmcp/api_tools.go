package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
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
	// Scripts lets agents attach JavaScript to api_request (dbMcp.apiScripts). Listed in the schema only
	// while the server also has an engine.
	Scripts bool
	// ReviewResponses holds API responses for the user's review (dbMcp.apiReviewResponses); an environment's
	// agentReview overrides it per request class.
	ReviewResponses bool
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
	// Script is set by an agent that attaches JavaScript to the request.
	Script *apiScriptArgs `json:"script,omitempty"`
	// Inputs are values only the user has; the user types them in the approval.
	Inputs []apiInput `json:"inputs,omitempty"`
}

func pairs(in []apiPair) []agentapi.NameValue {
	out := make([]agentapi.NameValue, len(in))
	for i, p := range in {
		out[i] = agentapi.NameValue(p)
	}
	return out
}

func (a apiRequestArgs) input(scope string) agentapi.Input {
	return agentapi.Input{
		EnvironmentID: a.EnvironmentID, Method: a.Method, URL: a.URL,
		Headers: pairs(a.Headers), Query: pairs(a.Query), Body: a.Body, ContentType: a.ContentType, Form: pairs(a.Form),
		TimeoutSeconds: a.TimeoutSeconds, MaxResponseBytes: a.MaxResponseBytes, FollowRedirects: a.FollowRedirects, Scope: scope,
		Inputs: inputDecls(a.Inputs), ScriptReads: scriptReads(a.Script),
	}
}

const apiRequestsOff = "API requests are turned off in Kira Space's Studio MCP settings"

func (s *Server) apiListEnvironments(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	if !s.apiEnabled() {
		return errResult(apiRequestsOff)
	}
	envs, err := s.cfg.API.Environments()
	if err != nil {
		return errResult("the environments could not be read")
	}
	return jsonResult(map[string]any{
		"environments": s.envListing(envs),
		"note":         "Requests without environmentId are allowed; they can use no variables.",
	})
}

// apiEnvListing is one api_list_environments entry: the view plus which request classes hold their
// responses for the user.
type apiEnvListing struct {
	agentapi.EnvView
	ReviewResults struct {
		Read  bool `json:"read"`
		Write bool `json:"write"`
	} `json:"reviewResults"`
}

func (s *Server) envListing(envs []agentapi.EnvView) []apiEnvListing {
	out := make([]apiEnvListing, len(envs))
	for i, v := range envs {
		info := agentapi.EnvInfo{Review: v.Review}
		out[i].EnvView = v
		out[i].ReviewResults.Read = s.apiReviewSetting(info, true)
		out[i].ReviewResults.Write = s.apiReviewSetting(info, false)
	}
	return out
}

func (s *Server) apiRequest(ctx context.Context, _ *mcp.CallToolRequest, a apiRequestArgs) (*mcp.CallToolResult, any, error) {
	settings := s.apiSettings()
	if !settings.Enabled {
		return errResult(apiRequestsOff)
	}
	in := a.input(keptScope(ctx))
	var sc *apiScripts
	if a.Script != nil {
		var refusal string
		if sc, refusal = s.openScripts(ctx, a.Script); refusal != "" {
			return errResult(refusal)
		}
		if sc.kept, refusal = s.keptRunFor(in.Scope, sc); refusal != "" {
			return errResult(refusal)
		}
		if sc.Prepare != "" {
			var text string
			if in, text = s.prepareAPI(ctx, in, sc); text != "" {
				return errResult(text)
			}
		}
	}
	plan, err := s.cfg.API.Plan(in)
	if err != nil {
		return errResult(err.Error())
	}
	var ap apiApproval
	// A missing input forces the card whatever the rules say: the card is the only place the value is typed.
	if needsAPIApproval(settings, plan) || plan.MissingInputs() > 0 {
		var res *mcp.CallToolResult
		if ap, res, err = s.approveAPI(ctx, plan, sc); res != nil || err != nil {
			return res, nil, err
		}
	}
	statuses, plan, refusal := s.settleInputs(ctx, in, plan, sc, ap.supplied)
	if refusal != "" {
		return errResult(refusal)
	}
	if sc.hasTransform() {
		in.ScriptBodyBytes = transformBodyBytes
	}
	raw, err := s.cfg.API.Send(ctx, in, &plan)
	if err != nil {
		return errResult(err.Error())
	}
	o := apiOutcome{Raw: raw, Inputs: statuses}
	if sc.hasTransform() {
		o = s.transformAPI(ctx, plan, sc, raw)
		o.Inputs = statuses
	}
	return s.deliverAPI(ctx, plan, sc, o, ap.requested)
}

// settleInputs stores what the user typed, then plans again so the plan binds the stored entries' ids (a needed
// input had none, a replaced one has a new id) and the scripts read the fresh entries. The refusal is
// non-empty when a value could not be kept or the request no longer plans.
func (s *Server) settleInputs(ctx context.Context, in agentapi.Input, plan agentapi.Planned, sc *apiScripts, supplied SuppliedInputs) ([]agentapi.InputStatus, agentapi.Planned, string) {
	if len(plan.Inputs) == 0 {
		supplied.Wipe()
		return nil, plan, ""
	}
	statuses, err := s.keepInputs(ctx, plan, supplied)
	if err != nil {
		return nil, plan, err.Error()
	}
	next, err := s.cfg.API.Plan(in)
	if err != nil {
		return nil, plan, err.Error()
	}
	names := make([]string, len(plan.Inputs))
	for i, u := range plan.Inputs {
		names[i] = u.Name
	}
	if sc != nil {
		sc.kept.rebind(names)
	}
	s.fillReused(ctx, statuses)
	return statuses, next, ""
}

// openScripts validates the script argument and syntax-checks both scripts before anything runs.
// A non-empty string is the refusal text.
func (s *Server) openScripts(ctx context.Context, a *apiScriptArgs) (*apiScripts, string) {
	if !s.scriptsOn() {
		return nil, apiScriptsOff
	}
	sc := newAPIScripts(a)
	switch {
	case sc.Prepare == "" && sc.Transform == "":
		return nil, apiScriptEmpty
	case len(sc.Input) > 0 && sc.Prepare == "":
		return nil, apiScriptNoPrep
	case len(sc.Input) > apiScriptInputBytes:
		return nil, fmt.Sprintf("script.input is %d bytes; the limit is %d", len(sc.Input), apiScriptInputBytes)
	}
	for _, p := range []struct {
		stage scriptStage
		src   string
	}{{stagePrepare, sc.Prepare}, {stageTransform, sc.Transform}} {
		if p.src == "" {
			continue
		}
		if text := s.checkScript(ctx, p.stage, p.src); text != "" {
			return nil, text
		}
	}
	return sc, ""
}

// needsAPIApproval says whether the request waits for the user: never when an allow rule matched; otherwise
// always, unless the user turned approval off or the environment lets body-less reads through.
func needsAPIApproval(st APISettings, p agentapi.Planned) bool {
	if p.AllowedByRule {
		return false
	}
	if st.Approval == "never" {
		return false
	}
	return !(p.Env.AutoApproveReads && isReadMethod(p.Method) && !p.HasBody())
}

func isReadMethod(m string) bool { return m == "GET" || m == "HEAD" || m == "OPTIONS" }

// apiApproval is what an approved card hands back: the review tick and the typed values.
type apiApproval struct {
	requested bool
	supplied  SuppliedInputs
}

// approveAPI asks the user. A non-nil result stops the call; requested says the user ticked "review the
// response" on the card and supplied holds the values the user typed.
func (s *Server) approveAPI(ctx context.Context, plan agentapi.Planned, sc *apiScripts) (apiApproval, *mcp.CallToolResult, error) {
	ctx, stop := s.watchKept(ctx, keptUsesOf(plan, sc))
	defer stop()
	outcome, choice, supplied := s.cfg.Approvals.RequestInputs(ctx, ApprovalRequest{
		Subject: approvals.KindAPIRequest, ConnectionName: plan.Host, Kind: "http", Class: adapters.ClassUnknown,
		Statement: plan.Method + " " + plan.URL, Reason: ApprovalReasonPermission, Origin: originFrom(ctx), API: &plan, APIScript: sc,
		OfferReview: true, ReviewOn: s.apiReviewSetting(plan.Env, apiReadClass(plan)), Inputs: approvalInputsOf(plan),
	})
	if outcome != ApprovalApproved {
		supplied.Wipe()
		var gone *keptGoneError
		if errors.As(context.Cause(ctx), &gone) {
			res, _, err := errResult(gone.Error())
			return apiApproval{}, res, err
		}
		res, _, err := approvalOutcomeError(outcome, "request")
		return apiApproval{}, withNoValueKept(res, plan), err
	}
	return apiApproval{requested: choice.ReviewResult, supplied: supplied}, nil, nil
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
	fixScriptSchema(sch)
	sch.Properties["method"].Enum = []any{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"}
	scriptNote := " " + llmprompts.DbmcpToolAPIRequestReview.Text(src, llmprompts.NoVars{})
	if s.scriptsOn() {
		scriptNote += " " + llmprompts.DbmcpToolAPIRequestScripts.Text(src, llmprompts.NoVars{})
	} else {
		delete(sch.Properties, "script")
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name: "api_request", Description: llmprompts.DbmcpToolAPIRequest.Text(src, llmprompts.NoVars{}) + scriptNote, InputSchema: sch,
	}, withPanicRecovery(s, "api_request", s.apiRequest))
}

// fixScriptSchema pins script to an object and makes script.input any JSON: the generated schemas are a
// nullable object and a byte array.
func fixScriptSchema(sch *jsonschema.Schema) {
	script := sch.Properties["script"]
	script.Type, script.Types = "object", nil
	input := script.Properties["input"]
	script.Properties["input"] = &jsonschema.Schema{Description: input.Description}
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
	Kept            []agentapi.KeptUse    `json:"kept,omitempty"`
	Script          *apiScriptPayload     `json:"script"`
	// Inputs are the values the card asks the user to type, or reuses; never a value.
	Inputs []apiInputPayload `json:"inputs"`
	// ReviewOffered shows "review the response"; ReviewResult says the settings already hold it.
	ReviewOffered bool `json:"reviewOffered"`
	ReviewResult  bool `json:"reviewResult"`
}

// apiResponsePayload is the review item's snapshot payload. It fans out to every Studio window, so it
// carries no body, text, raw response or script.
type apiResponsePayload struct {
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	Host        string         `json:"host"`
	Environment *apiEnvPayload `json:"environment"`
	Status      int            `json:"status"`
	StatusText  string         `json:"statusText"`
	IsError     bool           `json:"isError"`
	Scripted    bool           `json:"scripted"`
	Bytes       int            `json:"bytes"`
	RawBytes    int            `json:"rawBytes"`
	Class       string         `json:"class"`
	Reason      string         `json:"reason"`
}

func apiResponsePayloadOf(r ApprovalRequest) apiResponsePayload {
	p, held, meta := r.API, r.Response, r.APIReview
	if p == nil {
		p = &agentapi.Planned{}
	}
	if held == nil {
		held = &ApprovalResponse{}
	}
	if meta == nil {
		meta = &apiReviewMeta{}
	}
	out := apiResponsePayload{
		Method: p.Method, URL: p.URL, Host: p.Host, Status: meta.Status, StatusText: meta.StatusText,
		IsError: held.IsError, Scripted: held.Script != nil, Bytes: len(held.Text), RawBytes: len(held.Raw),
		Class: "write", Reason: meta.Reason,
	}
	if meta.Read {
		out.Class = "read"
	}
	if p.Env.ID != "" {
		out.Environment = &apiEnvPayload{ID: p.Env.ID, Name: p.Env.Name, Color: p.Env.Color}
	}
	return out
}

// apiItemParts projects a planned request into its approval item. It reads the plan only, which holds
// no secret value.
func apiItemParts(r ApprovalRequest) (approvals.Target, approvals.Operation, []byte) {
	p := r.API
	if p == nil {
		p = &agentapi.Planned{}
	}
	target, op := apiTargetOp(p)
	payload := apiPayloadOf(p)
	payload.Inputs = apiInputsPayload(p)
	payload.Script = apiScriptPayloadOf(r.APIScript)
	if r.APIScript != nil && r.APIScript.kept != nil {
		payload.Kept = mergeKept(payload.Kept, r.APIScript.kept.uses)
	}
	payload.ReviewOffered, payload.ReviewResult = r.OfferReview, r.ReviewOn
	return target, op, mustJSON(payload)
}

// apiTargetOp is the host, environment and method an API item shows in its header.
func apiTargetOp(p *agentapi.Planned) (approvals.Target, approvals.Operation) {
	if p == nil {
		p = &agentapi.Planned{}
	}
	location := "No environment"
	color := "none"
	if p.Env.ID != "" {
		location, color = p.Env.Name, p.Env.Color
	}
	target := approvals.Target{Name: p.Host, Kind: "http", Color: color, Location: location}
	return target, approvals.Operation{Class: "request", Tone: methodTone(p.Method)}
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
	out.Kept = displayedKept(p.Kept, out.Body)
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

// displayedKept drops the body sites the capped body text no longer holds.
func displayedKept(uses []agentapi.KeptUse, body *apiBodyPayload) []agentapi.KeptUse {
	out := make([]agentapi.KeptUse, len(uses))
	shown := 0
	if body != nil {
		shown = len(utf16.Encode([]rune(body.Text)))
	}
	for i, u := range uses {
		u.Sites = slices.DeleteFunc(slices.Clone(u.Sites), func(st agentapi.KeptSite) bool { return st.Field == "body" && st.To > shown })
		out[i] = u
	}
	return out
}
