// Package repos holds every table repo of the merged database.
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/updates"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// SettingsRepo reads and writes the `settings` table, one JSON-valued row per leaf
// (`${section}.${key}`), never a blob per section — same shape as Kira Studio's own SettingsRepo,
// trimmed to the three sections model.Settings actually has here (appearance/advanced/git).
var settingsLeaves = sqlb.LeafTable{Table: kiraschema.Settings.Table, Key: kiraschema.Settings.Key, Value: kiraschema.Settings.Value}

// settingsSelectAllSQL is the "every stored leaf" query repos.New prepares once.
var settingsSelectAllSQL = func() string {
	q, _ := sqlb.SQLite.Select(settingsLeaves.Key, settingsLeaves.Value).From(settingsLeaves.Table).Build()
	return q
}()

type SettingsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect

	// selectAll is prepared once by repos.New. nil when a SettingsRepo is constructed directly
	// (e.g. in tests), which falls back to an ad-hoc query with identical SQL.
	selectAll *sql.Stmt

	// Agents holds the providers whose model catalogs memory.models is checked against; nil means
	// the built-in providers.
	Agents *harness.Registry
}

func (r *SettingsRepo) agents() *harness.Registry {
	if r.Agents == nil {
		return providers.New()
	}
	return r.Agents
}

func (r *SettingsRepo) GetAll() (model.Settings, error) {
	stored, err := sqlb.QueryLeaves(r.DB, r.Dialect, r.selectAll, settingsLeaves)
	if err != nil {
		return model.Settings{}, err
	}

	result := model.DefaultSettings()
	result.Appearance = readAppearance(stored)
	result.Git = readGit(stored)
	result.Ade = readAde(stored)
	result.Mobile = readMobile(stored)
	result.Capture = readCapture(stored)
	readAgent(stored, &result.Agent)
	readTracking(stored, &result.Tracking)
	appsettings.Leaf(stored, "memory.models", &result.Memory.Models)
	result.Memory.Models = model.CleanMemoryModels(r.agents(), result.Memory.Models)
	appsettings.Leaf(stored, "experimental.memory", &result.Experimental.Memory)
	appsettings.LeafValid(stored, "advanced.gitLogLevel", &result.Advanced.GitLogLevel, appsettings.ValidLogLevel)
	appsettings.Leaf(stored, "advanced.notifyPrompts", &result.Advanced.NotifyPrompts)
	appsettings.LeafValid(stored, "advanced.opLogRetentionDays", &result.Advanced.OpLogRetentionDays, appsettings.InRange(1, 365))
	appsettings.LeafValid(stored, "advanced.expensiveQueryRows", &result.Advanced.ExpensiveQueryRows, appsettings.InRange(1_000, 1_000_000_000))
	appsettings.LeafValid(stored, "data.defaultPageSize", &result.Data.DefaultPageSize, model.ValidPageSize)
	appsettings.LeafValid(stored, "cache.l2BudgetMb", &result.Cache.L2BudgetMb, appsettings.InRange(8, 1024))
	appsettings.LeafValid(stored, "api.httpVersion", &result.Api.HTTPVersion, model.ValidHTTPVersion)
	appsettings.LeafValid(stored, "api.requestTimeoutMs", &result.Api.RequestTimeoutMs, appsettings.InRange(0, 3_600_000))
	appsettings.LeafValid(stored, "api.maxResponseMb", &result.Api.MaxResponseMb, appsettings.InRange(0, 2048))
	appsettings.Leaf(stored, "api.sslVerify", &result.Api.SSLVerify)
	appsettings.Leaf(stored, "api.followRedirects", &result.Api.FollowRedirects)
	appsettings.LeafValid(stored, "api.maxRedirects", &result.Api.MaxRedirects, appsettings.InRange(0, 100))
	appsettings.Leaf(stored, "api.disableCookieJar", &result.Api.DisableCookieJar)
	appsettings.Leaf(stored, "dbMcp.serverEnabled", &result.DbMcp.ServerEnabled)
	appsettings.Leaf(stored, "dbMcp.previewResponses", &result.DbMcp.PreviewResponses)
	appsettings.Leaf(stored, "dbMcp.focusOnApproval", &result.DbMcp.FocusOnApproval)
	appsettings.Leaf(stored, "dbMcp.apiRequests", &result.DbMcp.APIRequests)
	appsettings.LeafValid(stored, "dbMcp.apiApproval", &result.DbMcp.APIApproval, model.ValidAPIApproval)
	return result, nil
}

// readAppearance reads the shared appearance.* leaves plus this app's own two (inlineBlame/
// dateFormat) on top of model.DefaultSettings().Appearance, mirroring the former
// appsettings.ReadAppearance's own inlineBlame/dateFormat block (P120: only this app's git module
// uses either leaf).
func readAppearance(stored map[string]json.RawMessage) model.Appearance {
	result := model.DefaultSettings().Appearance
	result.Appearance = appsettings.ReadAppearance(stored)
	appsettings.Leaf(stored, "appearance.inlineBlame", &result.InlineBlame)
	appsettings.LeafValid(stored, "appearance.dateFormat", &result.DateFormat, model.ValidDateFormat)
	return result
}

// readGit reads every git.* leaf from stored on top of model.DefaultGitSettings(), mirroring the
// former appsettings.ReadGit verbatim (P120: Git is this app's own model, the only app with a git
// module).
func readGit(stored map[string]json.RawMessage) model.GitSettings {
	result := model.DefaultGitSettings()
	appsettings.Leaf(stored, "git.protectedBranches", &result.ProtectedBranches)
	appsettings.LeafValid(stored, "git.fetchAutoIntervalMinutes", &result.FetchAutoIntervalMinutes, model.ValidFetchAutoIntervalMinutes)
	appsettings.Leaf(stored, "git.path", &result.GitPath)
	appsettings.LeafValid(stored, "git.graphFontSize", &result.GraphFontSize, model.ValidGraphFontSize)
	return result
}

// readAde reads every ade.* leaf from stored on top of model.DefaultAdeSettings() (P129 Part 2 §7).
func readAde(stored map[string]json.RawMessage) model.AdeSettings {
	result := model.DefaultAdeSettings()
	appsettings.LeafValid(stored, "ade.historyDays", &result.HistoryDays, model.ValidAdeHistoryDays)
	appsettings.LeafValid(stored, "ade.offDays", &result.OffDays, model.ValidAdeDateList)
	appsettings.LeafValid(stored, "ade.workWeekendDays", &result.WorkWeekendDays, model.ValidAdeDateList)
	appsettings.LeafValid(stored, "ade.workdayHours", &result.WorkdayHours, model.ValidAdeWorkdayHours)
	appsettings.LeafValid(stored, "ade.spanDayShare", &result.SpanDayShare, model.ValidAdeSpanDayShare)
	appsettings.LeafValid(stored, "ade.headlessSettingSources", &result.HeadlessSettingSources, model.ValidAdeHeadlessSettingSources)
	return result
}

// readMobile reads every mobile.* leaf on top of model.DefaultMobileSettings().
func readMobile(stored map[string]json.RawMessage) model.MobileSettings {
	result := model.DefaultMobileSettings()
	appsettings.Leaf(stored, "mobile.enabled", &result.Enabled)
	appsettings.LeafValid(stored, "mobile.port", &result.Port, model.ValidMobilePort)
	appsettings.Leaf(stored, "mobile.agentInput", &result.AgentInput)
	return result
}

// readTracking overlays the stored known concepts of each kind on the catalog defaults.
func readTracking(stored map[string]json.RawMessage, t *model.TrackingSettings) {
	for _, leaf := range []struct {
		key  string
		kind string
		dst  *map[string]bool
	}{
		{"tracking.changeRequest", updates.KindChangeRequest, &t.ChangeRequest},
		{"tracking.issue", updates.KindIssue, &t.Issue},
		{"tracking.branch", updates.KindBranch, &t.Branch},
	} {
		var saved map[string]bool
		appsettings.Leaf(stored, leaf.key, &saved)
		*leaf.dst = model.OverlayTracking(leaf.kind, saved)
	}
}

func upsertTracking(tx *sql.Tx, t *model.TrackingPatch) error {
	if t == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "tracking.changeRequest", t.ChangeRequest); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "tracking.issue", t.Issue); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "tracking.branch", t.Branch)
}

func upsertMemory(tx *sql.Tx, m *model.MemoryPatch) error {
	if m == nil {
		return nil
	}
	return appsettings.UpsertOptional(tx, "memory.models", m.Models)
}

func upsertExperimental(tx *sql.Tx, e *model.ExperimentalPatch) error {
	if e == nil {
		return nil
	}
	return appsettings.UpsertOptional(tx, "experimental.memory", e.Memory)
}

// readCapture reads the capture.* leaves on top of model.DefaultCaptureSettings().
func readCapture(stored map[string]json.RawMessage) model.CaptureSettings {
	result := model.DefaultCaptureSettings()
	appsettings.Leaf(stored, "capture.enabled", &result.Enabled)
	appsettings.LeafValid(stored, "capture.hotkey", &result.Hotkey, model.ValidCaptureHotkey)
	return result
}

func upsertCapture(tx *sql.Tx, c *model.CapturePatch) error {
	if c == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "capture.enabled", c.Enabled); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "capture.hotkey", c.Hotkey)
}

func upsertMobile(tx *sql.Tx, m *model.MobilePatch) error {
	if m == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "mobile.enabled", m.Enabled); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "mobile.port", m.Port); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "mobile.agentInput", m.AgentInput)
}

// upsertAde mirrors upsertGit's own shape.
func upsertAde(tx *sql.Tx, a *model.AdePatch) error {
	if a == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "ade.historyDays", a.HistoryDays); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "ade.offDays", a.OffDays); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "ade.workWeekendDays", a.WorkWeekendDays); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "ade.workdayHours", a.WorkdayHours); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "ade.spanDayShare", a.SpanDayShare); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "ade.headlessSettingSources", a.HeadlessSettingSources)
}

// upsertGit mirrors the former appsettings.UpsertGit verbatim — "git.path" (not "git.gitPath") is
// the stored key for GitPath, matching this app's pre-existing row shape.
func upsertGit(tx *sql.Tx, g *model.GitPatch) error {
	if g == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "git.protectedBranches", g.ProtectedBranches); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "git.fetchAutoIntervalMinutes", g.FetchAutoIntervalMinutes); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "git.path", g.GitPath); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "git.graphFontSize", g.GraphFontSize)
}

// upsertAppearance upserts the shared appearance.* leaves plus this app's own two (inlineBlame/
// dateFormat) — mirrors the former appsettings.UpsertAppearance's own inlineBlame/dateFormat block.
func upsertAppearance(tx *sql.Tx, a *model.AppearancePatch) error {
	if a == nil {
		return nil
	}
	if err := appsettings.UpsertAppearance(tx, &a.AppearancePatch); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "appearance.inlineBlame", a.InlineBlame); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "appearance.dateFormat", a.DateFormat)
}

func upsertAdvancedSection(tx *sql.Tx, a *model.AdvancedPatch) error {
	if a == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "advanced.gitLogLevel", a.GitLogLevel); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "advanced.opLogRetentionDays", a.OpLogRetentionDays); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "advanced.expensiveQueryRows", a.ExpensiveQueryRows); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "advanced.notifyPrompts", a.NotifyPrompts)
}

func upsertDataSection(tx *sql.Tx, d *model.DataPatch) error {
	if d == nil {
		return nil
	}
	return appsettings.UpsertOptional(tx, "data.defaultPageSize", d.DefaultPageSize)
}

func upsertCacheSection(tx *sql.Tx, c *model.CachePatch) error {
	if c == nil {
		return nil
	}
	return appsettings.UpsertOptional(tx, "cache.l2BudgetMb", c.L2BudgetMb)
}

func upsertApiSection(tx *sql.Tx, a *model.ApiPatch) error {
	if a == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "api.httpVersion", a.HTTPVersion); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "api.requestTimeoutMs", a.RequestTimeoutMs); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "api.maxResponseMb", a.MaxResponseMb); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "api.sslVerify", a.SSLVerify); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "api.followRedirects", a.FollowRedirects); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "api.maxRedirects", a.MaxRedirects); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "api.disableCookieJar", a.DisableCookieJar)
}

func upsertDbMcpSection(tx *sql.Tx, dm *model.DbMcpPatch) error {
	if dm == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "dbMcp.serverEnabled", dm.ServerEnabled); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "dbMcp.previewResponses", dm.PreviewResponses); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "dbMcp.focusOnApproval", dm.FocusOnApproval); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "dbMcp.apiRequests", dm.APIRequests); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "dbMcp.apiApproval", dm.APIApproval)
}

// Set validates the patch, writes only the leaves the caller actually patched in one transaction,
// and returns GetAll() afterwards.
func (r *SettingsRepo) Set(patch model.SettingsPatch) (model.Settings, error) {
	if err := patch.Validate(); err != nil {
		return model.Settings{}, fmt.Errorf("repos: %w", err)
	}
	if err := patch.Memory.CheckModels(r.agents()); err != nil {
		return model.Settings{}, fmt.Errorf("repos: %w", err)
	}

	err := sqlb.UpdateLeaves(r.DB, r.Dialect, r.selectAll, settingsLeaves, func(tx *sql.Tx, _ map[string]json.RawMessage) error {
		if err := upsertAppearance(tx, patch.Appearance); err != nil {
			return err
		}
		if err := upsertAdvancedSection(tx, patch.Advanced); err != nil {
			return err
		}
		if err := upsertGit(tx, patch.Git); err != nil {
			return err
		}
		if err := upsertAde(tx, patch.Ade); err != nil {
			return err
		}
		if err := upsertMobile(tx, patch.Mobile); err != nil {
			return err
		}
		if err := upsertCapture(tx, patch.Capture); err != nil {
			return err
		}
		if err := upsertMemory(tx, patch.Memory); err != nil {
			return err
		}
		if err := upsertExperimental(tx, patch.Experimental); err != nil {
			return err
		}
		if err := upsertTracking(tx, patch.Tracking); err != nil {
			return err
		}
		if err := upsertDataSection(tx, patch.Data); err != nil {
			return err
		}
		if err := upsertCacheSection(tx, patch.Cache); err != nil {
			return err
		}
		if err := upsertApiSection(tx, patch.Api); err != nil {
			return err
		}
		if err := upsertDbMcpSection(tx, patch.DbMcp); err != nil {
			return err
		}
		return upsertAgent(tx, patch.Agent)
	})
	if err != nil {
		return model.Settings{}, err
	}
	return r.GetAll()
}

// agentLeaf pairs one agent.<key> bool leaf with its read target and patch field.
type agentLeaf struct {
	key string
	dst *bool
	set *bool
}

func agentLeaves(c *model.AgentSettings, p model.AgentPatch) []agentLeaf {
	return []agentLeaf{
		{"keepAwakeWithAgents", &c.KeepAwakeWithAgents, p.KeepAwakeWithAgents},
		{"notifyEnabled", &c.NotifyEnabled, p.NotifyEnabled},
		{"notifyOnFinished", &c.NotifyOnFinished, p.NotifyOnFinished},
		{"notifyOnNeedsInput", &c.NotifyOnNeedsInput, p.NotifyOnNeedsInput},
		{"notifyOnRunEnded", &c.NotifyOnRunEnded, p.NotifyOnRunEnded},
		{"notifyIncludeMessage", &c.NotifyIncludeMessage, p.NotifyIncludeMessage},
		{"usageEnabled", &c.UsageEnabled, p.UsageEnabled},
	}
}

// readAgent reads the shared agent.provider leaf and this app's bool leaves on top of dst.
func readAgent(stored map[string]json.RawMessage, dst *model.AgentSettings) {
	dst.Agent = appsettings.ReadAgent(stored)
	for _, l := range agentLeaves(dst, model.AgentPatch{}) {
		appsettings.Leaf(stored, "agent."+l.key, l.dst)
	}
}

func upsertAgent(tx *sql.Tx, p *model.AgentPatch) error {
	if p == nil {
		return nil
	}
	if err := appsettings.UpsertAgent(tx, &p.AgentPatch); err != nil {
		return err
	}
	var unused model.AgentSettings
	for _, l := range agentLeaves(&unused, *p) {
		if err := appsettings.UpsertOptional(tx, "agent."+l.key, l.set); err != nil {
			return err
		}
	}
	return nil
}
