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

// apiInput is a value the agent asks the user for in api_request.
type apiInput struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	// Reuse: the agent plans more requests with this value, so the card suggests keeping it.
	Reuse bool `json:"reuse,omitempty"`
}

func inputDecls(in []apiInput) []agentapi.InputDecl {
	out := make([]agentapi.InputDecl, len(in))
	for i, d := range in {
		out[i] = agentapi.InputDecl(d)
	}
	return out
}

// scriptReads lists the names the scripts read by literal kira.getValue: a declared input one of them reads
// counts as used.
func scriptReads(a *apiScriptArgs) []string {
	if a == nil {
		return nil
	}
	var out []string
	for _, src := range []string{a.Prepare, a.Transform} {
		for _, st := range keptvars.ScanScript(src) {
			if st.Op == "getValue" {
				out = append(out, st.Name)
			}
		}
	}
	return out
}

// apiInputPayload is a declared input as the card shows it. Never a value.
type apiInputPayload struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Origin string `json:"origin"`
	KeptID string `json:"keptId,omitempty"`
	// Mode is the choice the card starts on: "keep" when the agent asked for reuse, else "once".
	Mode string `json:"mode"`
	// KeepMinutes is how long "keep" holds the value.
	KeepMinutes int `json:"keepMinutes"`
}

func apiInputsPayload(p *agentapi.Planned, keepMinutes int) []apiInputPayload {
	out := make([]apiInputPayload, len(p.Inputs))
	for i, in := range p.Inputs {
		mode := "once"
		if in.Reuse {
			mode = "keep"
		}
		out[i] = apiInputPayload{
			Name: in.Name, Label: in.Label, State: string(in.State), Origin: in.Origin, KeptID: in.KeptID,
			Mode: mode, KeepMinutes: keepMinutes,
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
		out[i] = ApprovalInput{Name: in.Name, Label: in.Label, Needed: in.State == agentapi.InputNeeded}
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
	return "; no value was kept for " + strings.Join(names, ", ")
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
	return fmt.Sprintf("the value for %q could not be kept (%s); nothing was sent", e.name, e.err)
}

// keepInputs settles the values the user typed. A "keep" value is stored under its kept name, bound to the
// request's origin. A "once" value goes into the returned overlay only and never reaches the store; a stored
// value it replaces is cleared. It returns the status of every declared input. Called after approval with
// the item's watch already stopped, so replacing a stored value cancels other waiting items but not this one.
// The caller wipes the overlay.
func (s *Server) keepInputs(ctx context.Context, plan agentapi.Planned, supplied SuppliedInputs) ([]agentapi.InputStatus, map[string]keptvars.Entry, error) {
	defer supplied.Wipe()
	if len(plan.Inputs) == 0 {
		return nil, nil, nil
	}
	src := keptSourceOf(ctx, plan)
	src.Kind = keptvars.SourceUser
	scope := keptScope(ctx)
	statuses := make([]agentapi.InputStatus, 0, len(plan.Inputs))
	var once map[string]keptvars.Entry
	for _, in := range plan.Inputs {
		st := agentapi.InputStatus{Name: in.Name, Status: "reused"}
		typed, ok := supplied.Take(in.Name)
		if !ok {
			statuses = append(statuses, st)
			continue
		}
		raw, err := jsonString(typed.Value)
		clear(typed.Value)
		if err != nil {
			wipeOnce(once)
			return nil, nil, &inputKeepError{name: in.Name, err: err}
		}
		if typed.Once {
			if in.State == agentapi.InputStored && s.cfg.Kept != nil {
				s.cfg.Kept.Clear(in.KeptID)
			}
			if once == nil {
				once = map[string]keptvars.Entry{}
			}
			once[in.Name] = agentapi.OnceEntry(in.Name, plan.Origin, raw)
			st.Status = "once"
			statuses = append(statuses, st)
			continue
		}
		if s.cfg.Kept == nil {
			clear(raw)
			wipeOnce(once)
			return nil, nil, &inputKeepError{name: in.Name, err: keptvars.ErrClosed}
		}
		m, perr := s.cfg.Kept.Put(keptvars.Put{Scope: scope, Name: in.Name, Value: raw, Secret: true, Source: src, Origin: plan.Origin})
		clear(raw)
		if perr != nil {
			wipeOnce(once)
			return nil, nil, &inputKeepError{name: in.Name, err: perr}
		}
		st.Status = "supplied"
		if in.State == agentapi.InputStored {
			st.Status = "replaced"
		}
		st.ExpiresInSeconds = secondsLeft(m.ExpiresAt)
		statuses = append(statuses, st)
	}
	return statuses, once, nil
}

// wipeOnce zeroes the one-time values of a request. The resolver's copies share these slices.
func wipeOnce(once map[string]keptvars.Entry) {
	for name, e := range once {
		clear(e.Value)
		delete(once, name)
	}
}

func secondsLeft(at time.Time) int { return max(int(math.Ceil(time.Until(at).Seconds())), 0) }

// fillReused sets the remaining time of the inputs that were reused, read from the store after the re-plan.
func (s *Server) fillReused(ctx context.Context, statuses []agentapi.InputStatus) {
	if s.cfg.Kept == nil {
		return
	}
	for i, st := range statuses {
		if st.ExpiresInSeconds > 0 || st.Status == "once" {
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
