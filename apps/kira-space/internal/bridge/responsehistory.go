package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// ResponseHistoryService is the CollectionsService shape (a typed-struct wrapper per method,
// with an explicit guard and an ipcerr translation) over Deps.Repos.ResponseHistory. No op-log
// row and no new op kind: every method here is a single indexed local read or a single-row
// write the pane renders into directly, not a network exchange the op ring's ~150 ms invariant
// is about. Recording itself is not here at all — it is the one call inside bridge/http.go's
// existing Send closure.
type ResponseHistoryService struct{ Deps appcore.Deps }

// scopeKey computes the same scope a history row's own GENERATED column does (itemId when
// non-empty, else "tab:"+tabId) — one function, one place, so the Go side and SQLite can never
// disagree about what a scope is (D8).
func scopeKey(itemID, tabID string) string {
	if itemID != "" {
		return itemID
	}
	return "tab:" + tabID
}

type ResponseHistoryScopeArgs struct {
	ItemID string `json:"itemId"`
	TabID  string `json:"tabId"`
}

// List returns newest-first, at most 20 entries by construction (Record's own per-scope trim).
func (s *ResponseHistoryService) List(args ResponseHistoryScopeArgs) ([]model.ResponseHistoryEntry, error) {
	return ipcerr.InternalResult(s.Deps.Repos.ResponseHistory.List(scopeKey(args.ItemID, args.TabID)))
}

type ResponseHistoryIDArgs struct {
	ID string `json:"id"`
}

func (s *ResponseHistoryService) Get(args ResponseHistoryIDArgs) (model.ResponseHistorySnapshot, error) {
	if args.ID == "" {
		return model.ResponseHistorySnapshot{}, ipcerr.Required("id")
	}
	return ipcerr.InternalResult(s.Deps.Repos.ResponseHistory.Get(args.ID))
}

func (s *ResponseHistoryService) Delete(args ResponseHistoryIDArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	return ipcerr.InternalErr(s.Deps.Repos.ResponseHistory.Delete(args.ID))
}

func (s *ResponseHistoryService) Clear(args ResponseHistoryScopeArgs) error {
	return ipcerr.InternalErr(s.Deps.Repos.ResponseHistory.Clear(scopeKey(args.ItemID, args.TabID)))
}

type ResponseHistoryAdoptArgs struct {
	TabID  string `json:"tabId"`
	ItemID string `json:"itemId"`
}

type ResponseHistoryAdoptResult struct {
	Adopted int `json:"adopted"`
}

// Adopt is D14 — a scratch tab's entries move onto a newly-saved request's id.
func (s *ResponseHistoryService) Adopt(args ResponseHistoryAdoptArgs) (ResponseHistoryAdoptResult, error) {
	if args.TabID == "" || args.ItemID == "" {
		return ResponseHistoryAdoptResult{}, ipcerr.Contract("tabId and itemId are required")
	}
	n, err := s.Deps.Repos.ResponseHistory.Adopt(args.TabID, args.ItemID)
	if err != nil {
		return ResponseHistoryAdoptResult{}, ipcerr.InternalErr(err)
	}
	return ResponseHistoryAdoptResult{Adopted: n}, nil
}
