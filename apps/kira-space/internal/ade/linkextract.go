package ade

import (
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// linkextract.go pulls http(s) URLs out of note and handover text. Pure: no fetch, no DNS.

// foundURL is one extracted URL. Title is the markdown link text, "" for a bare URL.
type foundURL struct {
	URL, Title string
}

var (
	mdLinkRe   = regexp.MustCompile(`!?\[([^\]]*)\]\(\s*<?([^()\s<>]*(?:\([^()\s]*\)[^()\s<>]*)*)>?(?:\s+"[^"]*")?\s*\)`)
	autoLinkRe = regexp.MustCompile(`<(https?://[^<>\s]+)>`)
	bareURLRe  = regexp.MustCompile("(?i)https?://[^\\s<>\"`]+")
	codeSpanRe = regexp.MustCompile("`[^`\n]*`")
)

const linkTrimChars = ".,;:!?'\"*_]"

// extractLinks returns the body's URLs in reading order, deduped by exact string. Fenced blocks and
// inline code are skipped: URLs there are examples and commands.
func extractLinks(body string) []foundURL {
	var out []foundURL
	seen := map[string]bool{}
	add := func(raw, title string) {
		raw = trimLinkURL(raw)
		if !acceptFoundURL(raw) || seen[raw] {
			return
		}
		seen[raw] = true
		out = append(out, foundURL{URL: raw, Title: strings.TrimSpace(title)})
	}
	for _, seg := range proseOutsideFences(body) {
		seg = codeSpanRe.ReplaceAllString(seg, " ")
		// Markdown links and autolinks first, blanking their span so the bare scan skips them.
		type hit struct {
			pos        int
			url, title string
		}
		var hits []hit
		blank := []byte(seg)
		for _, m := range mdLinkRe.FindAllStringSubmatchIndex(seg, -1) {
			hits = append(hits, hit{m[0], seg[m[4]:m[5]], seg[m[2]:m[3]]})
			blankRange(blank, m[0], m[1])
		}
		for _, m := range autoLinkRe.FindAllStringSubmatchIndex(string(blank), -1) {
			hits = append(hits, hit{m[0], string(blank)[m[2]:m[3]], ""})
			blankRange(blank, m[0], m[1])
		}
		for _, m := range bareURLRe.FindAllStringIndex(string(blank), -1) {
			hits = append(hits, hit{m[0], string(blank)[m[0]:m[1]], ""})
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })
		for _, h := range hits {
			add(h.url, h.title)
		}
	}
	return out
}

func blankRange(b []byte, from, to int) {
	for i := from; i < to; i++ {
		b[i] = ' '
	}
}

// proseOutsideFences splits body at ``` and ~~~ fence lines and returns the text between fences.
func proseOutsideFences(body string) []string {
	var segs []string
	var cur strings.Builder
	fence := ""
	for _, line := range strings.SplitAfter(body, "\n") {
		t := strings.TrimLeft(line, " \t")
		switch {
		case fence != "":
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
		case strings.HasPrefix(t, "```"), strings.HasPrefix(t, "~~~"):
			fence = t[:3]
			segs = append(segs, cur.String())
			cur.Reset()
		default:
			cur.WriteString(line)
		}
	}
	return append(segs, cur.String())
}

// trimLinkURL drops sentence punctuation after a URL. A trailing ")" goes only while unbalanced.
func trimLinkURL(s string) string {
	for {
		t := strings.TrimRight(s, linkTrimChars)
		if strings.HasSuffix(t, ")") && strings.Count(t, ")") > strings.Count(t, "(") {
			t = t[:len(t)-1]
		}
		if t == s {
			return s
		}
		s = t
	}
}

// acceptFoundURL keeps http(s) URLs with a host, no credentials, no loopback, within the length cap.
func acceptFoundURL(raw string) bool {
	if raw == "" || len(raw) > agentmcp.MaxArtifactURL {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return false
	}
	return true
}
