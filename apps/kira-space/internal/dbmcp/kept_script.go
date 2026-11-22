package dbmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// maxKeptScriptNames bounds the distinct names a script reads and the values it keeps per run.
const maxKeptScriptNames = 16

// keptRun is the kept variables binding of one api_request. Reads are fixed at plan time (the card lists
// them); writes are staged and reach the store only after the response is final.
type keptRun struct {
	store KeptStore
	scope string
	sites map[scriptStage][]keptvars.ScriptSite
	// uses are the variables the scripts may read, as they stood at plan time, with script sites.
	uses []agentapi.KeptUse
	// mayKeep are the names the transform declares, with their sites in its source.
	mayKeep []keptMayKeep

	mu           sync.Mutex
	prepareReads bool
	staged       map[string]json.RawMessage
	order        []string
	userDropped  []string // staged values the user removed; commit reports them
	// secretNames are the staged names a kira.storeSecret call wrote: secret whatever the script read.
	secretNames map[string]bool
	// secretRead taints every value the run stages: it may have copied a secret.
	secretRead  bool
	secretForms []string
	// sourceExpiry is the earliest expiry among the secret or bound values read.
	sourceExpiry time.Time
	// boundOrigin is the origin of the bound (user or user-derived) values the run read; mixedOrigins is set
	// when it read bound values of two origins, so no single origin can own what it derives.
	boundOrigin  string
	mixedOrigins bool
}

// keptMayKeep is a name the transform script may keep, with where its source says so. Secret is set when a
// kira.storeSecret call names it; a value may also turn secret at run time (the script read a secret).
type keptMayKeep struct {
	Name   string              `json:"name"`
	Secret bool                `json:"secret"`
	Sites  []agentapi.KeptSite `json:"sites"`
}

// keptRunFor reads the scripts for kira.getValue and kira.storeValue calls and snapshots what they may read. nil
// without kept support or without a call. A non-empty string is the refusal text; nothing ran.
func (s *Server) keptRunFor(scope string, sc *apiScripts, nothing string) (*keptRun, string) {
	if s.cfg.Kept == nil || sc == nil {
		return nil, ""
	}
	k := &keptRun{store: s.cfg.Kept, scope: scope, sites: map[scriptStage][]keptvars.ScriptSite{}, staged: map[string]json.RawMessage{}, secretNames: map[string]bool{}}
	k.sites[stagePrepare] = keptvars.ScanScript(sc.Prepare)
	k.sites[stageTransform] = keptvars.ScanScript(sc.Transform)
	if slices.ContainsFunc(k.sites[stagePrepare], func(st keptvars.ScriptSite) bool { return st.Op != "getValue" }) {
		return nil, "kira.storeValue and kira.storeSecret are available in the transform script only. " + nothing
	}
	if len(k.sites[stagePrepare]) == 0 && len(k.sites[stageTransform]) == 0 {
		return nil, ""
	}
	reads, sets := map[string]bool{}, map[string]bool{}
	for _, stage := range []scriptStage{stagePrepare, stageTransform} {
		for _, st := range k.sites[stage] {
			if st.Op == "getValue" {
				reads[st.Name] = true
			} else {
				sets[st.Name] = true
			}
		}
	}
	if len(reads) > maxKeptScriptNames || len(sets) > maxKeptScriptNames {
		return nil, fmt.Sprintf("a script can read at most %d kept variables and keep at most %d. %s", maxKeptScriptNames, maxKeptScriptNames, nothing)
	}
	for _, stage := range []scriptStage{stagePrepare, stageTransform} {
		for _, st := range k.sites[stage] {
			site := agentapi.KeptSite{Field: "script", Part: string(stage), From: st.From, To: st.To}
			if st.Op != "getValue" {
				k.addMayKeep(st.Name, site, st.Op == "storeSecret")
				continue
			}
			k.addUse(st.Name, site)
		}
	}
	return k, ""
}

func (k *keptRun) addMayKeep(name string, site agentapi.KeptSite, secret bool) {
	for i := range k.mayKeep {
		if k.mayKeep[i].Name == name {
			k.mayKeep[i].Sites = append(k.mayKeep[i].Sites, site)
			k.mayKeep[i].Secret = k.mayKeep[i].Secret || secret
			return
		}
	}
	k.mayKeep = append(k.mayKeep, keptMayKeep{Name: name, Secret: secret, Sites: []agentapi.KeptSite{site}})
}

func (k *keptRun) addUse(name string, site agentapi.KeptSite) {
	for i := range k.uses {
		if k.uses[i].Name == name {
			k.uses[i].Sites = append(k.uses[i].Sites, site)
			return
		}
	}
	e, ok := k.lookup(name)
	if !ok {
		return
	}
	u := agentapi.KeptUse{
		ID: e.ID, Name: name, Type: e.Type, Items: e.Items, Bytes: e.Bytes, Secret: e.Secret,
		Source: e.Source, Origin: e.Origin, CreatedAt: e.CreatedAt, ExpiresAt: e.ExpiresAt, Sites: []agentapi.KeptSite{site},
	}
	if !e.Secret {
		u.Preview, u.PreviewTruncated = keptvars.Preview(e.Value, keptvars.PreviewBytes)
	}
	k.uses = append(k.uses, u)
}

func (k *keptRun) lookup(name string) (keptvars.Entry, bool) {
	return k.store.Get(k.scope, name)
}

// rebind refreshes the uses of names after the store changed them (a user value was supplied or replaced): a use made at plan time holds the old entry id, or none for a value that
// did not exist yet.
func (k *keptRun) rebind(names []string) {
	if k == nil {
		return
	}
	for _, name := range names {
		var sites []agentapi.KeptSite
		if i := slices.IndexFunc(k.uses, func(u agentapi.KeptUse) bool { return u.Name == name }); i >= 0 {
			sites = k.uses[i].Sites
			k.uses = slices.Delete(k.uses, i, i+1)
		} else {
			for _, stage := range []scriptStage{stagePrepare, stageTransform} {
				for _, st := range k.sites[stage] {
					if st.Op == "getValue" && st.Name == name {
						sites = append(sites, agentapi.KeptSite{Field: "script", Part: string(stage), From: st.From, To: st.To})
					}
				}
			}
		}
		for _, site := range sites {
			k.addUse(name, site)
		}
	}
}

// readsSecret reports whether a script reads a secret kept value (a user value, one derived from it, or any
// other secret). A transform can reshape it past any mask, so the result always goes through review.
func (k *keptRun) readsSecret() bool {
	if k == nil {
		return false
	}
	return slices.ContainsFunc(k.uses, func(u agentapi.KeptUse) bool { return u.Secret || u.Bound() })
}

// declared reports whether the stage's source calls op with the literal name.
func (k *keptRun) declared(stage scriptStage, op, name string) bool {
	return slices.ContainsFunc(k.sites[stage], func(st keptvars.ScriptSite) bool { return st.Op == op && st.Name == name })
}

// Bindings is the kira global for stage, or none when its source makes no call (a script may use its own
// variable named kira).
func (k *keptRun) Bindings(stage scriptStage) []jsengine.Binding {
	if k == nil || len(k.sites[stage]) == 0 {
		return nil
	}
	funcs := map[string]jsengine.HostFunc{"getValue": k.get(stage)}
	if stage == stageTransform {
		funcs["storeValue"] = k.set(false)
		funcs["storeSecret"] = k.set(true)
	}
	return []jsengine.Binding{{Name: "kira", Funcs: funcs}}
}

func hostArgs(args json.RawMessage, want int, usage string) ([]json.RawMessage, error) {
	var a []json.RawMessage
	if err := json.Unmarshal(args, &a); err != nil || len(a) != want {
		return nil, errors.New(usage)
	}
	return a, nil
}

func (k *keptRun) get(stage scriptStage) jsengine.HostFunc {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		a, err := hostArgs(args, 1, "kira.getValue takes a name")
		if err != nil {
			return nil, err
		}
		var name string
		if json.Unmarshal(a[0], &name) != nil || !k.declared(stage, "getValue", name) {
			return nil, errors.New("this kept variable is not listed in this script: call kira.getValue with a literal name")
		}
		planned := slices.IndexFunc(k.uses, func(u agentapi.KeptUse) bool { return u.Name == name })
		e, ok := k.lookup(name)
		if planned < 0 || !ok || e.ID != k.uses[planned].ID {
			return nil, fmt.Errorf("no kept variable %q (it may have expired, been cleared or been replaced since approval)", name)
		}
		if e.Secret {
			if stage == stagePrepare {
				return nil, fmt.Errorf("kept variable %q is secret; write {{kept.%s}} in the request instead of reading it in prepare", name, name)
			}
			k.noteSecret(e)
		}
		if e.Bound() {
			k.noteBound(e.Origin, e.ExpiresAt)
		}
		k.mu.Lock()
		k.prepareReads = k.prepareReads || stage == stagePrepare
		k.mu.Unlock()
		return e.Value, nil
	}
}

// noteBound records the origin a read bound value ties the run's derived values to.
func (k *keptRun) noteBound(origin string, expiresAt time.Time) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.noteExpiry(expiresAt)
	switch {
	case k.boundOrigin == "":
		k.boundOrigin = origin
	case k.boundOrigin != origin:
		k.mixedOrigins = true
	}
}

func (k *keptRun) noteSecret(e keptvars.Entry) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.secretRead = true
	k.noteExpiry(e.ExpiresAt)
	// A string is masked by its text, so a quoted JSON string stays valid JSON; a structure by its leaves too.
	k.secretForms = append(k.secretForms, keptvars.SecretTexts(e)...)
}

// noteExpiry keeps the earliest expiry of the secret or bound values the run read; a derived value never
// outlives its source. The caller holds k.mu.
func (k *keptRun) noteExpiry(at time.Time) {
	if k.sourceExpiry.IsZero() || at.Before(k.sourceExpiry) {
		k.sourceExpiry = at
	}
}

// set stages a value for kira.storeValue, or for kira.storeSecret when secret; nothing reaches the store before commit.
func (k *keptRun) set(secret bool) jsengine.HostFunc {
	op, call := "storeValue", "kira.storeValue"
	if secret {
		op, call = "storeSecret", "kira.storeSecret"
	}
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		a, err := hostArgs(args, 2, call+" takes a name and a value")
		if err != nil {
			return nil, err
		}
		var name string
		if json.Unmarshal(a[0], &name) != nil || !k.declared(stageTransform, op, name) {
			return nil, errors.New("this kept variable is not listed in this script: call " + call + " with a literal name")
		}
		if len(a[1]) > keptvars.MaxValueBytes {
			return nil, fmt.Errorf("kept variable %q: %w", name, keptvars.ErrTooLarge)
		}
		typ, _, _, err := keptvars.Describe(a[1])
		if err != nil {
			return nil, fmt.Errorf("kept variable %q: %w", name, err)
		}
		k.mu.Lock()
		defer k.mu.Unlock()
		if _, seen := k.staged[name]; !seen {
			if len(k.order) >= maxKeptScriptNames {
				return nil, fmt.Errorf("a script can keep at most %d values per run", maxKeptScriptNames)
			}
			k.order = append(k.order, name)
		}
		k.staged[name] = bytes.Clone(a[1])
		if secret {
			k.secretNames[name] = true
			// The agent never sees the value: mask it from the transform's result like a secret the script read.
			k.secretForms = append(k.secretForms, keptvars.SecretTexts(keptvars.Entry{Meta: keptvars.Meta{Type: typ}, Value: a[1]})...)
		}
		return nil, nil
	}
}

// secretStaged reports whether the staged name will be stored as a secret. The caller holds k.mu.
func (k *keptRun) secretStaged(name string) bool { return k.secretRead || k.secretNames[name] }

// drop forgets the staged values: the user withheld the response, or nothing completed.
func (k *keptRun) drop() {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, v := range k.staged {
		clear(v)
	}
	clear(k.staged)
	clear(k.secretNames)
	k.order = nil
}

// dropName forgets one staged value the user chose not to keep. false for an unknown name.
func (k *keptRun) dropName(name string) bool {
	if k == nil {
		return false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.staged[name]
	if !ok {
		return false
	}
	clear(v)
	delete(k.staged, name)
	delete(k.secretNames, name)
	k.order = slices.DeleteFunc(k.order, func(n string) bool { return n == name })
	k.userDropped = append(k.userDropped, name)
	return true
}

// stagedViews is what the review card shows under "Will keep": a preview per value, none for a secret (one
// kira.storeSecret wrote, or one the run may have copied from a secret).
func (k *keptRun) stagedViews() []HeldKeep {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]HeldKeep, 0, len(k.order))
	for _, name := range k.order {
		v := k.staged[name]
		typ, items, size, _ := keptvars.Describe(v)
		h := HeldKeep{Name: name, Type: typ, Items: items, Bytes: size, Secret: k.secretStaged(name)}
		if !h.Secret {
			h.Preview, h.PreviewTruncated = keptvars.Preview(v, keptvars.PreviewBytes)
		}
		out = append(out, h)
	}
	return out
}

// stagedSecret is the text of a staged secret value, for the user's reveal while its result is held. false
// for an unknown name, a value that is not secret (its preview is shown) or one already stored or dropped.
func (k *keptRun) stagedSecret(name string) (string, bool) {
	if k == nil {
		return "", false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.staged[name]
	if !ok || !k.secretStaged(name) {
		return "", false
	}
	typ, _, _, _ := keptvars.Describe(v)
	return keptvars.SecretText(keptvars.Entry{Meta: keptvars.Meta{Type: typ}, Value: v}), true
}

// keptSummary is what the agent learns about a value it kept: never the value.
type keptSummary struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	Items            int    `json:"items"`
	Secret           bool   `json:"secret"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
}

// errMixedOrigins is the refusal text for values derived from user values bound to different origins.
const errMixedOrigins = "the script read user values supplied for different origins, so its values cannot be bound to one origin"

// errUserDropped is the refusal text for a staged value the user removed before sending the result.
const errUserDropped = "the user chose not to keep it"

// keptFailure is a staged value the store refused.
type keptFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// commit stores the staged values. A refusal names its cap or rule, never a value. secretOrigin is the origin a
// kira.storeSecret value is bound to ("" binds none): the one its request went to, so the agent cannot send a
// session token somewhere else.
func (k *keptRun) commit(src keptvars.Source, secretOrigin string) ([]keptSummary, []keptFailure) {
	if k == nil {
		return nil, nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	var saved []keptSummary
	var failed []keptFailure
	for _, name := range k.userDropped {
		failed = append(failed, keptFailure{Name: name, Error: errUserDropped})
	}
	k.userDropped = nil
	for _, name := range k.order {
		if k.mixedOrigins {
			failed = append(failed, keptFailure{Name: name, Error: errMixedOrigins})
			continue
		}
		put := keptvars.Put{Scope: k.scope, Name: name, Value: k.staged[name], Secret: k.secretStaged(name), Source: src, Origin: k.boundOrigin}
		if put.Origin == "" && k.secretNames[name] {
			put.Origin = secretOrigin
		}
		put.NotAfter = k.sourceExpiry
		m, err := k.store.Put(put)
		if err != nil {
			failed = append(failed, keptFailure{Name: name, Error: err.Error()})
			continue
		}
		saved = append(saved, keptSummary{Name: name, Type: m.Type, Items: m.Items, Secret: m.Secret, ExpiresInSeconds: max(int(math.Ceil(time.Until(m.ExpiresAt).Seconds())), 0)})
	}
	for _, v := range k.staged {
		clear(v)
	}
	clear(k.staged)
	clear(k.secretNames)
	k.order = nil
	return saved, failed
}

// mask removes the secret kept values the transform read from text bound for the agent.
func (k *keptRun) mask(text string) string {
	if k == nil {
		return text
	}
	k.mu.Lock()
	forms := slices.Clone(k.secretForms)
	k.mu.Unlock()
	if len(forms) == 0 {
		return text
	}
	return secrets.NewMasker(forms...).Mask(text)
}

// checkPrepared refuses a prepare result that holds a {{...}} reference the user could not have read in the
// script or the agent's request. Without it a kept value holding "{{token}}" would be resolved as a secret
// reference once the script copies it into the request. "" is ok. The refusal never quotes the text.
func (k *keptRun) checkPrepared(orig, merged agentapi.Input, source string) string {
	if k == nil {
		return ""
	}
	k.mu.Lock()
	read := k.prepareReads
	k.mu.Unlock()
	if !read {
		return ""
	}
	known := source + "\x00" + inputText(orig)
	for _, span := range referenceSpans(inputText(merged)) {
		if !strings.Contains(known, span) {
			return "prepare script returned a {{...}} reference that is neither in its source nor in your request; references built from kept data are not resolved. Nothing was sent."
		}
	}
	return ""
}

func inputText(in agentapi.Input) string {
	parts := []string{in.URL, in.Body, in.ContentType}
	for _, list := range [][]agentapi.NameValue{in.Headers, in.Query, in.Form} {
		for _, nv := range list {
			parts = append(parts, nv.Name, nv.Value)
		}
	}
	return strings.Join(parts, "\x00")
}

// referenceSpans lists the {{...}} spans of text, same grammar as the request references: `{{` to the next `}}`.
func referenceSpans(text string) []string {
	var out []string
	for {
		open := strings.Index(text, "{{")
		if open < 0 {
			return out
		}
		end := strings.Index(text[open+2:], "}}")
		if end < 0 {
			return out
		}
		out = append(out, text[open:open+2+end+2])
		text = text[open+2+end+2:]
	}
}

// mergeKept adds the script's reads to the request's uses: one row per name, the script's sites joined.
func mergeKept(uses, script []agentapi.KeptUse) []agentapi.KeptUse {
	out := slices.Clone(uses)
	for _, su := range script {
		i := slices.IndexFunc(out, func(u agentapi.KeptUse) bool { return u.Name == su.Name })
		if i < 0 {
			out = append(out, su)
			continue
		}
		out[i].Sites = append(slices.Clone(out[i].Sites), su.Sites...)
	}
	return out
}

// keptUsesOf is every kept variable the request and its scripts read.
func keptUsesOf(plan agentapi.Planned, sc *apiScripts) []agentapi.KeptUse {
	if sc == nil || sc.kept == nil {
		return plan.Kept
	}
	return mergeKept(plan.Kept, sc.kept.uses)
}

// keptSource is where a value kept by this request comes from; Request is display text, templates only.
func keptSourceOf(ctx context.Context, plan agentapi.Planned) keptvars.Source {
	o := originFrom(ctx)
	req, _ := capUTF8(plan.Method+" "+plan.URL, 512)
	return keptvars.Source{
		Kind: keptvars.SourceScript, MCP: "api", Tool: "api_request", Request: req,
		Client: o.Client, TaskID: o.TaskID, SessionID: o.SessionID,
	}
}
