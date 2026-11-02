package agentport

import (
	"regexp"
	"strings"
)

// loc is where a string sits in a native config; it decides which credential rules apply.
type loc int

const (
	locEnv     loc = iota // settings env or MCP env value; name is the variable
	locHeader             // MCP header value; name is the header
	locURL                // MCP url
	locCommand            // MCP command
	locArg                // MCP arg; name is the previous arg
)

// part is a piece of a classified value: literal text, or a credential to seal.
type part struct {
	text   string
	secret bool
}

var (
	// safeHeaders carry no credential.
	safeHeaders = map[string]bool{"accept": true, "content-type": true, "user-agent": true, "mcp-protocol-version": true}

	secretNameParts = []string{"token", "secret", "passw", "pwd", "apikey", "api_key", "auth", "credential", "bearer", "private", "cookie"}
	secretNameEnds  = []string{"_pat", "_key"}

	valuePrefixes = []string{"sk-", "ghp_", "gho_", "github_pat_", "glpat-", "AKIA", "AIza"}
	slackToken    = regexp.MustCompile(`^xox[abp]-`)
	jwt           = regexp.MustCompile(`^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*$`)
	hexValue      = regexp.MustCompile(`^[0-9a-fA-F]{32,}$`)
	b64Value      = regexp.MustCompile(`^[A-Za-z0-9_-]{32,}$`)
	schemes       = []string{"Bearer ", "Basic ", "token "}
)

// nameIsSecret reports whether a variable, header-less flag or query parameter name marks a credential.
func nameIsSecret(name string) bool {
	n := strings.ReplaceAll(strings.ToLower(name), "-", "_")
	for _, p := range secretNameParts {
		if strings.Contains(n, p) {
			return true
		}
	}
	for _, e := range secretNameEnds {
		if strings.HasSuffix(n, e) {
			return true
		}
	}
	return false
}

// valueIsSecret reports whether v looks like a credential on its own: a known prefix, a JWT, or a long
// hex or mixed-case base64url run.
func valueIsSecret(v string) bool {
	for _, p := range valuePrefixes {
		if strings.HasPrefix(v, p) && len(v) > len(p) {
			return true
		}
	}
	if slackToken.MatchString(v) || jwt.MatchString(v) || hexValue.MatchString(v) {
		return true
	}
	return b64Value.MatchString(v) && strings.ContainsAny(v, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") &&
		strings.ContainsAny(v, "abcdefghijklmnopqrstuvwxyz") && strings.ContainsAny(v, "0123456789")
}

// classify splits value into literal and credential parts. A value with a reference, an empty value
// and a value with no credential come back as one literal part.
func classify(l loc, name, value string) []part {
	plain := []part{{text: value}}
	if value == "" || strings.Contains(value, "${") || strings.Contains(value, "{env:") {
		return plain
	}
	switch l {
	case locEnv:
		if nameIsSecret(name) || valueIsSecret(value) {
			return split(value)
		}
		return urlValue(value)
	case locHeader:
		if !safeHeaders[strings.ToLower(name)] {
			return split(value)
		}
	case locURL:
		return urlParts(value)
	case locCommand:
		if valueIsSecret(value) {
			return []part{{text: value, secret: true}}
		}
	case locArg:
		return argParts(name, value)
	}
	return plain
}

// split keeps a scheme prefix (Bearer, Basic, token) as literal text and seals the rest.
func split(value string) []part {
	for _, s := range schemes {
		if rest, ok := strings.CutPrefix(value, s); ok && rest != "" {
			return []part{{text: s}, {text: rest, secret: true}}
		}
	}
	return []part{{text: value, secret: true}}
}

// argParts classifies one argument: --flag=value or a value after a secret-named flag, else by pattern.
func argParts(prev, v string) []part {
	if strings.HasPrefix(v, "-") {
		if flag, val, ok := strings.Cut(v, "="); ok && val != "" && nameIsSecret(strings.TrimLeft(flag, "-")) {
			return append([]part{{text: flag + "="}}, split(val)...)
		}
		return []part{{text: v}}
	}
	if strings.HasPrefix(prev, "-") && !strings.Contains(prev, "=") && nameIsSecret(strings.TrimLeft(prev, "-")) {
		return split(v)
	}
	if valueIsSecret(v) {
		return []part{{text: v, secret: true}}
	}
	return urlValue(v)
}

// urlValue classifies a value that is a URL (a connection string in an env var or an argument).
func urlValue(v string) []part {
	if strings.Contains(v, "://") {
		return urlParts(v)
	}
	return []part{{text: v}}
}

// urlParts seals userinfo, a credential-looking path segment, and the value of a query parameter
// named like a credential, named key, or holding a credential-looking value.
func urlParts(u string) []part {
	var out []part
	rest := u
	if i := strings.Index(rest, "://"); i >= 0 {
		out = append(out, part{text: rest[:i+3]})
		rest = rest[i+3:]
		end := strings.IndexAny(rest, "/?#")
		if end < 0 {
			end = len(rest)
		}
		if at := strings.LastIndex(rest[:end], "@"); at > 0 {
			out = append(out, part{text: rest[:at], secret: true}, part{text: "@"})
			rest = rest[at+1:]
		}
	}
	base, query, hasQuery := strings.Cut(rest, "?")
	segs := strings.Split(base, "/")
	for i, seg := range segs {
		if i > 0 {
			out = append(out, part{text: "/"})
		}
		// The first segment is the host when a scheme came first.
		if i > 0 && valueIsSecret(seg) {
			out = append(out, part{text: seg, secret: true})
			continue
		}
		out = append(out, part{text: seg})
	}
	if hasQuery {
		out = append(out, part{text: "?"})
		for i, kv := range strings.Split(query, "&") {
			if i > 0 {
				out = append(out, part{text: "&"})
			}
			k, v, ok := strings.Cut(kv, "=")
			if ok && v != "" && (nameIsSecret(k) || strings.EqualFold(k, "key") || valueIsSecret(v)) {
				out = append(out, part{text: k + "="}, part{text: v, secret: true})
				continue
			}
			out = append(out, part{text: kv})
		}
	}
	return mergeLiterals(out)
}

func mergeLiterals(in []part) []part {
	var out []part
	for _, p := range in {
		if n := len(out); n > 0 && !p.secret && !out[n-1].secret {
			out[n-1].text += p.text
			continue
		}
		if p.text != "" {
			out = append(out, p)
		}
	}
	return out
}

// hasSecret reports whether any part is a credential.
func hasSecret(ps []part) bool {
	for _, p := range ps {
		if p.secret {
			return true
		}
	}
	return false
}
