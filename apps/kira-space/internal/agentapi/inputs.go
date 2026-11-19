package agentapi

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
)

// Limits on declared inputs.
const (
	maxInputs      = 4
	maxLabelRunes  = 80
	inputsDeclHint = "; to ask the user for it, declare it in inputs"
)

// InputDecl is a value the agent asks the user for: the kept name and the question shown on the card.
type InputDecl struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// InputState is where a declared input stands at plan time.
type InputState string

const (
	// InputNeeded: no live entry; the user must type the value.
	InputNeeded InputState = "needed"
	// InputStored: a live user entry of the same origin holds it.
	InputStored InputState = "stored"
)

// InputUse is a declared input as planned. KeptID is the live entry for a stored input, "" for a needed one.
type InputUse struct {
	Name   string     `json:"name"`
	Label  string     `json:"label"`
	State  InputState `json:"state"`
	KeptID string     `json:"keptId"`
	Origin string     `json:"origin"`
}

// MissingInputs counts the declared inputs without a stored value.
func (p Planned) MissingInputs() int {
	n := 0
	for _, in := range p.Inputs {
		if in.State == InputNeeded {
			n++
		}
	}
	return n
}

// UsesUserValue reports whether the request inserts a value the user supplied.
func (p Planned) UsesUserValue() bool {
	for _, k := range p.Kept {
		if k.Source.Kind == keptvars.SourceUser {
			return true
		}
	}
	return false
}

// OriginOf is scheme://host[:port] with the scheme and host lowercase and a default port dropped: the key a
// user value is bound to. "" when u carries no host.
func OriginOf(u *url.URL) string {
	if u == nil || u.Hostname() == "" {
		return ""
	}
	scheme, host, port := strings.ToLower(u.Scheme), strings.ToLower(u.Hostname()), u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host
}

func originOfText(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return OriginOf(u)
}

// checkDecls validates the declarations and returns them by name.
func checkDecls(decls []InputDecl) (map[string]InputDecl, error) {
	if len(decls) > maxInputs {
		return nil, fmt.Errorf("a request can declare at most %d inputs", maxInputs)
	}
	byName := make(map[string]InputDecl, len(decls))
	for _, d := range decls {
		if err := keptvars.ValidName(d.Name); err != nil {
			return nil, err
		}
		if _, dup := byName[d.Name]; dup {
			return nil, fmt.Errorf("input %q is declared twice", d.Name)
		}
		label := strings.TrimSpace(d.Label)
		if n := utf8.RuneCountInString(label); n < 1 || n > maxLabelRunes || strings.IndexFunc(label, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("input %q needs a label of 1 to %d characters on one line", d.Name, maxLabelRunes)
		}
		byName[d.Name] = InputDecl{Name: d.Name, Label: label}
	}
	return byName, nil
}

// judgeInput decides one declared input against the live entry, if any.
func judgeInput(d InputDecl, e keptvars.Entry, live bool, origin string) (InputUse, error) {
	use := InputUse{Name: d.Name, Label: d.Label, State: InputNeeded, Origin: origin}
	if !live {
		return use, nil
	}
	if e.Source.Kind != keptvars.SourceUser {
		return use, fmt.Errorf("%q is already a kept variable made by a script; choose another input name", d.Name)
	}
	if e.Origin != origin {
		return use, otherOriginError(d.Name, e.Origin, origin)
	}
	use.State, use.KeptID = InputStored, e.ID
	return use, nil
}

func otherOriginError(name, supplied, now string) error {
	return fmt.Errorf("the value kept as %q was supplied for %s; choose another input name for %s", name, supplied, now)
}

// checkRoom refuses a request whose missing inputs would not fit in the scope.
func (s *Service) checkRoom(scope string, missing int) error {
	if missing == 0 || s.Kept == nil || scope == "" {
		return nil
	}
	if len(s.Kept.List(scope))+missing > keptvars.MaxScopeEntries {
		return keptvars.ErrScopeFull
	}
	return nil
}

var errInputMissing = errors.New("an input has no value yet; nothing was sent")

// pendingEntry stands in for the value a needed input will get: a secret user string.
func pendingEntry(name, origin string) keptvars.Entry {
	return keptvars.Entry{Meta: keptvars.Meta{
		Name: name, Type: "string", Secret: true, Origin: origin,
		Source: keptvars.Source{Kind: keptvars.SourceUser},
	}}
}
