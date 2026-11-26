package ade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/kirathecat/kira-studio/internal/ids"
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/agentmcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// details.go: Details content on disk (P643). Notes, handovers and links live in files under
// <home>/tasks/<taskId>/; SQLite keeps the cache and the state. A UI action changes its rows as
// before and then mirrors them to files (mirror); an agent edit reaches the rows through submit
// (detailshandlers.go). The mirror never overwrites a file whose content nobody settled.

// detailsFiles is the file layer of a board: the submit service, its cache writer and the open
// approvals. Nil on a board with no files root.
type detailsFiles struct {
	b     *TaskBoard
	svc   *taskfiles.Service
	blobs *store.Blobs
	idx   taskfiles.Index
	// proofIndex lists the task's proofs for the README; nil when proofs are off.
	proofIndex func(taskID string) []taskfiles.ReadmeProof
	cache      repos.DetailsCache
	root       string

	mu       sync.Mutex
	pending  map[string]map[string]detailsPending // task id -> path -> open item
	lastSync map[string]time.Time                 // session key -> launch or last accepted submit
	// fileErrs holds the open problems of agent writes, by task and path: state.yaml fileErrors.
	fileErrs map[string]map[string]fileError

	// disk is each task's last DiskChanges answer, served while a task op holds the task lock. A write
	// that changes what DiskChanges reports drops it (dropDisk); diskGen keeps a computation that raced
	// the write from storing its stale result.
	diskMu  sync.Mutex
	disk    map[string][]adewire.DiskChange
	diskGen map[string]uint64
}

// fileError is the last invalid or rejected result of an agent write of one path.
type fileError struct {
	line int
	msg  string
}

// detailsPending is one file of an open details.edit approval.
type detailsPending struct {
	at   int64 // unix ms the approval opened
	item taskfiles.PendingItem
	by   string
}

func (b *TaskBoard) initDetails() {
	d := b.deps
	if d.FilesDir == "" || d.TaskFiles == nil || d.Tasks == nil {
		return
	}
	df := &detailsFiles{
		b: b, idx: taskfiles.RepoIndex(d.TaskFiles), cache: repos.DetailsCache{Dialect: d.Tasks.Dialect}, root: d.FilesDir,
		pending: map[string]map[string]detailsPending{}, lastSync: map[string]time.Time{}, fileErrs: map[string]map[string]fileError{},
		disk: map[string][]adewire.DiskChange{}, diskGen: map[string]uint64{},
	}
	h := &detailsHandlers{b: b, df: df}
	cfg := taskfiles.Config{
		Index: df.idx,
		Open:  df.open,
		Handlers: map[taskfiles.Kind]taskfiles.Handler{
			taskfiles.KindNote: h, taskfiles.KindDraft: h, taskfiles.KindHandover: h, taskfiles.KindLinks: h, taskfiles.KindReadme: h,
			taskfiles.KindTask: h, taskfiles.KindState: h,
		},
		Events: detailsEvents{b},
		Now:    d.Now,
	}
	if d.Proofs != nil && d.Proofs.Blobs() != nil {
		cfg.Proof = &taskfiles.ProofConfig{Cache: repos.ProofCache{Dialect: d.Tasks.Dialect}, Blobs: d.Proofs.Blobs(), NewID: ids.NewID}
		d.Proofs.SetOnEditResolved(b.proofEditResolved)
		d.Proofs.SetOnDelete(b.proofDelete)
		df.blobs = d.Proofs.Blobs()
	}
	df.proofIndex = b.proofIndex
	df.svc = taskfiles.New(cfg)
	b.files = df
	b.sweepBlobs()
}

// blobGrace is how old (file mtimes are wall time, so the cutoff ignores the board clock) an unreferenced blob must be before a sweep takes it: a submit stores its blobs
// before it commits the rows that point at them.
const blobGrace = time.Hour

// sweepBlobs deletes the blobs no task file or proof step references, in the background.
func (b *TaskBoard) sweepBlobs() {
	df := b.files
	if df == nil || df.blobs == nil || b.deps.TaskFiles == nil {
		return
	}
	b.goTracked(func() { sweepBlobs(df.blobs, b.deps.TaskFiles, time.Now().Add(-blobGrace)) })
}

func sweepBlobs(blobs *store.Blobs, files *repos.TaskFilesRepo, cutoff time.Time) {
	live, err := files.LiveBlobKeys()
	if err != nil {
		slog.Warn("ade: blob sweep: read live keys", "scope", "ade", "err", err)
		return
	}
	n, err := blobs.Sweep(live, cutoff)
	if err != nil {
		slog.Warn("ade: blob sweep", "scope", "ade", "removed", n, "err", err)
		return
	}
	if n > 0 {
		slog.Info("ade: blob sweep", "scope", "ade", "removed", n)
	}
}

func (df *detailsFiles) open(taskID string) (store.Store, error) {
	return store.NewLocal(filepath.Join(df.root, pathSegment(taskID)))
}

// detailsSession is who writes a submit.
type detailsSession struct {
	key, run string
	role     taskfiles.Role
}

type detailsCtxKey struct{}

type taskCtxKey struct{}

// taskCall is the task as a handler call sees it: loaded, with its current step, before the
// service opens its index transaction. Handlers run inside that transaction on the single DB
// connection, so they must not load either themselves.
type taskCall struct {
	tc   *taskCtx
	step string
}

func (b *TaskBoard) taskCallOf(tc *taskCtx) taskCall {
	return taskCall{tc: tc, step: b.currentStepID(tc)}
}

func withTaskCall(ctx context.Context, c taskCall) context.Context {
	return context.WithValue(ctx, taskCtxKey{}, c)
}

// ctxWithTask loads the task before a service call opens its transaction.
func (b *TaskBoard) ctxWithTask(ctx context.Context, taskID string) context.Context {
	if tc, err := b.loadTaskCtx(taskID); err == nil {
		return withTaskCall(ctx, b.taskCallOf(tc))
	}
	return ctx
}

func (h *detailsHandlers) taskOf(ctx context.Context, taskID string) (taskCall, error) {
	if c, ok := ctx.Value(taskCtxKey{}).(taskCall); ok && c.tc.task.ID == taskID {
		return c, nil
	}
	return taskCall{}, fmt.Errorf("ade: task %s not loaded for the files call", taskID)
}

func withDetailsSession(ctx context.Context, s detailsSession) context.Context {
	return context.WithValue(ctx, detailsCtxKey{}, s)
}

func detailsSessionOf(ctx context.Context) detailsSession {
	s, _ := ctx.Value(detailsCtxKey{}).(detailsSession)
	if s.role == "" {
		s.role = taskfiles.RoleKira
	}
	return s
}

var slugBad = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// fileSlug is a step id as a file name part: the characters the layout allows.
func fileSlug(id string) string {
	s := strings.Trim(slugBad.ReplaceAllString(id, "-"), "-_")
	if s == "" {
		return "step"
	}
	return s
}

func draftRel(stepID string) string { return "handovers/draft-" + fileSlug(stepID) + ".md" }

func noteRel(h model.AdeHandover) string {
	return "notes/" + time.UnixMilli(h.CreatedAt).UTC().Format("20060102T150405") + "-" + fileSlug(h.ID) + ".md"
}

var handedOrdinal = regexp.MustCompile(`^handovers/([0-9]{4})-`)

func handedRel(n int, h model.AdeHandover) string {
	from := cmpNonEmpty(fileSlugOrEmpty(h.FromStepID), "task")
	to := cmpNonEmpty(fileSlugOrEmpty(h.ToStepID), "done")
	return fmt.Sprintf("handovers/%04d-%s-to-%s.md", n, from, to)
}

func fileSlugOrEmpty(id string) string {
	if id == "" {
		return ""
	}
	return fileSlug(id)
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// wantFile is one file the cache says should exist.
type wantFile struct {
	rel  string
	data []byte
	// rowID is the handover row whose rel_path the write records; "" for links and README.
	rowID string
	kind  taskfiles.Kind
	// force overwrites a file that holds an unsettled change.
	force bool
	// renameFrom is the draft file a stage move turned into this handed file.
	renameFrom string
}

// mirrorOpts tune one mirror pass.
type mirrorOpts struct {
	// by is the writer recorded on the files it writes: "user" for a UI action, else "kira".
	by string
	// force lists handover row ids whose files a user action rewrites even over an unsettled edit.
	force []string
	// links forces links.yaml the same way.
	links bool
	// drop removes the file of a vanished row even over an unsettled edit: the user deleted it.
	drop bool
}

var userMirror = mirrorOpts{by: string(taskfiles.RoleUser)}

// userEdit is the mirror after a UI edit or delete of one handover row or the links.
func userEdit(links bool, rowIDs ...string) mirrorOpts {
	return mirrorOpts{by: string(taskfiles.RoleUser), force: rowIDs, links: links, drop: true}
}

// mirrorTask writes the task's cached notes, handovers and links to their files. The caller holds the
// task lock when one is held for the change that led here. A failure is logged: the cache stays
// right and the next pass writes the file.
func (b *TaskBoard) mirrorTask(taskID string, o mirrorOpts) {
	if b.files == nil {
		return
	}
	if err := b.files.mirror(b, taskID, o); err != nil {
		slog.Warn("ade: mirror task files", "scope", "ade", "task", taskID, "err", err)
	}
	b.markStateDirty(taskID, false)
}

func (df *detailsFiles) mirror(b *TaskBoard, taskID string, o mirrorOpts) error {
	defer df.dropDisk(taskID)
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return nil // archived tasks keep the files they have
	}
	call := b.taskCallOf(tc)
	ctx := withTaskCall(b.ctx, call)
	if o.by == "" {
		o.by = string(taskfiles.RoleKira)
	}
	want, err := df.wanted(b, call, o)
	if err != nil {
		return err
	}
	st, err := df.open(taskID)
	if err != nil {
		return err
	}
	rows, err := df.idx.Files(taskID)
	if err != nil {
		return err
	}
	byRel := make(map[string]model.TaskFile, len(rows))
	for _, r := range rows {
		byRel[r.RelPath] = r
	}
	var errs []error
	wanted := make(map[string]bool, len(want))
	for _, w := range want {
		wanted[w.rel] = true
		if err := df.writeWanted(ctx, st, taskID, w, byRel[w.rel], o.by); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", w.rel, err))
		}
	}
	for _, r := range rows {
		if wanted[r.RelPath] || !mirroredKind(r.Kind) || r.Review != model.TaskFileAccepted {
			continue
		}
		if err := df.dropStale(ctx, st, taskID, r, o.drop); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.RelPath, err))
		}
	}
	return errors.Join(errs...)
}

func mirroredKind(k string) bool {
	switch taskfiles.Kind(k) {
	case taskfiles.KindNote, taskfiles.KindDraft, taskfiles.KindHandover, taskfiles.KindLinks:
		return true
	}
	return false
}

// wanted builds every file the cache of the task calls for. Rows with no file yet get their path
// here: notes by creation time, handed rows by the next ordinal.
func (df *detailsFiles) wanted(b *TaskBoard, call taskCall, o mirrorOpts) ([]wantFile, error) {
	tc := call.tc
	taskID := tc.task.ID
	rows, err := b.deps.Tasks.Handovers(taskID)
	if err != nil {
		return nil, err
	}
	force := map[string]bool{}
	for _, id := range o.force {
		force[id] = true
	}
	out := planHandoverFiles(rows, force)
	links, err := df.currentLinks(b, taskID)
	if err != nil {
		return nil, err
	}
	if !links.empty() {
		out = append(out, wantFile{rel: taskfiles.LinksFile, kind: taskfiles.KindLinks, data: renderLinks(links), force: o.links})
	}
	readme := df.readmeText(tc, call.step)
	out = append(out,
		wantFile{rel: taskfiles.ReadmeFile, kind: taskfiles.KindReadme, data: []byte(readme), force: true},
		wantFile{rel: taskfiles.TaskFile, kind: taskfiles.KindTask, data: df.taskYAML(tc), force: true})
	return out, nil
}

// writeWanted writes one file when the cache and the accepted copy differ and the disk holds nothing
// unsettled.
func (df *detailsFiles) writeWanted(ctx context.Context, st store.Store, taskID string, w wantFile, row model.TaskFile, by string) error {
	if w.renameFrom != "" {
		if err := df.svc.Delete(ctx, taskID, w.renameFrom, by, nil); err != nil {
			return err
		}
	}
	wantSum := sha(w.data)
	hasRow := row.TaskID != ""
	info, statErr := st.Stat(ctx, w.rel)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, store.ErrNotExist) {
		return statErr
	}
	if exists && hasRow && row.Review == model.TaskFileAccepted && row.Size == info.Size && row.Version == string(info.Version) && row.AcceptedSHA256 == wantSum {
		return df.recordRel(ctx, taskID, w)
	}
	if exists {
		raw, _, err := st.Read(ctx, w.rel, taskfiles.MaxBytes(w.kind, w.rel))
		if err != nil && !errors.Is(err, store.ErrTooLarge) {
			return err
		}
		diskSum := sha(raw)
		switch {
		case err == nil && diskSum == wantSum && hasRow && row.AcceptedSHA256 == wantSum:
			return df.recordRel(ctx, taskID, w)
		case err == nil && diskSum == wantSum:
			// settled by content (migration): fall through to write, which records the index row
		case w.kind == taskfiles.KindLinks && err == nil && df.sameLinks(raw, w.data):
			return nil
		case !w.force && !(hasRow && err == nil && row.AcceptedSHA256 == diskSum):
			return nil // an edit nobody settled: the disk banner shows it
		}
	}
	return df.svc.Write(ctx, taskID, w.rel, w.data, by, func(tx taskfiles.Tx) error {
		if w.rowID == "" {
			return nil
		}
		return df.cache.SetHandoverRel(tx.Querier(), w.rowID, w.rel)
	})
}

// recordRel stores the file path on a handover row written before it had one.
func (df *detailsFiles) recordRel(_ context.Context, taskID string, w wantFile) error {
	if w.rowID == "" {
		return nil
	}
	return df.idx.InTx(func(tx taskfiles.Tx) error {
		h, err := df.cache.HandoverByRel(tx.Querier(), taskID, w.rel)
		if err != nil || h != nil {
			return err
		}
		return df.cache.SetHandoverRel(tx.Querier(), w.rowID, w.rel)
	})
}

// dropStale removes the file of a row that is gone, unless the disk holds an unsettled edit of it.
func (df *detailsFiles) dropStale(ctx context.Context, st store.Store, taskID string, r model.TaskFile, force bool) error {
	raw, _, err := st.Read(ctx, r.RelPath, taskfiles.MaxBytes(taskfiles.Kind(r.Kind), r.RelPath))
	switch {
	case errors.Is(err, store.ErrNotExist):
	case err != nil:
		return err
	case !force && sha(raw) != r.AcceptedSHA256:
		return nil
	}
	return df.svc.Delete(ctx, taskID, r.RelPath, string(taskfiles.RoleKira), nil)
}

// --- links ------------------------------------------------------------------------------------

type linkYAML struct {
	Kind  string `yaml:"kind"`
	Title string `yaml:"title,omitempty"`
	URL   string `yaml:"url,omitempty"`
	Ref   string `yaml:"ref,omitempty"`
}

type linksFile struct {
	Jira *struct {
		Key string `yaml:"key"`
		URL string `yaml:"url,omitempty"`
	} `yaml:"jira"`
	PR    string     `yaml:"pr"`
	Links []linkYAML `yaml:"links"`
}

// linksDoc is taskfiles.Links with the helper the mirror needs.
type linksDoc struct{ taskfiles.Links }

func (l linksDoc) empty() bool {
	return l.JiraKey == "" && l.JiraURL == "" && l.PR == "" && len(l.Entries) == 0
}

func normLinkURL(u string) string { return strings.TrimRight(strings.TrimSpace(u), "/") }

func entryOfArtifact(a model.AdeTaskArtifact) taskfiles.LinkEntry {
	return taskfiles.LinkEntry{Kind: a.Kind, Title: a.Title, URL: normLinkURL(a.URL), Ref: a.Ref}
}

// currentLinks is the task's links as the cache holds them: the slots and the stored rows.
func (df *detailsFiles) currentLinks(b *TaskBoard, taskID string) (linksDoc, error) {
	var out linksDoc
	err := df.idx.InTx(func(tx taskfiles.Tx) error {
		var err error
		out, err = df.linksIn(tx, taskID)
		return err
	})
	return out, err
}

func (df *detailsFiles) linksIn(tx taskfiles.Tx, taskID string) (linksDoc, error) {
	slots, err := df.cache.Slots(tx.Querier(), taskID)
	if err != nil {
		return linksDoc{}, err
	}
	arts, err := df.cache.Artifacts(tx.Querier(), taskID)
	if err != nil {
		return linksDoc{}, err
	}
	out := linksDoc{taskfiles.Links{JiraKey: slots.JiraKey, JiraURL: normLinkURL(slots.JiraURL), PR: normLinkURL(slots.HostURL)}}
	for _, a := range arts {
		out.Entries = append(out.Entries, entryOfArtifact(a))
	}
	return out, nil
}

func (df *detailsFiles) sameLinks(disk, want []byte) bool {
	id := func(a agentmcp.ArtifactInput) agentmcp.ArtifactInput { return a }
	a, err1 := parseLinks(disk, id)
	b, err2 := parseLinks(want, id)
	if err1 != nil || err2 != nil {
		return false
	}
	return len(taskfiles.DiffLinks(a.Links, b.Links)) == 0
}

// renderLinks writes links.yaml: the header, the slots, one flow-style line per link.
func renderLinks(l linksDoc) []byte {
	var sb strings.Builder
	sb.WriteString(taskfiles.Header(taskfiles.KindLinks))
	if l.JiraKey != "" || l.JiraURL != "" {
		node := yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		node.Content = append(node.Content, scalar("key"), scalar(l.JiraKey))
		if l.JiraURL != "" {
			node.Content = append(node.Content, scalar("url"), scalar(l.JiraURL))
		}
		sb.WriteString("jira: " + marshalNode(&node) + "\n")
	}
	if l.PR != "" {
		sb.WriteString("pr: " + marshalNode(scalar(l.PR)) + "\n")
	}
	if len(l.Entries) > 0 {
		sb.WriteString("links:\n")
		for _, e := range l.Entries {
			node := yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
			add := func(k, v string) {
				if v != "" {
					node.Content = append(node.Content, scalar(k), scalar(v))
				}
			}
			add("kind", e.Kind)
			add("title", e.Title)
			add("url", e.URL)
			add("ref", e.Ref)
			sb.WriteString("  - " + marshalNode(&node) + "\n")
		}
	}
	return []byte(sb.String())
}

func scalar(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: s} }

func marshalNode(n *yaml.Node) string {
	out, err := yaml.Marshal(n)
	if err != nil {
		panic("ade: marshal links node: " + err.Error())
	}
	return strings.TrimRight(string(out), "\n")
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

// parseLinks reads links.yaml: strict keys, the artifact rules of every entry.
func parseLinks(raw []byte, norm func(agentmcp.ArtifactInput) agentmcp.ArtifactInput) (linksDoc, error) {
	var f linksFile
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		pe := &taskfiles.ParseError{Msg: strings.TrimPrefix(err.Error(), "yaml: ")}
		if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
			pe.Line, _ = strconv.Atoi(m[1])
		}
		return linksDoc{}, pe
	}
	out := linksDoc{}
	if f.Jira != nil {
		out.JiraKey, out.JiraURL = strings.TrimSpace(f.Jira.Key), normLinkURL(f.Jira.URL)
	}
	out.PR = normLinkURL(f.PR)
	if out.PR != "" && !strings.HasPrefix(out.PR, "https://") && !strings.HasPrefix(out.PR, "http://") {
		return linksDoc{}, &taskfiles.ParseError{Msg: "pr must be an http or https address"}
	}
	seen := map[string]bool{}
	for i, e := range f.Links {
		clean, msg := agentmcp.CleanArtifact(agentmcp.ArtifactInput{Kind: e.Kind, Title: e.Title, URL: e.URL, Ref: e.Ref})
		if msg == "" {
			clean = norm(clean)
		}
		if msg != "" {
			return linksDoc{}, &taskfiles.ParseError{Msg: fmt.Sprintf("links[%d]: %s", i, msg)}
		}
		entry := taskfiles.LinkEntry{Kind: clean.Kind, Title: clean.Title, URL: normLinkURL(clean.URL), Ref: clean.Ref}
		if seen[entry.Key()] {
			return linksDoc{}, &taskfiles.ParseError{Msg: fmt.Sprintf("links[%d]: the same link is listed twice", i)}
		}
		seen[entry.Key()] = true
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}

// planHandoverFiles names the file of every note, draft and handed row: a row with a path keeps it,
// a handed row gets the next ordinal, and a handed row still on its draft path is renamed.
func planHandoverFiles(rows []model.AdeHandover, force map[string]bool) []wantFile {
	// oldest first: ordinals follow the handed order
	rows = slices.Clone(rows)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt < rows[j].CreatedAt })
	next := 1
	for _, r := range rows {
		if m := handedOrdinal.FindStringSubmatch(r.RelPath); m != nil {
			if n, _ := strconv.Atoi(m[1]); n >= next {
				next = n + 1
			}
		}
	}
	var out []wantFile
	for _, r := range rows {
		if r.Source == model.AdeHandoverSourceChild {
			continue // a child's finish stays a row: it derives from the child
		}
		w := wantFile{rowID: r.ID, data: []byte(r.Body), force: force[r.ID]}
		switch {
		case r.State == model.AdeHandoverNote:
			w.rel, w.kind = cmpNonEmpty(r.RelPath, noteRel(r)), taskfiles.KindNote
		case r.State == model.AdeHandoverDraft:
			w.rel, w.kind = cmpNonEmpty(r.RelPath, draftRel(r.FromStepID)), taskfiles.KindDraft
		case r.State == model.AdeHandoverHanded:
			rel := r.RelPath
			if rel == "" || strings.HasPrefix(rel, "handovers/draft-") {
				w.renameFrom = rel
				rel = handedRel(next, r)
				next++
				w.force = true
			}
			w.rel, w.kind = rel, taskfiles.KindHandover
		default:
			continue
		}
		out = append(out, w)
	}
	return out
}
