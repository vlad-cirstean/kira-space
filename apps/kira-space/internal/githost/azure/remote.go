package azure

import (
	"net/url"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

// Host is the one host every Azure Repos repository normalizes to.
const Host = "dev.azure.com"

const legacySuffix = ".visualstudio.com"

// ParseRemote reads the git remote forms Azure DevOps hands out:
//   - https://[org@]dev.azure.com/{org}/{project}/_git/{repo}
//   - https://{org}.visualstudio.com/[{collection}/]{project}/_git/{repo}
//   - {user}@ssh.dev.azure.com:v3/{org}/{project}/{repo}
//   - {org}@vs-ssh.visualstudio.com:v3/{org}/{project}/{repo}
//   - ssh://[user@]host[:22]/v3/{org}/{project}/{repo} and the legacy .../{project}/_ssh/{repo}
//
// A remote without a project segment is rejected rather than guessed at.
func ParseRemote(raw string) (githost.Repo, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \\") || hasControl(raw) {
		return githost.Repo{}, false
	}
	switch {
	case strings.HasPrefix(raw, "https://"):
		return parseHTTPS(raw)
	case strings.HasPrefix(raw, "ssh://"):
		return parseSSHURL(strings.TrimPrefix(raw, "ssh://"))
	case strings.Contains(raw, "://"):
		return githost.Repo{}, false
	default:
		return parseSCP(raw)
	}
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// unescapeSegments splits path on "/" and unescapes each segment. Empty, control-bearing and
// slash-bearing segments fail.
func unescapeSegments(path string) ([]string, bool) {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		d, err := url.PathUnescape(p)
		if err != nil || d == "" || strings.Contains(d, "/") || hasControl(d) {
			return nil, false
		}
		parts[i] = d
	}
	return parts, true
}

func repoOf(org, project, name string) (githost.Repo, bool) {
	name = strings.TrimSuffix(name, ".git")
	if org == "" || project == "" || name == "" {
		return githost.Repo{}, false
	}
	return githost.Repo{Host: Host, Owner: org + "/" + project, Name: name}, true
}

// legacyOrg is the organization of a {org}.visualstudio.com host.
func legacyOrg(host string) (string, bool) {
	host = strings.ToLower(host)
	org := strings.TrimSuffix(host, legacySuffix)
	if org == host || org == "" || strings.Contains(org, ".") {
		return "", false
	}
	return org, true
}

func parseHTTPS(raw string) (githost.Repo, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return githost.Repo{}, false
	}
	if u.Port() != "" && u.Port() != "443" {
		return githost.Repo{}, false
	}
	segs, ok := unescapeSegments(u.EscapedPath())
	if !ok {
		return githost.Repo{}, false
	}
	host := strings.ToLower(u.Hostname())
	if host == Host {
		if len(segs) != 4 || segs[2] != "_git" {
			return githost.Repo{}, false
		}
		if u.User != nil && !strings.EqualFold(u.User.Username(), segs[0]) {
			return githost.Repo{}, false
		}
		return repoOf(segs[0], segs[1], segs[3])
	}
	org, ok := legacyOrg(host)
	if !ok {
		return githost.Repo{}, false
	}
	switch {
	case len(segs) == 3 && segs[1] == "_git":
		return repoOf(org, segs[0], segs[2])
	case len(segs) == 4 && segs[2] == "_git": // {collection}/{project}/_git/{repo}
		return repoOf(org, segs[1], segs[3])
	}
	return githost.Repo{}, false
}

// parseSCP reads {user}@host:v3/{org}/{project}/{repo}.
func parseSCP(raw string) (githost.Repo, bool) {
	at := strings.LastIndexByte(raw, '@')
	if at < 0 {
		return githost.Repo{}, false
	}
	rest := raw[at+1:]
	host, path, ok := strings.Cut(rest, ":")
	if !ok || !sshHost(host) {
		return githost.Repo{}, false
	}
	return sshV3Path(path)
}

func sshHost(host string) bool {
	host = strings.ToLower(host)
	return host == "ssh.dev.azure.com" || host == "vs-ssh.visualstudio.com"
}

func sshV3Path(path string) (githost.Repo, bool) {
	segs, ok := unescapeSegments(path)
	if !ok || len(segs) != 4 || segs[0] != "v3" {
		return githost.Repo{}, false
	}
	return repoOf(segs[1], segs[2], segs[3])
}

// parseSSHURL reads [user@]host[:22]/v3/{org}/{project}/{repo} or the legacy
// [user@]{org}.visualstudio.com[:22]/[{collection}/]{project}/_ssh/{repo}.
func parseSSHURL(rest string) (githost.Repo, bool) {
	authority, path, ok := strings.Cut(rest, "/")
	if !ok {
		return githost.Repo{}, false
	}
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		authority = authority[at+1:]
	}
	host, port, hasPort := strings.Cut(authority, ":")
	if hasPort && port != "22" {
		return githost.Repo{}, false
	}
	if sshHost(host) {
		return sshV3Path(path)
	}
	org, ok := legacyOrg(host)
	if !ok {
		return githost.Repo{}, false
	}
	segs, ok := unescapeSegments(path)
	if !ok {
		return githost.Repo{}, false
	}
	switch {
	case len(segs) == 3 && segs[1] == "_ssh":
		return repoOf(org, segs[0], segs[2])
	case len(segs) == 4 && segs[2] == "_ssh":
		return repoOf(org, segs[1], segs[3])
	}
	return githost.Repo{}, false
}

// split reads r.Owner as "<org>/<project>".
func split(r githost.Repo) (org, project string, ok bool) {
	org, project, ok = strings.Cut(r.Owner, "/")
	return org, project, ok && org != "" && project != "" && r.Name != ""
}

// orgURL is the organization URL `az --org` takes.
func orgURL(org string) string { return "https://" + Host + "/" + url.PathEscape(org) }

// normalizeOrgURL canonicalizes an organization URL from `az devops configure`.
func normalizeOrgURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == Host {
		segs, ok := unescapeSegments(u.EscapedPath())
		if !ok || len(segs) != 1 {
			return "", false
		}
		return orgURL(segs[0]), true
	}
	if org, ok := legacyOrg(host); ok {
		return orgURL(org), true
	}
	return "", false
}
