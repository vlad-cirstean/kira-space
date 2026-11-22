package dbmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
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

// maxKeptScriptNames bounds the distinct names a script mentions and the values it saves per run.
const maxKeptScriptNames = 16

// kindVars marks a failure of the plain and secret globals (a refused write, a changed value): the text is
// the whole message.
const kindVars jsengine.Kind = "vars"

// keptRun is the plain and secret globals of one script run (api_request, run_query, kube reads). Names are
// read from the source at plan time (the card lists them); writes are staged and reach the store only after
// the response is final.
type keptRun struct {
	store KeptStore
	scope string
	sites map[scriptStage][]keptvars.ScriptSite
	// uses are the plain temporary variables the scripts mention, as they stood at plan time, with script sites.
	uses []agentapi.KeptUse
	// mayKeep are the names the scripts write or delete, with their sites in the source.
	mayKeep []keptMayKeep

	mu sync.Mutex
	// prepareReads is set when the prepare script was handed temporary values: it may copy one into the request.
	prepareReads bool
	// injected is what each stage's plain object held when it started: the baseline of the diff.
	injected     map[scriptStage]map[string]json.RawMessage
	staged       map[string]json.RawMessage
	order        []string
	deleted      map[string]string // name -> the entry id the script saw
	deletedOrder []string
	userDropped  []string // staged values the user removed; commit reports them
	// secretNames are the staged names a secret.x assignment wrote.
	secretNames map[string]bool
	secretForms []string
}

// keptMayKeep is a name the scripts may save or delete, with where the source says so.
type keptMayKeep struct {
	Name   string              `json:"name"`
	Secret bool                `json:"secret"`
	Sites  []agentapi.KeptSite `json:"sites"`
}

var scriptStages = []scriptStage{stagePrepare, stageTransform}

// keptRunFor reads the scripts for plain and secret mentions and snapshots the temporary values they may see. nil
// without temporary-variable support or without a mention. A non-empty string is the refusal text; nothing ran.
// tool names the MCP tool, for the secret-read refusal.
func (s *Server) keptRunFor(scope string, sc *apiScripts, tool, nothing string) (*keptRun, string) {
	if s.cfg.Kept == nil || sc == nil {
		return nil, ""
	}
	k := &keptRun{
		store: s.cfg.Kept, scope: scope, sites: map[scriptStage][]keptvars.ScriptSite{},
		injected: map[scriptStage]map[string]json.RawMessage{},
		staged:   map[string]json.RawMessage{}, secretNames: map[string]bool{}, deleted: map[string]string{},
	}
	k.sites[stagePrepare] = keptvars.ScanScript(sc.Prepare)
	k.sites[stageTransform] = keptvars.ScanScript(sc.Transform)
	if len(k.sites[stagePrepare]) == 0 && len(k.sites[stageTransform]) == 0 {
		return nil, ""
	}
	mentions, writes := map[string]bool{}, map[string]bool{}
	for _, stage := range scriptStages {
		for _, st := range k.sites[stage] {
			if st.Kind == keptvars.KindPlain {
				mentions[st.Name] = true
			}
			if st.Op != keptvars.OpRead {
				writes[st.Kind+"."+st.Name] = true
			}
		}
	}
	if len(mentions) > maxKeptScriptNames || len(writes) > maxKeptScriptNames {
		return nil, fmt.Sprintf("a script can mention at most %d temporary variables and set at most %d. %s", maxKeptScriptNames, maxKeptScriptNames, nothing)
	}
	if text := k.refuseSecretReads(tool); text != "" {
		return nil, text + " " + nothing
	}
	for _, stage := range scriptStages {
		for _, st := range k.sites[stage] {
			site := agentapi.KeptSite{Field: "script", Part: string(stage), From: st.From, To: st.To}
			if st.Op != keptvars.OpRead {
				k.addMayKeep(st.Name, site, st.Kind == keptvars.KindSecret)
			}
			if st.Kind == keptvars.KindPlain {
				k.addUse(st.Name, site)
			}
		}
	}
	return k, ""
}

// refuseSecretReads is the refusal text for a script that reads a temporary secret (as plain.x or secret.x), or writes a
// name as the other kind than it is set as. "" is ok. Secrets are used in api_request as {{secret.x}}.
func (k *keptRun) refuseSecretReads(tool string) string {
	for _, stage := range scriptStages {
		for _, st := range k.sites[stage] {
			e, ok := k.lookup(st.Name)
			switch {
			case !ok:
			case st.Kind == keptvars.KindPlain && e.Secret && st.Op == keptvars.OpWrite:
				return clashText(st.Name, true)
			case st.Kind == keptvars.KindPlain && e.Secret && tool == "api_request":
				return fmt.Sprintf("%q is secret; scripts cannot read secrets. Write {{secret.%s}} in the request.", st.Name, st.Name)
			case st.Kind == keptvars.KindPlain && e.Secret:
				return fmt.Sprintf("%q is secret; secrets are used only in api_request, as {{secret.%s}}.", st.Name, st.Name)
			case st.Kind == keptvars.KindSecret && !e.Secret:
				return clashText(st.Name, false)
			case st.Kind == keptvars.KindSecret && st.Op == keptvars.OpRead && tool == "api_request":
				return fmt.Sprintf("%q is secret; scripts cannot read secrets. Write {{secret.%s}} in the request.", st.Name, st.Name)
			case st.Kind == keptvars.KindSecret && st.Op == keptvars.OpRead:
				return fmt.Sprintf("%q is secret; secrets are used only in api_request, as {{secret.%s}}.", st.Name, st.Name)
			}
		}
	}
	return ""
}

// clashText is the refusal for a name written as the other kind than the store holds it as.
func clashText(name string, isSecret bool) string {
	if isSecret {
		return fmt.Sprintf("%q is a secret temporary variable; write secret.%s or use another name.", name, name)
	}
	return fmt.Sprintf("%q is a plain temporary variable; write plain.%s or use another name.", name, name)
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
	if !ok || e.Secret {
		return
	}
	u := agentapi.KeptUse{
		ID: e.ID, Name: name, Type: e.Type, Items: e.Items, Bytes: e.Bytes, Secret: e.Secret,
		Source: e.Source, Origin: e.Origin, CreatedAt: e.CreatedAt, ExpiresAt: e.ExpiresAt, Sites: []agentapi.KeptSite{site},
	}
	u.Preview, u.PreviewTruncated = keptvars.Preview(e.Value, keptvars.PreviewBytes)
	k.uses = append(k.uses, u)
}

func (k *keptRun) lookup(name string) (keptvars.Entry, bool) {
	if keptvars.ValidName(name) != nil {
		return keptvars.Entry{}, false
	}
	return k.store.Get(k.scope, name)
}

// bindings are the plain and secret globals of stage: plain holds the current values of the names the stage
// mentions, secret starts empty. none when the source mentions neither (a script may use its own variable
// of that name). A non-empty string is the refusal text: a value changed since the plan.
func (k *keptRun) bindings(stage scriptStage) ([]jsengine.Binding, string) {
	if k == nil || len(k.sites[stage]) == 0 {
		return nil, ""
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	plain := map[string]json.RawMessage{}
	for _, st := range k.sites[stage] {
		if st.Kind != keptvars.KindPlain {
			continue
		}
		if _, done := plain[st.Name]; done {
			continue
		}
		if v, ok := k.staged[st.Name]; ok && !k.secretNames[st.Name] {
			plain[st.Name] = bytes.Clone(v)
			continue
		}
		if _, gone := k.deleted[st.Name]; gone {
			continue
		}
		i := slices.IndexFunc(k.uses, func(u agentapi.KeptUse) bool { return u.Name == st.Name })
		if i < 0 {
			continue
		}
		e, ok := k.lookup(st.Name)
		if !ok || e.ID != k.uses[i].ID {
			return nil, fmt.Sprintf("no temporary variable %q (it may have expired, been cleared or been replaced since approval)", st.Name)
		}
		plain[st.Name] = e.Value
	}
	k.injected[stage] = maps.Clone(plain)
	k.prepareReads = k.prepareReads || stage == stagePrepare && len(plain) > 0
	plainJSON, _ := json.Marshal(plain)
	return []jsengine.Binding{
		{Name: keptvars.KindPlain, Value: plainJSON, ReadBack: true},
		{Name: keptvars.KindSecret, Value: json.RawMessage("{}"), ReadBack: true},
	}, ""
}

var invalidTypeText = map[string]string{
	"function": "a function", "symbol": "a symbol", "undefined": "undefined", "accessor": "an accessor property",
}

// scriptWrite is one staged write: kind is plain or secret.
type scriptWrite struct {
	kind, name string
	value      json.RawMessage
}

// scriptChanges is what a run changed: its writes, its plain deletes and every key written as a secret.
type scriptChanges struct {
	writes     []scriptWrite
	deletes    []string
	secretKeys map[string]bool
}

// diffGlobals compares the globals after a run with the plain values injected before it. A refusal text
// names the rule and never a value.
func diffGlobals(rb map[string]jsengine.ReadBack, inj map[string]json.RawMessage) (scriptChanges, string) {
	ch := scriptChanges{secretKeys: map[string]bool{}}
	plainKeys := map[string]bool{}
	for _, kind := range []string{keptvars.KindPlain, keptvars.KindSecret} {
		r := rb[kind]
		names := slices.Sorted(maps.Keys(r.JSON))
		names = append(names, slices.Sorted(maps.Keys(r.Invalid))...)
		for _, name := range names {
			if err := keptvars.ValidName(name); err != nil {
				return ch, fmt.Sprintf("%s[%q] is not a valid name: start with a letter, then letters, digits or _, at most 40.", kind, name)
			}
			if typ, bad := r.Invalid[name]; bad {
				return ch, fmt.Sprintf("%s.%s is %s; set text, a number, true/false, null, a list or an object.", kind, name, invalidTypeText[typ])
			}
			if kind == keptvars.KindSecret {
				ch.secretKeys[name] = true
				ch.writes = append(ch.writes, scriptWrite{kind, name, r.JSON[name]})
				continue
			}
			plainKeys[name] = true
			if old, had := inj[name]; !had || !bytes.Equal(old, r.JSON[name]) {
				ch.writes = append(ch.writes, scriptWrite{kind, name, r.JSON[name]})
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(inj)) {
		if !plainKeys[name] {
			ch.deletes = append(ch.deletes, name)
		}
	}
	return ch, ""
}

// checkKinds refuses a name written as both kinds, or in the other kind from what the run or the store holds.
func (k *keptRun) checkKinds(ch scriptChanges) string {
	for _, w := range ch.writes {
		if w.kind == keptvars.KindPlain && ch.secretKeys[w.name] {
			return bothKindsText(w.name)
		}
	}
	for _, name := range ch.deletes {
		if ch.secretKeys[name] {
			return bothKindsText(name)
		}
	}
	for _, w := range ch.writes {
		if _, ok := k.staged[w.name]; ok && k.secretNames[w.name] != (w.kind == keptvars.KindSecret) {
			return bothKindsText(w.name)
		}
	}
	return ""
}

// checkSites refuses a write or delete the script does not spell out literally, so the user sees it on the card.
func (k *keptRun) checkSites(stage scriptStage, ch scriptChanges) string {
	for _, w := range ch.writes {
		if !k.hasSite(stage, w.kind, w.name, keptvars.OpWrite) {
			return fmt.Sprintf("%s.%s is not written literally in the script; write %s.%s = … so the user sees it.", w.kind, w.name, w.kind, w.name)
		}
		if e, ok := k.lookup(w.name); ok && e.Secret != (w.kind == keptvars.KindSecret) {
			return clashText(w.name, e.Secret)
		}
	}
	for _, name := range ch.deletes {
		if !k.hasSite(stage, keptvars.KindPlain, name, keptvars.OpDelete) {
			return fmt.Sprintf("plain.%s is not deleted literally in the script; write delete plain.%s so the user sees it.", name, name)
		}
	}
	return ""
}

// checkLimits refuses a run that stages too many names or a value the store would not take.
func (k *keptRun) checkLimits(ch scriptChanges) string {
	added := 0
	for _, w := range ch.writes {
		if _, seen := k.staged[w.name]; !seen {
			added++
		}
	}
	if len(k.order)+added > maxKeptScriptNames {
		return fmt.Sprintf("a script can set at most %d values per run", maxKeptScriptNames)
	}
	for _, w := range ch.writes {
		if len(w.value) > keptvars.MaxValueBytes {
			return fmt.Sprintf("temporary variable %q: %s", w.name, keptvars.ErrTooLarge)
		}
		if _, _, _, err := keptvars.Describe(w.value); err != nil {
			return fmt.Sprintf("temporary variable %q: %s", w.name, err)
		}
	}
	return ""
}

// settle diffs the globals after a successful run and stages what changed: a plain key that is new or
// changed is a write, one that is gone a delete, every secret key a write. A refusal fails the stage like
// a thrown error; the text names the rule and never a value. "" is ok and everything was staged.
func (k *keptRun) settle(stage scriptStage, rb map[string]jsengine.ReadBack) string {
	if k == nil || len(rb) == 0 {
		return ""
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	ch, refusal := diffGlobals(rb, k.injected[stage])
	if refusal != "" {
		return refusal
	}
	// Every refusal comes before the first staging: a refusal leaves nothing half staged.
	for _, check := range []func() string{
		func() string { return k.checkKinds(ch) },
		func() string { return k.checkSites(stage, ch) },
		func() string { return k.checkLimits(ch) },
	} {
		if refusal := check(); refusal != "" {
			return refusal
		}
	}
	k.stage(ch)
	return ""
}

// stage records the checked changes.
func (k *keptRun) stage(ch scriptChanges) {
	for _, w := range ch.writes {
		if _, seen := k.staged[w.name]; !seen {
			k.order = append(k.order, w.name)
		}
		k.staged[w.name] = bytes.Clone(w.value)
		delete(k.deleted, w.name)
		k.deletedOrder = slices.DeleteFunc(k.deletedOrder, func(n string) bool { return n == w.name })
		if w.kind == keptvars.KindSecret {
			k.secretNames[w.name] = true
			typ, _, _, _ := keptvars.Describe(w.value)
			// The agent never sees the value: mask it from the transform's result like any secret.
			k.secretForms = append(k.secretForms, keptvars.SecretTexts(keptvars.Entry{Meta: keptvars.Meta{Type: typ}, Value: w.value})...)
		} else {
			delete(k.secretNames, w.name)
		}
	}
	for _, name := range ch.deletes {
		if v, ok := k.staged[name]; ok {
			clear(v)
			delete(k.staged, name)
			k.order = slices.DeleteFunc(k.order, func(n string) bool { return n == name })
		}
		if i := slices.IndexFunc(k.uses, func(u agentapi.KeptUse) bool { return u.Name == name }); i >= 0 {
			if _, dup := k.deleted[name]; !dup {
				k.deletedOrder = append(k.deletedOrder, name)
			}
			k.deleted[name] = k.uses[i].ID
		}
	}
}

func bothKindsText(name string) string {
	return fmt.Sprintf("%q is written as both plain.%s and secret.%s; use one.", name, name, name)
}

// hasSite reports whether stage's source has a literal site of kind for name with op (a write also counts
// for a read).
func (k *keptRun) hasSite(stage scriptStage, kind, name, op string) bool {
	return slices.ContainsFunc(k.sites[stage], func(st keptvars.ScriptSite) bool {
		return st.Kind == kind && st.Name == name && st.Op == op
	})
}

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
	clear(k.deleted)
	k.order, k.deletedOrder = nil, nil
}

// dropName forgets one staged write or delete the user chose not to apply. false for an unknown name.
func (k *keptRun) dropName(name string) bool {
	if k == nil {
		return false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.staged[name]
	_, wasDeleted := k.deleted[name]
	if !ok && !wasDeleted {
		return false
	}
	clear(v)
	delete(k.staged, name)
	delete(k.secretNames, name)
	delete(k.deleted, name)
	k.order = slices.DeleteFunc(k.order, func(n string) bool { return n == name })
	k.deletedOrder = slices.DeleteFunc(k.deletedOrder, func(n string) bool { return n == name })
	k.userDropped = append(k.userDropped, name)
	return true
}

// stagedEntries are the values the scripts staged so far, as entries a request can interpolate. Their ids
// carry stagedIDPrefix; the values are copies.
func (k *keptRun) stagedEntries() map[string]keptvars.Entry {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.staged) == 0 {
		return nil
	}
	out := make(map[string]keptvars.Entry, len(k.staged))
	for name, v := range k.staged {
		typ, items, size, _ := keptvars.Describe(v)
		out[name] = keptvars.Entry{
			Meta:  keptvars.Meta{ID: keptvars.StagedIDPrefix + name, Scope: k.scope, Name: name, Type: typ, Items: items, Bytes: size, Secret: k.secretNames[name]},
			Value: bytes.Clone(v),
		}
	}
	return out
}

// stagedViews is what the review card shows under "Will set": a preview per value, none for a secret, and a
// row per temporary value the script deletes.
func (k *keptRun) stagedViews() []HeldKeep {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]HeldKeep, 0, len(k.order)+len(k.deletedOrder))
	for _, name := range k.order {
		v := k.staged[name]
		typ, items, size, _ := keptvars.Describe(v)
		h := HeldKeep{Name: name, Type: typ, Items: items, Bytes: size, Secret: k.secretNames[name]}
		if !h.Secret {
			h.Preview, h.PreviewTruncated = keptvars.Preview(v, keptvars.PreviewBytes)
		}
		out = append(out, h)
	}
	for _, name := range k.deletedOrder {
		h := HeldKeep{Name: name, Deleted: true}
		if i := slices.IndexFunc(k.uses, func(u agentapi.KeptUse) bool { return u.Name == name }); i >= 0 {
			h.Type, h.Items, h.Bytes = k.uses[i].Type, k.uses[i].Items, k.uses[i].Bytes
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
	if !ok || !k.secretNames[name] {
		return "", false
	}
	typ, _, _, _ := keptvars.Describe(v)
	return keptvars.SecretText(keptvars.Entry{Meta: keptvars.Meta{Type: typ}, Value: v}), true
}

// keptSummary is what the agent learns about a value it set: never the value.
type keptSummary struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	Items            int    `json:"items"`
	Secret           bool   `json:"secret"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
}

// errUserDropped is the refusal text for a staged value the user removed before sending the result.
const errUserDropped = "the user chose not to set it"

// errDeleteChanged is the refusal text for a delete of a value that is not the one the script read.
const errDeleteChanged = "changed since the script read it"

// keptFailure is a staged value the store refused.
type keptFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// commit stores the staged values and deletes the ones the script removed. A refusal names its cap or rule,
// never a value. secretOrigin is the origin a secret.x value is bound to ("" binds none): the one its request
// went to, so the agent cannot send a session token somewhere else.
func (k *keptRun) commit(src keptvars.Source, secretOrigin string) (saved []keptSummary, failed []keptFailure, deleted []string) {
	if k == nil {
		return nil, nil, nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, name := range k.userDropped {
		failed = append(failed, keptFailure{Name: name, Error: errUserDropped})
	}
	k.userDropped = nil
	for _, name := range k.deletedOrder {
		e, ok := k.store.Get(k.scope, name)
		switch {
		case !ok:
			deleted = append(deleted, name)
		case e.ID != k.deleted[name]:
			failed = append(failed, keptFailure{Name: name, Error: errDeleteChanged})
		default:
			k.store.Clear(e.ID)
			deleted = append(deleted, name)
		}
	}
	for _, name := range k.order {
		put := keptvars.Put{Scope: k.scope, Name: name, Value: k.staged[name], Secret: k.secretNames[name], Source: src}
		if k.secretNames[name] {
			put.Origin = secretOrigin
		}
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
	clear(k.deleted)
	k.order, k.deletedOrder = nil, nil
	return saved, failed, deleted
}

// mask removes the secret values the scripts wrote from text bound for the agent.
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
// script or the agent's request. Without it a temporary value holding "{{token}}" would be resolved as a secret
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
			return "prepare script returned a {{...}} reference that is neither in its source nor in your request; references built from temporary values are not resolved. Nothing was sent."
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
