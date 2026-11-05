package scriptruns

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// Restore item states. Only ready is selectable.
const (
	RestoreReady      = "ready"
	RestoreRunning    = "running"
	RestoreNeedsInput = "needsInput"
	RestoreGone       = "gone"
	RestoreBlocked    = "blocked"
)

// commandPreviewRunes caps the prompt excerpt a smart item shows as its command.
const commandPreviewRunes = 160

// restoreOffer holds the run ids a quit or crash ended, until the user answers.
type restoreOffer struct {
	mu  sync.Mutex
	ids []string
}

func (o *restoreOffer) set(ids []string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ids = ids
}

func (o *restoreOffer) list() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.ids...)
}

func (o *restoreOffer) has(id string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, x := range o.ids {
		if x == id {
			return true
		}
	}
	return false
}

func (o *restoreOffer) drop(gone map[string]bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	kept := o.ids[:0]
	for _, id := range o.ids {
		if !gone[id] {
			kept = append(kept, id)
		}
	}
	o.ids = kept
}

// RestoreItem is one run offered again, planned against the script as it is now.
type RestoreItem struct {
	RunID       string  `json:"runId"`
	ScriptID    string  `json:"scriptId"`
	ScriptName  string  `json:"scriptName"`
	Color       string  `json:"color"`
	Kind        string  `json:"kind"`
	Trigger     Trigger `json:"trigger"`
	TaskID      string  `json:"taskId"`
	TaskTitle   string  `json:"taskTitle"`
	BranchID    string  `json:"branchId"`
	BranchLabel string  `json:"branchLabel"`
	// Cwd and Command are what the new run would use, not what the old one did.
	Cwd     string `json:"cwd"`
	Command string `json:"command"`
	Model   string `json:"model"`
	// Resume says a smart run continues its saved conversation.
	Resume  bool       `json:"resume"`
	Params  []RunParam `json:"params"`
	EndedAt int64      `json:"endedAt"`
	Hash    string     `json:"hash"`
	Status  string     `json:"status"`
	Reason  string     `json:"reason"`
}

// RestoreState is the current offer.
type RestoreState struct {
	Items []RestoreItem `json:"items"`
}

// RestoreChoice names an offered run and the plan hash the user saw.
type RestoreChoice struct {
	RunID string `json:"runId"`
	Hash  string `json:"hash"`
}

// RestoreArgs lists the offered runs the user confirmed.
type RestoreArgs struct {
	Items []RestoreChoice `json:"items"`
}

// RestoreOutcome is one item's result; a started item leaves the offer, a failed one stays.
type RestoreOutcome struct {
	RunID   string   `json:"runId"`
	Started *Started `json:"started"`
	Code    string   `json:"code"`
	Error   string   `json:"error"`
}

// RestoreResult answers Restore, one outcome per chosen item in order.
type RestoreResult struct {
	Items []RestoreOutcome `json:"items"`
}

type restoreKey struct{ script, task, branch string }

func keyOf(r Run) restoreKey { return restoreKey{r.ScriptID, r.TaskID, r.BranchID} }

// offered loads the offered runs, newest first, one per script, task and branch.
func (s *Service) offered() ([]Run, error) {
	var runs []Run
	for _, id := range s.restore.list() {
		run, err := s.Runs.Get(id)
		if IsCallerError(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if run.Trigger == TriggerMCP {
			continue // the agent that asked is gone
		}
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool { return endedAt(runs[i]) > endedAt(runs[j]) })
	seen := map[restoreKey]bool{}
	out := runs[:0]
	for _, r := range runs {
		if !seen[keyOf(r)] {
			seen[keyOf(r)] = true
			out = append(out, r)
		}
	}
	return out, nil
}

func endedAt(r Run) int64 {
	if r.FinishedAt != nil {
		return *r.FinishedAt
	}
	return r.CreatedAt
}

// RestoreState plans every offered run against the current script and reports what can start.
func (s *Service) RestoreState() (RestoreState, error) {
	runs, err := s.offered()
	if err != nil {
		return RestoreState{}, err
	}
	items := make([]RestoreItem, 0, len(runs))
	for _, run := range runs {
		item, err := s.restoreItem(run)
		if err != nil {
			return RestoreState{}, err
		}
		items = append(items, item)
	}
	return RestoreState{Items: items}, nil
}

func (s *Service) restoreItem(run Run) (RestoreItem, error) {
	item := RestoreItem{
		RunID: run.ID, ScriptID: run.ScriptID, ScriptName: run.ScriptName, Color: run.Color, Kind: run.Kind,
		Trigger: run.Trigger, TaskID: run.TaskID, TaskTitle: run.TaskTitle, BranchID: run.BranchID, BranchLabel: run.BranchLabel, Cwd: run.Cwd,
		Resume: run.Kind == KindSmart && run.Resumable, Params: nonNilParams(run.Params), EndedAt: endedAt(run),
	}
	rec, err := s.Scripts.Get(run.ScriptID)
	if err != nil {
		return item, ipcerr.InternalErr(err)
	}
	if rec == nil {
		item.Status, item.Reason = RestoreGone, ipcerr.T(ipcerr.CodeNotFound, "automations.script.gone.error", nil).Message
		return item, nil
	}
	args, secrets := resumeArgs(run, *rec)
	p, err := s.plan(s.lifeCtx(), args, restorePlanOpts(run))
	if err != nil {
		var ie *ipcerr.Error
		switch {
		case !errors.As(err, &ie) || ie.Code == string(ipcerr.CodeInternal):
			return item, err
		case ie.Code == string(ipcerr.CodeNotFound):
			item.Status = RestoreGone
		default:
			item.Status = RestoreBlocked
		}
		item.Reason = ie.Message
		return item, nil
	}
	pv := p.preview
	item.ScriptName, item.Color, item.Cwd, item.Model, item.Hash = rec.Name, rec.Color, pv.Dir.Path, pv.Model, pv.Hash
	item.Command = pv.Command
	if run.Kind == KindSmart {
		item.Command = excerpt(scripts.PlainText(pv.Prompt), commandPreviewRunes)
	}
	names := uniqueNames(secrets, pv.Missing)
	switch {
	case len(names) > 0:
		item.Status = RestoreNeedsInput
		item.Reason = ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.restoreNeedsInput.body", map[string]string{"names": strings.Join(names, ", ")}).Message
	case pv.Blocker != "":
		item.Status, item.Reason = RestoreBlocked, pv.Blocker
	default:
		live, err := s.Runs.Running(run.ScriptID, run.TaskID, run.BranchID)
		if err != nil {
			return item, ipcerr.InternalErr(err)
		}
		item.Status = RestoreReady
		if live != nil {
			item.Status = RestoreRunning
			item.Reason = ipcerr.T(ipcerr.CodeConflict, "backend.core.scriptRun.alreadyRunning.body", map[string]string{"name": rec.Name}).Message
		}
	}
	return item, nil
}

// restorePlanOpts pins a continued smart run to its conversation's provider, as Resume does.
func restorePlanOpts(run Run) planOpts {
	if run.Kind == KindSmart && run.Resumable {
		return planOpts{provider: run.Provider, informational: true}
	}
	return planOpts{informational: true}
}

func excerpt(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}

// Restore starts the confirmed runs one by one. An id outside the offer refuses the whole call.
func (s *Service) Restore(args RestoreArgs) (RestoreResult, error) {
	for _, c := range args.Items {
		if !s.restore.has(c.RunID) {
			return RestoreResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.restoreNotOffered.body", nil)
		}
	}
	out := RestoreResult{Items: make([]RestoreOutcome, 0, len(args.Items))}
	for _, c := range args.Items {
		item := RestoreOutcome{RunID: c.RunID}
		started, err := s.restoreOne(c)
		if err != nil {
			var ie *ipcerr.Error
			item.Code, item.Error = string(ipcerr.CodeInternal), err.Error()
			if errors.As(err, &ie) {
				item.Code, item.Error = ie.Code, ie.Message
			}
		} else {
			item.Started = &started
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Service) restoreOne(c RestoreChoice) (Started, error) {
	run, err := s.Runs.Get(c.RunID)
	if IsCallerError(err) {
		return Started{}, ipcerr.T(ipcerr.CodeNotFound, "backend.core.scriptRun.runNotFound.body", nil)
	}
	if err != nil {
		return Started{}, ipcerr.InternalErr(err)
	}
	var started Started
	if run.Kind == KindSmart && run.Resumable {
		started, err = s.resumeLocked(run, c.Hash)
	} else {
		rec, gerr := s.Scripts.Get(run.ScriptID)
		if gerr != nil {
			return Started{}, ipcerr.InternalErr(gerr)
		}
		if rec == nil {
			return Started{}, ipcerr.T(ipcerr.CodeNotFound, "automations.script.gone.error", nil)
		}
		args, names := resumeArgs(run, *rec)
		if len(names) > 0 {
			return Started{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.resumeMissing.body", map[string]string{"names": strings.Join(names, ", ")})
		}
		started, err = s.Start(StartArgs{RunArgs: args, Hash: c.Hash})
	}
	if err != nil {
		return Started{}, err
	}
	s.dropKey(run)
	return started, nil
}

// resumeLocked resumes under startMu, refusing a second live run of the key as Start does.
func (s *Service) resumeLocked(run Run, hash string) (Started, error) {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	live, err := s.Runs.Running(run.ScriptID, run.TaskID, run.BranchID)
	if err != nil {
		return Started{}, ipcerr.InternalErr(err)
	}
	if live != nil {
		return Started{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.scriptRun.alreadyRunning.body", map[string]string{"name": run.ScriptName})
	}
	return s.resume(run.ID, hash, true)
}

// dropKey removes a started run and every other offered run of its key from the offer.
func (s *Service) dropKey(run Run) {
	key := keyOf(run)
	gone := map[string]bool{run.ID: true}
	for _, id := range s.restore.list() {
		if other, err := s.Runs.Get(id); err == nil && keyOf(other) == key {
			gone[id] = true
		}
	}
	s.restore.drop(gone)
}

// RestoreDismiss forgets the offer.
func (s *Service) RestoreDismiss() { s.restore.set(nil) }
