package ade

import (
	"net/url"
	"regexp"
	"strings"
)

// JiraRef names one Jira issue found in pasted text.
type JiraRef struct {
	Key  string // PROJ-12
	URL  string // the pasted URL; "" for a bare key
	Site string // lowercase host of a pasted URL; "" for a bare key
}

const jiraKeyPattern = `[A-Z][A-Z0-9]+-\d+`

var (
	jiraKeyRe     = regexp.MustCompile(`^` + jiraKeyPattern + `$`)
	jiraPathKeyRe = regexp.MustCompile(`/(?:browse|issues)/(` + jiraKeyPattern + `)(?:[/?#]|$)`)
)

// ParseJiraRef accepts a bare key (PROJ-12), /browse/KEY, /jira/.../issues/KEY and any URL with
// selectedIssue=KEY. It does not check the host.
func ParseJiraRef(raw string) (JiraRef, bool) {
	raw = strings.TrimSpace(raw)
	if jiraKeyRe.MatchString(raw) {
		return JiraRef{Key: raw}, true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return JiraRef{}, false
	}
	key := u.Query().Get("selectedIssue")
	if !jiraKeyRe.MatchString(key) {
		m := jiraPathKeyRe.FindStringSubmatch(u.Path)
		if m == nil {
			return JiraRef{}, false
		}
		key = m[1]
	}
	return JiraRef{Key: key, URL: raw, Site: strings.ToLower(u.Host)}, true
}
