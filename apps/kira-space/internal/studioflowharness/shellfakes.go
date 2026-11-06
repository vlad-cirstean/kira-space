package studioflowharness

import (
	"context"
	"os"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/localauth"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// Browser records OpenURL calls.
type Browser struct {
	mu   sync.Mutex
	urls []string
}

func (b *Browser) OpenURL(url string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.urls = append(b.urls, url)
	return nil
}

// Opened returns a copy of the recorded URLs.
func (b *Browser) Opened() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.urls...)
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

// Dialogs answers the native pickers from queues; an empty queue answers like a cancel ("").
type Dialogs struct {
	mu      sync.Mutex
	saves   []string
	opens   []string
	manys   [][]string
	folders []string
	// Save, Open and Folder record every request, in order.
	Save []bridge.SaveFileRequest
	Open []bridge.OpenFileRequest
	// OpenMany records every OpenFiles request.
	OpenMany []bridge.OpenFilesRequest
	Folder   []bridge.OpenDirectoryRequest
}

// QueueSaveFile queues the path the next SaveFile returns.
func (d *Dialogs) QueueSaveFile(path string) { d.push(&d.saves, path) }

// QueueOpenFile queues the path the next OpenFile returns.
func (d *Dialogs) QueueOpenFile(path string) { d.push(&d.opens, path) }

// QueueOpenFiles queues the paths the next OpenFiles returns.
func (d *Dialogs) QueueOpenFiles(paths ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.manys = append(d.manys, paths)
}

// QueueDirectory queues the path the next OpenDirectory returns.
func (d *Dialogs) QueueDirectory(path string) { d.push(&d.folders, path) }

func (d *Dialogs) push(q *[]string, path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	*q = append(*q, path)
}

func pop(q *[]string) string {
	if len(*q) == 0 {
		return ""
	}
	p := (*q)[0]
	*q = (*q)[1:]
	return p
}

func (d *Dialogs) SaveFile(req bridge.SaveFileRequest) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Save = append(d.Save, req)
	return pop(&d.saves), nil
}

func (d *Dialogs) OpenFile(req bridge.OpenFileRequest) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Open = append(d.Open, req)
	return pop(&d.opens), nil
}

func (d *Dialogs) OpenFiles(req bridge.OpenFilesRequest) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OpenMany = append(d.OpenMany, req)
	if len(d.manys) == 0 {
		return nil, nil
	}
	p := d.manys[0]
	d.manys = d.manys[1:]
	return p, nil
}

func (d *Dialogs) OpenDirectory(req bridge.OpenDirectoryRequest) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Folder = append(d.Folder, req)
	return pop(&d.folders), nil
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

// McpInstaller records installs; it never touches the real claude config.
type McpInstaller struct {
	mu       sync.Mutex
	Installs []McpInstall
}

// McpInstall is one Install call.
type McpInstall struct{ Name, URL, Token string }

func (m *McpInstaller) Status() harness.InstallStatus { return harness.InstallStatus{} }

func (m *McpInstaller) Install(_ context.Context, name, url, token string) harness.InstallResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Installs = append(m.Installs, McpInstall{name, url, token})
	return harness.InstallResult{}
}

func (m *McpInstaller) InstallStdio(context.Context, string, string, []string) harness.InstallResult {
	return harness.InstallResult{}
}

func (m *McpInstaller) Command(name, url, helperPath string) string {
	return claude.New().MCPInstaller().Command(name, url, helperPath)
}

func (m *McpInstaller) StdioCommand(name, command string, args []string) string {
	return claude.New().MCPInstaller().StdioCommand(name, command, args)
}

// OSAuth scripts the OS authentication prompt. By default it is unavailable, so a reveal needs the
// in-app confirmation, as on a Mac with neither biometry nor a login password.
type OSAuth struct {
	mu        sync.Mutex
	available bool
	outcome   localauth.Outcome
	// Prompts records the reason of every evaluate call.
	Prompts []string
}

// Script sets whether OS authentication is available and what its prompt answers.
func (o *OSAuth) Script(available bool, outcome localauth.Outcome) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.available, o.outcome = available, outcome
}

func (o *OSAuth) evaluate(reason string) (localauth.Outcome, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Prompts = append(o.Prompts, reason)
	return o.outcome, nil
}

func (o *OSAuth) isAvailable() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.available
}

func mkdirs(paths ...string) error {
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// WindowFocus records window raises in order and keeps the focus state a raise sets.
type WindowFocus struct {
	mu      sync.Mutex
	keys    []string
	focused map[string]bool
}

func (w *WindowFocus) Focused(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.focused[key]
}

func (w *WindowFocus) Focus(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.focused == nil {
		w.focused = map[string]bool{}
	}
	w.keys = append(w.keys, key)
	w.focused[key] = true
	return true
}

// Blur clears every window's focus, like the user switching to another app.
func (w *WindowFocus) Blur() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.focused = nil
}

// Keys returns a copy of the raised window keys, oldest first.
func (w *WindowFocus) Keys() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.keys...)
}

// ProofShell stands in for the window manager under the proof window: it records opens and
// raises, and a window it opened stays live until Close.
type ProofShell struct {
	mu     sync.Mutex
	live   map[string]bool
	Opened []shell.WindowRecord
	Raised []string
	Titles map[string]string
}

func (p *ProofShell) Keys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	keys := make([]string, 0, len(p.live))
	for k := range p.live {
		keys = append(keys, k)
	}
	return keys
}

func (p *ProofShell) Focus(key string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Raised = append(p.Raised, key)
	return p.live[key]
}

func (p *ProofShell) SetTitle(key, title string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Titles == nil {
		p.Titles = map[string]string{}
	}
	p.Titles[key] = title
	return p.live[key]
}

func (p *ProofShell) Open(rec shell.WindowRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.live == nil {
		p.live = map[string]bool{}
	}
	p.live[rec.Key] = true
	p.Opened = append(p.Opened, rec)
}

// Close ends a window the way the user closing it does.
func (p *ProofShell) Close(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.live, key)
}
