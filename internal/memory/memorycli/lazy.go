package memorycli

import (
	"os"
	"sync"

	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
)

// lazyIngest opens the import engine on the first ingest tool call. Opening creates and migrates
// memory.db, so a bare session start must not do it. The runner the engine launches is a detached
// process: it outlives this session. A failed open is returned and retried on the next call.
type lazyIngest struct {
	store     *memory.Store
	available func() error

	mu     sync.Mutex
	engine *importer.Engine
}

func (l *lazyIngest) open() (*importer.Engine, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.engine != nil {
		return l.engine, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	e, err := importer.Open(l.store, importer.Options{
		Launcher:  importer.SpawnLauncher{Executable: exe, LogPath: importer.DefaultLogPath(l.store)},
		Available: l.available,
	})
	if err != nil {
		return nil, err
	}
	l.engine = e
	return e, nil
}

func (l *lazyIngest) Create(a importer.CreateArgs) (importer.Job, error) {
	e, err := l.open()
	if err != nil {
		return importer.Job{}, err
	}
	return e.Create(a)
}

func (l *lazyIngest) JobWithFacts(id string) (importer.JobDetail, error) {
	e, err := l.open()
	if err != nil {
		return importer.JobDetail{}, err
	}
	return e.JobWithFacts(id)
}

func (l *lazyIngest) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.engine != nil {
		_ = l.engine.Close()
	}
}
