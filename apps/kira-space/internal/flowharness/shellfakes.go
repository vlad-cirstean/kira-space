package flowharness

import (
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// Browser records OpenURL calls.
type Browser struct {
	mu   sync.Mutex
	URLs []string
	Err  error
}

func (b *Browser) OpenURL(url string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.URLs = append(b.URLs, url)
	return b.Err
}

// Opened returns a copy of the recorded URLs.
func (b *Browser) Opened() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.URLs...)
}

// FileOpener records the config files opened and revealed.
type FileOpener struct {
	mu       sync.Mutex
	opened   []string
	revealed []string
}

func (f *FileOpener) OpenFile(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, path)
	return nil
}

func (f *FileOpener) RevealFile(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revealed = append(f.revealed, path)
	return nil
}

// OpenedFiles returns a copy of the recorded OpenFile paths.
func (f *FileOpener) OpenedFiles() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.opened...)
}

// RevealedFiles returns a copy of the recorded RevealFile paths.
func (f *FileOpener) RevealedFiles() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.revealed...)
}

// Dialogs answers the native pickers from queues; an empty queue answers like a cancel.
type Dialogs struct {
	mu          sync.Mutex
	directories []string
	files       [][]string
	single      []string
	// Requests records every call, in order.
	Directory []bridge.OpenDirectoryRequest
	Files     []bridge.OpenFilesRequest
	File      []bridge.OpenFileRequest
}

// AnswerFile queues the path the next OpenFile returns.
func (d *Dialogs) AnswerFile(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.single = append(d.single, path)
}

func (d *Dialogs) OpenFile(req bridge.OpenFileRequest) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.File = append(d.File, req)
	if len(d.single) == 0 {
		return "", nil
	}
	p := d.single[0]
	d.single = d.single[1:]
	return p, nil
}

// AnswerDirectory queues the path the next OpenDirectory returns.
func (d *Dialogs) AnswerDirectory(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.directories = append(d.directories, path)
}

// AnswerFiles queues the paths the next OpenFiles returns.
func (d *Dialogs) AnswerFiles(paths ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.files = append(d.files, paths)
}

func (d *Dialogs) OpenDirectory(req bridge.OpenDirectoryRequest) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Directory = append(d.Directory, req)
	if len(d.directories) == 0 {
		return "", nil
	}
	p := d.directories[0]
	d.directories = d.directories[1:]
	return p, nil
}

func (d *Dialogs) OpenFiles(req bridge.OpenFilesRequest) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Files = append(d.Files, req)
	if len(d.files) == 0 {
		return nil, nil
	}
	p := d.files[0]
	d.files = d.files[1:]
	return p, nil
}

// WindowManager records the window-manager calls the app makes.
type WindowManager struct {
	mu         sync.Mutex
	Opened     []shell.WindowRecord
	Closed     []string
	Focused    []string
	Titles     map[string]string
	NewWindows int
	// OpenedPopups records the keys of capture popups opened.
	OpenedPopups []string
	// OpenedKeyed records the keys of windows opened for an external file open.
	OpenedKeyed []string
	// Known is the set of window keys Close and Focus report true for.
	Known      map[string]bool
	appFocused bool
}

// SetAppFocused sets what AppFocused reports; default false.
func (w *WindowManager) SetAppFocused(v bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appFocused = v
}

// AppFocused reports whether a test marked any app window key.
func (w *WindowManager) AppFocused() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.appFocused
}

func newWindowManager() *WindowManager {
	return &WindowManager{Titles: map[string]string{}, Known: map[string]bool{}}
}

func (w *WindowManager) OpenWindow(rec shell.WindowRecord) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Opened = append(w.Opened, rec)
	w.Known[rec.Key] = true
}

func (w *WindowManager) CloseWindow(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Closed = append(w.Closed, key)
	known := w.Known[key]
	delete(w.Known, key)
	return known
}

func (w *WindowManager) FocusWindow(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Focused = append(w.Focused, key)
	return w.Known[key]
}

func (w *WindowManager) SetWindowTitle(key, title string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Titles[key] = title
}

// openKeyed records a keyed window open and marks the key known.
func (w *WindowManager) openKeyed(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.OpenedKeyed = append(w.OpenedKeyed, key)
	w.Known[key] = true
}

// openPopup records a capture popup open and marks the key known.
func (w *WindowManager) openPopup(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.OpenedPopups = append(w.OpenedPopups, key)
	w.Known[key] = true
}

// openNew counts OpenNewWindow invocations into NewWindows.
func (w *WindowManager) openNew() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.NewWindows++
}

// KeepAwakeDriver records the power assertion instead of holding one.
type KeepAwakeDriver struct {
	mu       sync.Mutex
	held     bool
	Acquires int
	Releases int
}

func (k *KeepAwakeDriver) Acquire() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.held = true
	k.Acquires++
	return nil
}

func (k *KeepAwakeDriver) Release() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.held = false
	k.Releases++
}

func (k *KeepAwakeDriver) Supported() bool { return true }

// Held reports whether an assertion is currently held.
func (k *KeepAwakeDriver) Held() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.held
}
