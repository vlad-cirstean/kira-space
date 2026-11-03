package scripts

import (
	"database/sql"
	"fmt"
	"slices"
	"strconv"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// savedSet maps a script id to its saved secret names.
type savedSet map[string][]string

func (s savedSet) of(id string) []string {
	if names := s[id]; names != nil {
		return names
	}
	return []string{}
}

func (r *Repo) savedNames() (savedSet, error) {
	if r.Secrets == nil {
		return savedSet{}, nil
	}
	m, err := r.Secrets.Names(secrets.ScopeScriptParam)
	if err != nil {
		return nil, fmt.Errorf("scripts: list saved secrets: %w", err)
	}
	return m, nil
}

func (r *Repo) secretStatus() secrets.Status {
	if r.Secrets == nil {
		return secrets.Status{Backend: secrets.BackendUnavailable}
	}
	return r.Secrets.Status()
}

// SecretValues opens the saved secret values of one script. unreadable names a value that failed
// to decrypt, which a run treats as absent.
func (r *Repo) SecretValues(id string) (vals map[string]string, unreadable map[string]error, err error) {
	if r.Secrets == nil {
		return map[string]string{}, map[string]error{}, nil
	}
	return r.Secrets.Open(secrets.ScopeScriptParam, id)
}

func (r *Repo) pruneSecrets(q secrets.Execer, id string) error {
	if r.Secrets == nil {
		return nil
	}
	if err := r.Secrets.Prune(q, secrets.ScopeScriptParam, id, nil); err != nil {
		return fmt.Errorf("scripts: delete saved secrets of %s: %w", id, err)
	}
	return nil
}

// pruneUnsecret drops the saved values of id whose name is no longer a secret param.
func (r *Repo) pruneUnsecret(tx *sql.Tx, id string, params []Param) error {
	if r.Secrets == nil {
		return nil
	}
	saved, err := r.Secrets.Names(secrets.ScopeScriptParam)
	if err != nil {
		return fmt.Errorf("scripts: list saved secrets: %w", err)
	}
	var keep []string
	for _, n := range saved[id] {
		for _, p := range params {
			if p.Secret && p.Name == n {
				keep = append(keep, n)
			}
		}
	}
	if len(keep) == len(saved[id]) {
		return nil
	}
	if err := r.Secrets.Prune(tx, secrets.ScopeScriptParam, id, keep); err != nil {
		return fmt.Errorf("scripts: prune saved secrets of %s: %w", id, err)
	}
	return nil
}

// planSecrets validates secret values against the secret params and the store: set holds the values
// to seal, cleared the names to drop.
func (r *Repo) planSecrets(params []Param, values map[string]*string) (set map[string]string, cleared map[string]bool, err error) {
	secretParams := map[string]bool{}
	for _, p := range params {
		if p.Secret {
			secretParams[p.Name] = true
		}
	}
	set, cleared = map[string]string{}, map[string]bool{}
	for name, v := range values {
		switch {
		case !secretParams[name]:
			return nil, nil, invalid("", "backend.core.runs.secretNotParam.body", "name", name)
		case v == nil:
			cleared[name] = true
		case *v == "":
			return nil, nil, invalid("", "backend.core.runs.secretEmpty.body", "name", name)
		default:
			if key := textProblem(*v); key != "" {
				return nil, nil, invalid("", key, "max", strconv.Itoa(MaxTextValue))
			}
			set[name] = *v
		}
	}
	if len(set) > 0 && !r.secretStatus().Available {
		names := make([]string, 0, len(set))
		for n := range set {
			names = append(names, n)
		}
		slices.Sort(names)
		return nil, nil, ipcerr.T(ipcerr.CodeSecretStore, "backend.core.scripts.secretStoreOff.body", map[string]string{"name": names[0]})
	}
	return set, cleared, nil
}

// SetSecrets saves (string) or clears (nil) the values of secret params of id, in one transaction.
// Returns a wrapped sql.ErrNoRows for an unknown id.
func (r *Repo) SetSecrets(id string, values map[string]*string) error {
	rec, err := r.Get(id)
	if err != nil {
		return err
	}
	if rec == nil {
		return fmt.Errorf("scripts: set secrets %s: %w", id, sql.ErrNoRows)
	}
	set, cleared, err := r.planSecrets(rec.Params, values)
	if err != nil {
		return err
	}
	if r.Secrets == nil {
		return nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("scripts: set secrets %s: begin: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	keep := []string{}
	for _, n := range rec.SavedSecrets {
		if _, replaced := set[n]; !cleared[n] && !replaced {
			keep = append(keep, n)
		}
	}
	if err := r.Secrets.Prune(tx, secrets.ScopeScriptParam, id, keep); err != nil {
		return fmt.Errorf("scripts: prune saved secrets of %s: %w", id, err)
	}
	for n, v := range set {
		if err := r.Secrets.Put(tx, secrets.ScopeScriptParam, id, n, v); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scripts: set secrets %s: commit: %w", id, err)
	}
	return nil
}
