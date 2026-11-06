package testsupport

import (
	"strings"
	"testing"
)

// ServerMajor's parsing has several interacting lexical rules (a "-suffix" flavour tag, a
// namespaced image with a "/" before the tag's ":", a tagless digest pin, a bare name with no
// tag at all) — CLAUDE.md's bar for a dedicated unit test. ImageFor itself is a two-line
// getenv-or-default and gets nothing.
func TestServerMajor(t *testing.T) {
	cases := []struct {
		name, kind, pinned, want string
	}{
		{"flavour suffix", "postgres", "postgres:17-alpine", "17"},
		{"plain dotted tag", "mariadb", "mariadb:12.3", "12"},
		{"namespaced image", "clickhouse", "clickhouse/clickhouse-server:26.8", "26"},
		{"latest tag has no leading number", "localstack", "localstack/localstack:latest", ""},
		{"bare name, no tag", "kafka", "confluentinc/cp-kafka", ""},
		{"digest pin, no tag", "postgres", "postgres@sha256:deadbeefcafe", ""},
		{"dotted patch tag", "mongo", "mongo:7.0.45", "7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("KIRA_COMPAT_IMAGE_"+strings.ToUpper(c.kind), "")
			if got := ServerMajor(c.kind, c.pinned); got != c.want {
				t.Errorf("ServerMajor(%q, %q) = %q, want %q", c.kind, c.pinned, got, c.want)
			}
		})
	}
}

func TestServerMajorFollowsOverride(t *testing.T) {
	t.Setenv("KIRA_COMPAT_IMAGE_POSTGRES", "postgres:14-alpine")
	if got := ServerMajor("postgres", "postgres:17-alpine"); got != "14" {
		t.Errorf("ServerMajor with override = %q, want %q", got, "14")
	}
}

func TestImageForDefaultsWhenUnset(t *testing.T) {
	t.Setenv("KIRA_COMPAT_IMAGE_REDIS", "")
	if got := ImageFor("redis", "redis:7"); got != "redis:7" {
		t.Errorf("ImageFor with unset override = %q, want pinned default", got)
	}
}

func TestImageForOverride(t *testing.T) {
	t.Setenv("KIRA_COMPAT_IMAGE_REDIS", "  redis:8.8  ")
	if got := ImageFor("redis", "redis:7"); got != "redis:8.8" {
		t.Errorf("ImageFor with override = %q, want trimmed override", got)
	}
}
