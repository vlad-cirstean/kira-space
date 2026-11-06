package apifiles

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

// HostRule is one allowed-host entry of an agent-available secret: host, host:port or *.domain, with an
// optional http:// prefix that allows plain HTTP.
type HostRule struct {
	// Host is the lowercased ASCII (IDNA) hostname, without the wildcard label.
	Host     string
	Wildcard bool
	// Port 0 means any port.
	Port int
	// HTTP allows the plain-HTTP scheme for a non-loopback host.
	HTTP bool
}

var errHostRule = errors.New("not a host, host:port or *.domain entry")

// ParseHostRule reads one entry. It refuses a path, query, userinfo or empty host.
func ParseHostRule(s string) (HostRule, error) {
	var r HostRule
	s = strings.ToLower(strings.TrimSpace(s))
	if rest, ok := strings.CutPrefix(s, "http://"); ok {
		r.HTTP, s = true, rest
	}
	if rest, ok := strings.CutPrefix(s, "*."); ok {
		r.Wildcard, s = true, rest
	}
	if s == "" || strings.ContainsAny(s, "/?#@\\ \t*") {
		return HostRule{}, errHostRule
	}
	host, port, err := splitHostPort(s)
	if err != nil {
		return HostRule{}, err
	}
	r.Port = port
	if r.Host, err = normalizeHost(host); err != nil {
		return HostRule{}, err
	}
	if r.Wildcard {
		if _, err := netip.ParseAddr(r.Host); err == nil {
			return HostRule{}, errHostRule
		}
	}
	return r, nil
}

// splitHostPort splits s into host and port (0 when absent); an IPv6 literal is bracketed.
func splitHostPort(s string) (string, int, error) {
	host, portText := s, ""
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 {
			return "", 0, errHostRule
		}
		host = s[1:end]
		switch rest := s[end+1:]; {
		case rest == "":
		case strings.HasPrefix(rest, ":"):
			portText = rest[1:]
		default:
			return "", 0, errHostRule
		}
	} else if i := strings.LastIndex(s, ":"); i >= 0 {
		host, portText = s[:i], s[i+1:]
	}
	if host == "" || (!strings.HasPrefix(s, "[") && strings.Contains(host, ":")) {
		return "", 0, errHostRule
	}
	if portText == "" {
		if strings.HasSuffix(s, ":") {
			return "", 0, errHostRule
		}
		return host, 0, nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, errHostRule
	}
	return host, port, nil
}

// normalizeHost lowercases and IDNA-encodes a hostname; an IP literal passes through in canonical form.
func normalizeHost(h string) (string, error) {
	if addr, err := netip.ParseAddr(h); err == nil {
		return addr.String(), nil
	}
	ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(h, "."))
	if err != nil || ascii == "" {
		return "", errHostRule
	}
	return ascii, nil
}

// String is the entry as written back.
func (r HostRule) String() string {
	var b strings.Builder
	if r.HTTP {
		b.WriteString("http://")
	}
	if r.Wildcard {
		b.WriteString("*.")
	}
	if strings.Contains(r.Host, ":") {
		b.WriteString("[" + r.Host + "]")
	} else {
		b.WriteString(r.Host)
	}
	if r.Port != 0 {
		b.WriteString(":" + strconv.Itoa(r.Port))
	}
	return b.String()
}

// Allows reports whether u is a place the entry permits: scheme, host and port.
func (r HostRule) Allows(u *url.URL) bool {
	host, err := normalizeHost(strings.ToLower(u.Hostname()))
	if err != nil || !r.hostMatches(host) {
		return false
	}
	if r.Port != 0 && r.Port != effectivePort(u) {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return true
	case "http":
		return r.HTTP || isLoopback(host)
	}
	return false
}

func (r HostRule) hostMatches(host string) bool {
	if !r.Wildcard {
		return host == r.Host
	}
	return len(host) > len(r.Host)+1 && strings.HasSuffix(host, "."+r.Host)
}

func effectivePort(u *url.URL) int {
	if p := u.Port(); p != "" {
		n, _ := strconv.Atoi(p)
		return n
	}
	if strings.EqualFold(u.Scheme, "http") {
		return 80
	}
	return 443
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}
