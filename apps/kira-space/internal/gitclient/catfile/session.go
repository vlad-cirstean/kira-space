// Package catfile is the paged history pipeline's blob/metadata reader (docs/HISTORY.md):
// two persistent `git cat-file` processes
// (`--batch-check` for metadata-only lookups, `--batch` for content), each driven one request at
// a time by writing a revision per line and reading its two-phase response. Its first production
// caller is the commit.detail/commit.fileDiff/blob-read work — RepoEntry
// owns a Session lazily (constructed on first use, torn down with the entry) so a connection that
// never needs it never pays for two extra processes.
package catfile

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
)

// DefaultMaxBlobBytes bounds how large a blob Read will actually fetch — a size discovered via
// --batch-check that exceeds this is reported as ErrTooLarge without ever asking --batch for the
// content, so a caller can never be handed a multi-gigabyte allocation by surprise.
const DefaultMaxBlobBytes = 10 << 20

// ErrTooLarge is Read's outcome for an object whose size (known from --batch-check, checked
// before any content read) exceeds the session's gate.
var ErrTooLarge = errors.New("catfile: blob exceeds the configured size gate")

// maxConsecutiveFailures bounds the persistent process's own lazy-restart circuit breaker: after
// this many consecutive failures (a spawn failure, or a response readHeader/readContent could not
// parse), further requests fail immediately with errCircuitOpen rather than spawning yet another
// doomed process — a git binary that is broken (wrong permissions, corrupt install) must
// not be retried forever.
const maxConsecutiveFailures = 3

// ErrInvalidRev rejects a rev the line protocol cannot frame (a newline would read as two
// requests and desync the stream); such revs use the one-shot path.
var ErrInvalidRev = errors.New("catfile: rev contains a newline")

var errCircuitOpen = errors.New("catfile: process failed 3 consecutive times, refusing to restart")

// Deps is what a Session needs to spawn its two processes.
type Deps struct {
	Runner  gitclient.Runner
	GitPath string
	Dir     string
}

// persistentProcess is one lazily-started, long-lived `git cat-file` child, driven one request at
// a time under its own mutex (upstream's own #pump — "one request in flight" — made explicit as
// the lock every request call holds for its whole write+read). It is spawned against a long-lived
// context owned by the Session (never a single request's own ctx): gitclient.Runner ties process
// lifetime to the context passed to Start, and a persistent process must outlive any one request.
type persistentProcess struct {
	runner   gitclient.Runner
	gitPath  string
	dir      string
	args     []string
	spawnCtx context.Context

	mu       sync.Mutex
	proc     gitclient.Process
	stdin    io.WriteCloser
	reader   *bufio.Reader
	failures int
}

// ensureStarted lazily spawns the child. Caller holds mu.
func (p *persistentProcess) ensureStarted() error {
	if p.proc != nil {
		return nil
	}
	if p.failures >= maxConsecutiveFailures {
		return errCircuitOpen
	}
	proc, err := p.runner.Start(p.spawnCtx, p.gitPath, gitclient.Spec{
		Dir: p.dir, Args: p.args, ReadOnly: true, Stdin: true,
	})
	if err != nil {
		p.failures++
		return err
	}
	p.proc = proc
	p.stdin = proc.Stdin()
	p.reader = bufio.NewReader(proc.Stdout())
	return nil
}

// fail tears the current child down (a queued request behind this one, or the next call, gets a
// fresh spawn attempt rather than hanging against a process that has already gone bad) and counts
// toward the circuit breaker. Caller holds mu.
func (p *persistentProcess) fail() {
	if p.proc != nil {
		_ = p.proc.Close()
	}
	p.proc, p.stdin, p.reader = nil, nil, nil
	p.failures++
}

// watchCtx interrupts an in-flight write/read by closing proc once ctx is done (Check/Read/
// CheckMany previously took no context at all, so a stalled reply blocked the caller — and every
// later caller queued behind it on mu — with no way to cancel). It never touches persistentProcess
// state itself (only the main goroutine, already holding mu, does that) — Process.Close is
// documented safe to call concurrently with an in-flight Read/Write on the same process, same
// pattern logsession.readChunkLocked already relies on.
//
// stop is synchronous: it reports whether the watcher fired, and when it did, waits until Close
// has finished. A request ctx is cancelled the instant the handler returns, so an async
// watcher could close a healthy process after a successful request and fail the next one.
func watchCtx(ctx context.Context, proc gitclient.Process) (stop func() (fired bool)) {
	closed := make(chan struct{})
	stopAfter := context.AfterFunc(ctx, func() {
		defer close(closed)
		_ = proc.Close()
	})
	return func() bool {
		if stopAfter() {
			return false
		}
		<-closed
		return true
	}
}

// drop discards the current child without counting a failure: a ctx cancel is not a process
// fault and must not trip the circuit breaker. Caller holds mu.
func (p *persistentProcess) drop() {
	p.proc, p.stdin, p.reader = nil, nil, nil
}

// failOrCancel handles an I/O error mid-request. A fired watcher means this side killed the child
// for a cancelled ctx, so the error is the cancel, not a process fault: drop without counting.
// Caller holds mu.
func (p *persistentProcess) failOrCancel(ctx context.Context, stop func() bool, err error) error {
	if stop() {
		p.drop()
		return ctx.Err()
	}
	p.fail()
	return err
}

// request writes line to the child's stdin and hands its stdout reader to readResp — the whole
// call runs under mu, which is what makes "one request in flight" true regardless of how many
// goroutines call request concurrently: they queue on the lock, FIFO. readResp must return a
// non-nil error only for a genuine protocol/IO failure — a normal "object missing" answer is not
// one (readHeader itself returns found=false, err=nil for it), so a missing lookup never trips
// the circuit breaker.
func (p *persistentProcess) request(ctx context.Context, line string, readResp func(*bufio.Reader) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.ensureStarted(); err != nil {
		return err
	}
	stop := watchCtx(ctx, p.proc)
	if _, err := io.WriteString(p.stdin, line); err != nil {
		return p.failOrCancel(ctx, stop, err)
	}
	if err := readResp(p.reader); err != nil {
		return p.failOrCancel(ctx, stop, err)
	}
	if stop() {
		p.drop()
		return nil
	}
	p.failures = 0
	return nil
}

// requestPipelined is request's own multi-line counterpart, CheckMany's own use: request's
// write-then-read ordering deadlocks once a batch is
// large enough to fill BOTH this process's own pipe to the child (our write blocks) AND the
// child's stdout pipe back to us (git blocks writing responses to a reader that hasn't started
// draining, since we're still mid-write) — reachable in practice via CheckMany at roughly a
// thousand-plus revs (RangeFiles on a monorepo branch touching that many already-reviewed files).
// Pipelining the write on its own goroutine, concurrent with readResp below, removes the ordering
// constraint entirely — exactly what --batch-check's own "many requests, one write, streamed
// answers" protocol exists to allow. Held under the same mu, for the same "one request in flight"
// reason request's own doc comment gives.
func (p *persistentProcess) requestPipelined(ctx context.Context, lines string, readResp func(*bufio.Reader) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.ensureStarted(); err != nil {
		return err
	}
	stop := watchCtx(ctx, p.proc)
	// stdin is captured into a local before the goroutine starts, not read as p.stdin from
	// inside it: fail() (below, and the new early call on a respErr) nils p.stdin under mu as soon
	// as readResp returns, which could otherwise race this goroutine's own unsynchronized read of
	// that same field against a p that has already moved on.
	stdin := p.stdin
	writeErrCh := make(chan error, 1)
	go func() {
		_, werr := io.WriteString(stdin, lines)
		writeErrCh <- werr
	}()
	respErr := readResp(p.reader)
	if respErr != nil {
		// fail() (closing stdin, killing the process) BEFORE draining writeErrCh — a large
		// enough batch's own writer goroutine can be blocked on a full stdin pipe (git itself
		// blocked writing a response to a reader that just stopped reading, on the very same
		// batch), so waiting on writeErrCh first, as this used to, could deadlock this call
		// itself, wedging p.mu — and every later Check/Read queued behind it — forever.
		err := p.failOrCancel(ctx, stop, respErr)
		<-writeErrCh // drain: the writer goroutine must not leak past this call's own return.
		return err
	}
	writeErr := <-writeErrCh
	if writeErr != nil {
		return p.failOrCancel(ctx, stop, writeErr)
	}
	if stop() {
		p.drop()
		return nil
	}
	p.failures = 0
	return nil
}

func (p *persistentProcess) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.proc != nil {
		_ = p.proc.Close()
	}
	p.proc, p.stdin, p.reader = nil, nil, nil
}

// Session is the `--batch-check`/`--batch` pair, lazily started, driven by revision strings.
type Session struct {
	maxBlobBytes int64
	check        *persistentProcess
	batch        *persistentProcess
	cancelSpawn  context.CancelFunc

	runner  gitclient.Runner
	gitPath string
	dir     string
}

// NewSession constructs a Session over deps — nothing is spawned until the first Check/Read.
// maxBlobBytes <= 0 defaults to DefaultMaxBlobBytes.
func NewSession(deps Deps, maxBlobBytes int64) *Session {
	if maxBlobBytes <= 0 {
		maxBlobBytes = DefaultMaxBlobBytes
	}
	spawnCtx, cancel := context.WithCancel(context.Background())
	return &Session{
		maxBlobBytes: maxBlobBytes,
		check: &persistentProcess{
			runner: deps.Runner, gitPath: deps.GitPath, dir: deps.Dir,
			args: []string{"cat-file", "--batch-check"}, spawnCtx: spawnCtx,
		},
		batch: &persistentProcess{
			runner: deps.Runner, gitPath: deps.GitPath, dir: deps.Dir,
			args: []string{"cat-file", "--batch"}, spawnCtx: spawnCtx,
		},
		cancelSpawn: cancel,
		runner:      deps.Runner, gitPath: deps.GitPath, dir: deps.Dir,
	}
}

// Check resolves rev via --batch-check alone — no content is ever read. Returns ErrMissing when
// git could not resolve rev. ctx can cancel a stalled reply — a hung child, or a partial
// clone's lazy blob fetch from its promisor remote — without blocking this caller, and every other
// caller queued behind it on the persistent process's own mutex, forever.
func (s *Session) Check(ctx context.Context, rev string) (ObjectInfo, error) {
	if strings.Contains(rev, "\n") {
		return ObjectInfo{}, ErrInvalidRev
	}
	var info ObjectInfo
	var found bool
	err := s.check.request(ctx, rev+"\n", func(r *bufio.Reader) error {
		var rerr error
		info, found, rerr = readHeader(r)
		return rerr
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	if !found {
		return ObjectInfo{}, ErrMissing
	}
	return info, nil
}

// CheckMany resolves every rev in revs via ONE --batch-check round trip — one write of all of
// them, then all their header lines read back in the same order — rather than one request() call
// (one write, one read, under the process's own single-request mutex) per rev:
// `cat-file --batch-check` is explicitly designed to take many revisions in one write and stream
// back all the answers, the whole point of a persistent process in the first place. The returned
// slice is the same length as revs, in the same order; a rev git could not resolve gets a zero
// ObjectInfo at its own index (Check's own ErrMissing becomes a per-Go-error return for a single
// rev, but a batch of N cannot fail some and succeed others through one error return, so
// "missing" is a zero-value slot here instead).
func (s *Session) CheckMany(ctx context.Context, revs []string) ([]ObjectInfo, error) {
	if len(revs) == 0 {
		return nil, nil
	}
	var sb strings.Builder
	for _, rev := range revs {
		if strings.Contains(rev, "\n") {
			return nil, ErrInvalidRev
		}
		sb.WriteString(rev)
		sb.WriteByte('\n')
	}
	infos := make([]ObjectInfo, len(revs))
	err := s.check.requestPipelined(ctx, sb.String(), func(r *bufio.Reader) error {
		for i := range revs {
			info, found, rerr := readHeader(r)
			if rerr != nil {
				return rerr
			}
			if found {
				infos[i] = info
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return infos, nil
}

// Read resolves rev and returns its content. Checks the size via --batch-check first (Check) and
// answers ErrTooLarge without ever asking --batch for the bytes when it exceeds the session's
// gate. That gate alone is not enough against a mutable rev (e.g. "HEAD:<path>", codeworkspace/
// diff.go's readHeadSide, or a client-supplied rev via file.read) — the --batch-check process and
// the --batch process below are two SEPARATE, independently-spawned processes, each resolving rev
// against whatever HEAD happens to be at the moment IT runs, so HEAD moving between the two calls
// means the size that passed the gate is not the size about to be read. batchInfo.Size —
// this second process's own, independent snapshot — is re-checked inside the same closure that
// already has it, before readContent's own unbounded allocation.
func (s *Session) Read(ctx context.Context, rev string) (ObjectInfo, []byte, error) {
	info, err := s.Check(ctx, rev)
	if err != nil {
		return ObjectInfo{}, nil, err
	}
	if info.Size > s.maxBlobBytes {
		return info, nil, ErrTooLarge
	}

	var content []byte
	var found, tooLarge bool
	var batchInfo ObjectInfo
	err = s.batch.request(ctx, rev+"\n", func(r *bufio.Reader) error {
		var rerr error
		batchInfo, found, rerr = readHeader(r)
		if rerr != nil || !found {
			return rerr
		}
		if batchInfo.Size > s.maxBlobBytes {
			// Still consumed exactly per readContent's own framing (Size bytes plus the trailing
			// LF) so the process's own stdout stream stays in sync for the next request. A
			// legitimate outcome, not a protocol failure — returns nil here (not an error) so
			// request() resets the failure count instead of tearing the process down and
			// eventually tripping the circuit breaker on an ordinary large blob.
			tooLarge = true
			_, rerr = io.CopyN(io.Discard, r, batchInfo.Size+1)
			return rerr
		}
		content, rerr = readContent(r, batchInfo.Size)
		return rerr
	})
	if err != nil {
		return ObjectInfo{}, nil, err
	}
	if !found {
		return ObjectInfo{}, nil, ErrMissing
	}
	if tooLarge {
		return batchInfo, nil, ErrTooLarge
	}
	return batchInfo, content, nil
}

// CheckOneShot is Check's own counterpart to ReadOneShot — a rev the batch protocol cannot
// express (a newline anywhere in it: `cat-file --batch-check` reads one request per line, same
// framing limit ReadOneShot's own doc comment explains). Spawns `git rev-parse --verify <rev>`
// once, argv-only (no line framing to break): for a `<rev>:<path>` expression this resolves to
// exactly the same OID `Check`'s own --batch-check header line would have reported, with a
// non-zero exit standing in for "missing" — the same ErrMissing answer Check gives for an
// unresolvable rev, drawing no finer distinction than the batch protocol already does (mirrors
// ReadOneShot's own reasoning verbatim).
func (s *Session) CheckOneShot(ctx context.Context, rev string) (ObjectInfo, error) {
	res, err := gitclient.Run(ctx, s.runner, s.gitPath, gitclient.Spec{
		Dir: s.dir, Args: []string{"rev-parse", "--verify", rev}, ReadOnly: true,
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	if res.ExitCode != 0 {
		return ObjectInfo{}, ErrMissing
	}
	return ObjectInfo{OID: strings.TrimSpace(string(res.Stdout))}, nil
}

// ReadOneShot answers a rev the batch protocol cannot express — a path containing a newline
// (`cat-file --batch` reads one request per line, so a newline mid-request would be seen as two).
// Checks the size via `cat-file -s <rev>` FIRST, argv-only (no line framing to break, same as
// the read itself), so the gate is exact before any unbounded read starts — the previous design
// read the whole (possibly enormous) object into memory via `git show <rev>` and only checked its
// length afterward, reachable with a client-supplied rev plus a hostile path. A non-zero exit on
// either spawn is reported as ErrMissing, the same answer a batch lookup gives — the overwhelmingly
// likely cause is a path that does not resolve at rev, and this package draws no finer distinction
// than the batch protocol already does.
func (s *Session) ReadOneShot(ctx context.Context, rev string) (ObjectInfo, []byte, error) {
	// Pin the OID once: a mutable rev (HEAD:<path>) could resolve differently in each spawn.
	pinned, err := s.CheckOneShot(ctx, rev)
	if err != nil {
		return ObjectInfo{}, nil, err
	}
	rev = pinned.OID
	sizeRes, err := gitclient.Run(ctx, s.runner, s.gitPath, gitclient.Spec{
		Dir: s.dir, Args: []string{"cat-file", "-s", rev}, ReadOnly: true,
	})
	if err != nil {
		return ObjectInfo{}, nil, err
	}
	if sizeRes.ExitCode != 0 {
		return ObjectInfo{}, nil, ErrMissing
	}
	size, perr := strconv.ParseInt(strings.TrimSpace(string(sizeRes.Stdout)), 10, 64)
	if perr != nil {
		return ObjectInfo{}, nil, fmt.Errorf("catfile: cat-file -s %s: unparseable size %q: %w", rev, sizeRes.Stdout, perr)
	}
	if size > s.maxBlobBytes {
		return ObjectInfo{Type: "blob", Size: size}, nil, ErrTooLarge
	}

	res, err := gitclient.Run(ctx, s.runner, s.gitPath, gitclient.Spec{
		Dir: s.dir, Args: []string{"cat-file", "blob", rev}, ReadOnly: true,
	})
	if err != nil {
		return ObjectInfo{}, nil, err
	}
	if res.ExitCode != 0 {
		return ObjectInfo{}, nil, ErrMissing
	}
	return ObjectInfo{Type: "blob", Size: int64(len(res.Stdout))}, res.Stdout, nil
}

// Close stops both persistent processes. Idempotent.
func (s *Session) Close() {
	s.cancelSpawn()
	s.check.close()
	s.batch.close()
}
