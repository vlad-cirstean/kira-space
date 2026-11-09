// Package gitremote normalizes git remote URLs into a stable repository key.
package gitremote

import (
	"net/url"
	"strings"
	"unicode"
)

// Key turns a git remote URL into a project key: lowercase host/path, with no scheme,
// user, password, port or .git suffix. ok is false for anything that is not a host plus a path.
func Key(raw string) (key string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return "", false
	}
	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "file" {
			return "", false
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp form: [user@]host:path. A leading slash, dot or tilde means a filesystem path.
		if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, ".") || strings.HasPrefix(raw, "~") {
			return "", false
		}
		colon := strings.IndexByte(raw, ':')
		if colon < 1 {
			return "", false
		}
		host, path = raw[:colon], raw[colon+1:]
		if at := strings.LastIndexByte(host, '@'); at >= 0 {
			host = host[at+1:]
		}
	}
	host = strings.ToLower(host)
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	path = strings.Trim(strings.ToLower(path), "/")
	if host == "" || path == "" || strings.ContainsAny(host, "/@:") {
		return "", false
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", false
		}
	}
	return host + "/" + path, true
}
