// Package flowharness boots Kira Studio's real composition root (internal/appwire) over a temporary
// home for flow tests at the IPC boundary. Real: SQLite, the cipher, the adapter host and op log,
// httpclient, grpcclient, PTYs and login shells, the Docker engine, and local HTTP/HTTPS/gRPC
// servers (servers.go). Faked, each an OS seam: native dialogs, the keep-awake driver, the OS
// authentication prompt, the claude MCP installer and the window manager. Settings stay at their
// defaults; a test changes one only through SettingsService.Set, the way the UI does.
//
// Only _test.go files import this package. A flow test is:
//
//	func TestSend(t *testing.T) {
//		app := flowharness.New(t)
//		srv := flowharness.HTTP(t)
//		res, err := app.W.Http.Send(context.Background(), bridge.HttpSendArgs{ ... URL: srv.URL + "/echo" })
//		...
//		app.Events.Wait(t, "kira:api:dataChanged", nil, 5*time.Second)
//	}
package flowharness

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appwire"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/flowtest"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Event and Events record every emit toward a window; see internal/flowtest.
type (
	Event  = flowtest.Event
	Events = flowtest.Events
)

// EnvComplete turns on the complete flow suite.
const EnvComplete = flowtest.EnvComplete

// Complete skips a test unless the complete suite is on.
func Complete(t testing.TB) { flowtest.Complete(t) }

type options struct {
	noSecretStore   bool
	smartTimeout    time.Duration
	scheduleTimeout time.Duration
	clock           scriptruns.Clock
	dockerHubAPI    string
	dockerExtra     []docker.Option
	kubeExtra       []kubernetes.Option
}

// WithoutSecretStore boots without KIRA_INSECURE_SECRETS, so the cipher is unavailable on Linux.
func WithoutSecretStore() Opt { return func(o *options) { o.noSecretStore = true } }

// Opt tweaks New.
type Opt func(*options)

// WithSmartTimeout replaces every smart script's own timeout.
func WithSmartTimeout(d time.Duration) Opt { return func(o *options) { o.smartTimeout = d } }

// WithScheduleTimeout replaces every headless scheduled run's own timeout.
func WithScheduleTimeout(d time.Duration) Opt { return func(o *options) { o.scheduleTimeout = d } }

// WithClock drives the scheduler and the run times with c.
func WithClock(c scriptruns.Clock) Opt { return func(o *options) { o.clock = c } }

// WithDockerHubAPI points the Docker image registry check at base instead of Docker Hub.
func WithDockerHubAPI(base string) Opt { return func(o *options) { o.dockerHubAPI = base } }

// WithDockerOptions appends options to the Docker service (restore tracker timing and boot probe).
func WithDockerOptions(opts ...docker.Option) Opt {
	return func(o *options) { o.dockerExtra = append(o.dockerExtra, opts...) }
}

// WithKubernetesOptions appends options to the Kubernetes service (list limits, environment).
func WithKubernetesOptions(opts ...kubernetes.Option) Opt {
	return func(o *options) { o.kubeExtra = append(o.kubeExtra, opts...) }
}

// kubeEnv hands the Kubernetes service the test process environment instead of probing the user's
// login shell.
func kubeEnv() kubernetes.Env {
	return kubernetes.Env{Path: os.Getenv("PATH"), Kubeconfig: os.Getenv("KUBECONFIG")}
}

// App is one booted Kira Studio.
type App struct {
	t *testing.T
	// Home is the isolated HOME (also what DefaultCwd and the docker config dir read); KiraHome is
	// KIRA_HOME. BinDir is the PATH-first directory holding the fake claude and fake-mcp, FakeDir
	// where the fake agent records its calls.
	Home, KiraHome, BinDir, FakeDir string

	W            *appwire.Wired
	Events       *Events
	Dialogs      *Dialogs
	Browser      *Browser
	KeepAwake    *KeepAwakeDriver
	McpInstaller *McpInstaller
	OSAuth       *OSAuth
	Focus        *WindowFocus
	ProofWindows *ProofShell

	mu         sync.Mutex
	newWindows int
	db         *storage.DB
	stopped    bool
	opts       options
}

// New boots the app and tears it down at cleanup, in main's order.
func New(t *testing.T, opts ...Opt) *App {
	t.Helper()
	root := t.TempDir()
	a := &App{
		t: t, Home: filepath.Join(root, "home"), KiraHome: filepath.Join(root, "kira"),
		BinDir: filepath.Join(root, "bin"), FakeDir: filepath.Join(root, "fake"),
		Events: flowtest.NewEvents(), Dialogs: &Dialogs{}, Browser: &Browser{}, KeepAwake: &KeepAwakeDriver{},
		McpInstaller: &McpInstaller{}, OSAuth: &OSAuth{}, Focus: &WindowFocus{}, ProofWindows: &ProofShell{},
	}
	for _, opt := range opts {
		opt(&a.opts)
	}
	if err := mkdirs(a.Home, a.BinDir, a.FakeDir); err != nil {
		t.Fatal(err)
	}
	a.writeBin()
	t.Setenv("KIRA_HOME", a.KiraHome)
	t.Setenv("HOME", a.Home)
	t.Setenv("PATH", a.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(fakeagent.EnvDir, a.FakeDir)
	t.Setenv(fakeagent.EnvScenario, fakeagent.Scenario{}.Encode())
	t.Setenv("TZ", "UTC")
	// Linux has no keychain; a no-op on macOS, where the real Keychain-backed cipher is used.
	if a.opts.noSecretStore {
		t.Setenv("KIRA_INSECURE_SECRETS", "")
	} else {
		t.Setenv("KIRA_INSECURE_SECRETS", "1")
	}
	useDockerHost(t)
	a.build()
	t.Cleanup(a.stop)
	return a
}

// writeBin links the test binary as claude and fake-mcp (Main plays them by name) and writes the
// login-shell profiles that put BinDir first on PATH (login shells reorder it; P150).
func (a *App) writeBin() {
	self, err := os.Executable()
	if err != nil {
		a.t.Fatal(err)
	}
	for _, tool := range []string{"claude", "fake-mcp"} {
		if err := os.Symlink(self, filepath.Join(a.BinDir, tool)); err != nil {
			a.t.Fatal(err)
		}
	}
	profile := fmt.Sprintf("export PATH=%q:\"$PATH\"\n", a.BinDir)
	for _, f := range []string{".bash_profile", ".zprofile", ".profile"} {
		if err := os.WriteFile(filepath.Join(a.Home, f), []byte(profile), 0o644); err != nil {
			a.t.Fatal(err)
		}
	}
}

// Scenario installs the fake claude's scenario; processes started afterwards read it.
func (a *App) Scenario(s fakeagent.Scenario) {
	a.t.Helper()
	a.t.Setenv(fakeagent.EnvScenario, s.Encode())
}

func (a *App) build() {
	a.t.Helper()
	if err := config.EnsureLayout(); err != nil {
		a.t.Fatalf("ensure layout: %v", err)
	}
	db, err := storage.Open()
	if err != nil {
		a.t.Fatalf("storage.Open: %v", err)
	}
	r, err := repos.New(db.DB)
	if err != nil {
		a.t.Fatalf("repos.New: %v", err)
	}
	cipher := secrets.New()
	if !a.opts.noSecretStore && !cipher.Status().Available {
		a.t.Fatalf("cipher unavailable: %+v", cipher.Status())
	}
	w, err := appwire.Build(appwire.Options{
		DB: db, Repos: r, Cipher: cipher,
		Authorizer: localauth.New(time.Now, a.OSAuth.evaluate, a.OSAuth.isAvailable),
		Emitter:    a.Events, Browser: a.Browser, Dialogs: a.Dialogs, KeepAwakeDriver: a.KeepAwake, McpInstaller: a.McpInstaller,
		AppName: "Kira Studio", Version: "flowtest", SmartTimeout: a.opts.smartTimeout,
		ScheduleTimeout: a.opts.scheduleTimeout, Clock: a.opts.clock, DockerHubAPI: a.opts.dockerHubAPI, DockerExtra: a.opts.dockerExtra, WindowFocus: a.Focus,
		KubernetesExtra: append([]kubernetes.Option{kubernetes.WithEnv(kubeEnv)}, a.opts.kubeExtra...),
	})
	if err != nil {
		a.t.Fatalf("appwire.Build: %v", err)
	}
	w.Proof.Shell = a.ProofWindows
	w.WindowsSvc.OpenNewWindow = func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.newWindows++
	}
	a.db, a.W, a.stopped = db, w, false
	// The cookie jar is process-global in httpclient.
	if err := w.Http.ClearCookies(); err != nil {
		a.t.Fatalf("clear cookies: %v", err)
	}
}

// DB is the live database, for tests that plant or inspect rows no bound call reaches.
func (a *App) DB() *sql.DB { return a.db.DB }

// NewWindows is how many times the app asked the shell for a new window.
func (a *App) NewWindows() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.newWindows
}

// CloseWindow runs what closing a window runs in the shell: the terminal registry's CloseWindow,
// then each OnWindowClosing hook (internal/shell/openwindow.go).
func (a *App) CloseWindow(key string) {
	a.W.TerminalRegistry.CloseWindow(key)
	for _, hook := range a.W.OnWindowClosing {
		hook(key)
	}
}

func (a *App) stop() {
	if a.stopped {
		return
	}
	a.stopped = true
	a.W.BeforeFlush()
	a.W.Teardown()
}

// Quit runs the app-quit sequence: BeforeFlush, Teardown, then ServiceShutdown on every bound
// service that has one, as Wails does. Services and streams from before are dead afterwards.
func (a *App) Quit(t testing.TB) {
	t.Helper()
	a.stop()
	svcs := a.W.Bound()
	for i := len(svcs) - 1; i >= 0; i-- {
		if s, ok := svcs[i].Instance().(application.ServiceShutdown); ok {
			if err := s.ServiceShutdown(); err != nil {
				t.Errorf("ServiceShutdown: %v", err)
			}
		}
	}
}

// Restart tears the app down and rebuilds it on the same home, as a relaunch does. References to
// the old Wired are dead.
func (a *App) Restart(t testing.TB) {
	t.Helper()
	a.Quit(t)
	a.build()
}
