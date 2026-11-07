package adeflow

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// StepsFolder is the library module folder name under <home>/config and its config_last_valid folder name.
const StepsFolder = "steps"

// StepLibrary lists, resolves and writes library steps over a configfs.Folder. A library step is one
// step with no workflow-only keys; a workflow references it by id (see refs.go).
type StepLibrary struct {
	Dir string
	// DB holds the last valid version of each file (config_last_valid). Nil keeps it in memory.
	DB  *sql.DB
	Now func() time.Time

	wmu  sync.Mutex // serializes the writer methods
	once sync.Once
	f    *configfs.Folder[adewire.WorkflowStep]
}

// memProjection keeps last-valid values in memory, for a library without a DB.
type memProjection struct {
	mu sync.Mutex
	m  map[string]string
}

func (p *memProjection) Snapshot() (map[string]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]string, len(p.m))
	for k, v := range p.m {
		out[k] = v
	}
	return out, nil
}

func (p *memProjection) Record(file, js string, _ int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m[file] = js
	return nil
}

func (p *memProjection) Forget(file string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.m, file)
	return nil
}

func (l *StepLibrary) folder() *configfs.Folder[adewire.WorkflowStep] {
	l.once.Do(func() {
		var proj configfs.Projection = &memProjection{m: map[string]string{}}
		if l.DB != nil {
			proj = configfs.TableProjection{DB: l.DB, Folder: StepsFolder}
		}
		l.f = &configfs.Folder[adewire.WorkflowStep]{
			Dir:        l.Dir,
			Ext:        ".yaml",
			Name:       fileNamePattern,
			Decode:     decodeLibraryStep,
			Projection: proj,
			Now:        l.Now,
		}
	})
	return l.f
}

func decodeLibraryStep(file string, src []byte) (adewire.WorkflowStep, *configfs.Problem) {
	step, _, werr := ParseStep(src)
	if werr == nil && step.ID != strings.TrimSuffix(file, ".yaml") {
		werr = &adewire.WorkflowError{Message: "id must equal the file name without .yaml"}
	}
	if werr != nil {
		return adewire.WorkflowStep{}, &configfs.Problem{Line: werr.Line, Message: werr.Message}
	}
	return step, nil
}

func (l *StepLibrary) path(file string) (string, error) {
	if !fileNamePattern.MatchString(file) {
		return "", invalidf("file name %q must be lowercase letters, digits, - or _ and end in .yaml", file)
	}
	return filepath.Join(l.Dir, file), nil
}

func stepEntry(e configfs.Entry[adewire.WorkflowStep]) adewire.StepEntry {
	out := adewire.StepEntry{FileName: e.File, Path: e.Path, Hash: e.Hash, Step: e.Value, UsedBy: []string{}}
	if e.Problem != nil {
		out.Error = &adewire.WorkflowError{Line: e.Problem.Line, Message: e.Problem.Message}
	}
	return out
}

// Watch calls onChange (debounced) when a library file changes.
func (l *StepLibrary) Watch(onChange func()) (stop func(), err error) {
	return l.folder().Watch(onChange)
}

// WriteDocs writes the folder's AGENTS.md, creating the folder when missing.
func (l *StepLibrary) WriteDocs() error { return l.folder().WriteDocs(StepDocModel()) }

// List reads every library file, sorted by name. usedBy reports the workflow ids that reference a step id.
func (l *StepLibrary) List(usedBy func(id string) []string) []adewire.StepEntry {
	entries := l.folder().List()
	out := make([]adewire.StepEntry, 0, len(entries))
	for _, e := range entries {
		entry := stepEntry(e)
		if usedBy != nil {
			if ids := usedBy(strings.TrimSuffix(e.File, ".yaml")); ids != nil {
				entry.UsedBy = ids
			}
		}
		out = append(out, entry)
	}
	return out
}

// Get returns the effective library step with the given id: the file's current version when valid,
// else the last valid one.
func (l *StepLibrary) Get(id string) (adewire.WorkflowStep, bool) {
	if l == nil || id == "" {
		return adewire.WorkflowStep{}, false
	}
	if e, ok := l.folder().Get(id + ".yaml"); ok && e.Value != nil && e.Value.ID == id {
		return *e.Value, true
	}
	return adewire.WorkflowStep{}, false
}

// lookup is Lookup as a StepLookup; nil on a nil library, which refuses every reference.
func (l *StepLibrary) lookup() StepLookup {
	if l == nil {
		return nil
	}
	return l.Lookup
}

// Lookup is the StepLookup over the files' current text: a broken file is an error, never its last
// valid version, so a workflow that references it falls back as a whole.
func (l *StepLibrary) Lookup(id string) (*yaml.Node, string) {
	p, err := l.path(id + ".yaml")
	if err != nil {
		return nil, "does not exist"
	}
	src, err := configfs.ReadCapped(p, configfs.MaxFileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "does not exist"
		}
		return nil, "cannot be read"
	}
	step, node, werr := ParseStep(src)
	switch {
	case werr != nil:
		return nil, "is invalid: " + werr.Message
	case step.ID != id:
		return nil, "is invalid: id must equal the file name without .yaml"
	}
	return node, ""
}

// ReadYaml returns one library file's raw text.
func (l *StepLibrary) ReadYaml(file string) (adewire.StepYaml, error) {
	p, err := l.path(file)
	if err != nil {
		return adewire.StepYaml{}, err
	}
	src, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return adewire.StepYaml{}, invalidf("step %s does not exist", file)
		}
		return adewire.StepYaml{}, fmt.Errorf("adeflow: read %s: %w", file, err)
	}
	return adewire.StepYaml{FileName: file, Path: p, Yaml: string(src), Hash: configfs.TextHash(src)}, nil
}

// ValidateStepYaml parses src exactly as the library does.
func ValidateStepYaml(src string) adewire.StepValidation {
	step, _, werr := ParseStep([]byte(src))
	if werr != nil {
		return adewire.StepValidation{Error: werr}
	}
	return adewire.StepValidation{Step: &step}
}

// SaveYaml writes src as the file's content, valid or not.
func (l *StepLibrary) SaveYaml(file, baseHash, src string) (adewire.StepEntry, error) {
	if _, err := l.path(file); err != nil {
		return adewire.StepEntry{}, err
	}
	if len(src) > maxYamlBytes {
		return adewire.StepEntry{}, invalidf("the step file is larger than 1 MiB")
	}
	l.wmu.Lock()
	defer l.wmu.Unlock()
	e, err := l.folder().Save(file, baseHash, []byte(src))
	if err != nil {
		return adewire.StepEntry{}, err
	}
	return stepEntry(e), nil
}

// Save rewrites the file from the structured step, keeping comments and key order where the content
// is unchanged. st.ID must equal the file name stem.
func (l *StepLibrary) Save(file, baseHash string, st adewire.WorkflowStep) (adewire.StepEntry, error) {
	p, err := l.path(file)
	if err != nil {
		return adewire.StepEntry{}, err
	}
	if st.ID != strings.TrimSuffix(file, ".yaml") {
		return adewire.StepEntry{}, invalidf("id must equal the file name without .yaml")
	}
	l.wmu.Lock()
	defer l.wmu.Unlock()
	var doc yaml.Node
	src, err := os.ReadFile(p)
	if baseHash != "" && configfs.TextHash(src) != baseHash {
		return adewire.StepEntry{}, fmt.Errorf("%w: the file changed since it was loaded", ErrConflict)
	}
	switch {
	case err == nil:
		if err := yaml.Unmarshal(src, &doc); err != nil {
			return adewire.StepEntry{}, invalidf("fix the YAML error on line %d first", yamlErrLineOf(err))
		}
	case !os.IsNotExist(err):
		return adewire.StepEntry{}, fmt.Errorf("adeflow: read %s: %w", file, err)
	}
	out, err := encodeStep(&doc, st)
	if err != nil {
		return adewire.StepEntry{}, err
	}
	if err := configfs.WriteAtomic(p, out, 0o644); err != nil {
		return adewire.StepEntry{}, err
	}
	return l.entry(file)
}

// New creates a library step named name. The file name is the slug of the name, suffixed -2, -3 ...
// when taken. from set, the step is a copy of that step; else a minimal step of kind.
func (l *StepLibrary) New(name string, kind adewire.StepKind, from *adewire.WorkflowStep) (adewire.StepEntry, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 || strings.ContainsAny(name, "\r\n") {
		return adewire.StepEntry{}, invalidf("name must be 1 to 80 characters on one line")
	}
	st, err := newStepBody(name, kind, from)
	if err != nil {
		return adewire.StepEntry{}, err
	}
	l.wmu.Lock()
	defer l.wmu.Unlock()
	base := slug(name, "step")
	id := base
	for n := 2; ; n++ {
		if _, err := os.Lstat(filepath.Join(l.Dir, id+".yaml")); os.IsNotExist(err) {
			break
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
	st.ID, st.Name = id, name
	out, err := encodeStep(&yaml.Node{}, st)
	if err != nil {
		return adewire.StepEntry{}, err
	}
	file := id + ".yaml"
	if err := configfs.WriteAtomic(filepath.Join(l.Dir, file), out, 0o644); err != nil {
		return adewire.StepEntry{}, err
	}
	return l.entry(file)
}

// newStepBody is the starting content of a new library step.
func newStepBody(name string, kind adewire.StepKind, from *adewire.WorkflowStep) (adewire.WorkflowStep, error) {
	if from != nil {
		return libraryForm(*from), nil
	}
	switch kind {
	case adewire.KindManual:
		return adewire.WorkflowStep{Kind: kind, Results: []adewire.StepResult{}}, nil
	case adewire.KindAgent:
		return withTimeout(adewire.WorkflowStep{Kind: kind, RunsOn: "each repo", Prompt: name, Results: adewire.DefaultResults()}, kind), nil
	case adewire.KindScript:
		return withTimeout(adewire.WorkflowStep{Kind: kind, RunsOn: "once", Command: "true", Results: adewire.DefaultResults()}, kind), nil
	case adewire.KindReview:
		return withTimeout(adewire.WorkflowStep{Kind: kind, RunsOn: "each repo", Results: adewire.DefaultResults(), Review: &adewire.ReviewStepConfig{SendTo: adewire.ReviewSendImplementer}}, kind), nil
	}
	return adewire.WorkflowStep{}, invalidf("kind must be %s", oneOf(kinds))
}

func withTimeout(st adewire.WorkflowStep, kind adewire.StepKind) adewire.WorkflowStep {
	st.Timeout = "30m"
	st.Before = beforeValues[0]
	st.MaxReturns = adewire.DefaultLimit(kind == adewire.KindReview)
	st.Sandbox = adewire.DefaultSandboxFor(kind)
	return st
}

// libraryForm is st as it reads in a library file: no skip, no review implementer, no workflow
// reference, and result routes that name a step become the default route.
func libraryForm(st adewire.WorkflowStep) adewire.WorkflowStep {
	st.Skip, st.Use, st.PromptExtra = false, "", ""
	if st.Review != nil {
		rv := *st.Review
		rv.Implementer = ""
		st.Review = &rv
	}
	in := st.Results
	st.Results = make([]adewire.StepResult, len(in))
	for i, r := range in {
		r.Loop = false
		switch r.Next {
		case adewire.RouteNext, adewire.RouteEnd, adewire.RouteStop, "":
		default:
			r.Next = adewire.DefaultRoute(r.OK)
		}
		st.Results[i] = r
	}
	return st
}

// Delete removes one library file. The caller checks that no workflow uses it.
func (l *StepLibrary) Delete(file string) error {
	p, err := l.path(file)
	if err != nil {
		return err
	}
	l.wmu.Lock()
	defer l.wmu.Unlock()
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return invalidf("step %s does not exist", file)
		}
		return fmt.Errorf("adeflow: delete %s: %w", file, err)
	}
	return nil
}

func (l *StepLibrary) entry(file string) (adewire.StepEntry, error) {
	e, ok := l.folder().Get(file)
	if !ok {
		return adewire.StepEntry{}, fmt.Errorf("adeflow: %s vanished after write", file)
	}
	return stepEntry(e), nil
}

// encodeStep applies st onto doc (zero value = new document), encodes it and re-parses the bytes; a
// result that does not parse is refused.
func encodeStep(doc *yaml.Node, st adewire.WorkflowStep) ([]byte, error) {
	if doc.Kind == 0 || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		*doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	st = libraryForm(st)
	applyStep(doc.Content[0], st)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("adeflow: encode step: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("adeflow: encode step: %w", err)
	}
	if _, _, werr := ParseStep(buf.Bytes()); werr != nil {
		if werr.Line > 0 {
			return nil, invalidf("%s (line %d)", werr.Message, werr.Line)
		}
		return nil, invalidf("%s", werr.Message)
	}
	return buf.Bytes(), nil
}
