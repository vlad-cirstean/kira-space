package taskfiles

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// PartProofs is the migration marker of proof sessions.
const PartProofs = "proofs"

// ProofSource lists the legacy proofs of a task: sessions linked before proofs moved to files.
type ProofSource interface {
	LegacyProofs(taskID string) ([]repos.LegacyProof, error)
}

// ProofExporter moves the proof sessions of a task to proofs/<slug>/ folders. A session becomes one
// folder; step ids are s<ordinal>. A pending revision is what lands on disk, with its review still
// pending, and the current revision stays in the blob store as the accepted copy.
type ProofExporter struct {
	Source ProofSource
	Cache  repos.ProofCache
	Blobs  *store.Blobs
	Now    func() time.Time

	mu    sync.Mutex
	plans map[string][]*proofPlan
}

type proofPlan struct {
	p    repos.LegacyProof
	slug string
	cur  migUnit
	pend *migUnit
}

// migUnit is one revision rendered as a folder.
type migUnit struct {
	files map[string][]byte
	doc   proofDoc
}

// disk is the unit that goes on disk.
func (pl *proofPlan) disk() migUnit {
	if pl.pend != nil {
		return *pl.pend
	}
	return pl.cur
}

func (e *ProofExporter) Part() string { return PartProofs }

func (e *ProofExporter) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *ProofExporter) Export(ctx context.Context, taskID string, st store.Store) (Manifest, error) {
	list, err := e.Source.LegacyProofs(taskID)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{Counts: map[string]int{"proofs": len(list)}}
	taken := map[string]bool{}
	var plans []*proofPlan
	for _, p := range list {
		if p.Session.CurrentRev == nil || len(p.Cur) == 0 {
			return m, fmt.Errorf("proof %s has no current revision", p.Session.ID)
		}
		pl := &proofPlan{p: p, slug: uniqueSlug(p.Session.Name, taken)}
		curIDs := make([]string, len(p.Cur))
		for i := range p.Cur {
			curIDs[i] = fmt.Sprintf("s%d", p.Cur[i].Ordinal)
		}
		if pl.cur, err = buildUnit(p.Session.Name, p.Session.Summary, p.Cur, curIDs); err != nil {
			return m, fmt.Errorf("proof %s: %w", p.Session.ID, err)
		}
		if p.Session.PendingRev != nil && len(p.Pend) > 0 {
			u, err := buildUnit(p.Session.Name, p.Session.Summary, p.Pend, pendingIDs(p.Cur, curIDs, p.Pend))
			if err != nil {
				return m, fmt.Errorf("proof %s (pending edit): %w", p.Session.ID, err)
			}
			pl.pend = &u
		}
		for _, u := range []*migUnit{&pl.cur, pl.pend} {
			if u == nil {
				continue
			}
			for _, raw := range u.files { // blobs before the rows that name them
				if _, err := e.Blobs.Put(raw); err != nil {
					return m, err
				}
			}
		}
		for name, raw := range pl.disk().files {
			rel := proofFolder(pl.slug) + name
			if _, err := st.WriteAtomic(ctx, rel, raw, nil); err != nil {
				return m, err
			}
			m.Paths = append(m.Paths, rel)
		}
		plans = append(plans, pl)
	}
	sort.Strings(m.Paths)
	e.mu.Lock()
	if e.plans == nil {
		e.plans = map[string][]*proofPlan{}
	}
	e.plans[taskID] = plans
	e.mu.Unlock()
	return m, nil
}

// Verify re-reads the staged folders, parses them like a submit and compares them to the rows.
func (e *ProofExporter) Verify(ctx context.Context, taskID string, st store.Store, m Manifest) error {
	e.mu.Lock()
	plans := e.plans[taskID]
	e.mu.Unlock()
	if len(plans) != m.Counts["proofs"] {
		return fmt.Errorf("%d proofs planned, %d counted", len(plans), m.Counts["proofs"])
	}
	for _, pl := range plans {
		want := pl.disk()
		files := proofFiles{}
		for name := range want.files {
			raw, _, err := st.Read(ctx, proofFolder(pl.slug)+name, MaxBytesPerTask)
			if err != nil {
				return fmt.Errorf("%s%s: %w", proofFolder(pl.slug), name, err)
			}
			files[name] = raw
		}
		used := map[string]bool{}
		doc, probs := parseProofMD(files[ProofFile], files, used)
		if len(probs) > 0 {
			return fmt.Errorf("%s", probs[0].text(proofFolder(pl.slug)))
		}
		for name := range files {
			if name != ProofFile && !used[name] {
				return fmt.Errorf("%s%s is not used by proof.md", proofFolder(pl.slug), name)
			}
		}
		if !reflect.DeepEqual(doc.head(), want.doc.head()) {
			return fmt.Errorf("%s differs from the rows", proofFolder(pl.slug))
		}
	}
	return nil
}

// Record points the cache at the folders: slug, step ids, payloads (image metadata), the file rows and
// the pending state of an edit.
func (e *ProofExporter) Record(_ context.Context, tx Tx, taskID string, _ Manifest) error {
	e.mu.Lock()
	plans := e.plans[taskID]
	delete(e.plans, taskID)
	e.mu.Unlock()
	q := tx.Querier()
	now := e.now().UnixMilli()
	for _, pl := range plans {
		s := pl.p.Session
		if err := e.Cache.SetSlug(q, s.ID, pl.slug); err != nil {
			return err
		}
		for i, st := range pl.cur.doc.Steps {
			if err := e.Cache.SetStep(q, s.ID, *s.CurrentRev, i+1, st.ID, st.Payload); err != nil {
				return err
			}
		}
		if pl.pend != nil {
			for i, st := range pl.pend.doc.Steps {
				if err := e.Cache.SetStep(q, s.ID, *s.PendingRev, i+1, st.ID, st.Payload); err != nil {
					return err
				}
			}
			if err := e.Cache.SetRevisionText(q, s.ID, *s.PendingRev, s.Name, s.Summary); err != nil {
				return err
			}
		}
		disk := pl.disk()
		names := map[string]bool{}
		for n := range pl.cur.files {
			names[n] = true
		}
		for n := range disk.files {
			names[n] = true
		}
		for name := range names {
			rel := proofFolder(pl.slug) + name
			row := model.TaskFile{TaskID: taskID, RelPath: rel, Kind: string(Classify(rel).Kind), Review: model.TaskFileAccepted}
			acc, hadAcc := pl.cur.files[name]
			d, onDisk := disk.files[name]
			if hadAcc {
				row.AcceptedSHA256, row.AcceptedBy, row.AcceptedAt = sum(acc), string(RoleKira), now
			}
			if onDisk {
				row.SHA256, row.Size = sum(d), int64(len(d))
			}
			if pl.pend != nil {
				row.Review = model.TaskFilePending
			} else {
				row.AcceptedSHA256 = row.SHA256
			}
			if err := tx.PutFile(row); err != nil {
				return err
			}
		}
	}
	return nil
}

// uniqueSlug derives the folder name of a proof and claims it.
func uniqueSlug(name string, taken map[string]bool) string {
	base := SuggestSlug(name)
	slug := base
	for n := 2; taken[slug]; n++ {
		suffix := fmt.Sprintf("-%d", n)
		slug = strings.Trim(base[:min(len(base), MaxSlugLen-len(suffix))], "-") + suffix
	}
	taken[slug] = true
	return slug
}

// pendingIDs gives a pending step the id of the step it descends from, else s<ordinal>, made unique.
func pendingIDs(cur []repos.ProofStepRow, curIDs []string, pend []repos.ProofStepRow) []string {
	idOf := map[int]string{}
	for i, s := range cur {
		idOf[s.Ordinal] = curIDs[i]
	}
	used := map[string]bool{}
	out := make([]string, len(pend))
	for i, s := range pend {
		id := ""
		if s.OriginOrdinal != nil {
			id = idOf[*s.OriginOrdinal]
		}
		if id == "" || used[id] {
			id = fmt.Sprintf("s%d", s.Ordinal)
			for n := 2; used[id]; n++ {
				id = fmt.Sprintf("s%d-%d", s.Ordinal, n)
			}
		}
		used[id] = true
		out[i] = id
	}
	return out
}

// buildUnit renders a revision as proof.md and its side files, parses it back like a submit and checks
// the parse against the rows.
func buildUnit(name, summary string, steps []repos.ProofStepRow, ids []string) (migUnit, error) {
	files := proofFiles{}
	var list []*yaml.Node
	for i, s := range steps {
		n, err := renderStep(s, ids[i], files)
		if err != nil {
			return migUnit{}, fmt.Errorf("step %d: %w", s.Ordinal, err)
		}
		list = append(list, n)
	}
	root := &yaml.Node{Kind: yaml.MappingNode}
	root.Content = append(root.Content, str("name"), str(name), str("summary"), str(summary), str("steps"),
		&yaml.Node{Kind: yaml.SequenceNode, Content: list})
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return migUnit{}, err
	}
	if err := enc.Close(); err != nil {
		return migUnit{}, err
	}
	buf.WriteString("---\n")
	files[ProofFile] = buf.Bytes()
	used := map[string]bool{}
	doc, probs := parseProofMD(files[ProofFile], files, used)
	if len(probs) > 0 {
		return migUnit{}, fmt.Errorf("%s", probs[0].text(""))
	}
	if doc.Name != name || doc.Summary != summary || len(doc.Steps) != len(steps) {
		return migUnit{}, fmt.Errorf("name, summary or step count changed in the rendered file")
	}
	for i, s := range steps {
		if err := sameStep(s, doc.Steps[i], files); err != nil {
			return migUnit{}, fmt.Errorf("step %d: %w", s.Ordinal, err)
		}
	}
	return migUnit{files: files, doc: doc}, nil
}

func str(v string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
	if strings.Contains(v, "\n") {
		n.Style = yaml.LiteralStyle
	}
	return n
}

// renderStep renders one step and puts its side file, if any, into files.
func renderStep(s repos.ProofStepRow, id string, files proofFiles) (*yaml.Node, error) {
	m := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v *yaml.Node) { m.Content = append(m.Content, str(k), v) }
	add("id", str(id))
	add("description", str(strings.Join(strings.Fields(s.Description), " ")))
	add("view", str(s.View))
	if s.Query != "" {
		add("query", str(s.Query))
	}
	if s.Language != "" && s.Language != "text" {
		add("language", str(s.Language))
	}
	if s.Source != "" {
		add("source", str(s.Source))
	}
	switch s.View {
	case proof.ViewImage:
		var p struct {
			Base64 string `json:"base64"`
		}
		if err := json.Unmarshal(s.Payload, &p); err != nil || p.Base64 == "" {
			return nil, fmt.Errorf("image payload has no data")
		}
		raw, err := base64.StdEncoding.DecodeString(p.Base64)
		if err != nil {
			return nil, fmt.Errorf("image payload is not base64")
		}
		format := proof.SniffFormat(raw)
		if format == "" {
			return nil, fmt.Errorf("image payload is not PNG, JPEG or WebP")
		}
		name := id + "." + proof.ImageExt(format)
		files[name] = raw
		add("data", str(name))
	case proof.ViewDiagram:
		var p struct {
			Source string `json:"source"`
		}
		if err := json.Unmarshal(s.Payload, &p); err != nil {
			return nil, err
		}
		inline := str(p.Source)
		if len(p.Source)+16 > InlineStepMaxBytes {
			raw, _ := json.Marshal(map[string]string{"source": p.Source})
			files[id+".json"] = raw
			add("data", str(id+".json"))
			break
		}
		add("inline", inline)
	default:
		if len(s.Payload) > InlineStepMaxBytes {
			files[id+".json"] = append([]byte(nil), s.Payload...)
			add("data", str(id+".json"))
			break
		}
		var n yaml.Node
		if err := yaml.Unmarshal(s.Payload, &n); err != nil || len(n.Content) == 0 {
			return nil, fmt.Errorf("payload is not JSON")
		}
		unstyle(n.Content[0])
		add("inline", n.Content[0])
	}
	return m, nil
}

// unstyle turns a parsed JSON tree into block style YAML.
func unstyle(n *yaml.Node) {
	if n.Kind == yaml.ScalarNode {
		n.Style = 0
		if n.Tag == "!!str" && strings.Contains(n.Value, "\n") {
			n.Style = yaml.LiteralStyle
		}
		return
	}
	n.Style = 0
	for _, c := range n.Content {
		unstyle(c)
	}
}

// sameStep checks the parsed step carries the row's content.
func sameStep(row repos.ProofStepRow, got proofStep, files proofFiles) error {
	want := strings.Join(strings.Fields(row.Description), " ")
	lang := row.Language
	if lang == "" {
		lang = "text"
	}
	if got.Description != want || got.View != row.View || got.Query != row.Query || got.Language != lang || got.Source != row.Source {
		return fmt.Errorf("a field changed in the rendered file")
	}
	if row.View == proof.ViewImage {
		var p struct {
			Base64 string `json:"base64"`
		}
		_ = json.Unmarshal(row.Payload, &p)
		raw, _ := base64.StdEncoding.DecodeString(p.Base64)
		if !bytes.Equal(files[got.File], raw) {
			return fmt.Errorf("the image bytes changed")
		}
		return nil
	}
	var a, b any
	if err := json.Unmarshal(row.Payload, &a); err != nil {
		return err
	}
	if err := json.Unmarshal(got.Payload, &b); err != nil {
		return err
	}
	if !reflect.DeepEqual(a, b) {
		return fmt.Errorf("the data changed in the rendered file")
	}
	return nil
}
