package main

import (
	"regexp"
	"strings"
)

const notWord = `[^A-Za-z0-9_]`

var (
	emphasisUnderscore2 = regexp.MustCompile(`(?:^|` + notWord + `)__[^_\s](?:[^_]*[^_\s])?__(?:` + notWord + `|$)`)
	emphasisStar        = regexp.MustCompile(`(?:^|[^A-Za-z0-9_*])\*[^*\s](?:[^*]*[^*\s])?\*(?:[^A-Za-z0-9_*]|$)`)
	emphasisUnderscore  = regexp.MustCompile(`(?:^|` + notWord + `)_[^_\s](?:[^_]*[^_\s])?_(?:` + notWord + `|$)`)
	linkRe              = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	tagRe               = regexp.MustCompile(`<[A-Za-z/!][^>]*>`)
)

// stripMarkup removes inline Markdown pairs from s, keeping their content, and collapses
// whitespace runs. It returns s unchanged when it has no markup.
func stripMarkup(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	for i := 0; i < 4; i++ {
		before := s
		s = strings.NewReplacer("**", "", "~~", "", "`", "").Replace(s)
		s = stripEdge(s, emphasisUnderscore2, "__")
		s = stripEdge(s, emphasisStar, "*")
		s = stripEdge(s, emphasisUnderscore, "_")
		s = linkRe.ReplaceAllString(s, "$1")
		s = tagRe.ReplaceAllString(s, "")
		if s == before {
			break
		}
	}
	return strings.Join(strings.Fields(s), " ")
}

// stripEdge drops the delimiter d from the first and last position of each match of re.
func stripEdge(s string, re *regexp.Regexp, d string) string {
	return re.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.Index(m, d)
		j := strings.LastIndex(m, d)
		if i < 0 || j <= i {
			return m
		}
		return m[:i] + m[i+len(d):j] + m[j+len(d):]
	})
}
