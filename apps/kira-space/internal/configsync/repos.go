package configsync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/credshape"
	"github.com/kirathecat/kira-studio/internal/gitremote"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

const reposModule = "repos"

// RepoTarget is a registered code repository a shared repos file can apply to.
type RepoTarget struct {
	RepoID string
	// File is the repo's local settings file; "" when it has none.
	File string
	// LocalProblem: the local file does not load, so nothing merges into it.
	LocalProblem bool
}

// RepoFiles is the repocfg side of the repos module. configsync never imports repocfg.
type RepoFiles interface {
	// Targets lists the registered repositories, hidden ones included.
	Targets() []RepoTarget
	// CheckShared is the first problem of a shared repos file, "" when clean.
	CheckShared(data []byte) string
	// CheckLocal is the first problem of a merged local repo file, "" when clean.
	CheckLocal(file string, data []byte) string
	// NewFile names the file of a repository without one and gives its starting text. taken lists
	// names an earlier new file of the same plan already holds.
	NewFile(repoID string, taken map[string]bool) (file string, text []byte, err error)
	// SharedText is the shared file of repoID under origin: origin plus the shared keys of its file.
	SharedText(repoID, origin string) ([]byte, error)
	// Reload re-reads the repo files synchronously, after an apply wrote some.
	Reload()
}

// repoEntries splits a repos file, shared or local, into the entries a sync merges: the prepare
// keys as one unit (they are exclusive), the prepare timeout, the integration branches, each
// environment by name and the review base candidates. Every other key is ignored.
func repoEntries(text []byte) (entrySet, error) {
	set := newEntrySet()
	if len(bytes.TrimSpace(text)) == 0 {
		return set, nil
	}
	var doc any
	if err := yaml.Unmarshal(text, &doc); err != nil {
		return set, err
	}
	if doc == nil {
		return set, nil
	}
	top, ok := doc.(map[string]any)
	if !ok {
		return set, errNotMap
	}
	if err := putTopEntries(&set, top); err != nil {
		return set, err
	}
	return set, nil
}

// putTopEntries puts the entries of a decoded repos file's top-level mapping into set.
func putTopEntries(set *entrySet, top map[string]any) error {
	if w, ok := top["worktree"].(map[string]any); ok {
		if err := putWorktreeEntries(set, w); err != nil {
			return err
		}
	}
	if v, has := top["integration_branches"]; has {
		if err := set.put("integration_branches", v); err != nil {
			return err
		}
	}
	if r, ok := top["review"].(map[string]any); ok {
		if v, has := r["base_candidates"]; has {
			if err := set.put("review/base_candidates", v); err != nil {
				return err
			}
		}
	}
	if envs, ok := top["environments"].([]any); ok {
		return putEnvironmentEntries(set, envs)
	}
	return nil
}

// putWorktreeEntries puts the prepare keys as one unit and the prepare timeout.
func putWorktreeEntries(set *entrySet, w map[string]any) error {
	prep := map[string]any{}
	for _, k := range []string{"prepare_script", "prepare_run"} {
		if v, has := w[k]; has {
			prep[k] = v
		}
	}
	if len(prep) > 0 {
		if err := set.put("worktree/prepare", prep); err != nil {
			return err
		}
	}
	if v, has := w["prepare_timeout"]; has {
		return set.put("worktree/prepare_timeout", v)
	}
	return nil
}

// putEnvironmentEntries puts each named environment and records their order.
func putEnvironmentEntries(set *entrySet, envs []any) error {
	for _, e := range envs {
		m, isMap := e.(map[string]any)
		name, _ := m["name"].(string)
		if !isMap || name == "" {
			continue
		}
		if err := set.put("environments/"+name, m); err != nil {
			return err
		}
		set.order = append(set.order, name)
	}
	return nil
}

// applyRepoEntries returns the repo file text with the take ids set to their shared value, or removed
// when shared lacks them. Local keys, comments and key order outside the edited values stay.
func applyRepoEntries(src []byte, theirs entrySet, take []string) ([]byte, error) {
	var sets []configfs.NodeSet
	setOrUnset := func(id string, path ...string) {
		v, ok := theirs.raw[id]
		sets = append(sets, configfs.NodeSet{Path: path, Value: ordered(v), Unset: !ok})
	}
	envTake := map[string]bool{}
	for _, id := range take {
		switch {
		case id == "worktree/prepare":
			prep, _ := theirs.raw[id].(map[string]any)
			for _, k := range []string{"prepare_script", "prepare_run"} {
				v, ok := prep[k]
				sets = append(sets, configfs.NodeSet{Path: []string{"worktree", k}, Value: ordered(v), Unset: !ok})
			}
		case id == "worktree/prepare_timeout":
			setOrUnset(id, "worktree", "prepare_timeout")
		case id == "integration_branches":
			setOrUnset(id, "integration_branches")
		case id == "review/base_candidates":
			setOrUnset(id, "review", "base_candidates")
		case strings.HasPrefix(id, "environments/"):
			envTake[strings.TrimPrefix(id, "environments/")] = true
		}
	}
	if len(envTake) > 0 {
		list := mergeEnvironments(src, theirs, envTake)
		sets = append(sets, configfs.NodeSet{Path: []string{"environments"}, Value: list, Unset: len(list) == 0})
	}
	return configfs.EditYAML(src, sets)
}

// mergeEnvironments is the local environment list with the take names replaced by their shared
// value or dropped, and new shared names appended in shared order.
func mergeEnvironments(src []byte, theirs entrySet, take map[string]bool) []any {
	var doc struct {
		Environments []map[string]any `yaml:"environments"`
	}
	_ = yaml.Unmarshal(src, &doc)
	out := []any{}
	local := map[string]bool{}
	for _, e := range doc.Environments {
		name, _ := e["name"].(string)
		local[name] = true
		if !take[name] {
			out = append(out, ordered(e))
			continue
		}
		if v, ok := theirs.raw["environments/"+name]; ok {
			out = append(out, ordered(v))
		}
	}
	for _, name := range theirs.order {
		if v, ok := theirs.raw["environments/"+name]; ok && take[name] && !local[name] {
			out = append(out, ordered(v))
		}
	}
	return out
}

// originState is what a repository's origin remote says: its distinct normalized keys, and whether
// it is a usable single origin (every URL gives the same key).
type originState struct {
	keys   []string
	single bool
}

// originsOf reads the URLs of the origin remote of repoID. A repository with no origin, a git error
// or a URL that does not normalize has no keys. Raw URLs never leave this function.
func (s *Service) originsOf(ctx context.Context, repoID string) (originState, error) {
	res, e := s.mirror.run(ctx, repoID, []string{"remote", "get-url", "--all", "origin"}, nil, true)
	if e != nil {
		if ctx.Err() != nil {
			return originState{}, ctx.Err()
		}
		return originState{}, nil
	}
	var keys []string
	bad := false
	lines := 0
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines++
		k, ok := gitremote.Key(line)
		if !ok {
			bad = true
			continue
		}
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return originState{keys: keys, single: lines > 0 && !bad && len(keys) == 1}, nil
}

// repoBaseName is the merge base file of one target repository under a shared file's base dir. It
// hashes the repository path, so it survives a file rename and exists before the file does.
func repoBaseName(repoID string) string {
	h := sha256.Sum256([]byte(repoID))
	return hex.EncodeToString(h[:])[:16] + ".entries.json"
}

// repoBaseDirs lists the shared files that have a stored base.
func (s *Service) repoBaseDirs() []string {
	ents, err := os.ReadDir(s.baseAt(reposModule))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func (s *Service) readRepoOrigin(shared string) string {
	b, err := os.ReadFile(s.baseAt(filepath.Join(reposModule, shared, "origin")))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// sharedOriginKey is the normalized origin of a shared repos file that passed the gate.
func sharedOriginKey(data []byte) string {
	var f struct {
		Origin string `yaml:"origin"`
	}
	if yaml.Unmarshal(data, &f) != nil {
		return ""
	}
	key, _ := gitremote.Key(f.Origin)
	return key
}

// repoFingerprint identifies the repo set a plan was built for: each repository, its file and its origin.
func repoFingerprint(parts []string) string {
	sort.Strings(parts)
	h := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(h[:])
}

type repoTargets struct {
	single  map[string][]RepoTarget
	several map[string][]RepoTarget
	// fp fingerprints the target set.
	fp string
}

// repoTargets reads the origin of every registered repository.
func (s *Service) repoTargets(ctx context.Context) (repoTargets, error) {
	rt := repoTargets{single: map[string][]RepoTarget{}, several: map[string][]RepoTarget{}}
	if s.o.Repos == nil {
		return rt, nil
	}
	targets := s.o.Repos.Targets()
	sort.Slice(targets, func(i, j int) bool { return targets[i].RepoID < targets[j].RepoID })
	var parts []string
	for _, tg := range targets {
		st, err := s.originsOf(ctx, tg.RepoID)
		if err != nil {
			return rt, err
		}
		parts = append(parts, tg.RepoID+"\x00"+tg.File+"\x00"+strings.Join(st.keys, ",")+"\x00"+boolStr(st.single)+boolStr(tg.LocalProblem))
		switch {
		case len(st.keys) == 0:
		case st.single:
			rt.single[st.keys[0]] = append(rt.single[st.keys[0]], tg)
		default:
			for _, k := range st.keys {
				rt.several[k] = append(rt.several[k], tg)
			}
		}
	}
	rt.fp = repoFingerprint(parts)
	return rt, nil
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// sharedRepoFile is one shared repos file as the plan sees it.
type sharedRepoFile struct {
	data    []byte
	ok      bool
	key     string
	problem string
	// dup names the earlier file that already holds this origin.
	dup string
}

// planRepos plans the repos module: every shared file against every registered repository whose
// origin remote gives the file's origin key, entry by entry.
func (s *Service) planRepos(ctx context.Context, p *plan, t *sharedTree, set Settings) error {
	p.repoOwned, p.repoGone, p.repoOrigin = map[string]bool{}, map[string]bool{}, map[string]string{}
	if s.o.Repos == nil {
		return nil
	}
	rt, err := s.repoTargets(ctx)
	if err != nil {
		return err
	}
	p.repoFP = rt.fp
	sorted := s.sharedRepoNames(t)
	files, claimed := s.collectSharedRepoFiles(t, sorted)
	deact := set.deactivated(reposModule)
	taken := map[string]bool{}
	for _, name := range sorted {
		sh := files[name]
		if row, blocked := sharedFileBlock(s, name, sh); blocked {
			p.files = append(p.files, row)
			continue
		}
		p.repoOwned[name] = true
		if sh.ok {
			p.repoOrigin[name] = sh.key
		} else {
			p.repoGone[name] = true
			if _, live := claimed[sh.key]; live || sh.key == "" {
				continue
			}
		}
		p.files = append(p.files, s.originConflictRows(name, sh, rt)...)
		matched := rt.single[sh.key]
		if sh.ok && len(matched) == 0 && len(rt.several[sh.key]) == 0 {
			p.files = append(p.files, s.repoRowFile(name, Row{Kind: KindUnmatched, Origin: sh.key}))
		}
		for _, tg := range matched {
			fp, err := s.planRepoTarget(name, sh, tg, deact, taken)
			if err != nil {
				return err
			}
			p.files = append(p.files, fp)
		}
	}
	return nil
}

// sharedRepoNames lists, sorted, the shared repos files of the tree and of the repo base dirs.
func (s *Service) sharedRepoNames(t *sharedTree) []string {
	names := map[string]bool{}
	for f := range t.files[reposModule] {
		names[f] = true
	}
	for _, f := range s.repoBaseDirs() {
		if syncableFile(reposModule, f) {
			names[f] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for f := range names {
		sorted = append(sorted, f)
	}
	sort.Strings(sorted)
	return sorted
}

// collectSharedRepoFiles reads each named shared file; claimed maps an origin key to the first
// file that holds it.
func (s *Service) collectSharedRepoFiles(t *sharedTree, sorted []string) (files map[string]*sharedRepoFile, claimed map[string]string) {
	claimed = map[string]string{}
	files = map[string]*sharedRepoFile{}
	for _, name := range sorted {
		data, ok := t.files[reposModule][name]
		sh := &sharedRepoFile{data: data, ok: ok}
		files[name] = sh
		if !ok {
			sh.key = s.readRepoOrigin(name)
			continue
		}
		if sh.problem = s.sharedProblem(reposModule, name, data); sh.problem != "" {
			continue
		}
		sh.key = sharedOriginKey(data)
		if first, dup := claimed[sh.key]; dup {
			sh.dup = first
			continue
		}
		claimed[sh.key] = name
	}
	return files, claimed
}

// sharedFileBlock is the blocked row of a shared file with a problem or a duplicate origin.
func sharedFileBlock(s *Service, name string, sh *sharedRepoFile) (*filePlan, bool) {
	switch {
	case sh.problem != "":
		return s.repoRowFile(name, Row{Kind: KindBlocked, Problem: sh.problem}), true
	case sh.dup != "":
		msg := i18n.T("backend.space.configSync.gate.duplicateOrigin.body", map[string]string{"file": sh.dup})
		return s.repoRowFile(name, Row{Kind: KindBlocked, Problem: msg, Origin: sh.key}), true
	}
	return nil, false
}

// originConflictRows blocks a live shared file once per target whose origin several repos share.
func (s *Service) originConflictRows(name string, sh *sharedRepoFile, rt repoTargets) []*filePlan {
	if !sh.ok {
		return nil
	}
	var rows []*filePlan
	for _, tg := range rt.several[sh.key] {
		msg := i18n.T("backend.space.configSync.gate.repoOrigins.body", nil)
		rows = append(rows, s.repoRowFile(name, Row{Kind: KindBlocked, Problem: msg, Origin: sh.key, Target: tg.File, Repo: tg.RepoID}))
	}
	return rows
}

// repoRowFile is a file plan that holds one row and writes nothing.
func (s *Service) repoRowFile(shared string, r Row) *filePlan {
	r.Module, r.File = reposModule, shared
	r.ID = reposModule + "/" + shared
	if r.Repo != "" {
		r.ID += "@" + rowTargetID(r.Target, r.Repo)
	}
	return &filePlan{module: reposModule, shared: shared, entry: true, keepBase: true, rows: []*rowPlan{{Row: r}}}
}

// planRepoTarget plans one shared file against one target repository.
func (s *Service) planRepoTarget(shared string, sh *sharedRepoFile, tg RepoTarget, deact map[string]bool, taken map[string]bool) (*filePlan, error) {
	fp := &filePlan{module: reposModule, shared: shared, file: tg.File, repoID: tg.RepoID, origin: sh.key, entry: true, theirs: sh.data, theirsOK: sh.ok}
	fp.baseRel = filepath.Join(reposModule, shared, repoBaseName(tg.RepoID))
	if tg.File != "" {
		ours, ok, err := readLocal(s.localPath(reposModule, tg.File))
		if err != nil {
			return nil, err
		}
		fp.ours, fp.oursOK = ours, ok
	} else {
		fp.alloc = func() error {
			file, text, err := s.o.Repos.NewFile(tg.RepoID, taken)
			if err != nil {
				return err
			}
			fp.file, fp.seed = file, text
			taken[file] = true
			return nil
		}
	}
	fp.oursHash = configfs.TextHash(fp.ours)
	if tg.LocalProblem {
		fp.keepBase = true
		fp.rows = []*rowPlan{fp.newRow(&rowPlan{Row: Row{Kind: KindBlocked, Problem: i18n.T("backend.space.configSync.gate.localInvalid.body", nil)}}, "")}
		return fp, nil
	}
	if err := s.planEntryFile(fp, deact); err != nil {
		return nil, err
	}
	if fp.allocErr != nil {
		return nil, fp.allocErr
	}
	return fp, nil
}

// stageRepoBase settles the repos part of base.next after the unit bases are written: bases of
// targets no plan names any more go, a shared dir of a file deleted upstream goes once empty, and
// a live shared file records its origin key so a later deletion still finds its targets.
func (s *Service) stageRepoBase(p *plan, next string) error {
	keep := map[string]bool{}
	for _, fp := range p.files {
		if fp.module == reposModule && fp.baseRel != "" {
			keep[filepath.ToSlash(fp.baseRel)] = true
		}
	}
	for name := range p.repoOwned {
		dir := filepath.Join(next, reposModule, name)
		left, err := pruneRepoBaseDir(dir, name, keep)
		if err != nil {
			return err
		}
		if p.repoGone[name] && left == 0 {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			continue
		}
		if key := p.repoOrigin[name]; key != "" {
			if err := configfs.WriteAtomic(filepath.Join(dir, "origin"), []byte(key+"\n"), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// pruneRepoBaseDir removes the base files of dir no plan keeps and counts those left.
func pruneRepoBaseDir(dir, name string, keep map[string]bool) (left int, err error) {
	ents, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	for _, e := range ents {
		if e.IsDir() || e.Name() == "origin" {
			continue
		}
		if !keep[reposModule+"/"+name+"/"+e.Name()] {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return left, err
			}
			continue
		}
		left++
	}
	return left, nil
}

// scriptCredentialProblem finds a credential embedded in a word of a string value, such as a token
// pasted into a script. credentialProblem only sees values that are a credential on their own. Pure
// hex words pass: commit ids are common in scripts. The message names the line and field.
func scriptCredentialProblem(text []byte) string {
	var doc yaml.Node
	if yaml.Unmarshal(text, &doc) != nil {
		return ""
	}
	if line, path, hit := scanWords(&doc, nil); hit {
		return literalCredentialMessage(line, path)
	}
	return ""
}

func scanWords(n *yaml.Node, path []string) (line int, at []string, hit bool) {
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode, yaml.MappingNode:
		for i, c := range n.Content {
			child := path
			switch {
			case n.Kind == yaml.MappingNode && i%2 == 0:
				continue
			case n.Kind == yaml.MappingNode:
				child = append(append([]string(nil), path...), n.Content[i-1].Value)
			case n.Kind == yaml.SequenceNode:
				child = append(append([]string(nil), path...), strconv.Itoa(i))
			}
			if l, p, h := scanWords(c, child); h {
				return l, p, true
			}
		}
	case yaml.ScalarNode:
		if n.Tag == "!!str" && wordsHoldCredential(n.Value) {
			return n.Line, path, true
		}
	}
	return 0, nil, false
}

func wordsHoldCredential(s string) bool {
	for _, w := range strings.Fields(s) {
		if strings.Contains(w, "${") {
			continue
		}
		if uriPassword.MatchString(strings.Trim(w, "\"'")) {
			return true
		}
		for _, p := range strings.FieldsFunc(w, func(r rune) bool { return strings.ContainsRune("\"'=:;,()[]{}<>|&", r) }) {
			if p != "" && !isHex(p) && credshape.Value(p) {
				return true
			}
		}
	}
	return false
}

// keyMap renders a decoded mapping with name and script first, then the other keys sorted, so an
// entry written into a local file reads like one a person wrote.
type keyMap map[string]any

func (m keyMap) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	rank := func(k string) int {
		switch k {
		case "name":
			return 0
		case "script":
			return 1
		}
		return 2
	}
	sort.Slice(keys, func(i, j int) bool {
		if ri, rj := rank(keys[i]), rank(keys[j]); ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(m[k])
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// ordered wraps every mapping inside v in a keyMap.
func ordered(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(keyMap, len(x))
		for k, e := range x {
			out[k] = ordered(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = ordered(e)
		}
		return out
	}
	return v
}
