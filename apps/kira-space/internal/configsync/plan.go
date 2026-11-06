package configsync

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Row kinds.
const (
	KindAdd       = "add"
	KindModify    = "modify"
	KindDelete    = "delete"
	KindConflict  = "conflict"
	KindBlocked   = "blocked"
	KindUnchanged = "unchanged"
)

// Choices for a conflicted unit.
const (
	ChoiceMine   = "mine"
	ChoiceShared = "shared"
)

// Row is one unit in the review: a whole file, or one entry of a combined file.
type Row struct {
	ID     string `json:"id"`
	Module string `json:"module"`
	File   string `json:"file"`
	// Entry is the entry id inside File; "" for a file unit.
	Entry string `json:"entry"`
	Kind  string `json:"kind"`
	// Deactivated: this machine never applies the unit.
	Deactivated  bool `json:"deactivated"`
	RunsCommands bool `json:"runsCommands"`
	// Problem is why a blocked unit is not offered; it never echoes a value.
	Problem string `json:"problem"`
	// UnsetSecrets are the ${secret:name} references the unit holds that no local secret answers.
	UnsetSecrets []string `json:"unsetSecrets"`
}

func (r Row) actionable() bool {
	switch r.Kind {
	case KindAdd, KindModify, KindDelete, KindConflict:
		return true
	}
	return false
}

// FileDiff is a changed file's text now and after a pull with every conflict kept local.
type FileDiff struct {
	Module        string `json:"module"`
	File          string `json:"file"`
	Current       string `json:"current"`
	After         string `json:"after"`
	CurrentExists bool   `json:"currentExists"`
	AfterExists   bool   `json:"afterExists"`
}

type rowPlan struct {
	Row
	verdict verdict
	// theirsOK: a take writes shared content, else it removes the unit.
	theirsOK bool
}

type filePlan struct {
	module, file string
	entry        bool
	ours         []byte
	oursOK       bool
	oursHash     string
	rows         []*rowPlan
	// file unit
	theirs   []byte
	theirsOK bool
	// entry unit
	oursSet, theirsSet entrySet
	baseCanon          map[string]string
	blockedIDs         map[string]bool
	// newBase is the merge base after a clean pull of this file; baseKeep leaves the old base as is.
	newBaseFile  []byte
	newBaseExist bool
	mergeProblem string
}

type plan struct {
	commit string
	files  []*filePlan
}

func (p *plan) rows() []Row {
	var out []Row
	for _, f := range p.files {
		for _, r := range f.rows {
			row := r.Row
			if row.UnsetSecrets == nil {
				row.UnsetSecrets = []string{}
			}
			out = append(out, row)
		}
	}
	return out
}

func (p *plan) changes() int {
	n := 0
	for _, r := range p.rows() {
		if r.actionable() {
			n++
		}
	}
	return n
}

var secretRefRe = regexp.MustCompile(`\$\{secret:([A-Za-z0-9._-]+)\}`)

func (s *Service) localPath(module, file string) string {
	return filepath.Join(configfs.ModuleDir(s.o.Home, module), file)
}

func readLocal(path string) ([]byte, bool, error) {
	b, err := configfs.ReadCapped(path, configfs.MaxFileBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return b, err == nil, err
}

func baseName(file string, entry bool) string {
	if entry {
		return file + ".entries.json"
	}
	return file
}

func (s *Service) baseFilePath(module, file string, entry bool) string {
	return filepath.Join(syncDir(s.o.Home), baseDir, module, baseName(file, entry))
}

// baseFiles lists the shared files last applied for module.
func (s *Service) baseFiles(module string) []string {
	ents, err := os.ReadDir(filepath.Join(syncDir(s.o.Home), baseDir, module))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			out = append(out, strings.TrimSuffix(e.Name(), ".entries.json"))
		}
	}
	return out
}

// buildPlan decides every unit of the modules the manifest lists and the settings follow.
func (s *Service) buildPlan(t *sharedTree, set Settings) (*plan, error) {
	p := &plan{commit: t.commit}
	for _, module := range syncModules {
		if !slices.Contains(t.modules, module) || set.mode(module) == ModeLocal {
			continue
		}
		names := map[string]bool{}
		for f := range t.files[module] {
			names[f] = true
		}
		for _, f := range s.baseFiles(module) {
			if syncableFile(module, f) {
				names[f] = true
			}
		}
		sorted := make([]string, 0, len(names))
		for f := range names {
			sorted = append(sorted, f)
		}
		sort.Strings(sorted)
		for _, file := range sorted {
			fp, err := s.planFile(module, file, set, t.files[module][file])
			if err != nil {
				return nil, err
			}
			p.files = append(p.files, fp)
		}
	}
	return p, nil
}

func (s *Service) planFile(module, file string, set Settings, shared []byte) (*filePlan, error) {
	mode := set.mode(module)
	deact := set.deactivated(module)
	fp := &filePlan{module: module, file: file, entry: isEntryFile(module, file, mode), theirs: shared, theirsOK: shared != nil}
	ours, oursOK, err := readLocal(s.localPath(module, file))
	if err != nil {
		return nil, err
	}
	fp.ours, fp.oursOK, fp.oursHash = ours, oursOK, configfs.TextHash(ours)
	if fp.entry {
		return fp, s.planEntryFile(fp, deact)
	}
	return fp, s.planFileUnit(fp, deact[file])
}

func (s *Service) planFileUnit(fp *filePlan, deactivated bool) error {
	base, baseOK, err := readLocal(s.baseFilePath(fp.module, fp.file, false))
	if err != nil {
		return err
	}
	theirs := effective(side{val: string(fp.theirs), ok: fp.theirsOK}, deactivated)
	v := decide(side{val: string(fp.ours), ok: fp.oursOK}, side{val: string(base), ok: baseOK}, theirs)
	r := &rowPlan{Row: Row{ID: fp.module + "/" + fp.file, Module: fp.module, File: fp.file, Kind: KindUnchanged,
		Deactivated: deactivated && fp.theirsOK, RunsCommands: runsCommands(fp.module)}, verdict: v, theirsOK: theirs.ok}
	fp.rows = []*rowPlan{r}
	fp.newBaseFile, fp.newBaseExist = fp.theirs, theirs.ok
	if !theirs.ok {
		fp.newBaseFile = nil
	}
	if fp.module == "agent" && fp.file == agentSettingsName && fp.theirsOK {
		r.RunsCommands = settingsRunCommands(fp.theirs)
	}
	if v == verdictNone {
		if !fp.theirsOK && !baseOK {
			fp.rows = nil
		}
		return nil
	}
	if theirs.ok {
		if msg := s.sharedProblem(fp.module, fp.file, fp.theirs); msg != "" {
			r.Kind, r.Problem, r.verdict = KindBlocked, msg, verdictNone
			fp.newBaseExist, fp.newBaseFile = baseOK, base
			return nil
		}
		r.UnsetSecrets = s.unsetSecrets(fp.module, string(fp.theirs))
	}
	switch {
	case v == verdictConflict:
		r.Kind = KindConflict
	case !fp.oursOK:
		r.Kind = KindAdd
	case !theirs.ok:
		r.Kind = KindDelete
	default:
		r.Kind = KindModify
	}
	return nil
}

// sharedProblem is the gate every shared file passes before it can be applied: "" when clean.
func (s *Service) sharedProblem(module, file string, data []byte) string {
	md := strings.HasSuffix(file, ".md")
	if !md {
		var probe any
		if err := yaml.Unmarshal(data, &probe); err != nil {
			return i18n.T("backend.space.configSync.gate.invalid.body", map[string]string{"message": firstLine(err.Error())})
		}
	}
	if v := s.o.Validate[module]; v != nil {
		if msg := v(file, data); msg != "" {
			return i18n.T("backend.space.configSync.gate.invalid.body", map[string]string{"message": msg})
		}
	}
	return credentialProblem(data, md)
}

func (s *Service) unsetSecrets(module, text string) []string {
	out := []string{}
	if s.o.SecretUnset == nil {
		return out
	}
	for _, m := range secretRefRe.FindAllStringSubmatch(text, -1) {
		if s.o.SecretUnset(module, m[1]) && !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out
}

func settingsRunCommands(data []byte) bool {
	set, err := agentSettingsEntries(data)
	if err != nil {
		return false
	}
	for id, v := range set.raw {
		if entryRunsCommands("agent", id, v) {
			return true
		}
	}
	return false
}

func entryRunsCommands(module, id string, v any) bool {
	if module != "agent" {
		return false
	}
	if strings.HasPrefix(id, "hooks/") {
		return true
	}
	if strings.HasPrefix(id, "mcp_servers/") {
		m, ok := v.(map[string]any)
		_, cmd := m["command"]
		return ok && cmd
	}
	return false
}

func (s *Service) planEntryFile(fp *filePlan, deact map[string]bool) error {
	parse := agentSettingsEntries
	if fp.module == "prompts" {
		parse = promptEntries
	}
	fileRow := func(msg string) *rowPlan {
		return &rowPlan{Row: Row{ID: fp.module + "/" + fp.file, Module: fp.module, File: fp.file, Kind: KindBlocked, Problem: msg}}
	}
	oursSet, err := parse(fp.ours)
	if err != nil {
		fp.rows = []*rowPlan{fileRow(i18n.T("backend.space.configSync.gate.localInvalid.body", nil))}
		return nil
	}
	theirsSet, err := parse(fp.theirs)
	if err != nil {
		fp.rows = []*rowPlan{fileRow(i18n.T("backend.space.configSync.gate.invalid.body", map[string]string{"message": firstLine(err.Error())}))}
		return nil
	}
	baseData, baseExists, err := readLocal(s.baseFilePath(fp.module, fp.file, true))
	if err != nil {
		return err
	}
	baseCanon, err := decodeBaseEntries(baseData)
	if err != nil {
		return err
	}
	fp.oursSet, fp.theirsSet, fp.baseCanon, fp.blockedIDs = oursSet, theirsSet, baseCanon, map[string]bool{}
	adjusted, blockedMsg := gateEntries(theirsSet, baseCanon)
	deactIDs := map[string]bool{}
	for _, m := range []map[string]string{theirsSet.canon, baseCanon} {
		for id := range m {
			deactIDs[id] = deact[fp.file+"#"+id]
		}
	}
	for id := range blockedMsg {
		fp.blockedIDs[id] = true
	}
	s.entryRows(fp, planEntries(oursSet.canon, baseCanon, adjusted, deactIDs), adjusted, deactIDs, blockedMsg)
	// The merge base after a pull: shared entries as they stand, deactivated ones out, blocked ones as before.
	nb := map[string]string{}
	for id, canon := range adjusted {
		if !deactIDs[id] {
			nb[id] = canon
		}
	}
	enc, err := encodeBaseEntries(nb)
	if err != nil {
		return err
	}
	fp.newBaseFile, fp.newBaseExist = enc, len(nb) > 0 || len(baseCanon) > 0
	if len(fp.rows) == 0 {
		return nil
	}
	if _, _, err := s.merged(fp, nil, s.now()); err != nil {
		fp.mergeProblem = i18n.T("backend.space.configSync.gate.invalid.body", map[string]string{"message": firstLine(err.Error())})
		for _, r := range fp.rows {
			if r.verdict == verdictTake {
				r.Kind, r.Problem, r.verdict = KindBlocked, fp.mergeProblem, verdictNone
			}
		}
		fp.newBaseFile, fp.newBaseExist = baseData, baseExists
	}
	return nil
}

// gateEntries drops shared entries that hold a literal credential: each is replaced by its base
// value (or absent), so it never changes, and its problem is returned by id.
func gateEntries(theirs entrySet, base map[string]string) (adjusted map[string]string, blocked map[string]string) {
	adjusted, blocked = map[string]string{}, map[string]string{}
	for id, canon := range theirs.canon {
		if msg := entryCredentialProblem(theirs.raw[id]); msg != "" {
			blocked[id] = msg
			if b, ok := base[id]; ok {
				adjusted[id] = b
			}
			continue
		}
		adjusted[id] = canon
	}
	return adjusted, blocked
}

// entryRows lists the decided entries, the blocked ones and the deactivated ones, sorted by id.
func (s *Service) entryRows(fp *filePlan, decisions []entryDecision, adjusted map[string]string, deactIDs map[string]bool, blocked map[string]string) {
	seen := map[string]bool{}
	mk := func(id string) *rowPlan {
		seen[id] = true
		_, theirsHas := adjusted[id]
		return &rowPlan{Row: Row{ID: fp.module + "/" + fp.file + "#" + id, Module: fp.module, File: fp.file, Entry: id, Kind: KindUnchanged,
			Deactivated: deactIDs[id], RunsCommands: entryRunsCommands(fp.module, id, fp.theirsSet.raw[id])}, theirsOK: theirsHas && !deactIDs[id]}
	}
	for _, d := range decisions {
		r := mk(d.id)
		r.verdict = d.verdict
		switch {
		case d.verdict == verdictConflict:
			r.Kind = KindConflict
		case !d.ours.ok:
			r.Kind = KindAdd
		case !d.theirs.ok:
			r.Kind = KindDelete
		default:
			r.Kind = KindModify
		}
		if d.theirs.ok {
			r.UnsetSecrets = s.unsetSecrets(fp.module, d.theirs.val)
		}
		fp.rows = append(fp.rows, r)
	}
	for id, msg := range blocked {
		r := mk(id)
		r.Kind, r.Problem = KindBlocked, msg
		fp.rows = append(fp.rows, r)
	}
	for id, off := range deactIDs {
		if _, inTheirs := fp.theirsSet.canon[id]; off && !seen[id] && inTheirs {
			fp.rows = append(fp.rows, mk(id))
		}
	}
	sort.Slice(fp.rows, func(i, j int) bool { return fp.rows[i].ID < fp.rows[j].ID })
}

// merged returns the text a combined file has after the takes in choices (a conflict is taken only
// when its choice is shared); changed is false when nothing applies.
func (s *Service) merged(fp *filePlan, choices map[string]string, now time.Time) (text []byte, changed bool, err error) {
	var take []string
	for _, r := range fp.rows {
		if r.Entry == "" || r.Kind == KindBlocked {
			continue
		}
		if r.verdict == verdictTake || r.verdict == verdictConflict && choices[r.ID] == ChoiceShared {
			take = append(take, r.Entry)
		}
	}
	if len(take) == 0 {
		return fp.ours, false, nil
	}
	if fp.module == "prompts" {
		text, err = applyPromptEntries(fp.ours, fp.oursSet, fp.theirsSet, take, now)
	} else {
		text, err = applyAgentEntries(fp.ours, fp.theirsSet, take)
	}
	if err != nil {
		return nil, false, err
	}
	if msg := s.mergedProblem(fp, text); msg != "" {
		return nil, false, errors.New(msg)
	}
	return text, true, nil
}

// mergedProblem validates a combined file's merged text with its module's own decoder.
func (s *Service) mergedProblem(fp *filePlan, text []byte) string {
	if fp.module == "prompts" {
		texts, err := llmprompts.DecodeOverrides(text)
		if err != nil {
			return firstLine(err.Error())
		}
		return llmprompts.ValidateOverrides(texts)
	}
	if v := s.o.Validate[fp.module]; v != nil {
		return v(fp.file, text)
	}
	return ""
}

// fileResult is what a pull does to one file under choices.
type fileResult struct {
	data    []byte
	remove  bool
	changed bool
	// blocked: the merged text failed validation under these choices; the file stays as it is.
	blocked string
}

func (s *Service) resultFor(fp *filePlan, choices map[string]string, now time.Time) fileResult {
	if fp.entry {
		text, changed, err := s.merged(fp, choices, now)
		if err != nil {
			return fileResult{blocked: i18n.T("backend.space.configSync.gate.invalid.body", map[string]string{"message": firstLine(err.Error())})}
		}
		return fileResult{data: text, changed: changed && !bytesEqual(text, fp.ours, fp.oursOK)}
	}
	if len(fp.rows) == 0 || fp.rows[0].Kind == KindBlocked {
		return fileResult{}
	}
	r := fp.rows[0]
	take := r.verdict == verdictTake || r.verdict == verdictConflict && choices[r.ID] == ChoiceShared
	switch {
	case !take:
		return fileResult{}
	case !r.theirsOK:
		return fileResult{remove: fp.oursOK, changed: fp.oursOK}
	}
	return fileResult{data: fp.theirs, changed: !bytesEqual(fp.theirs, fp.ours, fp.oursOK)}
}

func bytesEqual(a, b []byte, bOK bool) bool { return bOK && bytes.Equal(a, b) }

// diffs lists the files a pull changes with every conflict kept local.
func (s *Service) diffs(p *plan) []FileDiff {
	var out []FileDiff
	for _, fp := range p.files {
		res := s.resultFor(fp, nil, s.now())
		if !res.changed {
			continue
		}
		d := FileDiff{Module: fp.module, File: fp.file, Current: string(fp.ours), CurrentExists: fp.oursOK, AfterExists: !res.remove}
		if !res.remove {
			d.After = string(res.data)
		}
		out = append(out, d)
	}
	return out
}
