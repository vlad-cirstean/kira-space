package dbmcp

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// statementTimeoutLimit is the built-in ExecLimit LimitStatementTimeout (P529).
type statementTimeoutLimit struct{}

func (statementTimeoutLimit) Name() string { return LimitStatementTimeout }

// Apply sets the connection's timeout on req unless c.Exempt or the setting is 0. An exempted call
// carries no timeout at all, so the adapters issue no SET and it runs at the connection's baseline.
func (statementTimeoutLimit) Apply(ctx context.Context, c LimitCall, req *adapterhost.ExecuteRequestWire) (context.Context, func()) {
	if !c.Exempt {
		applyStatementTimeout(req, c.Summary, c.Mode)
	}
	return ctx, func() {}
}

// Hit maps a typed timeout to the measured run time (never the router-level elapsed: that includes
// throttle queue wait) against the limit.
func (statementTimeoutLimit) Hit(_ LimitCall, err error, _ time.Duration) *LimitHit {
	var d *adapters.TimeoutDetail
	if !errors.As(err, &d) {
		return nil
	}
	return &LimitHit{
		Limit:    LimitStatementTimeout,
		Measured: math.Ceil(float64(max(d.Elapsed, d.Limit)) / float64(time.Millisecond)),
		Max:      float64(d.Limit / time.Millisecond),
		Unit:     UnitMillis,
	}
}

// applyStatementTimeout sets StatementTimeout and TimeoutByEngine on req for summary and mode.
func applyStatementTimeout(req *adapterhost.ExecuteRequestWire, summary model.ConnectionSummary, mode string) {
	if summary.McpStatementTimeoutSec <= 0 {
		return
	}
	req.StatementTimeout = time.Duration(summary.McpStatementTimeoutSec) * time.Second
	req.TimeoutByEngine = timeoutEnforcementOf(summary.Kind, mode) == "session"
}

// timeoutEnforcementOf names what stops a statement past the timeout for kind and mode: session (the
// engine), app (the host timer through the Stop path) or none (no console, nothing runs).
func timeoutEnforcementOf(kind, mode string) string {
	read, write, ddl := "none", "none", "none"
	switch kind {
	case "postgres":
		read, write, ddl = "session", "session", "session"
	case "mysql":
		read, write, ddl = "session", "app", "app"
	case "mariadb":
		read, write, ddl = "session", "session", "app"
	case "clickhouse":
		read, write, ddl = "session", "app", "app"
	case "sqlite", "mongodb", "redis":
		read, write, ddl = "app", "app", "app"
	case "kibana":
		read = "app"
	}
	switch mode {
	case modeRead, modeMasked:
		return read
	case modeWrite:
		return write
	}
	return ddl
}

// statementTimeoutView is list_connections' statementTimeout.
type statementTimeoutView struct {
	Seconds     int               `json:"seconds"`
	Enforcement map[string]string `json:"enforcement"`
}

func timeoutViewOf(c model.ConnectionSummary) statementTimeoutView {
	return statementTimeoutView{
		Seconds: c.McpStatementTimeoutSec,
		Enforcement: map[string]string{
			modeRead:  timeoutEnforcementOf(c.Kind, modeRead),
			modeWrite: timeoutEnforcementOf(c.Kind, modeWrite),
			modeDDL:   timeoutEnforcementOf(c.Kind, modeDDL),
		},
	}
}
