package dbmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func inputDecls(in []string) []agentapi.InputDecl {
	out := make([]agentapi.InputDecl, len(in))
	for i, name := range in {
		out[i] = agentapi.InputDecl{Name: name}
	}
	return out
}

// apiInputPayload is a declared input as the card shows it. Never a value.
type apiInputPayload struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Origin string `json:"origin"`
	KeptID string `json:"keptId,omitempty"`
	// KeepMinutes is how long a typed value is kept.
	KeepMinutes int `json:"keepMinutes"`
}

func apiInputsPayload(p *agentapi.Planned, keepMinutes int) []apiInputPayload {
	out := make([]apiInputPayload, len(p.Inputs))
	for i, in := range p.Inputs {
		out[i] = apiInputPayload{
			Name: in.Name, State: string(in.State), Origin: in.Origin, KeptID: in.KeptID,
			KeepMinutes: keepMinutes,
		}
	}
	return out
}

// keepMinutes is the retention a kept typed value gets, in whole minutes.
func (s *Server) keepMinutes() int {
	if s.cfg.Kept == nil {
		return int(keptvars.DefaultTTL / time.Minute)
	}
	return int(s.cfg.Kept.Retention() / time.Minute)
}

func approvalInputsOf(p agentapi.Planned) []ApprovalInput {
	out := make([]ApprovalInput, len(p.Inputs))
	for i, in := range p.Inputs {
		out[i] = ApprovalInput{Name: in.Name, Needed: in.State == agentapi.InputNeeded}
	}
	return out
}

// noValueKept is the tail of an agent text for a request that did not go ahead: which asked-for inputs
// stayed without a value. "" without a needed input.
func noValueKept(p agentapi.Planned) string {
	var names []string
	for _, in := range p.Inputs {
		if in.State == agentapi.InputNeeded {
			names = append(names, fmt.Sprintf("%q", in.Name))
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "; no value was set for " + strings.Join(names, ", ")
}

// withNoValueKept appends the no-value note to a refusal result.
func withNoValueKept(res *mcp.CallToolResult, p agentapi.Planned) *mcp.CallToolResult {
	tail := noValueKept(p)
	if res == nil || tail == "" || len(res.Content) != 1 {
		return res
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		tc.Text += tail
	}
	return res
}

// inputKeepError is a typed value the store refused. The text carries the store's fixed message, never a value.
type inputKeepError struct {
	name string
	err  error
}

func (e *inputKeepError) Error() string {
	return fmt.Sprintf("the value for %q could not be set (%s); nothing was sent", e.name, e.err)
}

// keepInputs settles the values the user typed: each is stored under its kept name, bound to the request's
// origin, and replaces a stored one. It returns the status of every declared input. Called after approval
// with the item's watch already stopped, so replacing a stored value cancels other waiting items but not
// this one.
func (s *Server) keepInputs(ctx context.Context, plan agentapi.Planned, supplied SuppliedInputs) ([]agentapi.InputStatus, error) {
	defer supplied.Wipe()
	if len(plan.Inputs) == 0 {
		return nil, nil
	}
	src := keptSourceOf(ctx, plan)
	src.Kind = keptvars.SourceUser
	scope := keptScope(ctx)
	statuses := make([]agentapi.InputStatus, 0, len(plan.Inputs))
	for _, in := range plan.Inputs {
		st := agentapi.InputStatus{Name: in.Name, Status: "reused"}
		typed, ok := supplied.Take(in.Name)
		if !ok {
			statuses = append(statuses, st)
			continue
		}
		raw, err := jsonString(typed)
		clear(typed)
		if err != nil {
			return nil, &inputKeepError{name: in.Name, err: err}
		}
		if s.cfg.Kept == nil {
			clear(raw)
			return nil, &inputKeepError{name: in.Name, err: keptvars.ErrClosed}
		}
		m, perr := s.cfg.Kept.Put(keptvars.Put{Scope: scope, Name: in.Name, Value: raw, Secret: true, Source: src, Origin: plan.Origin})
		clear(raw)
		if perr != nil {
			return nil, &inputKeepError{name: in.Name, err: perr}
		}
		st.Status = "supplied"
		if in.State == agentapi.InputStored {
			st.Status = "replaced"
		}
		st.ExpiresInSeconds = secondsLeft(m.ExpiresAt)
		statuses = append(statuses, st)
	}
	return statuses, nil
}

func secondsLeft(at time.Time) int { return max(int(math.Ceil(time.Until(at).Seconds())), 0) }

// fillReused sets the remaining time of the inputs that were reused, read from the store after the re-plan.
func (s *Server) fillReused(ctx context.Context, statuses []agentapi.InputStatus) {
	if s.cfg.Kept == nil {
		return
	}
	for i, st := range statuses {
		if st.ExpiresInSeconds > 0 {
			continue
		}
		if e, ok := s.cfg.Kept.Get(keptScope(ctx), st.Name); ok {
			statuses[i].ExpiresInSeconds = secondsLeft(e.ExpiresAt)
		}
	}
}

// jsonString encodes a typed value as a JSON string. The intermediate Go string is not wiped (see SuppliedInputs).
func jsonString(value []byte) (json.RawMessage, error) {
	return json.Marshal(string(value))
}
