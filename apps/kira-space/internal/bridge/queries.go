package bridge

import (
	"errors"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// QueriesService is 1:1 with src/main/ipc/queries.ts's nine methods, each a typed-struct wrapper
// over Deps.Repos.SavedQueries / Deps.Repos.FilterHistory with an explicit guard and an ipcerr
// translation. Name-length validation is not repeated here — it already lives in
// SavedQueriesRepo.insert/Update via model.ValidSavedQueryName.
type QueriesService struct{ Deps appcore.StudioDeps }

// SavedQueriesChangedEvent is ChannelSavedQueriesChanged's payload: whose list changed and which
// kind ("filter", "console" or "discover").
type SavedQueriesChangedEvent struct {
	ConnectionID string `json:"connectionId"`
	Kind         string `json:"kind"`
}

// FilterHistoryChangedEvent is ChannelFilterHistoryChanged's payload.
type FilterHistoryChangedEvent struct {
	ConnectionID string `json:"connectionId"`
	Path         string `json:"path"`
}

// notifySaved broadcasts a successful write; a failed one changed nothing.
func (s *QueriesService) notifySaved(q model.SavedQuery, err error) {
	if err == nil {
		s.Deps.Events.Emit(ChannelSavedQueriesChanged, SavedQueriesChangedEvent{ConnectionID: q.ConnectionID, Kind: q.Kind})
	}
}

type QueriesListArgs struct {
	ConnectionID string `json:"connectionId"`
	Path         string `json:"path"`
}

func (s *QueriesService) List(args QueriesListArgs) ([]model.SavedQuery, error) {
	if args.ConnectionID == "" {
		return nil, ipcerr.Required("connectionId")
	}
	return ipcerr.InternalResult(s.Deps.Repos.SavedQueries.ListFilters(args.ConnectionID, args.Path))
}

func (s *QueriesService) ListConsole(args QueriesListArgs) ([]model.SavedQuery, error) {
	if args.ConnectionID == "" {
		return nil, ipcerr.Required("connectionId")
	}
	return ipcerr.InternalResult(s.Deps.Repos.SavedQueries.ListConsole(args.ConnectionID, args.Path))
}

type QueriesSaveArgs struct {
	ConnectionID string           `json:"connectionId"`
	Path         string           `json:"path"`
	Name         string           `json:"name"`
	Body         model.FilterBody `json:"body"`
	Pinned       bool             `json:"pinned"`
}

func (s *QueriesService) Save(args QueriesSaveArgs) (model.SavedQuery, error) {
	if args.ConnectionID == "" {
		return model.SavedQuery{}, ipcerr.Required("connectionId")
	}
	q, err := s.Deps.Repos.SavedQueries.SaveFilter(args.ConnectionID, args.Path, args.Name, args.Body, args.Pinned)
	s.notifySaved(q, err)
	return ipcerr.InternalResult(q, err)
}

type QueriesSaveConsoleArgs struct {
	ConnectionID string            `json:"connectionId"`
	Path         string            `json:"path"`
	Name         string            `json:"name"`
	Body         model.ConsoleBody `json:"body"`
	Pinned       bool              `json:"pinned"`
}

func (s *QueriesService) SaveConsole(args QueriesSaveConsoleArgs) (model.SavedQuery, error) {
	if args.ConnectionID == "" {
		return model.SavedQuery{}, ipcerr.Required("connectionId")
	}
	q, err := s.Deps.Repos.SavedQueries.SaveConsole(args.ConnectionID, args.Path, args.Name, args.Body, args.Pinned)
	s.notifySaved(q, err)
	return ipcerr.InternalResult(q, err)
}

type QueriesUpdateArgs struct {
	ID     string  `json:"id"`
	Name   *string `json:"name,omitempty"`
	Pinned *bool   `json:"pinned,omitempty"`
}

func (s *QueriesService) Update(args QueriesUpdateArgs) (model.SavedQuery, error) {
	if args.ID == "" {
		return model.SavedQuery{}, ipcerr.Required("id")
	}
	name := ""
	if args.Name != nil {
		name = strings.TrimSpace(*args.Name)
	}
	q, err := s.Deps.Repos.SavedQueries.Update(args.ID, model.SavedQueryPatch{Name: args.Name, Pinned: args.Pinned})
	s.notifySaved(q, err)
	return queryResult(q, err, name)
}

// queryResult maps the repo errors a caller can act on to typed IPC errors.
func queryResult(q model.SavedQuery, err error, name string) (model.SavedQuery, error) {
	switch {
	case err == nil:
		return q, nil
	case errors.Is(err, repos.ErrSavedQueryNotFound):
		return model.SavedQuery{}, ipcerr.T(ipcerr.CodeNotFound, "backend.studio.query.notFound.body", nil)
	case errors.Is(err, repos.ErrSavedQueryNameTaken):
		return model.SavedQuery{}, ipcerr.T(ipcerr.CodeConflict, "backend.studio.query.nameTaken.body", map[string]string{"name": name})
	case errors.Is(err, model.ErrInvalidDiscoverBody):
		detail := strings.TrimPrefix(err.Error(), model.ErrInvalidDiscoverBody.Error()+": ")
		return model.SavedQuery{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.query.invalidBody.body", map[string]string{"detail": detail})
	}
	return ipcerr.InternalResult(q, err)
}

type QueriesListDiscoverArgs struct {
	ConnectionID string `json:"connectionId"`
}

// ListDiscover lists a connection's saved Discover queries, whatever data view they target.
func (s *QueriesService) ListDiscover(args QueriesListDiscoverArgs) ([]model.SavedQuery, error) {
	if args.ConnectionID == "" {
		return nil, ipcerr.Required("connectionId")
	}
	return ipcerr.InternalResult(s.Deps.Repos.SavedQueries.ListDiscover(args.ConnectionID))
}

type QueriesSaveDiscoverArgs struct {
	ConnectionID string             `json:"connectionId"`
	Name         string             `json:"name"`
	Body         model.DiscoverBody `json:"body"`
	Pinned       bool               `json:"pinned"`
}

func (s *QueriesService) SaveDiscover(args QueriesSaveDiscoverArgs) (model.SavedQuery, error) {
	if args.ConnectionID == "" {
		return model.SavedQuery{}, ipcerr.Required("connectionId")
	}
	q, err := s.Deps.Repos.SavedQueries.SaveDiscover(args.ConnectionID, args.Name, args.Body, args.Pinned)
	s.notifySaved(q, err)
	return queryResult(q, err, strings.TrimSpace(args.Name))
}

type QueriesUpdateDiscoverArgs struct {
	ID   string             `json:"id"`
	Body model.DiscoverBody `json:"body"`
}

// UpdateDiscover replaces a saved Discover query's body in place.
func (s *QueriesService) UpdateDiscover(args QueriesUpdateDiscoverArgs) (model.SavedQuery, error) {
	if args.ID == "" {
		return model.SavedQuery{}, ipcerr.Required("id")
	}
	q, err := s.Deps.Repos.SavedQueries.UpdateDiscover(args.ID, args.Body)
	s.notifySaved(q, err)
	return queryResult(q, err, "")
}

type QueriesIDArgs struct {
	ID string `json:"id"`
}

func (s *QueriesService) Delete(args QueriesIDArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	// Read first: the row is gone after the delete, and the event names its connection and kind.
	q, getErr := s.Deps.Repos.SavedQueries.Get(args.ID)
	if err := s.Deps.Repos.SavedQueries.Delete(args.ID); err != nil {
		return ipcerr.InternalErr(err)
	}
	s.notifySaved(q, getErr)
	return nil
}

func (s *QueriesService) Touch(args QueriesIDArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	if err := s.Deps.Repos.SavedQueries.Touch(args.ID); err != nil {
		return ipcerr.InternalErr(err)
	}
	q, err := s.Deps.Repos.SavedQueries.Get(args.ID)
	s.notifySaved(q, err)
	return nil
}

type QueriesHistoryListArgs struct {
	ConnectionID string `json:"connectionId"`
	Path         string `json:"path"`
	Limit        int    `json:"limit"`
}

func (s *QueriesService) HistoryList(args QueriesHistoryListArgs) ([]model.FilterHistoryEntry, error) {
	if args.ConnectionID == "" {
		return nil, ipcerr.Required("connectionId")
	}
	if args.Limit < 1 || args.Limit > 100 {
		return nil, ipcerr.Contract("limit must be between 1 and 100")
	}
	return ipcerr.InternalResult(s.Deps.Repos.FilterHistory.List(args.ConnectionID, args.Path, args.Limit))
}

type QueriesHistoryRecordArgs struct {
	ConnectionID string          `json:"connectionId"`
	Path         string          `json:"path"`
	Where        *string         `json:"where"`
	OrderBy      *model.SortSpec `json:"orderBy"`
}

func (s *QueriesService) HistoryRecord(args QueriesHistoryRecordArgs) error {
	if args.ConnectionID == "" {
		return ipcerr.Required("connectionId")
	}
	if err := s.Deps.Repos.FilterHistory.Record(args.ConnectionID, args.Path, args.Where, args.OrderBy); err != nil {
		return ipcerr.InternalErr(err)
	}
	s.Deps.Events.Emit(ChannelFilterHistoryChanged, FilterHistoryChangedEvent{ConnectionID: args.ConnectionID, Path: args.Path})
	return nil
}
