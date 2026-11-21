package bridge

import (
	"sort"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// TabsService persists a window's tabs in the one `tabs` table and moves them between windows.
type TabsService struct {
	Deps appcore.Deps
	// Windows is set by BindShell; the zero value means no shell (a build that cannot open or
	// measure windows).
	Windows TabWindows
	// Terminals re-owns a Runs terminal's session; zero value refuses terminal moves.
	Terminals TabTerminals

	inbox tabInbox
}

// TabWindows is the window manager as tab moves use it.
type TabWindows struct {
	Geometry func() []shell.WindowGeom
	// Open opens a window from an already-persisted record; nil Bounds cascades.
	Open  func(rec shell.WindowRecord)
	Focus func(key string) bool
	Close func(key string) bool
	// Positions reports whether window positions are global (false on Wayland).
	Positions func() bool
}

// TabTerminals is the terminal registry as tab moves use it.
type TabTerminals struct {
	Reown func(id, from, to string) bool
}

// tabInbox holds tabs moved to a window until it pulls them.
type tabInbox struct {
	mu      sync.Mutex
	entries map[string][]TabInbound
}

func (b *tabInbox) push(key string, in TabInbound) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries == nil {
		b.entries = map[string][]TabInbound{}
	}
	b.entries[key] = append(b.entries[key], in)
}

func (b *tabInbox) pop(key string) []TabInbound {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.entries[key]
	delete(b.entries, key)
	if out == nil {
		out = []TabInbound{}
	}
	return out
}

// ForgetTabWindow drops key's inbox: the window closed before pulling. A function, not a method,
// so it stays unbound.
func ForgetTabWindow(s *TabsService, key string) {
	s.inbox.mu.Lock()
	delete(s.inbox.entries, key)
	s.inbox.mu.Unlock()
}

type TabsListArgs struct {
	WindowKey string `json:"windowKey"`
}

// List returns the window's tabs ordered by their order field.
func (s *TabsService) List(args TabsListArgs) ([]model.TabRecord, error) {
	if args.WindowKey == "" {
		return nil, ipcerr.Required("windowKey")
	}
	return appstorage.ListWindowTabs(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, args.WindowKey)
}

type TabsSaveArgs struct {
	WindowKey string            `json:"windowKey"`
	Tabs      []model.TabRecord `json:"tabs"`
}

// Save replaces the window's tab set.
func (s *TabsService) Save(args TabsSaveArgs) error {
	if args.WindowKey == "" {
		return ipcerr.Required("windowKey")
	}
	return appstorage.SaveWindowTabs(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, args.WindowKey, args.Tabs)
}

// TabDrop is a drag-out pointerup in the source window's client coordinates (CSS px).
type TabDrop struct {
	ClientX     float64 `json:"clientX"`
	ClientY     float64 `json:"clientY"`
	GrabX       float64 `json:"grabX"`
	GrabY       float64 `json:"grabY"`
	InnerWidth  float64 `json:"innerWidth"`
	InnerHeight float64 `json:"innerHeight"`
}

// TabTerminal travels with a moved Runs terminal tab to the target's inbox untouched.
type TabTerminal struct {
	Snapshot string `json:"snapshot"` // SerializeAddon output, at most 4 MiB
	Cols     int    `json:"cols"`
	Rows     int    `json:"rows"`
	Status   string `json:"status"` // running | exited | failed
	ExitCode *int   `json:"exitCode,omitempty"`
	Error    string `json:"error,omitempty"`
	Shell    string `json:"shell"`
}

// TabsMoveArgs moves one tab out of window WindowKey. Exactly one of Target, NewWindow, Drop.
type TabsMoveArgs struct {
	WindowKey string          `json:"windowKey"`
	Tab       model.TabRecord `json:"tab"`
	Incognito bool            `json:"incognito"`
	Target    string          `json:"target"`
	NewWindow bool            `json:"newWindow"`
	Drop      *TabDrop        `json:"drop"`
	Terminal  *TabTerminal    `json:"terminal,omitempty"`
}

// TabsMoveResult says where the tab now lives; the source key means nothing moved.
type TabsMoveResult struct {
	WindowKey string `json:"windowKey"`
	Created   bool   `json:"created"`
}

// TabInbound is a tab waiting in a window's inbox.
type TabInbound struct {
	Tab       model.TabRecord `json:"tab"`
	Incognito bool            `json:"incognito"`
	Terminal  *TabTerminal    `json:"terminal,omitempty"`
}

// TabWindowTarget is another live window a tab can move to; Number is 1-based by stored order
// among live real windows, source included.
type TabWindowTarget struct {
	Key    string `json:"key"`
	Number int    `json:"number"`
}

// TabCaps says what the tab strip may offer.
type TabCaps struct {
	DragOut bool `json:"dragOut"`
}

func realWindows(geo []shell.WindowGeom) []shell.WindowGeom {
	out := make([]shell.WindowGeom, 0, len(geo))
	for _, g := range geo {
		if !g.Ephemeral {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func hasWindow(geo []shell.WindowGeom, key string) bool {
	for _, g := range geo {
		if g.Key == key {
			return true
		}
	}
	return false
}

func (s *TabsService) validateMove(args TabsMoveArgs) error {
	if args.WindowKey == "" {
		return ipcerr.Required("windowKey")
	}
	dests := 0
	if args.Target != "" {
		dests++
	}
	if args.NewWindow {
		dests++
	}
	if args.Drop != nil {
		dests++
	}
	if dests != 1 {
		return ipcerr.Contract("exactly one of target, newWindow, drop is required")
	}
	if args.Target == args.WindowKey {
		return ipcerr.Contract("target window is the source window")
	}
	if err := args.Tab.Validate(); err != nil {
		return ipcerr.Contract("%s", err.Error())
	}
	if !model.IsMovableTab(args.Tab) {
		return ipcerr.Contract("tab kind %q cannot move between windows", args.Tab.Kind)
	}
	if args.Terminal != nil && args.Tab.Kind != "terminal" {
		return ipcerr.Contract("terminal payload on a %s tab", args.Tab.Kind)
	}
	if args.Tab.Kind == "terminal" && s.Terminals.Reown == nil {
		return ipcerr.Contract("this build cannot move terminals")
	}
	if s.Windows.Geometry == nil || s.Windows.Open == nil {
		return ipcerr.Contract("this build cannot open a window")
	}
	return nil
}

// Move re-keys a tab to another window: a named one, a new one, or the one under a drag-out drop.
// One storage transaction; the target pulls the tab through Inbound.
func (s *TabsService) Move(args TabsMoveArgs) (TabsMoveResult, error) {
	if err := s.validateMove(args); err != nil {
		return TabsMoveResult{}, err
	}
	geo := s.Windows.Geometry()
	src := args.WindowKey
	if !hasWindow(geo, src) || !windowReal(geo, src) {
		return TabsMoveResult{}, ipcerr.Contract("unknown window: %s", src)
	}
	dst, newWindow, bounds, err := s.destination(args, geo)
	if err != nil {
		return TabsMoveResult{}, err
	}
	if dst == src {
		return TabsMoveResult{WindowKey: src}, nil
	}

	var created *shell.WindowRecord
	if newWindow {
		rec, err := s.createWindow(bounds)
		if err != nil {
			return TabsMoveResult{}, err
		}
		created, dst = rec, rec.Key
	}
	persist := !args.Incognito && !model.IsSessionTabKind(args.Tab.Kind)
	if err := appstorage.MoveWindowTab(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, src, dst, args.Tab, persist); err != nil {
		if created != nil {
			_ = s.Deps.Repos.Windows.Delete(created.Key)
		}
		return TabsMoveResult{}, err
	}
	if args.Tab.Kind == "terminal" {
		// false: the session already ended; the snapshot and a held exit carry it.
		s.Terminals.Reown(args.Tab.ID, src, dst)
	}
	s.inbox.push(dst, TabInbound{Tab: args.Tab, Incognito: args.Incognito, Terminal: args.Terminal})
	if created != nil {
		s.Windows.Open(*created)
	} else {
		s.Deps.Events.EmitTo(dst, ChannelTabsInbound, nil)
		if s.Windows.Focus != nil {
			s.Windows.Focus(dst)
		}
	}
	return TabsMoveResult{WindowKey: dst, Created: created != nil}, nil
}

func windowReal(geo []shell.WindowGeom, key string) bool {
	for _, g := range geo {
		if g.Key == key {
			return !g.Ephemeral
		}
	}
	return false
}

// destination resolves where the tab goes: an existing window key, or newWindow with the bounds
// to open it at (nil = cascade).
func (s *TabsService) destination(args TabsMoveArgs, geo []shell.WindowGeom) (key string, newWindow bool, bounds *shell.WindowBounds, err error) {
	src := args.WindowKey
	switch {
	case args.Target != "":
		if !windowReal(geo, args.Target) {
			return "", false, nil, ipcerr.Contract("unknown window: %s", args.Target)
		}
		return args.Target, false, nil, nil
	case args.NewWindow:
		return "", true, nil, nil
	}
	if s.Windows.Positions == nil || !s.Windows.Positions() {
		return "", false, nil, ipcerr.Contract("window positions are unavailable here: drag-out is off")
	}
	var srcBounds shell.WindowBounds
	for _, g := range geo {
		if g.Key == src {
			srcBounds = g.Bounds
		}
	}
	d := args.Drop
	in := shell.DropInput{ClientX: d.ClientX, ClientY: d.ClientY, GrabX: d.GrabX, GrabY: d.GrabY, InnerWidth: d.InnerWidth, InnerHeight: d.InnerHeight}
	x, y := shell.DropPoint(srcBounds, in)
	key, inSource, ok := shell.DropTarget(geo, src, x, y)
	switch {
	case inSource:
		return src, false, nil, nil
	case ok:
		return key, false, nil, nil
	}
	torn := shell.TornBounds(srcBounds, in)
	return "", true, &torn, nil
}

func (s *TabsService) createWindow(bounds *shell.WindowBounds) (*shell.WindowRecord, error) {
	records, err := s.Deps.Repos.Windows.List()
	if err != nil {
		return nil, ipcerr.Internal(err.Error())
	}
	order := 0
	for _, r := range records {
		if r.Order >= order {
			order = r.Order + 1
		}
	}
	rec := shell.WindowRecord{Key: ids.NewID(), Order: order, Bounds: bounds}
	if err := s.Deps.Repos.Windows.Create(rec); err != nil {
		return nil, ipcerr.Internal(err.Error())
	}
	return &rec, nil
}

// Inbound pops this window's inbox: tabs moved here since its last pull.
func (s *TabsService) Inbound(args TabsListArgs) ([]TabInbound, error) {
	if args.WindowKey == "" {
		return nil, ipcerr.Required("windowKey")
	}
	return s.inbox.pop(args.WindowKey), nil
}

// Targets lists the other live real windows, numbered from 1 by stored order, this one included
// in the numbering.
func (s *TabsService) Targets(args TabsListArgs) ([]TabWindowTarget, error) {
	if args.WindowKey == "" {
		return nil, ipcerr.Required("windowKey")
	}
	out := []TabWindowTarget{}
	if s.Windows.Geometry == nil {
		return out, nil
	}
	for i, g := range realWindows(s.Windows.Geometry()) {
		if g.Key != args.WindowKey {
			out = append(out, TabWindowTarget{Key: g.Key, Number: i + 1})
		}
	}
	return out, nil
}

// Capabilities says whether tabs may be dragged out of the window.
func (s *TabsService) Capabilities() TabCaps {
	return TabCaps{DragOut: s.Windows.Geometry != nil && s.Windows.Open != nil && s.Windows.Positions != nil && s.Windows.Positions()}
}

// CloseEmptyWindow closes this window when it holds no tab and another real window is live. true
// when it asked the window to close.
func (s *TabsService) CloseEmptyWindow(args TabsListArgs) (bool, error) {
	if args.WindowKey == "" {
		return false, ipcerr.Required("windowKey")
	}
	if s.Windows.Geometry == nil || s.Windows.Close == nil {
		return false, nil
	}
	real := realWindows(s.Windows.Geometry())
	if !windowReal(real, args.WindowKey) || len(real) < 2 {
		return false, nil
	}
	tabs, err := appstorage.ListWindowTabs(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, args.WindowKey)
	if err != nil {
		return false, err
	}
	if len(tabs) > 0 {
		return false, nil
	}
	return s.Windows.Close(args.WindowKey), nil
}
