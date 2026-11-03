package catfile_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/catfile"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// skipWithoutGit is testx.SkipWithoutGit (P107 I2-28).
var skipWithoutGit = testx.SkipWithoutGit

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", "hello.txt")
	run("commit", "-q", "-m", "add hello.txt")
	return dir
}

func TestSession_Read_FoundBlob(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	info, content, err := sess.Read(context.Background(), "HEAD:hello.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info.Type != "blob" {
		t.Fatalf("type = %q, want blob", info.Type)
	}
	if string(content) != "hello world\n" {
		t.Fatalf("content = %q", content)
	}
	if info.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", info.Size, len(content))
	}
}

func TestSession_Check_Missing(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	_, err := sess.Check(context.Background(), "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if !errors.Is(err, catfile.ErrMissing) {
		t.Fatalf("got %v, want ErrMissing", err)
	}
}

// TestSession_CheckMany_MixOfFoundAndMissing is the regression guard: CheckMany writes every rev in
// one request() call and must read back exactly one header per rev, IN ORDER, correctly separating
// found entries (their own real ObjectInfo) from missing ones (a zero ObjectInfo, matching Check's
// own "" convention) — not just for an all-found or all-missing batch, but for one that interleaves
// both, which is where an off-by-one in the response-reading loop would surface.
func TestSession_CheckMany_MixOfFoundAndMissing(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := exec.Command("git", "add", "second.txt")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-q", "-m", "add second.txt")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	infos, err := sess.CheckMany(context.Background(), []string{
		"HEAD:hello.txt",
		"HEAD:does-not-exist.txt",
		"HEAD:second.txt",
	})
	if err != nil {
		t.Fatalf("CheckMany: %v", err)
	}
	if len(infos) != 3 {
		t.Fatalf("len(infos) = %d, want 3", len(infos))
	}
	if infos[0].OID == "" || infos[0].Type != "blob" {
		t.Fatalf("infos[0] (hello.txt) = %+v, want a real found blob", infos[0])
	}
	if infos[1].OID != "" {
		t.Fatalf("infos[1] (missing) = %+v, want a zero ObjectInfo", infos[1])
	}
	if infos[2].OID == "" || infos[2].Type != "blob" {
		t.Fatalf("infos[2] (second.txt) = %+v, want a real found blob", infos[2])
	}
	if infos[0].OID == infos[2].OID {
		t.Fatalf("hello.txt and second.txt resolved to the SAME oid (%s) — the response reader misaligned", infos[0].OID)
	}

	// The session must still work normally afterward — a misaligned read would leave the process's
	// own response stream desynced for every subsequent request.
	single, err := sess.Check(context.Background(), "HEAD:hello.txt")
	if err != nil || single.OID != infos[0].OID {
		t.Fatalf("Check after CheckMany = %+v, %v; want the same oid as infos[0] with no error", single, err)
	}
}

// TestSession_CheckMany_Empty proves the zero-revs edge case is a plain no-op, not a hang waiting
// on a response that was never requested.
func TestSession_CheckMany_Empty(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	infos, err := sess.CheckMany(context.Background(), nil)
	if err != nil || infos != nil {
		t.Fatalf("CheckMany(nil) = %+v, %v; want nil, nil", infos, err)
	}
}

// TestSession_Read_FoundBlob_PathWithSpace is the "guard the existing fix" case:
// a space-containing path resolved through the ordinary batch protocol, proving readHeader's
// suffix-based " missing" recognition does not also misfire on a *found* line whose echoed input
// happens to contain a space.
func TestSession_Read_FoundBlob_PathWithSpace(t *testing.T) {
	skipWithoutGit(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "my file.txt"), []byte("spaced\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", "my file.txt")
	run("commit", "-q", "-m", "add spaced file")

	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	info, content, err := sess.Read(context.Background(), "HEAD:my file.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info.Type != "blob" || string(content) != "spaced\n" {
		t.Fatalf("info=%+v content=%q", info, content)
	}
}

func TestSession_Check_MissingWithSpacesInEchoedInput(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	// git echoes this exact (space-containing) input back ahead of " missing" — the case
	// readHeader's suffix-based recognition exists for.
	_, err := sess.Check(context.Background(), "HEAD:a file that does not exist.txt")
	if !errors.Is(err, catfile.ErrMissing) {
		t.Fatalf("got %v, want ErrMissing", err)
	}
}

// batchOnlyPanicRunner delegates every --batch-check spawn to the real exec runner and fails the
// test outright if --batch is ever spawned — the direct proof that an oversize blob is answered
// from --batch-check alone (Session.Read never asks --batch for content it is about to discard).
type batchOnlyPanicRunner struct {
	t    *testing.T
	real gitclient.Runner
}

func (r batchOnlyPanicRunner) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	for _, a := range spec.Args {
		if a == "--batch" {
			r.t.Fatal("--batch was spawned for a blob over the size gate — Read must never ask for its content")
		}
	}
	return r.real.Start(ctx, gitPath, spec)
}

// TestSession_ReadOneShot_NewlinePath is D10's own new case: a path containing a newline cannot be
// expressed in the batch protocol at all (one request per line), so it must resolve through the
// one-shot `git show` fallback instead.
func TestSession_ReadOneShot_NewlinePath(t *testing.T) {
	skipWithoutGit(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	name := "weird\nname.txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("newline path content\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "add newline-path file")

	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	info, content, err := sess.ReadOneShot(context.Background(), "HEAD:"+name)
	if err != nil {
		t.Fatalf("ReadOneShot: %v", err)
	}
	if info.Type != "blob" || string(content) != "newline path content\n" {
		t.Fatalf("info=%+v content=%q", info, content)
	}
}

func TestSession_ReadOneShot_Missing(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	_, _, err := sess.ReadOneShot(context.Background(), "HEAD:does\nnot\nexist.txt")
	if !errors.Is(err, catfile.ErrMissing) {
		t.Fatalf("got %v, want ErrMissing", err)
	}
}

// TestSession_CheckOneShot_NewlineRev is regression coverage — Check's counterpart to
// TestSession_ReadOneShot_NewlinePath above: a newline anywhere in the rev (not just the path half
// of it) cannot be expressed in the --batch-check protocol either, and must resolve through this
// one-shot `rev-parse --verify` fallback instead, giving the same OID Check's own batch header line
// would have.
func TestSession_CheckOneShot_NewlineRev(t *testing.T) {
	skipWithoutGit(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	name := "weird\nname.txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("newline path content\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "add newline-path file")

	// Independent oracle for the expected OID: `git hash-object` on the file's own bytes, a
	// wholly different code path than either Check or CheckOneShot's own rev-parse.
	hashOut, err := exec.Command("git", "-C", dir, "hash-object", name).Output()
	if err != nil {
		t.Fatalf("git hash-object: %v", err)
	}
	wantOID := strings.TrimSpace(string(hashOut))

	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	info, err := sess.CheckOneShot(context.Background(), "HEAD:"+name)
	if err != nil {
		t.Fatalf("CheckOneShot: %v", err)
	}
	if info.OID != wantOID {
		t.Fatalf("CheckOneShot OID = %q, want %q (git hash-object)", info.OID, wantOID)
	}
}

func TestSession_CheckOneShot_Missing(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	_, err := sess.CheckOneShot(context.Background(), "HEAD:does\nnot\nexist.txt")
	if !errors.Is(err, catfile.ErrMissing) {
		t.Fatalf("got %v, want ErrMissing", err)
	}
}

func TestSession_Read_TooLarge_AnsweredFromCheckAlone(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	runner := batchOnlyPanicRunner{t: t, real: gitclient.NewExecRunner()}
	sess := catfile.NewSession(catfile.Deps{Runner: runner, GitPath: "git", Dir: dir}, 5) // hello.txt is 12 bytes
	defer sess.Close()

	_, _, err := sess.Read(context.Background(), "HEAD:hello.txt")
	if !errors.Is(err, catfile.ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
}

// blobReadPanicRunner delegates every `cat-file -s` spawn to the real exec runner and fails the
// test outright if `cat-file blob` (the actual content read) is ever spawned — the direct
// proof that ReadOneShot's size gate is checked BEFORE any unbounded read starts, the newline-path
// counterpart to TestSession_Read_TooLarge_AnsweredFromCheckAlone's --batch-check/--batch split.
type blobReadPanicRunner struct {
	t    *testing.T
	real gitclient.Runner
}

func (r blobReadPanicRunner) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	for i, a := range spec.Args {
		if a == "blob" && i > 0 && spec.Args[i-1] == "cat-file" {
			r.t.Fatal("cat-file blob was spawned for a newline-path blob over the size gate — ReadOneShot must never read its content")
		}
	}
	return r.real.Start(ctx, gitPath, spec)
}

// TestSession_ReadOneShot_TooLarge_NeverReadsPastTheSizeCheck is the regression guard: the
// old design read the WHOLE object via `git show <rev>` and only checked its length afterward —
// reachable with a client-supplied rev plus a path containing a newline in a hostile repo. The fix
// checks `cat-file -s <rev>` first, argv-only, so the gate is exact before any unbounded read.
func TestSession_ReadOneShot_TooLarge_NeverReadsPastTheSizeCheck(t *testing.T) {
	skipWithoutGit(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	name := "weird\nbig.txt" // a newline in the path forces the ReadOneShot fallback (D10).
	if err := os.WriteFile(filepath.Join(dir, name), []byte("this content is over the size gate\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "add oversize newline-path file")

	runner := blobReadPanicRunner{t: t, real: gitclient.NewExecRunner()}
	sess := catfile.NewSession(catfile.Deps{Runner: runner, GitPath: "git", Dir: dir}, 5) // the file is well over 5 bytes.
	defer sess.Close()

	info, content, err := sess.ReadOneShot(context.Background(), "HEAD:"+name)
	if !errors.Is(err, catfile.ErrTooLarge) {
		t.Fatalf("ReadOneShot: err=%v, want ErrTooLarge", err)
	}
	if content != nil {
		t.Fatalf("content = %q, want nil on ErrTooLarge", content)
	}
	if info.Size <= 5 {
		t.Fatalf("info.Size = %d, want the real size over the 5-byte gate", info.Size)
	}
}

// --- a killed process fails every queued request rather than hanging -----------------------

// killableProcess is a fake gitclient.Process whose Stdout can be switched, mid-life, to a
// reader that returns io.EOF — simulating the child having been killed underneath the session.
type killableProcess struct {
	stdout *swappableReader
	stdin  *bytes.Buffer
}

func (p *killableProcess) Stdout() io.ReadCloser { return io.NopCloser(p.stdout) }
func (p *killableProcess) Stdin() io.WriteCloser { return nopWriteCloser{p.stdin} }
func (p *killableProcess) Wait() (gitclient.Result, error) {
	return gitclient.Result{ExitCode: 0}, nil
}
func (p *killableProcess) Close() error { return nil }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type swappableReader struct{ r io.Reader }

func (s *swappableReader) Read(p []byte) (int, error) { return s.r.Read(p) }

// killedAfterFirstRunner answers the first Start with a working process whose stdout is then
// swapped to EOF (simulating a kill); every subsequent Start fails outright, simulating a git
// binary that has become permanently unusable — the circuit breaker's own worst case.
type killedAfterFirstRunner struct {
	proc    *killableProcess
	starts  int
	failAll bool
}

func (r *killedAfterFirstRunner) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	r.starts++
	if r.failAll {
		return nil, errors.New("simulated: git is permanently broken")
	}
	stdout := "0123456789abcdef0123456789abcdef01234567 blob 5\nhello\n"
	r.proc = &killableProcess{stdout: &swappableReader{r: bytes.NewReader([]byte(stdout))}, stdin: &bytes.Buffer{}}
	return r.proc, nil
}

func TestSession_KilledProcess_FailsQueuedRequestsRatherThanHanging(t *testing.T) {
	runner := &killedAfterFirstRunner{}
	sess := catfile.NewSession(catfile.Deps{Runner: runner, GitPath: "git", Dir: "."}, 0)
	defer sess.Close()

	// First request succeeds against the live process.
	if _, _, err := sess.Read(context.Background(), "first"); err != nil {
		t.Fatalf("first Read: %v", err)
	}

	// Simulate the process dying: its stdout now yields EOF immediately.
	runner.proc.stdout.r = bytes.NewReader(nil)
	runner.failAll = true // every respawn attempt from here on fails, tripping the circuit breaker.

	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			_, _, err := sess.Read(context.Background(), "second")
			done <- err
		}()
	}
	deadline := time.After(5 * time.Second)
	for i := 0; i < 4; i++ {
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("expected every queued request to fail once the process is dead and unrestartable")
			}
		case <-deadline:
			t.Fatal("a request against a killed, unrestartable process hung instead of failing")
		}
	}
}

// TestSession_CheckMany_LargeBatchDoesNotDeadlock is the regression proof: CheckMany used to write
// its entire batch to the child's stdin before reading a single response line back. Past roughly a
// thousand-plus revs, the child's own stdout pipe back to us fills (nothing has started draining
// it yet) and git blocks writing further responses; our own write to its stdin then blocks too,
// since git has stopped reading it —
// deadlock, forever, with no ctx on this path to time it out. A real `git cat-file --batch-check`
// process is used (not a fake): the pipe buffers this depends on are a real OS/kernel property,
// not something a fake reader/writer would reproduce.
func TestSession_CheckMany_LargeBatchDoesNotDeadlock(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	sess := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer sess.Close()

	const n = 20000
	revs := make([]string, n)
	for i := range revs {
		revs[i] = "HEAD"
	}

	done := make(chan struct {
		infos []catfile.ObjectInfo
		err   error
	}, 1)
	go func() {
		infos, err := sess.CheckMany(context.Background(), revs)
		done <- struct {
			infos []catfile.ObjectInfo
			err   error
		}{infos, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("CheckMany: %v", r.err)
		}
		if len(r.infos) != n {
			t.Fatalf("got %d results, want %d", len(r.infos), n)
		}
		for i, info := range r.infos {
			if info.OID == "" {
				t.Fatalf("result[%d] is a zero ObjectInfo -- HEAD must resolve", i)
			}
		}
	case <-time.After(20 * time.Second):
		t.Fatal("CheckMany on a large batch hung instead of returning -- the write/read pipe deadlock")
	}
}

// --- F8: requestPipelined must not deadlock when readResp errors while the writer is still stuck ---

// f8BlockedWriteCloser never completes a Write until unblock is closed — simulating a stdin pipe
// so full that the writer goroutine is stuck on it (git itself blocked writing a response to a
// reader that has already stopped reading, on the very same batch).
type f8BlockedWriteCloser struct{ unblock chan struct{} }

func (w f8BlockedWriteCloser) Write(p []byte) (int, error) {
	<-w.unblock
	return 0, errors.New("simulated: process was killed mid-write")
}
func (w f8BlockedWriteCloser) Close() error { return nil }

// f8ErroringReader fails every Read immediately — standing in for whatever causes readResp
// (readHeader, in production) to return a genuine error, independent of the write side above.
type f8ErroringReader struct{}

func (f8ErroringReader) Read(p []byte) (int, error) { return 0, errors.New("simulated read failure") }

// f8Process pairs the two: Close (persistentProcess.fail's own call) is what unblocks the writer,
// exactly like killing a real process would close its stdin pipe out from under a stuck Write.
type f8Process struct{ unblock chan struct{} }

func (p *f8Process) Stdout() io.ReadCloser { return io.NopCloser(f8ErroringReader{}) }
func (p *f8Process) Stdin() io.WriteCloser { return f8BlockedWriteCloser{p.unblock} }
func (p *f8Process) Wait() (gitclient.Result, error) {
	return gitclient.Result{ExitCode: 0}, nil
}
func (p *f8Process) Close() error {
	select {
	case <-p.unblock:
	default:
		close(p.unblock)
	}
	return nil
}

type f8Runner struct{ proc *f8Process }

func (r *f8Runner) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	return r.proc, nil
}

// TestSession_CheckMany_RespErrDoesNotDeadlockOnBlockedWrite is the regression guard: on a
// respErr, requestPipelined must call fail() (closing stdin, unblocking the writer) BEFORE
// draining writeErrCh — waiting on writeErrCh first, as it used to, deadlocks this call itself
// whenever the writer goroutine is stuck on a full stdin pipe at the same moment readResp fails,
// wedging the persistent process's own mutex (and every later Check/Read queued behind it)
// forever.
func TestSession_CheckMany_RespErrDoesNotDeadlockOnBlockedWrite(t *testing.T) {
	runner := &f8Runner{proc: &f8Process{unblock: make(chan struct{})}}
	sess := catfile.NewSession(catfile.Deps{Runner: runner, GitPath: "git", Dir: "."}, 0)
	defer sess.Close()

	done := make(chan error, 1)
	go func() {
		_, err := sess.CheckMany(context.Background(), []string{"a", "b"})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("CheckMany: want the simulated read failure, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("CheckMany hung — respErr must fail() the process (unblocking the writer) before draining writeErrCh")
	}
}

// --- F9: Read must re-check the SEPARATE --batch process's own size against the gate -----------

// f9Process is a fake gitclient.Process over a fixed, pre-scripted stdout stream — standing in for
// one persistent `cat-file --batch[-check]` child, reused across every request that fake sees
// exactly like the real thing (ensureStarted only spawns once).
type f9Process struct {
	stdout *bytes.Reader
	stdin  bytes.Buffer
}

func (p *f9Process) Stdout() io.ReadCloser { return io.NopCloser(p.stdout) }
func (p *f9Process) Stdin() io.WriteCloser { return nopWriteCloser{&p.stdin} }
func (p *f9Process) Wait() (gitclient.Result, error) {
	return gitclient.Result{ExitCode: 0}, nil
}
func (p *f9Process) Close() error { return nil }

// f9Runner answers --batch-check and --batch with two INDEPENDENT scripted processes — mirroring
// production exactly: Session.check and Session.batch are two separate persistent children, each
// with its own view of a mutable rev at the moment IT runs. starts counts every Start call, so a
// test can assert the process was never torn down (persistentProcess.fail) and respawned.
type f9Runner struct {
	checkProc, batchProc *f9Process
	starts               int
}

func (r *f9Runner) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	r.starts++
	if len(spec.Args) > 1 && spec.Args[1] == "--batch-check" {
		return r.checkProc, nil
	}
	return r.batchProc, nil
}

// TestSession_Read_ReCheckBatchSizeAgainstGate is the regression guard: --batch-check (the
// outer gate) and --batch (the actual read) are two separate, independently-spawned processes, so
// a mutable rev (e.g. "HEAD:<path>") can resolve to a DIFFERENT, larger object by the time the
// second one runs — HEAD moved between the two calls. batchInfo.Size, the --batch process's own
// snapshot, must be re-checked against the gate before readContent's own unbounded allocation, the
// oversize content still fully (and correctly) discarded so the process's own response stream
// stays framed for the next request, and this must not count as a failure (no restart, no step
// toward the circuit breaker) since it is an entirely ordinary, expected outcome.
func TestSession_Read_ReCheckBatchSizeAgainstGate(t *testing.T) {
	oid := strings.Repeat("a", 40)
	bigContent := strings.Repeat("x", 999)
	// Two Check() calls worth of header (one per Read below), both reporting the SMALL size that
	// passes the outer gate — the point being that the --batch-check side never sees the problem.
	checkStdout := oid + " blob 5\n" + oid + " blob 5\n"
	// The --batch side's own two responses: the first is what --batch-check never warned about
	// (999 bytes, over the 50-byte gate below), the second is a small, ordinary follow-up.
	batchStdout := oid + " blob 999\n" + bigContent + "\n" + oid + " blob 5\nhello\n"

	runner := &f9Runner{
		checkProc: &f9Process{stdout: bytes.NewReader([]byte(checkStdout))},
		batchProc: &f9Process{stdout: bytes.NewReader([]byte(batchStdout))},
	}
	sess := catfile.NewSession(catfile.Deps{Runner: runner, GitPath: "git", Dir: "."}, 50)
	defer sess.Close()

	info, content, err := sess.Read(context.Background(), "HEAD:big.txt")
	if !errors.Is(err, catfile.ErrTooLarge) {
		t.Fatalf("Read: err=%v, want ErrTooLarge", err)
	}
	if content != nil {
		t.Fatalf("content = %q, want nil on ErrTooLarge", content)
	}
	if info.Size != 999 {
		t.Fatalf("info.Size = %d, want 999 (the SEPARATE --batch process's own snapshot, not --batch-check's 5)", info.Size)
	}

	// The --batch response stream must still be correctly framed for the next request — proving
	// the oversize content was fully consumed (readContent's own bytes-plus-trailing-LF framing),
	// not left partially buffered to desync every later read.
	info2, content2, err2 := sess.Read(context.Background(), "HEAD:small.txt")
	if err2 != nil {
		t.Fatalf("second Read: %v", err2)
	}
	if string(content2) != "hello" || info2.Size != 5 {
		t.Fatalf("second Read = %+v %q, want size 5 content \"hello\"", info2, content2)
	}

	if runner.starts != 2 {
		t.Fatalf("runner.starts = %d, want 2 (one --batch-check + one --batch spawn total — an "+
			"oversize blob must not count as a failure and tear the process down for a respawn)", runner.starts)
	}
}

// --- F11: Check/Read/CheckMany must accept a ctx that can cancel a stalled reply ----------------

// f11BlockingReadCloser blocks every Read until closed — standing in for a hung child (or a
// partial clone's lazy blob fetch stalled against its promisor remote) that never answers.
type f11BlockingReadCloser struct{ ch chan struct{} }

func (r *f11BlockingReadCloser) Read(p []byte) (int, error) {
	<-r.ch
	return 0, io.EOF
}
func (r *f11BlockingReadCloser) Close() error {
	select {
	case <-r.ch:
	default:
		close(r.ch)
	}
	return nil
}

type f11Process struct{ stdout *f11BlockingReadCloser }

func (p *f11Process) Stdout() io.ReadCloser { return p.stdout }
func (p *f11Process) Stdin() io.WriteCloser { return nopWriteCloser{&bytes.Buffer{}} }
func (p *f11Process) Wait() (gitclient.Result, error) {
	return gitclient.Result{ExitCode: 0}, nil
}
func (p *f11Process) Close() error { return p.stdout.Close() }

type f11Runner struct{ proc *f11Process }

func (r *f11Runner) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	return r.proc, nil
}

// TestSession_Check_CtxCancellationUnblocksAStalledReply is F11's own regression guard: before
// this, Check/Read/CheckMany took no context at all, so a stalled reply blocked the caller (and
// every later caller queued behind it on the persistent process's own mutex) with no way out.
func TestSession_Check_CtxCancellationUnblocksAStalledReply(t *testing.T) {
	runner := &f11Runner{proc: &f11Process{stdout: &f11BlockingReadCloser{ch: make(chan struct{})}}}
	sess := catfile.NewSession(catfile.Deps{Runner: runner, GitPath: "git", Dir: "."}, 0)
	defer sess.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := sess.Check(ctx, "deadbeef")
		done <- err
	}()

	// Give Check time to actually block on the stalled reply before cancelling.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Check: want an error once ctx is cancelled, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Check hung past ctx cancellation — watchCtx must kill the process to unblock the stalled reply")
	}
}

// A request ctx is cancelled the instant the handler returns. The ctx watcher must not
// close the healthy persistent process (or trip the circuit breaker) when that cancel lands.
func TestSession_CancelAfterSuccessDoesNotKillProcess(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	s := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer s.Close()
	for i := 0; i < 300; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_, _, err := s.Read(ctx, "HEAD:hello.txt")
		cancel()
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
	if _, _, err := s.Read(context.Background(), "HEAD:hello.txt"); err != nil {
		t.Fatalf("final read: %v", err)
	}
}

type freshStallRunner struct{ starts int }

func (r *freshStallRunner) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	r.starts++
	return &f11Process{stdout: &f11BlockingReadCloser{ch: make(chan struct{})}}, nil
}

// F5: a cancel mid-read kills the child but is not a process fault; it must return ctx.Err() and
// never count toward the circuit breaker.
func TestSession_CancelMidReadDoesNotTripCircuitBreaker(t *testing.T) {
	runner := &freshStallRunner{}
	sess := catfile.NewSession(catfile.Deps{Runner: runner, GitPath: "git", Dir: "."}, 0)
	defer sess.Close()

	for i := 0; i < 6; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		_, err := sess.Check(ctx, "deadbeef")
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("call %d: err = %v, want context.DeadlineExceeded", i, err)
		}
	}
	if runner.starts != 6 {
		t.Fatalf("starts = %d, want 6 (one fresh spawn per call, breaker never open)", runner.starts)
	}
}

// F13: a newline in a rev would make git answer twice and leave a stray reply for the next caller.
func TestSession_NewlineRevRejectedAndStreamStaysInStep(t *testing.T) {
	skipWithoutGit(t)
	dir := initRepo(t)
	s := catfile.NewSession(catfile.Deps{Runner: gitclient.NewExecRunner(), GitPath: "git", Dir: dir}, 0)
	defer s.Close()
	ctx := context.Background()

	if _, err := s.Check(ctx, "HEAD\nHEAD"); !errors.Is(err, catfile.ErrInvalidRev) {
		t.Fatalf("Check newline rev: err = %v, want ErrInvalidRev", err)
	}
	if _, err := s.CheckMany(ctx, []string{"HEAD", "HEAD\nHEAD"}); !errors.Is(err, catfile.ErrInvalidRev) {
		t.Fatalf("CheckMany newline rev: err = %v, want ErrInvalidRev", err)
	}
	if _, _, err := s.Read(ctx, "HEAD:hello.txt\nHEAD"); !errors.Is(err, catfile.ErrInvalidRev) {
		t.Fatalf("Read newline rev: err = %v, want ErrInvalidRev", err)
	}
	if _, err := s.Check(ctx, "doesnotexist"); !errors.Is(err, catfile.ErrMissing) {
		t.Fatalf("Check after rejected revs: err = %v, want ErrMissing", err)
	}
}
