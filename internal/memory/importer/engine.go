package importer

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/internal/filelock"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/memory"
)

const maxPaths = 100

// Options configure an Engine. Launcher is required.
type Options struct {
	// Launcher starts the runner process (SpawnLauncher) when work needs one and none holds the lock.
	Launcher Launcher
	// Available fails with a user-facing message when Claude Code cannot run; nil means always available.
	Available func() error
	OnChange  func() // runs after a state change this Engine made; must not block
	Now       func() time.Time
}

// Engine is the control side of bulk imports: it writes job rows and makes sure a runner exists to
// work them. The scheduler itself lives in Runner, one process per memory.db, so jobs run to the end
// with the app closed.
type Engine struct {
	st       *store
	opts     Options
	lockPath string
}

// Open prepares the control side over ms. Jobs a dead runner left scanning or running get a runner
// again, which then pauses the running ones as interrupted.
func Open(ms *memory.Store, opts Options) (*Engine, error) {
	if opts.Launcher == nil {
		return nil, fmt.Errorf("importer: launcher required")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	db, err := ms.DB()
	if err != nil {
		return nil, err
	}
	e := &Engine{st: &store{db: db, now: func() string { return kiratime.FormatISO(opts.Now()) }}, opts: opts, lockPath: lockPath(ms)}
	if active, err := e.st.hasActiveJobs(); err != nil {
		return nil, err
	} else if active {
		if err := e.ensureRunner(); err != nil {
			slog.Warn("memory import: start runner", "scope", "memory", "err", err)
		}
	}
	return e, nil
}

// Close releases the launcher when it holds resources (an in-process runner).
func (e *Engine) Close() error {
	if c, ok := e.opts.Launcher.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (e *Engine) changed() {
	if e.opts.OnChange != nil {
		e.opts.OnChange()
	}
}

func (e *Engine) available() error {
	if e.opts.Available == nil {
		return nil
	}
	return e.opts.Available()
}

// ensureRunner launches a runner unless one holds the lock. The probe releases the lock at once.
func (e *Engine) ensureRunner() error {
	f, free, err := filelock.TryLock(e.lockPath)
	if err != nil {
		return fmt.Errorf("Cannot start the import runner: %w", err)
	}
	if !free {
		return nil
	}
	_ = f.Close()
	if err := e.opts.Launcher.Launch(); err != nil {
		return fmt.Errorf("Cannot start the import runner: %w", err)
	}
	return nil
}

// runWork makes sure a runner will see the job; a spawn failure fails the job with the reason.
func (e *Engine) runWork(jobID string) error {
	err := e.ensureRunner()
	if err != nil {
		if ferr := e.st.failJob(jobID, err.Error()); ferr != nil {
			slog.Error("memory import: fail job", "scope", "memory", "err", ferr)
		}
		e.changed()
	}
	return err
}

// Create records a job over paths (files and folders); the runner scans them in the background and
// the job reaches awaiting when the scan ends. Nothing runs until Start, except an AutoStart job
// within the chunk cap. An MCP job identical to an active one returns that job.
func (e *Engine) Create(a CreateArgs) (Job, error) {
	if n := len(a.Paths); n < 1 || n > maxPaths {
		return Job{}, fmt.Errorf("%w: pick 1 to %d files or folders", memory.ErrInvalid, maxPaths)
	}
	if a.Provenance.Origin != "" {
		p := a.Provenance
		if err := memory.CheckProvenance(p.Origin, p.SourceTag, p.Scope, p.Project); err != nil {
			return Job{}, err
		}
		if p.Session != "" && !memory.ValidSession(p.Session) {
			return Job{}, fmt.Errorf("%w: session must be 1 to 128 characters from A-Z, a-z, 0-9 and ._:-", memory.ErrInvalid)
		}
	}
	if err := memory.CheckJobGroups(a.Groups); err != nil {
		return Job{}, err
	}
	id, err := e.st.createJob(ids.NewID(), a)
	if err != nil {
		return Job{}, err
	}
	e.changed()
	if err := e.runWork(id); err != nil {
		d, derr := e.st.jobDetail(id)
		if derr != nil {
			return Job{}, derr
		}
		return d.Job, nil
	}
	d, err := e.st.jobDetail(id)
	return d.Job, err
}

// Start begins an awaiting job. A non-nil a.Provenance and a.WriteDoc replace the job's own.
func (e *Engine) Start(id string, a StartArgs) error {
	if err := e.available(); err != nil {
		return err
	}
	if a.Provenance != nil {
		p := a.Provenance
		if err := memory.CheckProvenance(p.Origin, p.SourceTag, p.Scope, p.Project); err != nil {
			return err
		}
	}
	if err := e.st.startJob(id, a); err != nil {
		return err
	}
	e.changed()
	return e.runWork(id)
}

// Resume continues a paused job.
func (e *Engine) Resume(id string) error {
	if err := e.available(); err != nil {
		return err
	}
	if err := e.st.resumeJob(id); err != nil {
		return err
	}
	e.changed()
	return e.runWork(id)
}

// Pause stops a running job; the runner kills its in-flight calls within a tick and their chunks go
// back to pending.
func (e *Engine) Pause(id string) error {
	if err := e.st.pause(id, "Paused"); err != nil {
		return err
	}
	e.changed()
	return nil
}

// Cancel ends a running or paused job for good. Stored memories stay.
func (e *Engine) Cancel(id string) error {
	if err := e.st.cancelJob(id); err != nil {
		return err
	}
	e.changed()
	return nil
}

// Discard deletes an awaiting job without running it.
func (e *Engine) Discard(id string) error {
	if err := e.st.discardJob(id); err != nil {
		return err
	}
	e.changed()
	return nil
}

// Dismiss hides a finished job from the list. Its files still count for unchanged detection.
func (e *Engine) Dismiss(id string) error {
	if err := e.st.dismissJob(id); err != nil {
		return err
	}
	e.changed()
	return nil
}

// RetryFailed retries every failed file of a job.
func (e *Engine) RetryFailed(id string) error {
	if err := e.available(); err != nil {
		return err
	}
	if _, err := e.st.retryFailed(id); err != nil {
		return err
	}
	e.changed()
	return e.runWork(id)
}

// RetryFile retries one failed file: only its failed chunks rerun, or only the store step.
func (e *Engine) RetryFile(fileID string) error {
	if err := e.available(); err != nil {
		return err
	}
	jobID, err := e.st.retryFile(fileID)
	if err != nil {
		return err
	}
	e.changed()
	return e.runWork(jobID)
}

func (e *Engine) Jobs() ([]Job, error)             { return e.st.listJobs() }
func (e *Engine) Job(id string) (JobDetail, error) { return e.st.jobDetail(id) }

// maxFactIDs bounds the stored fact ids a status reply lists per file.
const maxFactIDs = 50

// JobWithFacts is Job plus up to 50 stored fact ids per finished file.
func (e *Engine) JobWithFacts(id string) (JobDetail, error) {
	d, err := e.st.jobDetail(id)
	if err != nil {
		return d, err
	}
	for i := range d.Files {
		if d.Files[i].State != FileDone {
			continue
		}
		if d.Files[i].FactIDs, err = e.st.factIDs(d.Files[i].ID, maxFactIDs); err != nil {
			return d, err
		}
	}
	return d, nil
}

// DefaultLogPath is where the spawned runner appends its stderr.
func DefaultLogPath(ms *memory.Store) string {
	return filepath.Join(filepath.Dir(ms.Path()), logFileName)
}
