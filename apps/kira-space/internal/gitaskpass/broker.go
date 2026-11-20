package gitaskpass

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/fileservice"
	"github.com/kirathecat/kira-studio/internal/localsock"
)

// DefaultTimeout is the broker's own bound on one credential wait — long enough for a
// human to type a passphrase, short enough that a broker-side bug can never hang a remote
// op forever. Mirrored to the helper via KIRA_ASKPASS_TIMEOUT_MS (WithOp) so the helper's own
// independent deadline agrees with the broker's.
const DefaultTimeout = 120 * time.Second

// Options configures a Broker. HelperCommand is the test seam (D8): defaulted in production to
// {os.Executable(), "askpass"}; tests set it to
// the stdlib os/exec "helper process" idiom instead, so the whole broker is provable with no app
// binary at all.
type Options struct {
	// Timeout bounds one credential wait. Zero uses DefaultTimeout.
	Timeout time.Duration
	// HelperCommand is the shim's own fixed argv, before git's prompt argument. Empty selects the
	// production default: this binary's own resolved path plus "askpass".
	HelperCommand []string
}

// opEntry is one in-flight remote op's registration (D4/D21): the ctx WithOp was called with, and
// the Prompter to relay a prompt to.
type opEntry struct {
	ctx      context.Context
	prompter Prompter
}

// Broker owns the shim file, the private socket and every in-flight op's registration. One per
// process (main.go constructs it once); Close removes its temp directory.
type Broker struct {
	shimPath string
	timeout  time.Duration
	// inheritedGitAskpass is captured once, at construction (D10) — os.Getenv read here rather
	// than per-op, matching upstream's own "the inherited value comes from os.Getenv('GIT_ASKPASS')
	// at broker construction".
	inheritedGitAskpass string

	ln *localsock.Listener

	mu  sync.Mutex
	ops map[string]*opEntry
}

// New constructs the broker's 0700 temp directory, writes the shim, opens the private socket
// (0600) and starts accepting connections. A failure here is never fatal to the caller
// ("a broker that fails to start... every remote op then runs with no interposition" — a nil
// *Broker is main.go's own already-supported path, not built here).
func New(opts Options) (*Broker, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	helperCommand := opts.HelperCommand
	if len(helperCommand) == 0 {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("gitaskpass: resolve executable: %w", err)
		}
		helperCommand = []string{exe, "askpass"}
	}
	shimBody, err := buildShim(helperCommand)
	if err != nil {
		return nil, err
	}

	// localsock.Listen is the Claude hooks listener's identical precedent (D8's whole security boundary,
	// P107 T2-9): no other OS user can read the shim or reach the socket.
	ln, err := localsock.Listen(localsock.Options{DirPrefix: "kira-askpass-", TokenBytes: 32})
	if err != nil {
		return nil, fmt.Errorf("gitaskpass: %w", err)
	}

	shimPath := filepath.Join(ln.Dir, "shim")
	if err := fileservice.Host().WriteFile(shimPath, []byte(shimBody), 0o700); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("gitaskpass: write shim: %w", err)
	}

	b := &Broker{
		shimPath: shimPath, timeout: timeout,
		inheritedGitAskpass: os.Getenv("GIT_ASKPASS"),
		ln:                  ln, ops: make(map[string]*opEntry),
	}
	go b.ln.Serve(b.handleConn)
	return b, nil
}

// buildShim renders the shim script: one `exec` of helperCommand's own argv, each element
// single-quoted, followed by "$1" (git's own prompt argument, still double-quoted — that is
// shell syntax the shim itself needs for word-splitting/expansion, never attacker-controlled
// content, so it is never quoted the same way as helperCommand's own elements).
func buildShim(helperCommand []string) (string, error) {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nexec")
	for _, arg := range helperCommand {
		b.WriteString(" ")
		b.WriteString(shellQuoteSingle(arg))
	}
	b.WriteString(` "$1"` + "\n")
	return b.String(), nil
}

// shellQuoteSingle wraps s in single quotes, escaping any embedded single quote as
//
//	'\''
//
// — close the quote, emit a backslash-escaped literal quote outside it, reopen the quote (the
// standard POSIX idiom). Unlike double-quoting (which still expands $var, $(...), backticks and
// backslash escapes inside it — the actual vulnerability this replaces), single quotes have no
// special character at all except the quote itself, so no character, including a literal newline
// (POSIX shell treats an embedded newline inside a single-quoted string as ordinary data, not a
// statement separator — verified against real /bin/sh), ever needs refusing.
func shellQuoteSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Env returns the constant env every remote-op spawn needs to reach this broker: GIT_ASKPASS/
// SSH_ASKPASS point at the shim, SSH_ASKPASS_REQUIRE=force is what makes OpenSSH >= 8.4 use
// SSH_ASKPASS unconditionally (probed against 9.6), and the session socket/token are constant
// for this broker's whole lifetime — WithOp layers the per-op id and timeout on top.
func (b *Broker) Env() []string {
	return []string{
		"GIT_ASKPASS=" + b.shimPath,
		"SSH_ASKPASS=" + b.shimPath,
		"SSH_ASKPASS_REQUIRE=force",
		"KIRA_ASKPASS_SOCK=" + b.ln.SockPath,
		"KIRA_ASKPASS_TOKEN=" + b.ln.Token,
	}
}

// ShouldInterpose reports whether this broker's shim should override coreAskPass for one spawn —
// D10: never overrides a user's own core.askPass or an inherited GIT_ASKPASS (captured once, at
// construction, above). The one-argument wrapper callers actually reach for.
func (b *Broker) ShouldInterpose(coreAskPass string) bool {
	return ShouldInterpose(coreAskPass, b.inheritedGitAskpass)
}

// WithOp registers prompter for one op (a fresh, unguessable 16-byte op id), runs fn with the
// per-op env it needs (KIRA_ASKPASS_OPID, KIRA_ASKPASS_TIMEOUT_MS), and unregisters the prompter
// on every exit path — a Prompter never outlives the op it was supplied for. ctx bounds every
// prompt this op's spawn might trigger; the table names the other three bounds (dismissal, the
// broker's own timer, Conn's own disconnect signal) as Prompter.Ask's own responsibility.
func (b *Broker) WithOp(ctx context.Context, prompter Prompter, fn func(opEnv []string) error) error {
	opID, err := localsock.RandHex(16)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.ops[opID] = &opEntry{ctx: ctx, prompter: prompter}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.ops, opID)
		b.mu.Unlock()
	}()

	opEnv := []string{
		"KIRA_ASKPASS_OPID=" + opID,
		"KIRA_ASKPASS_TIMEOUT_MS=" + strconv.FormatInt(b.timeout.Milliseconds(), 10),
	}
	return fn(opEnv)
}

// Close stops accepting new connections, waits for every in-flight one to finish, and removes the
// broker's temp directory (the shim and the socket file along with it).
func (b *Broker) Close() error {
	return b.ln.Close()
}

// handleConn answers exactly one request-response round trip — bounded by
// b.timeout plus a small grace so a slow-to-connect helper's own read never races the broker
// closing the connection out from under it. The op-scoped context additionally bounds
// Prompter.Ask below (WithOp's ctx) so this op's own cancellation ends the wait promptly, and a
// derived per-call timeout gives every wait the broker's own bound independent of that ctx.
func (b *Broker) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(b.timeout + 5*time.Second))

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	var req socketRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		writeResponse(conn, socketResponse{OK: false})
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Token), []byte(b.ln.Token)) != 1 {
		writeResponse(conn, socketResponse{OK: false})
		return
	}

	b.mu.Lock()
	entry, ok := b.ops[req.OpID]
	b.mu.Unlock()
	if !ok {
		writeResponse(conn, socketResponse{OK: false})
		return
	}

	askCtx, cancel := context.WithTimeout(entry.ctx, b.timeout)
	defer cancel()
	answer, answered := entry.prompter.Ask(askCtx, Request{
		Prompt: req.Prompt, Masked: !req.Confirm && DeriveMasked(req.Prompt), Confirm: req.Confirm,
	})
	if !answered {
		writeResponse(conn, socketResponse{OK: false})
		return
	}
	writeResponse(conn, socketResponse{OK: true, Answer: answer})
}

func writeResponse(conn net.Conn, resp socketResponse) {
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = conn.Write(b)
}
