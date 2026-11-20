package ade

import (
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/fileservice"
)

const (
	folderScanDepth     = 3
	folderRescanDelay   = 500 * time.Millisecond
	folderWatcherEvents = "ade folder watch"
	// folderWatchLatency coalesces FSEvents callbacks: the stream sees every build write below the
	// folder, and the rescan debounce makes the extra lag irrelevant.
	folderWatchLatency = time.Second
)

type folderScan struct {
	repos []string // repo roots, to import
	dirs  []string // visited non-repo directories, to watch
}

// scanFolder walks root to depth 3. It stops at a repo root (a .git directory), skips hidden
// directories, node_modules, bare repositories and linked worktrees (a .git file).
func scanFolder(root string) folderScan {
	var out folderScan
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if repo, stop := stopsFolderWalk(dir); stop {
			if repo {
				out.repos = append(out.repos, dir)
			}
			return
		}
		out.dirs = append(out.dirs, dir)
		if depth >= folderScanDepth {
			return
		}
		entries, err := fileservice.Host().ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name
			if !e.Dir || skipFolderName(name) {
				continue
			}
			walk(filepath.Join(dir, name), depth+1)
		}
	}
	walk(filepath.Clean(root), 0)
	return out
}

func skipFolderName(name string) bool { return strings.HasPrefix(name, ".") || name == "node_modules" }

// stopsFolderWalk reports whether a scan stops at dir: a repo root (a .git directory, repo set),
// a linked worktree (a .git file) or a bare repository.
func stopsFolderWalk(dir string) (repo, stop bool) {
	if info, err := fileservice.Host().Lstat(filepath.Join(dir, ".git")); err == nil {
		return info.Dir, true
	}
	return false, isBareRepo(dir)
}

// skipFolderDir is the watch's SkipDir: the rule scanFolder applies to every directory below the root.
func skipFolderDir(dir string) bool {
	_, stop := stopsFolderWalk(dir)
	return stop || skipFolderName(filepath.Base(dir))
}

func isBareRepo(dir string) bool {
	for _, n := range []string{"HEAD", "objects", "refs"} {
		if _, err := fileservice.Host().Lstat(filepath.Join(dir, n)); err != nil {
			return false
		}
	}
	return true
}

// folderWatcher keeps one watched folder's repos imported: any entry created or renamed under the
// folder (or a visited non-repo directory) triggers a rescan after 500ms of quiet.
type folderWatcher struct {
	board *TaskBoard
	path  string
	w     *fileservice.Watcher
	deb   *fileservice.Debouncer
	done  chan struct{}
}

func (b *TaskBoard) startFolderWatch(path string) {
	b.folderWMu.Lock()
	defer b.folderWMu.Unlock()
	if _, ok := b.folderW[path]; ok {
		return
	}
	w, err := fileservice.Host().Watch(fileservice.WatchOptions{Names: []string{path}, MaxDepth: folderScanDepth, SkipDir: skipFolderDir, Latency: folderWatchLatency})
	if err != nil {
		slog.Warn(folderWatcherEvents, "scope", "ade", "path", path, "err", err)
		return
	}
	fw := &folderWatcher{board: b, path: path, w: w, done: make(chan struct{})}
	fw.deb = fileservice.NewDebouncer(folderRescanDelay, 0, fw.rescan)
	b.folderW[path] = fw
	go fw.loop()
	// Catch repos created while the app was not running.
	fw.deb.Schedule()
}

func (b *TaskBoard) stopFolderWatch(path string) {
	b.folderWMu.Lock()
	fw := b.folderW[path]
	delete(b.folderW, path)
	b.folderWMu.Unlock()
	if fw != nil {
		fw.stop()
	}
}

func (f *folderWatcher) rescan() {
	imported, err := f.board.importFolder(f.board.ctx, f.path)
	if err != nil {
		slog.Warn(folderWatcherEvents, "scope", "ade", "path", f.path, "err", err)
		return
	}
	if len(imported) > 0 {
		f.board.notifyRepos()
	}
}

func (f *folderWatcher) loop() {
	defer close(f.done)
	for ev := range f.w.Events() {
		if ev.Rescan || ev.Op.Has(fileservice.Create) || ev.Op.Has(fileservice.Rename) {
			f.deb.Schedule()
		}
	}
}

func (f *folderWatcher) stop() {
	f.deb.Stop()
	_ = f.w.Close()
	<-f.done
}
