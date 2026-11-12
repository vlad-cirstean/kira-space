package apiaccess

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

const (
	maxRules   = 50
	maxHosts   = 20
	anyToken   = "*"
	effectDeny = "deny"
	effectOK   = "allow"

	// DefaultAsk and DefaultDeny are the values of dbMcp.apiDefault: what happens to a request no rule matches.
	DefaultAsk  = "ask"
	DefaultDeny = "deny"
)

// ValidDefault reports whether v is a dbMcp.apiDefault value.
func ValidDefault(v string) bool { return v == DefaultAsk || v == DefaultDeny }

// Verdict is the outcome of Decide for a request that no deny rule refused.
type Verdict int

const (
	// VerdictUnmatched means no rule matched and the default is ask: approval settings apply.
	VerdictUnmatched Verdict = iota
	// VerdictAllowed means an allow rule matched: the request needs no approval.
	VerdictAllowed
)

// Methods are the HTTP methods the api_request tool accepts.
var Methods = []string{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"}

// Rule is one stored API access rule. Hosts hold "*" or HostRule entries without a scheme; Methods hold "*"
// or distinct members of Methods.
type Rule struct {
	Effect  string   `json:"effect"`
	Hosts   []string `json:"hosts"`
	Methods []string `json:"methods"`
}

type compiledRule struct {
	rule    Rule
	deny    bool
	anyHost bool
	hosts   []HostRule
	anyVerb bool
}

// Policy is a validated rule list plus the default for unmatched requests. The zero value has no rules, asks by
// default and allows everything.
type Policy struct {
	rules         []compiledRule
	denyUnmatched bool
}

// WithDefault sets what happens to a request no rule matches. Any value but DefaultAsk denies, so an unknown
// value fails closed.
func (p Policy) WithDefault(d string) Policy {
	p.denyUnmatched = d != DefaultAsk
	return p
}

// AllowsAll reports whether the policy has no rules and asks by default: every request and redirect hop passes.
func (p Policy) AllowsAll() bool { return len(p.rules) == 0 && !p.denyUnmatched }

// Compile validates rules and prepares them for matching. Errors name the 1-based rule and the bad value.
func Compile(rules []Rule) (Policy, error) {
	if len(rules) > maxRules {
		return Policy{}, fmt.Errorf("at most %d rules, got %d", maxRules, len(rules))
	}
	var p Policy
	for i, r := range rules {
		c, err := compileRule(r)
		if err != nil {
			return Policy{}, fmt.Errorf("rule %d: %w", i+1, err)
		}
		p.rules = append(p.rules, c)
	}
	return p, nil
}

func compileRule(r Rule) (compiledRule, error) {
	c := compiledRule{rule: r}
	switch r.Effect {
	case effectOK:
	case effectDeny:
		c.deny = true
	default:
		return c, fmt.Errorf("effect %q: must be allow or deny", r.Effect)
	}
	if len(r.Hosts) < 1 || len(r.Hosts) > maxHosts {
		return c, fmt.Errorf("needs 1 to %d hosts, got %d", maxHosts, len(r.Hosts))
	}
	for _, h := range r.Hosts {
		if h == anyToken {
			c.anyHost = true
			continue
		}
		hr, err := ParseHostRule(h)
		if err != nil || hr.HTTP {
			return c, fmt.Errorf("host %q: not a host, host:port, *.domain or * entry", h)
		}
		c.hosts = append(c.hosts, hr)
	}
	if len(r.Methods) == 1 && r.Methods[0] == anyToken {
		c.anyVerb = true
		return c, nil
	}
	if len(r.Methods) < 1 || len(r.Methods) > len(Methods) {
		return c, fmt.Errorf("needs * or 1 to %d methods, got %d", len(Methods), len(r.Methods))
	}
	seen := map[string]bool{}
	for _, m := range r.Methods {
		if !slices.Contains(Methods, m) {
			return c, fmt.Errorf("method %q: must be * alone or one of %s", m, strings.Join(Methods, ", "))
		}
		if seen[m] {
			return c, fmt.Errorf("method %q: listed twice", m)
		}
		seen[m] = true
	}
	return c, nil
}

func (c compiledRule) matches(method string, u *url.URL) bool {
	if !c.anyVerb && !slices.Contains(c.rule.Methods, method) {
		return false
	}
	if c.anyHost {
		return true
	}
	for _, h := range c.hosts {
		if h.MatchesHost(u) {
			return true
		}
	}
	return false
}

func (c compiledRule) verbs() string {
	if c.anyVerb {
		return "any method"
	}
	return strings.Join(c.rule.Methods, ", ")
}

func (c compiledRule) hostList() string { return strings.Join(c.rule.Hosts, ", ") }

// String is the rule as shown to the agent, for example "deny DELETE on *".
func (c compiledRule) String() string {
	verbs := c.verbs()
	return c.rule.Effect + " " + verbs + " on " + c.hostList()
}

// Decide judges one request. A deny match refuses. An allow match returns VerdictAllowed. With no match the
// default decides: VerdictUnmatched under ask, a refusal under deny. The error names the method and
// scheme://host only, never path or query.
func (p Policy) Decide(method string, u *url.URL) (Verdict, error) {
	target := method + " " + u.Scheme + "://" + u.Host
	allowed := false
	for _, c := range p.rules {
		if !c.matches(method, u) {
			continue
		}
		if c.deny {
			return VerdictUnmatched, fmt.Errorf("%s is denied by the API access rule %q", target, c.String())
		}
		allowed = true
	}
	if allowed {
		return VerdictAllowed, nil
	}
	if !p.denyUnmatched {
		return VerdictUnmatched, nil
	}
	return VerdictUnmatched, errors.New(target + " is not allowed by the API access rules in Kira Space's Studio MCP settings: " +
		"no rule matches it, and requests no rule matches are denied. " + p.allowedText() + ". Ask the user to add an allow rule")
}

// CheckHop judges one redirect hop with the request's own method. A deny match refuses and an allow match
// follows. With no match the hop is refused, unless the policy allows everything or the request went through
// approval (approved) and the hop stays on the host and port of the request, origin, the user approved. An
// allow-matched request skipped the prompt, so its redirect must not leave the hosts the user vetted.
func (p Policy) CheckHop(method string, u, origin *url.URL, approved bool) error {
	if p.AllowsAll() {
		return nil
	}
	target := method + " " + u.Scheme + "://" + u.Host
	allowed := false
	for _, c := range p.rules {
		if !c.matches(method, u) {
			continue
		}
		if c.deny {
			return fmt.Errorf("%s is denied by the API access rule %q", target, c.String())
		}
		allowed = true
	}
	if allowed {
		return nil
	}
	if approved {
		if origin != nil && strings.EqualFold(u.Host, origin.Host) {
			return nil
		}
		return errors.New(target + " matches no allow rule in Kira Space's Studio MCP settings, " +
			"and a redirect must stay on the approved host or go to a host an allow rule matches")
	}
	return errors.New(target + " matches no allow rule in Kira Space's Studio MCP settings, " +
		"and a redirect must go to a host an allow rule matches")
}

func (p Policy) allowedText() string {
	var parts []string
	for _, c := range p.rules {
		if !c.deny {
			parts = append(parts, c.verbs()+" on "+c.hostList())
		}
	}
	if len(parts) == 0 {
		return "Allowed: nothing, there is no allow rule"
	}
	return "Allowed: " + strings.Join(parts, "; ")
}
