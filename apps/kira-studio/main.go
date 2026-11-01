package main

import (
	"embed"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appshell"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appwire"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/buildinfo"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/keepawake"
	"github.com/kirathecat/kira-studio/internal/logging"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/startupfail"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Any files in frontend/dist are embedded into the binary — built by `bun run build` from
// the real apps/kira-studio/frontend/src (P52 §2.3), not by this scaffold's own removed demo frontend project.
//
//go:embed all:frontend/dist
var assets embed.FS

// main's startup order mirrors src/main/index.ts, with the upgradeLegacySecrets step deleted, not
// ported: config.EnsureLayout -> logging.Init/Sweep -> storage.Open (migrates) -> secrets.New ->
// repos.New + repos.NewSecrets -> Settings.GetAll -> adapterhost.NewRouter -> preconnect.New ->
// connections.New(...).Start -> tree.New -> router.PushCacheConfig -> oplog.New(...).Start ->
// metrics ticker Start -> shell.NewQuitter -> application.New(Services, ShouldQuit, OnShutdown) ->
// attach the deferred emitter and the Quitter to the now-real App -> menu -> engine stream ->
// reopen handler -> the main window -> app.Run(). There is no Node engine child to start any more.
func main() {
	// startupfail moved to repo-root internal/ and can no longer read
	// internal/config/internal/buildinfo itself (Go's internal/ rule — a repo-root package cannot
	// import anything under apps/kira-studio/internal), so this app constructs its own Reporter,
	// carrying its own Info, and threads it through every boot-failure site below instead of
	// startupfail holding a single process-wide default one.
	reporter := startupfail.NewReporter(startupfail.Deps{
		Info: startupfail.Info{
			AppName: "Kira Studio",
			Version: buildinfo.Version,
			Home:    config.KiraHome,
			LogsDir: config.LogsDir,
			DbPath:  config.DbPath,
		},
	})

	core := openCore(reporter)

	// The two adapters below are needed inside the Services list, which is itself an argument to
	// application.New — but both need the *App that New alone produces (P56 §4.11's ordering
	// knot). Each is built "deferred": usable now, wired to the real App by attach() once New has
	// returned, well before Run() lets the renderer or any signal path actually call through it.
	emitter, attachEmitter := shell.NewDeferredEmitter()
	rawDialogs, attachDialogs := shell.NewDeferredDialogs()
	browserOpener, attachBrowser := shell.NewDeferredBrowser()

	w, err := appwire.Build(appwire.Options{
		DB: core.db, Repos: core.repositories, Cipher: core.cipher, Authorizer: core.authorizer,
		Emitter: emitter, Browser: browserOpener, Dialogs: appshell.NewDialogs(rawDialogs),
		// P87 §3/§4: a runtime.GOOS switch — a real caffeinate child on macOS, a documented no-op
		// everywhere else.
		KeepAwakeDriver: keepawake.NewPlatformDriver(),
		AppName:         "Kira Studio", Version: buildinfo.Version,
	})
	if err != nil {
		reporter.Fatal(startupfail.StepSettings, err)
	}

	app := application.New(application.Options{
		Name: "Kira Studio",
		// The macOS About item is `application.About` in internal/shell/menutemplate.go, and Wails
		// renders that role with its own dialog — Name, this Description and the icon, with no
		// version field of its own (pkg/application/menu_manager.go's ShowAbout). So the version
		// goes in the description, which is the only string that dialog will show.
		Description: "A visual database client for macOS\n\nVersion " + buildinfo.Version,
		Services:    append(w.Bound(), notifyServices(w)...),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// P56 D10: closing the last window leaves the app running, matching Electron's
			// default — AttachReopen below is what brings a window back.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		ShouldQuit:   w.Quitter.ShouldQuit,
		OnShutdown:   w.Quitter.Shutdown,
		ErrorHandler: buildErrorHandler(reporter),
	})

	attachEmitter(app)
	attachBrowser(app)
	w.Quitter.Attach(app)

	wireWindowsAndMenu(postAppDeps{app: app, w: w, attachDialogs: attachDialogs, reporter: reporter})

	if err := app.Run(); err != nil {
		reporter.Fatal(startupfail.StepRun, err)
	}
}

// coreOpened is openCore's own result — the boot-order prefix of main's own comment: config layout,
// logging, DB, cipher, authorizer, repos.
type coreOpened struct {
	db           *storage.DB
	cipher       *secrets.Cipher
	authorizer   *localauth.Authorizer
	repositories *repos.Repos
}

// openCore runs main's own boot-order prefix: config.EnsureLayout -> logging.Init/Sweep ->
// storage.Open (migrates) -> secrets.New -> localauth.New -> repos.New. A failure at any
// reporter.Fatal step here exits the process; it never returns an error for the caller to handle.
func openCore(reporter *startupfail.Reporter) coreOpened {
	if err := config.EnsureLayout(); err != nil {
		reporter.Fatal(startupfail.StepEnsureLayout, err)
	}
	if err := logging.Init(config.LogsDir(), config.IsDev()); err != nil {
		reporter.Fatal(startupfail.StepLogging, err)
	}
	logging.Sweep(config.LogsDir())

	db, err := storage.Open()
	if err != nil {
		reporter.Fatal(startupfail.StepStorage, err)
	}

	cipher := secrets.New()
	// P14: constructed beside the cipher so its own startup log records OS-authentication
	// availability the same way cipher's Status does.
	authorizer := localauth.New(time.Now, localauth.Evaluate, localauth.Available)

	repositories, err := repos.New(db.DB)
	if err != nil {
		reporter.Fatal(startupfail.StepRepos, err)
	}

	return coreOpened{db: db, cipher: cipher, authorizer: authorizer, repositories: repositories}
}

// postAppDeps is wireWindowsAndMenu's own argument bundle — every piece main built before `app`
// existed that this block still needs, gathered into one struct since the block itself (dialog
// attach, the three window closures, the menu, the startup window list) is one continuous unit
// that only makes sense once `app` is real.
type postAppDeps struct {
	app *application.App
	w   *appwire.Wired

	attachDialogs func(app *application.App, window func() application.Window)
	reporter      *startupfail.Reporter
}

// wireWindowsAndMenu runs main's own post-`app` block: the dialog attach point -> the engine
// stream -> the three window closures (open/openNew/reopen) -> the reopen/system-wake handlers ->
// the menu -> the startup window list, opened. Nothing here is needed past main's own app.Run()
// call, so this returns nothing.
func wireWindowsAndMenu(d postAppDeps) {
	app := d.app

	// The sheet a Save/Open dialog attaches to is the window that asked — Current()
	// resolves the real key window on darwin ([NSApp keyWindow], application_darwin.go); the
	// registry fallback only matters where Current() can't resolve one (this sandbox's Linux
	// build, mid-startup before any window is focused) so a dialog call still has some live
	// window to attach to rather than none (the single `mainWindow` var this
	// replaces always attached to whichever window was created most recently, not the caller).
	windowToActOn := func() application.Window {
		if w := app.Window.Current(); w != nil {
			return w
		}
		return d.w.Windows.Any()
	}
	d.attachDialogs(app, windowToActOn)

	appshell.RegisterEngineStream(app, d.w.Router)

	deps := shell.WindowOpenerDeps{
		App:        app,
		WindowDeps: shell.WindowDeps{Windows: windowStore{d.w.Repos.Windows}, StartedAt: d.w.StartedAt},
		Windows:    d.w.Windows, CloseFlush: d.w.CloseFlush, Quitter: d.w.Quitter,
		Terminal: d.w.TerminalRegistry, Repo: windowStore{d.w.Repos.Windows},
		OnWindowClosing: d.w.OnWindowClosing,
		Cfg:             shell.Config{AppName: "Kira Studio", WindowTitle: "Kira Studio"},
		// The proof window is never restored and never counts as the last window.
		Ephemeral: proof.IsWindowKey,
	}
	openNew := func() { shell.OpenNewWindow(deps) }
	d.w.BindProofShell(func(rec shell.WindowRecord) {
		rec.Bounds = shell.CascadeFrom(app.Window.Current())
		shell.OpenWindow(deps, rec)
	})
	d.w.WindowsSvc.OpenNewWindow = openNew
	d.w.SetReopen(func() { shell.ReopenWindows(deps) })
	shell.AttachReopen(app, func() { shell.ReopenWindows(deps) })
	shell.AttachActivationTrace(app)
	// P87 §5: a machine resume's own trigger — Rearm() while held, a no-op while idle.
	shell.AttachSystemWake(app, func() { bridge.KeepAwakeSystemDidWake(d.w.KeepAwake) })

	isDev := app.Env.Info().Debug
	app.Menu.Set(shell.BuildMenu(shell.MenuDeps{
		AppName: "Kira Studio", IsDev: isDev, Template: appshell.BuildTemplate("Kira Studio", isDev),
		OnEmit: d.w.Events.Signal, Quit: d.w.Quitter.RequestQuit, NewWindow: openNew,
	}))

	// Startup: one window per stored record (C1's migration guarantees at least the "main" row on
	// a fresh database), in order — the first time this app has ever been able to open more than
	// one.
	purgeProofWindows(d.w.Repos.Windows)
	records, err := d.w.Repos.Windows.List()
	if err != nil {
		d.reporter.Fatal(startupfail.StepWindowList, err)
	}
	if len(records) == 0 {
		rec := model.WindowRecord{Key: uuid.NewString(), Order: 0}
		if err := d.w.Repos.Windows.Create(rec); err != nil {
			d.reporter.Fatal(startupfail.StepWindowCreate, err)
		}
		records = []model.WindowRecord{rec}
	}
	for _, rec := range records {
		var bounds *shell.WindowBounds
		if rec.Bounds != nil {
			b := *rec.Bounds
			bounds = &b
		}
		shell.OpenWindow(deps, shell.ToWindowRecord(rec.Key, rec.Order, bounds))
	}
}

// purgeProofWindows drops proof window rows a previous run left behind: they are never restored.
func purgeProofWindows(windows *repos.WindowsRepo) {
	records, err := windows.List()
	if err != nil {
		slog.Warn("list windows to purge proof rows", "scope", "startup", "err", err)
		return
	}
	for _, rec := range records {
		if proof.IsWindowKey(rec.Key) {
			if err := windows.Delete(rec.Key); err != nil {
				slog.Warn("purge proof window row", "scope", "startup", "key", rec.Key, "err", err)
			}
		}
	}
}

// buildErrorHandler builds main's own application.Options.ErrorHandler: catches the two
// pre-window fatal paths Wails takes itself, without ever returning to main -- application.New's
// own transport-start failure, and webview_window_darwin.go's GetStartURL failure during
// first-window creation, inside Run(). handleError (application.go) prefers ErrorHandler over its
// own logger and calls it synchronously, before Wails' own os.Exit(1) -- so ReportPlatform's alert
// has already run and completed by the time that exit happens, and the returned func must never
// exit itself. A non-fatal handleError call (e.g. RegisterService after Run) passes a plain error
// here, not a *FatalError -- logged by Wails itself already, not alerted a second time. This is the
// one place startupfail needs a pkg/application type; the assertion stays here, in the file that
// already legitimately imports pkg/application (internal/shell/app.go's own documented rule), so
// internal/startupfail imports nothing from pkg/application at all.
func buildErrorHandler(reporter *startupfail.Reporter) func(error) {
	return func(err error) {
		fatalErr, ok := err.(*application.FatalError)
		if !ok {
			return
		}
		platformErrorOnce.Do(func() {
			reporter.ReportPlatform(fatalErr.Unwrap())
		})
	}
}

// windowStore adapts *repos.WindowsRepo to shell.WindowRepo. Kira
// Studio's own model.WindowRecord alone still carries Mode, so it stays its own type
// rather than a plain alias of shell.WindowRecord/appstorage.WindowRecord — this adapter is what
// that costs. Kira Space's identical-minus-Mode WindowRecord is a true alias now, so
// its own main.go passes *repos.WindowsRepo straight through, no adapter needed. WindowBounds is a
// plain alias on both sides already, so no per-field conversion remains here either.
type windowStore struct{ repo *repos.WindowsRepo }

func (w windowStore) SetBounds(key string, b shell.WindowBounds) error {
	return w.repo.SetBounds(key, b)
}

func (w windowStore) List() ([]shell.WindowRecord, error) {
	records, err := w.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]shell.WindowRecord, len(records))
	for i, r := range records {
		out[i] = shell.ToWindowRecord(r.Key, r.Order, r.Bounds)
	}
	return out, nil
}

func (w windowStore) Create(rec shell.WindowRecord) error {
	return w.repo.Create(model.WindowRecord{Key: rec.Key, Order: rec.Order, Bounds: rec.Bounds})
}

func (w windowStore) Delete(key string) error {
	return w.repo.Delete(key)
}

// platformErrorOnce bounds G29 D7's ErrorHandler to at most one alert per process, independent of
// internal/startupfail's own per-Reporter alertOnce bound -- both exist because this handler could,
// in principle, be reached more than once before the process actually exits.
var platformErrorOnce sync.Once
