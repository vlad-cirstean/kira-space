package bridge

import (
	"bytes"
	"encoding/base64"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/google/renameio/v2"
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
	maxPngBytes    = 64 << 20
	maxPngBase64   = maxPngBytes/3*4 + 8
	maxFilenameLen = 120
)

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

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

// ProofService is the validation-proof window's surface: history, sessions, steps, view state,
// accepting an agent's edit, the window itself and saving a screenshot.
type ProofService struct {
	Svc     *proof.Service
	Dialogs Dialogs
	Windows *repos.WindowsRepo
	Events  appcore.Emitter
	Targets *ProofWindowTargets
	// Shell is nil until main binds the window opener.
	Shell ProofShell

	openMu sync.Mutex
	shots  proofShotFolders
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

// ProofSavePngArgs saves a screenshot.
type ProofSavePngArgs struct {
	Filename  string `json:"filename"`
	PngBase64 string `json:"pngBase64"`
}

// ProofSavePngResult reports where a screenshot went; saved is false when the user cancelled.
type ProofSavePngResult struct {
	Saved bool    `json:"saved"`
	Path  *string `json:"path"`
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
	return ProofOpenResult{Key: key, Opened: true}, nil
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

// SavePng asks where to save a screenshot and writes it there.
func (s *ProofService) SavePng(args ProofSavePngArgs) (ProofSavePngResult, error) {
	if len(args.PngBase64) > maxPngBase64 {
		return ProofSavePngResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.tooLarge.body", nil)
	}
	data, err := base64.StdEncoding.DecodeString(args.PngBase64)
	if err != nil || len(data) > maxPngBytes || !bytes.HasPrefix(data, pngMagic) {
		return ProofSavePngResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.proof.notPng.body", nil)
	}
	path, err := s.Dialogs.SaveFile(SaveFileRequest{Directory: downloadsDir(), Filename: safePngName(args.Filename)})
	if err != nil {
		return ProofSavePngResult{}, ipcerr.InternalErr(err)
	}
	if path == "" {
		return ProofSavePngResult{}, nil
	}
	if err := renameio.WriteFile(path, data, 0o644); err != nil {
		return ProofSavePngResult{}, ipcerr.InternalErr(err)
	}
	return ProofSavePngResult{Saved: true, Path: &path}, nil
}

// safePngName reduces a suggested name to a plain file name ending in .png.
func safePngName(name string) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(name)), ".png")
	base = strings.Trim(unsafeFilenameChars.ReplaceAllString(base, "_"), " .")
	if base == "" {
		base = "proof"
	}
	if r := []rune(base); len(r) > maxFilenameLen {
		base = string(r[:maxFilenameLen])
	}
	return base + ".png"
}
