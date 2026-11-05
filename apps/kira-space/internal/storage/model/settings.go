package model

import (
	"fmt"
	"log/slog"
	"regexp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/updates"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// Kira Space's own trimmed Settings model. Data/Cache/Api/DbMcp are dropped
// entirely: all four are DB-client-only concerns (page sizes, the query cache budget, HTTP client
// tuning, the embedded DB MCP server) Kira Space has no use for. Agent carries the leaves.
// Appearance/Advanced/Git are this app's own: the only app with a git module, so the two
// appearance leaves only the git module uses (InlineBlame/DateFormat), its own diagnostic-log leaf
// (GitLogLevel — Kira Studio's own is a same-shaped but separately named/keyed advanced.logLevel,
// not a shared struct any more) and the server-owned git leaves all live here rather than
// in the shared appsettings package.
type AdvancedSettings struct {
	// GitLogLevel is P72 §9.2's genuinely app-wide replacement for the per-repo kiraSpace.log.level
	// (renamed from kiraVersion.log.level, P100 Part 3); internal/logging.SetLevel is its actual
	// mechanism. Validated against the shared appsettings.ValidLogLevel enum.
	GitLogLevel string `json:"gitLogLevel"`
	// NotifyPrompts posts an OS notification when a popup waits for an answer (P246).
	NotifyPrompts bool `json:"notifyPrompts"`
}

// Appearance embeds appsettings.Appearance for the five leaves both apps share, plus the two only
// this app's git module uses: InlineBlame (the git-blame annotation toggle in the repo file
// viewer) and DateFormat (the relative-vs-absolute commit timestamp preference). The
// embedding flattens on the wire — encoding/json promotes an embedded struct's fields on both
// marshal and unmarshal.
type Appearance struct {
	appsettings.Appearance
	InlineBlame bool   `json:"inlineBlame"`
	DateFormat  string `json:"dateFormat"`
}

// GitSettings mirrors G7 D16's two server-owned git leaves: two windows disagreeing about either
// is a correctness/safety issue (a force-push confirmation that only one window enforces, an
// auto-fetch cadence that differs per viewer), so both live here rather than per-window settings.
type GitSettings struct {
	ProtectedBranches []string `json:"protectedBranches"`
	// FetchAutoIntervalMinutes is minutes between automatic background fetches; 0 disables it.
	FetchAutoIntervalMinutes int `json:"fetchAutoIntervalMinutes"`
	// GitPath: this leaf was always classified server-owned. Empty means
	// "auto-discover" — gitclient.Discovery's own existing contract, unvalidated beyond "is a
	// string" (a bad path is tolerated the same way Discovery's own probe already falls through
	// its classified-error states rather than pre-validating).
	GitPath string `json:"gitPath"`
	// GraphFontSize is P92 item 9's git-graph font size, in whole pixels; 0 means "follow
	// appearance.fontSize" — see settingsDomain.ts's own doc comment for the propagation path
	// (--kira-graph-font-size, read by git-ui's theme/git.css).
	GraphFontSize int `json:"graphFontSize"`
}

// AdeSettings is P129 Part 2 §7's own UiPrefs-as-settings-leaves: Part 3's queue-board layout and
// history-window preferences, persisted so they survive a relaunch. The Plan reads them.
type AdeSettings struct {
	HistoryDays     int      `json:"historyDays"`
	OffDays         []string `json:"offDays"`
	WorkWeekendDays []string `json:"workWeekendDays"`
	WorkdayHours    float64  `json:"workdayHours"`
	SpanDayShare    float64  `json:"spanDayShare"`
	// HeadlessSettingSources is the claude -p --setting-sources scope: "all" (user, project, local)
	// or "user" (ignore repo-committed settings).
	HeadlessSettingSources string `json:"headlessSettingSources"`
}

// AgentSettings embeds appsettings.Agent (the provider both apps share) plus this app's agent
// leaves. KeepAwakeWithAgents: on, this Mac stays awake while any agent session runs (a terminal
// agent tab or a running headless ade session), independent of the title bar's own keep-awake
// toggle. Off by default — an OS power assertion is opt-in.
type AgentSettings struct {
	appsettings.Agent
	KeepAwakeWithAgents bool `json:"keepAwakeWithAgents"`
	// Notify* are P238's desktop-notification leaves, all on by default: NotifyEnabled is the master
	// switch, the next three pick which events notify, NotifyIncludeMessage puts the reply text in
	// the body.
	NotifyEnabled        bool `json:"notifyEnabled"`
	NotifyOnFinished     bool `json:"notifyOnFinished"`
	NotifyOnNeedsInput   bool `json:"notifyOnNeedsInput"`
	NotifyOnRunEnded     bool `json:"notifyOnRunEnded"`
	NotifyIncludeMessage bool `json:"notifyIncludeMessage"`
	// UsageEnabled is P239's usage-limits switch: on, new sessions get the statusline wrapper and
	// the ADE status bar shows the 5-hour and weekly windows.
	UsageEnabled bool `json:"usageEnabled"`
}

// DefaultAgentSettings mirrors settingsDomain.ts's agentSettingsSchema defaults.
func DefaultAgentSettings() AgentSettings {
	return AgentSettings{
		Agent: appsettings.DefaultAgent(), NotifyEnabled: true, NotifyOnFinished: true, NotifyOnNeedsInput: true, NotifyOnRunEnded: true,
		NotifyIncludeMessage: true, UsageEnabled: true,
	}
}

// MobileSettings is P212's mobile agents web server: off until enabled, with its one plain-HTTP
// listener port. Changed through bridge.MobileAccessService, which restarts the server when it is
// running.
type MobileSettings struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
	// AgentInput is the global switch for phones replying to agents and attaching to their
	// terminals; a device also needs its own can_agent_input flag.
	AgentInput bool `json:"agentInput"`
}

// DefaultCaptureHotkey is the clipboard capture chord: three keys, rare enough to grab globally
// without stealing one another app uses.
const DefaultCaptureHotkey = "CmdOrCtrl+Shift+K"

// CaptureSettings is P430's clipboard capture hotkey.
type CaptureSettings struct {
	Enabled bool   `json:"enabled"`
	Hotkey  string `json:"hotkey"`
}

// DefaultCaptureSettings mirrors settingsDomain.ts's captureSettingsSchema defaults.
func DefaultCaptureSettings() CaptureSettings {
	return CaptureSettings{Enabled: true, Hotkey: DefaultCaptureHotkey}
}

// ValidCaptureHotkey reports whether s is a canonical global chord that no app menu accelerator uses.
func ValidCaptureHotkey(s string) bool {
	c, err := shell.ParseChord(s)
	return err == nil && c.Accelerator() == s && !c.ConflictsWithMenu()
}

// MemorySettings is P314's memory-feature leaf: Models maps a provider id to the model alias its
// memory calls use (gate, reconcile, import, distill). A provider with no entry uses its catalog
// default.
type MemorySettings struct {
	Models map[string]string `json:"models"`
}

// TrackingSettings is P512's tracked-update leaf: per item kind, whether each concept counts as a
// meaningful change. A concept missing from a map reads its catalog default.
type TrackingSettings struct {
	ChangeRequest map[string]bool `json:"changeRequest"`
	Issue         map[string]bool `json:"issue"`
	Branch        map[string]bool `json:"branch"`
}

// DefaultTrackingSettings builds every concept's default from updates.Catalog.
func DefaultTrackingSettings() TrackingSettings {
	return TrackingSettings{
		ChangeRequest: trackingDefaults(updates.KindChangeRequest),
		Issue:         trackingDefaults(updates.KindIssue),
		Branch:        trackingDefaults(updates.KindBranch),
	}
}

func trackingDefaults(kind string) map[string]bool {
	cs := updates.Concepts(kind)
	out := make(map[string]bool, len(cs))
	for _, c := range cs {
		out[c.ID] = c.Default
	}
	return out
}

// OverlayTracking returns the catalog defaults of kind with the known keys of stored on top;
// unknown stored keys drop.
func OverlayTracking(kind string, stored map[string]bool) map[string]bool {
	out := trackingDefaults(kind)
	for id, v := range stored {
		if _, ok := out[id]; ok {
			out[id] = v
		}
	}
	return out
}

type Settings struct {
	Appearance Appearance       `json:"appearance"`
	Advanced   AdvancedSettings `json:"advanced"`
	Git        GitSettings      `json:"git"`
	Ade        AdeSettings      `json:"ade"`
	Mobile     MobileSettings   `json:"mobile"`
	Capture    CaptureSettings  `json:"capture"`
	Memory     MemorySettings   `json:"memory"`
	Agent      AgentSettings    `json:"agent"`
	Tracking   TrackingSettings `json:"tracking"`
}

// DefaultMobileSettings mirrors settingsDomain.ts's mobileSettingsSchema defaults.
func DefaultMobileSettings() MobileSettings {
	return MobileSettings{Enabled: false, Port: 7790, AgentInput: false}
}

// DefaultGitSettings mirrors docs/HISTORY.md (v1.3 G7) D16's own default: the same three-pattern default
// upstream's own kiraVersion.protectedBranches carried, before that phase moved it server-side.
func DefaultGitSettings() GitSettings {
	return GitSettings{
		ProtectedBranches:        []string{"main", "master", "release/*"},
		FetchAutoIntervalMinutes: 0,
		GitPath:                  "",
		GraphFontSize:            0,
	}
}

// DefaultAdeSettings mirrors settingsDomain.ts's own adeSettingsSchema defaults (P129 Part 2 §7).
func DefaultAdeSettings() AdeSettings {
	return AdeSettings{
		HistoryDays:     14,
		OffDays:         []string{},
		WorkWeekendDays: []string{},
		WorkdayHours:    6,
		SpanDayShare:    0.5,

		HeadlessSettingSources: "all",
	}
}

// DefaultSettings mirrors the Appearance/Advanced/Git/Ade slice of
// packages/shared/domain/settings.ts's defaultSettings.
func DefaultSettings() Settings {
	return Settings{
		Appearance: Appearance{
			Appearance:  appsettings.DefaultAppearance(),
			InlineBlame: true,
			DateFormat:  "relative",
		},
		Advanced: AdvancedSettings{GitLogLevel: "info", NotifyPrompts: true},
		Git:      DefaultGitSettings(),
		Ade:      DefaultAdeSettings(),
		Mobile:   DefaultMobileSettings(),
		Capture:  DefaultCaptureSettings(),
		Memory:   MemorySettings{Models: map[string]string{}},
		Agent:    DefaultAgentSettings(),
		Tracking: DefaultTrackingSettings(),
	}
}

// AdvancedPatch mirrors AdvancedSettings' own `.partial()` shape.
type AdvancedPatch struct {
	GitLogLevel   *string `json:"gitLogLevel,omitempty"`
	NotifyPrompts *bool   `json:"notifyPrompts,omitempty"`
}

// AppearancePatch embeds appsettings.AppearancePatch for the five leaves both apps share, plus this
// app's own two (InlineBlame/DateFormat) — same embedding AdvancedPatch uses.
type AppearancePatch struct {
	appsettings.AppearancePatch
	InlineBlame *bool   `json:"inlineBlame,omitempty"`
	DateFormat  *string `json:"dateFormat,omitempty"`
}

// ValidDateFormat mirrors settingsDomain.ts's appearanceSettingsSchema.dateFormat enum (P120: only
// this app's git module has a dateFormat leaf).
func ValidDateFormat(v string) bool {
	return v == "relative" || v == "absolute"
}

// GitPatch mirrors GitSettings' own `.partial()` shape (G7 D16).
type GitPatch struct {
	ProtectedBranches        *[]string `json:"protectedBranches,omitempty"`
	FetchAutoIntervalMinutes *int      `json:"fetchAutoIntervalMinutes,omitempty"`
	GitPath                  *string   `json:"gitPath,omitempty"`
	GraphFontSize            *int      `json:"graphFontSize,omitempty"`
}

// AdePatch mirrors AdeSettings' own `.partial()` shape (§7).
type AdePatch struct {
	HistoryDays     *int      `json:"historyDays,omitempty"`
	OffDays         *[]string `json:"offDays,omitempty"`
	WorkWeekendDays *[]string `json:"workWeekendDays,omitempty"`
	WorkdayHours    *float64  `json:"workdayHours,omitempty"`
	SpanDayShare    *float64  `json:"spanDayShare,omitempty"`

	HeadlessSettingSources *string `json:"headlessSettingSources,omitempty"`
}

// AgentPatch mirrors AgentSettings' own `.partial()` shape.
type AgentPatch struct {
	appsettings.AgentPatch
	KeepAwakeWithAgents  *bool `json:"keepAwakeWithAgents,omitempty"`
	NotifyEnabled        *bool `json:"notifyEnabled,omitempty"`
	NotifyOnFinished     *bool `json:"notifyOnFinished,omitempty"`
	NotifyOnNeedsInput   *bool `json:"notifyOnNeedsInput,omitempty"`
	NotifyOnRunEnded     *bool `json:"notifyOnRunEnded,omitempty"`
	NotifyIncludeMessage *bool `json:"notifyIncludeMessage,omitempty"`
	UsageEnabled         *bool `json:"usageEnabled,omitempty"`
}

// MobilePatch mirrors MobileSettings' own `.partial()` shape.
type MobilePatch struct {
	Enabled    *bool `json:"enabled,omitempty"`
	Port       *int  `json:"port,omitempty"`
	AgentInput *bool `json:"agentInput,omitempty"`
}

// CapturePatch mirrors CaptureSettings' own `.partial()` shape.
type CapturePatch struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Hotkey  *string `json:"hotkey,omitempty"`
}

// MemoryPatch mirrors MemorySettings' own `.partial()` shape.
type MemoryPatch struct {
	// Models replaces the whole map.
	Models *map[string]string `json:"models,omitempty"`
}

// TrackingPatch mirrors TrackingSettings' own `.partial()` shape; each map replaces its whole leaf.
type TrackingPatch struct {
	ChangeRequest *map[string]bool `json:"changeRequest,omitempty"`
	Issue         *map[string]bool `json:"issue,omitempty"`
	Branch        *map[string]bool `json:"branch,omitempty"`
}

type SettingsPatch struct {
	Appearance *AppearancePatch `json:"appearance,omitempty"`
	Advanced   *AdvancedPatch   `json:"advanced,omitempty"`
	Git        *GitPatch        `json:"git,omitempty"`
	Ade        *AdePatch        `json:"ade,omitempty"`
	Mobile     *MobilePatch     `json:"mobile,omitempty"`
	Capture    *CapturePatch    `json:"capture,omitempty"`
	Memory     *MemoryPatch     `json:"memory,omitempty"`
	Agent      *AgentPatch      `json:"agent,omitempty"`
	Tracking   *TrackingPatch   `json:"tracking,omitempty"`
}

// validateAppearanceSection mirrors upsertAppearance's own leaf list (repos/settings.go).
func validateAppearanceSection(a *AppearancePatch) error {
	if a == nil {
		return nil
	}
	if err := appsettings.ValidateAppearance(&a.AppearancePatch); err != nil {
		return err
	}
	if a.DateFormat != nil && !ValidDateFormat(*a.DateFormat) {
		return fmt.Errorf("model: appearance.dateFormat: invalid value %q", *a.DateFormat)
	}
	return nil
}

func validateAdvancedSection(a *AdvancedPatch) error {
	if a == nil {
		return nil
	}
	if a.GitLogLevel != nil && !appsettings.ValidLogLevel(*a.GitLogLevel) {
		return fmt.Errorf("model: advanced.gitLogLevel: invalid value %q", *a.GitLogLevel)
	}
	return nil
}

// ValidFetchAutoIntervalMinutes and ValidGraphFontSize are exported (not just used by
// validateGitSection below): repos/settings.go's own readGit needs the identical bound to filter a
// stored leaf on read.
var (
	ValidFetchAutoIntervalMinutes = appsettings.InRange(0, 1440)
	// P92 item 9: settingsDomain.ts's own FONT_SIZE_RANGE, floored at 0 (the "follow the app"
	// sentinel) rather than FONT_SIZE_RANGE.min — the schema's own comment states why.
	ValidGraphFontSize = appsettings.InRange(0, 24)
)

// validateGitSection mirrors upsertGit's own leaf list (repos/settings.go).
func validateGitSection(g *GitPatch) error {
	if g == nil {
		return nil
	}
	if g.FetchAutoIntervalMinutes != nil && !ValidFetchAutoIntervalMinutes(*g.FetchAutoIntervalMinutes) {
		return fmt.Errorf("model: git.fetchAutoIntervalMinutes: out of range value %d", *g.FetchAutoIntervalMinutes)
	}
	if g.GraphFontSize != nil && !ValidGraphFontSize(*g.GraphFontSize) {
		return fmt.Errorf("model: git.graphFontSize: out of range value %d", *g.GraphFontSize)
	}
	return nil
}

// ValidAdeHeadlessSettingSources mirrors adeSettingsSchema.headlessSettingSources' enum.
func ValidAdeHeadlessSettingSources(v string) bool {
	return v == "user" || v == "all"
}

func floatInRange(lo, hi float64) func(float64) bool {
	return func(v float64) bool { return v >= lo && v <= hi }
}

var (
	ValidAdeHistoryDays  = appsettings.InRange(1, 365)
	ValidAdeWorkdayHours = floatInRange(1, 24)
	ValidAdeSpanDayShare = floatInRange(0.05, 1)
)

var adeISODateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ValidAdeDateList mirrors settingsDomain.ts's adeDateListSchema: at most 1000 entries, each an ISO
// (YYYY-MM-DD) date — offDays/workWeekendDays share this one shape.
func ValidAdeDateList(vs []string) bool {
	if len(vs) > 1000 {
		return false
	}
	for _, v := range vs {
		if !adeISODateRe.MatchString(v) {
			return false
		}
	}
	return true
}

// validateAdeSection mirrors upsertAde's own leaf list (repos/settings.go).
func validateAdeSection(a *AdePatch) error {
	if a == nil {
		return nil
	}
	if a.HistoryDays != nil && !ValidAdeHistoryDays(*a.HistoryDays) {
		return fmt.Errorf("model: ade.historyDays: out of range value %d", *a.HistoryDays)
	}
	if a.OffDays != nil && !ValidAdeDateList(*a.OffDays) {
		return fmt.Errorf("model: ade.offDays: invalid date list")
	}
	if a.WorkWeekendDays != nil && !ValidAdeDateList(*a.WorkWeekendDays) {
		return fmt.Errorf("model: ade.workWeekendDays: invalid date list")
	}
	if a.WorkdayHours != nil && !ValidAdeWorkdayHours(*a.WorkdayHours) {
		return fmt.Errorf("model: ade.workdayHours: out of range value %v", *a.WorkdayHours)
	}
	if a.SpanDayShare != nil && !ValidAdeSpanDayShare(*a.SpanDayShare) {
		return fmt.Errorf("model: ade.spanDayShare: out of range value %v", *a.SpanDayShare)
	}
	if a.HeadlessSettingSources != nil && !ValidAdeHeadlessSettingSources(*a.HeadlessSettingSources) {
		return fmt.Errorf("model: ade.headlessSettingSources: invalid value %q", *a.HeadlessSettingSources)
	}
	return nil
}

// ValidMobilePort mirrors settingsDomain.ts's mobileSettingsSchema port bounds: unprivileged ports.
var ValidMobilePort = appsettings.InRange(1024, 65535)

func validateMobileSection(m *MobilePatch) error {
	if m == nil {
		return nil
	}
	if m.Port != nil && !ValidMobilePort(*m.Port) {
		return fmt.Errorf("model: mobile.port: out of range value %d", *m.Port)
	}
	return nil
}

func validateCaptureSection(c *CapturePatch) error {
	if c == nil || c.Hotkey == nil {
		return nil
	}
	if !ValidCaptureHotkey(*c.Hotkey) {
		return fmt.Errorf("model: capture.hotkey: invalid or reserved chord %q", *c.Hotkey)
	}
	return nil
}

// validMemoryModel reports whether alias is in provider's catalog. A provider that is not
// registered passes: its entry is kept and ignored until the provider returns.
func validMemoryModel(reg *harness.Registry, provider, alias string) bool {
	p, ok := reg.Get(provider)
	return !ok || p.Models().Valid(alias)
}

// CleanMemoryModels drops the entries whose provider id is malformed or whose alias is not in the
// catalog of the provider registered in reg.
func CleanMemoryModels(reg *harness.Registry, models map[string]string) map[string]string {
	out := make(map[string]string, len(models))
	for id, alias := range models {
		if !appsettings.ValidProviderID(id) || !validMemoryModel(reg, id, alias) {
			slog.Warn("settings: ignoring memory model", "scope", "settings", "provider", id, "model", alias)
			continue
		}
		out[id] = alias
	}
	return out
}

func validateMemorySection(m *MemoryPatch) error {
	if m == nil || m.Models == nil {
		return nil
	}
	for id := range *m.Models {
		if !appsettings.ValidProviderID(id) {
			return fmt.Errorf("model: memory.models: invalid provider id %q", id)
		}
	}
	return nil
}

func validateTrackingSection(t *TrackingPatch) error {
	if t == nil {
		return nil
	}
	for _, leaf := range []struct {
		kind string
		m    *map[string]bool
	}{{updates.KindChangeRequest, t.ChangeRequest}, {updates.KindIssue, t.Issue}, {updates.KindBranch, t.Branch}} {
		if leaf.m == nil {
			continue
		}
		for id := range *leaf.m {
			if !updates.Known(leaf.kind, id) {
				return fmt.Errorf("model: tracking.%s: unknown concept %q", leaf.kind, id)
			}
		}
	}
	return nil
}

// CheckModels checks every patched alias against the catalog of its provider registered in reg.
func (m *MemoryPatch) CheckModels(reg *harness.Registry) error {
	if m == nil || m.Models == nil {
		return nil
	}
	for id, alias := range *m.Models {
		if !validMemoryModel(reg, id, alias) {
			return fmt.Errorf("model: memory.models.%s: invalid value %q", id, alias)
		}
	}
	return nil
}

// Validate checks every leaf the caller actually patched against settings.ts's bounds, naming the
// offending leaf in the error — fontFamily and fontSize have no bounds in the TS schema either, so
// they are accepted as-is.
func (p SettingsPatch) Validate() error {
	if err := validateAppearanceSection(p.Appearance); err != nil {
		return err
	}
	if err := validateAdvancedSection(p.Advanced); err != nil {
		return err
	}
	if err := validateGitSection(p.Git); err != nil {
		return err
	}
	if err := validateAdeSection(p.Ade); err != nil {
		return err
	}
	if err := validateMobileSection(p.Mobile); err != nil {
		return err
	}
	if err := validateCaptureSection(p.Capture); err != nil {
		return err
	}
	if err := validateMemorySection(p.Memory); err != nil {
		return err
	}
	if err := validateTrackingSection(p.Tracking); err != nil {
		return err
	}
	if p.Agent == nil {
		return nil
	}
	return appsettings.ValidateAgent(&p.Agent.AgentPatch)
}
