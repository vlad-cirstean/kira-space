package bridge

import (
	"log/slog"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// ChannelKeptChanged tells every window the kept variable set changed. Desktop only; the payload is a
// revision, the renderer refetches.
const ChannelKeptChanged = "kira:kept:changed"

// KeptChanged is the ChannelKeptChanged payload.
type KeptChanged struct {
	Revision uint64 `json:"revision"`
}

// KeptRetention is keptvars.Options.Retention's backend, read per Put. A read failure falls back to the
// default and logs one static warning.
func KeptRetention(settings *repos.SettingsRepo) func() time.Duration {
	var warn sync.Once
	return func() time.Duration {
		all, err := settings.GetAll()
		if err != nil {
			warn.Do(func() {
				slog.Warn("temporary variables: read retention, using one hour", "scope", "keptvars", "err", err)
			})
			return keptvars.DefaultTTL
		}
		return time.Duration(all.DbMcp.KeptRetentionMinutes) * time.Minute
	}
}

// KeptVarsService is the Kept variables view's surface: list, read, clear. The store is one per app run
// and memory only.
type KeptVarsService struct {
	Store *keptvars.Store
	// Settings gives the retention shown in the view.
	Settings *repos.SettingsRepo
	// TaskTitle names an ADE task for the scope label; "" when unknown.
	TaskTitle func(taskID string) string
	// Auth gates Reveal; the app's one authorizer, so the grace window is shared with connection and
	// variable reveals.
	Auth *localauth.Authorizer
	// Staged reads a secret value a held result will keep.
	Staged StagedSecrets
}

// StagedSecrets is the broker's view of the values held results will keep: *dbmcp.ApprovalBroker.
type StagedSecrets interface {
	StagedSecret(requestID, name string) (string, bool)
	DropStaged(requestID, name string) bool
}

// KeptEntryView is one live kept variable: its metadata and a preview. A secret has no preview.
type KeptEntryView struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	Bytes            int       `json:"bytes"`
	Items            int       `json:"items"`
	Secret           bool      `json:"secret"`
	Source           KeptSrc   `json:"source"`
	CreatedAt        time.Time `json:"createdAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
	Preview          string    `json:"preview"`
	PreviewTruncated bool      `json:"previewTruncated"`
	ScopeLabel       string    `json:"scopeLabel"`
	// Origin is the scheme://host[:port] a user-supplied value goes to; "" for other kinds.
	Origin string `json:"origin"`
}

// KeptSrc is where an entry came from.
type KeptSrc struct {
	Kind    string `json:"kind"`
	MCP     string `json:"mcp"`
	Tool    string `json:"tool"`
	Request string `json:"request"`
	Client  string `json:"client"`
	TaskID  string `json:"taskId,omitempty"`
}

// KeptList is the Kept variables view's data.
type KeptList struct {
	Entries          []KeptEntryView `json:"entries"`
	RetentionMinutes int             `json:"retentionMinutes"`
}

// List returns every live entry, newest first.
func (s *KeptVarsService) List() (KeptList, error) {
	all, err := s.Settings.GetAll()
	if err != nil {
		return KeptList{}, ipcerr.InternalErr(err)
	}
	metas := s.Store.List("")
	out := KeptList{Entries: make([]KeptEntryView, 0, len(metas)), RetentionMinutes: all.DbMcp.KeptRetentionMinutes}
	for _, m := range metas {
		v := KeptEntryView{
			ID: m.ID, Name: m.Name, Type: m.Type, Bytes: m.Bytes, Items: m.Items, Secret: m.Secret,
			Source:    KeptSrc{Kind: string(m.Source.Kind), MCP: m.Source.MCP, Tool: m.Source.Tool, Request: m.Source.Request, Client: m.Source.Client, TaskID: m.Source.TaskID},
			CreatedAt: m.CreatedAt, ExpiresAt: m.ExpiresAt, ScopeLabel: s.scopeLabel(m), Origin: m.Origin,
		}
		if !m.Secret {
			if e, ok := s.Store.ByID(m.ID); ok {
				v.Preview, v.PreviewTruncated = keptvars.Preview(e.Value, keptvars.PreviewBytes)
			}
		}
		out.Entries = append(out.Entries, v)
	}
	return out, nil
}

func (s *KeptVarsService) scopeLabel(m keptvars.Meta) string {
	if m.Scope == "external" {
		return "External clients"
	}
	if id := m.Source.TaskID; id != "" {
		if s.TaskTitle != nil {
			if title := s.TaskTitle(id); title != "" {
				return title
			}
		}
		return id
	}
	return m.Scope
}

// KeptValueArgs names one entry.
type KeptValueArgs struct {
	ID string `json:"id"`
}

// KeptValue is a full value as compact JSON text.
type KeptValue struct {
	JSON string `json:"json"`
	Type string `json:"type"`
}

// Value returns one full value. A secret is refused here: it has its own reveal path.
func (s *KeptVarsService) Value(args KeptValueArgs) (KeptValue, error) {
	if args.ID == "" {
		return KeptValue{}, ipcerr.Required("id")
	}
	e, ok := s.Store.ByID(args.ID)
	if !ok {
		return KeptValue{}, ipcerr.NotFoundNamed("temporary variable")
	}
	if e.Secret {
		return KeptValue{}, ipcerr.Contract("a secret temporary variable is not shown here")
	}
	return KeptValue{JSON: string(e.Value), Type: e.Type}, nil
}

// keptRevealReason is the OS prompt text (macOS prefixes it with "Kira Studio wants to").
const keptRevealReason = "show a secret temporary value an agent request set."

// KeptRevealArgs names the entry to reveal. Confirmed is honoured only where the OS cannot authenticate.
type KeptRevealArgs struct {
	ID        string `json:"id"`
	Confirmed bool   `json:"confirmed"`
}

// KeptReveal never carries a value except on outcome revealed; the outcomes are apivars.RevealResult's.
type KeptReveal struct {
	Value   *string `json:"value"`
	Error   *string `json:"error"`
	Outcome string  `json:"outcome"`
}

// Reveal shows a secret kept value (any source) behind the same gate as connection and variable reveals.
// The renderer keeps the plaintext in the component showing it and nowhere else.
func (s *KeptVarsService) Reveal(args KeptRevealArgs) (KeptReveal, error) {
	if args.ID == "" {
		return KeptReveal{}, ipcerr.Required("id")
	}
	res, err := s.reveal(args.Confirmed, func() (*string, error) {
		e, ok := s.Store.ByID(args.ID)
		if !ok {
			return nil, ipcerr.NotFoundNamed("temporary variable")
		}
		if !e.Secret {
			return nil, ipcerr.Contract("this temporary variable is not secret; it is shown in the list")
		}
		text := keptvars.SecretText(e)
		clear(e.Value)
		return &text, nil
	})
	if err == nil && res.Outcome == apivars.OutcomeRevealed {
		slog.Info("secret revealed", "scope", "keptvars", "id", args.ID)
	}
	return res, err
}

// reveal runs fetch behind the local-auth gate and maps the outcome.
func (s *KeptVarsService) reveal(confirmed bool, fetch func() (*string, error)) (KeptReveal, error) {
	outcome, value, err := localauth.Gated(s.Auth.Authorize, keptRevealReason, confirmed, fetch)
	switch outcome {
	case localauth.GateCancelled:
		return KeptReveal{Outcome: apivars.OutcomeCancelled}, nil
	case localauth.GateConfirmationRequired:
		return KeptReveal{Outcome: apivars.OutcomeConfirmationRequired}, nil
	case localauth.GateAuthError:
		slog.Warn("local authentication errored before reveal", "scope", "keptvars", "err", err)
		msg := err.Error()
		return KeptReveal{Outcome: apivars.OutcomeError, Error: &msg}, nil
	case localauth.GateFetchError:
		return KeptReveal{}, err
	}
	return KeptReveal{Outcome: apivars.OutcomeRevealed, Value: value}, nil
}

// KeptRevealStagedArgs names a secret value a held result will keep, by the request under review and its
// name. Confirmed is honoured only where the OS cannot authenticate.
type KeptRevealStagedArgs struct {
	RequestID string `json:"requestId"`
	Name      string `json:"name"`
	Confirmed bool   `json:"confirmed"`
}

// RevealStaged shows a secret value a held result will keep, behind the same gate as Reveal. The value is
// not stored yet, so it is read from the held result and gone once the result is sent or withheld.
func (s *KeptVarsService) RevealStaged(args KeptRevealStagedArgs) (KeptReveal, error) {
	if args.RequestID == "" {
		return KeptReveal{}, ipcerr.Required("requestId")
	}
	if args.Name == "" {
		return KeptReveal{}, ipcerr.Required("name")
	}
	return s.reveal(args.Confirmed, func() (*string, error) {
		text, ok := s.Staged.StagedSecret(args.RequestID, args.Name)
		if !ok {
			return nil, ipcerr.NotFoundNamed("secret value under review")
		}
		return &text, nil
	})
}

// KeptDropStagedArgs names a value a held result will keep, by the request under review and its name.
type KeptDropStagedArgs struct {
	RequestID string `json:"requestId"`
	Name      string `json:"name"`
}

// DropStaged removes a value a held result will keep: it is not stored when the result is sent, and the
// agent is told the user chose not to keep it.
func (s *KeptVarsService) DropStaged(args KeptDropStagedArgs) error {
	if args.RequestID == "" {
		return ipcerr.Required("requestId")
	}
	if args.Name == "" {
		return ipcerr.Required("name")
	}
	if !s.Staged.DropStaged(args.RequestID, args.Name) {
		return ipcerr.NotFoundNamed("value under review")
	}
	return nil
}

// KeptClearArgs names the entries to clear.
type KeptClearArgs struct {
	IDs []string `json:"ids"`
}

// Clear removes the entries and reports how many it removed.
func (s *KeptVarsService) Clear(args KeptClearArgs) (int, error) {
	if len(args.IDs) == 0 {
		return 0, ipcerr.Required("ids")
	}
	return s.Store.Clear(args.IDs...), nil
}

// ClearAll removes every entry.
func (s *KeptVarsService) ClearAll() (int, error) { return s.Store.ClearAll(), nil }
