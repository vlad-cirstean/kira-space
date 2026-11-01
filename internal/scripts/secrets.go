package scripts

import (
	"database/sql"
	"fmt"
	"slices"

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

// secretPlan is a validated write of saved secret values.
type secretPlan struct {
	set     map[string]string
	cleared map[string]bool
	// kept are the secret param names with a saved value once the write lands.
	kept map[string]bool
}

func (p secretPlan) savedNames(params []Param) []string {
	out := []string{}
	for _, par := range params {
		if p.kept[par.Name] {
			out = append(out, par.Name)
		}
	}
	return out
}

// planSecrets validates secretValues against the script's secret params and the schedule rule.
// current are the names already saved for the script.
func (r *Repo) planSecrets(f CustomScriptFields, secretValues map[string]*string, current []string) (secretPlan, error) {
	secretParams := map[string]bool{}
	for _, p := range f.Params {
		if p.Secret {
			secretParams[p.Name] = true
		}
	}
	plan := secretPlan{set: map[string]string{}, cleared: map[string]bool{}, kept: map[string]bool{}}
	for name, v := range secretValues {
		switch {
		case !secretParams[name]:
			return plan, invalid("scripts: %s is not a secret param", name)
		case v == nil:
			plan.cleared[name] = true
		case *v == "":
			return plan, invalid("scripts: the saved value of %s is empty: clear it instead", name)
		default:
			if err := checkText(*v); err != nil {
				return plan, invalid("scripts: saved value of %s %v", name, err)
			}
			plan.set[name] = *v
		}
	}
	if len(plan.set) > 0 && !r.secretStatus().Available {
		names := make([]string, 0, len(plan.set))
		for n := range plan.set {
			names = append(names, n)
		}
		slices.Sort(names)
		return plan, ipcerr.T(ipcerr.CodeSecretStore, "backend.core.scripts.secretStoreOff.body", map[string]string{"name": names[0]})
	}
	for _, n := range current {
		if secretParams[n] && !plan.cleared[n] {
			plan.kept[n] = true
		}
	}
	for n := range plan.set {
		plan.kept[n] = true
	}
	if err := checkScheduleSecrets(f.Schedule, f.Params, plan.kept); err != nil {
		return plan, err
	}
	return plan, nil
}

// applySecrets writes plan inside the script's own tx: it drops every saved value whose param was
// removed, renamed, un-secreted or cleared, then seals the new ones.
func (r *Repo) applySecrets(tx *sql.Tx, id string, plan secretPlan) error {
	if r.Secrets == nil {
		return nil
	}
	keep := make([]string, 0, len(plan.kept))
	for n := range plan.kept {
		if _, isSet := plan.set[n]; !isSet {
			keep = append(keep, n)
		}
	}
	if err := r.Secrets.Prune(tx, secrets.ScopeScriptParam, id, keep); err != nil {
		return fmt.Errorf("scripts: prune saved secrets of %s: %w", id, err)
	}
	for n, v := range plan.set {
		if err := r.Secrets.Put(tx, secrets.ScopeScriptParam, id, n, v); err != nil {
			return err
		}
	}
	return nil
}
