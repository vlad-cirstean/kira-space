package appupdate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/procgroup"
)

const (
	// InstallScriptURL is fetched fresh on every Stage call, never bundled — see this file's own
	// "fresh-fetch, not bundled" doc comment on Installer for why.
	InstallScriptURL = "https://raw.githubusercontent.com/vlad-cirstean/kira-space/main/scripts/install.sh"
	// installContract is scripts/install.sh's own line 2, byte-exact (§2.1/§2.2 there). A fetched
	// script missing this exact line is refused before it ever reaches sh — the one thing keeping
	// an incompatible future script version, or a captive-portal HTML page, from being executed.
	installContract = "# kira-install-contract: 1"
	maxScriptBytes  = 256 << 10
	scriptTimeout   = 30 * time.Second
	// stageTimeout is a backstop only — the script's own `curl --speed-limit` aborts a stalled
	// transfer well before this fires.
	stageTimeout = 15 * time.Minute

	// handoffGrace bounds how long Stage waits for the real exit once the hand-off pipe reports
	// EOF or an unexpected line, so a failed run's own tail is read from a log the child has
	// actually finished writing.
	handoffGrace = 5 * time.Second
	// cancelGrace bounds how long Stage waits for a SIGTERM'd group to exit on its own before
	// escalating to SIGKILL.
	cancelGrace = 10 * time.Second
)

type installState int

const (
	stateIdle installState = iota
	stateStaging
	stateHandedOff
)

// Installer runs scripts/install.sh detached, in self-update mode, and waits for its own fd-3
// hand-off (the script's own contract: one line `staged <tag>` then the fd closes) before
// Stage returns — the caller (each app's own UpdateService.InstallUpdate) quits only once Stage
// has returned nil, never before.
//
// Fresh-fetch, not bundled — decision. The app fetches the script from `main` each time, the same
// bytes the documented curl command runs: one code path for first install and update, and an
// installer bugfix reaches every installed version at once (a bundled script with a bug is
// unfixable in place — the broken installer is the thing that would install the fix). The cost is
// that an old app runs installer logic it never shipped with; the contract marker above is what
// makes an incompatible future script fail closed with a clear error instead of misbehaving.
//
// Security note, normal prose on purpose: self-update executes a shell script downloaded at click
// time, with the user's own privileges, from the main branch of a public repository. The trust is
// exactly that of the documented curl | sh install and of the DMGs themselves, which are built
// from the same repository — anyone who can push to main can change what every user's Update click
// runs. Branch protection on main is therefore a real security boundary for this feature. TLS to
// the fetch host above (InstallScriptURL) and the contract-marker check guard transport and
// compatibility, not authorship. (Spelled out instead of named here so this stays the file's only
// literal reference — S11 in scripts/verify-packaging.sh checks for exactly one.)
type Installer struct {
	app     App
	running string

	// Seams. NewInstaller sets each to its real value; tests replace them directly (same package).
	fetch   func(ctx context.Context) ([]byte, error)
	logPath string
	shell   string // "/bin/sh"

	mu     sync.Mutex
	state  installState
	cancel context.CancelFunc
}

// NewInstaller builds an Installer over the real network fetch and the real shell, for app
// (App.ScriptArg is what --app= the script receives) and runningVersion (the dev-build guard).
func NewInstaller(app App, runningVersion string) *Installer {
	i := &Installer{
		app:     app,
		running: runningVersion,
		shell:   "/bin/sh",
		logPath: defaultLogPath(app.Name),
	}
	i.fetch = i.fetchScript
	return i
}

// DisplayLogPath is what the dialog shows the user — never crosses back into a filesystem call
// here (the literal "~" is for display only). Same path scripts/install.sh computes from
// $HOME/Library/Logs/<App name>/install.log — the two spellings are a contract; this
// comment and that script's own comment each name the other.
func (i *Installer) DisplayLogPath() string {
	return filepath.Join("~", "Library", "Logs", i.app.Name, "install.log")
}

func defaultLogPath(appName string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, "Library", "Logs", appName, "install.log")
}

// Stage fetches, validates, and runs the installer script detached in self-update mode, and
// returns once the new bundle is staged and verified (the script's own `staged <tag>` line) —
// never before, and never after the swap itself, which the script performs with no app open.
func (i *Installer) Stage(ctx context.Context) error {
	sctx, ok, err := i.begin(ctx)
	if !ok {
		return err
	}

	handedOff := false
	defer func() { i.finish(handedOff) }()

	body, err := i.fetch(sctx)
	if err == nil {
		err = validateScript(body)
	}
	if err != nil {
		return ipcerr.T(ipcerr.CodeInternal, "backend.core.appUpdate.downloadFailed.body", map[string]string{"cause": err.Error()})
	}

	scriptPath, err := writeScriptFile(body)
	if err != nil {
		return ipcerr.T(ipcerr.CodeInternal, "backend.core.appUpdate.stageFailed.body", map[string]string{"cause": err.Error()})
	}

	logFile, err := openLogFile(i.logPath, i.app.Name, i.running)
	if err != nil {
		_ = os.Remove(scriptPath)
		return ipcerr.T(ipcerr.CodeInternal, "backend.core.appUpdate.logOpenFailed.body", map[string]string{"cause": err.Error()})
	}

	staged, stageErr := i.spawnAndAwait(sctx, scriptPath, logFile)
	handedOff = staged
	return stageErr
}

// begin locks in the staging state, or reports why it couldn't — a dev build (isReleaseBuild's own
// guard) or a run already in progress. ok is false whenever the caller should return err
// immediately without touching sctx.
func (i *Installer) begin(ctx context.Context) (sctx context.Context, ok bool, err error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !isReleaseBuild(i.running) {
		return nil, false, ipcerr.T(ipcerr.CodeUnsupported, "backend.core.appUpdate.notRelease.body", nil)
	}
	if i.state != stateIdle {
		return nil, false, ipcerr.T(ipcerr.CodeConflict, "backend.core.appUpdate.inProgress.body", nil)
	}
	i.state = stateStaging
	sctx, cancel := context.WithTimeout(ctx, stageTimeout)
	i.cancel = cancel
	return sctx, true, nil
}

// finish always releases sctx's own cancel (context.WithTimeout's documented contract), and resets
// to idle unless handedOff — after a real hand-off the quit is the whole point, so Cancel must
// become a no-op rather than ever race the swap.
func (i *Installer) finish(handedOff bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cancel != nil {
		i.cancel()
		i.cancel = nil
	}
	if handedOff {
		i.state = stateHandedOff
	} else {
		i.state = stateIdle
	}
}

// Cancel acts only while staging — idle (nothing running) and handedOff (the swap is the point,
// nothing must stop it now) are both no-ops.
func (i *Installer) Cancel() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state == stateStaging && i.cancel != nil {
		i.cancel()
	}
}

// fetchScript is Installer.fetch's real implementation — reused httpClient (checker.go's own
// proxy-aware package var) rather than a second one.
func (i *Installer) fetchScript(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, InstallScriptURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", i.app.Name+"/"+i.running)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	// Read one byte past the limit so an over-size script is a clean error, not a silent
	// truncation that might still happen to satisfy validateScript's first two lines.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxScriptBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxScriptBytes {
		return nil, fmt.Errorf("installer script exceeds %d bytes", maxScriptBytes)
	}
	return body, nil
}

// validateScript checks the two lines that matter before anything is ever handed to sh: the
// shebang and the contract marker (installContract). This also stops a captive-portal HTML page
// from ever reaching sh.
func validateScript(body []byte) error {
	lines := strings.SplitN(string(body), "\n", 3)
	if len(lines) < 2 || lines[0] != "#!/bin/sh" {
		return errors.New("script does not start with '#!/bin/sh'")
	}
	if lines[1] != installContract {
		return fmt.Errorf("script's contract marker is %q, want %q — this app may be older than the installer it fetched", lines[1], installContract)
	}
	return nil
}

func writeScriptFile(body []byte) (string, error) {
	f, err := os.CreateTemp("", "kira-install-*.sh")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Chmod(0o700); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// openLogFile is the Go side of scripts/install.sh's own §2.1 log discipline: truncate past 1 MiB,
// else append, mode 0600 under a 0700 directory. The returned file becomes the child's own
// stdout/stderr, so every line the script's `log()` writes lands here directly.
func openLogFile(logPath, appName, running string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Stat(logPath); err == nil && info.Size() > 1<<20 {
		if err := os.Truncate(logPath, 0); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	header := fmt.Sprintf("---- %s self-update app=%s version=%s pid=%d ----\n",
		time.Now().UTC().Format(time.RFC3339), appName, running, os.Getpid())
	if _, err := f.WriteString(header); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// spawnAndAwait runs scriptPath detached and waits for its hand-off: a
// `staged <tag>` line on fd 3 means the new bundle is ready and the caller may quit; anything else
// (EOF, an unexpected line, cancellation, or the stage deadline) means nothing changed. logFile is
// always closed here — the child holds its own duplicated copy once Start succeeds.
func (i *Installer) spawnAndAwait(ctx context.Context, scriptPath string, logFile *os.File) (staged bool, err error) {
	r, w, err := os.Pipe()
	if err != nil {
		_ = logFile.Close()
		return false, fmt.Errorf("create hand-off pipe: %w", err)
	}

	cmd := exec.Command(i.shell, scriptPath,
		"--app="+i.app.ScriptArg,
		"--wait-pid="+strconv.Itoa(os.Getpid()),
		"--notify-fd=3",
	)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.ExtraFiles = []*os.File{w}
	cmd.Dir = "/"
	cmd.Env = os.Environ()
	// Its own session and process group (pgid == pid), no controlling terminal — the app's own
	// exit neither signals nor reaps it, and a later procgroup.Kill(pid, …) reaches whatever the
	// script itself forked (hdiutil, ditto, osascript).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if startErr := cmd.Start(); startErr != nil {
		_ = r.Close()
		_ = w.Close()
		_ = logFile.Close()
		return false, fmt.Errorf("start installer: %w", startErr)
	}
	// The child holds its own copies of w and logFile now; Go's CLOEXEC default keeps every other
	// fd (the DB, sockets) out of it.
	_ = w.Close()
	_ = logFile.Close()

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	line := make(chan string, 1)
	go func() {
		defer r.Close()
		scanner := bufio.NewScanner(r)
		if scanner.Scan() {
			line <- scanner.Text()
			return
		}
		line <- ""
	}()

	select {
	case text := <-line:
		return i.handleLine(scriptPath, text, exited)
	case <-ctx.Done():
		return false, i.handleCancel(scriptPath, cmd.Process.Pid, ctx, exited)
	}
}

func (i *Installer) handleLine(scriptPath, text string, exited <-chan error) (bool, error) {
	if strings.HasPrefix(text, "staged ") {
		// sh holds its own open fd to the file; unlinking it here is safe.
		_ = os.Remove(scriptPath)
		return true, nil
	}
	select {
	case <-exited:
	case <-time.After(handoffGrace):
	}
	_ = os.Remove(scriptPath)
	return false, ipcerr.T(ipcerr.CodeInternal, "backend.core.appUpdate.installerStopped.body", map[string]string{"cause": tailError(i.logPath)})
}

func (i *Installer) handleCancel(scriptPath string, pid int, ctx context.Context, exited <-chan error) error {
	_ = procgroup.Kill(pid, syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(cancelGrace):
		_ = procgroup.Kill(pid, syscall.SIGKILL)
		<-exited
	}
	_ = os.Remove(scriptPath)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ipcerr.T(ipcerr.CodeTimeout, "backend.core.appUpdate.timedOut.body", nil)
	}
	return ipcerr.T(ipcerr.CodeCancelled, "backend.core.appUpdate.canceled.body", nil)
}

// tailError returns the last ERROR: line (else the last non-empty line) of logPath's final 4 KiB —
// the failure detail shown in the dialog's error state (§5).
func tailError(logPath string) string {
	data, err := readTail(logPath, 4<<10)
	if err != nil || len(data) == 0 {
		return "see " + logPath
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	for idx := len(lines) - 1; idx >= 0; idx-- {
		if strings.HasPrefix(strings.TrimSpace(lines[idx]), "ERROR:") {
			return strings.TrimSpace(lines[idx])
		}
	}
	for idx := len(lines) - 1; idx >= 0; idx-- {
		if strings.TrimSpace(lines[idx]) != "" {
			return strings.TrimSpace(lines[idx])
		}
	}
	return "see " + logPath
}

func readTail(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := int64(0)
	if info.Size() > n {
		offset = info.Size() - n
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}
