package apifiles

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// CollectionExt and EnvironmentExt are the file suffixes of the two kinds.
	CollectionExt  = ".collection.yaml"
	EnvironmentExt = ".environment.yaml"
	maxStem        = 64
	maxSecretName  = 64
)

var (
	stemRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	secretNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)
	nonSlug      = regexp.MustCompile(`[^a-z0-9]+`)
)

// Slug lowercases s into a stem: letters and digits, single dashes, at most maxStem characters. fallback
// stands in for a name with no usable character.
func Slug(s, fallback string) string {
	out := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(out) > maxStem {
		out = strings.Trim(out[:maxStem], "-")
	}
	if out == "" {
		return fallback
	}
	return out
}

// freeName returns base, or base-2, base-3 ... the first that taken does not report, within maxStem.
func freeName(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("-%d", n)
		head := base
		if len(head)+len(suffix) > maxStem {
			head = strings.Trim(head[:maxStem-len(suffix)], "-")
		}
		if cand := head + suffix; !taken(cand) {
			return cand
		}
	}
}

// StemOf strips ext from file and reports whether the rest is a valid stem.
func StemOf(file, ext string) (string, bool) {
	stem, ok := strings.CutSuffix(file, ext)
	return stem, ok && stemRe.MatchString(stem)
}

// SecretName builds a secret name from an owner stem and a variable name, within the name pattern.
func SecretName(owner, variable string) string {
	v := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(variable), "-"), "-")
	if v == "" {
		v = "secret"
	}
	name := owner + "." + v
	if len(name) > maxSecretName {
		name = strings.TrimRight(name[:maxSecretName], "-.")
	}
	return name
}

func validSecretName(name string) bool { return secretNameRe.MatchString(name) }
