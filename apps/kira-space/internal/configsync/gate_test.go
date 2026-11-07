package configsync

import "testing"

// TestGateRules guards the name, URI and entry-id rules of the credential gate.
func TestGateRules(t *testing.T) {
	file := []struct {
		name, text string
		blocked    bool
	}{
		{"password literal", "password: hunter2\n", true},
		{"password reference", "password: ${secret:db}\n", false},
		{"nested token literal", "auth:\n  token: abc\n", true},
		{"uri with password", "uri: postgres://u:p@h/db\n", true},
		{"uri without password", "uri: postgres://u@h/db\n", false},
		{"uri reference", "uri: postgres://u:${secret:p}@h/db\n", false},
		{"env name rule", "env:\n  GITHUB_TOKEN: hunter2\n", true},
		{"env plain", "env:\n  MODE: dev\n", false},
		{"variable name value", "bearer_token_env_var: MY_TOKEN\n", false},
		{"non-string password", "password: 12\n", false},
	}
	for _, c := range file {
		if got := credentialProblem([]byte(c.text), false) != ""; got != c.blocked {
			t.Errorf("%s: blocked = %v, want %v", c.name, got, c.blocked)
		}
	}
	entry := []struct {
		id      string
		v       any
		blocked bool
	}{
		{"env/GITHUB_TOKEN", "hunter2", true},
		{"env/GITHUB_TOKEN", "${secret:gh}", false},
		{"env/MODE", "dev", false},
		{"mcp_servers/s", map[string]any{"env": map[string]any{"API_KEY": "x"}}, true},
		{"mcp_servers/s", map[string]any{"password": "x"}, true},
	}
	for _, c := range entry {
		if got := entryCredentialProblem(c.id, c.v) != ""; got != c.blocked {
			t.Errorf("entry %s %v: blocked = %v, want %v", c.id, c.v, got, c.blocked)
		}
	}
}
