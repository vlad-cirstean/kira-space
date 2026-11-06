package model

import (
	"fmt"

	"github.com/kirathecat/kira-studio/internal/appsettings"
)

type DataSettings struct {
	DefaultPageSize int `json:"defaultPageSize"`
}

type CacheSettings struct {
	L2BudgetMb int `json:"l2BudgetMb"`
}

// DbAdvancedSettings' own LogLevel is Studio's own diagnostic-log verbosity, its own name/key
// (`advanced.logLevel`), no longer the shared appsettings.AdvancedCore embed both apps used to
// share under a name that never fit Studio's own module set.
// internal/logging.SetLevel is its actual mechanism; appsettings.ValidLogLevel is its validator,
// genuinely shared with Kira Space's own equivalent leaf.
type DbAdvancedSettings struct {
	OpLogRetentionDays int `json:"opLogRetentionDays"`
	// P18 D14/D20: an estimated-rows-read threshold, never a cost unit — settings.ts's own
	// EXPENSIVE_QUERY_ROWS_RANGE comment carries the full argument.
	ExpensiveQueryRows int    `json:"expensiveQueryRows"`
	LogLevel           string `json:"logLevel"`
	// NotifyPrompts posts an OS notification when a popup waits for an answer (P246).
	NotifyPrompts bool `json:"notifyPrompts"`
}

// DbMcpSettings is the embedded DB MCP server instance's persisted on/off record
// (internal/bridge/dbmcp.go owns the actual start/stop side effect).
type DbMcpSettings struct {
	ServerEnabled bool `json:"serverEnabled"`
	// PreviewResponses holds each run_query result for the user's review before the AI client gets it.
	PreviewResponses bool `json:"previewResponses"`
	// FocusOnApproval brings the window forward when a query or result waits for approval.
	FocusOnApproval bool `json:"focusOnApproval"`
	// APIRequests lists the API request tools to agents.
	APIRequests bool `json:"apiRequests"`
	// APIApproval is "always" (each request waits for the user) or "never".
	APIApproval string `json:"apiApproval"`
}

// ApiSettings mirrors packages/shared/domain/settings.ts's apiSettingsSchema — the
// seven request settings that used to be httpclient package constants (internal/httpclient's own
// options.go states the coupling back at this file: its normalize() defaults must equal
// DefaultDbSettings().Api field for field). HTTPVersion is "1.1" | "2"; RequestTimeoutMs is
// milliseconds, 0 = none; MaxResponseMb is whole MB, 0 = unlimited.
type ApiSettings struct {
	HTTPVersion      string `json:"httpVersion"`
	RequestTimeoutMs int    `json:"requestTimeoutMs"`
	MaxResponseMb    int    `json:"maxResponseMb"`
	SSLVerify        bool   `json:"sslVerify"`
	FollowRedirects  bool   `json:"followRedirects"`
	MaxRedirects     int    `json:"maxRedirects"`
	DisableCookieJar bool   `json:"disableCookieJar"`
}

type DbSettings struct {
	Appearance appsettings.Appearance `json:"appearance"`
	Data       DataSettings           `json:"data"`
	Cache      CacheSettings          `json:"cache"`
	Advanced   DbAdvancedSettings     `json:"advanced"`
	Api        ApiSettings            `json:"api"`
	DbMcp      DbMcpSettings          `json:"dbMcp"`
	Agent      appsettings.Agent      `json:"agent"`
}

// DefaultDbSettings mirrors packages/shared/domain/settings.ts's defaultSettings verbatim.
func DefaultDbSettings() DbSettings {
	return DbSettings{
		Appearance: appsettings.DefaultAppearance(),
		Data:       DataSettings{DefaultPageSize: 100},
		Cache:      CacheSettings{L2BudgetMb: 64},
		Advanced: DbAdvancedSettings{
			OpLogRetentionDays: 30,
			ExpensiveQueryRows: 100_000,
			LogLevel:           "info",
			NotifyPrompts:      true,
		},
		// P90 §2.1: three deliberate default changes from pre-P90 httpclient behaviour — timeout
		// 30s -> none, max response 10 MiB -> 50 MB (P160: -> 5 MB, large bodies freeze the viewer),
		// max redirects unchanged at 10.
		Api: ApiSettings{
			HTTPVersion:      "2",
			RequestTimeoutMs: 0,
			MaxResponseMb:    5,
			SSLVerify:        true,
			FollowRedirects:  true,
			MaxRedirects:     10,
			DisableCookieJar: true,
		},
		DbMcp: DbMcpSettings{ServerEnabled: false, PreviewResponses: false, FocusOnApproval: true, APIRequests: false, APIApproval: "always"},
		Agent: appsettings.DefaultAgent(),
	}
}

// DataPatch and CachePatch mirror settings.ts's `.partial()` per-section patch shapes — every leaf
// is optional, present only when the caller means to change it (SettingsRepo.Set writes only
// the leaves patched). Appearance's own patch shape is appsettings.AppearancePatch.
type DataPatch struct {
	DefaultPageSize *int `json:"defaultPageSize,omitempty"`
}

type CachePatch struct {
	L2BudgetMb *int `json:"l2BudgetMb,omitempty"`
}

// DbAdvancedPatch mirrors DbAdvancedSettings' own `.partial()` shape.
type DbAdvancedPatch struct {
	OpLogRetentionDays *int    `json:"opLogRetentionDays,omitempty"`
	ExpensiveQueryRows *int    `json:"expensiveQueryRows,omitempty"`
	LogLevel           *string `json:"logLevel,omitempty"`
	NotifyPrompts      *bool   `json:"notifyPrompts,omitempty"`
}

// ApiPatch mirrors ApiSettings' own `.partial()` shape (P90 item 1).
type ApiPatch struct {
	HTTPVersion      *string `json:"httpVersion,omitempty"`
	RequestTimeoutMs *int    `json:"requestTimeoutMs,omitempty"`
	MaxResponseMb    *int    `json:"maxResponseMb,omitempty"`
	SSLVerify        *bool   `json:"sslVerify,omitempty"`
	FollowRedirects  *bool   `json:"followRedirects,omitempty"`
	MaxRedirects     *int    `json:"maxRedirects,omitempty"`
	DisableCookieJar *bool   `json:"disableCookieJar,omitempty"`
}

// DbMcpPatch mirrors DbMcpSettings' own `.partial()` shape (M1 §6.2).
type DbMcpPatch struct {
	ServerEnabled    *bool   `json:"serverEnabled,omitempty"`
	PreviewResponses *bool   `json:"previewResponses,omitempty"`
	FocusOnApproval  *bool   `json:"focusOnApproval,omitempty"`
	APIRequests      *bool   `json:"apiRequests,omitempty"`
	APIApproval      *string `json:"apiApproval,omitempty"`
}

type DbSettingsPatch struct {
	Appearance *appsettings.AppearancePatch `json:"appearance,omitempty"`
	Data       *DataPatch                   `json:"data,omitempty"`
	Cache      *CachePatch                  `json:"cache,omitempty"`
	Advanced   *DbAdvancedPatch             `json:"advanced,omitempty"`
	Api        *ApiPatch                    `json:"api,omitempty"`
	DbMcp      *DbMcpPatch                  `json:"dbMcp,omitempty"`
	Agent      *appsettings.AgentPatch      `json:"agent,omitempty"`
}

// ValidHTTPVersion mirrors settings.ts's HTTP_VERSIONS — Go's HTTP/1 *is* 1.1, so "1" is never a
// member (internal/httpclient/options.go's own comment states why).
func ValidHTTPVersion(v string) bool {
	return v == "1.1" || v == "2"
}

// ValidAPIApproval mirrors settings.ts's apiApproval enum.
func ValidAPIApproval(v string) bool {
	return v == "always" || v == "never"
}

// ValidPageSize mirrors settings.ts's pageSizeSchema (shared with tabs.ts's per-kind page sizes).
func ValidPageSize(v int) bool {
	switch v {
	case 10, 100, 1000, 10000:
		return true
	default:
		return false
	}
}

// InRange (settings.ts's z.number().int().min(lo).max(hi)), ValidRowDensity and ValidLogLevel
// moved to appsettings — this file's own remaining bounds
// (Data/Cache/Advanced/Api are all per-app-only sections) are built on appsettings.InRange
// directly rather than a redundant local copy.
var (
	validL2BudgetMb         = appsettings.InRange(8, 1024)
	validOpLogRetentionDays = appsettings.InRange(1, 365)
	validExpensiveQueryRows = appsettings.InRange(1_000, 1_000_000_000)
	// P90 §2.1: settings.ts's own REQUEST_TIMEOUT_MS_RANGE/MAX_RESPONSE_MB_RANGE/MAX_REDIRECTS_RANGE.
	validRequestTimeoutMs = appsettings.InRange(0, 3_600_000)
	validMaxResponseMb    = appsettings.InRange(0, 2048)
	validMaxRedirects     = appsettings.InRange(0, 100)
)

func validateDataSection(d *DataPatch) error {
	if d != nil && d.DefaultPageSize != nil && !ValidPageSize(*d.DefaultPageSize) {
		return fmt.Errorf("model: data.defaultPageSize: invalid value %d", *d.DefaultPageSize)
	}
	return nil
}

func validateCacheSection(c *CachePatch) error {
	if c != nil && c.L2BudgetMb != nil && !validL2BudgetMb(*c.L2BudgetMb) {
		return fmt.Errorf("model: cache.l2BudgetMb: out of range value %d", *c.L2BudgetMb)
	}
	return nil
}

func validateDbAdvancedSection(a *DbAdvancedPatch) error {
	if a == nil {
		return nil
	}
	if a.OpLogRetentionDays != nil && !validOpLogRetentionDays(*a.OpLogRetentionDays) {
		return fmt.Errorf("model: advanced.opLogRetentionDays: out of range value %d", *a.OpLogRetentionDays)
	}
	if a.ExpensiveQueryRows != nil && !validExpensiveQueryRows(*a.ExpensiveQueryRows) {
		return fmt.Errorf("model: advanced.expensiveQueryRows: out of range value %d", *a.ExpensiveQueryRows)
	}
	if a.LogLevel != nil && !appsettings.ValidLogLevel(*a.LogLevel) {
		return fmt.Errorf("model: advanced.logLevel: invalid value %q", *a.LogLevel)
	}
	return nil
}

func validateApiSection(a *ApiPatch) error {
	if a == nil {
		return nil
	}
	if a.HTTPVersion != nil && !ValidHTTPVersion(*a.HTTPVersion) {
		return fmt.Errorf("model: api.httpVersion: invalid value %q", *a.HTTPVersion)
	}
	if a.RequestTimeoutMs != nil && !validRequestTimeoutMs(*a.RequestTimeoutMs) {
		return fmt.Errorf("model: api.requestTimeoutMs: out of range value %d", *a.RequestTimeoutMs)
	}
	if a.MaxResponseMb != nil && !validMaxResponseMb(*a.MaxResponseMb) {
		return fmt.Errorf("model: api.maxResponseMb: out of range value %d", *a.MaxResponseMb)
	}
	if a.MaxRedirects != nil && !validMaxRedirects(*a.MaxRedirects) {
		return fmt.Errorf("model: api.maxRedirects: out of range value %d", *a.MaxRedirects)
	}
	return nil
}

func validateDbMcpSection(d *DbMcpPatch) error {
	if d != nil && d.APIApproval != nil && !ValidAPIApproval(*d.APIApproval) {
		return fmt.Errorf("model: dbMcp.apiApproval: invalid value %q", *d.APIApproval)
	}
	return nil
}

// Validate checks every leaf the caller actually patched against settings.ts's bounds, naming
// the offending leaf in the error — fontFamily and fontSize have no bounds in the TS schema
// either, so they are accepted as-is.
func (p DbSettingsPatch) Validate() error {
	if err := appsettings.ValidateAppearance(p.Appearance); err != nil {
		return err
	}
	if err := validateDataSection(p.Data); err != nil {
		return err
	}
	if err := validateCacheSection(p.Cache); err != nil {
		return err
	}
	if err := validateDbAdvancedSection(p.Advanced); err != nil {
		return err
	}
	if err := validateApiSection(p.Api); err != nil {
		return err
	}
	if err := validateDbMcpSection(p.DbMcp); err != nil {
		return err
	}
	return appsettings.ValidateAgent(p.Agent)
}
