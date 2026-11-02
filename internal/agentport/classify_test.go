package agentport

import (
	"reflect"
	"testing"
)

// TestClassify guards the interacting rules: location, name, value pattern, flag adjacency and the
// scheme split. A wrong answer either leaks a credential into settings.yaml or seals a plain value.
func TestClassify(t *testing.T) {
	lit := func(s string) []part { return []part{{text: s}} }
	cases := []struct {
		name  string
		l     loc
		field string
		value string
		want  []part
	}{
		{"env secret name", locEnv, "JIRA_TOKEN", "abc", []part{{text: "abc", secret: true}}},
		{"env plain name and value", locEnv, "NODE_ENV", "production", lit("production")},
		{"env key suffix", locEnv, "SSH_KEY", "x", []part{{text: "x", secret: true}}},
		{"env value prefix", locEnv, "MARK", "ghp_abcdef", []part{{text: "ghp_abcdef", secret: true}}},
		{"env jwt value", locEnv, "MARK", "eyJhbGciOi.eyJzdWIi.sig", []part{{text: "eyJhbGciOi.eyJzdWIi.sig", secret: true}}},
		{"env long mixed base64url", locEnv, "MARK", "Abcdefghijklmnopqrstuvwxyz0123456789", []part{{text: "Abcdefghijklmnopqrstuvwxyz0123456789", secret: true}}},
		{"env long lowercase word", locEnv, "MARK", "some-long-kebab-case-name-with-many-words", lit("some-long-kebab-case-name-with-many-words")},
		{"env reference stays", locEnv, "API_KEY", "${API_KEY}", lit("${API_KEY}")},
		{"env empty stays", locEnv, "API_KEY", "", lit("")},
		{"header bearer split", locHeader, "Authorization", "Bearer ghp_x", []part{{text: "Bearer "}, {text: "ghp_x", secret: true}}},
		{"header custom sealed", locHeader, "X-Api", "v", []part{{text: "v", secret: true}}},
		{"header safe", locHeader, "Content-Type", "application/json", lit("application/json")},
		{"header safe case-insensitive", locHeader, "accept", "x", lit("x")},
		{"header reference stays", locHeader, "Authorization", "Bearer ${gh}", lit("Bearer ${gh}")},
		{"url userinfo", locURL, "", "https://u:p@host/x", []part{{text: "https://"}, {text: "u:p", secret: true}, {text: "@host/x"}}},
		{"url secret query", locURL, "", "https://h/x?token=abc&a=b", []part{{text: "https://h/x?token="}, {text: "abc", secret: true}, {text: "&a=b"}}},
		{"url plain", locURL, "", "https://h/x?a=b", lit("https://h/x?a=b")},
		{"command plain", locCommand, "", "npx", lit("npx")},
		{"arg after secret flag", locArg, "--api-key", "xyz", []part{{text: "xyz", secret: true}}},
		{"arg after plain flag", locArg, "--port", "8080", lit("8080")},
		{"arg flag=value", locArg, "", "--token=abc", []part{{text: "--token="}, {text: "abc", secret: true}}},
		{"arg flag only", locArg, "", "--token", lit("--token")},
		{"arg prefix pattern", locArg, "", "sk-abcdef", []part{{text: "sk-abcdef", secret: true}}},
	}
	for _, c := range cases {
		if got := classify(c.l, c.field, c.value); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: classify(%q) = %+v, want %+v", c.name, c.value, got, c.want)
		}
	}
}
