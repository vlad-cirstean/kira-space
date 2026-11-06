package configsync

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	scpLike   = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[^\s]+$`)
	branchRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)
	errBadURL = errors.New("configsync: repository URL not allowed")
)

// validRepoURL accepts https, ssh, scp-like and file URLs, and refuses userinfo that could hold a
// credential: any on https and file, a password on ssh. git reads credentials from its own helper.
func validRepoURL(raw string) error {
	if raw == "" || strings.ContainsAny(raw, " \t\r\n\x00") || strings.HasPrefix(raw, "-") {
		return errBadURL
	}
	if !strings.Contains(raw, "://") {
		if scpLike.MatchString(raw) {
			return nil
		}
		return errBadURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errBadURL
	}
	switch u.Scheme {
	case "https":
		if u.User != nil || u.Host == "" {
			return errBadURL
		}
	case "ssh":
		if u.Host == "" {
			return errBadURL
		}
		if u.User != nil {
			if _, hasPassword := u.User.Password(); hasPassword {
				return errBadURL
			}
		}
	case "file":
		if u.User != nil || !strings.HasPrefix(u.Path, "/") {
			return errBadURL
		}
	default:
		return errBadURL
	}
	return nil
}

// validBranch accepts a plain branch name; a stricter set than git's, so a name never reads as an option.
func validBranch(b string) bool {
	return branchRe.MatchString(b) && !strings.Contains(b, "..") && !strings.HasSuffix(b, "/") &&
		!strings.HasSuffix(b, ".") && !strings.HasSuffix(b, ".lock") && !strings.Contains(b, "//")
}
