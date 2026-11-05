package scripts

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MCP approval modes of an exposed script.
const (
	MCPRunPrompt = "prompt"
	MCPRunAllow  = "allow"
)

// MaxMCPDescriptionRunes caps the description an agent reads.
const MaxMCPDescriptionRunes = 1000

// MCPAccess opts a normal script into the Database MCP server. nil on a script without the block.
type MCPAccess struct {
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
	// Run is MCPRunPrompt (ask before each run) or MCPRunAllow.
	Run string `json:"run"`
	// Timeout bounds an agent's run; "" means DefaultScheduleTimeout.
	Timeout string `json:"timeout"`
}

// TimeoutDuration parses Timeout; Validate has already bounded it.
func (m MCPAccess) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(m.Timeout)
	if err != nil || d <= 0 {
		d, _ = time.ParseDuration(DefaultScheduleTimeout)
	}
	return d
}

// isZero reports whether the block says nothing beyond the defaults.
func (m MCPAccess) isZero() bool {
	return !m.Enabled && m.Description == "" && (m.Run == "" || m.Run == MCPRunPrompt) && m.Timeout == ""
}

// checkTimeoutText reports whether text is a duration within the shared 1m to 24h bounds.
func checkTimeoutText(text string) bool {
	d, err := time.ParseDuration(text)
	return err == nil && d >= MinScheduleTimeout && d <= MaxScheduleTimeout
}

// validMCPAccess checks and normalises m against the kind of script it belongs to.
func validMCPAccess(m *MCPAccess, kind string) error {
	m.Description = strings.TrimSpace(m.Description)
	m.Timeout = strings.TrimSpace(m.Timeout)
	switch m.Run {
	case "":
		m.Run = MCPRunPrompt
	case MCPRunPrompt, MCPRunAllow:
	default:
		return invalid("/mcp/run", "backend.core.runs.mcpRun.body")
	}
	if utf8.RuneCountInString(m.Description) > MaxMCPDescriptionRunes {
		return invalid("/mcp/description", "backend.core.runs.mcpDescriptionLong.body", "max", strconv.Itoa(MaxMCPDescriptionRunes))
	}
	if m.Timeout != "" && !checkTimeoutText(m.Timeout) {
		return invalid("/mcp/timeout", "backend.core.runs.mcpTimeout.body")
	}
	if m.Enabled && kind == KindSmart {
		return invalid("/mcp/enabled", "backend.core.runs.mcpSmart.body")
	}
	return nil
}
