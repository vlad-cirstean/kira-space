// Package startupfail is the whole of the logic: showing the user a native alert, and getting a
// log record written, for any of the boot-sequence failures apps/kira-space/main.go hits before
// any window (and, for five of the nine sites, before any *application.App at all) exists.
//
// Why this package exists rather than reaching for what is already in the tree: Wails v3's own
// dialog API is a nil-pointer dereference at every one of these sites — MessageDialog.Show's
// InvokeSync (pkg/application/dialogs.go) dispatches through globalApplication.dispatchOnMainThread
// (pkg/application/mainthread.go), and globalApplication is nil until application.New returns
// (assigned inside New, pkg/application/application.go), while a.impl — also dereferenced along
// that path — is assigned only inside Run(). Every boot-failure site in main.go runs before one or
// both of those exist. So the mechanism here is a plain os/exec spawn of macOS's own
// /usr/bin/osascript, the same argv-only, no-shell discipline internal/toolexec already uses
// for its spawns: title and body always travel as argv items, never interpolated into any
// AppleScript source string — the error text this package renders can be anything an
// underlying library chose to put in an error, including quotes, backslashes and newlines, and none
// of it is trusted to be safe inside a script.
//
// Pure Go, no cgo: the whole point over a cgo NSAlert/CFUserNotification shim is that this
// package cross-compiles and unit-tests for darwin/arm64 in an ordinary Linux CI container
// (verified: GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build succeeds), where a cgo darwin file
// would be compiled by nobody until a human builds on a Mac.
package startupfail

// Step names one of this app's pre-window boot-failure sites — the vocabulary Classify's table (D5)
// switches on. Adding a new Step without adding it to Classify's switch is a compile error: the
// switch has no default case (see classify.go).
type Step string

const (
	// StepEnsureLayout is config.EnsureLayout() failing to create/chmod the app home (and its
	// logs subdirectory) — main.go's very first fallible call, before logging.Init has run.
	StepEnsureLayout Step = "ensureLayout"
	// StepLogging is logging.Init() failing to install the slog file handler — the second call,
	// also before any durable logging exists.
	StepLogging Step = "logging"
	// StepStorage is storage.Open() failing — either the schema-too-new refusal (recognised via
	// the schemaTooNew duck-typed interface, D1) or any other database-open failure.
	StepStorage Step = "storage"
	// StepRepos is repos.New() failing — the app's data-access layer (five db.Prepare calls)
	// failing to prepare against the just-opened (and just-migrated) database.
	StepRepos Step = "repos"
	// StepSettings is the boot-time Settings.GetAll() read (the cache-budget read) failing.
	StepSettings Step = "settings"
	// StepWindowList is the startup Windows.List() read failing.
	StepWindowList Step = "windowList"
	// StepWindowCreate is Windows.Create() failing when a fresh database has no window row yet.
	StepWindowCreate Step = "windowCreate"
	// StepInstanceLock is the app-wide single-instance flock (P108 Part 20 F7) failing with a real
	// I/O error (permission denied, disk full) — not the "another copy already owns this lock" case,
	// which is a clean os.Exit(0) before this Step is ever reached.
	StepInstanceLock Step = "instanceLock"
	// StepRun is app.Run() itself returning an error.
	StepRun Step = "run"
	// StepPlatform is Wails' own pre-window fatal path — application.New's transport-start
	// failure, or webview_window_darwin.go's GetStartURL failure during first-window creation —
	// caught via application.Options.ErrorHandler, never reachable from any log.Fatal* site
	// in main.go because Wails calls os.Exit itself without ever returning to main.
	StepPlatform Step = "platform"
	// StepOldStudio is a running Kira Studio.app found at launch: P537 folded it into this app,
	// and both would write the same data.
	StepOldStudio Step = "oldStudio"
)
