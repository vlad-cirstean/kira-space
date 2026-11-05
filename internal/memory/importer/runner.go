package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/sync/semaphore"

	"github.com/kirathecat/kira-studio/internal/filelock"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
)

const (
	maxRetries       = 3
	maxFinalizeToken = 80000

	lockFileName = "import-runner.lock"
	logFileName  = "import-runner.log"

	defaultTick     = 500 * time.Millisecond
	defaultIdleExit = 30 * time.Second
	// autoStartChunks is the most chunks an agent-started job may run without the user's consent.
	autoStartChunks = 300
)

// ErrRunnerBusy is Run's answer when another runner holds the store's lock: a supported state.
var ErrRunnerBusy = errors.New("importer: another import runner holds the lock")

// RunnerOptions configure a Runner. Only Agent is required.
type RunnerOptions struct {
	Agent      Agent
	Extractors int                    // concurrent extract calls; 0 means 3
	Backoff    func() backoff.BackOff // per-item retry schedule; nil means exponential from 10 s
	OnChange   func()                 // runs after any state change; must not block
	Now        func() time.Time
	Caps       Caps          // zero means DefaultCaps
	Budget     Budget        // zero means DefaultBudget
	Tick       time.Duration // how often the runner re-reads job states; 0 means 500 ms
	IdleExit   time.Duration // no scanning or running job for this long ends the runner; 0 means 30 s
	Docs       DocApplier    // applies kira-memory docs; nil fails such files
}

func defaultBackoff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 10 * time.Second
	b.Multiplier = 2
	b.MaxInterval = 2 * time.Minute
	b.RandomizationFactor = 0.5
	b.MaxElapsedTime = 0
	return b
}

type armedJob struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Runner is the scheduler of bulk imports: scan, materialise, extract, finalize. Exactly one runs
// per memory.db, held by an exclusive flock; it follows the job rows the control side (Engine)
// writes by polling them, so a pause or cancel from another process takes effect within one tick.
// All state lives in memory.db, so a halted runner resumes from disk.
type Runner struct {
	st       *store
	ms       *memory.Store
	agent    Agent
	opts     RunnerOptions
	lockPath string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	busy   atomic.Int32 // in-flight goroutines of this runner
	wake   chan struct{}
	ready  chan struct{} // closed once the runner holds the lock and has recovered

	extractSem *semaphore.Weighted
	finalizing atomic.Bool

	mu        sync.Mutex
	armed     map[string]armedJob // running jobs' contexts
	scanning  map[string]bool     // scanning jobs this runner works on
	notBefore time.Time           // shared rate-limit cooldown
}

// NewRunner prepares a runner over ms. It takes no lock until Run.
func NewRunner(ms *memory.Store, opts RunnerOptions) (*Runner, error) {
	if opts.Agent == nil {
		return nil, errors.New("importer: agent required")
	}
	if opts.Extractors <= 0 {
		opts.Extractors = 3
	}
	if opts.Backoff == nil {
		opts.Backoff = defaultBackoff
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Caps == (Caps{}) {
		opts.Caps = DefaultCaps
	}
	if opts.Budget == (Budget{}) {
		opts.Budget = DefaultBudget
	}
	if opts.Tick <= 0 {
		opts.Tick = defaultTick
	}
	if opts.IdleExit <= 0 {
		opts.IdleExit = defaultIdleExit
	}
	db, err := ms.DB()
	if err != nil {
		return nil, err
	}
	return &Runner{
		ms: ms, st: &store{db: db, now: func() string { return kiratime.FormatISO(opts.Now()) }}, agent: opts.Agent, opts: opts,
		lockPath: lockPath(ms), wake: make(chan struct{}, 1), ready: make(chan struct{}),
		extractSem: semaphore.NewWeighted(int64(opts.Extractors)), armed: map[string]armedJob{}, scanning: map[string]bool{},
	}, nil
}

func lockPath(ms *memory.Store) string { return filepath.Join(filepath.Dir(ms.Path()), lockFileName) }

// Run takes the store's runner lock and works until ctx ends or the store has been idle for
// IdleExit. A runner taking the lock resets what a crashed predecessor left mid-flight (running
// jobs become paused; scanning jobs rescan) and never resumes a job by itself. It returns
// ErrRunnerBusy when another runner holds the lock.
func (r *Runner) Run(ctx context.Context) error {
	lock, ok, err := filelock.TryLock(r.lockPath)
	if err != nil {
		return fmt.Errorf("importer: runner lock: %w", err)
	}
	if !ok {
		return ErrRunnerBusy
	}
	defer func() {
		if lock != nil {
			_ = lock.Close()
		}
	}()
	if err := r.st.recoverInterrupted(); err != nil {
		return fmt.Errorf("importer: recover: %w", err)
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	close(r.ready)
	tick := time.NewTicker(r.opts.Tick)
	defer tick.Stop()
	var idleSince time.Time
	for {
		if _, err := os.Stat(filepath.Dir(r.lockPath)); err != nil {
			r.shutdown(false)
			return nil
		}
		active := r.sync()
		r.schedule()
		switch {
		case active || r.busy.Load() > 0:
			idleSince = time.Time{}
		case idleSince.IsZero():
			idleSince = time.Now()
		case time.Since(idleSince) >= r.opts.IdleExit:
			if relocked, done := r.tryExit(lock); done {
				r.shutdown(false)
				return nil
			} else {
				lock = relocked
			}
			idleSince = time.Time{}
		}
		select {
		case <-r.ctx.Done():
			r.shutdown(true)
			return nil
		case <-tick.C:
		case <-r.wake:
		}
	}
}

// tryExit releases the lock, then looks once more for work a control call wrote meanwhile: a job set
// running just before the lock was released finds no runner otherwise. It reports done when the
// runner should end; otherwise it returns the lock taken again.
func (r *Runner) tryExit(lock *os.File) (*os.File, bool) {
	_ = lock.Close()
	active, err := r.st.hasActiveJobs()
	if err == nil && !active {
		return nil, true
	}
	again, ok, lerr := filelock.TryLock(r.lockPath)
	if lerr != nil || !ok {
		return nil, true // another runner took over
	}
	return again, false
}

// shutdown stops all work and waits for it. With recover, running jobs end paused as interrupted and
// in-flight chunks go back to pending; an idle exit has nothing to recover.
func (r *Runner) shutdown(recover bool) {
	r.cancel()
	r.wg.Wait()
	if recover {
		if err := r.st.recoverInterrupted(); err != nil {
			slog.Error("memory import: recover", "scope", "memory", "err", err)
		}
	}
}

func (r *Runner) changed() {
	if r.opts.OnChange != nil {
		r.opts.OnChange()
	}
}

func (r *Runner) kick() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// spawn runs fn on a goroutine the runner waits for and counts as in flight.
func (r *Runner) spawn(fn func()) {
	r.wg.Add(1)
	r.busy.Add(1)
	go func() {
		defer r.wg.Done()
		defer r.busy.Add(-1)
		fn()
	}()
}

// arm gives a job the context its in-flight calls run under. Pause, cancel and finish disarm it,
// which kills those calls.
func (r *Runner) arm(jobID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.armed[jobID]; !ok {
		ctx, cancel := context.WithCancel(r.ctx)
		r.armed[jobID] = armedJob{ctx: ctx, cancel: cancel}
	}
}

func (r *Runner) disarm(jobID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.armed[jobID]; ok {
		a.cancel()
		delete(r.armed, jobID)
	}
}

func (r *Runner) jobCtx(jobID string) (context.Context, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.armed[jobID]
	return a.ctx, ok
}

// sync reads the job states the control side wrote: running jobs get a context, jobs that left
// running lose theirs (killing their calls), scanning jobs get a scan, finished jobs settle. It
// reports whether any job is scanning or running.
func (r *Runner) sync() bool {
	jobs, err := r.st.activeJobs()
	if err != nil {
		slog.Error("memory import: read jobs", "scope", "memory", "err", err)
		return true
	}
	running := map[string]bool{}
	for _, j := range jobs {
		switch j.State {
		case JobRunning:
			running[j.ID] = true
			r.arm(j.ID)
			if err := r.settle(j.ID); err != nil {
				slog.Error("memory import: settle", "scope", "memory", "err", err)
			}
		case JobScanning:
			r.startScan(j)
		}
	}
	r.mu.Lock()
	var stale []string
	for id := range r.armed {
		if !running[id] {
			stale = append(stale, id)
		}
	}
	r.mu.Unlock()
	for _, id := range stale {
		r.disarm(id)
	}
	return len(jobs) > 0
}

func (r *Runner) startScan(j activeJob) {
	r.mu.Lock()
	if r.scanning[j.ID] {
		r.mu.Unlock()
		return
	}
	r.scanning[j.ID] = true
	r.mu.Unlock()
	r.spawn(func() {
		defer func() {
			r.mu.Lock()
			delete(r.scanning, j.ID)
			r.mu.Unlock()
		}()
		r.scan(j.ID, j.Roots)
	})
}

func (r *Runner) scan(id string, paths []string) {
	defer r.changed()
	res, err := Scan(r.ctx, paths, r.opts.Caps)
	if err != nil {
		if r.ctx.Err() == nil {
			slog.Warn("memory import: scan failed", "scope", "memory", "err", err)
			_ = r.st.failScan(id, err.Error())
		}
		return
	}
	files, err := r.plan(id, res)
	if err == nil {
		err = r.st.scanned(id, res.Base, res.Truncated, res.Ignored, files, autoStartChunks)
	}
	if err == nil {
		r.kick()
	}
	if err != nil && r.ctx.Err() == nil {
		slog.Warn("memory import: scan failed", "scope", "memory", "err", err)
		_ = r.st.failScan(id, err.Error())
	}
}

// plan turns a scan into file rows: duplicates and already imported content are skipped with a
// reason, the rest are chunked once to size the job.
func (r *Runner) plan(jobID string, res ScanResult) ([]newFile, error) {
	files := make([]newFile, 0, len(res.Files)+len(res.Skipped))
	for _, sk := range res.Skipped {
		files = append(files, newFile{ID: ids.NewID(), Path: sk.Path, Rel: sk.Rel, Kind: orKind(sk.Kind), Size: sk.Size,
			State: FileSkipped, Reason: sk.Reason})
	}
	first := map[string]string{}
	for _, c := range res.Files {
		nf := newFile{ID: ids.NewID(), Path: c.Path, Rel: c.Rel, Kind: c.Kind, Size: c.Size, Hash: c.Hash, State: FileSkipped}
		if other, dup := first[c.Hash]; dup {
			nf.Reason = "same content as " + other
			files = append(files, nf)
			continue
		}
		first[c.Hash] = c.Rel
		at, done, err := r.st.doneHash(c.Hash, jobID)
		if err != nil {
			return nil, err
		}
		if done {
			nf.Reason = "unchanged since import on " + strings.SplitN(at, "T", 2)[0]
			files = append(files, nf)
			continue
		}
		text, _, reason, err := ReadText(c.Path, r.opts.Caps.MaxFileBytes)
		if err != nil || reason != "" {
			nf.Reason = orReason(reason, err)
			files = append(files, nf)
			continue
		}
		if doc, ok := parseMemdoc(c.Kind, text); ok {
			nf.Kind, nf.State = KindMemdoc, FilePending
			nf.DocOrigin, nf.DocScope, nf.DocProject = doc.Origin, doc.Scope, doc.Project
			files = append(files, nf)
			continue
		}
		chunks, title, err := chunkFile(c.Kind, text, c.Rel, r.opts.Budget)
		if err != nil {
			nf.State, nf.Reason = FileFailed, err.Error()
			files = append(files, nf)
			continue
		}
		nf.State, nf.Title, nf.Chunks = FilePending, title, len(chunks)
		for _, ch := range chunks {
			nf.Tokens += ch.Tokens
		}
		files = append(files, nf)
	}
	return files, nil
}

func orKind(k string) string {
	if k == "" {
		return KindText
	}
	return k
}

func orReason(reason string, err error) string {
	if reason != "" {
		return reason
	}
	return "unreadable: " + err.Error()
}

func chunkFile(kind, text, rel string, b Budget) ([]Chunk, string, error) {
	var chunks []Chunk
	var err error
	title := ""
	switch kind {
	case KindMarkdown:
		chunks, title = ChunkMarkdown(text, b)
	case KindJSON:
		chunks, err = ChunkJSON(text, b)
	case KindJSONL:
		chunks, err = ChunkJSONLines(text, b)
	default:
		chunks = ChunkText(text, b)
	}
	if err != nil {
		return nil, "", err
	}
	if title == "" {
		name := path.Base(filepath.ToSlash(rel))
		title = strings.TrimSuffix(name, path.Ext(name))
	}
	return chunks, title, nil
}

// schedule starts as much work as the slots allow: extract workers for pending chunks, then
// cross-check workers, then reading and chunking the next file, and the single finalize worker.
func (r *Runner) schedule() {
	for r.ctx.Err() == nil && r.extractSem.TryAcquire(1) {
		if !r.startSlotWork() {
			break
		}
	}
	r.startFinalize()
}

// startSlotWork spends the extract slot the caller holds. It reports whether more work may follow;
// when it returns false or starts nothing, the slot is released.
func (r *Runner) startSlotWork() bool {
	it, ok, err := r.st.claimChunk()
	if err != nil {
		r.extractSem.Release(1)
		slog.Error("memory import: claim chunk", "scope", "memory", "err", err)
		return false
	}
	if ok {
		ctx, armed := r.jobCtx(it.JobID)
		if !armed {
			_ = r.st.releaseChunk(it, usage{})
			r.extractSem.Release(1)
			return false
		}
		r.spawn(func() { r.runExtract(ctx, it) })
		return true
	}
	vit, ok, err := r.st.claimVerify()
	if err != nil {
		r.extractSem.Release(1)
		slog.Error("memory import: claim verify", "scope", "memory", "err", err)
		return false
	}
	if ok {
		ctx, armed := r.jobCtx(vit.JobID)
		if !armed {
			_ = r.st.endVerify(vit, usage{})
			r.extractSem.Release(1)
			return false
		}
		r.spawn(func() { r.runVerify(ctx, vit) })
		return true
	}
	r.extractSem.Release(1)
	progressed, err := r.materialiseNext()
	if err != nil {
		slog.Error("memory import: materialise", "scope", "memory", "err", err)
		return false
	}
	return progressed
}

func (r *Runner) startFinalize() {
	if r.ctx.Err() != nil || !r.finalizing.CompareAndSwap(false, true) {
		return
	}
	it, ok, err := r.st.claimFinalize()
	if err != nil || !ok {
		r.finalizing.Store(false)
		if err != nil {
			slog.Error("memory import: claim finalize", "scope", "memory", "err", err)
		}
		return
	}
	ctx, armed := r.jobCtx(it.JobID)
	if !armed {
		_ = r.st.releaseFinalize(it, usage{})
		r.finalizing.Store(false)
		return
	}
	r.spawn(func() { r.runFinalize(ctx, it) })
}

// materialiseNext reads, validates, hashes and chunks the next pending file. It reports whether it
// changed anything, so the scheduler asks again.
func (r *Runner) materialiseNext() (bool, error) {
	ref, ok, err := r.st.nextPendingFile()
	if err != nil || !ok {
		return false, err
	}
	defer r.changed()
	end := func(state, reason string) (bool, error) {
		err := r.st.endFile(ref.FileID, []string{FileExtracting}, state, reason)
		if err == nil {
			err = r.settle(ref.JobID)
		}
		return true, err
	}
	text, hash, skip, err := ReadText(ref.Path, r.opts.Caps.MaxFileBytes)
	switch {
	case err != nil:
		return end(FileFailed, "Cannot read file: "+err.Error())
	case skip != "":
		return end(FileSkipped, skip)
	}
	if at, done, err := r.st.doneHash(hash, ref.JobID); err != nil {
		return false, err
	} else if done {
		return end(FileSkipped, "unchanged since import on "+strings.SplitN(at, "T", 2)[0])
	}
	if isMemdoc(ref.Kind, text) || ref.Kind == KindMemdoc {
		doc, _, problems := memfmt.ParseDoc([]byte(text))
		if len(problems) > 0 {
			return end(FileFailed, docError(problems))
		}
		return true, r.st.storeDoc(ref.FileID, hash, doc.Title)
	}
	chunks, title, err := chunkFile(ref.Kind, text, ref.Rel, r.opts.Budget)
	if err != nil {
		return end(FileFailed, err.Error())
	}
	if len(chunks) == 0 {
		return end(FileSkipped, "empty")
	}
	return true, r.st.storeChunks(ref.FileID, hash, title, chunks)
}

// settle finishes a job whose files are all terminal.
func (r *Runner) settle(jobID string) error {
	done, err := r.st.finishJobIfIdle(jobID)
	if done {
		r.disarm(jobID)
	}
	return err
}

func isFatal(err error) bool {
	return errors.Is(err, harness.ErrNotFound) || errors.Is(err, harness.ErrAuth) ||
		errors.Is(err, harness.ErrOutdated) || errors.Is(err, harness.ErrUsageLimit)
}

// retry runs op with exponential backoff. Fatal errors, a spent budget and a cancelled context
// stop at once; a rate limit also holds every other worker back until its wait is over.
func (r *Runner) retry(ctx context.Context, op func() error) error {
	bo := backoff.WithContext(backoff.WithMaxRetries(r.opts.Backoff(), maxRetries), ctx)
	return backoff.RetryNotify(func() error {
		if err := r.waitCooldown(ctx); err != nil {
			return backoff.Permanent(err)
		}
		err := op()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil || isFatal(err) || errors.Is(err, harness.ErrBudget) {
			return backoff.Permanent(err)
		}
		return err
	}, bo, func(err error, next time.Duration) {
		if errors.Is(err, harness.ErrRateLimited) {
			r.cooldown(next)
		}
		slog.Warn("memory import: retrying", "scope", "memory", "err", err, "in", next)
	})
}

func (r *Runner) cooldown(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if until := time.Now().Add(d); until.After(r.notBefore) {
		r.notBefore = until
	}
}

func (r *Runner) waitCooldown(ctx context.Context) error {
	for {
		r.mu.Lock()
		wait := time.Until(r.notBefore)
		r.mu.Unlock()
		if wait <= 0 {
			return ctx.Err()
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (r *Runner) pauseFatal(jobID string, err error) {
	if paused, perr := r.st.pauseIfRunning(jobID, err.Error()); perr != nil {
		slog.Error("memory import: pause", "scope", "memory", "err", perr)
	} else if paused {
		r.disarm(jobID)
	}
}

func (r *Runner) runExtract(ctx context.Context, it chunkItem) {
	defer r.kick()
	defer r.extractSem.Release(1)
	defer r.changed()

	var u usage
	var out ExtractOutput
	tries := 0
	runErr := r.retry(ctx, func() error {
		tries++
		u.Calls++
		o, err := r.agent.Extract(ctx, ExtractInput{Path: it.Path, Title: it.Title, Index: it.Index, Count: it.Count,
			HeadingPath: it.HeadingPath, Context: it.Context, Text: it.Text})
		u.CostUSD += o.CostUSD
		if err == nil {
			out = o
		}
		return err
	})
	var err error
	switch {
	case runErr == nil:
		err = r.st.finishChunk(it, out.Facts, tries, u)
	case ctx.Err() != nil:
		err = r.st.releaseChunk(it, u)
	case isFatal(runErr):
		err = r.st.releaseChunk(it, u)
		r.pauseFatal(it.JobID, runErr)
	default:
		err = r.st.failChunk(it, boundedMessage(runErr), tries, u)
	}
	if err == nil {
		err = r.settle(it.JobID)
	}
	if err != nil {
		slog.Error("memory import: extract result", "scope", "memory", "err", err)
	}
}

func boundedMessage(err error) string {
	msg := strings.TrimSpace(err.Error())
	if r := []rune(msg); len(r) > 300 {
		msg = string(r[:300]) + "..."
	}
	return msg
}

// runVerify cross-checks a file's questionable facts against a copy of its source, one batch at a
// time, saving each batch's verdicts before the next so an interrupted run keeps them.
func (r *Runner) runVerify(ctx context.Context, it finalItem) {
	defer r.kick()
	defer r.extractSem.Release(1)
	defer r.changed()

	var u usage
	err := r.verifyBatches(ctx, it, &u)
	switch {
	case err == nil || ctx.Err() != nil:
		err = r.st.endVerify(it, u)
	case isFatal(err):
		cause := err
		err = r.st.endVerify(it, u)
		r.pauseFatal(it.JobID, cause)
	default:
		err = r.st.failVerify(it, boundedMessage(err), u)
	}
	if err == nil {
		err = r.settle(it.JobID)
	}
	if err != nil {
		slog.Error("memory import: verify result", "scope", "memory", "err", err)
	}
}

func (r *Runner) verifyBatches(ctx context.Context, it finalItem, u *usage) error {
	text, hash, skip, err := ReadText(it.Path, r.opts.Caps.MaxFileBytes)
	switch {
	case err != nil:
		return fmt.Errorf("cannot read the source: %w", err)
	case skip != "":
		return errors.New(skip)
	case hash != it.Hash:
		return errors.New("the file changed since it was read; retry the file")
	}
	for start := 0; start < len(it.Verify); start += verifyBatch {
		batch := it.Verify[start:min(start+verifyBatch, len(it.Verify))]
		var out VerifyOutput
		err := r.retry(ctx, func() error {
			u.Calls++
			o, err := r.agent.Verify(ctx, VerifyInput{Path: it.Rel, Title: it.Title, Source: []byte(text), Ext: path.Ext(it.Rel), Facts: batch})
			u.CostUSD += o.CostUSD
			if err == nil {
				out = o
			}
			return err
		})
		if err != nil {
			return err
		}
		if err := r.st.saveVerdicts(it, batchVerdicts(batch, out), *u); err != nil {
			return err
		}
		*u = usage{}
	}
	return nil
}

func (r *Runner) runFinalize(ctx context.Context, it finalItem) {
	defer r.kick()
	defer r.finalizing.Store(false)
	defer r.changed()

	var u usage
	var err error
	finish := func(out FinalizeOutput) error {
		err := r.st.finishFile(it, out, u)
		if err == nil {
			r.writeBack(r.ctx, it)
		}
		return err
	}
	switch {
	case it.Kind == KindMemdoc:
		err = r.finalizeDoc(ctx, it)
	case len(it.Facts) == 0:
		err = finish(FinalizeOutput{})
	case tooManyFacts(it.Facts):
		err = r.st.failFinalize(it, fmt.Sprintf("Too many facts (%d) for one pass. Split the file.", len(it.Facts)), u)
	default:
		var out FinalizeOutput
		runErr := r.retry(ctx, func() error {
			u.Calls++
			o, err := r.agent.Finalize(ctx, FinalizeInput{FileID: it.FileID, Path: it.Rel, Title: it.Title,
				ChunkCount: it.ChunkCount, Prov: it.Prov, Facts: it.Facts})
			u.CostUSD += o.CostUSD
			if err == nil {
				out = o
			}
			return err
		})
		switch {
		case runErr == nil:
			err = finish(out)
		case ctx.Err() != nil:
			err = r.st.releaseFinalize(it, u)
		case isFatal(runErr):
			err = r.st.releaseFinalize(it, u)
			r.pauseFatal(it.JobID, runErr)
		default:
			err = r.st.failFinalize(it, boundedMessage(runErr), u)
		}
	}
	if err == nil {
		err = r.settle(it.JobID)
	}
	if err != nil {
		slog.Error("memory import: finalize result", "scope", "memory", "err", err)
	}
}

// tooManyFacts guards the step-2 prompt size; a file over it fails instead of being truncated.
func tooManyFacts(facts []FinalFact) bool {
	raw, _ := json.Marshal(facts)
	return EstimateTokens(string(raw)) > maxFinalizeToken
}
