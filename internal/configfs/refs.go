package configfs

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Ref is one reference inside a string: ${NAME} reads an environment variable, ${secret:name} an app
// secret. Start and End are byte offsets of the whole ${...} in the text, End exclusive. $${ is a
// literal ${ and yields no Ref.
type Ref struct {
	Kind       string // "env" | "secret"
	Name       string
	Start, End int
}

const (
	RefEnv    = "env"
	RefSecret = "secret"
)

const refSyntax = "${NAME} or ${secret:name}"

var (
	envNameRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	secretNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)
)

// Refs parses the references in text. Anything malformed inside ${...} is a Problem (no line: the
// caller adds it) naming the byte column, never the content.
func Refs(text string) ([]Ref, *Problem) {
	var out []Ref
	for i := 0; i < len(text); {
		if text[i] != '$' {
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "$${") {
			i += 3
			continue
		}
		if !strings.HasPrefix(text[i:], "${") {
			i++
			continue
		}
		rel := strings.IndexAny(text[i+2:], "}$")
		if rel < 0 || text[i+2+rel] == '$' {
			return nil, badRef(i)
		}
		end := i + 2 + rel + 1
		body := text[i+2 : end-1]
		switch {
		case strings.HasPrefix(body, "secret:") && secretNameRe.MatchString(body[len("secret:"):]):
			out = append(out, Ref{Kind: RefSecret, Name: body[len("secret:"):], Start: i, End: end})
		case envNameRe.MatchString(body):
			out = append(out, Ref{Kind: RefEnv, Name: body, Start: i, End: end})
		default:
			return nil, badRef(i)
		}
		i = end
	}
	return out, nil
}

func badRef(at int) *Problem {
	return &Problem{Message: i18n.T("backend.core.config.refInvalid.body", map[string]string{"column": strconv.Itoa(at + 1), "syntax": refSyntax})}
}

// wholeSecret reports whether text is exactly one secret reference.
func wholeSecret(text string) bool {
	refs, p := Refs(text)
	return p == nil && len(refs) == 1 && refs[0].Kind == RefSecret && refs[0].Start == 0 && refs[0].End == len(text)
}

// Unescape turns $${ into ${ in the literal parts of a resolved string.
func unescape(s string) string { return strings.ReplaceAll(s, "$${", "${") }

const secretSyntax = "${secret:name}"
