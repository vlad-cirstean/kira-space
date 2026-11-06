package model

import (
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
)

// ResponseHistoryEntry is one api_response_history row's list projection — no body, ever (D4's
// List never selects snapshot_json). ItemID is nil for a scratch tab's own history.
type ResponseHistoryEntry struct {
	ID          string  `json:"id"`
	ItemID      *string `json:"itemId"`
	TabID       string  `json:"tabId"`
	SentAt      string  `json:"sentAt"`
	Method      string  `json:"method"`
	URL         string  `json:"url"`
	Environment string  `json:"environment"`
	Status      int     `json:"status"`
	StatusText  string  `json:"statusText"`
	ElapsedMs   int     `json:"elapsedMs"`
	BodyBytes   int     `json:"bodyBytes"`
	StoredBytes int     `json:"storedBytes"`
}

// ResponseHistoryRequest is the stage-1 request stored beside a response: {{$dynamic}} and
// non-secret {{name}} substituted, a secret still spelled {{name}} — never the resolved,
// secret-bearing request httpclient.Send sent. httpclient.Header/Body are reused
// directly rather than re-declared: their JSON shape already matches
// packages/shared/domain/http.ts's HttpHeaderWire/HttpBodyWire exactly, and unlike
// model.SavedRequest (which mirrors a different renderer-owned shape, collections.go) there is no
// existing type here to keep field-identical to instead.
type ResponseHistoryRequest struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Headers []httpclient.Header `json:"headers"`
	Body    httpclient.Body     `json:"body"`
}

// ResponseHistorySnapshot is one entry's full stored shape — Entry rebuilt from the row's own
// summary columns (the Get; never duplicated inside snapshot_json), Response the unmodified
// httpclient.Response (this identity is what makes ResponsePane.vue's source swap a one-line
// change), and the two storage-cap flags for each side.
type ResponseHistorySnapshot struct {
	Entry                       ResponseHistoryEntry   `json:"entry"`
	Request                     ResponseHistoryRequest `json:"request"`
	Response                    httpclient.Response    `json:"response"`
	BodyStored                  bool                   `json:"bodyStored"`
	BodyStorageTruncated        bool                   `json:"bodyStorageTruncated"`
	RequestBodyStorageTruncated bool                   `json:"requestBodyStorageTruncated"`
	// RequestFieldsElided: urlencoded/form-data field values were dropped to keep the row within
	// the history budget (repos/response_history.go).
	RequestFieldsElided bool `json:"requestFieldsElided,omitempty"`
}

// ResponseHistoryRecord is Record's one argument — the bridge/http.go call site builds this
// directly from HttpSendArgs (stage-1, never resolved) and the httpclient.Response the send just
// produced. ItemID is "" for a scratch tab (never nil — the repo is the one place that decides
// whether "" becomes a NULL column); EnvironmentID is an id, resolved to a name by Record itself
// at write time.
type ResponseHistoryRecord struct {
	ItemID        string
	TabID         string
	EnvironmentID string
	Method        string
	URL           string
	Headers       []httpclient.Header
	Body          httpclient.Body
	Response      httpclient.Response
}

// Validate checks what SQL cannot, refusing on write (repos/tabs.go's posture) — a non-empty
// TabID, a Method, and a Status SQLite's own schema has no CHECK for.
func (r ResponseHistoryRecord) Validate() error {
	if r.TabID == "" {
		return fmt.Errorf("model: response history: tabId is required")
	}
	if r.Method == "" {
		return fmt.Errorf("model: response history: method is required")
	}
	if r.Response.Status < 100 || r.Response.Status > 599 {
		return fmt.Errorf("model: response history: status %d out of range", r.Response.Status)
	}
	return nil
}
