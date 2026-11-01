package main

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appshell"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appwire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/buildinfo"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/filehandlers"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/keepawake"
	"github.com/kirathecat/kira-studio/internal/logging"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/startupfail"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Any files in frontend/dist are embedded into the binary — empty until Part 2's own frontend
// build lands; frontend/dist/index.html is a placeholder so this compiles (P100 Part 1).
//
//go:embed all:frontend/dist
var assets embed.FS

//go:embed all:frontend/dist-mobile
var mobileAssets embed.FS

// Startup order: the argv shims (askpass, memory-mcp, memory-embed) -> config.EnsureLayout ->
// logging.Init/Sweep -> storage.Open (migrates) -> repos.New -> appwire.Build (terminal registry,
// ADE tracker with the Claude Code hooks, task board, keep-awake, the 31 bound services) ->
// application.New (the bound services plus the git stream registration) -> the menu -> the
// startup window list, opened -> app.Run(). No adapters, connections, HTTP/gRPC or DB MCP:
// not this app's module.
func main() {
	if code, ok := appwire.RunArgvShim(os.Args); ok {
		os.Exit(code)
	}

	reporter := startupfail.NewReporter(startupfail.Deps{
		Info: startupfail.Info{
			AppName: "Kira Space",
			Version: buildinfo.Version,
			Home:    config.KiraSpaceHome,
			LogsDir: config.LogsDir,
			DbPath:  config.DbPath,
		},
	})

	// P103 Part 3: repo-root internal/terminal's own two process-constant vars, set once before
	// any Registry.Open.
	terminal.TermProgram = "Kira Space"
	terminal.TermProgramVersion = buildinfo.Version

	startedAt := time.Now()

	if err := config.EnsureLayout(); err != nil {
		reporter.Fatal(startupfail.StepEnsureLayout, err)
	}
	if err := logging.Init(config.LogsDir(), config.IsDev()); err != nil {
		reporter.Fatal(startupfail.StepLogging, err)
	}
	logging.Sweep(config.LogsDir())

	instanceLock := acquireSingleInstance(reporter)
	_ = instanceLock // kept open for the process's lifetime (AcquireLock's own doc); closing it releases the lock.

	db, err := storage.Open()
	if err != nil {
		reporter.Fatal(startupfail.StepStorage, err)
	}
	repositories, err := repos.New(db.DB)
	if err != nil {
		reporter.Fatal(startupfail.StepRepos, err)
	}

	settings, err := repositories.Settings.GetAll()
	if err != nil {
		reporter.Fatal(startupfail.StepSettings, err)
	}
	logging.SetLevel(settings.Advanced.GitLogLevel)

	mobileAssetsFS, err := fs.Sub(mobileAssets, "frontend/dist-mobile")
	if err != nil {
		panic(err) // constant embed path: only a build-time mistake fails here
	}
	rawEmitter, attachEmitter := shell.NewDeferredEmitter()
	browserOpener, attachBrowser := shell.NewDeferredBrowser()
	fileOpener, attachFileOpener := shell.NewDeferredFileOpener()
	rawDialogs, attachDialogs := shell.NewDeferredDialogs()
	wired := appwire.Build(appwire.Options{
		Repos: repositories, DB: db, Cipher: secrets.New(), Emitter: rawEmitter, Browser: browserOpener, FileOpener: fileOpener,
		Dialogs: appshell.NewDialogs(rawDialogs), Locator: platformLocator(),
		KeepAwakeDriver: keepawake.NewPlatformDriver(), MobileAssets: mobileAssetsFS,
		FileHandlers: filehandlers.NewRegistry(),
	})
	windows, quitter := wired.Windows, wired.Quitter

	app := application.New(application.Options{
		Name:        "Kira Space",
		Description: "A git client for macOS\n\nVersion " + buildinfo.Version,
		Services:    append(wired.Bound(), notifyServices(wired)...),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		ShouldQuit: quitter.ShouldQuit,
		OnShutdown: quitter.Shutdown,
		ErrorHandler: func(err error) {
			fatalErr, ok := err.(*application.FatalError)
			if !ok {
				// Wails reports a deferred or portal global shortcut failure only here; match its own
				// message prefixes (wails v3 3.1.0 global_shortcut_manager.go, ..._linux_portal.go).
				if msg := err.Error(); strings.HasPrefix(msg, "failed to register global shortcut") || strings.HasPrefix(msg, "global shortcuts:") {
					bridge.CaptureReportShortcutError(wired.Capture, err)
				}
				return
			}
			platformErrorOnce.Do(func() {
				reporter.ReportPlatform(fatalErr.Unwrap())
			})
		},
	})

	attachEmitter(app)
	wired.StartMobile()
	attachBrowser(app)
	attachFileOpener(app)
	quitter.Attach(app)

	// The sheet a folder-picker dialog attaches to is the window that asked — Current()
	// resolves the real key window on darwin; the registry fallback only matters where Current()
	// can't resolve one (this sandbox's Linux build, mid-startup before any window is focused).
	// Kira Studio's own main.go carries the identical fallback (wireWindowsAndMenu's
	// windowToActOn).
	windowToActOn := func() application.Window {
		if w := app.Window.Current(); w != nil {
			return w
		}
		return windows.Any()
	}
	attachDialogs(app, windowToActOn)

	appshell.RegisterGitStream(app, wired.GitRouter(), wired.CredentialRelay)

	winDeps := shell.WindowOpenerDeps{
		App:        app,
		WindowDeps: shell.WindowDeps{Windows: repositories.Windows, StartedAt: startedAt},
		Windows:    windows, CloseFlush: wired.CloseFlush, Quitter: quitter,
		Terminal: wired.Terminal.Registry, Repo: repositories.Windows,
		Cfg: shell.Config{AppName: "Kira Space", WindowTitle: "Kira Space"},
		Ephemeral: func(key string) bool {
			if bridge.IsHostViewKey(key) || bridge.IsCaptureKey(key) {
				return true
			}
			ok, _ := repositories.AdeReview.IsReviewKey(key)
			return ok
		},
	}
	winDeps.OnWindowClosing = append(winDeps.OnWindowClosing, wired.OpenedFiles.Forget, wired.HostView.Targets.Forget,
		func(key string) { bridge.CaptureForget(wired.Capture, key) })
	openNew := func() { shell.OpenNewWindow(winDeps) }
	// OpenNewWindow is the title bar's "New window" button (P116 G6) — the same action the ⇧⌘N
	// menu command below ties to.
	wired.BindShell(appwire.ShellHooks{
		OpenWindow: func(rec shell.WindowRecord) {
			rec.Bounds = shell.CascadeFrom(app.Window.Current())
			shell.OpenWindow(winDeps, rec)
		},
		OpenNewWindow:      openNew,
		OpenNewWindowKeyed: func(key string) { shell.OpenNewWindowKeyed(winDeps, key) },
		OpenPopup:          func(key string) { shell.OpenPopup(winDeps, key, shell.PopupSpec{Width: 560, Height: 320}) },
		Clipboard:          app.Clipboard.Text,
		Shortcuts:          app.GlobalShortcut,
	})
	// Finder opens (Open With, a default-app double-click) arrive as one event per file, on cold
	// start as well as while running; the coalescer turns a multi-file open into one window.
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		wired.OpenedFiles.Add(e.Context().Filename())
	})
	shell.AttachReopen(app, func() { shell.ReopenWindows(winDeps) })
	shell.AttachActivationTrace(app)
	wired.SetReopen(func() { shell.ReopenWindows(winDeps) })
	// P116 G5: a machine resume's own trigger — Rearm() while held, a no-op while idle.
	shell.AttachSystemWake(app, func() { bridge.KeepAwakeSystemDidWake(wired.KeepAwake) })

	isDev := app.Env.Info().Debug
	app.Menu.Set(shell.BuildMenu(shell.MenuDeps{
		AppName: "Kira Space", IsDev: isDev, Template: appshell.BuildTemplate("Kira Space", isDev),
		OnEmit: wired.Events.Signal, Quit: quitter.RequestQuit, NewWindow: openNew,
	}))

	purgeReviewWindows(repositories)
	purgeHostViewWindows(repositories)
	purgeCaptureWindows(repositories)
	records, err := repositories.Windows.List()
	if err != nil {
		reporter.Fatal(startupfail.StepWindowList, err)
	}
	if len(records) == 0 {
		rec := model.WindowRecord{Key: uuid.NewString(), Order: 0}
		if err := repositories.Windows.Create(rec); err != nil {
			reporter.Fatal(startupfail.StepWindowCreate, err)
		}
		records = []model.WindowRecord{rec}
	}
	for _, rec := range records {
		var bounds *shell.WindowBounds
		if rec.Bounds != nil {
			b := *rec.Bounds
			bounds = &b
		}
		shell.OpenWindow(winDeps, shell.ToWindowRecord(rec.Key, rec.Order, bounds))
	}

	if err := app.Run(); err != nil {
		reporter.Fatal(startupfail.StepRun, err)
	}
}

// platformErrorOnce bounds ErrorHandler to at most one alert per process — Kira Studio's own
// main.go carries the same bound, for the same reason (G29 D7).
var platformErrorOnce sync.Once

// acquireSingleInstance is the app-wide single-instance guard, called before storage.Open: a second
// launch on the same KIRA_SPACE_HOME (open -n, the binary run directly, or a dev build) would open
// the same kira.db and restore the same window rows, last-writer-wins on every table the first
// instance owns. Not acquired means a real other instance owns this home: exit quietly (a supported
// state, not a failure) rather than show a window over a database another process owns. Fatals
// through reporter on a real error; exits the process directly when another instance holds the lock.
func acquireSingleInstance(reporter *startupfail.Reporter) *os.File {
	instanceLock, acquired, err := config.AcquireLock(filepath.Join(config.KiraSpaceHome(), "app.lock"))
	if err != nil {
		reporter.Fatal(startupfail.StepInstanceLock, err)
	}
	if !acquired {
		slog.Info("kira-space: another instance already owns this home; exiting", "scope", "startup")
		os.Exit(0)
	}
	return instanceLock
}

// purgeReviewWindows drops review windows a previous run left behind: they are never restored.
func purgeReviewWindows(repositories *repos.Repos) {
	if err := repositories.AdeReview.PurgeAll(); err != nil {
		slog.Warn("ade: purge review windows", "scope", "ade", "err", err)
	}
}

// purgeCaptureWindows drops capture popup rows a previous run left behind: they are never restored.
func purgeCaptureWindows(repositories *repos.Repos) {
	records, err := repositories.Windows.List()
	if err != nil {
		slog.Warn("capture: list windows to purge", "scope", "capture", "err", err)
		return
	}
	for _, r := range records {
		if !bridge.IsCaptureKey(r.Key) {
			continue
		}
		if err := repositories.Windows.Delete(r.Key); err != nil {
			slog.Warn("capture: purge window", "scope", "capture", "key", r.Key, "err", err)
		}
	}
}

// purgeHostViewWindows drops host view window rows a previous run left behind: they are never restored.
func purgeHostViewWindows(repositories *repos.Repos) {
	records, err := repositories.Windows.List()
	if err != nil {
		slog.Warn("hostview: list windows to purge", "scope", "hostview", "err", err)
		return
	}
	for _, r := range records {
		if !bridge.IsHostViewKey(r.Key) {
			continue
		}
		if err := repositories.Windows.Delete(r.Key); err != nil {
			slog.Warn("hostview: purge window", "scope", "hostview", "key", r.Key, "err", err)
		}
	}
}
