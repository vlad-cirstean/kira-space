// Package repos is the Go analogue of src/main/storage/repos/*.ts.
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/appstorage"
)

// SettingsRepo reads and writes the `settings` table, one JSON-valued row per leaf
// (`${section}.${key}`), never a blob per section — settings.ts's own per-leaf fallback is what
// lets a row written before a key existed still parse (P52 §4.3).
const settingsSelectAllSQL = `SELECT key, value FROM settings`

type SettingsRepo struct {
	DB *sql.DB

	// selectAll is prepared once by repos.New (P52 §5.4 — this is one of the app's hot boot-path
	// reads). nil when a SettingsRepo is constructed directly (e.g. in tests), which falls back
	// to an ad-hoc query with identical SQL.
	selectAll *sql.Stmt
}

func (r *SettingsRepo) GetAll() (model.Settings, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if r.selectAll != nil {
		rows, err = r.selectAll.Query()
	} else {
		rows, err = r.DB.Query(settingsSelectAllSQL)
	}
	stored, err := appstorage.ScanLeafRows(rows, err)
	if err != nil {
		return model.Settings{}, err
	}

	result := model.DefaultSettings()
	result.Appearance = appsettings.ReadAppearance(stored)
	appsettings.LeafValid(stored, "data.defaultPageSize", &result.Data.DefaultPageSize, model.ValidPageSize)
	appsettings.LeafValid(stored, "cache.l2BudgetMb", &result.Cache.L2BudgetMb, appsettings.InRange(8, 1024))
	appsettings.LeafValid(stored, "advanced.opLogRetentionDays", &result.Advanced.OpLogRetentionDays, appsettings.InRange(1, 365))
	appsettings.LeafValid(stored, "advanced.expensiveQueryRows", &result.Advanced.ExpensiveQueryRows, appsettings.InRange(1_000, 1_000_000_000))
	appsettings.LeafValid(stored, "advanced.logLevel", &result.Advanced.LogLevel, appsettings.ValidLogLevel)
	appsettings.Leaf(stored, "advanced.notifyPrompts", &result.Advanced.NotifyPrompts)
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
	result.Agent = appsettings.ReadAgent(stored)
	return result, nil
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

func upsertAdvancedSection(tx *sql.Tx, a *model.AdvancedPatch) error {
	if a == nil {
		return nil
	}
	if err := appsettings.UpsertOptional(tx, "advanced.opLogRetentionDays", a.OpLogRetentionDays); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "advanced.expensiveQueryRows", a.ExpensiveQueryRows); err != nil {
		return err
	}
	if err := appsettings.UpsertOptional(tx, "advanced.logLevel", a.LogLevel); err != nil {
		return err
	}
	return appsettings.UpsertOptional(tx, "advanced.notifyPrompts", a.NotifyPrompts)
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

// Set validates the patch, writes only the leaves the caller actually patched in one transaction
// (D15 — a full rewrite would touch eleven unrelated rows), and returns GetAll() afterwards.
func (r *SettingsRepo) Set(patch model.SettingsPatch) (model.Settings, error) {
	if err := patch.Validate(); err != nil {
		return model.Settings{}, fmt.Errorf("repos/settings: %w", err)
	}

	err := appstorage.UpdateLeaves(r.DB, r.selectAll, settingsSelectAllSQL, func(tx *sql.Tx, _ map[string]json.RawMessage) error {
		if err := appsettings.UpsertAppearance(tx, patch.Appearance); err != nil {
			return err
		}
		if err := upsertDataSection(tx, patch.Data); err != nil {
			return err
		}
		if err := upsertCacheSection(tx, patch.Cache); err != nil {
			return err
		}
		if err := upsertAdvancedSection(tx, patch.Advanced); err != nil {
			return err
		}
		if err := upsertApiSection(tx, patch.Api); err != nil {
			return err
		}
		if err := upsertDbMcpSection(tx, patch.DbMcp); err != nil {
			return err
		}
		return appsettings.UpsertAgent(tx, patch.Agent)
	})
	if err != nil {
		return model.Settings{}, err
	}
	return r.GetAll()
}
