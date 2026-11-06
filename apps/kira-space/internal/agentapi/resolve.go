package agentapi

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"golang.org/x/net/http/httpguts"
)

// Limits checked before any approval is raised.
const (
	maxURLBytes     = 8 << 10
	maxHeaders      = 50
	maxHeaderBytes  = 8 << 10
	maxBodyBytes    = 1 << 20
	maxFormFields   = 200
	maxQueryFields  = 200
	defaultTimeout  = 30
	maxTimeout      = 300
	defaultResponse = 64 << 10
	maxResponse     = 512 << 10
)

var methods = map[string]bool{"GET": true, "HEAD": true, "OPTIONS": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// credential is a secret variable a request references, with the rules it must satisfy.
type credential struct {
	name       string
	secretName string
	rules      []apifiles.HostRule
}

// Plan checks in against the policy without sending it. A problem is returned as an error the
// agent can act on; it never carries a secret value.
func (s *Service) Plan(in Input) (Planned, error) {
	env, avail, err := s.environmentOf(in.EnvironmentID)
	if err != nil {
		return Planned{}, err
	}
	norm, err := normalize(in)
	if err != nil {
		return Planned{}, err
	}
	if err := checkNames(norm, env, avail); err != nil {
		return Planned{}, err
	}
	resolved, used := resolvePlain(norm, avail)
	p, err := buildPlanned(env, resolved, avail, used)
	if err != nil {
		return Planned{}, err
	}
	return p, checkCredentials(p)
}

func (s *Service) environmentOf(id string) (EnvInfo, map[string]available, error) {
	if id == "" {
		return EnvInfo{}, map[string]available{}, nil
	}
	ref, err := s.Envs.EnvironmentByID(id)
	if err != nil {
		return EnvInfo{}, nil, fmt.Errorf("no environment %q; call api_list_environments for the ids", id)
	}
	return envInfoOf(ref), availableOf(ref), nil
}

func (in Input) clone() Input {
	in.Headers = append([]NameValue(nil), in.Headers...)
	in.Query = append([]NameValue(nil), in.Query...)
	in.Form = append([]NameValue(nil), in.Form...)
	return in
}

// texts calls fn on every string of the request that may carry a {{name}} reference.
func (in *Input) texts(fn func(*string)) {
	fn(&in.URL)
	fn(&in.Body)
	fn(&in.ContentType)
	for _, list := range [][]NameValue{in.Headers, in.Query, in.Form} {
		for i := range list {
			fn(&list[i].Name)
			fn(&list[i].Value)
		}
	}
}

// normalize validates the request shape and limits and fills the defaults.
func normalize(in Input) (Input, error) {
	in = in.clone()
	in.Method = strings.ToUpper(strings.TrimSpace(in.Method))
	in.URL = strings.TrimSpace(in.URL)
	if !methods[in.Method] {
		return in, errors.New("method must be GET, HEAD, OPTIONS, POST, PUT, PATCH or DELETE")
	}
	if in.URL == "" {
		return in, errors.New("url is required")
	}
	if err := checkSizes(in); err != nil {
		return in, err
	}
	if err := checkBodyArgs(in); err != nil {
		return in, err
	}
	in.TimeoutSeconds = clampOr(in.TimeoutSeconds, 1, maxTimeout, defaultTimeout)
	in.MaxResponseBytes = clampOr(in.MaxResponseBytes, 1, maxResponse, defaultResponse)
	return in, checkHeaders(in.Headers)
}

func clampOr(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	return min(max(v, lo), hi)
}

func checkSizes(in Input) error {
	switch {
	case len(in.URL) > maxURLBytes:
		return fmt.Errorf("url is longer than %d bytes", maxURLBytes)
	case len(in.Headers) > maxHeaders:
		return fmt.Errorf("more than %d headers", maxHeaders)
	case len(in.Body) > maxBodyBytes:
		return fmt.Errorf("body is larger than %d bytes", maxBodyBytes)
	case len(in.Form) > maxFormFields:
		return fmt.Errorf("more than %d form fields", maxFormFields)
	case len(in.Query) > maxQueryFields:
		return fmt.Errorf("more than %d query entries", maxQueryFields)
	}
	for _, h := range in.Headers {
		if len(h.Value) > maxHeaderBytes {
			return fmt.Errorf("header %q is longer than %d bytes", h.Name, maxHeaderBytes)
		}
	}
	return nil
}

func checkBodyArgs(in Input) error {
	switch {
	case in.Body != "" && len(in.Form) > 0:
		return errors.New("send body or form, not both")
	case in.ContentType != "" && in.Body == "":
		return errors.New("contentType applies to body only")
	case in.ContentType != "" && hasHeader(in.Headers, "Content-Type"):
		return errors.New("set Content-Type once: as contentType or as a header")
	}
	return nil
}

func hasHeader(list []NameValue, name string) bool {
	for _, h := range list {
		if strings.EqualFold(h.Name, name) {
			return true
		}
	}
	return false
}

func checkHeaders(list []NameValue) error {
	for _, h := range list {
		switch {
		case !httpguts.ValidHeaderFieldName(h.Name):
			return fmt.Errorf("header name %q is not valid", h.Name)
		case strings.EqualFold(h.Name, "Host"):
			return errors.New("the Host header is set from the url and cannot be overridden")
		case strings.ContainsAny(h.Value, "\r\n"):
			return fmt.Errorf("header %q has a line break in its value", h.Name)
		}
	}
	return nil
}

// checkNames refuses any reference the agent may not use. A name that does not exist and one that is
// not opted in read the same, so a probe learns nothing.
func checkNames(in Input, env EnvInfo, avail map[string]available) error {
	var err error
	in.texts(func(s *string) {
		for _, n := range apivars.Names(*s) {
			switch {
			case err != nil:
			case apivars.IsDynamicReference(n):
				err = errors.New("dynamic values are not supported here; send the literal value")
			default:
				if _, ok := avail[n]; !ok {
					err = unavailable(n, env)
				}
			}
		}
	})
	return err
}

func unavailable(name string, env EnvInfo) error {
	if env.ID == "" {
		return fmt.Errorf("variable %s is not available to agents: no environment selected", name)
	}
	return fmt.Errorf("variable %s is not available to agents in environment %s", name, env.Name)
}

// resolvePlain substitutes the plain variables and leaves each secret reference as written. It also
// returns the variable names the original request referenced.
func resolvePlain(in Input, avail map[string]available) (Input, []string) {
	values := map[string]string{}
	var secretNames []string
	for n, a := range avail {
		if a.secret {
			secretNames = append(secretNames, n)
		} else {
			values[n] = a.value
		}
	}
	sort.Strings(secretNames)
	seen := map[string]bool{}
	var used []string
	in.texts(func(s *string) {
		for _, n := range apivars.Names(*s) {
			if !seen[n] {
				seen[n] = true
				used = append(used, n)
			}
		}
		*s = apivars.Resolve(*s, values, secretNames).Text
	})
	return in, used
}

// buildPlanned parses the target and gathers what the approval and the send need.
func buildPlanned(env EnvInfo, in Input, avail map[string]available, usedBefore []string) (Planned, error) {
	display := withQuery(in.URL, in.Query, true)
	u, err := url.Parse(display)
	if err != nil {
		return Planned{}, errors.New("url is not valid; a credential cannot be part of the host")
	}
	if err := checkTarget(u); err != nil {
		return Planned{}, err
	}
	creds := referencedCredentials(in, avail)
	p := Planned{
		Env: env, Method: in.Method, URL: display, Host: u.Hostname(), Query: queryOf(u),
		Headers: in.Headers, Body: in.Body, ContentType: in.ContentType, Form: in.Form,
		Variables:       variablesUsed(usedBefore, creds, avail),
		FollowRedirects: followOf(in), TimeoutSeconds: in.TimeoutSeconds, MaxResponseBytes: in.MaxResponseBytes,
		base: in.URL, extra: in.Query, creds: creds,
	}
	for _, c := range creds {
		hosts := make([]string, len(c.rules))
		for i, r := range c.rules {
			hosts[i] = r.String()
		}
		p.Credentials = append(p.Credentials, Credential{Name: c.name, Hosts: hosts})
	}
	return p, nil
}

func followOf(in Input) bool { return in.FollowRedirects == nil || *in.FollowRedirects }

func checkTarget(u *url.URL) error {
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return errors.New("url must start with http:// or https://")
	case u.Hostname() == "":
		return errors.New("url has no host")
	case u.User != nil:
		return errors.New("url must not carry a user name or password; use a variable for a credential")
	}
	return nil
}

var refSpan = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// escapeKeepingRefs query-escapes s except for {{name}} spans, so a reference survives into the template.
func escapeKeepingRefs(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range refSpan.FindAllStringIndex(s, -1) {
		b.WriteString(url.QueryEscape(s[last:loc[0]]))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(url.QueryEscape(s[last:]))
	return b.String()
}

// withQuery appends the agent's query entries to base. With keepRefs the entries keep their {{name}}
// spans for display; otherwise they are escaped whole, for a resolved URL.
func withQuery(base string, q []NameValue, keepRefs bool) string {
	if len(q) == 0 {
		return base
	}
	esc := url.QueryEscape
	if keepRefs {
		esc = escapeKeepingRefs
	}
	parts := make([]string, len(q))
	for i, e := range q {
		parts[i] = esc(e.Name) + "=" + esc(e.Value)
	}
	sep := "?"
	switch {
	case strings.Contains(base, "?") && (strings.HasSuffix(base, "?") || strings.HasSuffix(base, "&")):
		sep = ""
	case strings.Contains(base, "?"):
		sep = "&"
	}
	return base + sep + strings.Join(parts, "&")
}

func queryOf(u *url.URL) []NameValue {
	out := []NameValue{}
	if u.RawQuery == "" {
		return out
	}
	for _, part := range strings.Split(u.RawQuery, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		out = append(out, NameValue{Name: unescapeOr(name), Value: unescapeOr(value)})
	}
	return out
}

func unescapeOr(s string) string {
	if v, err := url.QueryUnescape(s); err == nil {
		return v
	}
	return s
}

// referencedCredentials lists the secret variables the resolved request still references.
func referencedCredentials(in Input, avail map[string]available) []credential {
	seen := map[string]bool{}
	var out []credential
	in.texts(func(s *string) {
		for _, n := range apivars.Names(*s) {
			a, ok := avail[n]
			if !ok || !a.secret || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, credential{name: n, secretName: a.secretName, rules: a.hosts})
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// variablesUsed lists every variable the request referenced, plain and secret.
func variablesUsed(before []string, creds []credential, avail map[string]available) []VarUse {
	seen := map[string]bool{}
	var out []VarUse
	add := func(n string) {
		a, ok := avail[n]
		if !ok || seen[n] {
			return
		}
		seen[n] = true
		if a.secret {
			out = append(out, VarUse{Name: n, Value: secrets.Mask, Secret: true})
			return
		}
		out = append(out, VarUse{Name: n, Value: a.value})
	}
	for _, n := range before {
		add(n)
	}
	for _, c := range creds {
		add(c.name)
	}
	return out
}

// checkCredentials blocks a secret bound for a host its list does not allow.
func checkCredentials(p Planned) error {
	u, err := url.Parse(p.URL)
	if err != nil {
		return errors.New("url is not valid")
	}
	for _, c := range p.creds {
		if !allowsAny(c.rules, u) {
			return fmt.Errorf("credential %q cannot be sent to %s://%s; its allowed hosts are %s. Ask the user to allow the host in the environment's agent access",
				c.name, u.Scheme, u.Host, hostList(c.rules))
		}
	}
	return nil
}

func allowsAny(rules []apifiles.HostRule, u *url.URL) bool {
	for _, r := range rules {
		if r.Allows(u) {
			return true
		}
	}
	return false
}

func hostList(rules []apifiles.HostRule) string {
	if len(rules) == 0 {
		return "none"
	}
	names := make([]string, len(rules))
	for i, r := range rules {
		names[i] = r.String()
	}
	return strings.Join(names, ", ")
}

// redirectGuard blocks a redirect that would carry a credential to a host its list does not allow.
func redirectGuard(creds []credential) func(*url.URL) error {
	if len(creds) == 0 {
		return nil
	}
	return func(next *url.URL) error {
		for _, c := range creds {
			if !allowsAny(c.rules, next) {
				return fmt.Errorf("redirect to %s blocked: credential %q is not allowed there", next.Host, c.name)
			}
		}
		return nil
	}
}
