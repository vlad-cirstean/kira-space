package appstorage

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// ErrInvalidTab marks a tab record ValidateTab refused; SaveWindowTabs reports it as E_CONTRACT.
var ErrInvalidTab = errors.New("invalid tab")

// ErrTabOwnedElsewhere means a save named a tab id that another window owns.
var ErrTabOwnedElsewhere = errors.New("appstorage: tab id belongs to another window")

// IsJSONObject reports whether raw is valid JSON whose top-level value is an object — both apps'
// own storage/model.TabRecord require their own `state` column to round-trip a Record<...>-shaped
// value, never a bare array or scalar.
func IsJSONObject(raw []byte) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	_, ok := v.(map[string]any)
	return ok
}

// TabFields is the envelope both apps' own TabRecord types share — ValidateTab checks only this
// subset; each model keeps its own extra fields (Kira Studio's ConnectionID) and its own kind
// table around it.
type TabFields struct {
	ID          string
	Path        string
	Kind        string
	State       json.RawMessage
	WorkspaceID *string
}

// ValidateTab asserts the same envelope repos.TabsRepo.List already enforces on read (kind is
// renderable, state is a JSON object), plus the non-empty identity fields no SQL constraint
// covers. isRenderable is the caller's own RenderableTabKinds lookup; requireWorkspace reports
// whether that kind needs a non-empty WorkspaceID — Kira Studio requires it only for "terminal",
// Kira Space for every kind.
func ValidateTab(t TabFields, isRenderable, requireWorkspace func(kind string) bool) error {
	if err := validateTab(t, isRenderable, requireWorkspace); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTab, err)
	}
	return nil
}

func validateTab(t TabFields, isRenderable, requireWorkspace func(kind string) bool) error {
	if t.ID == "" {
		return fmt.Errorf("model: tab: id is required")
	}
	if !ids.IsClientID(t.ID) {
		return fmt.Errorf("model: tab: id must be 1 to %d characters of letters, digits, _ : -", ids.MaxClientIDBytes)
	}
	if t.Path == "" {
		return fmt.Errorf("model: tab %q: path is required", t.ID)
	}
	if !isRenderable(t.Kind) {
		return fmt.Errorf("model: tab %q: unrecognised kind %q", t.ID, t.Kind)
	}
	if !IsJSONObject(t.State) {
		return fmt.Errorf("model: tab %q: state must be a JSON object", t.ID)
	}
	if requireWorkspace(t.Kind) && (t.WorkspaceID == nil || *t.WorkspaceID == "") {
		return fmt.Errorf("model: tab %q: workspaceId is required", t.ID)
	}
	return nil
}

// WindowExistsChecker is the one method CheckWindow/SaveWindowTabs need from an app's own
// *repos.WindowsRepo.
type WindowExistsChecker interface {
	Exists(key string) (bool, error)
}

// CheckWindow is List/Save's shared guard: reject a windowKey that names no `windows` row with a
// real E_CONTRACT, rather than letting Save surface TabsRepo.Save's own raw FOREIGN KEY constraint
// failure — List has no such constraint to fall back on, so it runs the same check up front purely
// for a consistent error.
func CheckWindow(windows WindowExistsChecker, windowKey string) error {
	ok, err := windows.Exists(windowKey)
	if err != nil {
		return ipcerr.Internal(err.Error())
	}
	if !ok {
		return ipcerr.Contract("unknown window: %s", windowKey)
	}
	return nil
}

// TabsSaver is the one method SaveWindowTabs needs from an app's own *repos.TabsRepo — Save's own
// signature, generic over each app's own storage/model.TabRecord (a distinct type per app, P103
// §2.3, so this can't be a plain interface without a type parameter).
type TabsSaver[T any] interface {
	Save(windowKey string, records []T) error
}

// TabsLister is the one method ListWindowTabs needs from an app's own *repos.TabsRepo.
type TabsLister[T any] interface {
	List(windowKey string) ([]T, error)
}

// SaveWindowTabs is both apps' own bridge.TabsService.Save (P107 I2-6): CheckWindow's guard, then
// persist.
func SaveWindowTabs[T any](windows WindowExistsChecker, tabs TabsSaver[T], windowKey string, records []T) error {
	if err := CheckWindow(windows, windowKey); err != nil {
		return err
	}
	if err := tabs.Save(windowKey, records); err != nil {
		if errors.Is(err, ErrTabOwnedElsewhere) {
			return ipcerr.Contract("tab id belongs to another window")
		}
		if errors.Is(err, ErrInvalidTab) {
			return ipcerr.Contract("%s", err.Error())
		}
		return ipcerr.Internal(err.Error())
	}
	return nil
}

// ListWindowTabs is both apps' own bridge.TabsService.List (P113 G4): CheckWindow's guard, then the
// list itself.
func ListWindowTabs[T any](windows WindowExistsChecker, tabs TabsLister[T], windowKey string) ([]T, error) {
	if err := CheckWindow(windows, windowKey); err != nil {
		return nil, err
	}
	return ipcerr.InternalResult(tabs.List(windowKey))
}
