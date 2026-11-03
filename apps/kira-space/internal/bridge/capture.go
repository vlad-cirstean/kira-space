package bridge

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// CaptureWindowKeyPrefix marks a clipboard capture popup key: never restored.
const CaptureWindowKeyPrefix = "capture-"

// IsCaptureKey reports whether key names a capture popup.
func IsCaptureKey(key string) bool { return strings.HasPrefix(key, CaptureWindowKeyPrefix) }

// Capture push channels: the item replaced in an open popup (EmitTo), and the hotkey state (Emit).
const (
	ChannelCaptureItem   = "kira:capture:item"
	ChannelCaptureStatus = "kira:capture:status"
)

// Capture item kinds.
const (
	CaptureKindChangeRequest = "changeRequest"
	CaptureKindIssue         = "issue"
	CaptureKindText          = "text"
	CaptureKindEmpty         = "empty"
)

const (
	captureMaxBytes      = 64 << 10
	captureMaxTitleBytes = adeMaxNameBytes
)

// CaptureService is the clipboard capture popup: the global hotkey reads the clipboard once, the
// popup shows what it holds, Confirm adds it as a task.
type CaptureService struct {
	Deps     appcore.Deps
	HostView *HostViewService
	Tasks    *AdeTaskService
	Windows  hostViewWindowRows
	Binding  *shell.HotkeyBinding

	// Seams filled by appwire.BindShell.
	Clipboard   func() (string, bool)
	OpenPopup   func(key string)
	FocusWindow func(key string) bool
	CloseWindow func(key string) bool
	Shortcuts   shell.GlobalShortcuts

	mu      sync.Mutex
	key     string // the open popup, "" when none
	entries map[string]captureEntry
}

type captureEntry struct {
	item CaptureItem
	jira *adewire.Jira
}

// CaptureItem is what the popup shows.
type CaptureItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	Label     string `json:"label"`
	Title     string `json:"title"`
}

// CaptureWindowArgs names a popup.
type CaptureWindowArgs struct {
	WindowKey string `json:"windowKey"`
}

// CaptureConfirmArgs adds the popup's item as a task; ItemID guards against a replaced item.
type CaptureConfirmArgs struct {
	WindowKey string `json:"windowKey"`
	ItemID    string `json:"itemId"`
	Title     string `json:"title"`
	// Day is the plan day (YYYY-MM-DD) the task lands on; nil lands at the top of Later.
	Day *string `json:"day"`
}

// CaptureResult is Confirm's result. Notice names what the added task lacks ("" when complete);
// with a notice the popup stays open to show it.
type CaptureResult struct {
	TaskID string `json:"taskId"`
	Notice string `json:"notice"`
}

// WindowItem returns the item a popup shows, null for a non-capture or closed window.
func (s *CaptureService) WindowItem(_ context.Context, args CaptureWindowArgs) (*CaptureItem, error) {
	if err := validateAdeItemID(args.WindowKey, "windowKey"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[args.WindowKey]
	if !ok {
		return nil, nil
	}
	item := e.item
	return &item, nil
}

// Status returns the hotkey binding state.
func (s *CaptureService) Status(context.Context) (shell.HotkeyState, error) {
	if s.Binding == nil {
		return shell.HotkeyState{State: shell.HotkeyOff}, nil
	}
	return s.Binding.State(), nil
}

// Dismiss closes the popup; idempotent.
func (s *CaptureService) Dismiss(_ context.Context, args CaptureWindowArgs) error {
	if err := validateAdeItemID(args.WindowKey, "windowKey"); err != nil {
		return err
	}
	if !IsCaptureKey(args.WindowKey) {
		return ipcerr.Contract("windowKey is not a capture window")
	}
	s.closePopup(args.WindowKey)
	return nil
}

// Confirm adds the item as a task and closes the popup. An error means no task was added: the
// popup and its item stay, so a retry is safe. A notice means a task exists but lacks its details
// or branch: the popup stays to show it and the item is dropped, so a repeat Confirm is refused.
func (s *CaptureService) Confirm(ctx context.Context, args CaptureConfirmArgs) (CaptureResult, error) {
	if err := validateAdeItemID(args.WindowKey, "windowKey"); err != nil {
		return CaptureResult{}, err
	}
	s.mu.Lock()
	e, ok := s.entries[args.WindowKey]
	s.mu.Unlock()
	if !ok || e.item.ID != args.ItemID {
		return CaptureResult{}, ipcerr.Contract("capture item is stale")
	}
	if e.item.Kind == CaptureKindEmpty {
		return CaptureResult{}, ipcerr.Contract("nothing to capture")
	}
	if args.Day != nil {
		if err := validateAdeISODate(*args.Day); err != nil {
			return CaptureResult{}, err
		}
	}
	res, err := s.add(ctx, e, args.Title, args.Day)
	if err != nil {
		return CaptureResult{}, err
	}
	if res.Notice == "" {
		s.closePopup(args.WindowKey)
		return res, nil
	}
	s.mu.Lock()
	delete(s.entries, args.WindowKey)
	s.mu.Unlock()
	return res, nil
}

// add captures a task at the top of Later or of day, adds what the title cannot hold, then fetches a
// change request's head branch onto the task as a review branch. The steps are not atomic: a
// failed one leaves the task and returns a notice naming what it lacks.
func (s *CaptureService) add(ctx context.Context, e captureEntry, title string, day *string) (CaptureResult, error) {
	title = strings.TrimSpace(title)
	task, err := s.Tasks.CaptureTask(ctx, adewire.CaptureTaskArgs{Title: title, Day: day})
	if err != nil {
		return CaptureResult{}, err
	}
	res := CaptureResult{TaskID: task.ID}
	var patch adewire.TaskPatch
	if e.item.Text != title {
		patch.Notes = &e.item.Text
	}
	if s.singleLink(e.item.Text) {
		if _, ok := s.Tasks.Hosts.ParseLink(e.item.Text); ok {
			patch.HostURL = &e.item.Text
		}
	}
	patch.Jira = e.jira
	if patch.Notes != nil || patch.HostURL != nil || patch.Jira != nil {
		if _, err := s.Tasks.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: task.ID, Patch: patch}); err != nil {
			res.Notice = i18n.T("backend.space.capture.partial.body",
				map[string]string{"title": title, "error": strings.TrimSuffix(errText(err), ".")})
			return res, nil
		}
	}
	if e.item.Kind == CaptureKindChangeRequest {
		_, err := s.Tasks.Engine.AddReviewLink(adeUserCtx(ctx), adewire.HostLinkArgs{URL: e.item.Text, TaskID: task.ID})
		if err != nil {
			res.Notice = i18n.T("backend.space.capture.noBranch.body",
				map[string]string{"title": title, "error": strings.TrimSuffix(errText(adeTaskError(err)), ".")})
		}
	}
	return res, nil
}

func errText(err error) string {
	if e, ok := err.(*ipcerr.Error); ok {
		return e.Message
	}
	return err.Error()
}

func (s *CaptureService) singleLink(text string) bool {
	return len(text) <= maxHostViewURLBytes && !strings.ContainsFunc(text, unicode.IsSpace)
}

func (s *CaptureService) closePopup(key string) {
	if s.CloseWindow != nil {
		s.CloseWindow(key)
	}
	CaptureForget(s, key)
}

// CaptureForget drops key's item; a window close calls it. A function, not a method: not bound.
func CaptureForget(s *CaptureService, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	if s.key == key {
		s.key = ""
	}
}

// CaptureApply registers (or releases) the global hotkey from settings and pushes the new state.
// A function, not a method: not bound.
func CaptureApply(s *CaptureService, c model.CaptureSettings) shell.HotkeyState {
	if s.Binding == nil {
		return shell.HotkeyState{State: shell.HotkeyOff}
	}
	before := s.Binding.State()
	after := s.Binding.Apply(c.Enabled, c.Hotkey, func() { captureTrigger(s) })
	if after != before {
		s.Deps.Events.Emit(ChannelCaptureStatus, after)
	}
	return after
}

// CaptureReportShortcutError turns an asynchronous Wails registration failure into the failed
// state. A function, not a method: not bound.
func CaptureReportShortcutError(s *CaptureService, err error) {
	if s.Binding == nil {
		return
	}
	before := s.Binding.State()
	if after := s.Binding.ReportError(err); after != before {
		s.Deps.Events.Emit(ChannelCaptureStatus, after)
	}
}

// captureTrigger is the hotkey callback: read the clipboard once, classify, show the popup.
func captureTrigger(s *CaptureService) {
	if s.Clipboard == nil || s.OpenPopup == nil || s.FocusWindow == nil || s.Windows == nil {
		return
	}
	text, ok := s.Clipboard()
	entry := s.classify(context.Background(), text, ok)
	slog.Debug("capture", "scope", "capture", "kind", entry.item.Kind, "bytes", len(entry.item.Text))

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = map[string]captureEntry{}
	}
	if s.key != "" {
		if s.FocusWindow(s.key) {
			s.entries[s.key] = entry
			s.Deps.Events.EmitTo(s.key, ChannelCaptureItem, entry.item)
			return
		}
		delete(s.entries, s.key)
		s.key = ""
	}
	records, err := s.Windows.List()
	if err != nil {
		slog.Warn("capture: list windows", "scope", "capture", "err", err)
		return
	}
	order := 0
	for _, r := range records {
		if r.Order >= order {
			order = r.Order + 1
		}
	}
	key := CaptureWindowKeyPrefix + uuid.NewString()
	if err := s.Windows.Create(model.WindowRecord{Key: key, Order: order}); err != nil {
		slog.Warn("capture: create window row", "scope", "capture", "err", err)
		return
	}
	s.key = key
	s.entries[key] = entry
	s.OpenPopup(key)
}

// classify builds the item for clipboard text: a change request link, issue key or link, or other
// text; nothing shows the empty state.
func (s *CaptureService) classify(ctx context.Context, raw string, ok bool) captureEntry {
	text := strings.TrimSpace(raw)
	item := CaptureItem{ID: uuid.NewString(), Kind: CaptureKindEmpty}
	if !ok || text == "" {
		return captureEntry{item: item}
	}
	if len(text) > captureMaxBytes {
		text = strings.TrimSpace(cutUTF8(text, captureMaxBytes))
		item.Truncated = true
	}
	item.Text = text
	item.Title = cutUTF8(firstLine(text), captureMaxTitleBytes)
	item.Kind = CaptureKindText
	e := captureEntry{item: item}
	if !s.singleLink(text) {
		return e
	}
	if hosts := s.HostView.hosts(); hosts != nil {
		if p, _, repo, n, ok := hosts.ParseChangeRequestLink(ctx, text); ok {
			e.item.Kind = CaptureKindChangeRequest
			e.item.Label = p.Terms().ChangeRequestShort + " #" + strconv.Itoa(n) + " " + repo.Owner + "/" + repo.Name
			return e
		}
	}
	if p, ref, ok := s.HostView.issueProvider(text); ok {
		e.item.Kind = CaptureKindIssue
		e.item.Label = p.Terms().Label + " " + ref.Key
		e.jira = &adewire.Jira{Key: ref.Key, URL: ref.URL}
	}
	return e
}

func firstLine(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// cutUTF8 cuts s to at most n bytes on a rune boundary.
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
