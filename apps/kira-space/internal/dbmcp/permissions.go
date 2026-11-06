package dbmcp

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// modes is one connection's MCP permission words (M2 §4.1), each one of deny/allow/prompt. masked is
// browse_table's, with inherit already resolved to read's value.
type modes struct{ read, write, ddl, masked string }

// reviewResolver maps a review setting to held or not. inherit follows the app setting, read at most
// once per resolver.
func (s *Server) reviewResolver() func(string) bool {
	var app, read bool
	return func(v string) bool {
		switch v {
		case "review":
			return true
		case "send":
			return false
		}
		if !read {
			app, read = s.cfg.PreviewResponses(), true
		}
		return app
	}
}

// reviewFor reports whether a call's result is held: the setting of the declared mode or of the
// detected class. Either one holds, so declaring a weaker mode than the statement looks like never
// skips review (tightenByClass accepts a weaker statement than its mode). An unknown class holds when
// any mode does.
func (s *Server) reviewFor(c model.ConnectionSummary, mode string, class adapters.OpClass) bool {
	if mode == modeMasked {
		return s.reviewForMode(c, mode)
	}
	held := s.reviewResolver()
	if held(c.McpReview.For(mode)) {
		return true
	}
	if class == adapters.ClassUnknown {
		return held(c.McpReview.Read) || held(c.McpReview.Write) || held(c.McpReview.Ddl)
	}
	return held(c.McpReview.For(classMode(class)))
}

// reviewForMode reports whether mode's own setting holds its results. browse_table uses it alone: it has
// no agent SQL that could look stronger than its mode, so read's setting never forces review on it.
func (s *Server) reviewForMode(c model.ConnectionSummary, mode string) bool {
	return s.reviewResolver()(c.McpReview.For(mode))
}

// reviewView is list_connections' reviewResults: which modes hold their results for the user.
type reviewView struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
	DDL   bool `json:"ddl"`
	// Masked is browse_table's.
	Masked bool `json:"masked"`
}

func (s *Server) reviewViewOf(c model.ConnectionSummary) reviewView {
	held := s.reviewResolver()
	return reviewView{
		Read: held(c.McpReview.Read), Write: held(c.McpReview.Write), DDL: held(c.McpReview.Ddl),
		Masked: held(c.McpReview.Masked),
	}
}

// modesOf reads c's own MCP permission fields. An inherit masked value follows read.
func modesOf(c model.ConnectionSummary) modes {
	m := modes{read: c.McpReadMode, write: c.McpWriteMode, ddl: c.McpDdlMode, masked: c.McpMaskedMode}
	if m.masked == "" || m.masked == model.McpMaskedInherit {
		m.masked = m.read
	}
	return m
}

// strictnessRank orders the three modes from least to most permissive: deny beats prompt beats
// allow, so the strictest of several modes is the one with the lowest rank.
var strictnessRank = map[string]int{"deny": 0, "prompt": 1, "allow": 2}

// strictestOf returns the strictest (lowest-ranked) of the three configured modes — an
// unclassifiable statement must never be easier to run than the statement it might be.
func (m modes) strictestOf() string {
	strictest := m.read
	for _, other := range []string{m.write, m.ddl} {
		if strictnessRank[other] < strictnessRank[strictest] {
			strictest = other
		}
	}
	return strictest
}

// verdictFor returns the mode governing class: the named one for read/write/ddl, and for
// ClassUnknown the strictest of the three.
func verdictFor(m modes, class adapters.OpClass) string {
	switch class {
	case adapters.ClassRead:
		return m.read
	case adapters.ClassWrite:
		return m.write
	case adapters.ClassDDL:
		return m.ddl
	default:
		return m.strictestOf()
	}
}

// Declared run_query modes, plus browse_table's masked read. masked is no run_query mode or statement
// class: it gates only browse_table.
const (
	modeRead   = "read"
	modeWrite  = "write"
	modeDDL    = "ddl"
	modeMasked = "masked"
)

// browseKinds have a structured browse: the shared relational builder plus ClickHouse's. Mirrors
// BROWSE_KINDS in packages/shared/domain/dbmcp.ts.
var browseKinds = map[string]bool{"postgres": true, "mysql": true, "mariadb": true, "sqlite": true, "clickhouse": true}

// HasBrowse reports whether kind supports browse_table.
func HasBrowse(kind string) bool { return browseKinds[kind] }

var queryModes = []string{modeRead, modeWrite, modeDDL}

// modeRank orders the declared modes and the classes a statement can look like.
var modeRank = map[string]int{modeRead: 0, modeWrite: 1, modeDDL: 2}

// classRank is the rank of a statement class; unknown sits above write, so it passes only under ddl.
func classRank(class adapters.OpClass) int {
	switch class {
	case adapters.ClassRead:
		return modeRank[modeRead]
	case adapters.ClassWrite:
		return modeRank[modeWrite]
	case adapters.ClassDDL:
		return modeRank[modeDDL]
	default:
		return modeRank[modeWrite] + 1
	}
}

// classMode names the mode a statement class needs; unknown needs ddl.
func classMode(class adapters.OpClass) string {
	switch class {
	case adapters.ClassRead:
		return modeRead
	case adapters.ClassWrite:
		return modeWrite
	default:
		return modeDDL
	}
}

// writeConsoleKinds are the kinds with a console that can change data or schema. The rest have no
// write path: kibana runs one Discover search; kafka, s3 and sqs have no console.
var writeConsoleKinds = map[string]bool{
	"postgres": true, "mysql": true, "mariadb": true, "sqlite": true,
	"clickhouse": true, "mongodb": true, "redis": true,
}

// HasWriteConsole reports whether kind can run write or ddl statements.
func HasWriteConsole(kind string) bool { return writeConsoleKinds[kind] }

// enforcementOf names what backs mode read for kind: session (the engine refuses writes on the read
// lane), app (Kira's own check; the engine has no read-only session) or none.
func enforcementOf(kind string) string {
	switch kind {
	case "postgres", "mysql", "mariadb", "sqlite", "clickhouse":
		return "session"
	case "mongodb", "redis":
		return "app"
	}
	return "none"
}

// modeAccess is one mode's answer for a connection.
type modeAccess struct {
	Allowed  bool   `json:"allowed"`
	Approval bool   `json:"approval"`
	Reason   string `json:"reason,omitempty"`
}

// connectionAccess is what a connection lets an MCP client do per declared mode.
type connectionAccess struct {
	Read  modeAccess `json:"read"`
	Write modeAccess `json:"write"`
	DDL   modeAccess `json:"ddl"`
	// Masked is browse_table's access; Summary lists run_query modes only.
	Masked  modeAccess `json:"masked"`
	Summary string     `json:"summary"`
}

func (a connectionAccess) of(mode string) modeAccess {
	switch mode {
	case modeWrite:
		return a.Write
	case modeDDL:
		return a.DDL
	case modeMasked:
		return a.Masked
	}
	return a.Read
}

// accessOf is the one place a connection's permissions, read-only flag and kind become per-mode
// access. list_connections reports it and every gate and refusal text reads it.
func accessOf(c model.ConnectionSummary) connectionAccess {
	m := modesOf(c)
	one := func(mode, configured string) modeAccess {
		switch {
		case mode != modeRead && c.ReadOnly:
			return modeAccess{Reason: "read-only connection"}
		case mode != modeRead && !HasWriteConsole(c.Kind):
			return modeAccess{Reason: "no write console"}
		}
		switch configured {
		case "allow":
			return modeAccess{Allowed: true}
		case "prompt":
			return modeAccess{Allowed: true, Approval: true}
		}
		return modeAccess{Reason: "permission denied"}
	}
	a := connectionAccess{Read: one(modeRead, m.read), Write: one(modeWrite, m.write), DDL: one(modeDDL, m.ddl)}
	// A read: neither read_only nor the write console matters. Only the kind's browse support does.
	if HasBrowse(c.Kind) {
		a.Masked = one(modeRead, m.masked)
	} else {
		a.Masked = modeAccess{Reason: "no structured browse"}
	}
	var parts []string
	for _, mode := range queryModes {
		ma := a.of(mode)
		switch {
		case ma.Approval:
			parts = append(parts, mode+" (approval)")
		case ma.Allowed:
			parts = append(parts, mode)
		}
	}
	a.Summary = "none"
	if len(parts) > 0 {
		a.Summary = strings.Join(parts, ", ")
	}
	return a
}

// modeVerdict is the gate for a declared mode: deny, prompt or allow. Only the declared mode's own
// permission decides; the statement text never does.
func modeVerdict(c model.ConnectionSummary, mode string) string {
	ma := accessOf(c).of(mode)
	switch {
	case !ma.Allowed:
		return "deny"
	case ma.Approval:
		return "prompt"
	}
	return "allow"
}

// maskedModeOf is the stored browse_table setting, inherit when unset.
func maskedModeOf(c model.ConnectionSummary) string {
	if c.McpMaskedMode == "" {
		return model.McpMaskedInherit
	}
	return c.McpMaskedMode
}
