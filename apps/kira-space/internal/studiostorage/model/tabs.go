package model

import (
	"encoding/json"

	"github.com/kirathecat/kira-studio/internal/appstorage"
)

// TabRecord's `State` stays raw JSON here — packages/shared/domain/tabs.ts's per-kind discriminated
// union (data/definition/console/document/keyvalue/stream/browse) is renderer-side validation
// logic, not storage shape. Go validates only the envelope it owns: kind is in
// RenderableTabKinds and State parses as a JSON object; per-kind shape and forward-compatible
// defaults stay renderer-side.
type TabRecord struct {
	ID           string          `json:"id"`
	ConnectionID *string         `json:"connectionId"`
	Path         string          `json:"path"`
	Kind         string          `json:"kind"`
	State        json.RawMessage `json:"state"`
	Order        int             `json:"order"`
	Active       bool            `json:"active"`
	// WorkspaceID is C5 §3.1/§4.2's own tab-isolation dimension: nil for studio/api tabs
	// (workspaceKeyOf's own `??` fallback then derives the workspace from Kind, exactly as
	// today), "terminal" for a terminal tab.
	WorkspaceID *string `json:"workspaceId"`
}

// RenderableTabKinds mirrors tabs.ts's RENDERABLE_TAB_KINDS — a row of any other kind is dropped
// on restore, logged, and not re-saved. Note 'ddl' is deliberately absent (P52 §4.3 / P53 §3.1):
// the legacy 'ddl'->'definition' coercion is dropped, not ported.
var RenderableTabKinds = map[string]bool{
	"data": true, "definition": true, "console": true, "document": true,
	"keyvalue": true, "stream": true, "browse": true,
	// P2: the first Http-mode tab kind (§2 F1) — this list is the one of the four kind
	// vocabularies TypeScript's own exhaustiveness checks can't catch a miss on (D10's parity test).
	"http-request": true,
	// P11 D2: the second kind inside the 'http' mode.
	"grpc-request": true,
	// P17 D16: the third kind inside the 'api' mode — one collection's or one environment's
	// variable set, opened as a tab. This is the exact vocabulary F8 warns is easy to miss: a row
	// of this kind is silently dropped on restore if this line is forgotten.
	"variable-set": true,
	// P28 D16(c): the fourth kind inside the 'api' mode — the environment list, where an
	// environment is created/renamed/duplicated/deleted/reordered. Same F8 warning as the line
	// above: forget this and a row of this kind is silently dropped on restore.
	"environments": true,
	// An embedded shell tab. Never reaches restore (persistableTabs() filters it out before save,
	// tabs.ts:132) — still required here: the parity test (go-ts-vocabulary-parity.spec.ts) demands
	// a vocabulary complete regardless of what currently reaches it.
	"terminal": true,
	// P242: a smart script run; never persisted either, still part of the vocabulary.
	"script-run": true,
}

// IsRenderableTabKind reports whether kind is one of the renderable tab kinds.
func IsRenderableTabKind(kind string) bool {
	return RenderableTabKinds[kind]
}

// IsJSONObject reports whether raw is valid JSON whose top-level value is an object — tabs.ts's
// row.state must round-trip a Record<...>-shaped state, never a bare array or scalar.
func IsJSONObject(raw []byte) bool {
	return appstorage.IsJSONObject(raw)
}

// Validate asserts the same envelope repos.TabsRepo.List already enforces on read (kind is
// renderable, state is a JSON object), plus the non-empty identity fields no SQL constraint
// covers (Save previously wrote records unvalidated, so a bad row round-tripped silently —
// it persisted, then vanished on the next List() with nothing at the write site to say why).
// WorkspaceID is required only for "terminal" — every other kind derives its workspace from
// TAB_KIND_MODE instead.
func (t TabRecord) Validate() error {
	return appstorage.ValidateTab(appstorage.TabFields{
		ID: t.ID, Path: t.Path, Kind: t.Kind, State: t.State, WorkspaceID: t.WorkspaceID,
	}, IsRenderableTabKind, func(kind string) bool { return kind == "terminal" })
}
