package apifiles

import (
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

// Variable edit ops.
const (
	OpSet      = "set"
	OpUnset    = "unset"
	OpRename   = "rename"
	OpLock     = "lock"
	OpUnlock   = "unlock"
	OpDescribe = "describe"
	OpShare    = "share"
	OpMove     = "move"
)

// Variable edit errors, matched with errors.Is.
var (
	ErrVariableGone = errors.New("apifiles: the variable is not in the file")
	ErrVariableKind = errors.New("apifiles: the variable holds the other kind of value")
	ErrUnconfirmed  = errors.New("apifiles: unlocking a secret needs the user's confirmation")
	ErrEditInvalid  = errors.New("apifiles: invalid variable edit")
)

// VariableEdit is one change to one variable of one collection or environment file. Pointer fields
// tell "not sent" from an empty value. Scope is collection or environment, OwnerID the row id of that
// owner.
type VariableEdit struct {
	Scope   string `json:"scope"`
	OwnerID string `json:"ownerId"`
	Op      string `json:"op"`
	Name    string `json:"name"`
	// NewName is the new name of a rename.
	NewName string `json:"newName,omitempty"`
	// Value is a plain variable's value (set).
	Value *string `json:"value,omitempty"`
	// SecretValue is a secret variable's value (set); it is sealed and never written to the file.
	SecretValue *string `json:"secretValue,omitempty"`
	// Description is the new description (describe); empty removes it.
	Description *string `json:"description,omitempty"`
	// Agent and AgentHosts are the sharing state (share).
	Agent      *bool    `json:"agent,omitempty"`
	AgentHosts []string `json:"agentHosts,omitempty"`
	// Before names the variable a move puts the row in front of; empty moves it last.
	Before string `json:"before,omitempty"`
	// Confirmed is the user's go-ahead after the reveal gate (unlock).
	Confirmed bool `json:"confirmed,omitempty"`
}

// editRun is the state of one EditVariables call: the secret writes and releases its edits queue.
type editRun struct {
	s *Store
	// taken holds every secret name in use or queued, for freeName.
	taken map[string]bool
	// puts are the secret values to seal, in order; created lists the names that did not exist before.
	puts    []secretPut
	created []string
	// release lists secret names a removed or unlocked variable named; each is deleted when nothing
	// references it any more.
	release []string
	owners  map[OwnerKey]bool
}

type secretPut struct{ name, value string }

// EditVariables applies the edits to the variable lists of the files they name, as YAML node edits
// (comments and key order stay). Edits are grouped by file and applied in order. Secret values are
// sealed first, every file is validated before the first is written, and one change event fires at
// the end. A file with a problem is refused with BrokenError and nothing is written.
func (s *Store) EditVariables(edits []VariableEdit) error {
	if len(edits) == 0 {
		return nil
	}
	type target struct {
		kind, file string
		key        OwnerKey
		edits      []VariableEdit
	}
	var order []OwnerKey
	byOwner := map[OwnerKey]*target{}
	for _, e := range edits {
		key := OwnerKey{Scope: e.Scope, ID: e.OwnerID}
		t, ok := byOwner[key]
		if !ok {
			var (
				file string
				kind string
				err  error
			)
			switch e.Scope {
			case OwnerCollection:
				kind = kindCollection
				file, err = s.collectionFile(e.OwnerID)
			case OwnerEnvironment:
				kind = kindEnvironment
				file, err = s.environmentFile(e.OwnerID)
			default:
				err = fmt.Errorf("%w: scope %q", ErrEditInvalid, e.Scope)
			}
			if err != nil {
				return err
			}
			t = &target{kind: kind, file: file, key: key}
			byOwner[key] = t
			order = append(order, key)
		}
		t.edits = append(t.edits, e)
	}

	s.mu.Lock()
	s.pend = &Changes{}
	err := s.editVariablesLocked(len(order), func(i int, run *editRun) (string, string, []byte, error) {
		t := byOwner[order[i]]
		out, err := run.applyFile(t.kind, t.file, t.key, t.edits)
		return t.kind, t.file, out, err
	})
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)
	return err
}

func (s *Store) editVariablesLocked(n int, next func(i int, run *editRun) (kind, file string, out []byte, err error)) error {
	run := &editRun{s: s, taken: map[string]bool{}, owners: map[OwnerKey]bool{}}
	names, referenced, err := s.secretNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		run.taken[name] = true
	}
	for name := range referenced {
		run.taken[name] = true
	}
	existing := map[string]bool{}
	for _, name := range names {
		existing[name] = true
	}

	type pending struct {
		kind, file string
		hash       string
		before     string
		out        []byte
	}
	var writes []pending
	for i := 0; i < n; i++ {
		kind, file, out, err := next(i, run)
		if err != nil {
			return err
		}
		src, hash, err := s.readLocked(kind, file)
		if err != nil {
			return err
		}
		writes = append(writes, pending{kind: kind, file: file, hash: hash, before: string(src), out: out})
	}

	if len(run.puts) > 0 && !s.o.Secrets.Status().Available {
		return ErrSecretStoreOff
	}
	// A failed call leaves nothing behind: files already written go back to their text, a secret
	// that existed gets its old value back, and a new one is deleted unless a file still names it.
	var sealed []string
	olds := map[string]string{}
	undo := func(saved []pending, hashes []string) {
		for i, w := range saved {
			_, _ = s.saveLocked(w.kind, w.file, hashes[i], w.before)
		}
		_, referenced, err := s.secretNames()
		if err != nil {
			// References unknown: keep every new secret rather than drop one a file may name.
			referenced = map[string]bool{}
			for _, name := range sealed {
				referenced[name] = true
			}
		}
		for _, name := range sealed {
			switch old, had := olds[name]; {
			case had:
				_ = s.o.Secrets.Put(name, old)
			case !existing[name] && !referenced[name]:
				_ = s.o.Secrets.Delete(name)
			}
		}
	}
	for _, p := range run.puts {
		if existing[p.name] {
			if _, kept := olds[p.name]; !kept {
				old, err := s.o.Secrets.Reveal(p.name)
				if err != nil && !errors.Is(err, configfs.ErrSecretMissing) {
					undo(nil, nil)
					return fmt.Errorf("apifiles: read secret: %w", err)
				}
				if err == nil {
					olds[p.name] = old
				}
			}
		}
		if err := s.o.Secrets.Put(p.name, p.value); err != nil {
			undo(nil, nil)
			return fmt.Errorf("apifiles: seal secret: %w", err)
		}
		sealed = append(sealed, p.name)
	}
	var saved []pending
	var hashes []string
	for _, w := range writes {
		d, err := s.saveLocked(w.kind, w.file, w.hash, string(w.out))
		if err != nil {
			undo(saved, hashes)
			return err
		}
		saved = append(saved, w)
		hashes = append(hashes, d.Hash)
	}
	for key := range run.owners {
		s.pend.variables(key.Scope, key.ID)
	}
	s.pend.Config = true
	return run.releaseUnused()
}

func (s *Store) readLocked(kind, file string) ([]byte, string, error) {
	if kind == kindCollection {
		return s.collections.Read(file)
	}
	return s.environments.Read(file)
}

// releaseUnused deletes each queued secret that no variable references after the writes.
func (r *editRun) releaseUnused() error {
	if len(r.release) == 0 {
		return nil
	}
	_, referenced, err := r.s.secretNames()
	if err != nil {
		return err
	}
	for _, name := range r.release {
		if !referenced[name] {
			if err := r.s.o.Secrets.Delete(name); err != nil {
				return fmt.Errorf("apifiles: release secret: %w", err)
			}
		}
	}
	return nil
}

// applyFile applies one file's edits to its current text and validates the result.
func (r *editRun) applyFile(kind, file string, key OwnerKey, edits []VariableEdit) ([]byte, error) {
	s := r.s
	if kind == kindCollection {
		e, ok := s.collections.Get(file)
		switch {
		case !ok:
			return nil, fmt.Errorf("%w: %s", ErrNotFound, file)
		case e.Problem != nil:
			s.noteProblems1(file, collectionSig(e))
			return nil, &BrokenError{File: file, Line: e.Problem.Line}
		case e.Value != nil && len(e.Value.Skipped) > 0:
			s.noteProblems1(file, collectionSig(e))
			return nil, &BrokenError{File: file, Line: e.Value.Skipped[0].Line}
		}
	} else {
		e, ok := s.environments.Get(file)
		switch {
		case !ok:
			return nil, fmt.Errorf("%w: %s", ErrNotFound, file)
		case e.Problem != nil:
			s.noteProblems1(file, problemSig(e.Problem))
			return nil, &BrokenError{File: file, Line: e.Problem.Line}
		}
	}
	src, _, err := s.readLocked(kind, file)
	if err != nil {
		return nil, err
	}
	doc, root, err := parseDoc(src)
	if err != nil {
		return nil, err
	}
	ext := EnvironmentExt
	if kind == kindCollection {
		ext = CollectionExt
	}
	stem, _ := StemOf(file, ext)
	for _, e := range edits {
		if err := r.applyOne(root, stem, kind, e); err != nil {
			return nil, err
		}
		r.owners[key] = true
	}
	out, err := encodeDoc(doc)
	if err != nil {
		return nil, err
	}
	var p *configfs.Problem
	if kind == kindCollection {
		_, p = s.dec.collection(file, out)
	} else {
		_, p = s.dec.environment(file, out)
	}
	if p != nil {
		return nil, &InvalidError{Problem: p}
	}
	return out, nil
}

// variablesSeq returns the `variables` sequence of a file's root, creating it when create is set.
func variablesSeq(root *yaml.Node, create bool) *yaml.Node {
	if i := mapIndex(root, "variables"); i >= 0 {
		n := root.Content[i]
		if n.Kind == yaml.SequenceNode {
			n.Style &^= yaml.FlowStyle
			return n
		}
		if !create {
			return nil
		}
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		root.Content[i] = seq
		return seq
	}
	if !create {
		return nil
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "variables"}, seq)
	return seq
}

// findVariable returns the first variable mapping named name and its index in seq.
func findVariable(seq *yaml.Node, name string) (*yaml.Node, int) {
	if seq == nil {
		return nil, -1
	}
	for i, n := range seq.Content {
		if n.Kind != yaml.MappingNode {
			continue
		}
		if at := mapIndex(n, "name"); at >= 0 && n.Content[at].Value == name {
			return n, i
		}
	}
	return nil, -1
}

func removeKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

func setValueKey(m *yaml.Node, key string, v any) error {
	node, err := valueNode(v)
	if err != nil {
		return err
	}
	if i := mapIndex(m, key); i >= 0 {
		node.HeadComment, node.LineComment = m.Content[i].HeadComment, m.Content[i].LineComment
		m.Content[i] = node
		return nil
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, node)
	return nil
}

func scalarOf(m *yaml.Node, key string) string {
	if i := mapIndex(m, key); i >= 0 {
		return m.Content[i].Value
	}
	return ""
}

func (r *editRun) newSecretName(stem, variable string) string {
	name := freeName(SecretName(stem, variable), func(c string) bool { return r.taken[c] })
	r.taken[name] = true
	return name
}

func (r *editRun) put(name, value string) {
	r.puts = append(r.puts, secretPut{name: name, value: value})
}

func invalidEdit(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrEditInvalid, fmt.Sprintf(format, args...))
}

func (r *editRun) applyOne(root *yaml.Node, stem, kind string, e VariableEdit) error {
	if e.Name == "" {
		return invalidEdit("name is required")
	}
	create := e.Op == OpSet
	seq := variablesSeq(root, create)
	v, at := findVariable(seq, e.Name)
	if v == nil && e.Op != OpSet {
		return fmt.Errorf("%w: %s", ErrVariableGone, e.Name)
	}
	switch e.Op {
	case OpSet:
		return r.opSet(seq, v, stem, e)
	case OpUnset:
		if ref := SecretNameOf(scalarOf(v, "secret")); ref != "" {
			r.release = append(r.release, ref)
		}
		seq.Content = append(seq.Content[:at], seq.Content[at+1:]...)
	case OpRename:
		if e.NewName == "" {
			return invalidEdit("newName is required")
		}
		setScalar(v, "name", e.NewName)
	case OpLock:
		if scalarOf(v, "secret") != "" {
			return nil
		}
		val := scalarOf(v, "value")
		name := r.newSecretName(stem, e.Name)
		if val != "" {
			r.put(name, val)
		}
		removeKey(v, "value")
		// A shared secret needs allowed hosts the lock cannot invent; the user shares it again.
		removeKey(v, "agent")
		removeKey(v, "agentHosts")
		return setValueKey(v, "secret", "${secret:"+name+"}")
	case OpUnlock:
		if !e.Confirmed {
			return ErrUnconfirmed
		}
		ref := SecretNameOf(scalarOf(v, "secret"))
		if scalarOf(v, "secret") == "" {
			return nil
		}
		val := ""
		if ref != "" {
			got, err := r.s.o.Secrets.Reveal(ref)
			if err != nil && !errors.Is(err, configfs.ErrSecretMissing) {
				return err
			}
			val = got
			r.release = append(r.release, ref)
		}
		removeKey(v, "secret")
		// A plain shared value reaches every host; the user shares it again on purpose.
		removeKey(v, "agent")
		removeKey(v, "agentHosts")
		if val != "" {
			return setValueKey(v, "value", val)
		}
	case OpDescribe:
		if e.Description == nil {
			return invalidEdit("description is required")
		}
		if *e.Description == "" {
			removeKey(v, "description")
			return nil
		}
		return setValueKey(v, "description", *e.Description)
	case OpShare:
		if kind != kindEnvironment {
			return invalidEdit("only an environment variable can be shared with agents")
		}
		if e.Agent == nil {
			return invalidEdit("agent is required")
		}
		if !*e.Agent {
			removeKey(v, "agent")
			removeKey(v, "agentHosts")
			return nil
		}
		if err := setValueKey(v, "agent", true); err != nil {
			return err
		}
		if len(e.AgentHosts) > 0 {
			return setValueKey(v, "agentHosts", e.AgentHosts)
		}
		if scalarOf(v, "secret") == "" {
			removeKey(v, "agentHosts")
		}
	case OpMove:
		seq.Content = append(seq.Content[:at], seq.Content[at+1:]...)
		to := len(seq.Content)
		if e.Before != "" {
			if _, i := findVariable(seq, e.Before); i >= 0 {
				to = i
			}
		}
		seq.Content = append(seq.Content[:to], append([]*yaml.Node{v}, seq.Content[to:]...)...)
	default:
		return invalidEdit("op %q", e.Op)
	}
	return nil
}

func (r *editRun) opSetExisting(v *yaml.Node, e VariableEdit) error {
	ref := scalarOf(v, "secret")
	switch {
	case ref != "":
		if e.Value != nil {
			return fmt.Errorf("%w: %s is a secret", ErrVariableKind, e.Name)
		}
		name := SecretNameOf(ref)
		if name == "" {
			return fmt.Errorf("%w: %s has no secret name", ErrVariableKind, e.Name)
		}
		r.put(name, *e.SecretValue)
	default:
		if e.SecretValue != nil {
			return fmt.Errorf("%w: %s is not a secret", ErrVariableKind, e.Name)
		}
		if *e.Value == "" {
			removeKey(v, "value")
			return nil
		}
		return setValueKey(v, "value", *e.Value)
	}
	return nil
}

func (r *editRun) opSet(seq, v *yaml.Node, stem string, e VariableEdit) error {
	if e.Value != nil && e.SecretValue != nil {
		return invalidEdit("a variable has a value or a secret value, not both")
	}
	if e.SecretValue != nil && (*e.SecretValue == "" || strings.ContainsRune(*e.SecretValue, 0)) {
		return invalidEdit("a secret value is not empty and holds no NUL")
	}
	if e.Value == nil && e.SecretValue == nil {
		empty := ""
		e.Value = &empty
	}
	if v != nil {
		return r.opSetExisting(v, e)
	}
	nv := Variable{Name: e.Name}
	if e.SecretValue != nil {
		name := r.newSecretName(stem, e.Name)
		r.put(name, *e.SecretValue)
		nv.Secret = "${secret:" + name + "}"
	} else {
		nv.Value = *e.Value
	}
	node, err := valueNode(nv)
	if err != nil {
		return err
	}
	seq.Style = 0
	seq.Content = append(seq.Content, node)
	return nil
}

// UnsetSecrets lists the names of the variables of one file whose secret has no saved value.
func (s *Store) UnsetSecrets(file string) ([]string, error) {
	kind, err := kindOf(file)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var vars []Variable
	if kind == kindCollection {
		e, ok := s.collections.Get(file)
		if !ok || e.Value == nil {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, file)
		}
		vars = e.Value.Variables
	} else {
		e, ok := s.environments.Get(file)
		if !ok || e.Value == nil {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, file)
		}
		vars = e.Value.Variables
	}
	saved, err := s.o.Secrets.Names()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, n := range saved {
		set[n] = true
	}
	out := []string{}
	for _, v := range vars {
		if v.Secret != "" && !set[SecretNameOf(v.Secret)] {
			out = append(out, v.Name)
		}
	}
	return out, nil
}
