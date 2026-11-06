package bridge

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/configsync"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// ChannelConfigSyncChanged broadcasts the sync Status after any change.
const ChannelConfigSyncChanged = "kira:configSync:changed"

const configSyncKeyPrefix = "configsync-"

// IsConfigSyncKey reports whether key names the shared-config review window.
func IsConfigSyncKey(key string) bool { return strings.HasPrefix(key, configSyncKeyPrefix) }

// ConfigSyncService is the bound face of configsync.Service plus the review window.
type ConfigSyncService struct {
	Sync *configsync.Service
	// Window hooks; fields so they stay off the bound surface.
	OpenWindow     func(key string)
	FocusWindow    func(key string) bool
	SetWindowTitle func(key, title string)

	mu     sync.Mutex
	openMu sync.Mutex
	review string
}

// Forget drops the review window's key when it closes.
func (s *ConfigSyncService) Forget(key string) {
	s.mu.Lock()
	if s.review == key {
		s.review = ""
	}
	s.mu.Unlock()
}

func syncErr(err error) error {
	if err == nil {
		return nil
	}
	var ce *configsync.Error
	if !errors.As(err, &ce) {
		return ipcerr.Internal(err.Error())
	}
	code := ipcerr.CodeInternal
	switch ce.Kind {
	case configsync.ErrBadRequest:
		code = ipcerr.CodeBadRequest
	case configsync.ErrNotFound:
		code = ipcerr.CodeNotFound
	case configsync.ErrConflict:
		code = ipcerr.CodeConflict
	case configsync.ErrInternal:
	}
	e := ipcerr.T(code, ce.Key, ce.Params)
	if ce.Cause != nil && ce.Kind == configsync.ErrInternal {
		e.Message += ": " + ce.Cause.Error()
	}
	return e
}

func syncResult[T any](v T, err error) (T, error) {
	if err != nil {
		var zero T
		return zero, syncErr(err)
	}
	return v, nil
}

type ConfigSyncSetRepoArgs struct {
	URL       string `json:"url"`
	Branch    string `json:"branch"`
	WindowKey string `json:"windowKey"`
}

type ConfigSyncMinutesArgs struct {
	Minutes int `json:"minutes"`
}

type ConfigSyncModeArgs struct {
	Module string `json:"module"`
	Mode   string `json:"mode"`
}

type ConfigSyncDeactivateArgs struct {
	Module string `json:"module"`
	Unit   string `json:"unit"`
	Off    bool   `json:"off"`
}

type ConfigSyncFetchArgs struct {
	WindowKey string `json:"windowKey"`
}

type ConfigSyncApplyArgs struct {
	Token   string            `json:"token"`
	Choices map[string]string `json:"choices"`
}

type ConfigSyncDismissArgs struct {
	Sha string `json:"sha"`
}

type ConfigSyncWindowArgs struct {
	WindowKey string `json:"windowKey"`
}

func (s *ConfigSyncService) Status() (configsync.Status, error) { return s.Sync.Status(), nil }

func (s *ConfigSyncService) SetRepo(args ConfigSyncSetRepoArgs) (configsync.Status, error) {
	return syncResult(s.Sync.SetRepo(context.Background(), args.URL, args.Branch, args.WindowKey))
}

func (s *ConfigSyncService) RemoveRepo() (configsync.Status, error) {
	return syncResult(s.Sync.RemoveRepo())
}

func (s *ConfigSyncService) SetFetchMinutes(args ConfigSyncMinutesArgs) (configsync.Status, error) {
	return syncResult(s.Sync.SetFetchMinutes(args.Minutes))
}

func (s *ConfigSyncService) SetMode(args ConfigSyncModeArgs) (configsync.Status, error) {
	return syncResult(s.Sync.SetMode(args.Module, args.Mode))
}

func (s *ConfigSyncService) SetDeactivated(args ConfigSyncDeactivateArgs) (configsync.Status, error) {
	return syncResult(s.Sync.SetDeactivated(args.Module, args.Unit, args.Off))
}

func (s *ConfigSyncService) FetchNow(args ConfigSyncFetchArgs) (configsync.Status, error) {
	return syncResult(s.Sync.FetchNow(context.Background(), args.WindowKey))
}

func (s *ConfigSyncService) Preview() (configsync.Preview, error) {
	return syncResult(s.Sync.Preview(context.Background()))
}

func (s *ConfigSyncService) Apply(args ConfigSyncApplyArgs) (configsync.ApplyResult, error) {
	return syncResult(s.Sync.Apply(context.Background(), args.Token, args.Choices))
}

func (s *ConfigSyncService) Dismiss(args ConfigSyncDismissArgs) (configsync.Status, error) {
	return syncResult(s.Sync.Dismiss(args.Sha))
}

// IsReviewWindow reports whether windowKey is the shared-config review window.
func (s *ConfigSyncService) IsReviewWindow(args ConfigSyncWindowArgs) (bool, error) {
	if args.WindowKey == "" {
		return false, ipcerr.Required("windowKey")
	}
	return IsConfigSyncKey(args.WindowKey), nil
}

// OpenReview opens the review window, or focuses the one already open.
func (s *ConfigSyncService) OpenReview() error {
	if s.OpenWindow == nil {
		return nil
	}
	s.openMu.Lock()
	defer s.openMu.Unlock()
	s.mu.Lock()
	key := s.review
	s.mu.Unlock()
	if key != "" && s.FocusWindow != nil && s.FocusWindow(key) {
		return nil
	}
	key = configSyncKeyPrefix + ids.NewID()
	s.mu.Lock()
	s.review = key
	s.mu.Unlock()
	s.OpenWindow(key)
	if s.SetWindowTitle != nil {
		s.SetWindowTitle(key, i18n.T("backend.space.configSync.window.title", nil))
	}
	return nil
}
