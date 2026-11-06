package adapterhost

import (
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// This file is data-ops.ts's eight request/response wire shapes, plus a Validate() per request:
// a naive json.Unmarshal is not a substitute for zod's safeParse, so every field zod constrains
// here gets its own explicit Go check rather than trusting the decode alone. Field names and JSON
// keys match data-ops.ts's wire form exactly, byte for byte — this is the payload shape
// a data-plane frame decodes into on both sides of the wire.

const maxFilterChars = 4096

func validFilter(f *string) error {
	if f != nil && model.UTF16Len(*f) > maxFilterChars {
		return fmt.Errorf("filter exceeds %d UTF-16 units", maxFilterChars)
	}
	return nil
}

func validPageSize(n int) error {
	switch n {
	case 10, 100, 1000, 10000:
		return nil
	default:
		return fmt.Errorf("invalid pageSize: %d", n)
	}
}

func validCursor(c model.PageCursor) error {
	switch c.Mode {
	case "offset":
		if c.Offset < 0 {
			return fmt.Errorf("cursor offset must be >= 0")
		}
		return nil
	case "after", "before":
		return nil
	default:
		return fmt.Errorf("invalid cursor mode: %q", c.Mode)
	}
}

// ReadRequestWire is data-ops.ts's ReadRequestWire.
type ReadRequestWire struct {
	OpID         string           `json:"opId"`
	TabID        *string          `json:"tabId"`
	ConnectionID string           `json:"connectionId"`
	Path         string           `json:"path"`
	Projection   []string         `json:"projection"`
	Filter       *string          `json:"filter"`
	Sort         *model.SortSpec  `json:"sort"`
	PageSize     int              `json:"pageSize"`
	Cursor       model.PageCursor `json:"cursor"`
}

func (r ReadRequestWire) Validate() error {
	if r.ConnectionID == "" {
		return fmt.Errorf("connectionId is required")
	}
	if err := validFilter(r.Filter); err != nil {
		return err
	}
	if err := validPageSize(r.PageSize); err != nil {
		return err
	}
	return validCursor(r.Cursor)
}

// ReadResponse is data-ops.ts's ReadResponse.
type ReadResponse struct {
	Page   page.Page `json:"page"`
	Source string    `json:"source"` // "cache" | "server"
}

// CountRequestWire is data-ops.ts's CountRequestWire.
type CountRequestWire struct {
	OpID         string  `json:"opId"`
	TabID        *string `json:"tabId"`
	ConnectionID string  `json:"connectionId"`
	Path         string  `json:"path"`
	Filter       *string `json:"filter"`
	Refresh      bool    `json:"refresh"`
}

func (r CountRequestWire) Validate() error {
	if r.ConnectionID == "" {
		return fmt.Errorf("connectionId is required")
	}
	return validFilter(r.Filter)
}

// CountResponse is data-ops.ts's CountResponse.
type CountResponse struct {
	Value  int64  `json:"value"`
	Exact  bool   `json:"exact"`
	At     int64  `json:"at"`
	Stale  bool   `json:"stale"`
	Source string `json:"source"` // "cache" | "server"
}

// InvalidateRequestWire is data-ops.ts's InvalidateRequestWire. Scope defaults to "all" (the
// explicit Refresh button) when empty, matching the TS optional field's own default.
type InvalidateRequestWire struct {
	ConnectionID string `json:"connectionId"`
	Path         string `json:"path"`
	Scope        string `json:"scope"` // "" | "all" | "pages"
}

func (r InvalidateRequestWire) Validate() error {
	if r.ConnectionID == "" {
		return fmt.Errorf("connectionId is required")
	}
	switch r.Scope {
	case "", "all", "pages":
		return nil
	default:
		return fmt.Errorf("invalid scope: %q", r.Scope)
	}
}

// PreviewRequestWire is data-ops.ts's PreviewRequestWire.
type PreviewRequestWire struct {
	ConnectionID string                `json:"connectionId"`
	Path         string                `json:"path"`
	Ops          []model.MutationRowOp `json:"ops"`
}

func (r PreviewRequestWire) Validate() error {
	if r.ConnectionID == "" {
		return fmt.Errorf("connectionId is required")
	}
	return nil
}

// PreviewResponse is data-ops.ts's PreviewResponse.
type PreviewResponse struct {
	Statements []string `json:"statements"`
}

// MutateRequestWire is data-ops.ts's MutateRequestWire.
type MutateRequestWire struct {
	OpID         string                `json:"opId"`
	TabID        *string               `json:"tabId"`
	ConnectionID string                `json:"connectionId"`
	Path         string                `json:"path"`
	Ops          []model.MutationRowOp `json:"ops"`
}

func (r MutateRequestWire) Validate() error {
	if r.ConnectionID == "" {
		return fmt.Errorf("connectionId is required")
	}
	return nil
}

// MutateResponse is data-ops.ts's MutateResponse.
type MutateResponse struct {
	AffectedRows int `json:"affectedRows"`
}

// ExecuteRequestWire is data-ops.ts's ExecuteRequestWire.
type ExecuteRequestWire struct {
	OpID         string   `json:"opId"`
	TabID        *string  `json:"tabId"`
	ConnectionID string   `json:"connectionId"`
	Path         string   `json:"path"`
	Statements   []string `json:"statements"`
	// Cap is set by Go callers only (dbmcp); zero means adapters.DefaultConsoleCap.
	Cap page.ResultCap `json:"-"`
	// ReadOnly is set by Go callers only (dbmcp); see model.ConsoleRequest.ReadOnly.
	ReadOnly bool `json:"-"`
	// StatementTimeout is set by Go callers only (dbmcp); zero means none. See model.ConsoleRequest.
	StatementTimeout time.Duration `json:"-"`
	// TimeoutByEngine is set with StatementTimeout when the engine enforces it too: the host timer
	// then fires after a grace period instead of at the limit.
	TimeoutByEngine bool `json:"-"`
}

func (r ExecuteRequestWire) Validate() error {
	if r.OpID == "" {
		return fmt.Errorf("opId is required")
	}
	if r.ConnectionID == "" {
		return fmt.Errorf("connectionId is required")
	}
	if len(r.Statements) == 0 {
		return fmt.Errorf("statements must be non-empty")
	}
	return nil
}

// ExecuteResponse is data-ops.ts's ExecuteResponse.
type ExecuteResponse struct {
	Pages []page.Page `json:"pages"`
}

const maxLocalFilePathChars = 4096

// ObjectDownloadRequestWire is data-ops.ts's ObjectDownloadRequestWire.
type ObjectDownloadRequestWire struct {
	OpID         string  `json:"opId"`
	TabID        *string `json:"tabId"`
	ConnectionID string  `json:"connectionId"`
	Path         string  `json:"path"`
	DestPath     string  `json:"destPath"`
}

func (r ObjectDownloadRequestWire) Validate() error {
	if r.ConnectionID == "" {
		return fmt.Errorf("connectionId is required")
	}
	if r.DestPath == "" || len([]rune(r.DestPath)) > maxLocalFilePathChars {
		return fmt.Errorf("destPath must be 1-%d characters", maxLocalFilePathChars)
	}
	return nil
}

// ObjectDownloadResponse is data-ops.ts's ObjectDownloadResponse.
type ObjectDownloadResponse struct {
	Bytes int64 `json:"bytes"`
}
