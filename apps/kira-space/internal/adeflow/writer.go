package adeflow

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
)

// ErrInvalid marks a caller mistake: a bad file name, an invalid workflow, a YAML syntax error in
// the file being edited. The ade engine maps it to E_BAD_REQUEST.
var ErrInvalid = configfs.ErrInvalid

// ErrConflict marks a save built on a file text that has since changed. The ade engine maps it to
// E_CONFLICT.
var ErrConflict = configfs.ErrConflict

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

const maxYamlBytes = configfs.MaxFileBytes

var fileNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*\.yaml$`)

// ValidateYaml parses src exactly as the reader does; exactly one of the result fields is set.
func ValidateYaml(src string) adewire.WorkflowValidation {
	wf, werr := Parse([]byte(src))
	if werr != nil {
		return adewire.WorkflowValidation{Error: werr}
	}
	return adewire.WorkflowValidation{Workflow: &wf}
}

func (r *Reader) path(file string) (string, error) {
	if !fileNamePattern.MatchString(file) {
		return "", invalidf("file name %q must be lowercase letters, digits, - or _ and end in .yaml", file)
	}
	return filepath.Join(r.Dir, file), nil
}

// ReadYaml returns one workflow file's raw text.
func (r *Reader) ReadYaml(file string) (adewire.WorkflowYaml, error) {
	p, err := r.path(file)
	if err != nil {
		return adewire.WorkflowYaml{}, err
	}
	src, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return adewire.WorkflowYaml{}, invalidf("workflow %s does not exist", file)
		}
		return adewire.WorkflowYaml{}, fmt.Errorf("adeflow: read %s: %w", file, err)
	}
	return adewire.WorkflowYaml{FileName: file, Path: p, Yaml: string(src), Hash: configfs.TextHash(src)}, nil
}

// SaveYaml writes src as the file's content, valid or not: the user's text is never refused, and
// the last valid version stays in use while the file is broken.
//
// baseHash, when set, must match the file's current text (see configfs.TextHash), else ErrConflict.
func (r *Reader) SaveYaml(file, baseHash, src string) (adewire.WorkflowEntry, error) {
	if _, err := r.path(file); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	if len(src) > maxYamlBytes {
		return adewire.WorkflowEntry{}, invalidf("the workflow file is larger than 1 MiB")
	}
	r.wmu.Lock()
	defer r.wmu.Unlock()
	e, err := r.folder().Save(file, baseHash, []byte(src))
	if err != nil {
		return adewire.WorkflowEntry{}, err
	}
	return toEntry(e), nil
}

// Save rewrites the file from the structured workflow, keeping comments, key order and the user's
// scalar styles wherever the content is unchanged. wf.ID must equal the file name stem.
//
// baseHash, when set, must match the file's current text (see configfs.TextHash), else ErrConflict.
func (r *Reader) Save(file, baseHash string, wf adewire.Workflow) (adewire.WorkflowEntry, error) {
	p, err := r.path(file)
	if err != nil {
		return adewire.WorkflowEntry{}, err
	}
	if wf.ID != strings.TrimSuffix(file, ".yaml") {
		return adewire.WorkflowEntry{}, invalidf("id must equal the file name without .yaml")
	}
	r.wmu.Lock()
	defer r.wmu.Unlock()
	var doc yaml.Node
	src, err := os.ReadFile(p)
	if baseHash != "" && configfs.TextHash(src) != baseHash {
		return adewire.WorkflowEntry{}, fmt.Errorf("%w: the file changed since it was loaded", ErrConflict)
	}
	switch {
	case err == nil:
		if err := yaml.Unmarshal(src, &doc); err != nil {
			return adewire.WorkflowEntry{}, invalidf("fix the YAML error on line %d first", yamlErrLineOf(err))
		}
	case !os.IsNotExist(err):
		return adewire.WorkflowEntry{}, fmt.Errorf("adeflow: read %s: %w", file, err)
	}
	out, err := encodeWorkflow(&doc, wf)
	if err != nil {
		return adewire.WorkflowEntry{}, err
	}
	if err := configfs.WriteAtomic(filepath.Join(r.Dir, file), out, 0o644); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	return r.entry(file, p)
}

// Import copies a workflow file from anywhere into the workflows dir. The target is <id>.yaml, or
// the sanitized source base name when the source does not parse. An existing target is refused.
func (r *Reader) Import(srcPath string) (adewire.WorkflowEntry, error) {
	if !filepath.IsAbs(srcPath) {
		return adewire.WorkflowEntry{}, invalidf("path must be absolute")
	}
	info, err := os.Stat(srcPath)
	if err != nil || !info.Mode().IsRegular() {
		return adewire.WorkflowEntry{}, invalidf("%s is not a readable file", srcPath)
	}
	if info.Size() > maxYamlBytes {
		return adewire.WorkflowEntry{}, invalidf("the workflow file is larger than 1 MiB")
	}
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return adewire.WorkflowEntry{}, invalidf("cannot read %s: %v", srcPath, err)
	}
	stem := ""
	if wf, werr := Parse(src); werr == nil {
		stem = wf.ID
	} else {
		stem = slug(strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath)), "workflow")
	}
	file := stem + ".yaml"
	r.wmu.Lock()
	defer r.wmu.Unlock()
	p := filepath.Join(r.Dir, file)
	if _, err := os.Lstat(p); err == nil {
		return adewire.WorkflowEntry{}, invalidf("%s already exists in the workflows folder", file)
	}
	if err := configfs.WriteAtomic(filepath.Join(r.Dir, file), src, 0o644); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	return r.entry(file, p)
}

// New creates a minimal valid one-step manual workflow named name. The file name is the slug of the
// name, suffixed -2, -3 ... when taken.
func (r *Reader) New(name string) (adewire.WorkflowEntry, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 || strings.ContainsAny(name, "\r\n") {
		return adewire.WorkflowEntry{}, invalidf("name must be 1 to 80 characters on one line")
	}
	base := slug(name, "workflow")
	r.wmu.Lock()
	defer r.wmu.Unlock()
	id := base
	for n := 2; ; n++ {
		if _, err := os.Lstat(filepath.Join(r.Dir, id+".yaml")); os.IsNotExist(err) {
			break
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
	wf := adewire.Workflow{ID: id, Name: name, Steps: []adewire.WorkflowStep{
		{ID: "work", Name: "Work", Kind: adewire.KindManual, Results: []adewire.StepResult{}},
	}}
	out, err := encodeWorkflow(&yaml.Node{}, wf)
	if err != nil {
		return adewire.WorkflowEntry{}, err
	}
	file := id + ".yaml"
	if err := configfs.WriteAtomic(filepath.Join(r.Dir, file), out, 0o644); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	return r.entry(file, filepath.Join(r.Dir, file))
}

func (r *Reader) entry(file, path string) (adewire.WorkflowEntry, error) {
	e, ok := r.folder().Get(file)
	if !ok {
		return adewire.WorkflowEntry{}, fmt.Errorf("adeflow: %s vanished after write", path)
	}
	return toEntry(e), nil
}

var slugStrip = regexp.MustCompile(`[^a-z0-9]+`)

// slug lowercases s to the idPattern alphabet; fallback when nothing is left.
func slug(s, fallback string) string {
	out := strings.Trim(slugStrip.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	if out == "" {
		return fallback
	}
	return out
}

func yamlErrLineOf(err error) int {
	if m := yamlErrLine.FindStringSubmatch(err.Error()); m != nil {
		n := 0
		fmt.Sscanf(m[1], "%d", &n)
		return n
	}
	return 0
}

// encodeWorkflow applies wf onto doc (zero value = new document), encodes it and re-parses the
// bytes; a result that does not parse is refused.
func encodeWorkflow(doc *yaml.Node, wf adewire.Workflow) ([]byte, error) {
	if doc.Kind == 0 || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		*doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	applyWorkflow(doc.Content[0], wf)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("adeflow: encode workflow: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("adeflow: encode workflow: %w", err)
	}
	if _, werr := Parse(buf.Bytes()); werr != nil {
		if werr.Line > 0 {
			return nil, invalidf("%s (line %d)", werr.Message, werr.Line)
		}
		return nil, invalidf("%s", werr.Message)
	}
	return buf.Bytes(), nil
}

// --- yaml.Node editing ---------------------------------------------------------------------------

type scalarKind int

const (
	kText scalarKind = iota
	kBlock
	kBool
	kInt
)

func find(m *yaml.Node, key string) (int, *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i, m.Content[i+1]
		}
	}
	return -1, nil
}

// put sets key to v, replacing in place or inserting after the nearest preceding key of order.
func put(m *yaml.Node, key string, v *yaml.Node, order []string) {
	if i, _ := find(m, key); i >= 0 {
		m.Content[i+1] = v
		return
	}
	at := 0
	for _, k := range order {
		if k == key {
			break
		}
		if i, _ := find(m, k); i >= 0 {
			at = i + 2
		}
	}
	kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	m.Content = append(m.Content, nil, nil)
	copy(m.Content[at+2:], m.Content[at:])
	m.Content[at], m.Content[at+1] = kn, v
}

func newScalar(kind scalarKind, want string) *yaml.Node {
	switch kind {
	case kBool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: want}
	case kInt:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: want}
	case kBlock:
		if strings.Contains(want, "\n") {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.LiteralStyle, Value: want + "\n"}
		}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: want}
}

func sameScalar(kind scalarKind, n *yaml.Node, want string) bool {
	if n.Kind != yaml.ScalarNode {
		return false
	}
	switch kind {
	case kBlock:
		return strings.TrimRight(n.Value, "\n") == want
	case kBool, kInt:
		return n.Value == want
	}
	return strings.TrimSpace(n.Value) == want
}

// setScalar updates key to want, leaving an equal node untouched (comments and style survive).
// add=false skips a missing key (a default value the file never spelled out).
func setScalar(m *yaml.Node, key string, kind scalarKind, want string, add bool, order []string) {
	_, n := find(m, key)
	switch {
	case n == nil:
		if add {
			put(m, key, newScalar(kind, want), order)
		}
	case sameScalar(kind, n, want):
	case n.Kind != yaml.ScalarNode:
		put(m, key, newScalar(kind, want), order)
	default:
		fresh := newScalar(kind, want)
		keepQuote := n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) != 0 && fresh.Style == 0 && kind != kBool && kind != kInt
		wasBlock := n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0
		n.Value, n.Tag, n.Alias = fresh.Value, fresh.Tag, nil
		switch {
		case fresh.Style != 0:
			n.Style = fresh.Style
		case kind == kBlock && wasBlock:
			n.Style, n.Value = yaml.LiteralStyle, want+"\n"
		case keepQuote:
		default:
			n.Style = 0
		}
	}
}

func scalarList(vals []string) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if len(vals) == 0 {
		seq.Style = yaml.FlowStyle
	}
	for _, v := range vals {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}
	return seq
}

func setList(m *yaml.Node, key string, vals []string, order []string) {
	_, n := find(m, key)
	if n == nil {
		if len(vals) > 0 {
			put(m, key, scalarList(vals), order)
		}
		return
	}
	if n.Kind == yaml.SequenceNode && len(n.Content) == len(vals) {
		same := true
		for i, c := range n.Content {
			if c.Kind != yaml.ScalarNode || c.Value != vals[i] {
				same = false
			}
		}
		if same {
			return
		}
	}
	put(m, key, scalarList(vals), order)
}

func itemID(n *yaml.Node) string {
	if n.Kind != yaml.MappingNode {
		return ""
	}
	if _, v := find(n, "id"); v != nil {
		return strings.TrimSpace(v.Value)
	}
	return ""
}

// syncItems returns the sequence content for ids: existing nodes matched by id, in ids order, with
// fresh mappings for new ids. apply fills each.
func syncItems(old *yaml.Node, ids []string, apply func(i int, item *yaml.Node)) *yaml.Node {
	seq := old
	var existing []*yaml.Node
	if seq != nil && seq.Kind == yaml.SequenceNode {
		existing = seq.Content
	} else {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	}
	used := make([]bool, len(existing))
	content := make([]*yaml.Node, 0, len(ids))
	for i, id := range ids {
		var item *yaml.Node
		for j, e := range existing {
			if !used[j] && itemID(e) == id {
				used[j], item = true, e
				break
			}
		}
		if item == nil {
			item = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		apply(i, item)
		content = append(content, item)
	}
	seq.Content = content
	return seq
}

func applyWorkflow(root *yaml.Node, wf adewire.Workflow) {
	setScalar(root, "id", kText, wf.ID, true, topOrder)
	setScalar(root, "name", kText, wf.Name, true, topOrder)
	if i, _ := find(root, "kira_space_mcp"); i >= 0 { // legacy switch (P408)
		root.Content = append(root.Content[:i], root.Content[i+2:]...)
	}
	ids := make([]string, len(wf.Steps))
	for i, st := range wf.Steps {
		ids[i] = st.ID
	}
	_, old := find(root, "steps")
	put(root, "steps", syncItems(old, ids, func(i int, item *yaml.Node) { applyStep(item, wf.Steps[i]) }), topOrder)
}

// applyStep writes one step: the keys of its kind, and none of another kind's (a kind change leaves them behind).
func applyStep(m *yaml.Node, s adewire.WorkflowStep) {
	setScalar(m, "id", kText, s.ID, true, stepOrder)
	setScalar(m, "name", kText, s.Name, true, stepOrder)
	setScalar(m, "kind", kText, s.Kind, true, stepOrder)
	setScalar(m, "status", kText, s.Status, s.Status != "", stepOrder)
	if s.Skip {
		setScalar(m, "skip", kBool, "true", true, stepOrder)
	} else {
		dropKeys(m, "skip")
	}
	keep := map[string]bool{"id": true, "name": true, "kind": true, "status": true, "skip": true}
	for _, k := range stepKeys[s.Kind] {
		keep[k] = true
	}
	dropOthers(m, keep)
	if s.Kind == adewire.KindManual {
		setScalar(m, "session", kBool, fmt.Sprint(s.Session), s.Session, stepOrder)
		if s.Session && s.Prompt != "" {
			setScalar(m, "prompt", kBlock, s.Prompt, true, stepOrder)
		} else {
			dropKeys(m, "prompt")
		}
		if s.Session {
			applySettings(m, s.Settings)
		} else {
			dropKeys(m, "settings")
		}
		return
	}
	setScalar(m, "runs_on", kText, s.RunsOn, true, stepOrder)
	setScalar(m, "before", kText, defaultStr(s.Before, "auto"), s.Before != "" && s.Before != "auto", stepOrder)
	applyResults(m, s)
	setScalar(m, "timeout", kText, s.Timeout, true, stepOrder)
	switch s.Kind {
	case adewire.KindScript:
		setScalar(m, "command", kBlock, s.Command, true, stepOrder)
	case adewire.KindReview:
		applySandbox(m, s.Sandbox)
		applySettings(m, s.Settings)
		applyReview(m, defaultReview(s.Review))
	default:
		applySandbox(m, s.Sandbox)
		applySettings(m, s.Settings)
		if s.SmartScript == "" {
			dropKeys(m, "smart_script", "params")
			setScalar(m, "prompt", kBlock, s.Prompt, true, stepOrder)
			setList(m, "allowed_tools", s.AllowedTools, stepOrder)
			return
		}
		dropKeys(m, "prompt", "allowed_tools")
		setScalar(m, "smart_script", kText, s.SmartScript, true, stepOrder)
		setParams(m, s.Params)
	}
}

func defaultReview(r *adewire.ReviewStepConfig) adewire.ReviewStepConfig {
	if r == nil {
		return adewire.ReviewStepConfig{SendTo: adewire.ReviewSendImplementer}
	}
	return *r
}

// applySandbox writes a step's sandbox block, or removes it when the policy is the default so a
// file without one stays unchanged.
func applySandbox(m *yaml.Node, sb adewire.StepSandbox) {
	if sb.IsDefault() {
		dropKeys(m, "sandbox")
		return
	}
	sb = sb.Normalized()
	_, sn := find(m, "sandbox")
	if sn == nil || sn.Kind != yaml.MappingNode {
		sn = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		put(m, "sandbox", sn, stepOrder)
	}
	order := []string{"access", "read", "allow", "network"}
	setScalar(sn, "access", kText, sb.Access, true, order)
	if sb.Access == adewire.SandboxOff {
		dropKeys(sn, "read", "allow", "network")
		return
	}
	setList(sn, "read", sb.Read, order)
	setList(sn, "allow", sb.Allow, order)
	if len(sb.Read) == 0 {
		dropKeys(sn, "read")
	}
	if len(sb.Allow) == 0 {
		dropKeys(sn, "allow")
	}
	setScalar(sn, "network", kBool, fmt.Sprint(sb.Network), !sb.Network, order)
	if sb.Network {
		dropKeys(sn, "network")
	}
}

// applySettings writes a step's settings block, or removes it when no catalog setting is set so a
// file without one stays unchanged. Keys and agents leave in catalog order; ones no longer set are dropped.
func applySettings(m *yaml.Node, settings map[string]map[string]string) {
	if !anyStageSetting(settings) {
		dropKeys(m, "settings")
		return
	}
	_, sn := find(m, "settings")
	if sn == nil || sn.Kind != yaml.MappingNode {
		sn = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		put(m, "settings", sn, stepOrder)
	}
	var agentOrder []string
	for _, set := range stageCatalog {
		agentOrder = append(agentOrder, set.ID)
	}
	for _, set := range stageCatalog {
		applyProviderSettings(sn, set, settings[set.ID], agentOrder)
	}
}

// anyStageSetting reports whether any catalog setting has a value.
func anyStageSetting(settings map[string]map[string]string) bool {
	for _, set := range stageCatalog {
		for _, st := range set.Settings {
			if settings[set.ID][st.Key] != "" {
				return true
			}
		}
	}
	return false
}

// applyProviderSettings writes one provider's block in sn, or drops it when none of its settings is set.
func applyProviderSettings(sn *yaml.Node, set providers.StageSet, vals map[string]string, agentOrder []string) {
	keyOrder := make([]string, 0, len(set.Settings))
	for _, st := range set.Settings {
		keyOrder = append(keyOrder, st.Key)
	}
	var pn *yaml.Node
	for _, st := range set.Settings {
		val := vals[st.Key]
		if val == "" {
			continue
		}
		if pn == nil {
			_, pn = find(sn, set.ID)
			if pn == nil || pn.Kind != yaml.MappingNode {
				pn = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				put(sn, set.ID, pn, agentOrder)
			}
		}
		setScalar(pn, st.Key, kInt, val, true, keyOrder)
	}
	if pn == nil {
		dropKeys(sn, set.ID)
		return
	}
	for _, st := range set.Settings {
		if vals[st.Key] == "" {
			dropKeys(pn, st.Key)
		}
	}
}

// applyReview writes a review step's block. It is optional: an all-default block is not added to a file
// without one.
func applyReview(m *yaml.Node, r adewire.ReviewStepConfig) {
	sendTo := defaultStr(r.SendTo, adewire.ReviewSendImplementer)
	_, rn := find(m, "review")
	if rn == nil && sendTo == adewire.ReviewSendImplementer && r.Implementer == "" {
		return
	}
	if rn == nil || rn.Kind != yaml.MappingNode {
		rn = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		put(m, "review", rn, stepOrder)
	}
	order := []string{"send_to", "implementer"}
	setScalar(rn, "send_to", kText, sendTo, sendTo != adewire.ReviewSendImplementer, order)
	if r.Implementer == "" {
		dropKeys(rn, "implementer")
	} else {
		setScalar(rn, "implementer", kText, r.Implementer, true, order)
	}
}

// applyResults writes a step's routing: max_returns when it is not the kind's default, and a results list
// when the results are not the defaults (or the file already has one).
func applyResults(m *yaml.Node, s adewire.WorkflowStep) {
	results := s.Results
	if len(results) == 0 {
		results = adewire.DefaultResults()
	}
	review := s.Kind == adewire.KindReview
	if def := adewire.DefaultLimit(review); s.MaxReturns != def {
		setScalar(m, "max_returns", kInt, strconv.Itoa(s.MaxReturns), true, stepOrder)
	} else if _, cur := find(m, "max_returns"); cur != nil && cur.Value != strconv.Itoa(def) {
		dropKeys(m, "max_returns")
	}
	_, old := find(m, "results")
	switch {
	case old != nil && sameResults(results, nodeResults(old)):
		return
	case sameResults(results, adewire.DefaultResults()):
		dropKeys(m, "results")
		return
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	put(m, "results", syncItems(old, ids, func(i int, item *yaml.Node) { applyResult(item, results[i]) }), stepOrder)
}

// nodeResults reads the results a file spells out, without validating them.
func nodeResults(n *yaml.Node) []adewire.StepResult {
	var out []adewire.StepResult
	for _, item := range n.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		r := adewire.StepResult{}
		for i := 0; i+1 < len(item.Content); i += 2 {
			val := item.Content[i+1].Value
			switch item.Content[i].Value {
			case "id":
				r.ID = strings.TrimSpace(val)
			case "ok":
				r.OK = val == "true"
			case "description":
				r.Description = strings.TrimSpace(val)
			case "next":
				r.Next = strings.TrimSpace(val)
			}
		}
		out = append(out, r)
	}
	return out
}

// sameResults compares what a file spells out: id, ok, description and route.
func sameResults(a, b []adewire.StepResult) bool {
	return slices.EqualFunc(a, b, func(x, y adewire.StepResult) bool {
		return x.ID == y.ID && x.OK == y.OK && x.Description == y.Description && defaultStr(x.Next, adewire.DefaultRoute(x.OK)) == defaultStr(y.Next, adewire.DefaultRoute(y.OK))
	})
}

func applyResult(m *yaml.Node, r adewire.StepResult) {
	setScalar(m, "id", kText, r.ID, true, resultOrder)
	setScalar(m, "ok", kBool, fmt.Sprint(r.OK), true, resultOrder)
	if r.Description != "" {
		setScalar(m, "description", kText, r.Description, true, resultOrder)
	} else {
		dropKeys(m, "description")
	}
	if next := defaultStr(r.Next, adewire.DefaultRoute(r.OK)); next != adewire.DefaultRoute(r.OK) {
		setScalar(m, "next", kText, next, true, resultOrder)
	} else {
		dropKeys(m, "next")
	}
}

// dropKeys removes keys a step of this shape does not allow.
func dropKeys(m *yaml.Node, keys ...string) {
	for _, k := range keys {
		if i, _ := find(m, k); i >= 0 {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
		}
	}
}

// setParams writes a smart step's params: a scalar for one value, a list for more. Existing keys keep
// their place; new ones are added in name order. No params removes the key.
func setParams(m *yaml.Node, params map[string][]string) {
	if len(params) == 0 {
		dropKeys(m, "params")
		return
	}
	_, old := find(m, "params")
	if old == nil || old.Kind != yaml.MappingNode {
		old = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	for i := 0; i+1 < len(old.Content); {
		if _, keep := params[old.Content[i].Value]; keep {
			i += 2
			continue
		}
		old.Content = append(old.Content[:i], old.Content[i+2:]...)
	}
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		vals := params[name]
		if _, cur := find(old, name); cur != nil && sameParam(cur, vals) {
			continue
		}
		var v *yaml.Node
		if len(vals) == 1 {
			v = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: vals[0]}
		} else {
			v = scalarList(vals)
		}
		if i, _ := find(old, name); i >= 0 {
			old.Content[i+1] = v
			continue
		}
		old.Content = append(old.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, v)
	}
	put(m, "params", old, stepOrder)
}

// sameParam reports whether a node already holds vals.
func sameParam(n *yaml.Node, vals []string) bool {
	got, ok := scalarValues(n)
	return ok && slices.Equal(got, vals)
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// dropOthers removes keys not in keep.
func dropOthers(m *yaml.Node, keep map[string]bool) {
	for i := 0; i+1 < len(m.Content); {
		if keep[m.Content[i].Value] {
			i += 2
			continue
		}
		m.Content = append(m.Content[:i], m.Content[i+2:]...)
	}
}
