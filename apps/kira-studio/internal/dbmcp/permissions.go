package dbmcp

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// modes is one connection's three MCP permission words (M2 §4.1), each one of deny/allow/prompt.
type modes struct{ read, write, ddl string }

// reviewResults resolves a connection's result review: its own override, else the app setting.
func (s *Server) reviewResults(c model.ConnectionSummary) bool {
	switch c.McpReviewResults {
	case "review":
		return true
	case "send":
		return false
	default:
		return s.cfg.PreviewResponses()
	}
}

// modesOf reads c's own MCP permission fields.
func modesOf(c model.ConnectionSummary) modes {
	return modes{read: c.McpReadMode, write: c.McpWriteMode, ddl: c.McpDdlMode}
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

// Declared run_query modes.
const (
	modeRead  = "read"
	modeWrite = "write"
	modeDDL   = "ddl"
)

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
	Read    modeAccess `json:"read"`
	Write   modeAccess `json:"write"`
	DDL     modeAccess `json:"ddl"`
	Summary string     `json:"summary"`
}

func (a connectionAccess) of(mode string) modeAccess {
	switch mode {
	case modeWrite:
		return a.Write
	case modeDDL:
		return a.DDL
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
