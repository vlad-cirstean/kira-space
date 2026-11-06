package testsupport

import (
	"os"
	"regexp"
	"strings"
)

// ImageFor returns the container image for kind. An on-demand compatibility run
// (scripts/db-compat.sh) overrides the pinned default through KIRA_COMPAT_IMAGE_<KIND>; with the
// variable unset every caller gets exactly the image it pinned, so `bun run test:go`
// and CI are unchanged.
func ImageFor(kind, pinned string) string {
	v := strings.TrimSpace(os.Getenv("KIRA_COMPAT_IMAGE_" + strings.ToUpper(kind)))
	if v == "" {
		return pinned
	}
	return v
}

// ServerMajor returns the major-version component of the image tag resolved for kind ("17" for
// postgres:17-alpine, "12" for mariadb:12.3, "8" for mongo:8.3), or "" when the tag carries no
// leading number (":latest", a digest pin). Conformance suites build their own ServerVersion
// assertion from it, so running against a different image still asserts the version that was
// asked for instead of asserting nothing. See docs/HISTORY.md for the parsing rule.
func ServerMajor(kind, pinned string) string {
	image := ImageFor(kind, pinned)
	// A "@sha256:…" digest suffix is never the tag — strip it before hunting for a ":tag", so a
	// digest-only pin (no tag at all) correctly falls through to "".
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	_, tag, ok := strings.Cut(lastSegment(image), ":")
	if !ok {
		return ""
	}
	tag, _, _ = strings.Cut(tag, "-")
	end := 0
	for end < len(tag) && tag[end] >= '0' && tag[end] <= '9' {
		end++
	}
	return tag[:end]
}

// lastSegment isolates the "name:tag" portion of an image reference from any registry/namespace
// path segments, so a ':' inside a namespaced name's path (there is none here, but a future
// registry-host:port prefix would have one) is never mistaken for the tag separator.
func lastSegment(image string) string {
	if i := strings.LastIndex(image, "/"); i >= 0 {
		return image[i+1:]
	}
	return image
}

// VersionPattern builds the ^<prefix> <major>\. assertion a conformance suite's own
// ServerVersion check runs against — "^PostgreSQL 17\." for major "17" — or falls back to
// "^<prefix> \d+\." when ServerMajor could not derive one (P16 D4). Every kind whose pin is
// wired through ImageFor uses this so the assertion tracks whichever image the compat run
// actually asked for, instead of a version baked in at the time the suite was written.
func VersionPattern(prefix, major string) *regexp.Regexp {
	digits := `\d+`
	if major != "" {
		digits = regexp.QuoteMeta(major)
	}
	return regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + ` ` + digits + `\.`)
}

// PostgresServerMajor, MysqlServerMajor, MariaServerMajor, ClickHouseServerMajor, MongoServerMajor
// and RedisServerMajor are ServerMajor pre-bound to each kind's own pinned default image, so a
// conformance suite's version assertion never has to duplicate the pinned literal testsupport
// already owns.
func PostgresServerMajor() string   { return ServerMajor("postgres", defaultPostgresImage) }
func MysqlServerMajor() string      { return ServerMajor("mysql", mysqlImage) }
func MariaServerMajor() string      { return ServerMajor("mariadb", mariaImage) }
func ClickHouseServerMajor() string { return ServerMajor("clickhouse", clickhouseImage) }
func MongoServerMajor() string      { return ServerMajor("mongo", MongoImage) }
func RedisServerMajor() string      { return ServerMajor("redis", RedisImage) }

// Images lists every image the fixtures start, resolved through ImageFor. One source for
// scripts/pull-test-images.sh.
func Images() []string {
	return []string{
		ImageFor("postgres", defaultPostgresImage),
		ImageFor("mysql", mysqlImage),
		ImageFor("mariadb", mariaImage),
		ImageFor("clickhouse", clickhouseImage),
		ImageFor("redis", RedisImage),
		ImageFor("mongo", MongoImage),
		ImageFor("kafka", kafkaImage),
		ImageFor("localstack", LocalStackImage),
		ImageFor("elasticsearch", elasticsearchImage),
		ImageFor("kibana", kibanaImage),
	}
}
