// Package logging installs the process-wide slog default that internal/storage/repos and
// internal/enginehost already log through (P53/P54's late-bound seam) — a daily-rolling file
// under the app home logs folder, mirroring src/main/log.ts's electron-log configuration.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// dailyWriter re-resolves the dated log filename on every Write and reopens the file when the
// date changes, matching electron-log's resolvePathFn being evaluated per write — a long-running
// session's log rolls over at local midnight rather than staying pinned to the file open at
// startup.
type dailyWriter struct {
	mu       sync.Mutex
	now      func() time.Time
	dir      string
	openDate string
	file     *os.File
}

func newDailyWriter(dir string, now func() time.Time) *dailyWriter {
	return &dailyWriter{dir: dir, now: now}
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	date := w.now().Format("2006-01-02")
	if date != w.openDate {
		if w.file != nil {
			_ = w.file.Close()
		}
		path := filepath.Join(w.dir, "kira-"+date+".log")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return 0, err
		}
		w.file = f
		w.openDate = date
	}
	return w.file.Write(p)
}

// level backs the process default handler's threshold — a slog.LevelVar so SetLevel can adjust
// verbosity at runtime (advanced.gitLogLevel, P72 §9.2) instead of it being fixed at Init. Its
// zero value is slog.LevelInfo (both are 0), matching this app's own default.
var level slog.LevelVar

// Init installs a slog handler writing to dir/kira-YYYY-MM-DD.log as the process default, so every
// existing slog.Default() call in storage/repos and enginehost lands there with zero change to
// those packages. In a dev build (isDev) the same records also go to stderr, mirroring
// electron-log's console transport (silenced only under NODE_ENV=test, which has no Go analogue
// since tests here use their own captured logger, per the helpers_test.go).
//
// dir/isDev are explicit arguments, not read from internal/config directly, since this package was
// hoisted to repo-root internal/ — it can no longer import either app's own
// internal/config (Go's internal/ visibility rule). Each app's own main.go passes
// config.LogsDir()/config.IsDev() in.
func Init(dir string, isDev bool) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	var w io.Writer = newDailyWriter(dir, time.Now)
	if isDev {
		w = io.MultiWriter(w, os.Stderr)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: &level})))
	return nil
}

// SetLevel adjusts the process-wide log verbosity at runtime — advanced.gitLogLevel's own actual
// mechanism. "off" needs a threshold
// above every standard level: slog's Enabled check is level >= threshold, and slog.LevelError is
// the highest standard level, so setting the threshold to LevelError would still let Error
// records through. An unrecognized value is a no-op, never a panic — it can only reach here from
// an already-validated settings write (model.ValidLogLevel).
func SetLevel(s string) {
	switch s {
	case "off":
		level.Set(slog.LevelError + 4)
	case "error":
		level.Set(slog.LevelError)
	case "warn":
		level.Set(slog.LevelWarn)
	case "info":
		level.Set(slog.LevelInfo)
	case "debug":
		level.Set(slog.LevelDebug)
	}
}
