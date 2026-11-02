package agentport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentctx"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

var (
	secretNameChars = regexp.MustCompile(`[^a-z0-9_.-]`)
	serverNameRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	envNameRe       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	atImport        = regexp.MustCompile(`(?m)^\s*@\S+`)
)

// Source is one native file an import read, or tried to.
type Source struct {
	Role    string `json:"role"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Problem string `json:"problem"`
}

// ImportPreview is everything an import would do, for the user to select from.
type ImportPreview struct {
	Token    string   `json:"token"`
	Provider string   `json:"provider"`
	Sources  []Source `json:"sources"`
	Items    []Item   `json:"items"`
	Notes    []string `json:"notes"`
}

type importSession struct {
	items        map[string]*Item
	settingsHash string
	contextHash  string
}

// importer builds the items of one import against the current settings.
type importer struct {
	cur        map[string]any
	curSet     agentcfg.Settings
	ctxText    string
	saved      map[string]string
	unreadable map[string]bool
	reserved   map[string]string
	refIn      func(string) (string, bool)
	// envRefs maps each variable an imported value names to the settings fields that use it; nil
	// when the provider needs no probe.
	envRefs    map[string][]string
	hookNames  map[string]bool
	claudeEnum []string

	items   []*Item
	sources []Source
	notes   []string
	source  string
}

func (s *Service) newImporter(refIn func(string) (string, bool)) (*importer, *importSession, error) {
	if s.o.Settings == nil || s.o.Context == nil {
		return nil, nil, ipcerr.Internal(i18n.T("backend.core.agentSettings.unavailable.body", nil))
	}
	sv, cv := s.o.Settings.View(), s.o.Context.View()
	if sv.Problem != nil || !sv.Editable {
		return nil, nil, ipcerr.BadRequest(i18n.T("backend.core.agentPort.fixSettings.body", nil))
	}
	if cv.Problem != nil || !cv.Editable {
		return nil, nil, ipcerr.BadRequest(i18n.T("backend.core.agentPort.fixContext.body", nil))
	}
	im := &importer{
		saved: map[string]string{}, unreadable: map[string]bool{}, reserved: map[string]string{},
		hookNames: map[string]bool{}, ctxText: cv.Text,
	}
	im.refIn = func(v string) (string, bool) {
		if refIn != nil {
			var ok bool
			if v, ok = refIn(v); !ok {
				return "", false
			}
		}
		return im.mapSecretRefs(v), true
	}
	im.curSet, _ = s.o.Settings.Settings()
	b, err := json.Marshal(im.curSet)
	if err != nil {
		return nil, nil, ipcerr.InternalErr(err)
	}
	if err := json.Unmarshal(b, &im.cur); err != nil || im.cur == nil {
		im.cur = map[string]any{}
	}
	if s.o.Secrets != nil {
		vals, bad, err := s.o.Secrets.Store.Open(secrets.ScopeAgentCredential, agentcfg.SecretOwner)
		if err != nil {
			return nil, nil, ipcerr.Wrap(err)
		}
		im.saved = vals
		for n := range bad {
			im.unreadable[n] = true
		}
	}
	return im, &importSession{items: map[string]*Item{}, settingsHash: sv.Hash, contextHash: cv.Hash}, nil
}

// secretEnvName is the shell variable an exported secret travels as.
func secretEnvName(name string) string {
	out := []byte("KIRA_SECRET_")
	for _, c := range []byte(strings.ToUpper(name)) {
		if c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// mapSecretRefs turns ${KIRA_SECRET_X} back into ${secret:x} when a saved secret derives that name.
func (im *importer) mapSecretRefs(v string) string {
	refs, p := configfs.Refs(v)
	if p != nil {
		return v
	}
	for i := len(refs) - 1; i >= 0; i-- {
		r := refs[i]
		if r.Kind != configfs.RefEnv || !strings.HasPrefix(r.Name, "KIRA_SECRET_") {
			continue
		}
		for name := range im.saved {
			if secretEnvName(name) == r.Name {
				v = v[:r.Start] + "${secret:" + name + "}" + v[r.End:]
				break
			}
		}
	}
	return v
}

func sanitizeName(s string) string {
	s = secretNameChars.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.TrimLeft(s, "-_.")
	if len(s) > 56 {
		s = s[:56]
	}
	if s == "" {
		s = "secret"
	}
	return s
}

// assign picks the secret name for value: an equal saved value is reused, a different one under the
// base name takes -2, -3.
func (im *importer) assign(base, value string) (name string, reuse bool) {
	base = sanitizeName(base)
	name = base
	for i := 2; ; i++ {
		if v, ok := im.saved[name]; ok {
			if v == value {
				return name, true
			}
		} else if !im.unreadable[name] {
			if r, ok := im.reserved[name]; !ok {
				im.reserved[name] = value
				return name, false
			} else if r == value {
				return name, false
			}
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

// value translates one native string into settings text: references mapped, credentials sealed.
// reason is "" on success, else the i18n text for why the string cannot be expressed.
func (im *importer) value(l loc, name, raw, base, field string) (text string, slots []SecretSlot, pend []pendingSecret, reason string) {
	txt, ok := im.refIn(raw)
	if !ok {
		return "", nil, nil, i18n.T("backend.core.agentPort.skip.refForm.body", nil)
	}
	if _, p := configfs.Refs(txt); p != nil {
		return "", nil, nil, i18n.T("backend.core.agentPort.skip.refForm.body", nil)
	}
	if im.envRefs != nil {
		refs, _ := configfs.Refs(txt)
		for _, r := range refs {
			if r.Kind == configfs.RefEnv && !slices.Contains(im.envRefs[r.Name], field) {
				im.envRefs[r.Name] = append(im.envRefs[r.Name], field)
			}
		}
	}
	parts := classify(l, name, txt)
	if !hasSecret(parts) {
		return txt, nil, nil, ""
	}
	var b strings.Builder
	for _, p := range parts {
		if !p.secret {
			b.WriteString(p.text)
			continue
		}
		n, reuse := im.assign(base, p.text)
		b.WriteString("${secret:" + n + "}")
		slots = append(slots, SecretSlot{Name: n, Field: field, Reuse: reuse})
		pend = append(pend, pendingSecret{n, p.text})
	}
	return b.String(), slots, pend, ""
}

// add records an item built from edits and resolves its state.
func (im *importer) add(id, kind string, ops []op, slots []SecretSlot, pend []pendingSecret) *Item {
	it := &Item{ID: id, Kind: kind, Source: im.source, Modes: []string{}, Secrets: slots, ops: ops, pending: pend}
	if it.Secrets == nil {
		it.Secrets = []SecretSlot{}
	}
	texts := make([]string, 0, len(ops))
	for _, o := range ops {
		texts = append(texts, snippet(o.path, o.value, o.append))
	}
	it.Text = strings.Join(texts, "\n")
	it.resolveState(im.cur)
	im.items = append(im.items, it)
	return it
}

// skip records an item Kira cannot take, with the reason.
func (im *importer) skip(id, kind, reason string) {
	im.items = append(im.items, &Item{ID: id, Kind: kind, Source: im.source, State: StateSkipped, Reason: reason, Modes: []string{}, Secrets: []SecretSlot{}})
}

func (im *importer) skipKey(key, kind, reason string) { im.skip("skip/"+key, kind, reason) }

// hookName is a new unique hook name for event.
func (im *importer) hookName(prefix, event string) string {
	base := sanitizeName(event)
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s-%d", base, i)
		if !im.hookNames[prefix+n] && !im.curHasHook(prefix, n) {
			im.hookNames[prefix+n] = true
			return n
		}
	}
}

func (im *importer) curHasHook(prefix, name string) bool {
	if prefix == "" {
		_, ok := im.curSet.Hooks[name]
		return ok
	}
	for n := range im.sectionHooks(prefix) {
		if n == name {
			return true
		}
	}
	return false
}

type hookRow struct {
	Event   string `json:"event"`
	Matcher string `json:"matcher"`
	Command string `json:"command"`
}

// sectionHooks are the provider-only hooks of the current settings; prefix is the provider id.
func (im *importer) sectionHooks(provider string) map[string]hookRow {
	var sec struct {
		Hooks map[string]hookRow `json:"hooks"`
	}
	if raw := im.curSet.Sections[provider]; raw != nil {
		_ = json.Unmarshal(raw, &sec)
	}
	return sec.Hooks
}

// sameHook reports whether the current settings hold an equal hook under any name.
func (im *importer) sameHook(provider, event, matcher, command string) bool {
	for _, h := range im.curSet.Hooks {
		if k, ok := agentcfg.HookEventKind(h.Event); ok && string(k) == event && h.Matcher == matcher && h.Command == command {
			return true
		}
	}
	for _, h := range im.sectionHooks(provider) {
		if h.Event == event && h.Matcher == matcher && h.Command == command {
			return true
		}
	}
	return false
}

// importContext adds the context item for the native context file at t.
func (im *importer) importContext(t target, display string, src *Source) {
	data, _, exists, err := readIfExists(t.Path, maxContextFile)
	src.Exists = exists
	if err != nil {
		src.Problem = readProblem(t.Path, err, maxContextFile)
		return
	}
	text := strings.TrimSpace(string(data))
	if !exists || text == "" {
		return
	}
	im.source = display
	if len(text) > agentctx.MaxBytes {
		im.skip("context", "context", i18n.T("backend.core.agentPort.skip.contextTooLarge.body", nil))
		return
	}
	it := &Item{ID: "context", Kind: "context", Source: display, Text: text, Modes: []string{}, Secrets: []SecretSlot{}, ctxText: text}
	switch cur := strings.TrimSpace(im.ctxText); {
	case cur == "":
		it.State = StateNew
	case cur == text:
		it.State = StateSame
	default:
		it.State = StateConflict
		it.Current = cur
		it.Modes = []string{"replace", "append"}
	}
	im.items = append(im.items, it)
	if atImport.MatchString(text) {
		im.notes = append(im.notes, i18n.T("backend.core.agentPort.note.atImports.body", nil))
	}
}

func readProblem(path string, err error, limit int64) string {
	name := baseName(path)
	if errors.Is(err, errTooLarge) {
		return i18n.T("backend.core.agentPort.tooLarge.body", map[string]string{"file": name, "limit": fmt.Sprint(limit >> 20)})
	}
	return i18n.T("backend.core.agentPort.unreadable.body", map[string]string{"file": name})
}

func baseName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// finish turns the built items into the preview and the apply session.
func (s *Service) finishImport(provider string, im *importer, sess *importSession) ImportPreview {
	for _, it := range im.items {
		sess.items[it.ID] = it
	}
	out := ImportPreview{Provider: provider, Sources: im.sources, Items: make([]Item, 0, len(im.items)), Notes: im.notes}
	if out.Sources == nil {
		out.Sources = []Source{}
	}
	if out.Notes == nil {
		out.Notes = []string{}
	}
	for _, it := range im.items {
		out.Items = append(out.Items, *it)
	}
	out.Token = s.sessions.put(&session{imp: sess})
	return out
}

// ImportPreviewArgs names the provider to read.
type ImportPreviewArgs struct {
	Provider string `json:"provider"`
}

// ImportPreview reads the provider's native user files and lists what an import would bring in. It
// writes nothing.
func (s *Service) ImportPreview(ctx context.Context, args ImportPreviewArgs) (ImportPreview, error) {
	files, _, ok := s.targets(ctx, args.Provider)
	if !ok || !s.has(args.Provider) {
		return ImportPreview{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.unknownProvider.body", map[string]string{"provider": args.Provider}))
	}
	var (
		im   *importer
		sess *importSession
		err  error
	)
	switch args.Provider {
	case providerClaude:
		im, sess, err = s.newImporter(nil)
		if err == nil {
			s.importClaude(im, files)
		}
	default:
		im, sess, err = s.newImporter(openCodeRefIn)
		if err == nil {
			im.envRefs = map[string][]string{}
			s.importOpenCode(im, files)
			s.noteUnsetEnv(ctx, im)
		}
	}
	if err != nil {
		return ImportPreview{}, err
	}
	return s.finishImport(args.Provider, im, sess), nil
}

// noteUnsetEnv adds a note for each variable the imported values name that the login shell lacks:
// OpenCode reads it as empty, but Kira fails every agent launch that resolves it.
func (s *Service) noteUnsetEnv(ctx context.Context, im *importer) {
	if len(im.envRefs) == 0 {
		return
	}
	names := sortedKeys(im.envRefs)
	got := s.env(ctx, names...)
	for _, n := range names {
		if _, ok := got[n]; !ok {
			im.notes = append(im.notes, i18n.T("backend.core.agentPort.note.envUnset.body", map[string]string{"name": n, "fields": strings.Join(im.envRefs[n], ", ")}))
		}
	}
}

// has reports whether the provider is registered.
func (s *Service) has(provider string) bool {
	if s.o.Providers == nil {
		return false
	}
	_, ok := s.o.Providers.Get(provider)
	return ok
}

// ImportSelection is one item the user ticked; Mode applies to a conflicting context.
type ImportSelection struct {
	ID   string `json:"id"`
	Mode string `json:"mode"`
}

// ImportApplyArgs applies a preview's selection.
type ImportApplyArgs struct {
	Token string            `json:"token"`
	Items []ImportSelection `json:"items"`
}

// PortResult is what an apply wrote.
type PortResult struct {
	Written []string       `json:"written"`
	Backups []string       `json:"backups"`
	Servers []ServerResult `json:"servers"`
	Secrets []string       `json:"secrets"`
	Notes   []string       `json:"notes"`
}

// ServerResult is the outcome of one MCP server export.
type ServerResult struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
}

func expired() error {
	return ipcerr.T(ipcerr.CodeBadRequest, "backend.core.agentPort.expired.body", nil)
}

func stale() error {
	return ipcerr.T(ipcerr.CodeConflict, "backend.core.agentPort.stale.body", nil)
}

// ImportApply writes the selected items: secrets first, then settings.yaml and context.md through
// their stores. A file changed since the preview is a conflict and nothing is written.
func (s *Service) ImportApply(_ context.Context, args ImportApplyArgs) (PortResult, error) {
	sess, ok := s.sessions.take(args.Token)
	if !ok || sess.imp == nil {
		return PortResult{}, expired()
	}
	imp := sess.imp
	res := PortResult{Written: []string{}, Backups: []string{}, Servers: []ServerResult{}, Secrets: []string{}, Notes: []string{}}
	var chosen []*Item
	var ctxItem *Item
	var ctxMode string
	for _, sel := range args.Items {
		it, ok := imp.items[sel.ID]
		if !ok || it.State == StateSkipped || it.State == StateSame {
			return PortResult{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.badItem.body", nil))
		}
		if len(it.Modes) > 0 && !slices.Contains(it.Modes, sel.Mode) {
			return PortResult{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.badItem.body", nil))
		}
		if it.Kind == "context" {
			ctxItem, ctxMode = it, sel.Mode
			continue
		}
		chosen = append(chosen, it)
	}
	sv, cv := s.o.Settings.View(), s.o.Context.View()
	if sv.Hash != imp.settingsHash || cv.Hash != imp.contextHash {
		return PortResult{}, stale()
	}
	newCtx, err := contextText(ctxItem, ctxMode, cv.Text)
	if err != nil {
		return PortResult{}, err
	}
	var text []byte
	if len(chosen) > 0 {
		if text, err = s.editSettings([]byte(sv.Text), chosen); err != nil {
			return PortResult{}, err
		}
		if err := s.putSecrets(chosen, &res); err != nil {
			return PortResult{}, err
		}
		if _, err := s.o.Settings.Save(string(text), sv.Hash); err != nil {
			// Names sealed here are new (putSecrets refuses an existing one), so no other file refers to them.
			for _, n := range res.Secrets {
				_ = s.o.Secrets.Delete(n)
			}
			return PortResult{}, saveError(err)
		}
		res.Written = append(res.Written, sv.Path)
	}
	if ctxItem != nil {
		if _, err := s.o.Context.Save(newCtx, cv.Hash); err != nil {
			return PortResult{}, saveError(err)
		}
		res.Written = append(res.Written, cv.Path)
	}
	return res, nil
}

// contextText is the new context.md text for the chosen context item; "" when none is chosen.
func contextText(it *Item, mode, cur string) (string, error) {
	if it == nil {
		return "", nil
	}
	text := it.ctxText + "\n"
	if mode == "append" && strings.TrimSpace(cur) != "" {
		text = strings.TrimRight(cur, "\n") + "\n\n" + text
	}
	if len(text) > agentctx.MaxBytes {
		return "", ipcerr.BadRequest(i18n.T("backend.core.agentPort.skip.contextTooLarge.body", nil))
	}
	return text, nil
}

// editSettings applies the chosen items' edits to the settings.yaml text and checks the result.
func (s *Service) editSettings(text []byte, chosen []*Item) ([]byte, error) {
	ed, err := newEditor(text)
	if err != nil {
		return nil, ipcerr.InternalErr(err)
	}
	for _, it := range chosen {
		for _, o := range it.ops {
			if err := ed.apply(o); err != nil {
				return nil, ipcerr.InternalErr(err)
			}
		}
	}
	out, err := ed.bytes()
	if err != nil {
		return nil, ipcerr.InternalErr(err)
	}
	if p := s.o.Settings.Check(out); p != nil {
		return nil, ipcerr.InternalErr(fmt.Errorf("agentport: edited settings.yaml is invalid: %s", p.Message))
	}
	return out, nil
}

// putSecrets seals the chosen items' new credentials, each name once. A name saved since the preview
// is a conflict, never overwritten; on any error it removes what it sealed.
func (s *Service) putSecrets(chosen []*Item, res *PortResult) (err error) {
	defer func() {
		if err != nil {
			for _, n := range res.Secrets {
				_ = s.o.Secrets.Delete(n)
			}
			res.Secrets = []string{}
		}
	}()
	var have map[string]string
	var unreadable map[string]error
	done := map[string]bool{}
	for _, it := range chosen {
		reuse := map[string]bool{}
		for _, sl := range it.Secrets {
			reuse[sl.Name] = sl.Reuse
		}
		for _, p := range it.pending {
			if done[p.name] || reuse[p.name] {
				continue
			}
			if s.o.Secrets == nil {
				return ipcerr.SecretStore(i18n.T("backend.core.agentSettings.noStore.body", nil))
			}
			if have == nil {
				if have, unreadable, err = s.o.Secrets.Store.Open(secrets.ScopeAgentCredential, agentcfg.SecretOwner); err != nil {
					return ipcerr.Wrap(err)
				}
			}
			if _, bad := unreadable[p.name]; bad {
				return stale()
			}
			if v, taken := have[p.name]; taken {
				if v != p.value {
					return stale()
				}
				done[p.name] = true
				continue
			}
			if err := s.o.Secrets.Put(p.name, p.value); err != nil {
				return ipcerr.Wrap(err)
			}
			done[p.name] = true
			res.Secrets = append(res.Secrets, p.name)
		}
	}
	slices.Sort(res.Secrets)
	return nil
}

func saveError(err error) error {
	if errors.Is(err, configfs.ErrConflict) {
		return stale()
	}
	return ipcerr.InternalErr(err)
}

// CancelArgs names the preview to drop.
type CancelArgs struct {
	Token string `json:"token"`
}

// Cancel drops a preview.
func (s *Service) Cancel(args CancelArgs) { s.sessions.drop(args.Token) }
