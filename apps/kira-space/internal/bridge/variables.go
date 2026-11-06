package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// VariablesService reads environments and variables from the rows the api config files project to,
// switches the active environment (app state, not config) and creates an environment file. Names
// and plain values are edited in the files; the environment order is app state.
// Reveal/RevealHistory need the Authorizer gate, so they go through Deps.ApiVars.
type VariablesService struct {
	Deps  appcore.Deps
	Files *apifiles.Store
}

// ---- environments ----

func (s *VariablesService) ListEnvironments() ([]model.Environment, error) {
	return ipcerr.InternalResult(s.Deps.Repos.Variables.ListEnvironments())
}

type VariablesCreateEnvironmentArgs struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// CreateEnvironment writes a new environment file; a color of "none" writes none.
func (s *VariablesService) CreateEnvironment(args VariablesCreateEnvironmentArgs) (model.Environment, error) {
	if args.Name == "" {
		return model.Environment{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.validation.nameRequired.body", nil)
	}
	if !model.ValidPaletteColor(args.Color) {
		return model.Environment{}, ipcerr.Contract("invalid color")
	}
	color := args.Color
	if color == "none" {
		color = ""
	}
	id, err := s.Files.CreateEnvironment(apifiles.EnvironmentFile{Name: args.Name, Description: args.Description, Color: color})
	if err != nil {
		return model.Environment{}, apiFilesErr(err)
	}
	envs, err := s.Deps.Repos.Variables.ListEnvironments()
	if err != nil {
		return model.Environment{}, ipcerr.InternalErr(err)
	}
	for _, e := range envs {
		if e.ID == id {
			return e, nil
		}
	}
	return model.Environment{}, ipcerr.NotFoundNamed(args.Name)
}

type VariablesEnvironmentIDArgs struct {
	ID string `json:"id"`
}

// DeleteEnvironment removes the environment's file.
func (s *VariablesService) DeleteEnvironment(args VariablesEnvironmentIDArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	return apiFilesErr(s.Files.DeleteEnvironment(args.ID))
}

type VariablesReorderEnvironmentsArgs struct {
	IDs []string `json:"ids"`
}

// ReorderEnvironments puts the listed environments first, in that order; ids left out keep their order after.
func (s *VariablesService) ReorderEnvironments(args VariablesReorderEnvironmentsArgs) error {
	if len(args.IDs) == 0 {
		return ipcerr.Required("ids")
	}
	if err := s.Files.ReorderEnvironments(args.IDs); err != nil {
		return apiFilesErr(err)
	}
	return nil
}

// DuplicateEnvironment copies the environment file under a free name.
func (s *VariablesService) DuplicateEnvironment(args VariablesEnvironmentIDArgs) (model.Environment, error) {
	if args.ID == "" {
		return model.Environment{}, ipcerr.Required("id")
	}
	id, err := s.Files.DuplicateEnvironment(args.ID)
	if err != nil {
		return model.Environment{}, apiFilesErr(err)
	}
	envs, err := s.Deps.Repos.Variables.ListEnvironments()
	if err != nil {
		return model.Environment{}, ipcerr.InternalErr(err)
	}
	for _, e := range envs {
		if e.ID == id {
			return e, nil
		}
	}
	return model.Environment{}, ipcerr.NotFound("environment " + id + " does not exist")
}

// SetActiveEnvironment's id may be "" — D3: that selects "No environment", the first half of the
// repo's own transaction alone.
func (s *VariablesService) SetActiveEnvironment(args VariablesEnvironmentIDArgs) error {
	if err := s.Deps.Repos.Variables.SetActiveEnvironment(args.ID); err != nil {
		return ipcerr.InternalErr(err)
	}
	emitApiData(s.Deps.Events, environmentsChange())
	return nil
}

// ---- variables ----

// VariablesScopeArgs names one scope's owner — 'collection' or 'environment' plus that table's own id.
type VariablesScopeArgs struct {
	Scope   model.VariableScope `json:"scope"`
	OwnerID string              `json:"ownerId"`
}

func validScope(scope model.VariableScope) error {
	if scope != model.VariableScopeCollection && scope != model.VariableScopeEnvironment {
		return ipcerr.Contract("scope must be 'collection' or 'environment'")
	}
	return nil
}

func (s *VariablesService) List(args VariablesScopeArgs) ([]model.Variable, error) {
	if err := validScope(args.Scope); err != nil {
		return nil, err
	}
	if args.OwnerID == "" {
		return nil, ipcerr.Required("ownerId")
	}
	return ipcerr.InternalResult(s.Deps.Repos.Variables.List(args.Scope, args.OwnerID))
}

// ---- history (D13) ----

type VariablesHistoryArgs struct {
	VariableID string `json:"variableId"`
}

func (s *VariablesService) History(args VariablesHistoryArgs) ([]model.VariableHistoryEntry, error) {
	if args.VariableID == "" {
		return nil, ipcerr.Required("variableId")
	}
	return ipcerr.InternalResult(s.Deps.Repos.Variables.History(args.VariableID))
}

// ---- the gated reveal (D8/D9) ----

type VariablesRevealArgs struct {
	VariableID string `json:"variableId"`
	// Confirmed is honoured only when the backend has itself determined OS authentication is
	// unavailable (mirrors bridge.ConnectionsRevealArgs.Confirmed exactly — P14 D6).
	Confirmed bool `json:"confirmed"`
}

// Reveal never errors (P25 D9) — see apivars.Service.Reveal's own comment.
func (s *VariablesService) Reveal(args VariablesRevealArgs) apivars.RevealResult {
	if args.VariableID == "" {
		msg := "variableId is required"
		return apivars.RevealResult{Outcome: apivars.OutcomeError, Error: &msg}
	}
	return s.Deps.ApiVars.Reveal(args.VariableID, args.Confirmed)
}

type VariablesRevealHistoryArgs struct {
	HistoryID string `json:"historyId"`
	Confirmed bool   `json:"confirmed"`
}

func (s *VariablesService) RevealHistory(args VariablesRevealHistoryArgs) apivars.RevealResult {
	if args.HistoryID == "" {
		msg := "historyId is required"
		return apivars.RevealResult{Outcome: apivars.OutcomeError, Error: &msg}
	}
	return s.Deps.ApiVars.RevealHistory(args.HistoryID, args.Confirmed)
}
