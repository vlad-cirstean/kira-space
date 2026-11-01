// Package flowharness boots Kira Space's real composition root (internal/appwire) over a temporary
// home for flow tests at the IPC boundary. Real: git, SQLite, the git stream, PTYs, file
// watchers, the phone HTTP server, the hooks and askpass sockets. Faked, each an OS or third-party
// seam with no git in it: the claude and gh CLIs, native dialogs, the browser, the power assertion,
// LAN detection and the window manager. Settings stay at their defaults; a test changes one only
// through SettingsService.Set, the way the UI does.
//
// Only _test.go files import this package. A flow test is:
//
//	func TestSomething(t *testing.T) {
//		app := flowharness.New(t)
//		repo := app.NewRepo("proj")
//		repo.Commit("first", map[string]string{"a.txt": "a\n"})
//		gs := app.OpenGitStream()
//		var res gitrpc.RepoOpenResult
//		gs.MustRequest("repo.open", map[string]any{"path": repo.Dir}, &res)
//		...
//	}
package flowharness

import (
	"database/sql"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appwire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/filehandlers"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/azure"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/github"
	githostproviders "github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/providers"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/lannet"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
	"github.com/kirathecat/kira-studio/internal/agentsandbox/sandboxtest"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/internal/memory/embed"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type options struct {
	noSecretStore   bool
	smartTimeout    time.Duration
	sandbox         agentsandbox.Enforcer
	scheduleTimeout time.Duration
	clock           scriptruns.Clock
	noGh            bool
	issueTracker    *jira.Options
	azure           *azure.Options
	gitHosts        []githost.Provider
	noClaude        bool
	fileHandlers    filehandlers.Registry
	trackerGrace    time.Duration
	rebaseTimeout   time.Duration
	branchWait      time.Duration
	mobilePoll      time.Duration
	seed            func(*App)
	embedSpec       *embed.Spec
	provider        harness.Provider
	crSync          gitsession.CRSyncConfig
	wayland         bool
}

// WithoutSecretStore boots without KIRA_INSECURE_SECRETS, so the cipher is unavailable on Linux.
func WithoutSecretStore() Opt { return func(o *options) { o.noSecretStore = true } }

// Opt tweaks New.
type Opt func(*options)

// WithFileHandlers swaps the default-app seam for a fake; without it the build reports default
// apps as unsupported.
func WithFileHandlers(r filehandlers.Registry) Opt { return func(o *options) { o.fileHandlers = r } }

// WithoutGh leaves gh off PATH entirely, system installs included (claude stays the fake).
func WithoutGh() Opt { return func(o *options) { o.noGh = true } }

// WithIssueTracker replaces the Jira provider's acli seams. Without it the harness finds no acli, so a
// flow never reaches the host's real one. ChangeRequests is the harness's own.
func WithIssueTracker(o jira.Options) Opt { return func(opts *options) { opts.issueTracker = &o } }

// WithAzure replaces the Azure Repos provider's az seams. Without it the harness finds no az, so a flow
// never reaches the host's real one.
func WithAzure(o azure.Options) Opt { return func(opts *options) { opts.azure = &o } }

// WithGitHost registers an extra code-host provider after GitHub and Azure Repos.
func WithGitHost(p githost.Provider) Opt {
	return func(o *options) { o.gitHosts = append(o.gitHosts, p) }
}

// WithChangeRequestSync replaces the sync config. Flows default to a manual one: no startup run, no
// timer, no run on a refs change, a 1 ns min gap and checks interval, and a short backoff, so a
// test drives each run through branch.syncChangeRequests.
func WithChangeRequestSync(c gitsession.CRSyncConfig) Opt { return func(o *options) { o.crSync = c } }

// WithWayland makes the capture hotkey binding report a Wayland session (the portal decides the
// trigger), whatever the process environment says.
func WithWayland() Opt { return func(o *options) { o.wayland = true } }

// WithoutClaude leaves claude off PATH entirely, system installs included.
func WithoutClaude() Opt { return func(o *options) { o.noClaude = true } }

// WithTrackerGrace shortens the ADE tracker's grace window, the wait after a TUI session spawns
// before a missing process reads as stopped (30s by default).
func WithTrackerGrace(d time.Duration) Opt { return func(o *options) { o.trackerGrace = d } }

// WithSmartTimeout replaces every smart script's own timeout.
// WithSandbox swaps the OS sandbox enforcer for a test one.
func WithSandbox(e agentsandbox.Enforcer) Opt { return func(o *options) { o.sandbox = e } }

func WithSmartTimeout(d time.Duration) Opt { return func(o *options) { o.smartTimeout = d } }

// WithScheduleTimeout replaces every headless scheduled run's own timeout.
func WithScheduleTimeout(d time.Duration) Opt { return func(o *options) { o.scheduleTimeout = d } }

// WithClock drives the scheduler and the run times with c.
func WithClock(c scriptruns.Clock) Opt { return func(o *options) { o.clock = c } }

// WithMobilePoll sets how often the phone server's network supervisor re-checks (10s by default).
func WithMobilePoll(d time.Duration) Opt { return func(o *options) { o.mobilePoll = d } }

// WithBranchRequestWait bounds how long an agent's branch request waits for the user.
func WithBranchRequestWait(d time.Duration) Opt { return func(o *options) { o.branchWait = d } }

// WithRebaseTimeout bounds an ADE rebase run (20 minutes by default).
func WithRebaseTimeout(d time.Duration) Opt { return func(o *options) { o.rebaseTimeout = d } }

// WithEmbedSpec replaces the semantic-search model, so a test serves its files from a fake host.
func WithEmbedSpec(s embed.Spec) Opt { return func(o *options) { o.embedSpec = &s } }

// WithProvider registers a test-only agent provider next to claude.
func WithProvider(p harness.Provider) Opt { return func(o *options) { o.provider = p } }

// WithSeed runs seed on the isolated homes before the first boot. Startup work that runs in the
// background (sweeps) would race a seed written after New.
func WithSeed(seed func(*App)) Opt { return func(o *options) { o.seed = seed } }

// noGhLocator finds no gh anywhere, system installs included.
type noGhLocator struct{}

// noAcliLocator finds no acli anywhere.
type noAcliLocator struct{}

// noAzLocator finds no az anywhere.
type noAzLocator struct{}

func (noAzLocator) Locate() (string, []string, bool) { return "", []string{"az (on PATH)"}, false }

func (noAcliLocator) Locate() (string, []string, bool) { return "", []string{"acli (on PATH)"}, false }

func (noGhLocator) Locate() (string, []string, bool) { return "", []string{"gh (on PATH)"}, false }

// App is one booted Kira Space.
type App struct {
	t *testing.T
	// Root is the short temp root (not t.TempDir: macOS paths there exceed the 104-byte unix socket
	// limit for the hooks and askpass sockets). Home is the isolated HOME, Work the
	// directory test repositories are built in, BinDir the PATH-first fake tool directory, FakeDir
	// where the fake agent records its calls.
	Root, Home, SpaceHome, MemoryHome, Work, BinDir, FakeDir string

	W         *appwire.Wired
	Events    *Events
	Browser   *Browser
	Dialogs   *Dialogs
	WindowMgr *WindowManager
	KeepAwake *KeepAwakeDriver
	// Clipboard and Shortcuts fake the capture hotkey's two OS seams.
	Clipboard *Clipboard
	Shortcuts *Shortcuts

	db       *storage.DB
	repos    *repos.Repos
	stopped  bool
	built    int
	scenario fakeagent.Scenario
	opts     options
}

// New boots the app; the test is skipped without git on PATH. Everything is torn down in main's
// order at cleanup.
func New(t *testing.T, opts ...Opt) *App {
	t.Helper()
	testx.SkipWithoutGit(t)
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.crSync == (gitsession.CRSyncConfig{}) {
		o.crSync = gitsession.CRSyncConfig{
			Disabled: true, MinGap: time.Nanosecond, ChecksInterval: time.Nanosecond,
			BackoffInitial: 20 * time.Millisecond, BackoffMax: 100 * time.Millisecond,
		}
	}
	root, err := os.MkdirTemp("", "ksf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if o.noClaude {
		t.Cleanup(claude.SetSystemPaths())
	}
	a := &App{
		t: t, Root: root,
		Home: filepath.Join(root, "home"), SpaceHome: filepath.Join(root, "space"),
		MemoryHome: filepath.Join(root, "memory"), Work: filepath.Join(root, "work"),
		BinDir: filepath.Join(root, "bin"), FakeDir: filepath.Join(root, "fake"),
		Events: newEvents(), Browser: &Browser{}, Dialogs: &Dialogs{}, WindowMgr: newWindowManager(),
		KeepAwake: &KeepAwakeDriver{}, Clipboard: &Clipboard{}, Shortcuts: newShortcuts(),
	}
	for _, d := range []string{a.Home, a.Work, a.BinDir, a.FakeDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a.opts = o
	a.writeHome()
	a.writeBin(o)
	t.Setenv("KIRA_HOME", filepath.Join(root, "studio"))
	t.Setenv("KIRA_SPACE_HOME", a.SpaceHome)
	t.Setenv("KIRA_MEMORY_HOME", a.MemoryHome)
	t.Setenv("HOME", a.Home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(a.Home, ".gitconfig"))
	t.Setenv("PATH", a.BinDir+string(os.PathListSeparator)+a.pathTail(o))
	t.Setenv("TZ", "UTC")
	if a.opts.noSecretStore {
		t.Setenv("KIRA_INSECURE_SECRETS", "")
	} else {
		t.Setenv("KIRA_INSECURE_SECRETS", "1")
	}
	t.Setenv(fakeagent.EnvDir, a.FakeDir)
	t.Setenv(fakeagent.EnvScenario, a.scenario.Encode())
	if o.seed != nil {
		o.seed(a)
	}
	a.build()
	t.Cleanup(a.stop)
	return a
}

// writeHome writes the isolated user config: a git identity and the PTY login-shell PATH fix
// (login shells reorder PATH, so a prefix set in the environment is lost; P150).
func (a *App) writeHome() {
	gitconfig := "[user]\n\tname = Test Author\n\temail = author@example.com\n[init]\n\tdefaultBranch = main\n" +
		"[commit]\n\tgpgsign = false\n[protocol \"file\"]\n\tallow = always\n"
	if err := os.WriteFile(filepath.Join(a.Home, ".gitconfig"), []byte(gitconfig), 0o644); err != nil {
		a.t.Fatal(err)
	}
	profile := fmt.Sprintf("export PATH=%q:\"$PATH\"\n", a.BinDir)
	for _, f := range []string{".bash_profile", ".zprofile", ".profile"} {
		if err := os.WriteFile(filepath.Join(a.Home, f), []byte(profile), 0o644); err != nil {
			a.t.Fatal(err)
		}
	}
}

// writeBin links the test binary as claude and gh: Main plays the fake agent when it runs under
// either name.
func (a *App) writeBin(o options) {
	self, err := os.Executable()
	if err != nil {
		a.t.Fatal(err)
	}
	tools := []string{"fake-mcp", fakeagent.Persona}
	if !o.noClaude {
		tools = append(tools, "claude")
	}
	if !o.noGh {
		tools = append(tools, "gh")
	}
	for _, tool := range tools {
		if err := os.Symlink(self, filepath.Join(a.BinDir, tool)); err != nil {
			a.t.Fatal(err)
		}
	}
}

// pathTail is the inherited PATH, or for WithoutGh and WithoutClaude a directory of wrappers for
// every inherited executable except gh and claude.
func (a *App) pathTail(o options) string {
	inherited := os.Getenv("PATH")
	if !o.noGh && !o.noClaude {
		return inherited
	}
	clean := filepath.Join(a.Root, "pathbin")
	if err := os.MkdirAll(clean, 0o755); err != nil {
		a.t.Fatal(err)
	}
	for _, dir := range filepath.SplitList(inherited) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if name == "gh" || name == "claude" || e.IsDir() {
				continue
			}
			target := filepath.Join(dir, name)
			if st, err := os.Stat(target); err != nil || st.Mode()&0o111 == 0 {
				continue
			}
			wrapper := filepath.Join(clean, name)
			if _, err := os.Lstat(wrapper); err == nil {
				continue
			}
			_ = os.WriteFile(wrapper, []byte("#!/bin/sh\nexec "+shellQuote(target)+" \"$@\"\n"), 0o755)
		}
	}
	return clean
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func loopbackNetwork() lannet.Network {
	addr := netip.MustParsePrefix("127.0.0.1/8")
	return lannet.Network{
		Interface: "lo", Addr: addr,
		Identity: lannet.Identity{
			Subnet: addr.Masked(), RouterIP: netip.MustParseAddr("127.0.0.254"), RouterMAC: "02:00:00:00:00:01",
		},
	}
}

func (a *App) build() {
	a.t.Helper()
	db, err := storage.Open()
	if err != nil {
		a.t.Fatalf("storage.Open: %v", err)
	}
	r, err := repos.New(db.DB)
	if err != nil {
		a.t.Fatalf("repos.New: %v", err)
	}
	a.db, a.repos, a.stopped = db, r, false
	if a.opts.sandbox == nil {
		// Same on every OS: flows never depend on the host's real Seatbelt.
		a.opts.sandbox = &sandboxtest.Fake{Dir: a.t.TempDir(), NotEnforced: true}
	}
	var ghOptions github.Options
	if a.opts.noGh {
		ghOptions.Locator = noGhLocator{}
	}
	var registry *harness.Registry
	if a.opts.provider != nil {
		registry = providers.New(a.opts.provider)
	}
	az := azure.Options{Locator: noAzLocator{}}
	if a.opts.azure != nil {
		az = *a.opts.azure
	}
	tracker := jira.Options{Locator: noAcliLocator{}}
	if a.opts.issueTracker != nil {
		tracker = *a.opts.issueTracker
	}
	a.W = appwire.Build(appwire.Options{
		Providers: registry, IssueTracker: tracker,
		GitHosts: githostproviders.New(ghOptions, append([]githost.Provider{azure.New(az)}, a.opts.gitHosts...)...), TrackerGrace: a.opts.trackerGrace, RebaseTimeout: a.opts.rebaseTimeout, BranchRequestWait: a.opts.branchWait, SmartTimeout: a.opts.smartTimeout, Sandbox: a.opts.sandbox, ScheduleTimeout: a.opts.scheduleTimeout, Clock: a.opts.clock, EmbedSpec: a.opts.embedSpec, MobilePoll: a.opts.mobilePoll, ChangeRequestSync: a.opts.crSync,
		Repos: r, DB: db, Emitter: a.Events, Browser: a.Browser, Dialogs: a.Dialogs,
		Locator: gitclient.NewHostLocator(), KeepAwakeDriver: a.KeepAwake, FileHandlers: a.opts.fileHandlers,
		MobileAssets: fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>phone</title>")}},
		MobileIsLAN:  func(ip netip.Addr) bool { return ip.IsLoopback() },
		MobileDetect: func() (lannet.Network, error) { return loopbackNetwork(), nil },
		MobileFind: func(id lannet.Identity) (lannet.Network, error) {
			n := loopbackNetwork()
			if id != n.Identity {
				return lannet.Network{}, lannet.ErrAway
			}
			return n, nil
		},
	})
	a.Shortcuts.reset()
	a.W.BindShell(appwire.ShellHooks{
		OpenPopup: a.WindowMgr.openPopup, Clipboard: a.Clipboard.Text, Shortcuts: a.Shortcuts,
		Wayland:    func() bool { return a.opts.wayland },
		OpenWindow: a.WindowMgr.OpenWindow, OpenNewWindow: a.WindowMgr.openNew, OpenNewWindowKeyed: a.WindowMgr.openKeyed,
		CloseWindow: a.WindowMgr.CloseWindow, FocusWindow: a.WindowMgr.FocusWindow,
		SetWindowTitle: a.WindowMgr.SetWindowTitle, AppFocused: a.WindowMgr.AppFocused,
	})
	a.built++
}

func (a *App) stop() {
	if a.stopped {
		return
	}
	a.stopped = true
	a.W.BeforeFlush()
	a.W.Teardown()
}

// Restart tears the app down and rebuilds it on the same home, as a relaunch does. Open streams
// and service references from before are dead.
func (a *App) Restart() {
	a.t.Helper()
	a.stop()
	a.build()
}

// Unregister drops the test provider from the next boot (Restart), as an uninstalled provider is.
func (a *App) Unregister() { a.opts.provider = nil }

// DB is the live database, for tests that plant or inspect rows no bound call reaches.
func (a *App) DB() *sql.DB { return a.db.DB }

// Scenario installs the fake agent's scenario; processes started afterwards read it.
func (a *App) Scenario(s fakeagent.Scenario) {
	a.t.Helper()
	a.scenario = s
	a.t.Setenv(fakeagent.EnvScenario, s.Encode())
}

// NewRepo creates an empty repository under Work.
func (a *App) NewRepo(name string) *Repo {
	a.t.Helper()
	return NewRepo(a.t, filepath.Join(a.Work, name))
}

// NewBare creates an empty bare remote under Work.
func (a *App) NewBare(name string) *Bare {
	a.t.Helper()
	return BareRemote(a.t, filepath.Join(a.Work, name+".git"))
}

// NewHistory builds a generated history under Work.
func (a *App) NewHistory(name string, spec HistorySpec) *Repo {
	a.t.Helper()
	return History(a.t, filepath.Join(a.Work, name), spec)
}
