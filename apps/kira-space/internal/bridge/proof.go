package bridge

import (
	"errors"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// The proof channels: changed reaches every window, select reaches the one proof window.
const (
	ChannelProofChanged = proof.ChannelChanged
	ChannelProofSelect  = "kira:proof:select"
)

const (
	maxShotBytes   = 64 << 20
	maxShotBase64  = maxShotBytes/3*4 + 8
	maxFilenameLen = 120
)

var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
)

var unsafeFilenameChars = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

// ProofShell is the window manager ProofService drives; main binds Open once the shell exists.
type ProofShell interface {
	Keys() []string
	Focus(key string) bool
	SetTitle(key, title string) bool
	Open(rec shell.WindowRecord)
}

// ProofWindowTargets remembers which session each proof window was opened for, so a window reads
// it once at boot. Not bound to the renderer.
type ProofWindowTargets struct {
	mu sync.Mutex
	m  map[string]string
}

// Set records the session a window shows first.
func (t *ProofWindowTargets) Set(key, sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]string{}
	}
	t.m[key] = sessionID
}

// Get returns the recorded session of key.
func (t *ProofWindowTargets) Get(key string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	id, ok := t.m[key]
	return id, ok
}

// Forget drops key's record when its window closes.
func (t *ProofWindowTargets) Forget(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, key)
}

// ProofShotLocations persists the custom screenshot parent folder.
type ProofShotLocations interface {
	ProofShotLocation() (string, error)
	SetProofShotLocation(path string) error
}

// ProofRevealer shows a folder in the file manager; configsvc.FileOpener satisfies it.
type ProofRevealer interface {
	RevealFile(path string) error
}

// ProofClipboard puts several images on the system clipboard as separate items.
type ProofClipboard interface {
	MultiImage() bool
	WriteImages(paths []string) error
}

// ProofService is the validation-proof window's surface: history, sessions, steps, view state,
// accepting an agent's edit, the window itself and saving screenshots.
type ProofService struct {
	Svc     *proof.Service
	Dialogs Dialogs
	Windows *repos.WindowsRepo
	Events  appcore.Emitter
	Targets *ProofWindowTargets
	// Shell is nil until main binds the window opener.
	Shell ProofShell
	// Locations stores the custom screenshot folder; Opener reveals a saved folder (nil: reveal unavailable).
	Locations ProofShotLocations
	Opener    ProofRevealer
	// Clipboard writes image batches; nil means no multi-image clipboard.
	Clipboard ProofClipboard

	openMu sync.Mutex
	shots  proofShotFolders
	// clips holds copy batches; lastClip is the dir of the batch now on the clipboard.
	clips    proofShotFolders
	clipMu   sync.Mutex
	lastClip string
}

// ProofSessionArgs names one session.
type ProofSessionArgs struct {
	SessionID string `json:"sessionId"`
}

// ProofRevArgs names one revision of a session.
type ProofRevArgs struct {
	SessionID string `json:"sessionId"`
	Rev       int    `json:"rev"`
}

// ProofStepArgs names one step of a revision.
type ProofStepArgs struct {
	SessionID string `json:"sessionId"`
	Rev       int    `json:"rev"`
	Ordinal   int    `json:"ordinal"`
}

// ProofSetViewStateArgs saves one step's view state.
type ProofSetViewStateArgs struct {
	SessionID string          `json:"sessionId"`
	Rev       int             `json:"rev"`
	Ordinal   int             `json:"ordinal"`
	State     proof.ViewState `json:"state"`
}

// ProofOpenArgs opens the proof window, on SessionID when given.
type ProofOpenArgs struct {
	SessionID string `json:"sessionId,omitempty"`
}

// ProofOpenResult says which window shows the proofs and whether this call created it.
type ProofOpenResult struct {
	Key    string `json:"key"`
	Opened bool   `json:"opened"`
}

// ProofWindowKeyArgs names a window.
type ProofWindowKeyArgs struct {
	Key string `json:"key"`
}

// ProofWindowTarget is the session a proof window was opened for; null when none.
type ProofWindowTarget struct {
	SessionID *string `json:"sessionId"`
}

// ProofSelect is the kira:proof:select payload.
type ProofSelect struct {
	SessionID string `json:"sessionId"`
}

func proofErr(err error) error {
	if err == nil {
		return nil
	}
	var pe *repos.ProofError
	if errors.As(err, &pe) {
		if pe.Code == repos.ProofNotFound {
			return ipcerr.NotFound(pe.Msg)
		}
		return ipcerr.BadRequest(pe.Msg)
	}
	var inv *proof.InvalidError
	if errors.As(err, &inv) {
		return ipcerr.BadRequest(inv.Msg)
	}
	return ipcerr.InternalErr(err)
}

func proofResult[T any](v T, err error) (T, error) { return v, proofErr(err) }

func proofID(id string) error {
	if id == "" || len(id) > 64 {
		return ipcerr.Contract("sessionId is required and at most 64 bytes")
	}
	return nil
}

// Sessions lists closed sessions, newest first.
func (s *ProofService) Sessions() ([]proof.Summary, error) { return proofResult(s.Svc.Sessions()) }

// Session returns one session with its current and pending revisions.
func (s *ProofService) Session(args ProofSessionArgs) (proof.Detail, error) {
	if err := proofID(args.SessionID); err != nil {
		return proof.Detail{}, err
	}
	return proofResult(s.Svc.Session(args.SessionID))
}

// Step returns one step with its query and data.
func (s *ProofService) Step(args ProofStepArgs) (proof.StepDetail, error) {
	if err := proofID(args.SessionID); err != nil {
		return proof.StepDetail{}, err
	}
	return proofResult(s.Svc.Step(args.SessionID, args.Rev, args.Ordinal))
}

// ViewState returns the saved view state of every step of one revision.
func (s *ProofService) ViewState(args ProofRevArgs) ([]proof.ViewStateEntry, error) {
	if err := proofID(args.SessionID); err != nil {
		return nil, err
	}
	return proofResult(s.Svc.ViewStates(args.SessionID, args.Rev))
}

// SetViewState saves one step's scroll position and highlight.
func (s *ProofService) SetViewState(args ProofSetViewStateArgs) error {
	if err := proofID(args.SessionID); err != nil {
		return err
	}
	return proofErr(s.Svc.SetViewState(args.SessionID, args.Rev, args.Ordinal, args.State))
}

// AcceptEdit makes the agent's pending revision current.
func (s *ProofService) AcceptEdit(args ProofSessionArgs) error {
	if err := proofID(args.SessionID); err != nil {
		return err
	}
	return proofErr(s.Svc.AcceptEdit(args.SessionID))
}

// RejectEdit drops the agent's pending revision.
func (s *ProofService) RejectEdit(args ProofSessionArgs) error {
	if err := proofID(args.SessionID); err != nil {
		return err
	}
	return proofErr(s.Svc.RejectEdit(args.SessionID))
}

// Delete removes a session and everything under it.
func (s *ProofService) Delete(args ProofSessionArgs) error {
	if err := proofID(args.SessionID); err != nil {
		return err
	}
	return proofErr(s.Svc.Delete(args.SessionID))
}

// MarkSeen records that the user opened the session.
func (s *ProofService) MarkSeen(args ProofSessionArgs) error {
	if err := proofID(args.SessionID); err != nil {
		return err
	}
	return proofErr(s.Svc.MarkSeen(args.SessionID))
}

// liveKey is the key of the open proof window, "" when none.
func (s *ProofService) liveKey() string {
	for _, k := range s.Shell.Keys() {
		if proof.IsWindowKey(k) {
			return k
		}
	}
	return ""
}

// OpenWindow shows the proof window, creating it or focusing the live one. With a sessionId the
// window selects that session.
func (s *ProofService) OpenWindow(args ProofOpenArgs) (ProofOpenResult, error) {
	if args.SessionID != "" {
		if err := proofID(args.SessionID); err != nil {
			return ProofOpenResult{}, err
		}
	}
	if s.Shell == nil {
		return ProofOpenResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.unavailable.body", nil)
	}
	s.openMu.Lock()
	defer s.openMu.Unlock()
	if key := s.liveKey(); key != "" {
		s.Shell.Focus(key)
		if args.SessionID != "" {
			s.Targets.Set(key, args.SessionID)
			s.Events.EmitTo(key, ChannelProofSelect, ProofSelect{SessionID: args.SessionID})
		}
		s.markOpened(args.SessionID)
		return ProofOpenResult{Key: key}, nil
	}
	records, err := s.Windows.List()
	if err != nil {
		return ProofOpenResult{}, ipcerr.InternalErr(err)
	}
	order := 0
	for _, r := range records {
		if r.Order >= order {
			order = r.Order + 1
		}
	}
	key := proof.WindowKeyPrefix + ids.NewID()
	if err := s.Windows.Create(model.WindowRecord{Key: key, Order: order}); err != nil {
		return ProofOpenResult{}, ipcerr.InternalErr(err)
	}
	if args.SessionID != "" {
		s.Targets.Set(key, args.SessionID)
	}
	s.Shell.Open(shell.WindowRecord{Key: key, Order: order})
	s.Shell.SetTitle(key, i18n.T("backend.studio.proof.window.title", nil))
	s.markOpened(args.SessionID)
	return ProofOpenResult{Key: key, Opened: true}, nil
}

// markOpened records that the user opened sessionID, which withdraws its ready popup. The window
// open already succeeded, so a failure here only logs.
func (s *ProofService) markOpened(sessionID string) {
	if sessionID == "" {
		return
	}
	if err := s.Svc.MarkSeen(sessionID); err != nil {
		slog.Warn("proof: mark opened session seen", "scope", "proof", "session", sessionID, "err", err)
	}
}

// WindowTarget returns the session a proof window was opened for, read once at boot.
func (s *ProofService) WindowTarget(args ProofWindowKeyArgs) (ProofWindowTarget, error) {
	if !proof.IsWindowKey(args.Key) {
		return ProofWindowTarget{}, nil
	}
	if id, ok := s.Targets.Get(args.Key); ok {
		return ProofWindowTarget{SessionID: &id}, nil
	}
	return ProofWindowTarget{}, nil
}

// safeBaseName reduces a suggested name to a plain file name without its image suffix.
func safeBaseName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	switch strings.ToLower(filepath.Ext(base)) {
	case ".jpg", ".jpeg", ".png":
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	base = strings.Trim(unsafeFilenameChars.ReplaceAllString(base, "_"), " .")
	if base == "" {
		base = "proof"
	}
	if r := []rune(base); len(r) > maxFilenameLen {
		base = string(r[:maxFilenameLen])
	}
	return base
}

// safeJpegName is safeBaseName with the .jpg suffix.
func safeJpegName(name string) string { return safeBaseName(name) + ".jpg" }

// safePngName is safeBaseName with the .png suffix.
func safePngName(name string) string { return safeBaseName(name) + ".png" }
