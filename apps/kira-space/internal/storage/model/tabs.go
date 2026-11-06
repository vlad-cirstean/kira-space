package model

import (
	"encoding/json"

	"github.com/kirathecat/kira-studio/internal/appstorage"
)

// TabRecord's `State` stays raw JSON: packages/shared/domain/tabs.ts's per-kind discriminated
// union is renderer-side validation logic, not storage shape. Go validates only the envelope it
// owns: kind is in RenderableTabKinds and State parses as a JSON object.
type TabRecord struct {
	ID string `json:"id"`
	// ConnectionID scopes a database-module tab to its connection; nil for every other kind.
	ConnectionID *string         `json:"connectionId,omitempty"`
	Path         string          `json:"path"`
	Kind         string          `json:"kind"`
	State        json.RawMessage `json:"state"`
	Order        int             `json:"order"`
	Active       bool            `json:"active"`
	// WorkspaceID scopes a repository tab (and a terminal) to its workspace. Database-module
	// kinds carry none: the renderer derives their workspace from the kind.
	WorkspaceID *string `json:"workspaceId"`
}

// dbTabKinds is the database-module vocabulary (tabs.ts's RENDERABLE_TAB_KINDS minus the
// workbench kinds). A row of any kind outside RenderableTabKinds is dropped on restore, logged,
// and not re-saved ('ddl' is deliberately absent: the legacy coercion is gone).
var dbTabKinds = map[string]bool{
	"data": true, "definition": true, "console": true, "document": true,
	"keyvalue": true, "stream": true, "browse": true,
	"http-request": true,
	"grpc-request": true,
	// A collection's or environment's variable set, opened as a tab. A row of this kind is
	// silently dropped on restore if this line is forgotten.
	"variable-set": true,
	// The environment list. Same drop-on-restore warning as variable-set.
	"environments": true,
}

// spaceTabKinds is the repository workbench kinds plus the shared workbench kinds (terminal, script-run).
var spaceTabKinds = map[string]bool{
	"repo-graph":      true,
	"repo-file":       true,
	"local-file":      true,
	"repo-diff":       true,
	"repo-multi-diff": true,
	"terminal":        true,
	"script-run":      true,
}

// RenderableTabKinds is every kind that persists.
var RenderableTabKinds = func() map[string]bool {
	kinds := map[string]bool{}
	for k := range spaceTabKinds {
		kinds[k] = true
	}
	for k := range dbTabKinds {
		kinds[k] = true
	}
	return kinds
}()

// IsRenderableTabKind reports whether kind is one of the renderable tab kinds.
func IsRenderableTabKind(kind string) bool {
	return RenderableTabKinds[kind]
}

// IsJSONObject reports whether raw is valid JSON whose top-level value is an object.
func IsJSONObject(raw []byte) bool {
	return appstorage.IsJSONObject(raw)
}

// Validate asserts the same envelope repos.TabsRepo.List already enforces on read, plus the
// non-empty identity fields no SQL constraint covers. WorkspaceID is required for every kind
// except the database-module ones, which derive their workspace from the kind.
func (t TabRecord) Validate() error {
	return appstorage.ValidateTab(appstorage.TabFields{
		ID: t.ID, Path: t.Path, Kind: t.Kind, State: t.State, WorkspaceID: t.WorkspaceID,
	}, IsRenderableTabKind, func(kind string) bool { return !dbTabKinds[kind] })
}
